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

// TestAvaCorpusNoDuplicateSpans guards the real invariant Split.Filter
// (internal/eval/split.go) and the scoring pipeline depend on: no two
// labels assert the SAME (span, predicate) pair, which would make it
// ambiguous which one a matching prediction should score against. It
// deliberately does NOT require span text to be globally unique across
// every family (T1.33): one real span of text can genuinely assert
// several different predicate facts at once -- exactly what
// owns_account's bonus facts encode, reusing has_balance's own span text
// verbatim because that span's possessive phrasing ("Ava's Chase checking
// balance is $4,230.18.") really does assert both facts, and Split.Filter
// (keyed on span text alone) correctly holds out every label sharing that
// text regardless of family, not just one.
func TestAvaCorpusNoDuplicateSpans(t *testing.T) {
	type spanPredicate struct{ span, predicate string }
	seen := make(map[spanPredicate]bool)
	for _, l := range loadAvaLabels(t) {
		key := spanPredicate{l.Span, l.Expected.Predicate}
		if seen[key] {
			t.Errorf("duplicate (span, predicate): span %q predicate %q", l.Span, l.Expected.Predicate)
		}
		seen[key] = true
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
	// spanHasLabel confirms a span text names at least one real label
	// (Split.Filter would otherwise silently ignore a stale entry).
	// Deliberately NOT span->single-family (T1.33): a span can carry more
	// than one label under different predicates (owns_account's bonus
	// facts reuse has_balance's own span text verbatim, by design -- see
	// familySpec's bonusRegular/bonusExtra doc comment), so per-family
	// held-out counts below are computed straight from labels, not from
	// a lossy span->family map that would silently drop one family every
	// time two labels share a span.
	spanHasLabel := make(map[string]bool, len(labels))
	for _, l := range labels {
		spanHasLabel[l.Span] = true
	}

	split, err := LoadSplit(filepath.Join(avaCorpusDir(t), "split.yaml"))
	if err != nil {
		t.Fatalf("LoadSplit: %v", err)
	}
	if len(split.HeldOut) == 0 {
		t.Fatal("split.yaml has zero held-out spans")
	}

	seen := make(map[string]bool, len(split.HeldOut))
	for _, span := range split.HeldOut {
		if seen[span] {
			t.Errorf("held_out lists span %q more than once", span)
		}
		seen[span] = true

		if !spanHasLabel[span] {
			t.Errorf("held_out names a span with no matching label (Split.Filter would silently ignore it): %q", span)
		}
	}

	heldOutSpanSet := make(map[string]bool, len(split.HeldOut))
	for _, span := range split.HeldOut {
		heldOutSpanSet[span] = true
	}
	byFamily := map[string]int{}
	for _, l := range labels {
		if heldOutSpanSet[l.Span] {
			byFamily[l.Expected.Predicate]++
		}
	}

	for _, f := range config.Default().FamilyNames() {
		if byFamily[f] == 0 {
			t.Errorf("family %q has zero held-out spans -- the split is degenerate for this family", f)
		}
	}

	heldOut, rest := split.Filter(labels)
	// wantHeldOutLabels is the count of LABELS whose span is in the
	// held-out set, independently recomputed from labels+heldOutSpanSet
	// rather than assumed equal to len(split.HeldOut) (a count of unique
	// span-text strings): T1.33 means those two counts can legitimately
	// differ now that a span can carry more than one Label (owns_account
	// held out span-for-span alongside has_balance).
	wantHeldOutLabels := 0
	for _, l := range labels {
		if heldOutSpanSet[l.Span] {
			wantHeldOutLabels++
		}
	}
	if len(heldOut) != wantHeldOutLabels {
		t.Errorf("split.Filter matched %d labels, want %d (labels whose span is in held_out)", len(heldOut), wantHeldOutLabels)
	}
	if len(rest)+len(heldOut) != len(labels) {
		t.Errorf("split.Filter partition sizes %d+%d don't add up to the corpus size %d", len(heldOut), len(rest), len(labels))
	}
}

// TestAvaCorpusHeldOutMeetsT132Floor is T1.32's own acc-line floor: every
// one of the 12 families named in T1.29's acc line has >= 20 scored units
// (== held-out Labels, one per (span, predicate) pair -- T1.32's "a scored
// unit is one predicate instance actually evaluated, not a span" wording
// matters here for real: T1.33 gives owns_account a second Label on
// several has_balance spans, so a held-out span no longer maps to exactly
// one scored unit corpus-wide, only to one per family; byFamily below
// counts Labels, never spans, so this stays correct) in the held-out set
// -- the statistical floor chief-architect set so a bootstrap recall
// confidence interval is meaningful (at 4 units, a true recall of 0.85
// fails a point-estimate 0.80 bar about one run in three from sampling
// noise alone; at 20 units, under one in eight). Checked against all 13
// seeded families, not just T1.29's 12 -- gen_corpus.go expands every
// family uniformly, so `costs` clears the floor too even though it isn't
// named in T1.29's acc line.
func TestAvaCorpusHeldOutMeetsT132Floor(t *testing.T) {
	labels := loadAvaLabels(t)

	split, err := LoadSplit(filepath.Join(avaCorpusDir(t), "split.yaml"))
	if err != nil {
		t.Fatalf("LoadSplit: %v", err)
	}
	heldOutSpanSet := make(map[string]bool, len(split.HeldOut))
	for _, span := range split.HeldOut {
		heldOutSpanSet[span] = true
	}

	byFamily := map[string]int{}
	for _, l := range labels {
		if heldOutSpanSet[l.Span] {
			byFamily[l.Expected.Predicate]++
		}
	}

	for _, f := range config.Default().FamilyNames() {
		if byFamily[f] < 20 {
			t.Errorf("family %q has %d held-out scored units, want >= 20 (T1.32 floor)", f, byFamily[f])
		}
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
