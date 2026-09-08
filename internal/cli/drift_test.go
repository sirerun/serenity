package cli

// The drift pairs exercise shared CLI entry points and live protocol handlers.
// MEMORY_VERBS v1 has a different wire contract from local CLI output, so its
// pairs compare the shared search text/page identity and answer/citation data.
// Local-private versus remote-world audience behavior is tested separately.
// The brief pair remains a shared-engine check: no CLI brief command exists.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/sirerun/serenity/docs/protocol/schemas"
	"github.com/sirerun/serenity/internal/config"
	coredirection "github.com/sirerun/serenity/internal/direction"
	"github.com/sirerun/serenity/internal/direction/check"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/events"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/router"
	"github.com/sirerun/serenity/internal/search"
	"github.com/sirerun/serenity/internal/server"
	serverdirection "github.com/sirerun/serenity/internal/server/direction"
	serverdisposition "github.com/sirerun/serenity/internal/server/disposition"
	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/server/memory"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

var driftNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

// ---- check / check_plan: holds by construction (check.ToWire, T4.6) ----

// TestDriftCheckMatchesCheckPlan drives the real runCheck (structured
// --actions, --json) and the real DIRECTION v1 check_plan HTTP handler
// over the same fixture ledger, asserting their parsed check.WireResult
// deep-equal -- the same proof internal/server/direction's own
// TestCheckPlanWireVerdictsEqualCLIJSONOnSameFixture already gives
// (T4.6), reproduced here in-process (no built binary) as this task's
// own entry in the five-pair suite RFC 0001 section 13.1 names.
func TestDriftCheckMatchesCheckPlan(t *testing.T) {
	root := initBrainRepo(t)
	seedConstraint(t, root, "cst-0001", "spend_over", "{amount: {gte: 200}}",
		"unbounded spend risks a runaway ceiling breach", "if the monthly ceiling doubles")

	actionsJSON := `[{"action":"spend_over","params":{"amount":300}}]`

	var buf bytes.Buffer
	err := runCheck(context.Background(), root, "", actionsJSON, true, &buf)
	var exitErr *ExitError
	if err != nil && !isExitError(err, &exitErr) {
		t.Fatalf("runCheck: %v", err)
	}
	var cliWire check.WireResult
	if err := json.Unmarshal(buf.Bytes(), &cliWire); err != nil {
		t.Fatalf("unmarshal CLI --json output: %v\n%s", err, buf.String())
	}
	if cliWire.Status != "violated" {
		t.Fatalf("CLI status = %q, want %q (sanity: a vacuous pass on both sides would prove nothing)", cliWire.Status, "violated")
	}

	env := newDriftDirectionServerEnv(t, root)
	resp := postJSON(t, env, "/direction/check_plan", map[string]any{"actions": []map[string]any{
		{"action": "spend_over", "params": map[string]any{"amount": 300}},
	}})
	var httpWire check.WireResult
	if err := json.Unmarshal(resp, &httpWire); err != nil {
		t.Fatalf("unmarshal check_plan response: %v\n%s", err, resp)
	}

	if !reflect.DeepEqual(cliWire, httpWire) {
		t.Fatalf("CLI check.WireResult != check_plan's:\nCLI:  %+v\nHTTP: %+v", cliWire, httpWire)
	}
	validateAgainstProtocolSchema(t, "direction", "check_plan_response", resp)
}

func isExitError(err error, target **ExitError) bool {
	if ee, ok := err.(*ExitError); ok {
		*target = ee
		return true
	}
	return false
}

// ---- brief / brief: holds by construction (BuildBrief, T4.9) ----

// TestDriftBriefMatchesProtocolBrief drives serverdirection.Handlers.
// BuildBrief directly and the real DIRECTION v1 /direction/brief HTTP
// handler over the same fixture ledger, asserting byte-identical JSON --
// both call the identical BuildBrief, so this holds by construction, the
// same guarantee check.ToWire already gives check/check_plan.
//
// No `serenity brief` CLI verb exists yet: RFC 0001 section 13.1 names one
// in the P0 CLI surface, but building it -- flags, human rendering, its
// own acc line, and the security review T4.11's fuzz pass gave every
// other verb handler in this epic -- is real new user-facing surface with
// no acc criteria or UC coverage of its own, out of this task's scope.
// Filed as a follow-up task rather than folded in here (see docs/plan.md,
// E4). This test calls BuildBrief directly as the stand-in for what that
// future verb would wrap unchanged, once it exists.
func TestDriftBriefMatchesProtocolBrief(t *testing.T) {
	root := initBrainRepo(t)
	seedConstraint(t, root, "cst-0001", "spend_over", "", "unbounded spend risks overrun", "if the ceiling doubles")

	ledgerStore := coredirection.NewStore(root, nil)
	cliH := serverdirection.New(ledgerStore, nil, root)
	cliResp, err := cliH.BuildBrief(context.Background(), "", 800)
	if err != nil {
		t.Fatalf("BuildBrief: %v", err)
	}

	env := newDriftDirectionServerEnv(t, root)
	resp := postJSON(t, env, "/direction/brief", map[string]any{"task_hint": "", "token_budget": 800})

	var cliParsed, httpParsed map[string]any
	if err := json.Unmarshal(cliResp, &cliParsed); err != nil {
		t.Fatalf("unmarshal BuildBrief output: %v\n%s", err, cliResp)
	}
	if err := json.Unmarshal(resp, &httpParsed); err != nil {
		t.Fatalf("unmarshal /direction/brief response: %v\n%s", err, resp)
	}
	if !reflect.DeepEqual(cliParsed, httpParsed) {
		t.Fatalf("BuildBrief output != /direction/brief's:\nBuildBrief: %s\nHTTP: %s", cliResp, resp)
	}
	validateAgainstProtocolSchema(t, "direction", "brief_response", resp)
}

// ---- search / recall ----

// TestDriftSearchMatchesRecall drives the real searchResults (the exact
// function `serenity search` calls) and MEMORY_VERBS's real recall verb
// over the same synced public fixture, comparing retrieved text, page
// identity, and explicit char/4 budget against recall's actual output.
func TestDriftSearchMatchesRecall(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	var out bytes.Buffer
	if err := runInit(root, &out); err != nil {
		t.Fatal(err)
	}

	q := writer.NewQueue(nil)
	defer q.Close()
	fw := store.NewFenceWriter(root)
	p := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice-tan"})
	p.Summary = "Runs engineering at Acme, leading the quasar rollout."
	if _, _, err := writer.Fence(q, fw, p); err != nil {
		t.Fatal(err)
	}
	if err := runSync(ctx, root, &out); err != nil {
		t.Fatal(err)
	}

	const budget = 2000
	cliResults, _, err := searchResults(ctx, root, "quasar", 50)
	if err != nil {
		t.Fatalf("searchResults: %v", err)
	}
	if len(cliResults) == 0 {
		t.Fatal("searchResults returned nothing -- fixture did not index")
	}

	h := newDriftMemoryHandlers(t, root)
	tool := findTool(t, h, "recall")
	result, err := tool.Handler(ctx, mustMarshalJSON(t, map[string]any{"query": "quasar", "budget_tokens": budget}))
	if err != nil {
		t.Fatalf("recall handler: %v", err)
	}
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("recall failed: %+v", result)
	}
	if err := compareRecallSearch(cliResults, []byte(result.Content[0].Text), budget); err != nil {
		t.Fatal(err)
	}
	validateAgainstProtocolSchema(t, "memory_verbs", "recall_response", []byte(result.Content[0].Text))
}

// Mutate the actual protocol response after first proving the unmodified pair
// matches. The same comparison must reject an added field and changed text.
func TestDriftSearchMatchesRecallCatchesAOneSidedFieldAddition(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	var out bytes.Buffer
	if err := runInit(root, &out); err != nil {
		t.Fatal(err)
	}
	q := writer.NewQueue(nil)
	defer q.Close()
	fw := store.NewFenceWriter(root)
	p := store.NewEntityPage(domain.Entity{Type: "person", Slug: "bob-lee"})
	p.Summary = "Owns the nebula migration at Acme."
	if _, _, err := writer.Fence(q, fw, p); err != nil {
		t.Fatal(err)
	}
	if err := runSync(ctx, root, &out); err != nil {
		t.Fatal(err)
	}

	cliResults, _, err := searchResults(ctx, root, "nebula", 50)
	if err != nil {
		t.Fatalf("searchResults: %v", err)
	}
	if len(cliResults) == 0 {
		t.Fatal("searchResults returned nothing -- fixture did not index")
	}

	h := newDriftMemoryHandlers(t, root)
	tool := findTool(t, h, "recall")
	result, err := tool.Handler(ctx, mustMarshalJSON(t, map[string]any{"query": "nebula", "budget_tokens": 2000}))
	if err != nil {
		t.Fatalf("recall handler: %v", err)
	}
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("recall failed: %+v", result)
	}
	wire := []byte(result.Content[0].Text)
	if err := compareRecallSearch(cliResults, wire, 2000); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"added_field", "changed_chunk"} {
		t.Run(mutation, func(t *testing.T) {
			var response map[string]any
			if err := json.Unmarshal(wire, &response); err != nil {
				t.Fatal(err)
			}
			results := response["results"].([]any)
			first := results[0].(map[string]any)
			if mutation == "added_field" {
				first["unexpected_drift_field"] = true
			} else {
				first["chunk"] = "one-sided drift"
			}
			changed, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			if err := compareRecallSearch(cliResults, changed, 2000); err == nil {
				t.Fatal("actual response mutation escaped drift comparison")
			}
		})
	}

}

// ---- ask / synthesize ----

// fakeCompleter is a test double implementing compose.Completer -- no
// production code path constructs one, per the zero-stub policy.
type fakeCompleter struct{ text string }

func (f fakeCompleter) Complete(context.Context, router.TaskClass, router.Prompt, router.Budget) (router.Result, error) {
	return router.Result{Text: f.text, ModelVersion: "fake-composer@v1"}, nil
}

// Compare the real shared ask answer and its cited page identities with the
// canonical synthesize response. Usage stays unknown when the test provider
// does not report measured usage.
func TestDriftAskMatchesSynthesize(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	var out bytes.Buffer
	if err := runInit(root, &out); err != nil {
		t.Fatal(err)
	}

	q := writer.NewQueue(nil)
	defer q.Close()
	fw := store.NewFenceWriter(root)
	// acmecorp, not acme-corp: internal/index.SQLite.SearchFTS passes a
	// query string straight into FTS5's own MATCH grammar unescaped, and a
	// bare hyphenated token there (e.g. "acme-corp") is a pre-existing bug
	// in that grammar, not this task's own -- verified directly against
	// sqlite3's fts5 extension ("acme-corp" MATCH errors "no such column:
	// corp"; "acmecorp" does not). compose.Composer.relevantSubjects calls
	// search.Search unconditionally, so that error would abort Ask itself
	// before lexicalScore ever gets a chance to run -- flagged to
	// team-lead as a found-not-fixed gap, this fixture just avoids it.
	p := store.NewEntityPage(domain.Entity{Type: "org", Slug: "acmecorp"})
	claim := domain.Claim{
		ID: "c-drift-1", SubjectSlug: "acmecorp", Predicate: "works_at", Object: "acme",
		Confidence: 0.9, ValidFrom: "2026-01-01T00:00:00Z", State: domain.StateActive,
		Family: "works_at", SourceRef: "src#1",
	}
	p.Claims = []domain.Claim{claim}
	if _, _, err := writer.Fence(q, fw, p); err != nil {
		t.Fatal(err)
	}

	const query = "acmecorp works_at acme"
	completer := fakeCompleter{text: "Acme Corp is [claim:c-drift-1]."}
	cfg := config.Default()
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	defer func() { _ = eng.Close() }()

	answer, err := askAnswerFor(ctx, root, cfg, eng, nil, completer, "fake-composer@v1", query)
	if err != nil {
		t.Fatalf("askAnswerFor: %v", err)
	}
	if answer.Gap != "" {
		t.Fatalf("Gap = %q, want empty (the fixture claim should be found)", answer.Gap)
	}
	if len(answer.Citations) != 1 {
		t.Fatalf("Citations = %v, want exactly 1", answer.Citations)
	}
	wantSources := []string{answer.Citations[0].Subject}
	sort.Strings(wantSources)

	memoryQueue := writer.NewQueue(nil)
	t.Cleanup(memoryQueue.Close)
	h := memory.New(memory.Deps{
		Root: root, Config: cfg, Index: eng,
		Composer: completer, ComposerModelVersion: "fake-composer@v1",
		Queue:   memoryQueue,
		Sources: store.NewSourceStore(root), Fence: store.NewFenceWriter(root), Shard: store.NewShardStore(root),
	})
	tool := findToolFrom(t, h.Tools(), "synthesize")
	result, err := tool.Handler(ctx, mustMarshalJSON(t, map[string]any{"question": query}))
	if err != nil {
		t.Fatalf("synthesize handler: %v", err)
	}
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("synthesize failed: %+v", result)
	}
	var got struct {
		ProtocolVersion int      `json:"protocol_version"`
		Answer          string   `json:"answer"`
		Sources         []string `json:"sources"`
		Gaps            []string `json:"gaps"`
		Cost            struct {
			Model        string   `json:"model"`
			InputTokens  *int     `json:"input_tokens"`
			OutputTokens *int     `json:"output_tokens"`
			USD          *float64 `json:"usd_estimate"`
		} `json:"cost"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &got); err != nil {
		t.Fatal(err)
	}
	if got.ProtocolVersion != 1 || got.Answer != answer.Text || len(got.Gaps) != 0 {
		t.Fatalf("synthesize answer drift: %+v; shared answer: %+v", got, answer)
	}
	if !reflect.DeepEqual(got.Sources, wantSources) {
		t.Fatalf("synthesize sources %v != shared citations %v", got.Sources, wantSources)
	}
	if got.Cost.Model != "fake-composer@v1" || got.Cost.InputTokens != nil || got.Cost.OutputTokens != nil || got.Cost.USD != nil {
		t.Fatalf("synthesize fabricated or changed usage: %+v", got.Cost)
	}
	validateAgainstProtocolSchema(t, "memory_verbs", "synthesize_response", []byte(result.Content[0].Text))
}

// ---- inbox dispose / dispose ----

// TestDriftInboxDisposeMatchesDispose drives the real runInteractive
// (`inbox`'s space-to-accept path) and the real DISPOSITION v1 dispose
// HTTP handler against two identically-shaped staged items, comparing
// the resulting disposition.Item (fetched after each path, ignoring the
// two items' own distinct random IDs and the fixture's own item-specific
// setup) field for field.
func TestDriftInboxDisposeMatchesDispose(t *testing.T) {
	ctx := context.Background()
	root := initBrainRepo(t)
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	dispStore := disposition.NewStore(eng)

	item1 := seedReconcileItem(t, dispStore, ctx, driftNow, "acme-corp", "works_at", "acme", "initech", "")
	item2 := seedReconcileItem(t, dispStore, ctx, driftNow, "acme-corp", "works_at", "acme", "initech", "")
	seedInboxCanonical(t, root, item1)

	// CLI path: feed runInteractive a single space keystroke (accept the
	// row under the cursor) via a synthetic io.Reader, the same
	// scripted-TTY pattern internal/cli/inbox_test.go's own acc-line
	// tests already use. Which of item1/item2 lands under the cursor is
	// not fixed in advance -- disposition.Store.List sorts by CreatedAt
	// via sort.Slice, unstable for these two identically-shaped items'
	// tied CreatedAt -- so rather than assume an order, this looks up
	// both afterward and treats whichever one actually got disposed as
	// the CLI path's own result, leaving the other for the HTTP path.
	dirStore := newTestDirectionStore(t, root)
	sw := newTestSupersedeWriter(t, root)
	var cliOut bytes.Buffer
	if err := runInteractive(ctx, dispStore, sw, dirStore, bytes.NewBufferString(" "), &cliOut, "drift-test-actor", driftNow); err != nil {
		t.Fatalf("runInteractive: %v", err)
	}
	r1, err := dispStore.Get(ctx, item1.ID)
	if err != nil {
		t.Fatalf("Get(item1): %v", err)
	}
	r2, err := dispStore.Get(ctx, item2.ID)
	if err != nil {
		t.Fatalf("Get(item2): %v", err)
	}
	var cliResult, httpPending disposition.Item
	switch {
	case r1.State == disposition.StateDisposed && r2.State != disposition.StateDisposed:
		cliResult, httpPending = r1, r2
	case r2.State == disposition.StateDisposed && r1.State != disposition.StateDisposed:
		cliResult, httpPending = r2, r1
	default:
		t.Fatalf("want exactly one of item1/item2 disposed by runInteractive's single space keystroke, got item1=%s item2=%s", r1.State, r2.State)
	}

	// HTTP path: a DISPOSITION-only server sharing this same eng/dispStore
	// (not a second, independent index connection) so httpPending --
	// staged through dispStore above -- is visible to the HTTP dispose
	// call, with its clock fixed to the same driftNow runInteractive used
	// so both paths' UpdatedAt/DisposedAt genuinely match rather than
	// merely being cleared before comparison.
	env := newDriftDispositionServerEnv(t, eng, dispStore, driftNow)
	resp := postJSON(t, env, "/disposition/dispose", map[string]any{
		"item_id": httpPending.ID, "verdict": "accept", "idempotency_key": "drift-http-1", "actor": "drift-test-actor",
	})
	var httpWire struct {
		Results []struct {
			Item disposition.Item `json:"item"`
		} `json:"results"`
	}
	if err := json.Unmarshal(resp, &httpWire); err != nil {
		t.Fatalf("unmarshal dispose response: %v\n%s", err, resp)
	}
	if len(httpWire.Results) != 1 {
		t.Fatalf("dispose Results = %v, want exactly 1", httpWire.Results)
	}
	httpResult := httpWire.Results[0].Item

	// Compare everything except the two items' own distinct, randomly
	// generated IDs and IdempotencyKey -- runInteractive's own
	// dispStore.Dispose call always passes "" for idempotency key (the
	// interactive space-to-accept path has no such concept at all), while
	// the HTTP dispose wire request requires a non-empty one. That is a
	// genuine, disclosed asymmetry between the two entrypoints, not
	// something either side's mapping could ever make equal, so it is
	// cleared here rather than silently left to fail the comparison.
	// The local-owner CLI now also publishes canonical claims. This protocol
	// fixture deliberately registers only the decision store, so compare the
	// shared decision contract while asserting that distinct effect explicitly.
	if cliResult.AppliedClaimID == "" || httpResult.AppliedClaimID != "" {
		t.Fatalf("unexpected publication boundary: CLI=%q HTTP=%q", cliResult.AppliedClaimID, httpResult.AppliedClaimID)
	}
	cliResult.AppliedClaimID = ""
	cliResult.ID, httpResult.ID = "", ""
	cliResult.IdempotencyKey, httpResult.IdempotencyKey = "", ""
	if !reflect.DeepEqual(cliResult, httpResult) {
		t.Fatalf("CLI-disposed item != HTTP-disposed item (IDs/IdempotencyKey cleared):\nCLI:  %+v\nHTTP: %+v", cliResult, httpResult)
	}
	if cliResult.State != disposition.StateDisposed || cliResult.Verdict != disposition.VerdictAccept {
		t.Fatalf("CLI item state/verdict = %s/%s, want disposed/accept", cliResult.State, cliResult.Verdict)
	}
	validateAgainstProtocolSchema(t, "disposition", "dispose_response", resp)
}

// ---- shared fixture/HTTP helpers ----

// driftServerEnv bundles a real *server.Server with a bearer token to
// call it with -- the same real-HTTP-listener convention T4.3/T4.4/T4.6's
// own tests already establish ("hitting a real listener over real HTTP,
// not internal calls").
type driftServerEnv struct {
	baseURL string
	token   string
}

const driftBearerToken = "drift-test-token"

// newDriftDirectionServerEnv starts a real *server.Server with DIRECTION
// v1 (T4.6) registered over root's ledger, for the check_plan and brief
// pairs -- both operations are filesystem-based (.dira/ entries, fence
// entity pages), so no index/eng is needed here at all.
func newDriftDirectionServerEnv(t *testing.T, root string) *driftServerEnv {
	t.Helper()
	q := writer.NewQueue(nil)
	t.Cleanup(q.Close)
	ledgerStore := coredirection.NewStore(root, q)
	dirH := serverdirection.New(ledgerStore, nil, root)

	s := server.New(server.Config{Bind: "127.0.0.1:0", TokenSource: func() (string, error) { return driftBearerToken, nil }})
	dirH.Register(s)
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
	return &driftServerEnv{baseURL: "http://" + s.Addr(), token: driftBearerToken}
}

// driftClock is a fixed-time events/disposition-server Clock (both
// packages share the same `Now() time.Time` seam) -- injected here so the
// HTTP dispose path's own UpdatedAt/DisposedAt land on the identical
// timestamp runInteractive's own explicit `now` argument gives the CLI
// path, making the two paths' resulting items genuinely comparable rather
// than requiring those fields to be cleared before comparison.
type driftClock struct{ now time.Time }

func (c driftClock) Now() time.Time { return c.now }

// newDriftDispositionServerEnv starts a real *server.Server with
// DISPOSITION v1 (T4.4) registered over the CALLER'S OWN eng/dispStore --
// not a fresh index.Open, which would give the HTTP dispose handler a
// second, independent SQLite connection to a different backing store than
// the one the test's own CLI-path items were staged/disposed through.
// Sharing eng (the same pattern internal/server/disposition's own
// newTestEnv builds dispStore/evStore over) is what makes httpPending
// visible to /disposition/dispose at all.
func newDriftDispositionServerEnv(t *testing.T, eng *index.SQLite, dispStore *disposition.Store, now time.Time) *driftServerEnv {
	t.Helper()
	evStore := events.NewStore(eng)
	h := serverdisposition.New(dispStore, evStore, serverdisposition.WithClock(driftClock{now}))

	s := server.New(server.Config{Bind: "127.0.0.1:0", TokenSource: func() (string, error) { return driftBearerToken, nil }})
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
	return &driftServerEnv{baseURL: "http://" + s.Addr(), token: driftBearerToken}
}

func postJSON(t *testing.T, env *driftServerEnv, path string, body any) []byte {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, env.baseURL+path, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+env.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out bytes.Buffer
	if _, err := out.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s: status %d: %s", path, resp.StatusCode, out.String())
	}
	return out.Bytes()
}

func newDriftMemoryHandlers(t *testing.T, root string) *memory.Handlers {
	t.Helper()
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	memoryQueue := writer.NewQueue(nil)
	t.Cleanup(memoryQueue.Close)
	return memory.New(memory.Deps{
		Root: root, Config: config.Default(), Index: eng,
		Queue:   memoryQueue,
		Sources: store.NewSourceStore(root), Fence: store.NewFenceWriter(root), Shard: store.NewShardStore(root),
	})
}

func findTool(t *testing.T, h *memory.Handlers, name string) mcp.Tool {
	t.Helper()
	return findToolFrom(t, h.Tools(), name)
}

func findToolFrom(t *testing.T, tools []mcp.Tool, name string) mcp.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("no tool named %q registered", name)
	return mcp.Tool{}
}

// validateAgainstProtocolSchema is T4.7's own reuse of these already-real
// wire responses: rather than spin up separate fixtures, every drift pair
// above that produces a genuine live protocol response (over MCP or real
// HTTP) also gets validated against its own docs/protocol/schemas/*.json
// entry here, with the real draft-2020-12 validator
// (santhosh-tekuri/jsonschema/v6) -- broadening "every CLI --json output
// validates against its schema in CI" (T4.7's acc line) to every wire
// object this test file already exercises end-to-end, not only
// `serenity check --json`'s own dedicated test.
func validateAgainstProtocolSchema(t *testing.T, protocol, object string, data []byte) {
	t.Helper()
	e, ok := schemas.Lookup(protocol, object)
	if !ok {
		t.Fatalf("no schema registered for %s/%s", protocol, object)
	}
	if err := schemas.Validate(e, data); err != nil {
		t.Fatalf("%s/%s: live response violates its own schema: %v\nresponse: %s", protocol, object, err, data)
	}
}

func mustMarshalJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// compareRecallSearch compares the real shared retrieval payload; the fixture
// contains public entity-page evidence and no raw remembered facts. Scores are
// local ranking details absent from the adopted wire contract.
func compareRecallSearch(cli []search.Result, wire []byte, budget int) error {
	var got struct {
		Version      int               `json:"protocol_version"`
		Facts        []json.RawMessage `json:"facts"`
		Total        int               `json:"total"`
		Results      []map[string]any  `json:"results"`
		BudgetTokens *int              `json:"budget_tokens"`
		BudgetUsed   *int              `json:"budget_used"`
		Dropped      *int              `json:"dropped_count"`
		Degraded     string            `json:"search_degraded"`
	}
	if err := json.Unmarshal(wire, &got); err != nil {
		return err
	}
	if got.Version != 1 || got.Facts == nil || len(got.Facts) != 0 || got.Total != 0 || got.Degraded == "" {
		return fmt.Errorf("recall envelope drift: %s", wire)
	}
	if len(got.Results) != len(cli) {
		return fmt.Errorf("recall returned %d search results; CLI returned %d", len(got.Results), len(cli))
	}
	used := 0
	allowed := map[string]bool{"slug": true, "title": true, "chunk": true, "evidence": true, "create_safety": true, "provenance": true}
	for i, hit := range cli {
		r := got.Results[i]
		for key := range r {
			if !allowed[key] {
				return fmt.Errorf("unexpected recall result field %q", key)
			}
		}
		if r["slug"] != hit.EntitySlug || r["provenance"] != hit.EntitySlug || r["chunk"] != hit.Text {
			return fmt.Errorf("recall result %d diverges: %v; CLI: %+v", i, r, hit)
		}
		if r["evidence"] != "keyword_exact" || r["create_safety"] != "exists" {
			return fmt.Errorf("recall public page classification drift: %v", r)
		}
		used += (utf8.RuneCountInString(hit.Text) + 3) / 4
	}
	if got.BudgetTokens == nil || *got.BudgetTokens != budget || got.BudgetUsed == nil || *got.BudgetUsed != used || got.Dropped == nil || *got.Dropped != 0 {
		return fmt.Errorf("recall budget drift: %s", wire)
	}
	return nil
}
