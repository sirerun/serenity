package supersede

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

type reconcilePublication struct {
	Version  int                 `json:"version"`
	Decision string              `json:"decision"`
	ClaimID  string              `json:"claim_id"`
	Complete bool                `json:"complete"`
	Changes  []writer.FileChange `json:"changes"`
}

// ApplyAndCommitReconcile publishes a recorded human decision. Its durable
// before/after plan is saved before canonical writes, making interruption and
// Git failure retryable without interpreting current human edits as our output.
// It deliberately does not re-dispose the item or add another history record.
func (w *Writer) ApplyAndCommitReconcile(ctx context.Context, dispStore *disposition.Store, item disposition.Item, now time.Time) (string, error) {
	a, b, err := acceptedReconcileClaims(item, now)
	if err != nil {
		return "", err
	}
	if !safeClaimPart(a.SubjectSlug) || !safeClaimPart(a.Family) || a.SubjectSlug != b.SubjectSlug || a.Predicate != b.Predicate || a.Family != b.Family {
		return "", fmt.Errorf("reconcile: invalid or mismatched claim identity")
	}
	root := w.Fence.Root
	runtime, err := openPublicationRuntime(root)
	if err != nil {
		return "", err
	}
	defer func() { _ = runtime.Close() }()
	lock, err := lockPublication(runtime)
	if err != nil {
		return "", err
	}
	defer func() { _ = lock.Close() }()
	// Re-read after locking: another reviewer may have won since the UI loaded.
	item, err = dispStore.Get(ctx, item.ID)
	if err != nil {
		return "", err
	}
	a, b, err = acceptedReconcileClaims(item, now)
	if err != nil {
		return "", err
	}
	if !safeClaimPart(a.SubjectSlug) || !safeClaimPart(a.Family) || a.SubjectSlug != b.SubjectSlug || a.Predicate != b.Predicate || a.Family != b.Family {
		return "", fmt.Errorf("reconcile: invalid or mismatched claim identity")
	}
	decision, err := json.Marshal(struct {
		ID         string
		Verdict    disposition.Verdict
		Actor      string
		DisposedAt time.Time
		A, B       domain.Claim
	}{item.ID, item.Verdict, item.Actor, item.DisposedAt, a, b})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(decision)
	fingerprint := hex.EncodeToString(sum[:])
	idSum := sha256.Sum256([]byte(item.ID))
	name := hex.EncodeToString(idSum[:]) + ".json"
	publication := reconcilePublication{}
	if info, statErr := runtime.Lstat(name); statErr == nil && !info.Mode().IsRegular() {
		return "", fmt.Errorf("reconcile: unsafe publication receipt")
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", statErr
	}
	raw, err := runtime.ReadFile(name)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &publication); err != nil {
			return "", fmt.Errorf("reconcile: corrupt publication receipt: %w", err)
		}
		if publication.Version != 1 || publication.Decision != fingerprint || publication.ClaimID != a.ID || len(publication.Changes) == 0 {
			return "", fmt.Errorf("reconcile: publication receipt does not match recorded decision")
		}
	case errors.Is(err, fs.ErrNotExist):
		changes, err := w.preparePublication(a, b)
		if err != nil {
			return "", err
		}
		publication = reconcilePublication{Version: 1, Decision: fingerprint, ClaimID: a.ID, Changes: changes}
		if err := savePublication(runtime, name, publication); err != nil {
			return "", err
		}
	default:
		return "", err
	}
	if !publication.Complete {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		q := writer.NewQueue(nil)
		defer q.Close()
		if err := writer.PublishFiles(q, root, publication.Changes); err != nil {
			return "", err
		}
		if _, err := writer.Flush(q, root); err != nil {
			return "", fmt.Errorf("reconcile: canonical publication awaits commit: %w", err)
		}
		publication.Complete = true
		if err := savePublication(runtime, name, publication); err != nil {
			return "", err
		}
	}
	if err := dispStore.RecordResultClaimID(ctx, item.ID, publication.ClaimID, now); err != nil {
		return "", err
	}
	return publication.ClaimID, nil
}

func safeClaimPart(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, "/\\\x00*?[]")
}

func openPublicationRuntime(root string) (*os.Root, error) {
	dir, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()
	for _, path := range []string{".serenity", ".serenity/reconcile"} {
		if info, err := dir.Lstat(path); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return nil, fmt.Errorf("reconcile: unsafe runtime directory %s", path)
		} else if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if err := dir.MkdirAll(path, 0700); err != nil {
			return nil, err
		}
	}
	return dir.OpenRoot(".serenity/reconcile")
}

func savePublication(root *os.Root, name string, value reconcilePublication) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	temp := name + ".tmp"
	// The operation lock owns this staging name, including a predecessor killed
	// before rename. A symlink is refused instead of followed or deleted.
	if info, err := root.Lstat(temp); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("reconcile: unsafe receipt staging file")
		}
		if err := root.Remove(temp); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close(); _ = root.Remove(temp) }()
	if _, err := file.Write(raw); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := root.Rename(temp, name); err != nil {
		return err
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}

// preparePublication reuses the real tier writers in an isolated snapshot. Only
// this subject's entity page and shard files are copied; there is no model call,
// alternate serializer or production mock. The resulting bytes are the journal.
func (w *Writer) preparePublication(a, b domain.Claim) ([]writer.FileChange, error) {
	root := w.Fence.Root
	paths, err := filepath.Glob(filepath.Join(root, "brain", "entities", "*", a.SubjectSlug+".md"))
	if err != nil {
		return nil, err
	}
	if len(paths) > 1 {
		return nil, fmt.Errorf("reconcile: multiple entity pages for %s", a.SubjectSlug)
	}
	entityType := w.entityType(a.SubjectSlug)
	if len(paths) == 1 {
		entityType = filepath.Base(filepath.Dir(paths[0]))
	}
	if !safeClaimPart(entityType) {
		return nil, fmt.Errorf("reconcile: invalid entity type")
	}
	pagePath := w.Fence.PathFor(entityType, a.SubjectSlug)
	snapshot := map[string][]byte{}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = dir.Close() }()
	add := func(path string) error {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		// Every ancestor is checked; os.Root also prevents an escaping symlink race.
		pieces := strings.Split(rel, "/")
		for i := range pieces {
			info, err := dir.Lstat(strings.Join(pieces[:i+1], "/"))
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("reconcile: symlink in canonical snapshot")
			}
		}
		raw, err := dir.ReadFile(rel)
		if err != nil {
			return err
		}
		snapshot[rel] = raw
		return nil
	}
	if len(paths) == 1 {
		if err := add(pagePath); err != nil {
			return nil, err
		}
	}
	claimsDir := filepath.Join(root, "brain", "claims", a.SubjectSlug)
	err = filepath.WalkDir(claimsDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if errors.Is(walkErr, fs.ErrNotExist) && path == claimsDir {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("reconcile: symlink in shard snapshot")
		}
		return add(path)
	})
	if err != nil {
		return nil, err
	}
	if len(snapshot) == 0 {
		return nil, fmt.Errorf("reconcile: prior canonical claim is missing")
	}
	for path := range snapshot {
		cmd := exec.Command("git", "status", "--porcelain", "--", path)
		cmd.Dir = root
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("reconcile: cannot verify canonical Git state: %w", err)
		}
		if len(bytes.TrimSpace(out)) != 0 {
			return nil, fmt.Errorf("%w: %s; preserve or resolve the edit before retry", writer.ErrDirtyTree, path)
		}
	}
	var current *domain.Claim
	priorMatches := 0
	if w.Config.TierOf(a.Family) == domain.TierShard {
		heads, err := w.Shard.ResolveHeads(a.SubjectSlug, a.Family)
		if err != nil {
			return nil, err
		}
		lines, err := w.Shard.Lines(a.SubjectSlug, a.Family)
		if err != nil {
			return nil, err
		}
		for _, line := range lines {
			if line.ID == a.ID {
				return nil, fmt.Errorf("reconcile: replacement claim identity already exists")
			}
		}
		for _, head := range heads {
			if head.ID == b.ID {
				priorMatches++
				copy := head
				current = &copy
			}
		}
	} else {
		page, err := w.Fence.ParseEntity(pagePath)
		if err != nil {
			return nil, err
		}
		for _, claim := range page.Claims {
			if claim.ID == a.ID {
				return nil, fmt.Errorf("reconcile: replacement claim identity already exists")
			}
			if claim.ID == b.ID {
				priorMatches++
				copy := claim
				current = &copy
			}
		}
	}
	if current == nil || current.State != domain.StateActive || priorMatches != 1 {
		return nil, fmt.Errorf("reconcile: prior claim %s is no longer active", b.ID)
	}
	if !sameCanonicalClaim(w.Fence, *current, b, entityType) {
		return nil, fmt.Errorf("reconcile: prior claim %s changed since the proposal", b.ID)
	}
	if a.ID == b.ID {
		return nil, fmt.Errorf("reconcile: replacement has the prior claim's identity")
	}
	return writer.PlanFiles(root, snapshot, func(preview string) error {
		q := writer.NewQueue(nil)
		defer q.Close()
		fw, ss := store.NewFenceWriter(preview), store.NewShardStore(preview)
		fw.Vocabulary = w.Fence.Vocabulary
		ss.Vocabulary = w.Shard.Vocabulary
		ss.RolloverBytes = w.Shard.RolloverBytes
		planned := New(q, fw, ss, w.Config)
		planned.EntityType = func(string) string { return entityType }
		_, err := planned.Apply(a, b)
		return err
	})
}

func sameCanonicalClaim(fw *store.FenceWriter, a, b domain.Claim, entityType string) bool {
	render := func(claim domain.Claim) ([]byte, error) {
		page := store.NewEntityPage(domain.Entity{Type: entityType, Slug: claim.SubjectSlug})
		page.Claims = []domain.Claim{claim}
		return fw.RenderEntity(page)
	}
	ar, ae := render(a)
	br, be := render(b)
	return ae == nil && be == nil && bytes.Equal(ar, br)
}
