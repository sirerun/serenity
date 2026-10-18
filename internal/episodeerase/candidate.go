// Package episodeerase constructs inactive, isolated Git erasure candidates.
// It provides no activation or physical-purge receipt. See DESIGN.md.
package episodeerase

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/sirerun/serenity/internal/gitrun"
	"github.com/sirerun/serenity/internal/store"
)

var (
	ErrUnsafe = errors.New("episodeerase: unsupported or unsafe repository")
	ErrFence  = errors.New("episodeerase: incomplete, stale or mismatched current fence input")
	ErrLimit  = errors.New("episodeerase: qualification limit exceeded")
)

const maxBytes = 64 << 20
const maxTotal = 256 << 20
const maxCommits = 128

// Request must come from a caller holding exclusive maintenance ownership of
// Source and Destination's parent. CurrentFences is a complete trusted snapshot,
// not fences recovered from Source. This package cannot authenticate freshness.
type Request struct {
	Source        string
	Destination   string
	BrainID       string
	CurrentFences []store.EpisodeRecord
}

// Result is preparation evidence only. Destination is an inert directory
// containing repository.git and manifest.json, never an active brain.
type Result struct {
	Schema                    string      `json:"schema"`
	RetryID                   string      `json:"retry_id"`
	BrainID                   string      `json:"brain_id"`
	SourceHead                string      `json:"source_head"`
	CandidateHead             string      `json:"candidate_head"`
	FenceDigest               string      `json:"fence_digest"`
	Generation                uint64      `json:"generation"`
	Commits                   int         `json:"commits"`
	RemovedSources            []string    `json:"removed_sources"`
	RetainedAttachmentDigests []string    `json:"retained_attachment_digests"`
	Activation                string      `json:"activation"`
	PhysicalPurge             string      `json:"physical_purge"`
	Derivatives               string      `json:"derivatives"`
	RetainedSources           []SourcePin `json:"retained_sources,omitempty"`
}

type budget struct{ used int }
type limitedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *limitedBuffer) Len() int       { return b.buffer.Len() }
func (b *limitedBuffer) Bytes() []byte  { return b.buffer.Bytes() }
func (b *limitedBuffer) String() string { return b.buffer.String() }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		return 0, ErrLimit
	}
	return b.buffer.Write(p)
}
func (b *budget) git(ctx context.Context, r *gitrun.Runner, input []byte, args ...string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cmd, err := r.Command(ctx, args...)
	if err != nil {
		return nil, err
	}
	out := &limitedBuffer{limit: maxBytes}
	stderr := &limitedBuffer{limit: 4096}
	cmd.Stdout = out
	cmd.Stderr = stderr
	cmd.Stdin = bytes.NewReader(input)
	if err = cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, ErrLimit) {
			return nil, ErrLimit
		}
		// Never expose repository text through Git stderr.
		return nil, fmt.Errorf("%w: git %s failed", ErrUnsafe, args[0])
	}
	b.used += out.Len()
	if b.used > maxTotal {
		return nil, ErrLimit
	}
	return out.Bytes(), nil
}

// Build constructs, verifies, syncs and atomically publishes an INACTIVE
// candidate directory. Retry with the same source/fences has the same RetryID;
// existing destinations are refused rather than trusted or overwritten.
func Build(ctx context.Context, req Request) (Result, error) {
	return build(ctx, req, nil)
}

func build(ctx context.Context, req Request, mixed *mixedSources) (result Result, err error) {
	if !store.ValidEpisodeID(req.BrainID) || len(req.CurrentFences) == 0 || len(req.CurrentFences) > store.EpisodeMaxFences {
		return result, ErrFence
	}
	// Reject non-fence payloads before encoding: a typed caller must not turn
	// the snapshot copy into an unbounded allocation or content-bearing state.
	for _, f := range req.CurrentFences {
		if f.Schema != store.EpisodeSchema || f.Kind != "exclude" || f.BrainID != req.BrainID || f.Input != nil || len(f.Refs) != 0 || !store.ValidSourceSHA(f.CommandKey) || !store.ValidSourceSHA(f.ConversationKey) || !store.ValidSourceSHA(f.RequestDigest) || f.RecordedAt.IsZero() {
			return result, ErrFence
		}
	}
	// Detach caller-owned slices before work.
	encoded, e := json.Marshal(req.CurrentFences)
	if e != nil {
		return result, ErrFence
	}
	if len(encoded) > 1<<20 {
		return result, ErrLimit
	}
	var fences []store.EpisodeRecord
	if json.Unmarshal(encoded, &fences) != nil {
		return result, ErrFence
	}
	sort.Slice(fences, func(i, j int) bool { return fences[i].Generation < fences[j].Generation })
	fenceMap := map[string]store.EpisodeRecord{}
	for i, f := range fences {
		if f.Kind != "exclude" || f.BrainID != req.BrainID || f.Generation != uint64(i+1) || fenceMap[f.ConversationKey].Generation != 0 {
			return result, ErrFence
		}
		fenceMap[f.ConversationKey] = f
	}
	source, e := safeDirectory(req.Source)
	if e != nil {
		return result, e
	}
	destination, e := filepath.Abs(req.Destination)
	if e != nil {
		return result, ErrUnsafe
	}
	parent, e := safeDirectory(filepath.Dir(destination))
	if e != nil {
		return result, e
	}
	if destination == source || strings.HasPrefix(destination, source+string(os.PathSeparator)) || strings.HasPrefix(source, destination+string(os.PathSeparator)) {
		return result, ErrUnsafe
	}
	if _, e = os.Lstat(destination); !errors.Is(e, os.ErrNotExist) {
		return result, ErrUnsafe
	}
	if e = inspectSource(ctx, source); e != nil {
		return result, e
	}
	b := &budget{}
	read := gitrun.CanonicalReadOnly(source)
	head, e := qualifyGit(ctx, b, read, source)
	if e != nil {
		return result, e
	}
	history, e := b.git(ctx, read, nil, "rev-list", "--reverse", "--parents", "refs/heads/main")
	if e != nil {
		return result, e
	}
	lines := strings.FieldsFunc(strings.TrimSpace(string(history)), func(r rune) bool { return r == '\n' })
	if len(lines) == 0 || len(lines) > maxCommits {
		return result, ErrLimit
	}
	previous := ""
	commits := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) != 1 && len(fields) != 2 {
			return result, ErrUnsafe
		}
		if !gitID(fields[0]) || (previous == "" && len(fields) != 1) || (previous != "" && (len(fields) != 2 || fields[1] != previous)) {
			return result, ErrUnsafe
		}
		previous = fields[0]
		commits = append(commits, previous)
	}
	stage, e := os.MkdirTemp(parent, ".episode-erase-stage-")
	if e != nil {
		return result, e
	}
	defer func() {
		if stage != "" {
			err = errors.Join(err, os.RemoveAll(stage))
		}
	}()
	fenceRoot := filepath.Join(stage, "fences")
	if e = os.Mkdir(fenceRoot, 0700); e != nil {
		return result, e
	}
	for _, f := range fences {
		if _, e = store.NewSourceStore(fenceRoot).PutEpisode(f); e != nil {
			return result, ErrFence
		}
	}
	if _, e = store.LoadEpisodes(ctx, store.NewSourceStore(fenceRoot)); e != nil {
		if ctx.Err() != nil {
			return Result{}, ctx.Err()
		}
		return result, ErrFence
	}
	fenceFiles, e := readFiles(ctx, fenceRoot)
	if e != nil {
		return result, e
	}
	repo := filepath.Join(stage, "repository.git")
	if e = os.Mkdir(repo, 0700); e != nil {
		return result, e
	}
	write := gitrun.Quarantine(repo)
	// An explicit empty template prevents installation of configured templates.
	template := filepath.Join(stage, "template")
	if e = os.Mkdir(template, 0700); e != nil {
		return result, e
	}
	if _, e = b.git(ctx, write, nil, "init", "--bare", "--object-format=sha1", "--initial-branch=main", "--template="+template); e != nil {
		return result, e
	}
	result = Result{Schema: "serenity.episode-erasure-candidate/v1", BrainID: req.BrainID, SourceHead: head, FenceDigest: store.EpisodeHash(fences), Generation: uint64(len(fences)), Activation: "not_activated", PhysicalPurge: "unsupported", Derivatives: "candidate_contains_canonical_episode_sources_only", RemovedSources: []string{}, RetainedAttachmentDigests: []string{}}
	result.RetryID = store.EpisodeHash([]string{result.Schema, req.BrainID, head, result.FenceDigest})
	if mixed != nil {
		result.Schema = "serenity.mixed-episode-erasure-candidate/v1"
		result.Derivatives = "candidate_contains_canonical_episodes_and_pinned_independent_raw_sources_only"
		result.RetainedSources = append([]SourcePin(nil), mixed.pins...)
		result.RetryID = store.EpisodeHash(struct {
			EpisodeRetryID string
			Schema         string
			Pins           []SourcePin
		}{result.RetryID, result.Schema, mixed.pins})
	}
	removed := map[string]bool{}
	attachments := map[string]bool{}
	oldBlobs := map[string]bool{}
	keptBlobs := map[string]bool{}
	previous = ""
	for _, commit := range commits {
		if e = ctx.Err(); e != nil {
			return Result{}, e
		}
		files, e := snapshot(ctx, b, read, commit)
		if e != nil {
			return Result{}, e
		}
		root, e := os.MkdirTemp(stage, "snapshot-")
		if e != nil {
			return Result{}, e
		}
		if e = materialize(root, files); e != nil {
			return Result{}, e
		}
		p, e := store.LoadEpisodes(ctx, store.NewSourceStore(root))
		if e != nil {
			if ctx.Err() != nil {
				return Result{}, ctx.Err()
			}
			return Result{}, ErrUnsafe
		}
		if p.BrainID != "" && p.BrainID != req.BrainID {
			return Result{}, ErrFence
		}
		if mixed == nil {
			if len(files) != 2*len(p.SourceIDs) {
				return Result{}, ErrUnsafe
			}
		} else if e = mixed.validateSnapshot(root, files, p.SourceIDs); e != nil {
			return Result{}, e
		}
		// Historical fences must be a byte-identical subset of current authority.
		for _, r := range p.Records {
			if r.Generation > result.Generation {
				return Result{}, ErrFence
			}
			if r.Kind == "exclude" && store.EpisodeHash(r) != store.EpisodeHash(fenceMap[r.ConversationKey]) {
				return Result{}, ErrFence
			}
		}
		for key := range p.Fences {
			delete(p.Fences, key)
		}
		for key, f := range fenceMap {
			p.Fences[key] = f.Generation
		}
		for key, r := range p.Records {
			sha := p.SourceIDs[key]
			base := sourceBase(sha)
			if _, ok := files[base+"bytes"]; !ok {
				return Result{}, ErrUnsafe
			}
			if _, ok := files[base+"meta.yaml"]; !ok {
				return Result{}, ErrUnsafe
			}
			if e = store.ValidateEpisodeErasureSource(r, files[base+"bytes"], files[base+"meta.yaml"]); e != nil {
				return Result{}, ErrUnsafe
			}
			if r.Kind == "exclude" || !p.RecordEligible(r) {
				for _, name := range []string{"bytes", "meta.yaml"} {
					raw := files[base+name]
					// Record original Git blob identities without ever copying them.
					oldBlobs[blobID(raw)] = true
					delete(files, base+name)
				}
				if r.Kind != "exclude" {
					removed[sha] = true
				}
			} else if r.Input != nil {
				for _, ev := range r.Input.Events {
					for _, a := range ev.Attachments {
						attachments[a.SHA256] = true
					}
				}
			}
		}
		for path, data := range fenceFiles {
			files[path] = data
		}
		// Validate closure after deletion, including re-import and checkpoint refs.
		if e = os.RemoveAll(root); e != nil {
			return Result{}, e
		}
		if e = os.Mkdir(root, 0700); e != nil {
			return Result{}, e
		}
		if e = materialize(root, files); e != nil {
			return Result{}, e
		}
		if _, e = store.LoadEpisodes(ctx, store.NewSourceStore(root)); e != nil {
			if ctx.Err() != nil {
				return Result{}, ctx.Err()
			}
			return Result{}, ErrUnsafe
		}
		if e = os.RemoveAll(root); e != nil {
			return Result{}, e
		}
		tree, e := writeTree(ctx, b, write, files, keptBlobs)
		if e != nil {
			return Result{}, e
		}
		// Construct only fixed, content-free commit metadata through Git
		// plumbing. Do not override gitrun's hardened command environment.
		commitBytes := "tree " + tree + "\n"
		if previous != "" {
			commitBytes += "parent " + previous + "\n"
		}
		commitBytes += "author Serenity <episode-erasure@invalid> 946684800 +0000\ncommitter Serenity <episode-erasure@invalid> 946684800 +0000\n\nIsolated episode erasure candidate\n"
		id, e := b.git(ctx, write, []byte(commitBytes), "hash-object", "-t", "commit", "-w", "--stdin")
		if e != nil {
			return Result{}, e
		}
		previous = strings.TrimSpace(string(id))
		if !gitID(previous) {
			return Result{}, ErrUnsafe
		}
	}
	if mixed != nil && len(mixed.seen) != len(mixed.pins) {
		return Result{}, ErrUnsafe
	}
	if _, e = b.git(ctx, write, nil, "update-ref", "refs/heads/main", previous); e != nil {
		return Result{}, e
	}
	result.CandidateHead = previous
	result.Commits = len(commits)
	result.RemovedSources = sortedKeys(removed)
	result.RetainedAttachmentDigests = sortedKeys(attachments)
	if e = verifyCandidate(ctx, b, repo, previous, len(commits), oldBlobs, keptBlobs); e != nil {
		return Result{}, e
	}
	// Recheck source cleanliness and identity before publishing preparation.
	now, e := qualifyGit(ctx, b, read, source)
	if e != nil {
		return Result{}, e
	}
	if now != head {
		return Result{}, ErrUnsafe
	}
	for _, path := range []string{fenceRoot, template} {
		if e = os.RemoveAll(path); e != nil {
			return Result{}, e
		}
	}
	manifest, e := json.MarshalIndent(result, "", "  ")
	if e != nil {
		return Result{}, e
	}
	if e = os.WriteFile(filepath.Join(stage, "manifest.json"), append(manifest, '\n'), 0600); e != nil {
		return Result{}, e
	}
	if e = syncTree(ctx, stage); e != nil {
		return Result{}, e
	}
	if e = ctx.Err(); e != nil {
		return Result{}, e
	}
	// Exclusive parent ownership is required; recheck absence and never adopt an
	// existing candidate as evidence. Rename is atomic within this parent.
	if _, e = os.Lstat(destination); !errors.Is(e, os.ErrNotExist) {
		return Result{}, ErrUnsafe
	}
	if e = os.Rename(stage, destination); e != nil {
		return Result{}, e
	}
	stage = ""
	if e = syncDir(parent); e != nil {
		return Result{}, e
	}
	return result, nil
}

func gitID(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}
func sourceBase(sha string) string { return "brain/sources/" + sha[:2] + "/" + sha + "/" }
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func safeDirectory(path string) (string, error) {
	a, e := filepath.Abs(path)
	if e != nil {
		return "", ErrUnsafe
	}
	resolved, e := filepath.EvalSymlinks(a)
	if e != nil || resolved != a {
		return "", ErrUnsafe
	}
	st, e := os.Lstat(a)
	if e != nil || !st.IsDir() {
		return "", ErrUnsafe
	}
	return a, nil
}

func inspectSource(ctx context.Context, root string) error {
	count := 0
	var total int64
	if st, e := os.Lstat(filepath.Join(root, ".git")); e != nil || !st.IsDir() {
		return ErrUnsafe
	}
	for _, name := range []string{"objects/info/alternates", "objects/info/http-alternates", "shallow", "info/grafts", "commondir", "worktrees", "modules"} {
		if _, e := os.Lstat(filepath.Join(root, ".git", name)); !errors.Is(e, os.ErrNotExist) {
			return ErrUnsafe
		}
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return ErrUnsafe
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		count++
		if count > 65536 {
			return ErrLimit
		}
		info, e := d.Info()
		if e != nil {
			return ErrUnsafe
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return ErrUnsafe
		}
		if strings.HasPrefix(path, filepath.Join(root, ".git", "objects")+string(os.PathSeparator)) && strings.HasSuffix(d.Name(), ".promisor") {
			return ErrUnsafe
		}
		if info.Mode().IsRegular() {
			total += info.Size()
			if total > maxTotal {
				return ErrLimit
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	cfg, e := os.ReadFile(filepath.Join(root, ".git", "config"))
	if e != nil {
		return ErrUnsafe
	}
	allowed := map[string]bool{"[core]": true, "repositoryformatversion = 0": true, "filemode = true": true, "filemode = false": true, "bare = false": true, "logallrefupdates = true": true, "ignorecase = true": true, "ignorecase = false": true, "precomposeunicode = true": true, "precomposeunicode = false": true}
	for _, line := range strings.Split(string(cfg), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !allowed[line] {
			return ErrUnsafe
		}
	}
	return nil
}

func qualifyGit(ctx context.Context, b *budget, r *gitrun.Runner, root string) (string, error) {
	top, e := b.git(ctx, r, nil, "rev-parse", "--show-toplevel")
	if e != nil || strings.TrimSpace(string(top)) != root {
		return "", ErrUnsafe
	}
	status, e := b.git(ctx, r, nil, "status", "--porcelain=v1", "--untracked-files=all", "--ignored")
	if e != nil {
		return "", e
	}
	if len(status) != 0 {
		return "", ErrUnsafe
	}
	refs, e := b.git(ctx, r, nil, "for-each-ref", "--format=%(refname) %(objectname)")
	if e != nil {
		return "", e
	}
	fields := strings.Fields(string(refs))
	if len(fields) != 2 || fields[0] != "refs/heads/main" || !gitID(fields[1]) {
		return "", ErrUnsafe
	}
	head, e := b.git(ctx, r, nil, "rev-parse", "HEAD")
	if e != nil || strings.TrimSpace(string(head)) != fields[1] {
		return "", ErrUnsafe
	}
	symbolic, e := os.ReadFile(filepath.Join(root, ".git", "HEAD"))
	if e != nil || string(symbolic) != "ref: refs/heads/main\n" {
		return "", ErrUnsafe
	}
	if _, e = allObjects(ctx, b, r); e != nil {
		return "", e
	}
	return fields[1], nil
}

func allObjects(ctx context.Context, b *budget, r *gitrun.Runner) (map[string]string, error) {
	reachable, e := b.git(ctx, r, nil, "rev-list", "--objects", "--all", "--no-object-names")
	if e != nil {
		return nil, e
	}
	reach := map[string]bool{}
	for _, id := range strings.Fields(string(reachable)) {
		if !gitID(id) {
			return nil, ErrUnsafe
		}
		reach[id] = true
	}
	raw, e := b.git(ctx, r, nil, "cat-file", "--batch-all-objects", "--batch-check=%(objectname) %(objecttype) %(objectsize)")
	if e != nil {
		return nil, e
	}
	out := map[string]string{}
	var total int64
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || !reach[f[0]] {
			return nil, ErrUnsafe
		}
		n, e := strconv.ParseInt(f[2], 10, 64)
		if e != nil || n < 0 || n > maxBytes {
			return nil, ErrLimit
		}
		total += n
		if total > maxTotal {
			return nil, ErrLimit
		}
		if f[1] != "blob" && f[1] != "tree" && f[1] != "commit" {
			return nil, ErrUnsafe
		}
		out[f[0]] = f[1]
	}
	if len(out) != len(reach) {
		return nil, ErrUnsafe
	}
	// Every reflog target must already belong to the retained graph.
	logs, e := b.git(ctx, r, nil, "rev-list", "--reflog", "--all")
	if e != nil {
		return nil, e
	}
	for _, id := range strings.Fields(string(logs)) {
		if !reach[id] {
			return nil, ErrUnsafe
		}
	}
	return out, nil
}

func validPath(path string) bool {
	parts := strings.Split(path, "/")
	return len(parts) == 5 && parts[0] == "brain" && parts[1] == "sources" && store.ValidSourceSHA(parts[3]) && parts[2] == parts[3][:2] && (parts[4] == "bytes" || parts[4] == "meta.yaml")
}
func snapshot(ctx context.Context, b *budget, r *gitrun.Runner, commit string) (map[string][]byte, error) {
	tree, e := b.git(ctx, r, nil, "ls-tree", "-rz", "--full-tree", commit)
	if e != nil {
		return nil, e
	}
	entries := bytes.Split(tree, []byte{0})
	if len(entries) > 4097 {
		return nil, ErrLimit
	}
	files := map[string][]byte{}
	for _, entry := range entries {
		if len(entry) == 0 {
			continue
		}
		header, path, ok := strings.Cut(string(entry), "\t")
		f := strings.Fields(header)
		if !ok || len(f) != 3 || f[0] != "100644" || f[1] != "blob" || !gitID(f[2]) || !validPath(path) {
			return nil, ErrUnsafe
		}
		// A malformed historical tree may list the same full path twice.
		// Reject even identical entries before the map can hide either one.
		if _, exists := files[path]; exists {
			return nil, ErrUnsafe
		}
		data, e := b.git(ctx, r, nil, "cat-file", "blob", f[2])
		if e != nil {
			return nil, e
		}
		if len(data) > store.EpisodeMaxRecordBytes {
			return nil, ErrLimit
		}
		files[path] = data
	}
	return files, nil
}
func materialize(root string, files map[string][]byte) error {
	for path, data := range files {
		if !validPath(path) {
			return ErrUnsafe
		}
		full := filepath.Join(root, filepath.FromSlash(path))
		if e := os.MkdirAll(filepath.Dir(full), 0700); e != nil {
			return e
		}
		if e := os.WriteFile(full, data, 0600); e != nil {
			return e
		}
	}
	return nil
}
func readFiles(ctx context.Context, root string) (map[string][]byte, error) {
	out := map[string][]byte{}
	e := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		if !validPath(rel) {
			return ErrUnsafe
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		out[rel] = data
		return nil
	})
	return out, e
}

type treeNode struct {
	files map[string]string
	dirs  map[string]*treeNode
}

func newNode() *treeNode { return &treeNode{files: map[string]string{}, dirs: map[string]*treeNode{}} }
func writeTree(ctx context.Context, b *budget, r *gitrun.Runner, files map[string][]byte, kept map[string]bool) (string, error) {
	root := newNode()
	for path, data := range files {
		id, e := b.git(ctx, r, data, "hash-object", "-w", "--stdin")
		if e != nil {
			return "", e
		}
		sha := strings.TrimSpace(string(id))
		if !gitID(sha) {
			return "", ErrUnsafe
		}
		kept[sha] = true
		parts := strings.Split(path, "/")
		n := root
		for _, part := range parts[:len(parts)-1] {
			if n.dirs[part] == nil {
				n.dirs[part] = newNode()
			}
			n = n.dirs[part]
		}
		n.files[parts[len(parts)-1]] = sha
	}
	var write func(*treeNode) (string, error)
	write = func(n *treeNode) (string, error) {
		var entries []string
		for name, id := range n.files {
			entries = append(entries, "100644 blob "+id+"\t"+name+"\x00")
		}
		for name, child := range n.dirs {
			id, e := write(child)
			if e != nil {
				return "", e
			}
			entries = append(entries, "040000 tree "+id+"\t"+name+"\x00")
		}
		sort.Strings(entries)
		id, e := b.git(ctx, r, []byte(strings.Join(entries, "")), "mktree", "-z")
		return strings.TrimSpace(string(id)), e
	}
	return write(root)
}
func verifyCandidate(ctx context.Context, b *budget, repo, head string, commits int, old, kept map[string]bool) error {
	r := gitrun.CanonicalReadOnly(repo)
	refs, e := b.git(ctx, r, nil, "for-each-ref", "--format=%(refname) %(objectname)")
	if e != nil {
		return e
	}
	if strings.TrimSpace(string(refs)) != "refs/heads/main "+head {
		return ErrUnsafe
	}
	objects, e := allObjects(ctx, b, r)
	if e != nil {
		return e
	}
	for id := range old {
		if _, ok := objects[id]; ok && !kept[id] {
			return ErrUnsafe
		}
	}
	history, e := b.git(ctx, r, nil, "rev-list", "--all")
	if e != nil {
		return e
	}
	if len(strings.Fields(string(history))) != commits {
		return ErrUnsafe
	}
	return nil
}
func syncDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	return errors.Join(f.Sync(), f.Close())
}
func syncTree(ctx context.Context, root string) error {
	var dirs []string
	e := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		if d.IsDir() {
			dirs = append(dirs, path)
			return nil
		}
		return syncDir(path)
	})
	if e != nil {
		return e
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		if e = syncDir(dirs[i]); e != nil {
			return e
		}
	}
	return nil
}

func blobID(raw []byte) string {
	h := sha1.New() // Git SHA1 object identity, not an authorization digest.
	_, _ = fmt.Fprintf(h, "blob %d\x00", len(raw))
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}
