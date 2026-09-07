package direction

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/dira/ledger"
	coredirection "github.com/sirerun/serenity/internal/direction"
	"github.com/sirerun/serenity/internal/direction/check"
	coredisp "github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/secrets"
	"github.com/sirerun/serenity/internal/server"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

func TestMain(m *testing.M) {
	secrets.MockForTesting() // never touch the real OS keychain from tests
	m.Run()
}

var fixedNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

// testEnv bundles a real ledger Store (over a temp brain root) and a real
// disposition Store (over a temp index.Open engine), plus Handlers over
// both -- mirroring internal/server/disposition_test.go's own testEnv
// convention.
type testEnv struct {
	root      string
	ledger    *coredirection.Store
	dispStore *coredisp.Store
	handlers  *Handlers
}

func newTestEnv(t *testing.T, opts ...Option) *testEnv {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".dira", "entries"), 0o755); err != nil {
		t.Fatal(err)
	}

	q := writer.NewQueue(nil)
	t.Cleanup(q.Close)
	ledgerStore := coredirection.NewStore(root, q)

	eng, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	dispStore := coredisp.NewStore(eng)

	allOpts := append([]Option{WithClock(fakeClock{fixedNow})}, opts...)
	h := New(ledgerStore, dispStore, root, allOpts...)
	return &testEnv{root: root, ledger: ledgerStore, dispStore: dispStore, handlers: h}
}

// startTestServer registers env's handlers onto a real *server.Server
// bound to loopback and serves it in the background -- the same real-
// HTTP-listener convention internal/server/disposition_test.go
// establishes.
func startTestServer(t *testing.T, h *Handlers) (baseURL, token string) {
	t.Helper()
	const want = "s3cr3t-token"
	s := server.New(server.Config{Bind: "127.0.0.1:0", TokenSource: func() (string, error) { return want, nil }})
	h.Register(s)
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = s.Serve(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(8 * time.Second):
			t.Fatal("server did not shut down within 8s")
		}
	})
	return "http://" + s.Addr(), want
}

func postJSON(t *testing.T, base, token, path string, body any) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, base+path, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	return buf.Bytes()
}

// --- ledger fixture helpers ---------------------------------------------

func seedConstraint(t *testing.T, s *coredirection.Store, id, title, action, paramsYAML, whyNot, revisitIf string) {
	t.Helper()
	body := "Fixture constraint.\n\n```serenity:applies_when\naction: " + action + "\n"
	if paramsYAML != "" {
		body += "params: " + paramsYAML + "\n"
	}
	body += "```\n"
	entry := &ledger.Entry{
		ID:      id,
		Kind:    ledger.KindConstraint,
		Title:   title,
		State:   ledger.StateActive,
		Created: "2026-08-28T00:00:00Z",
		Alternatives: []ledger.Alternative{
			{Option: "no ceiling", WhyNot: whyNot, RevisitIf: revisitIf},
		},
		Body: body,
	}
	if err := s.Create(context.Background(), entry); err != nil {
		t.Fatalf("seedConstraint %s: %v", id, err)
	}
}

func seedIntent(t *testing.T, s *coredirection.Store, id, title string, derivesFrom ...string) {
	t.Helper()
	var edges []ledger.Edge
	for _, d := range derivesFrom {
		edges = append(edges, ledger.Edge{Type: ledger.EdgeDerivesFrom, To: d})
	}
	entry := &ledger.Entry{
		ID: id, Kind: ledger.KindIntent, Title: title, State: ledger.StateActive,
		Created: "2026-08-28T00:00:00Z", Edges: edges,
	}
	if err := s.Create(context.Background(), entry); err != nil {
		t.Fatalf("seedIntent %s: %v", id, err)
	}
}

func seedQuestion(t *testing.T, s *coredirection.Store, id, title string) {
	t.Helper()
	entry := &ledger.Entry{
		ID: id, Kind: ledger.KindQuestion, Title: title, State: ledger.StateOpen,
		Created: "2026-08-28T00:00:00Z",
	}
	if err := s.Create(context.Background(), entry); err != nil {
		t.Fatalf("seedQuestion %s: %v", id, err)
	}
}

// --- brief ----------------------------------------------------------------

// TestBriefFreshLedgerReturnsAllFourSectionsEmpty proves brief's four
// fixed sections are always present, in RFC 0001 §12's priority order,
// even on a ledger with nothing in it -- the same "fixed sections are
// unconditional" discipline T2.17's own daily-briefing test establishes.
func TestBriefFreshLedgerReturnsAllFourSectionsEmpty(t *testing.T) {
	env := newTestEnv(t)
	base, token := startTestServer(t, env.handlers)

	resp := postJSON(t, base, token, "/direction/brief", map[string]any{"token_budget": 800})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("brief: status %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var out BriefResponse
	if err := json.Unmarshal(readBody(t, resp), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.BudgetEstimator != "words" {
		t.Errorf("budget_estimator = %q, want %q", out.BudgetEstimator, "words")
	}
	wantNames := []string{"precepts", "intents", "entities", "questions"}
	if len(out.Sections) != len(wantNames) {
		t.Fatalf("got %d sections, want %d", len(out.Sections), len(wantNames))
	}
	for i, want := range wantNames {
		sec := out.Sections[i]
		if sec.Name != want {
			t.Errorf("section %d name = %q, want %q", i, sec.Name, want)
		}
		if sec.Omitted != 0 {
			t.Errorf("section %q omitted = %d on an empty ledger, want 0 (nothing to omit, not dropped)", sec.Name, sec.Omitted)
		}
		if len(sec.Items) != 0 {
			t.Errorf("section %q items = %v, want empty", sec.Name, sec.Items)
		}
	}
}

// TestBriefZeroBudgetReturnsMinimalValidObject is T4.6's own acc-line
// clause: "a zero budget returns the minimal valid object" -- every
// section with real candidate items is dropped whole (Omitted equal to
// its candidate count), yet the response itself is still fully
// structured: four named sections, budget_estimator named, nothing
// malformed or missing.
func TestBriefZeroBudgetReturnsMinimalValidObject(t *testing.T) {
	env := newTestEnv(t)
	seedConstraint(t, env.ledger, "cst-0001", "fixture spend ceiling", "spend_over", "{amount: {gte: 200}}",
		"unbounded spend risk", "quarterly review")
	seedQuestion(t, env.ledger, "qst-0001", "should we raise the ceiling?")

	base, token := startTestServer(t, env.handlers)
	resp := postJSON(t, base, token, "/direction/brief", map[string]any{"token_budget": 0})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("brief: status %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var out BriefResponse
	if err := json.Unmarshal(readBody(t, resp), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.BudgetEstimator != "words" {
		t.Errorf("budget_estimator = %q, want %q", out.BudgetEstimator, "words")
	}
	if len(out.Sections) != 4 {
		t.Fatalf("got %d sections, want 4 even at budget 0", len(out.Sections))
	}
	for _, sec := range out.Sections {
		switch sec.Name {
		case "precepts":
			if sec.Omitted != 1 || len(sec.Items) != 0 {
				t.Errorf("precepts at budget 0 = %+v, want omitted=1 items=[]", sec)
			}
		case "questions":
			if sec.Omitted != 1 || len(sec.Items) != 0 {
				t.Errorf("questions at budget 0 = %+v, want omitted=1 items=[]", sec)
			}
		case "intents", "entities":
			if sec.Omitted != 0 || len(sec.Items) != 0 {
				t.Errorf("%s at budget 0 with no candidates = %+v, want omitted=0 items=[] (nothing to omit, not dropped)", sec.Name, sec)
			}
		}
	}
}

// TestBriefBudget800DropsOverflowingSectionWhole is T4.6's own acc-line
// clause: "budget 800 fills sections by priority and drops an overflowing
// section whole with omitted: N". ledger.Entry.Validate caps Title at 120
// characters (~entry.go), so the bulk needed to overflow an 800-word
// budget is put in each precept's why_not instead (Alternative.WhyNot
// carries no such cap) -- twelve precepts (preceptCap) at roughly 70
// words of why_not each comfortably clears 800 words on its own, so the
// precepts section -- despite being RFC 0001 §12's highest-priority
// section -- is dropped whole once its own true cost is measured, rather
// than truncated to fit. Questions, seeded short, still fill: dropping
// one section never starves a later one of budget (briefing.Pack's own
// documented behavior, unchanged here).
func TestBriefBudget800DropsOverflowingSectionWhole(t *testing.T) {
	env := newTestEnv(t)

	longWord := "supercalifragilisticexpialidocious "
	var longWhyNot bytes.Buffer
	for i := 0; i < 70; i++ {
		longWhyNot.WriteString(longWord)
	}
	for i := 1; i <= preceptCap; i++ {
		id := fmt.Sprintf("cst-%04d", i)
		seedConstraint(t, env.ledger, id, fmt.Sprintf("fixture precept number %d", i), "spend_over", "{amount: {gte: 200}}",
			longWhyNot.String(), "quarterly review")
	}
	seedQuestion(t, env.ledger, "qst-0001", "should we raise the ceiling?")
	seedQuestion(t, env.ledger, "qst-0002", "who owns this precept?")

	base, token := startTestServer(t, env.handlers)
	resp := postJSON(t, base, token, "/direction/brief", map[string]any{"token_budget": 800})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("brief: status %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var out BriefResponse
	if err := json.Unmarshal(readBody(t, resp), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}

	var precepts, questions BriefSectionWire
	for _, sec := range out.Sections {
		switch sec.Name {
		case "precepts":
			precepts = sec
		case "questions":
			questions = sec
		}
	}
	if precepts.Omitted != preceptCap || len(precepts.Items) != 0 {
		t.Errorf("precepts = %+v, want dropped whole with omitted=%d", precepts, preceptCap)
	}
	if questions.Omitted != 0 || len(questions.Items) != 2 {
		t.Errorf("questions = %+v, want both fixture questions included whole (dropping precepts must not starve a later section)", questions)
	}
}

// TestBriefIntentItemRendersDerivesFromEdges proves intents actually reads
// the ledger (not a stub) and surfaces derives_from edges -- RFC 0001
// §12's "current intents chained to ambitions", read via dira's existing
// edge vocabulary (see intentItems' own doc comment).
func TestBriefIntentItemRendersDerivesFromEdges(t *testing.T) {
	env := newTestEnv(t)
	seedIntent(t, env.ledger, "int-0001", "ship the MVP", "int-0000")

	base, token := startTestServer(t, env.handlers)
	resp := postJSON(t, base, token, "/direction/brief", map[string]any{"token_budget": 800})
	var out BriefResponse
	if err := json.Unmarshal(readBody(t, resp), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var intents BriefSectionWire
	for _, sec := range out.Sections {
		if sec.Name == "intents" {
			intents = sec
		}
	}
	if len(intents.Items) != 1 {
		t.Fatalf("intents = %+v, want exactly one item", intents)
	}
	want := "int-0001: ship the MVP (derives_from: int-0000)"
	if intents.Items[0] != want {
		t.Errorf("intent item = %q, want %q", intents.Items[0], want)
	}
}

// --- brief: entities ------------------------------------------------------

func writeFixtureEntity(t *testing.T, root, typ, slug, title string, claims ...domain.Claim) {
	t.Helper()
	fw := &store.FenceWriter{Root: root, Vocabulary: map[string]bool{"works_at": true, "prefers": true}}
	page := &store.EntityPage{
		Entity: domain.Entity{Type: typ, Slug: slug},
		Title:  title,
		Claims: claims,
	}
	if _, err := fw.WriteEntity(page); err != nil {
		t.Fatalf("WriteEntity %s/%s: %v", typ, slug, err)
	}
}

// TestBriefEntitiesRankedByLexicalOverlapWithTaskHint proves the entities
// section actually reads brain/entities/**/*.md (not a stub) and ranks by
// the disclosed lexical heuristic: an entity whose claim mentions the
// task_hint's own words ranks first, ahead of an unrelated entity, even
// though the unrelated one was written more recently.
func TestBriefEntitiesRankedByLexicalOverlapWithTaskHint(t *testing.T) {
	env := newTestEnv(t)
	writeFixtureEntity(t, env.root, "person", "jane-doe", "Jane Doe", domain.Claim{
		ID: "c1", SubjectSlug: "jane-doe", Predicate: "works_at", Object: "Acme Rocket Corp",
		Confidence: 0.9, ValidFrom: "2026-01-01T00:00:00Z", SourceRef: "e1#1", State: domain.StateActive,
	})
	writeFixtureEntity(t, env.root, "person", "john-smith", "John Smith", domain.Claim{
		ID: "c2", SubjectSlug: "john-smith", Predicate: "prefers", Object: "tea over coffee",
		Confidence: 0.7, ValidFrom: "2026-06-01T00:00:00Z", SourceRef: "e2#1", State: domain.StateActive,
	})

	base, token := startTestServer(t, env.handlers)
	resp := postJSON(t, base, token, "/direction/brief", map[string]any{
		"task_hint": "who works at Acme Rocket Corp?", "token_budget": 800,
	})
	var out BriefResponse
	if err := json.Unmarshal(readBody(t, resp), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var entities BriefSectionWire
	for _, sec := range out.Sections {
		if sec.Name == "entities" {
			entities = sec
		}
	}
	if len(entities.Items) != 2 {
		t.Fatalf("entities = %+v, want both fixture entities", entities)
	}
	want := "person/jane-doe: works_at Acme Rocket Corp (confidence 0.90, source e1#1)"
	if entities.Items[0] != want {
		t.Errorf("entities[0] = %q, want %q (the task_hint-relevant entity ranked first)", entities.Items[0], want)
	}
}

// TestBriefEntitiesFallBackToRecencyWithNoTaskHint proves that with no
// task_hint, ranking falls back entirely to most-recent-claim recency.
func TestBriefEntitiesFallBackToRecencyWithNoTaskHint(t *testing.T) {
	env := newTestEnv(t)
	writeFixtureEntity(t, env.root, "person", "older", "Older Entity", domain.Claim{
		ID: "c1", SubjectSlug: "older", Predicate: "prefers", Object: "coffee",
		Confidence: 0.5, ValidFrom: "2026-01-01T00:00:00Z", SourceRef: "e1#1", State: domain.StateActive,
	})
	writeFixtureEntity(t, env.root, "person", "newer", "Newer Entity", domain.Claim{
		ID: "c2", SubjectSlug: "newer", Predicate: "prefers", Object: "tea",
		Confidence: 0.5, ValidFrom: "2026-08-01T00:00:00Z", SourceRef: "e2#1", State: domain.StateActive,
	})

	base, token := startTestServer(t, env.handlers)
	resp := postJSON(t, base, token, "/direction/brief", map[string]any{"token_budget": 800})
	var out BriefResponse
	if err := json.Unmarshal(readBody(t, resp), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var entities BriefSectionWire
	for _, sec := range out.Sections {
		if sec.Name == "entities" {
			entities = sec
		}
	}
	if len(entities.Items) != 2 {
		t.Fatalf("entities = %+v, want both fixture entities", entities)
	}
	if entities.Items[0] != "person/newer: prefers tea (confidence 0.50, source e2#1)" {
		t.Errorf("entities[0] = %q, want the more-recently-claimed entity first", entities.Items[0])
	}
}

// --- check_plan -------------------------------------------------------

func TestCheckPlanRequiresExactlyOneOfPlanTextOrActions(t *testing.T) {
	env := newTestEnv(t)
	base, token := startTestServer(t, env.handlers)

	for name, body := range map[string]map[string]any{
		"neither": {},
		"both":    {"plan_text": "deploy to prod", "actions": []map[string]any{{"action": "deploy_to_prod"}}},
	} {
		t.Run(name, func(t *testing.T) {
			resp := postJSON(t, base, token, "/direction/check_plan", body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body %s", resp.StatusCode, readBody(t, resp))
			}
		})
	}
}

// TestCheckPlanViolatedStructuredActions is the direct structured-actions
// path, exercised over real HTTP.
func TestCheckPlanViolatedStructuredActions(t *testing.T) {
	env := newTestEnv(t)
	seedConstraint(t, env.ledger, "cst-0001", "fixture spend ceiling", "spend_over", "{amount: {gte: 200}}",
		`Unbounded spend risk: "no ceiling" was rejected outright.`, "quarterly budget review")

	base, token := startTestServer(t, env.handlers)
	resp := postJSON(t, base, token, "/direction/check_plan", map[string]any{
		"actions": []map[string]any{{"action": "spend_over", "params": map[string]any{"amount": 500}}},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var wire check.WireResult
	if err := json.Unmarshal(readBody(t, resp), &wire); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if wire.Status != string(check.StatusViolated) {
		t.Errorf("status = %q, want %q", wire.Status, check.StatusViolated)
	}
	if len(wire.Constraints) != 1 || wire.Constraints[0].PreceptID != "cst-0001" {
		t.Errorf("constraints = %+v, want one verdict for cst-0001", wire.Constraints)
	}
}

// TestCheckPlanFreeTextWithNoRouterConfiguredReturnsUnverified proves the
// HTTP surface honors the same floor internal/cli.runCheck does: with no
// router wired (Handlers' default), free-text check_plan is
// StatusUnverified -- never a silent pass.
func TestCheckPlanFreeTextWithNoRouterConfiguredReturnsUnverified(t *testing.T) {
	env := newTestEnv(t)
	base, token := startTestServer(t, env.handlers)

	resp := postJSON(t, base, token, "/direction/check_plan", map[string]any{"plan_text": "deploy to prod now"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var wire check.WireResult
	if err := json.Unmarshal(readBody(t, resp), &wire); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if wire.Status != "unverified" {
		t.Errorf("status = %q, want %q", wire.Status, "unverified")
	}
}

// buildSerenityBinary builds cmd/serenity into a temp path, mirroring
// internal/cli.buildSerenityBinary's own established technique for a
// genuine binary-vs-package comparison.
func buildSerenityBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "serenity")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/sirerun/serenity/cmd/serenity")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build serenity binary: %v\n%s", err, out)
	}
	return bin
}

// TestCheckPlanWireVerdictsEqualCLIJSONOnSameFixture is T4.6's own
// acc-line clause, exercised literally: the same fixture ledger, checked
// once through the built `serenity check --json` binary and once through
// this package's HTTP check_plan handler, must parse to the identical
// check.WireResult. Both paths call the same check.ToWire converter (see
// this package's doc comment), so this test is the proof that promise
// actually holds against a real fixture, not just a structural argument.
func TestCheckPlanWireVerdictsEqualCLIJSONOnSameFixture(t *testing.T) {
	bin := buildSerenityBinary(t)

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".dira", "entries"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.Default().Save(filepath.Join(root, config.FileName)); err != nil {
		t.Fatal(err)
	}
	q := writer.NewQueue(nil)
	defer q.Close()
	ledgerStore := coredirection.NewStore(root, q)
	seedConstraint(t, ledgerStore, "cst-0001", "fixture spend ceiling", "spend_over", "{amount: {gte: 200}}",
		`Unbounded spend risk: "no ceiling" was rejected outright.`, "quarterly budget review")

	const actionsJSON = `[{"action":"spend_over","params":{"amount":500}}]`

	cliCmd := exec.Command(bin, "-C", root, "check", "--actions", actionsJSON, "--json")
	cliOut, err := cliCmd.CombinedOutput()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("run CLI check: %v\n%s", err, cliOut)
		}
		// A violated verdict exits 2 (ADR 010) -- expected here, not a
		// failure to run.
	}
	var cliWire check.WireResult
	if err := json.Unmarshal(cliOut, &cliWire); err != nil {
		t.Fatalf("parse CLI json: %v\nraw: %s", err, cliOut)
	}

	eng, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	h := New(ledgerStore, coredisp.NewStore(eng), root)
	base, token := startTestServer(t, h)

	resp := postJSON(t, base, token, "/direction/check_plan", map[string]any{
		"actions": []map[string]any{{"action": "spend_over", "params": map[string]any{"amount": 500}}},
	})
	var httpWire check.WireResult
	if err := json.Unmarshal(readBody(t, resp), &httpWire); err != nil {
		t.Fatalf("decode HTTP response: %v", err)
	}

	if !reflect.DeepEqual(cliWire, httpWire) {
		t.Fatalf("check_plan HTTP verdict != `serenity check --json` verdict on the same fixture:\nCLI:  %+v\nHTTP: %+v", cliWire, httpWire)
	}
	if httpWire.Status != string(check.StatusViolated) {
		t.Fatalf("fixture sanity: status = %q, want %q (test would pass vacuously on an empty verdict otherwise)", httpWire.Status, check.StatusViolated)
	}
}

// --- propose --------------------------------------------------------------

func TestProposeUnknownKindReturnsInvalidKind(t *testing.T) {
	env := newTestEnv(t)
	base, token := startTestServer(t, env.handlers)

	resp := postJSON(t, base, token, "/direction/propose", map[string]any{
		"kind": "reconcile", "payload": json.RawMessage(`{}`),
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", resp.StatusCode, readBody(t, resp))
	}
}

// TestProposePreceptDraftMissingAlternativesRejectedBeforeStaging proves
// validateProposePayload actually runs before anything is staged: a
// payload with no alternatives is rejected with 400, and no disposition
// item is created for it.
func TestProposePreceptDraftMissingAlternativesRejectedBeforeStaging(t *testing.T) {
	env := newTestEnv(t)
	base, token := startTestServer(t, env.handlers)

	before, err := env.dispStore.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(coredirection.PreceptDraftPayload{
		Title: "cap monthly spend at $50", Body: "because runaway spend",
	})
	resp := postJSON(t, base, token, "/direction/propose", map[string]any{
		"kind": "precept_draft", "payload": json.RawMessage(payload),
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", resp.StatusCode, readBody(t, resp))
	}

	after, err := env.dispStore.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("a rejected propose call staged %d item(s), want 0", len(after)-len(before))
	}
}

// hashDir sha256-hashes every file under dir (relative path + content, in
// filepath.WalkDir's own deterministic per-directory lexical order) into
// one digest -- a directory whose content genuinely did not change
// produces the identical digest both times.
func hashDir(t *testing.T, dir string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		h.Write([]byte(rel))
		h.Write(data)
		return nil
	})
	if err != nil {
		t.Fatalf("hashDir %s: %v", dir, err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// TestProposePreceptDraftCreatesItemAndDiraHashUnchanged is T4.6's own
// acc-line clause, exercised literally: propose(precept_draft) must
// create a disposition item, and .dira/'s content hash must be
// byte-for-byte identical before and after the call -- the strongest
// available proof that this handler never touches the ledger (see this
// package's doc comment on why that holds by construction, not just by
// this test passing).
func TestProposePreceptDraftCreatesItemAndDiraHashUnchanged(t *testing.T) {
	env := newTestEnv(t)
	// A pre-existing precept, so .dira/ is non-empty and this test would
	// actually notice a stray write into it, not just an empty-directory
	// hash trivially matching itself.
	seedConstraint(t, env.ledger, "cst-0001", "fixture spend ceiling", "spend_over", "{amount: {gte: 200}}",
		"unbounded spend risk", "quarterly review")

	base, token := startTestServer(t, env.handlers)
	diraDir := filepath.Join(env.root, ".dira")
	before := hashDir(t, diraDir)

	payload, _ := json.Marshal(coredirection.PreceptDraftPayload{
		QuestionID: "qst-0001",
		Question:   "should we cap monthly spend?",
		Answer:     "yes, at $50",
		Title:      "cap monthly spend at $50",
		Body:       "because runaway spend",
		Alternatives: []domain.RejectedAlternative{
			{Option: "no ceiling", WhyNot: "unbounded risk", RevisitIf: "quarterly review"},
		},
	})
	resp := postJSON(t, base, token, "/direction/propose", map[string]any{
		"kind": "precept_draft", "payload": json.RawMessage(payload),
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("propose: status %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var out ProposeResponse
	if err := json.Unmarshal(readBody(t, resp), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.ItemID == "" {
		t.Fatal("propose: empty item_id")
	}

	after := hashDir(t, diraDir)
	if before != after {
		t.Fatalf(".dira hash changed across a propose call: before=%s after=%s", before, after)
	}

	item, err := env.dispStore.Get(context.Background(), out.ItemID)
	if err != nil {
		t.Fatalf("Get %s: %v", out.ItemID, err)
	}
	if item.Kind != coredisp.KindPreceptDraft {
		t.Errorf("item kind = %s, want %s", item.Kind, coredisp.KindPreceptDraft)
	}
	if item.State != coredisp.StatePending {
		t.Errorf("item state = %s, want %s", item.State, coredisp.StatePending)
	}
}

// TestProposeEffectCreatesItem proves the "effect" kind also stages
// successfully with its lighter validation (see validateProposePayload's
// doc comment on why it is lighter than precept_draft's).
func TestProposeEffectCreatesItem(t *testing.T) {
	env := newTestEnv(t)
	base, token := startTestServer(t, env.handlers)

	payload := `{"row":{"id":"s1","task_class":"judgment","tier":"judgment","provider":"anthropic","model_version":"claude-x@v1","input_tokens":500,"output_tokens":200,"cost_usd":1.5,"occurred_at":"2026-09-07T11:00:00Z"},"month_to_date_usd":40,"ceiling_usd":50}`
	resp := postJSON(t, base, token, "/direction/propose", map[string]any{
		"kind": "effect", "payload": json.RawMessage(payload),
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("propose: status %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var out ProposeResponse
	if err := json.Unmarshal(readBody(t, resp), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	item, err := env.dispStore.Get(context.Background(), out.ItemID)
	if err != nil {
		t.Fatalf("Get %s: %v", out.ItemID, err)
	}
	if item.Kind != coredisp.KindEffect {
		t.Errorf("item kind = %s, want %s", item.Kind, coredisp.KindEffect)
	}
}

func TestProposeEmptyPayloadReturnsInvalidPayload(t *testing.T) {
	env := newTestEnv(t)
	base, token := startTestServer(t, env.handlers)

	resp := postJSON(t, base, token, "/direction/propose", map[string]any{"kind": "effect"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", resp.StatusCode, readBody(t, resp))
	}
}

// --- method handling ------------------------------------------------------

func TestHandlersRejectNonPOST(t *testing.T) {
	env := newTestEnv(t)
	base, token := startTestServer(t, env.handlers)

	for _, path := range []string{"/direction/brief", "/direction/check_plan", "/direction/propose"} {
		req, err := http.NewRequest(http.MethodGet, base+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("GET %s: status = %d, want 405", path, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
}
