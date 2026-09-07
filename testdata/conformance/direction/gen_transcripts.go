//go:build ignore

// Command gen_transcripts captures testdata/conformance/direction/'s
// three transcript files (T4.13) by running real HTTP round trips
// against a real *internal/server.Server wrapping real
// internal/server/direction.Handlers over a real internal/direction.Store
// (ledger, over a temp brain root) and internal/disposition.Store
// (index.Open-backed) -- the identical real-listener harness
// internal/server/direction/direction_test.go itself uses (T4.6), just
// driven from a standalone command instead of *testing.T so its captures
// can be written to disk and frozen.
//
// These are recordings, not hand-authored fixtures: every request_body
// and response in the generated JSON is exactly what a live handler sent
// and received. Re-run after a DELIBERATE change to DIRECTION v1's wire
// shapes or this generator's own scenarios, then re-pin the manifest:
//
//	GOWORK=off go run testdata/conformance/direction/gen_transcripts.go
//
// (GOWORK=off per docs/lore.md L-0001 -- unnecessary from an offloaded
// worktree, required from a plain sirerun/serenity checkout.)
//
// This intentionally overwrites the checksum-frozen fixtures in place --
// review the diff before committing, the same discipline
// evals/corpora/*/gen_manifest.go and gen_corpus.go already establish for
// their own checksum-pinned corpora.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/sirerun/serenity/internal/conformance"
	"github.com/sirerun/serenity/internal/dira/ledger"
	coredirection "github.com/sirerun/serenity/internal/direction"
	coredisp "github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/secrets"
	"github.com/sirerun/serenity/internal/server"
	serverdirection "github.com/sirerun/serenity/internal/server/direction"
	"github.com/sirerun/serenity/internal/writer"
)

const outDir = "testdata/conformance/direction"

var fixedNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

// env is one fresh, isolated ledger + disposition store + server, one per
// Case -- the same isolation newTestEnv gives each *testing.T in the real
// test suite.
type env struct {
	root      string
	ledger    *coredirection.Store
	dispStore *coredisp.Store
	base      string
	token     string
	client    *http.Client
	cancel    context.CancelFunc
	done      chan struct{}
}

func newEnv(opts ...serverdirection.Option) *env {
	root, err := os.MkdirTemp("", "gen-direction-*")
	if err != nil {
		log.Fatalf("MkdirTemp: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".dira", "entries"), 0o755); err != nil {
		log.Fatalf("MkdirAll .dira/entries: %v", err)
	}
	q := writer.NewQueue(nil)
	ledgerStore := coredirection.NewStore(root, q)

	eng, err := index.Open(filepath.Join(root, "index.db"))
	if err != nil {
		log.Fatalf("index.Open: %v", err)
	}
	dispStore := coredisp.NewStore(eng)

	allOpts := append([]serverdirection.Option{serverdirection.WithClock(fakeClock{fixedNow})}, opts...)
	h := serverdirection.New(ledgerStore, dispStore, root, allOpts...)

	const token = "s3cr3t-token"
	s := server.New(server.Config{Bind: "127.0.0.1:0", TokenSource: func() (string, error) { return token, nil }})
	h.Register(s)
	if err := s.Listen(); err != nil {
		log.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = s.Serve(ctx)
		close(done)
	}()
	return &env{
		root: root, ledger: ledgerStore, dispStore: dispStore,
		base: "http://" + s.Addr(), token: token,
		client: &http.Client{Transport: &http.Transport{DisableKeepAlives: true}},
		cancel: cancel, done: done,
	}
}

func (e *env) close() {
	e.cancel()
	select {
	case <-e.done:
	case <-time.After(8 * time.Second):
		log.Fatal("server did not shut down within 8s")
	}
	_ = os.RemoveAll(e.root)
}

func (e *env) step(method, path string, body any, noAuth bool) conformance.Step {
	var reqBytes json.RawMessage
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			log.Fatalf("marshal request body: %v", err)
		}
		reqBytes = b
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.base+path, reader)
	if err != nil {
		log.Fatalf("NewRequest: %v", err)
	}
	if !noAuth {
		req.Header.Set("Authorization", "Bearer "+e.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.client.Do(req)
	if err != nil {
		log.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("read response body: %v", err)
	}
	return conformance.Step{
		Method: method, Path: path, RequestBody: reqBytes, NoAuth: noAuth,
		Response: conformance.StepResponse{Status: resp.StatusCode, Body: string(respBody)},
	}
}

// --- ledger fixture helpers, copied from
// internal/server/direction/direction_test.go's own identically-named
// helpers (t.Fatalf swapped for log.Fatalf; this is a standalone
// command, not a *testing.T body). ---

func seedConstraint(s *coredirection.Store, id, title, action, paramsYAML, whyNot, revisitIf string) {
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
		log.Fatalf("seedConstraint %s: %v", id, err)
	}
}

func seedQuestion(s *coredirection.Store, id, title string) {
	entry := &ledger.Entry{
		ID: id, Kind: ledger.KindQuestion, Title: title, State: ledger.StateOpen,
		Created: "2026-08-28T00:00:00Z",
	}
	if err := s.Create(context.Background(), entry); err != nil {
		log.Fatalf("seedQuestion %s: %v", id, err)
	}
}

func writeTranscript(operation string, cases ...conformance.Case) {
	tr := conformance.Transcript{Protocol: "direction", Operation: operation, Cases: cases}
	b, err := json.MarshalIndent(tr, "", "  ")
	if err != nil {
		log.Fatalf("marshal %s transcript: %v", operation, err)
	}
	path := filepath.Join(outDir, operation+".json")
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		log.Fatalf("write %s: %v", path, err)
	}
	fmt.Printf("wrote %s (%d cases)\n", path, len(cases))
}

func main() {
	secrets.MockForTesting() // never touch the real OS keychain from a generator run
	genBrief()
	genCheckPlan()
	genPropose()

	if err := conformance.WriteManifest(outDir); err != nil {
		log.Fatalf("WriteManifest: %v", err)
	}
	fmt.Printf("wrote %s\n", filepath.Join(outDir, conformance.ManifestFile))
}

// preceptCap mirrors internal/server/direction.preceptCap (unexported;
// duplicated here as a plain literal, the same way this generator
// duplicates the test file's fixture helpers rather than reaching into
// its unexported symbols across the package boundary).
const preceptCap = 12

func genBrief() {
	var cases []conformance.Case

	func() {
		e := newEnv()
		defer e.close()
		s := e.step("POST", "/direction/brief", serverdirection.BriefRequest{TokenBudget: 800}, false)
		cases = append(cases, conformance.Case{
			Name:  "brief on a fresh ledger returns all four fixed sections, empty",
			Steps: []conformance.Step{s},
		})
	}()

	func() {
		e := newEnv()
		defer e.close()
		seedConstraint(e.ledger, "cst-0001", "fixture spend ceiling", "spend_over", "{amount: {gte: 200}}",
			"unbounded spend risk", "quarterly review")
		seedQuestion(e.ledger, "qst-0001", "should we raise the ceiling?")
		s := e.step("POST", "/direction/brief", serverdirection.BriefRequest{TokenBudget: 0}, false)
		cases = append(cases, conformance.Case{
			Name:  "a zero token_budget returns the minimal valid object: four sections, real candidates all omitted",
			Steps: []conformance.Step{s},
		})
	}()

	func() {
		e := newEnv()
		defer e.close()
		longWord := "supercalifragilisticexpialidocious "
		var longWhyNot bytes.Buffer
		for i := 0; i < 70; i++ {
			longWhyNot.WriteString(longWord)
		}
		for i := 1; i <= preceptCap; i++ {
			id := fmt.Sprintf("cst-%04d", i)
			seedConstraint(e.ledger, id, fmt.Sprintf("fixture precept number %d", i), "spend_over", "{amount: {gte: 200}}",
				longWhyNot.String(), "quarterly review")
		}
		seedQuestion(e.ledger, "qst-0001", "should we raise the ceiling?")
		seedQuestion(e.ledger, "qst-0002", "who owns this precept?")
		s := e.step("POST", "/direction/brief", serverdirection.BriefRequest{TokenBudget: 800}, false)
		cases = append(cases, conformance.Case{
			Name:  "budget 800 fills sections by priority and drops an overflowing precepts section whole, never starving questions",
			Steps: []conformance.Step{s},
		})
	}()

	writeTranscript("brief", cases...)
}

func genCheckPlan() {
	var cases []conformance.Case

	func() {
		e := newEnv()
		defer e.close()
		s := e.step("POST", "/direction/check_plan", map[string]any{}, false)
		cases = append(cases, conformance.Case{
			Name:  "check_plan with neither plan_text nor actions returns invalid_request",
			Steps: []conformance.Step{s},
		})
	}()

	func() {
		e := newEnv()
		defer e.close()
		seedConstraint(e.ledger, "cst-0001", "fixture spend ceiling", "spend_over", "{amount: {gte: 200}}",
			`Unbounded spend risk: "no ceiling" was rejected outright.`, "quarterly budget review")
		s := e.step("POST", "/direction/check_plan", map[string]any{
			"actions": []map[string]any{{"action": "spend_over", "params": map[string]any{"amount": 500}}},
		}, false)
		cases = append(cases, conformance.Case{
			Name:  "check_plan with a structured action matching an active constraint returns status violated",
			Steps: []conformance.Step{s},
		})
	}()

	func() {
		e := newEnv()
		defer e.close()
		// "start_project" is a real member of domain.ActionSet (the
		// closed action vocabulary), so this exercises "considered but
		// matched nothing" rather than ErrUnknownAction's own 400 --
		// the fresh ledger below seeds zero constraints on purpose.
		s := e.step("POST", "/direction/check_plan", map[string]any{
			"actions": []map[string]any{{"action": "start_project"}},
		}, false)
		cases = append(cases, conformance.Case{
			Name:  "check_plan matching zero active constraints returns no_applicable_constraints, never a bare pass",
			Steps: []conformance.Step{s},
		})
	}()

	func() {
		e := newEnv()
		defer e.close()
		s := e.step("POST", "/direction/check_plan", map[string]any{"plan_text": "deploy to prod now"}, false)
		cases = append(cases, conformance.Case{
			Name:  "check_plan free text with no classifier router configured returns status unverified, never a silent pass",
			Steps: []conformance.Step{s},
		})
	}()

	writeTranscript("check_plan", cases...)
}

func genPropose() {
	var cases []conformance.Case

	func() {
		e := newEnv()
		defer e.close()
		s := e.step("POST", "/direction/propose", map[string]any{"kind": "reconcile", "payload": json.RawMessage(`{}`)}, false)
		cases = append(cases, conformance.Case{
			Name:  "propose with an unsupported kind returns invalid_kind",
			Steps: []conformance.Step{s},
		})
	}()

	func() {
		e := newEnv()
		defer e.close()
		payload, err := json.Marshal(coredirection.PreceptDraftPayload{
			Title: "cap monthly spend at $50", Body: "because runaway spend",
		})
		if err != nil {
			log.Fatalf("marshal payload: %v", err)
		}
		s := e.step("POST", "/direction/propose", map[string]any{
			"kind": "precept_draft", "payload": json.RawMessage(payload),
		}, false)
		cases = append(cases, conformance.Case{
			Name:  "propose precept_draft with no alternatives is rejected before anything is staged",
			Steps: []conformance.Step{s},
		})
	}()

	func() {
		e := newEnv()
		defer e.close()
		// A pre-existing precept, so .dira/ is non-empty -- the property
		// this case documents (propose never writes to the ledger; see
		// internal/server/direction/direction_test.go's own
		// TestProposePreceptDraftCreatesItemAndDiraHashUnchanged, which
		// proves it by hashing .dira/ before and after) is only
		// meaningful against a non-trivial ledger.
		seedConstraint(e.ledger, "cst-0001", "fixture spend ceiling", "spend_over", "{amount: {gte: 200}}",
			"unbounded spend risk", "quarterly review")
		payload, err := json.Marshal(coredirection.PreceptDraftPayload{
			QuestionID: "qst-0001",
			Question:   "should we cap monthly spend?",
			Answer:     "yes, at $50",
			Title:      "cap monthly spend at $50",
			Body:       "because runaway spend",
			Alternatives: []domain.RejectedAlternative{
				{Option: "no ceiling", WhyNot: "unbounded risk", RevisitIf: "quarterly review"},
			},
		})
		if err != nil {
			log.Fatalf("marshal payload: %v", err)
		}
		s := e.step("POST", "/direction/propose", map[string]any{
			"kind": "precept_draft", "payload": json.RawMessage(payload),
		}, false)
		cases = append(cases, conformance.Case{
			Name:  "propose precept_draft with a title and one alternative creates a pending disposition item (.dira/ is never touched by propose -- see internal/server/direction's own doc comment and test)",
			Steps: []conformance.Step{s},
		})
	}()

	func() {
		e := newEnv()
		defer e.close()
		const payload = `{"row":{"id":"s1","task_class":"judgment","tier":"judgment","provider":"anthropic","model_version":"claude-x@v1","input_tokens":500,"output_tokens":200,"cost_usd":1.5,"occurred_at":"2026-09-07T11:00:00Z"},"month_to_date_usd":40,"ceiling_usd":50}`
		s := e.step("POST", "/direction/propose", map[string]any{
			"kind": "effect", "payload": json.RawMessage(payload),
		}, false)
		cases = append(cases, conformance.Case{
			Name:  "propose effect creates a pending disposition item",
			Steps: []conformance.Step{s},
		})
	}()

	func() {
		e := newEnv()
		defer e.close()
		s := e.step("POST", "/direction/propose", map[string]any{"kind": "effect"}, false)
		cases = append(cases, conformance.Case{
			Name:  "propose with no payload returns invalid_payload",
			Steps: []conformance.Step{s},
		})
	}()

	writeTranscript("propose", cases...)
}
