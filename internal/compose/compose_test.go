package compose

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/router"
	"github.com/sirerun/serenity/internal/store"
)

// fakeProvider is a test double implementing router.Provider -- the same
// pattern internal/extract/extract_test.go and internal/router/router_test.go
// use. Test-file only, per the zero-stub policy: no production code path
// constructs one.
type fakeProvider struct {
	modelVersion string
	resp         router.Response
	err          error
}

func (f *fakeProvider) Name() string         { return "fake" }
func (f *fakeProvider) ModelVersion() string { return f.modelVersion }
func (f *fakeProvider) Send(_ context.Context, _ string) (router.Response, error) {
	return f.resp, f.err
}

// fakeLedger is a test double implementing router.SpendLedger.
type fakeLedger struct{ entries []router.SpendEntry }

func (f *fakeLedger) Record(_ context.Context, e router.SpendEntry) error {
	f.entries = append(f.entries, e)
	return nil
}

// fakeSearchStore satisfies search.Store with nothing indexed. Ask's
// relevant() still finds candidates through its other signal --
// lexicalScore's direct match against each claim's own subject/predicate/
// object text -- so these tests exercise that path deliberately rather
// than standing up a real *index.SQLite with chunks to search over
// (internal/search's own tests already cover the chunk-search ranking
// itself).
type fakeSearchStore struct{}

func (fakeSearchStore) SearchVectors(context.Context, string, []float32, int) ([]index.Hit, error) {
	return nil, nil
}

func (fakeSearchStore) SearchFTS(context.Context, string, int) ([]index.Hit, error) {
	return nil, nil
}

func (fakeSearchStore) VectorFor(context.Context, string, string) ([]float32, bool, error) {
	return nil, false, nil
}

// newTestRouter builds a real *router.Router (judgment tier, matching
// TaskClassComposerSynthesis's fixed resolution) over a fake provider --
// the router's own tier resolution, confidence clamp, and spend-ledger
// recording all run for real, per Completer's doc comment.
func newTestRouter(fp *fakeProvider) *router.Router {
	return router.New(map[router.Tier]router.Provider{router.TierJudgment: fp}, &fakeLedger{})
}

func fixedNow(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

const avaSlug = "ava-standardo"

// avaFact/avaLabel decode just enough of internal/eval.ExpectedFact/
// Label's YAML shape (ADR-005) to read evals/corpora/ava/labels/*.yaml
// directly. Not imported from internal/eval: the Ava corpus (T1.14) is
// extraction-labeled spans, not QA pairs (disclosed scoping call, T1.12
// dispatch) -- these tests derive a small, self-contained QA subset from
// its real labeled facts rather than reading a QA-shaped fixture that
// does not exist, and reading it directly here (never writing back to
// it) keeps that derivation local to this package instead of adding a
// production dependency on the eval harness.
type avaFact struct {
	Predicate string `yaml:"predicate"`
	Object    string `yaml:"object"`
	ValidFrom string `yaml:"valid_from,omitempty"`
	ValidTo   string `yaml:"valid_to,omitempty"`
}

type avaLabel struct {
	Expected avaFact `yaml:"expected"`
}

// loadAvaFact reads one evals/corpora/ava/labels/<name> file, located
// relative to this test file's own path (the technique
// internal/eval/ava_corpus_test.go uses) so it resolves regardless of the
// working directory a test runner uses.
func loadAvaFact(t *testing.T, name string) avaFact {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "evals", "corpora", "ava", "labels", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (is evals/corpora/ava/labels/ present?)", path, err)
	}
	var l avaLabel
	if err := yaml.Unmarshal(b, &l); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return l.Expected
}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse date %q: %v", s, err)
	}
	return tm
}

// writeAvaEntity writes one fence-tier entity page for avaSlug carrying
// claims -- store.FenceWriter's real render path, not a hand-built file,
// so parsing round-trips exactly the way AllClaims/Ask expects.
func writeAvaEntity(t *testing.T, root string, claims []domain.Claim) {
	t.Helper()
	fw := store.NewFenceWriter(root)
	page := store.NewEntityPage(domain.Entity{Type: "person", Slug: avaSlug})
	page.Claims = claims
	b, err := fw.RenderEntity(page)
	if err != nil {
		t.Fatalf("render entity: %v", err)
	}
	path := fw.PathFor("person", avaSlug)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestAskCitationsResolveToRealClaims is the acc line's first behavior:
// on the Ava QA subset, every citation resolves to a real claim id. The
// fake model's response deliberately also cites a claim tag that was
// never retrieved -- extractCitations must drop it silently rather than
// surface it.
func TestAskCitationsResolveToRealClaims(t *testing.T) {
	root := t.TempDir()
	fact := loadAvaFact(t, "ava-belongs_to_project-01.yaml") // project-lighthouse

	claim := domain.Claim{
		ID:          "bp-lighthouse",
		SubjectSlug: avaSlug,
		Predicate:   fact.Predicate,
		Object:      fact.Object,
		Confidence:  0.9,
		ValidFrom:   fact.ValidFrom,
		ValidTo:     fact.ValidTo,
		State:       domain.StateActive,
		Family:      fact.Predicate,
		SourceRef:   "ava#1",
		Provenance:  domain.Provenance{SourceSHA256: "aaaa1111", ObservedAt: mustDate(t, "2024-02-01")},
	}
	writeAvaEntity(t, root, []domain.Claim{claim})

	fp := &fakeProvider{
		modelVersion: "fake-composer@v1",
		resp: router.Response{Text: "Ava belongs to Project Lighthouse [claim:bp-lighthouse], " +
			"and also supposedly [claim:hallucinated-id] which was never shown to the model."},
	}
	c := New(root, config.Default(), fakeSearchStore{}, nil, newTestRouter(fp), "fake-composer@v1")

	ans, err := c.Ask(context.Background(), "What project does Ava belong to?")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if ans.Gap != "" {
		t.Fatalf("Gap = %q, want empty (a matching claim exists)", ans.Gap)
	}
	if len(ans.Citations) != 1 {
		t.Fatalf("Citations = %v, want exactly 1 (the hallucinated tag must be dropped)", ans.Citations)
	}
	got := ans.Citations[0]
	if got.ClaimID != "bp-lighthouse" {
		t.Fatalf("Citations[0].ClaimID = %q, want %q", got.ClaimID, "bp-lighthouse")
	}
	if got.Object != "project-lighthouse" {
		t.Fatalf("Citations[0].Object = %q, want %q", got.Object, "project-lighthouse")
	}
	if strings.Contains(ans.Text, "hallucinated-id") {
		t.Fatalf("Text still contains the hallucinated tag verbatim: %q", ans.Text)
	}
}

// TestAskStaleClaimSupersessionChain is the acc line's second behavior:
// the stale-claim fixture answer contains the supersession chain. Ava's
// real has_role history (T1.14) gives a genuine chronological
// supersession -- senior-backend-engineer (2022-07 to 2023-12) replaced
// by engineering-manager (2024-01 onward) -- rather than an invented one.
func TestAskStaleClaimSupersessionChain(t *testing.T) {
	root := t.TempDir()
	oldFact := loadAvaFact(t, "ava-has_role-09.yaml") // senior-backend-engineer, valid_to 2023-12
	newFact := loadAvaFact(t, "ava-has_role-15.yaml") // engineering-manager, valid_from 2024-01

	oldClaim := domain.Claim{
		ID:           "role-sbe",
		SubjectSlug:  avaSlug,
		Predicate:    oldFact.Predicate,
		Object:       oldFact.Object,
		Confidence:   0.88,
		ValidFrom:    oldFact.ValidFrom,
		ValidTo:      oldFact.ValidTo,
		State:        domain.StateSuperseded,
		SupersededBy: "role-em",
		Family:       oldFact.Predicate,
		SourceRef:    "ava#2",
		Provenance:   domain.Provenance{SourceSHA256: "bbbb2222", ObservedAt: mustDate(t, "2022-07-01")},
	}
	newClaim := domain.Claim{
		ID:          "role-em",
		SubjectSlug: avaSlug,
		Predicate:   newFact.Predicate,
		Object:      newFact.Object,
		Confidence:  0.93,
		ValidFrom:   newFact.ValidFrom,
		ValidTo:     newFact.ValidTo,
		State:       domain.StateActive,
		Supersedes:  "role-sbe",
		Family:      newFact.Predicate,
		SourceRef:   "ava#3",
		Provenance:  domain.Provenance{SourceSHA256: "bbbb3333", ObservedAt: mustDate(t, "2024-01-01")},
	}
	writeAvaEntity(t, root, []domain.Claim{oldClaim, newClaim})

	fp := &fakeProvider{
		modelVersion: "fake-composer@v1",
		resp:         router.Response{Text: "Ava currently works as an engineering manager [claim:role-em]."},
	}
	c := New(root, config.Default(), fakeSearchStore{}, nil, newTestRouter(fp), "fake-composer@v1")

	ans, err := c.Ask(context.Background(), "What is Ava's current role?")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if ans.Gap != "" {
		t.Fatalf("Gap = %q, want empty", ans.Gap)
	}
	if len(ans.Supersessions) != 1 {
		t.Fatalf("Supersessions = %v, want exactly 1", ans.Supersessions)
	}
	chain := ans.Supersessions[0].Chain
	if len(chain) != 2 || chain[0].ClaimID != "role-sbe" || chain[1].ClaimID != "role-em" {
		t.Fatalf("Chain = %v, want [role-sbe role-em] oldest first", chain)
	}
	const want = "Supersession (ava-standardo has_role): believed senior-backend-engineer until 2023-12 [claim:role-sbe], then now engineering-manager [claim:role-em]."
	if !strings.Contains(ans.Text, want) {
		t.Fatalf("Text = %q\ndoes not contain the expected supersession phrasing:\n%q", ans.Text, want)
	}
}

// TestAskUnanswerableGapStatement is the acc line's third behavior: an
// unanswerable question returns a non-empty gap statement naming the
// newest evidence's age, never a fabricated answer.
//
// Uses a shard-tier fact (has_balance): ShardStore's JSONL round-trip is
// lossless, unlike FenceWriter's table format (no column carries
// Provenance at all -- see ancestorsOf's doc comment), so
// Provenance.ObservedAt survives to make the age assertion below
// meaningful rather than incidentally exercising the "brain has no
// claims yet" branch instead.
//
// The question deliberately never names Ava: relevant()'s lexical signal
// also matches on the subject-slug's own tokens (RFC's other retrieval
// widening), so any question mentioning "Ava" pulls in every claim about
// her regardless of topic -- a real, disclosed scope limit of the
// lexical-only fallback (never a problem for a real embedding-backed
// relevantSubjects call, which ranks by meaning, not name overlap). This
// test asks about a topic genuinely absent from the brain to exercise
// the zero-candidate structural gap path specifically.
func TestAskUnanswerableGapStatement(t *testing.T) {
	root := t.TempDir()
	fact := loadAvaFact(t, "ava-has_balance-01.yaml") // Chase checking, $4,230.18, 2026-03
	claim := domain.Claim{
		SubjectSlug: avaSlug,
		Predicate:   fact.Predicate,
		Object:      fact.Object,
		Confidence:  0.9,
		ValidFrom:   fact.ValidFrom,
		State:       domain.StateActive,
		Family:      fact.Predicate,
		SourceRef:   "ava#4",
		Provenance:  domain.Provenance{SourceSHA256: "dddd5555", ObservedAt: mustDate(t, "2026-03-01")},
	}
	if err := store.NewShardStore(root).Append(claim); err != nil {
		t.Fatalf("shard append: %v", err)
	}

	// The fake provider would hand back a confident answer if ever
	// called -- Ask must never reach it once relevant() finds zero
	// candidates, which is exactly what this assertion structurally
	// proves: err below can only be nil via the zero-candidate gap path,
	// since fp.err would otherwise surface as an Ask error.
	fp := &fakeProvider{modelVersion: "fake-composer@v1", err: errUnexpectedCall}
	c := New(root, config.Default(), fakeSearchStore{}, nil, newTestRouter(fp), "fake-composer@v1")
	c.now = fixedNow(mustDate(t, "2026-08-28"))

	ans, err := c.Ask(context.Background(), "What's the weather forecast for tomorrow?")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if ans.Text != "" {
		t.Fatalf("Text = %q, want empty for an unanswerable question", ans.Text)
	}
	if ans.Gap == "" {
		t.Fatal("Gap is empty, want a non-empty gap statement")
	}
	if !strings.Contains(ans.Gap, "2026-03-01") {
		t.Fatalf("Gap = %q, want it to name the newest evidence's date (2026-03-01)", ans.Gap)
	}
	if !strings.Contains(ans.Gap, "180 days") {
		t.Fatalf("Gap = %q, want it to name the newest evidence's age (180 days, 2026-03-01 to 2026-08-28)", ans.Gap)
	}
}

var errUnexpectedCall = fmt.Errorf("compose test: the fake provider must never be called on the gap-statement path")
