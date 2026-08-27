package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sirerun/serenity/internal/domain"
)

// ShardStore holds high-volume claim families as append-only JSONL under
// brain/claims/<entity-slug>/<family>.jsonl — one claim per line, same
// fields as a fence row, human-readable and hand-repairable (RFC §7.2a).
// For shard-tier families the shard is canonical; fence head rows are
// derived. Supersession appends a superseding line (Claim.Supersedes
// points at the replaced id); nothing rewrites history in place except
// explicit, disposition-approved compaction (§7.7).
type ShardStore struct {
	Root string
}

func NewShardStore(root string) *ShardStore { return &ShardStore{Root: root} }

func (s *ShardStore) PathFor(slug, family string) string {
	return filepath.Join(s.Root, "brain", "claims", slug, family+".jsonl")
}

// Append adds one claim line. IDs are derived when absent (§7.2).
func (s *ShardStore) Append(c domain.Claim) error {
	if strings.ContainsRune(c.Object, '\n') {
		return fmt.Errorf("shard append: objects must be single-line")
	}
	if c.ObjectKey == "" {
		c.ObjectKey = NormalizeKey(c.Object)
	}
	if c.ID == "" {
		c.ID = DerivedID(c.SubjectSlug, c.Predicate, c.ObjectKey, c.ValidFrom, c.Provenance.SourceSHA256)
	}
	p := s.PathFor(c.SubjectSlug, c.Family)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	line, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return f.Close()
}

// Lines reads every claim line in file order. A corrupt line is a hard
// error (the file is canonical; silently skipping would hide data loss).
func (s *ShardStore) Lines(slug, family string) ([]domain.Claim, error) {
	return readShardFile(s.PathFor(slug, family))
}

func readShardFile(path string) ([]domain.Claim, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []domain.Claim
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for sc.Scan() {
		ln := strings.TrimSpace(sc.Text())
		if ln == "" {
			continue
		}
		var c domain.Claim
		if err := json.Unmarshal([]byte(ln), &c); err != nil {
			return nil, fmt.Errorf("shard %s: corrupt line: %w", path, err)
		}
		out = append(out, c)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("shard %s: %w", path, err)
	}
	return out, nil
}

// ResolveHeads computes the current resolved head per object key. The
// resolution is order-independent — a claim is live unless a line
// supersedes or retracts it, and ties break on (ObservedAt, ID) — so two
// divergent copies of a shard merge (line union) to identical heads,
// which is what makes git merges of shards safe (§7.7).
func (s *ShardStore) ResolveHeads(slug, family string) (map[string]domain.Claim, error) {
	lines, err := s.Lines(slug, family)
	if err != nil {
		return nil, err
	}
	return ResolveHeadLines(lines), nil
}

// ResolveHeadLines is the pure resolution over a set of claim lines.
func ResolveHeadLines(lines []domain.Claim) map[string]domain.Claim {
	dead := map[string]bool{}
	for _, c := range lines {
		if c.Supersedes != "" {
			dead[c.Supersedes] = true
		}
		if c.State == domain.StateRetracted {
			dead[c.ID] = true
		}
	}
	heads := map[string]domain.Claim{}
	for _, c := range lines {
		if c.State != domain.StateActive || dead[c.ID] {
			continue
		}
		key := c.ObjectKey
		if key == "" {
			key = NormalizeKey(c.Object)
		}
		cur, ok := heads[key]
		if !ok || laterClaim(c, cur) {
			heads[key] = c
		}
	}
	return heads
}

func laterClaim(a, b domain.Claim) bool {
	at, bt := a.Provenance.ObservedAt, b.Provenance.ObservedAt
	if !at.Equal(bt) {
		return at.After(bt)
	}
	return a.ID > b.ID
}

// HeadKeys returns the resolved heads in deterministic key order.
func HeadKeys(heads map[string]domain.Claim) []string {
	keys := make([]string, 0, len(heads))
	for k := range heads {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Families lists shard families present for an entity, sorted.
func (s *ShardStore) Families(slug string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(s.Root, "brain", "claims", slug))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".archive.jsonl") {
			continue
		}
		out = append(out, strings.TrimSuffix(name, ".jsonl"))
	}
	sort.Strings(out)
	return out, nil
}

// Slugs lists entities that have shard families, sorted.
func (s *ShardStore) Slugs() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(s.Root, "brain", "claims"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// Compact moves superseded and retracted lines into the family's archive
// shard (<family>.archive.jsonl) and rewrites the live shard atomically.
// It must only run inside an explicit, disposition-approved `serenity
// compact` — never silently (§7.7).
func (s *ShardStore) Compact(slug, family string) (moved int, err error) {
	lines, err := s.Lines(slug, family)
	if err != nil {
		return 0, err
	}
	heads := ResolveHeadLines(lines)
	live := map[string]bool{}
	for _, c := range heads {
		live[c.ID] = true
	}

	var keep, archive []domain.Claim
	for _, c := range lines {
		if live[c.ID] {
			keep = append(keep, c)
		} else {
			archive = append(archive, c)
		}
	}
	if len(archive) == 0 {
		return 0, nil
	}

	p := s.PathFor(slug, family)
	archPath := strings.TrimSuffix(p, ".jsonl") + ".archive.jsonl"
	af, err := os.OpenFile(archPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, err
	}
	defer func() { _ = af.Close() }()
	for _, c := range archive {
		line, err := json.Marshal(c)
		if err != nil {
			return 0, err
		}
		if _, err := af.Write(append(line, '\n')); err != nil {
			return 0, err
		}
	}
	if err := af.Close(); err != nil {
		return 0, err
	}

	tmp := p + ".compact"
	tf, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	abort := func(err error) (int, error) {
		_ = tf.Close()
		_ = os.Remove(tmp)
		return 0, err
	}
	bw := bufio.NewWriter(tf)
	for _, c := range keep {
		line, err := json.Marshal(c)
		if err != nil {
			return abort(err)
		}
		if _, err := bw.Write(line); err != nil {
			return abort(err)
		}
		if err := bw.WriteByte('\n'); err != nil {
			return abort(err)
		}
	}
	if err := bw.Flush(); err != nil {
		return abort(err)
	}
	if err := tf.Close(); err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}
	return len(archive), os.Rename(tmp, p)
}

// MergeLines unions two divergent copies of a shard (exact-line dedup) —
// the deterministic merge used when git leaves both sides of a conflict.
func MergeLines(a, b []domain.Claim) []domain.Claim {
	seen := map[string]bool{}
	var out []domain.Claim
	for _, c := range append(append([]domain.Claim(nil), a...), b...) {
		line, _ := json.Marshal(c)
		k := string(line)
		if !seen[k] {
			seen[k] = true
			out = append(out, c)
		}
	}
	return out
}
