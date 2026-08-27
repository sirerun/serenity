package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/sirerun/serenity/internal/domain"
)

// ErrIDCollision is returned when a claim id (derived or supplied) matches
// an existing row in the same shard file whose (subject, predicate, object
// key, valid_from, source ref) tuple differs — a hash collision, or two
// distinct claims fighting over one id. The writer never resolves this by
// overwriting: §7.2 and ADR 004 D2
// (docs/adr/004-writer-queue-pending-records-and-hash-width.md) make it a
// hard error, and the fix is to widen DerivedID's width for the family and
// re-render, never to retry the write.
var ErrIDCollision = errors.New("store: claim id collision")

// ShardStore holds high-volume claim families as append-only JSONL under
// brain/claims/<entity-slug>/<family>.jsonl — one claim per line, same
// fields as a fence row, human-readable and hand-repairable (RFC §7.2a).
// For shard-tier families the shard is canonical; fence head rows are
// derived. Supersession appends a superseding line (Claim.Supersedes
// points at the replaced id); nothing rewrites history in place except
// explicit, disposition-approved compaction (§7.7).
type ShardStore struct {
	Root string
	// Vocabulary restricts which predicate families Append accepts. Nil
	// uses the controlled vocabulary seeded at install (defaultVocabulary);
	// set it from a loaded config.Config.Families to enforce a project's
	// actual, migrated vocabulary instead of the seed (§7.2).
	Vocabulary map[string]bool

	// mu guards registry: concurrent Append calls across different shard
	// files (once the writer queue lands, §7.7) share one ShardStore's
	// registry map.
	mu sync.Mutex
	// registry is the per-file claim-id registry (ADR 004 D2): path -> id
	// -> the claim already on disk under that id. It is lazily built from
	// disk on first use per path and kept current in memory after each
	// successful Append, so a long-lived ShardStore does one file parse per
	// path rather than one per Append (TestShard10KProperty depends on
	// that: 10,000 appends into one file must stay O(n), not O(n^2)). A
	// fresh ShardStore instance always rebuilds from the file's actual
	// bytes on first use — the cache is a performance layer over that
	// on-disk truth, never a second source of it.
	registry map[string]map[string]domain.Claim
}

func NewShardStore(root string) *ShardStore { return &ShardStore{Root: root} }

func (s *ShardStore) vocabulary() map[string]bool {
	if s.Vocabulary != nil {
		return s.Vocabulary
	}
	return defaultVocabulary
}

func (s *ShardStore) PathFor(slug, family string) string {
	return filepath.Join(s.Root, "brain", "claims", slug, family+".jsonl")
}

// Append adds one claim line. IDs are derived when absent (§7.2). Before
// writing, an ACTIVE claim's id is checked against the per-file id
// registry (ADR 004 D2): an id that already names a different (subject,
// predicate, object key, valid_from, source ref) tuple in this file is
// ErrIDCollision, and Append writes nothing — never a silent overwrite. A
// retracted row is exempt: a retraction is a lifecycle tombstone that
// deliberately reuses its target claim's id (ResolveHeadLines keys
// retraction off exactly that id match) rather than asserting a new,
// independent claim identity — it is never a collision candidate.
func (s *ShardStore) Append(c domain.Claim) error {
	if strings.ContainsRune(c.Object, '\n') {
		return fmt.Errorf("shard append: objects must be single-line")
	}
	if vocab := s.vocabulary(); !vocab[c.Family] {
		return fmt.Errorf("shard append: family %q is not in the controlled vocabulary (extend via serenity.yml + migration)", c.Family)
	}
	if c.ObjectKey == "" {
		c.ObjectKey = NormalizeKey(c.Object)
	}
	if c.ID == "" {
		c.ID = DerivedID(c.SubjectSlug, c.Predicate, c.ObjectKey, c.ValidFrom, c.Provenance.SourceSHA256, DefaultIDWidth)
	}
	p := s.PathFor(c.SubjectSlug, c.Family)

	s.mu.Lock()
	defer s.mu.Unlock()
	reg, err := s.loadRegistryLocked(p)
	if err != nil {
		return err
	}
	if c.State != domain.StateRetracted {
		if prior, ok := reg[c.ID]; ok && !sameClaimTuple(prior, c) {
			return fmt.Errorf("shard %s: %w: id %s already identifies subject=%s predicate=%s object_key=%s valid_from=%s source=%s",
				p, ErrIDCollision, c.ID, prior.SubjectSlug, prior.Predicate, prior.ObjectKey, prior.ValidFrom, prior.Provenance.SourceSHA256)
		}
	}

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
	if err := f.Close(); err != nil {
		return err
	}
	if c.State != domain.StateRetracted {
		reg[c.ID] = c // a retraction's thin tombstone tuple must never shadow the identity it retracts
	}
	return nil
}

// loadRegistryLocked returns the id registry for path, building it from
// disk on first use and caching it on s.registry afterward (s.mu must be
// held). Reading is O(file) once per path, not once per Append call.
func (s *ShardStore) loadRegistryLocked(path string) (map[string]domain.Claim, error) {
	if s.registry == nil {
		s.registry = map[string]map[string]domain.Claim{}
	}
	if reg, ok := s.registry[path]; ok {
		return reg, nil
	}
	lines, err := readShardFile(path)
	if err != nil {
		return nil, err
	}
	reg := make(map[string]domain.Claim, len(lines))
	for _, c := range lines {
		reg[c.ID] = c // last line for a given id wins, matching file order
	}
	s.registry[path] = reg
	return reg, nil
}

// sameClaimTuple reports whether a and b are the same logical claim
// identity (§7.2's id-derivation tuple) — true here means a repeated
// observation, not a collision.
func sameClaimTuple(a, b domain.Claim) bool {
	return a.SubjectSlug == b.SubjectSlug &&
		a.Predicate == b.Predicate &&
		a.ObjectKey == b.ObjectKey &&
		a.ValidFrom == b.ValidFrom &&
		a.Provenance.SourceSHA256 == b.Provenance.SourceSHA256
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
