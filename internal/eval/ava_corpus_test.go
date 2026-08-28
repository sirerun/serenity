package eval

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/sirerun/serenity/internal/config"
)

// avaCorpusDir locates evals/corpora/ava relative to this test file's own
// path (internal/eval/ -- two levels below the repo root), so it resolves
// regardless of the working directory a test runner uses -- the same
// technique internal/eval/direction/direction_test.go uses for its own
// corpus.
func avaCorpusDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "evals", "corpora", "ava")
}

func loadAvaLabels(t *testing.T) []Label {
	t.Helper()
	labels, err := LoadLabels(filepath.Join(avaCorpusDir(t), "labels"))
	if err != nil {
		t.Fatalf("LoadLabels: %v", err)
	}
	if len(labels) == 0 {
		t.Fatal("LoadLabels returned zero labels -- is evals/corpora/ava/labels/ present?")
	}
	return labels
}

// TestAvaCorpusManifestVerifies is T1.14's checksum-pinning gate, reusing
// T1.13's exact tooling (VerifyManifest) unmodified per ADR-005 and the
// team lead's instruction to not invent a parallel scheme -- a label file
// edited without regenerating evals/corpora/ava/gen_corpus.go fails here.
// The manifest lives one directory above labels/ (see gen_corpus.go's
// manifestPath comment for why), which VerifyManifest supports unmodified.
func TestAvaCorpusManifestVerifies(t *testing.T) {
	dir := filepath.Join(avaCorpusDir(t), "labels")
	manifest := filepath.Join(avaCorpusDir(t), "checksums.yaml")
	if err := VerifyManifest(dir, manifest); err != nil {
		t.Fatalf("VerifyManifest: %v\n(if you edited the corpus deliberately, regenerate with: go run evals/corpora/ava/gen_corpus.go)", err)
	}
}

// TestAvaCorpusCoversSeededVocabularyWithFloor is the plan T1.14 acc line
// directly: every predicate in internal/config's seeded 13-family
// vocabulary (T0.8) has >= 20 labeled spans, and no span carries a
// predicate outside that vocabulary (a typo'd family name would otherwise
// silently form its own single-member group instead of failing loudly).
func TestAvaCorpusCoversSeededVocabularyWithFloor(t *testing.T) {
	labels := loadAvaLabels(t)

	wantFamilies := config.Default().FamilyNames()
	if len(wantFamilies) != 13 {
		t.Fatalf("internal/config's seeded vocabulary has %d families, want 13 -- RFC 0001 predicate count changed; T1.14's floor assumes 13", len(wantFamilies))
	}

	byFamily := map[string]int{}
	for _, l := range labels {
		byFamily[l.Expected.Predicate]++
	}

	known := map[string]bool{}
	for _, f := range wantFamilies {
		known[f] = true
		if byFamily[f] < 20 {
			t.Errorf("family %q has %d labeled spans, want >= 20", f, byFamily[f])
		}
	}
	for f, n := range byFamily {
		if !known[f] {
			t.Errorf("family %q (%d spans) is not in internal/config's seeded vocabulary -- typo, or the vocabulary changed", f, n)
		}
	}
}

// TestAvaCorpusNoDuplicateSpans guards the split file's addressability:
// Split.Filter (internal/eval/split.go) keys held-out membership by exact
// span text, so two labels sharing identical span text would make a
// held-out entry ambiguous.
func TestAvaCorpusNoDuplicateSpans(t *testing.T) {
	seen := make(map[string]bool)
	for _, l := range loadAvaLabels(t) {
		if seen[l.Span] {
			t.Errorf("duplicate span text: %q", l.Span)
		}
		seen[l.Span] = true
	}
}

// TestAvaCorpusContentPopulated is a light content-quality floor: every
// span carries a real predicate, object, and labeler.
func TestAvaCorpusContentPopulated(t *testing.T) {
	for _, l := range loadAvaLabels(t) {
		if l.Span == "" {
			t.Error("label with empty span")
		}
		if l.Expected.Predicate == "" {
			t.Errorf("span %q: empty predicate", l.Span)
		}
		if l.Expected.Object == "" {
			t.Errorf("span %q: empty object", l.Span)
		}
		if l.Labeler == "" {
			t.Errorf("span %q: empty labeler", l.Span)
		}
	}
}

// TestAvaCorpusSplitFileValid is the "held-out split" half of the acc
// line, checked against the exact mechanism internal/eval/split.go wires:
// Split.HeldOut names Label.Span values. A held-out entry naming a span
// that doesn't exist would be silently ignored by Split.Filter rather than
// erroring, so this test catches that failure mode explicitly (the same
// property TestAvaCorpusManifestVerifies checks for checksum drift).
func TestAvaCorpusSplitFileValid(t *testing.T) {
	labels := loadAvaLabels(t)
	spanFamily := make(map[string]string, len(labels))
	for _, l := range labels {
		spanFamily[l.Span] = l.Expected.Predicate
	}

	split, err := LoadSplit(filepath.Join(avaCorpusDir(t), "split.yaml"))
	if err != nil {
		t.Fatalf("LoadSplit: %v", err)
	}
	if len(split.HeldOut) == 0 {
		t.Fatal("split.yaml has zero held-out spans")
	}

	byFamily := map[string]int{}
	seen := make(map[string]bool, len(split.HeldOut))
	for _, span := range split.HeldOut {
		if seen[span] {
			t.Errorf("held_out lists span %q more than once", span)
		}
		seen[span] = true

		family, ok := spanFamily[span]
		if !ok {
			t.Errorf("held_out names a span with no matching label (Split.Filter would silently ignore it): %q", span)
			continue
		}
		byFamily[family]++
	}

	for _, f := range config.Default().FamilyNames() {
		if byFamily[f] == 0 {
			t.Errorf("family %q has zero held-out spans -- the split is degenerate for this family", f)
		}
	}

	heldOut, rest := split.Filter(labels)
	if len(heldOut) != len(split.HeldOut) {
		t.Errorf("split.Filter matched %d of the %d declared held-out spans", len(heldOut), len(split.HeldOut))
	}
	if len(rest)+len(heldOut) != len(labels) {
		t.Errorf("split.Filter partition sizes %d+%d don't add up to the corpus size %d", len(heldOut), len(rest), len(labels))
	}
}

// contradictionRecord is the raw shape gen_corpus.go writes into each
// label file's contradiction_pair_id/contradiction_role fields --
// deliberately not part of eval.Label (see label.go's doc comment on why
// family/pairing metadata does not belong on the golden-set type), so this
// test re-parses the same files eval.LoadLabels already validated to read
// them.
type contradictionRecord struct {
	Span                string `yaml:"span"`
	ContradictionPairID string `yaml:"contradiction_pair_id"`
	ContradictionRole   string `yaml:"contradiction_role"`
}

func loadContradictionTags(t *testing.T, labelsDir string) []contradictionRecord {
	t.Helper()
	entries, err := os.ReadDir(labelsDir)
	if err != nil {
		t.Fatalf("read labels dir: %v", err)
	}
	var out []contradictionRecord
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" || e.Name() == "checksums.yaml" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(labelsDir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		var r contradictionRecord
		if err := yaml.Unmarshal(b, &r); err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		if r.ContradictionPairID != "" {
			out = append(out, r)
		}
	}
	return out
}

type contradictionPair struct {
	ID     string `yaml:"id"`
	Family string `yaml:"family"`
	SpanA  string `yaml:"span_a"`
	SpanB  string `yaml:"span_b"`
	Why    string `yaml:"why"`
}

func loadContradictionPairs(t *testing.T) []contradictionPair {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(avaCorpusDir(t), "contradictions.yaml"))
	if err != nil {
		t.Fatalf("read contradictions.yaml: %v", err)
	}
	var doc struct {
		Pairs []contradictionPair `yaml:"pairs"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		t.Fatalf("parse contradictions.yaml: %v", err)
	}
	return doc.Pairs
}

// monthNum parses a "YYYY-MM" validity-window endpoint into a monotonic
// integer (year*12+month) for range-overlap comparison. An empty string is
// the open-ended case and is handled by the caller, not here.
func monthNum(t *testing.T, s string) int {
	t.Helper()
	tm, err := time.Parse("2006-01", s)
	if err != nil {
		t.Fatalf("valid_from/valid_to %q is not YYYY-MM: %v", s, err)
	}
	return tm.Year()*12 + int(tm.Month())
}

// windowsOverlap reports whether two [from, to] validity windows overlap,
// treating an empty "to" as open-ended (unbounded future) -- the
// contradiction-pair correctness test below requires this to be true for
// every declared pair, since two claims that never coexist in time are not
// actually a contradiction.
func windowsOverlap(t *testing.T, aFrom, aTo, bFrom, bTo string) bool {
	t.Helper()
	const openEnd = 1 << 30
	aStart := monthNum(t, aFrom)
	bStart := monthNum(t, bFrom)
	aEnd, bEnd := openEnd, openEnd
	if aTo != "" {
		aEnd = monthNum(t, aTo)
	}
	if bTo != "" {
		bEnd = monthNum(t, bTo)
	}
	return aStart <= bEnd && bStart <= aEnd
}

// TestAvaCorpusContradictionPairsMeetFloor is the ">= 10 embedded
// contradiction pairs" acc line, checked against evals/corpora/ava's
// human-readable contradictions.yaml index.
func TestAvaCorpusContradictionPairsMeetFloor(t *testing.T) {
	pairs := loadContradictionPairs(t)
	if len(pairs) < 10 {
		t.Fatalf("evals/corpora/ava/contradictions.yaml has %d pairs, want >= 10", len(pairs))
	}
}

// TestAvaCorpusContradictionPairsAreGenuineConflicts is the real
// correctness gate (mirroring T3.13's TestExpectedVerdictMatchesReferenceEvaluator):
// it does not trust contradictions.yaml's say-so. For every declared pair
// it independently re-derives, from the actual label files, that span_a
// and span_b (a) both exist, (b) share the same predicate family as the
// pair declares, (c) assert different objects -- otherwise the two spans
// would agree, not conflict -- and (d) have overlapping validity windows,
// since two claims about disjoint time periods are not a contradiction. A
// pair that merely SAYS it conflicts without its labels actually
// conflicting would be exactly the "mislabeled contradiction" failure mode
// this corpus exists to avoid.
func TestAvaCorpusContradictionPairsAreGenuineConflicts(t *testing.T) {
	labels := loadAvaLabels(t)
	bySpan := make(map[string]Label, len(labels))
	for _, l := range labels {
		bySpan[l.Span] = l
	}

	for _, p := range loadContradictionPairs(t) {
		la, ok := bySpan[p.SpanA]
		if !ok {
			t.Errorf("pair %s: span_a has no matching label: %q", p.ID, p.SpanA)
			continue
		}
		lb, ok := bySpan[p.SpanB]
		if !ok {
			t.Errorf("pair %s: span_b has no matching label: %q", p.ID, p.SpanB)
			continue
		}
		if p.SpanA == p.SpanB {
			t.Errorf("pair %s: span_a and span_b are identical", p.ID)
		}
		if la.Expected.Predicate != p.Family {
			t.Errorf("pair %s: span_a's predicate %q != declared family %q", p.ID, la.Expected.Predicate, p.Family)
		}
		if la.Expected.Predicate != lb.Expected.Predicate {
			t.Errorf("pair %s: span_a predicate %q != span_b predicate %q -- not the same (subject, predicate)", p.ID, la.Expected.Predicate, lb.Expected.Predicate)
			continue
		}
		if la.Expected.Object == lb.Expected.Object {
			t.Errorf("pair %s: span_a and span_b assert the identical object %q -- that's agreement, not a conflict", p.ID, la.Expected.Object)
		}
		if !windowsOverlap(t, la.Expected.ValidFrom, la.Expected.ValidTo, lb.Expected.ValidFrom, lb.Expected.ValidTo) {
			t.Errorf("pair %s: validity windows [%s,%s) and [%s,%s) do not overlap -- the two claims never coexist, so they don't actually contradict", p.ID, la.Expected.ValidFrom, la.Expected.ValidTo, lb.Expected.ValidFrom, lb.Expected.ValidTo)
		}
		if p.Why == "" {
			t.Errorf("pair %s: empty why", p.ID)
		}
	}
}

// TestAvaCorpusContradictionTagsMatchIndex cross-checks the other
// direction: every label file's inline contradiction_pair_id/
// contradiction_role tag (gen_corpus.go's embedding of "which pair, which
// side") is consistent with contradictions.yaml -- exactly two spans
// tagged role "a" and two tagged role "b" per pair id, and the pair id
// appears in the index with a matching family, and the index's span_a/
// span_b are each among that pair's two same-role tagged spans.
func TestAvaCorpusContradictionTagsMatchIndex(t *testing.T) {
	labelsDir := filepath.Join(avaCorpusDir(t), "labels")
	tags := loadContradictionTags(t, labelsDir)
	if len(tags) == 0 {
		t.Fatal("no label file carries a contradiction_pair_id -- contradiction pairs are not actually embedded in the labels")
	}

	byPair := map[string]map[string][]string{} // pair id -> role -> spans
	for _, tag := range tags {
		if byPair[tag.ContradictionPairID] == nil {
			byPair[tag.ContradictionPairID] = map[string][]string{}
		}
		byPair[tag.ContradictionPairID][tag.ContradictionRole] = append(byPair[tag.ContradictionPairID][tag.ContradictionRole], tag.Span)
	}

	pairs := loadContradictionPairs(t)
	indexed := make(map[string]contradictionPair, len(pairs))
	for _, p := range pairs {
		indexed[p.ID] = p
	}

	var ids []string
	for id := range byPair {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		roles := byPair[id]
		if len(roles["a"]) != 2 {
			t.Errorf("pair %s: %d spans tagged role a, want 2", id, len(roles["a"]))
		}
		if len(roles["b"]) != 2 {
			t.Errorf("pair %s: %d spans tagged role b, want 2", id, len(roles["b"]))
		}
		p, ok := indexed[id]
		if !ok {
			t.Errorf("pair %s: tagged in label files but missing from contradictions.yaml", id)
			continue
		}
		if !contains(roles["a"], p.SpanA) {
			t.Errorf("pair %s: contradictions.yaml span_a %q is not among the role-a tagged spans %v", id, p.SpanA, roles["a"])
		}
		if !contains(roles["b"], p.SpanB) {
			t.Errorf("pair %s: contradictions.yaml span_b %q is not among the role-b tagged spans %v", id, p.SpanB, roles["b"])
		}
	}

	if len(byPair) < 10 {
		t.Errorf("only %d distinct contradiction_pair_id values tagged across the labels, want >= 10", len(byPair))
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
