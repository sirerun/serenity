package conformance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirerun/serenity/internal/server/mcp"
)

func TestReplayTranscriptPassesOnDynamicFieldsAndRealMismatchFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/disposition/dispose", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer s3cr3t" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		switch body["idempotency_key"] {
		case "ok-key":
			// A different (freshly-random) id and timestamp than the
			// fixture recorded -- must still pass via normalization.
			_, _ = fmt.Fprint(w, `{"results":[{"item":{"id":"00112233445566778899aabbccddeeff","verdict":"accept","disposed_at":"2027-03-04T05:06:07Z"}}]}`)
		case "wrong-key":
			// A genuine behavioral difference (verdict), must fail.
			_, _ = fmt.Fprint(w, `{"results":[{"item":{"id":"00112233445566778899aabbccddeeff","verdict":"reject","disposed_at":"2027-03-04T05:06:07Z"}}]}`)
		}
	})
	mux.HandleFunc("/disposition/subscribe", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tr := Transcript{
		Protocol:  "disposition",
		Operation: "dispose",
		Cases: []Case{
			{
				Name: "accept passes under id/timestamp normalization",
				Steps: []Step{{
					Method:      "POST",
					Path:        "/disposition/dispose",
					RequestBody: json.RawMessage(`{"item_id":"cda08aff5a5749c7f6ad9b50d90554ac","verdict":"accept","idempotency_key":"ok-key"}`),
					Response: StepResponse{
						Status: 200,
						Body:   `{"results":[{"item":{"id":"cda08aff5a5749c7f6ad9b50d90554ac","verdict":"accept","disposed_at":"2026-09-07T12:00:00Z"}}]}`,
					},
				}},
			},
			{
				Name: "a genuine verdict divergence fails",
				Steps: []Step{{
					Method:      "POST",
					Path:        "/disposition/dispose",
					RequestBody: json.RawMessage(`{"item_id":"cda08aff5a5749c7f6ad9b50d90554ac","verdict":"accept","idempotency_key":"wrong-key"}`),
					Response: StepResponse{
						Status: 200,
						Body:   `{"results":[{"item":{"id":"cda08aff5a5749c7f6ad9b50d90554ac","verdict":"accept","disposed_at":"2026-09-07T12:00:00Z"}}]}`,
					},
				}},
			},
			{
				Name: "no_auth step really is sent without a bearer token",
				Steps: []Step{{
					Method: "GET",
					Path:   "/disposition/subscribe",
					NoAuth: true,
					Response: StepResponse{
						Status: 401,
						Body:   "unauthorized\n",
					},
				}},
			},
		},
	}

	out := ReplayTranscript(context.Background(), srv.Client(), srv.URL, "s3cr3t", tr)
	if len(out.Cases) != 3 {
		t.Fatalf("got %d case outcomes, want 3", len(out.Cases))
	}
	if !out.Cases[0].Passed() {
		t.Errorf("case 0 (dynamic fields) should pass, steps: %+v", out.Cases[0].Steps)
	}
	if out.Cases[1].Passed() {
		t.Errorf("case 1 (real verdict mismatch) should fail")
	}
	if len(out.Cases[1].Steps[0].Mismatches) == 0 {
		t.Errorf("case 1 should report at least one mismatch")
	}
	if !out.Cases[2].Passed() {
		t.Errorf("case 2 (no_auth 401) should pass, steps: %+v", out.Cases[2].Steps)
	}
}

func TestReplayTranscriptReportsStatusMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"not_found","message":"no such item"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	tr := Transcript{
		Protocol: "disposition", Operation: "dispose",
		Cases: []Case{{
			Name: "expects 200 but the target 404s",
			Steps: []Step{{
				Method: "POST", Path: "/disposition/dispose",
				Response: StepResponse{Status: 200, Body: `{"results":[]}`},
			}},
		}},
	}
	out := ReplayTranscript(context.Background(), srv.Client(), srv.URL, "tok", tr)
	if out.Cases[0].Passed() {
		t.Fatal("expected a status-code mismatch to fail the case")
	}
	if out.Cases[0].Steps[0].StatusActual != 404 {
		t.Fatalf("StatusActual = %d, want 404", out.Cases[0].Steps[0].StatusActual)
	}
}

func TestReplayTranscriptReportsTransportError(t *testing.T) {
	tr := Transcript{
		Protocol: "disposition", Operation: "dispose",
		Cases: []Case{{
			Name:  "unreachable target",
			Steps: []Step{{Method: "POST", Path: "/disposition/dispose", Response: StepResponse{Status: 200, Body: `{}`}}},
		}},
	}
	out := ReplayTranscript(context.Background(), http.DefaultClient, "http://127.0.0.1:1", "tok", tr)
	if out.Cases[0].Passed() {
		t.Fatal("expected an unreachable target to fail the case")
	}
	if out.Cases[0].Steps[0].Err == nil {
		t.Fatal("expected a transport error to be recorded")
	}
}

// --- memory_verbs replay: a real round trip against a real
// mcp.HTTPHandler, the same production Streamable HTTP transport
// serenity serve --http uses, over two hand-registered tools standing in
// for MEMORY_VERBS' own remember/recall so this test exercises the wire
// path (initialize, session header lifecycle, tools/call) rather than a
// hand-rolled fake.

func newTestMemoryServer(t *testing.T) *httptest.Server {
	t.Helper()
	rememberSchema := json.RawMessage(`{"type":"object","properties":{"fact":{"type":"string"}},"required":["fact"]}`)
	recallSchema := json.RawMessage(`{"type":"object","properties":{"entity":{"type":"string"}},"required":["entity"]}`)

	remember := mcp.Tool{
		Name:        "remember",
		InputSchema: rememberSchema,
		Handler: func(_ context.Context, args json.RawMessage) (mcp.Result, error) {
			var p struct {
				Fact string `json:"fact"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return mcp.Result{}, err
			}
			body, _ := json.Marshal(map[string]any{"protocol_version": 1, "id": "fact-001", "status": "inserted", "fact": p.Fact})
			return mcp.Result{Content: []mcp.Content{{Type: "text", Text: string(body)}}}, nil
		},
	}
	recall := mcp.Tool{
		Name:        "recall",
		InputSchema: recallSchema,
		Handler: func(_ context.Context, args json.RawMessage) (mcp.Result, error) {
			var p struct {
				Entity string `json:"entity"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return mcp.Result{}, err
			}
			if p.Entity == "" {
				body, _ := json.Marshal(map[string]any{"protocol_version": 1, "error": "invalid_params", "suggestion": "pass an entity"})
				return mcp.Result{Content: []mcp.Content{{Type: "text", Text: string(body)}}, IsError: true}, nil
			}
			body, _ := json.Marshal(map[string]any{"protocol_version": 1, "total": 1, "facts": []map[string]any{{"fact_id": "fact-001", "entity": p.Entity}}})
			return mcp.Result{Content: []mcp.Content{{Type: "text", Text: string(body)}}}, nil
		},
	}
	srv, err := mcp.New("conformance-test", []mcp.Tool{remember, recall})
	if err != nil {
		t.Fatalf("mcp.New: %v", err)
	}
	handler := mcp.NewHTTPHandler(srv)
	t.Cleanup(handler.Close)
	return httptest.NewServer(handler)
}

func TestReplayMemoryVerbCasesRoundTripsOverHTTPWithSaveAsChaining(t *testing.T) {
	ts := newTestMemoryServer(t)
	defer ts.Close()

	client := NewMCPClient(ts.Client(), ts.URL, "")
	if err := client.Initialize(context.Background(), "conformance-test-client", "1"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	cases := []MemoryVerbCase{
		{
			Name:   "remember a fact",
			Verb:   "remember",
			Params: map[string]any{"fact": "conformance {{marker}} fact"},
			Expect: []map[string]any{{"path": "status", "equals": "inserted"}},
			SaveAs: struct {
				Key  string `json:"key"`
				Path string `json:"path"`
			}{Key: "fact1", Path: "id"},
		},
		{
			Name:   "recall by entity uses the saved id as the entity slug",
			Verb:   "recall",
			Params: map[string]any{"entity": "people/{{id:fact1}}"},
			Expect: []map[string]any{
				{"path": "facts.0.entity", "equals": "people/fact-001"},
				{"path": "total", "gte": float64(1)},
			},
		},
		{
			Name:            "recall with no entity returns invalid_params",
			Verb:            "recall",
			Params:          map[string]any{"entity": ""},
			ExpectErrorCode: "invalid_params",
		},
	}

	outcomes := ReplayMemoryVerbCases(context.Background(), client, cases, "run-marker")
	if len(outcomes) != 3 {
		t.Fatalf("got %d outcomes, want 3", len(outcomes))
	}
	for _, o := range outcomes {
		if !o.Passed() {
			t.Errorf("case %q failed: err=%v fails=%v", o.Name, o.Err, o.Fails)
		}
	}
}

func TestReplayMemoryVerbCasesReportsAssertionFailureWithoutAborting(t *testing.T) {
	ts := newTestMemoryServer(t)
	defer ts.Close()

	client := NewMCPClient(ts.Client(), ts.URL, "")
	if err := client.Initialize(context.Background(), "conformance-test-client", "1"); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	cases := []MemoryVerbCase{
		{Name: "wrong expectation", Verb: "remember", Params: map[string]any{"fact": "x"}, Expect: []map[string]any{{"path": "status", "equals": "duplicate"}}},
		{Name: "still runs after a prior failure", Verb: "remember", Params: map[string]any{"fact": "y"}, Expect: []map[string]any{{"path": "status", "equals": "inserted"}}},
	}
	outcomes := ReplayMemoryVerbCases(context.Background(), client, cases, "run-marker")
	if outcomes[0].Passed() {
		t.Fatal("expected the first case's wrong expectation to fail")
	}
	if !outcomes[1].Passed() {
		t.Fatalf("expected the second case to still run and pass: %+v", outcomes[1])
	}
}
