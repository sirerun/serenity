package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/server/mcp"
)

func TestProtocolConformanceRequiresTarget(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"protocol", "conformance"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--target") {
		t.Fatalf("expected a --target-required error, got %v", err)
	}
}

func TestProtocolConformanceRejectsUnknownProtocol(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"protocol", "conformance", "--target", "http://127.0.0.1:1", "--protocol", "bogus"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unknown --protocol") {
		t.Fatalf("expected an unknown-protocol error, got %v", err)
	}
}

// newConformanceTestServer wires a memory verbs MCP endpoint at /mcp
// (mirroring `serenity serve --http`, T4.21) alongside a minimal
// disposition-shaped stub at /disposition/dispose, so one server exercises
// both this command's HTTP-transcript path and its MCP path.
func newConformanceTestServer(t *testing.T, disposeMessage string) *httptest.Server {
	t.Helper()
	rememberTool := mcp.Tool{
		Name:        "remember",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"fact":{"type":"string"}},"required":["fact"]}`),
		Handler: func(_ context.Context, args json.RawMessage) (mcp.Result, error) {
			var p struct {
				Fact string `json:"fact"`
			}
			if err := json.Unmarshal(args, &p); err != nil {
				return mcp.Result{}, err
			}
			body, _ := json.Marshal(map[string]any{"protocol_version": 1, "id": "fact-001", "status": "inserted"})
			return mcp.Result{Content: []mcp.Content{{Type: "text", Text: string(body)}}}, nil
		},
	}
	mcpServer, err := mcp.New("conformance-cli-test", []mcp.Tool{rememberTool})
	if err != nil {
		t.Fatalf("mcp.New: %v", err)
	}
	handler := mcp.NewHTTPHandler(mcpServer)
	t.Cleanup(handler.Close)

	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)
	// "dispose by an unknown group_id returns not_found" is deliberately NOT
	// one of the cases httpTranscriptCasesNeedingSeededState marks skip: it
	// needs no pre-existing item/group, so it's fully reproducible against
	// any target and stays a genuine pass/fail signal in these tests.
	mux.HandleFunc("/disposition/dispose", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, `{"error":"not_found","message":%q}`, disposeMessage)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestProtocolConformanceEndToEndAgainstALiveServer(t *testing.T) {
	srv := newConformanceTestServer(t, "no items found for group_id no-such-group")

	fixtures := t.TempDir()
	writeConformanceFixtures(t, fixtures)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"protocol", "conformance",
		"--target", srv.URL,
		"--fixtures", fixtures,
		"--protocol", "memory_verbs", "--protocol", "disposition",
		"--json",
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\noutput:\n%s", err, out.String())
	}

	var report conformanceReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal report: %v\n%s", err, out.String())
	}
	if !report.Passed {
		t.Fatalf("expected the whole run to pass, got %+v", report)
	}
	if report.Total != 2 {
		t.Fatalf("expected 2 total cases (1 memory_verbs + 1 disposition), got %d", report.Total)
	}
	if report.Skipped != 0 {
		t.Fatalf("expected no skips when every case matches, got %+v", report)
	}
}

func TestProtocolConformanceReportsFailureAndNonzeroExit(t *testing.T) {
	// A case that needs no pre-existing item/group (so it isn't one of
	// httpTranscriptCasesNeedingSeededState's skip cases) still fails loudly
	// on a genuine mismatch.
	srv := newConformanceTestServer(t, "no items found for group_id some-other-group") // diverges from the fixture's recorded message

	fixtures := t.TempDir()
	writeConformanceFixtures(t, fixtures)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"protocol", "conformance", "--target", srv.URL, "--fixtures", fixtures, "--protocol", "disposition"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected a body mismatch to fail the command")
	}
	if !strings.Contains(out.String(), "FAIL") {
		t.Fatalf("expected the text report to show a FAIL line, got:\n%s", out.String())
	}
}

// TestProtocolConformanceSkipsCasesNeedingSeededState covers the ruling this
// command implements for list_pending/dispose (and direction's brief/
// check_plan): a case named in httpTranscriptCasesNeedingSeededState that
// mismatches against --target is reported skip, not fail, and does not fail
// the command -- this command cannot seed the item ids T4.13's own
// fixture-generator run minted, so a mismatch there means "not seeded to
// match," not "the target regressed."
func TestProtocolConformanceSkipsCasesNeedingSeededState(t *testing.T) {
	fixtures := t.TempDir()
	mustMkdir(t, fixtures+"/disposition")
	mustWriteFile(t, fixtures+"/disposition/dispose.json", `{
		"protocol": "disposition", "operation": "dispose",
		"cases": [{
			"name": "dispose accept records the verdict",
			"steps": [{
				"method": "POST", "path": "/disposition/dispose",
				"request_body": {"item_id": "cda08aff5a5749c7f6ad9b50d90554ac", "verdict": "accept", "idempotency_key": "k1"},
				"response": {"status": 200, "body": "{\"results\":[{\"item\":{\"id\":\"cda08aff5a5749c7f6ad9b50d90554ac\",\"verdict\":\"accept\"}}]}"}
			}]
		}]
	}`)

	mux := http.NewServeMux()
	mux.HandleFunc("/disposition/dispose", func(w http.ResponseWriter, r *http.Request) {
		// A real, unseeded target: the item this fixture references was
		// never created here, so a real disposition server legitimately
		// returns not_found.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"error":"not_found","message":"disposition: item not found"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"protocol", "conformance", "--target", srv.URL, "--fixtures", fixtures, "--protocol", "disposition", "--json"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected a skip-eligible mismatch not to fail the command, got: %v\noutput:\n%s", err, out.String())
	}

	var report conformanceReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal report: %v\n%s", err, out.String())
	}
	if !report.Passed {
		t.Fatalf("expected the run to pass overall (skip is not a failure), got %+v", report)
	}
	if report.Skipped != 1 || report.Failed != 0 {
		t.Fatalf("expected exactly 1 skip and 0 failures, got %+v", report)
	}
	if len(report.Protocols) != 1 || len(report.Protocols[0].Cases) != 1 {
		t.Fatalf("expected exactly 1 case, got %+v", report)
	}
	c := report.Protocols[0].Cases[0]
	if c.Status != caseStatusSkip {
		t.Fatalf("expected status skip, got %q (detail: %s)", c.Status, c.Detail)
	}
	if !strings.Contains(c.Detail, "go test ./internal/conformance") {
		t.Fatalf("expected the skip detail to point at the byte-exact authority, got: %s", c.Detail)
	}
}

// writeConformanceFixtures writes a minimal one-case memory_verbs/
// cases.json and disposition/dispose.json into dir, standing in for the
// real testdata/conformance/ set so these tests don't depend on --target
// matching the full frozen fixture corpus's own seeded state. The
// disposition case ("dispose by an unknown group_id returns not_found")
// is deliberately NOT one of httpTranscriptCasesNeedingSeededState's skip
// cases -- it needs no pre-existing item/group, so it stays a genuine
// pass/fail signal for these tests.
func writeConformanceFixtures(t *testing.T, dir string) {
	t.Helper()
	mustMkdir(t, dir+"/memory_verbs")
	mustMkdir(t, dir+"/disposition")

	mustWriteFile(t, dir+"/memory_verbs/cases.json", `[
		{"name": "remember a fact", "verb": "remember", "params": {"fact": "conformance {{marker}} fact"},
		 "expect": [{"path": "status", "equals": "inserted"}]}
	]`)

	mustWriteFile(t, dir+"/disposition/dispose.json", `{
		"protocol": "disposition", "operation": "dispose",
		"cases": [{
			"name": "dispose by an unknown group_id returns not_found",
			"steps": [{
				"method": "POST", "path": "/disposition/dispose",
				"request_body": {"group_id": "no-such-group", "verdict": "accept", "idempotency_key": "group-key-2"},
				"response": {"status": 404, "body": "{\"error\":\"not_found\",\"message\":\"no items found for group_id no-such-group\"}"}
			}]
		}]
	}`)
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
