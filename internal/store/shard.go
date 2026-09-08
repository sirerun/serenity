package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

// DefaultRolloverBytes is the shard segment size at which Append opens a
// new numbered segment file for a family rather than growing the current
// one without bound (RFC §7.7/§16, T2.9: "Shard files roll over at a
// configured size"). 8 MiB comfortably holds tens of thousands of claim
// lines (TestShard10KProperty's 10,000-claim fixture stays under ~1 MiB)
// while still being small enough that a real long-lived family will
// exercise real rollover well before it becomes an operator-visible
// problem.
const DefaultRolloverBytes int64 = 8 * 1024 * 1024

// segmentPattern matches a numbered shard segment file's name --
// "<family>.<N>.jsonl", e.g. "has_balance.3.jsonl" is family
// "has_balance" segment 3. The base file "<family>.jsonl" (segment 0) is
// deliberately unnumbered and never matches this pattern -- it is the
// only shard path any other package (internal/writer, internal/supersede,
// internal/entities, every existing test) has ever referenced via
// PathFor, and it stays that way: rollover is purely an Append-time
// storage-layout detail, invisible to every caller that only ever reads
// (Lines/ResolveHeads span every segment transparently) or writes through
// Append.
var segmentPattern = regexp.MustCompile(`^(.+)\.(\d+)\.jsonl$`)

// ShardStore holds high-volume claim families as append-only JSONL under
// brain/claims/<entity-slug>/<family>.jsonl — one claim per line, same
// fields as a fence row, human-readable and hand-repairable (RFC §7.2a).
// For shard-tier families the shard is canonical; fence head rows are
// derived. Supersession appends a superseding line (Claim.Supersedes
// points at the replaced id); nothing rewrites history in place except
// explicit, disposition-approved compaction (§7.7).
//
// A family's lines may span more than one on-disk file once rollover
// (T2.9) has opened numbered segments alongside the base file -- Lines,
// ResolveHeads, and Append's id registry all transparently span every
// segment; PathFor keeps returning only the base file's path, since that
// stays the one every other package and existing test already treats as
// "the" shard path for a (slug, family) pair.
type ShardStore struct {
	Root string
	// Vocabulary restricts which predicate families Append accepts. Nil
	// uses the controlled vocabulary seeded at install (defaultVocabulary);
	// set it from a loaded config.Config.Families to enforce a project's
	// actual, migrated vocabulary instead of the seed (§7.2).
	Vocabulary map[string]bool
	// RolloverBytes is the configured size (RFC §7.7) at which Append
	// opens a new numbered segment instead of continuing to grow a
	// family's current segment. Zero (the field's default) uses
	// DefaultRolloverBytes -- the same "zero/nil means use the seeded
	// default" convention Vocabulary already establishes on this struct.
	RolloverBytes int64

	// mu guards registry: concurrent Append calls across different shard
	// files (once the writer queue lands, §7.7) share one ShardStore's
	// registry map.
	mu sync.Mutex
	// registry is the per-family claim-id registry (ADR 004 D2): a stable
	// per-(slug,family) key (PathFor's base path, used only as a map key
	// here -- not necessarily where a given claim's line physically
	// lives once rollover is in play) -> id -> the claim already on disk
	// under that id, aggregated across every segment. It is lazily built
	// from disk on first use per key and kept current in memory after
	// each successful Append, so a long-lived ShardStore does one family
	// parse rather than one per Append (TestShard10KProperty depends on
	// that: 10,000 appends into one family must stay O(n), not O(n^2)). A
	// fresh ShardStore instance always rebuilds from the files' actual
	// bytes on first use — the cache is a performance layer over that
	// on-disk truth, never a second source of it. Compact does not
	// invalidate a family's cached entry (see Compact's own doc comment):
	// compaction only relocates claim lines across files, never mutates
	// their content, so an already-cached id->claim mapping stays
	// accurate afterward.
	registry map[string]map[string]domain.Claim
}

func NewShardStore(root string) *ShardStore { return &ShardStore{Root: root} }

func (s *ShardStore) rolloverBytes() int64 {
	if s.RolloverBytes > 0 {
		return s.RolloverBytes
	}
	return DefaultRolloverBytes
}

func (s *ShardStore) vocabulary() map[string]bool {
	if s.Vocabulary != nil {
		return s.Vocabulary
	}
	return defaultVocabulary
}

func (s *ShardStore) PathFor(slug, family string) string {
	return filepath.Join(s.Root, "brain", "claims", slug, family+".jsonl")
}

// segments lists every on-disk segment file for (slug, family): the base
// file (segment 0) first if present, then numbered segments in ascending
// order. A family with nothing on disk yet returns nil, nil -- the same
// "not an error" shape readShardFile already uses for a missing file.
func (s *ShardStore) segments(slug, family string) ([]string, error) {
	dir := filepath.Join(s.Root, "brain", "claims", slug)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	base := family + ".jsonl"
	type numbered struct {
		n    int
		path string
	}
	var haveBase bool
	var nums []numbered
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		switch {
		case name == base:
			haveBase = true
		case strings.HasSuffix(name, ".archive.jsonl"):
			// not a live segment
		default:
			m := segmentPattern.FindStringSubmatch(name)
			if m == nil || m[1] != family {
				continue
			}
			n, err := strconv.Atoi(m[2])
			if err != nil {
				continue
			}
			nums = append(nums, numbered{n, filepath.Join(dir, name)})
		}
	}
	sort.Slice(nums, func(i, j int) bool { return nums[i].n < nums[j].n })
	var out []string
	if haveBase {
		out = append(out, filepath.Join(dir, base))
	}
	for _, nm := range nums {
		out = append(out, nm.path)
	}
	return out, nil
}

// writeTargetLocked returns the path Append should write c's line to: the
// family's current (highest-numbered, or base if none numbered yet)
// segment, unless that segment has already reached the configured
// rollover size, in which case a new segment numbered one higher than the
// highest existing one is opened (RFC §7.7, T2.9). s.mu must be held by
// the caller.
func (s *ShardStore) writeTargetLocked(slug, family string) (string, error) {
	paths, err := s.segments(slug, family)
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return s.PathFor(slug, family), nil // first-ever write for this family
	}
	current := paths[len(paths)-1]
	info, err := os.Stat(current)
	if err != nil {
		return "", err
	}
	if info.Size() < s.rolloverBytes() {
		return current, nil
	}
	dir := filepath.Join(s.Root, "brain", "claims", slug)
	return filepath.Join(dir, fmt.Sprintf("%s.%d.jsonl", family, nextSegmentN(family, paths))), nil
}

// nextSegmentN returns the next segment number to open for family, one
// higher than the highest numbered segment already present in paths (the
// base file, segment 0, does not match segmentPattern and so never raises
// this above 1 on its own).
func nextSegmentN(family string, paths []string) int {
	max := 0
	for _, p := range paths {
		m := segmentPattern.FindStringSubmatch(filepath.Base(p))
		if m == nil || m[1] != family {
			continue
		}
		if n, err := strconv.Atoi(m[2]); err == nil && n > max {
			max = n
		}
	}
	return max + 1
}

// Append adds one claim line. IDs are derived when absent (§7.2). Before
// writing, an ACTIVE claim's id is checked against the family's id
// registry (ADR 004 D2, spanning every segment -- T2.9): an id that
// already names a different (subject, predicate, object key, valid_from,
// source ref) tuple anywhere in the family is ErrIDCollision, and Append
// writes nothing — never a silent overwrite. A retracted row is exempt: a
// retraction is a lifecycle tombstone that deliberately reuses its target
// claim's id (ResolveHeadLines keys retraction off exactly that id match)
// rather than asserting a new, independent claim identity — it is never a
// collision candidate.
func (s *ShardStore) Append(c domain.Claim) error {
	if strings.ContainsRune(c.Object, '\n') {
		return fmt.Errorf("shard append: objects must be single-line")
	}
	if vocab := s.vocabulary(); !vocab[c.Family] {
		return fmt.Errorf("shard append: family %q: %w", c.Family, ErrUnknownPredicate)
	}
	if c.ObjectKey == "" {
		c.ObjectKey = NormalizeKey(c.Object)
	}
	if c.ID == "" {
		c.ID = DerivedID(c.SubjectSlug, c.Predicate, c.ObjectKey, c.ValidFrom, c.Provenance.SourceSHA256, DefaultIDWidth)
	}
	// The registry cache key stays the family's base path regardless of
	// which segment a line physically lands in -- it identifies "this
	// family's id registry," not any one file (see the registry field's
	// own doc comment).
	regKey := s.PathFor(c.SubjectSlug, c.Family)

	s.mu.Lock()
	defer s.mu.Unlock()
	reg, err := s.loadRegistryLocked(regKey, c.SubjectSlug, c.Family)
	if err != nil {
		return err
	}
	if c.State != domain.StateRetracted {
		if prior, ok := reg[c.ID]; ok && !sameClaimTuple(prior, c) {
			return fmt.Errorf("shard %s: %w: id %s already identifies subject=%s predicate=%s object_key=%s valid_from=%s source=%s",
				regKey, ErrIDCollision, c.ID, prior.SubjectSlug, prior.Predicate, prior.ObjectKey, prior.ValidFrom, prior.Provenance.SourceSHA256)
		}
	}

	p, err := s.writeTargetLocked(c.SubjectSlug, c.Family)
	if err != nil {
		return err
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

// loadRegistryLocked returns the id registry for (slug, family), keyed by
// key (its base path, used only as a stable cache key), building it from
// every one of the family's on-disk segments on first use and caching it
// on s.registry afterward (s.mu must be held). Reading is O(family) once
// per key, not once per Append call.
func (s *ShardStore) loadRegistryLocked(key, slug, family string) (map[string]domain.Claim, error) {
	if s.registry == nil {
		s.registry = map[string]map[string]domain.Claim{}
	}
	if reg, ok := s.registry[key]; ok {
		return reg, nil
	}
	lines, err := s.Lines(slug, family)
	if err != nil {
		return nil, err
	}
	reg := make(map[string]domain.Claim, len(lines))
	for _, c := range lines {
		reg[c.ID] = c // last line for a given id wins, matching cross-segment file order
	}
	s.registry[key] = reg
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

// Lines reads every claim line across the family's segment(s) -- the base
// file, then any numbered rollover segments (T2.9), in that order. A
// corrupt line is a hard error (the file is canonical; silently skipping
// would hide data loss).
func (s *ShardStore) Lines(slug, family string) ([]domain.Claim, error) {
	paths, err := s.segments(slug, family)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}
	var out []domain.Claim
	for _, p := range paths {
		lines, err := readShardFile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, lines...)
	}
	return out, nil
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
	return scanShard(f, path)
}

// ParseShardBytes reads canonical JSONL using the same parser as disk-backed
// shard reads. It supports callers working from a validated byte snapshot.
func ParseShardBytes(raw []byte) ([]domain.Claim, error) {
	return scanShard(bytes.NewReader(raw), "snapshot")
}

func scanShard(reader io.Reader, path string) ([]domain.Claim, error) {
	var out []domain.Claim
	sc := bufio.NewScanner(reader)
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
//
// If rollover (T2.9) has opened numbered segments alongside the base
// file, Compact reads across all of them (Lines already does this
// transparently), writes every kept (live-head) line back to the single
// base file, and removes the now-fully-redistributed numbered segments --
// compaction always leaves a family with at most one live segment, so the
// next Append after a compact starts a fresh rollover count rather than
// resuming a partially-full numbered segment.
//
// Compact does not touch Append's in-memory id registry cache
// (loadRegistryLocked), and deliberately need not: Compact never mutates
// a claim's content, only which file it lives in, so every cache entry a
// prior Append call already populated -- including one for a claim this
// call is about to archive -- stays a byte-accurate record of an id
// already spent in this family. Invalidating it would only make a
// long-lived *ShardStore instance's collision detection *weaker* after a
// compact (matching a fresh instance's own gap: readShardFile/segments
// never scan <family>.archive.jsonl, so a fresh instance's registry
// already cannot see ids that were archived before it was constructed --
// a pre-existing, disclosed limitation this task's acc line does not ask
// it to close), not stronger.
func (s *ShardStore) Compact(slug, family string) (moved int, err error) {
	segs, err := s.segments(slug, family)
	if err != nil {
		return 0, err
	}
	if len(segs) == 0 {
		return 0, nil
	}
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
	if err := os.Rename(tmp, p); err != nil {
		return 0, err
	}

	// Every other segment's lines have now been fully redistributed into
	// either the base file (keep) or the archive (archive) -- remove them
	// so the family holds at most one live segment again.
	for _, seg := range segs {
		if seg == p {
			continue
		}
		if err := os.Remove(seg); err != nil {
			return 0, fmt.Errorf("shard compact: remove old segment %s: %w", seg, err)
		}
	}

	return len(archive), nil
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
