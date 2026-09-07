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
func newConformanceTestServer(t *testing.T, disposeVerdict string) *httptest.Server {
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
	mux.HandleFunc("/disposition/dispose", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"results":[{"item":{"id":"ffffffffffffffffffffffffffffffff","verdict":%q}}]}`, disposeVerdict)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestProtocolConformanceEndToEndAgainstALiveServer(t *testing.T) {
	srv := newConformanceTestServer(t, "accept")

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
}

func TestProtocolConformanceReportsFailureAndNonzeroExit(t *testing.T) {
	srv := newConformanceTestServer(t, "reject") // diverges from the fixture's "accept"

	fixtures := t.TempDir()
	writeConformanceFixtures(t, fixtures)

	cmd := newRootCmd()
	cmd.SetArgs([]string{"protocol", "conformance", "--target", srv.URL, "--fixtures", fixtures, "--protocol", "disposition"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected a verdict mismatch to fail the command")
	}
	if !strings.Contains(out.String(), "FAIL") {
		t.Fatalf("expected the text report to show a FAIL line, got:\n%s", out.String())
	}
}

// writeConformanceFixtures writes a minimal one-case memory_verbs/
// cases.json and disposition/dispose.json into dir, standing in for the
// real testdata/conformance/ set so these tests don't depend on --target
// matching the full frozen fixture corpus's own seeded state.
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
			"name": "dispose accept records the verdict",
			"steps": [{
				"method": "POST", "path": "/disposition/dispose",
				"request_body": {"item_id": "cda08aff5a5749c7f6ad9b50d90554ac", "verdict": "accept", "idempotency_key": "k1"},
				"response": {"status": 200, "body": "{\"results\":[{\"item\":{\"id\":\"cda08aff5a5749c7f6ad9b50d90554ac\",\"verdict\":\"accept\"}}]}"}
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
