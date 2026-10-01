package pool

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirerun/serenity/internal/gitrun"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/store"
	"gopkg.in/yaml.v3"
)

// Check proves a remember operation only from the captured canonical HEAD.
// It deliberately does not inspect source bytes in the working tree or infer
// that an entered operation is absent after cancellation/forget/history purge.
func (r *Runtime) Check(ctx context.Context, rec contracts.OperationRecord) (contracts.CanonicalVerdict, error) {
	unknown := contracts.CanonicalVerdict{Outcome: contracts.CanonicalUnknown}
	if ctx == nil {
		return unknown, errors.New("hosted pool: nil canonical-check context")
	}
	if rec.BrainID != r.brainID || r.brainID == "" || rec.Source != "gateway.remember" || !store.ValidMemoryOperationKey(rec.ID) || rec.ID == "" {
		return unknown, nil
	}
	head, factID, found, safe := findCommittedOperation(ctx, r.Root, rec.ID)
	if ctx.Err() != nil {
		return unknown, ctx.Err()
	}
	if !safe {
		return unknown, nil
	}
	if found {
		if rec.CanonicalEnteredAt.IsZero() {
			return unknown, nil
		}
		return contracts.CanonicalVerdict{Outcome: contracts.CanonicalLanded, Ref: "fact:" + factID}, nil
	}
	if !rec.CanonicalEnteredAt.IsZero() {
		return unknown, nil
	}
	return contracts.CanonicalVerdict{Outcome: contracts.CanonicalAbsent, Ref: head}, nil
}

type treeBlob struct {
	mode string
	sha  string
	path string
}

type sourceTree struct {
	meta  []byte
	bytes []byte
}

func findCommittedOperation(ctx context.Context, root, operationID string) (head, factID string, found, safe bool) {
	if err := validateCanonicalRepository(root); err != nil {
		return "", "", false, false
	}
	runner := gitrun.CanonicalReadOnly(root)
	rawHead, err := runner.Output(ctx, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", "", false, false
	}
	head = strings.TrimSpace(string(rawHead))
	if !validObjectID(head) {
		return "", "", false, false
	}
	refs, err := runner.Output(ctx, "for-each-ref", "--format=%(refname)", "refs/replace")
	if err != nil || strings.TrimSpace(string(refs)) != "" {
		return "", "", false, false
	}
	rawTree, err := runner.Output(ctx, "ls-tree", "-r", "-z", "--full-tree", head, "--", "brain/sources")
	if err != nil {
		return "", "", false, false
	}
	blobs, ok := parseSourceTree(string(rawTree))
	if !ok {
		return "", "", false, false
	}
	sources := make(map[string]*sourceTree)
	for _, blob := range blobs {
		parts := strings.Split(blob.path, "/")
		sha := parts[3]
		source := sources[sha]
		if source == nil {
			source = &sourceTree{}
			sources[sha] = source
		}
		data, err := runner.Output(ctx, "cat-file", "blob", blob.sha)
		if err != nil {
			return "", "", false, false
		}
		switch parts[4] {
		case "meta.yaml":
			source.meta = data
		case "bytes":
			source.bytes = data
		}
	}
	var exact []string
	for sha, source := range sources {
		if len(source.meta) == 0 || len(source.bytes) == 0 {
			return "", "", false, false
		}
		var meta struct {
			Kind string `yaml:"kind"`
		}
		if err := yaml.Unmarshal(source.meta, &meta); err != nil || strings.TrimSpace(meta.Kind) == "" {
			return "", "", false, false
		}
		var envelope struct {
			RecordType string `json:"record_type"`
		}
		jsonErr := json.Unmarshal(source.bytes, &envelope)
		if meta.Kind != store.SourceKindMemoryFact {
			if jsonErr == nil && envelope.RecordType == store.SourceKindMemoryFact {
				return "", "", false, false
			}
			continue
		}
		payload, err := store.DecodeMemoryFact(source.bytes)
		if err != nil {
			return "", "", false, false
		}
		digest := sha256.Sum256(source.bytes)
		if hex.EncodeToString(digest[:]) != sha {
			return "", "", false, false
		}
		idMatch := payload.CanonicalOperationID == operationID
		keyMatch := payload.OperationKey == operationID
		if idMatch != keyMatch {
			return "", "", false, false
		}
		if idMatch && keyMatch {
			exact = append(exact, sha)
		}
	}
	if len(exact) > 1 {
		return "", "", false, false
	}
	if len(exact) == 1 {
		return head, exact[0], true, true
	}
	return head, "", false, true
}

func parseSourceTree(raw string) ([]treeBlob, bool) {
	if raw == "" {
		return nil, true
	}
	entries := strings.Split(raw, "\x00")
	if entries[len(entries)-1] != "" {
		return nil, false
	}
	seen := make(map[string]bool, len(entries)-1)
	blobs := make([]treeBlob, 0, len(entries)-1)
	for _, entry := range entries[:len(entries)-1] {
		meta, path, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 || fields[1] != "blob" || fields[0] != "100644" || !validObjectID(fields[2]) {
			return nil, false
		}
		parts := strings.Split(path, "/")
		if len(parts) != 5 || parts[0] != "brain" || parts[1] != "sources" || !store.ValidSourceSHA(parts[3]) || parts[2] != parts[3][:2] || (parts[4] != "meta.yaml" && parts[4] != "bytes") {
			return nil, false
		}
		if seen[path] {
			return nil, false
		}
		seen[path] = true
		blobs = append(blobs, treeBlob{mode: fields[0], sha: fields[2], path: path})
	}
	return blobs, true
}

func validObjectID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func validateCanonicalRepository(root string) error {
	gitDir := filepath.Join(root, ".git")
	info, err := os.Lstat(gitDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("unsafe canonical git directory")
	}
	for _, rel := range []string{"commondir", "objects/info/alternates", "objects/info/http-alternates"} {
		if _, err := os.Lstat(filepath.Join(gitDir, rel)); err == nil || !errors.Is(err, os.ErrNotExist) {
			return errors.New("canonical git metadata redirects object lookup")
		}
	}
	objects := filepath.Join(gitDir, "objects")
	objectInfo, err := os.Lstat(objects)
	if err != nil || !objectInfo.IsDir() || objectInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("unsafe canonical object directory")
	}
	for _, rel := range []string{"objects/info", "objects/pack"} {
		path := filepath.Join(gitDir, rel)
		if info, err := os.Lstat(path); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return errors.New("unsafe canonical object metadata directory")
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	packDir := filepath.Join(objects, "pack")
	if entries, err := os.ReadDir(packDir); err == nil {
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".promisor") {
				return errors.New("promisor object store is unsupported for canonical proof")
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return validateCanonicalGitConfig(filepath.Join(gitDir, "config"))
}

func validateCanonicalGitConfig(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	section := ""
	seen := make(map[string]bool)
	coreFormat := false
	for lineNo, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")))
			if section != "core" && section != "user" {
				return fmt.Errorf("canonical git config has unsupported section on line %d", lineNo+1)
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || section == "" {
			return fmt.Errorf("canonical git config has malformed line %d", lineNo+1)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		full := section + "." + key
		if seen[full] {
			return fmt.Errorf("canonical git config repeats %s", full)
		}
		seen[full] = true
		switch full {
		case "core.repositoryformatversion":
			if value != "0" {
				return errors.New("unsupported canonical git repository format")
			}
			coreFormat = true
		case "core.filemode", "core.logallrefupdates":
			if value != "true" && value != "false" {
				return fmt.Errorf("invalid canonical git config value for %s", full)
			}
		case "core.bare":
			if value != "false" {
				return errors.New("canonical git repository must be non-bare")
			}
		case "user.name", "user.email":
			if value == "" {
				return fmt.Errorf("empty canonical git config value for %s", full)
			}
		default:
			return fmt.Errorf("unsupported canonical git config key %s", full)
		}
	}
	if !coreFormat {
		return errors.New("canonical git config lacks repository format version")
	}
	return nil
}

var _ contracts.CanonicalChecker = (*Runtime)(nil)
var _ contracts.BrainFence = (*Runtime)(nil)
