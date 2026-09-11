package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/secrets"
)

func TestServeHTTPRequiresExactlyOneMode(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"serve"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--stdio") || !strings.Contains(err.Error(), "--http") {
		t.Fatalf("neither flag: err = %v", err)
	}

	cmd = newRootCmd()
	cmd.SetArgs([]string{"serve", "--stdio", "--http"})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err == nil {
		t.Fatal("both flags accepted")
	}
}

// runningHTTPServe starts `serenity -C root serve --http` in the
// background against a cancellable context and returns once it has
// printed its bound endpoint, so the caller can drive real HTTP requests
// against it. cancel (from the caller) ends the server; done reports its
// final error, which must be nil for a clean SIGTERM/SIGINT-equivalent
// shutdown (mirroring stdio's own ctx-cancel-is-not-an-error contract).
func runningHTTPServe(t *testing.T, root string) (addr string, stdout *syncBuffer, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetArgs([]string{"-C", root, "serve", "--http"})
	out := &syncBuffer{}
	cmd.SetOut(out)
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	ctx, cancelFn := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- cmd.ExecuteContext(ctx) }()

	deadline := time.Now().Add(5 * time.Second)
	var line string
	for time.Now().Before(deadline) {
		line = out.String()
		if strings.Contains(line, "http://") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(line, "http://") {
		cancelFn()
		t.Fatalf("serve --http never printed its bound endpoint; stdout=%q stderr=%q", line, stderr.String())
	}
	idx := strings.Index(line, "http://")
	endpoint := strings.TrimSpace(line[idx:])
	endpoint = strings.SplitN(endpoint, "\n", 2)[0]
	return endpoint, out, cancelFn, errCh
}

// syncBuffer is a concurrency-safe io.Writer, since the CLI command writes
// its startup line from the goroutine driving ExecuteContext while the
// test's own goroutine polls it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}
func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// TestServeHTTPEndToEnd drives the real `serenity serve --http` process
// entry point (in-process, over a real TCP loopback listener) through
// initialize/tools-list, authenticated DIRECTION/DISPOSITION route
// registration, an unauthenticated-request rejection, and a persistent
// remember/recall/forget round trip on a throwaway brain, then a clean
// shutdown. This is the CLI-level acc line: "the actual serve command
// starts on configured loopback port zero and reports its bound endpoint
// without secrets" plus the round-trip, protocol-route and auth acc lines,
// exercised through the real command rather than package-level handlers.
func TestServeHTTPEndToEnd(t *testing.T) {
	requireGit(t)
	root := pushFixture(t)

	token, err := secrets.DaemonToken()
	if err != nil {
		t.Fatalf("daemon token: %v", err)
	}

	endpoint, stdout, cancel, done := runningHTTPServe(t, root)
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("serve --http exited with error on shutdown: %v", err)
			}
		case <-time.After(8 * time.Second):
			t.Error("serve --http did not shut down")
		}
	}()

	if strings.Contains(stdout.String(), token) {
		t.Fatalf("bound-endpoint line leaked the daemon token: %q", stdout.String())
	}

	post := func(sessionID string, extraHeaders map[string]string, body string) (int, http.Header, map[string]json.RawMessage) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		if sessionID != "" {
			req.Header.Set("Mcp-Session-Id", sessionID)
			req.Header.Set("MCP-Protocol-Version", "2025-11-25")
		}
		for k, v := range extraHeaders {
			req.Header.Set(k, v)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		var decoded map[string]json.RawMessage
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &decoded)
		}
		return resp.StatusCode, resp.Header, decoded
	}

	// Unauthenticated: /mcp is wrapped by internal/server's own bearer
	// auth the same as every other route -- acc: "all MCP methods use
	// existing rotating bearer auth ... rejected auth ... requests cause
	// no tool invocation or writes."
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated initialize: status = %d, want 401", resp.StatusCode)
	}

	baseEndpoint := strings.TrimSuffix(endpoint, "/mcp")
	for _, route := range []string{"/direction/brief", "/disposition/list_pending"} {
		unauthReq, err := http.NewRequest(http.MethodGet, baseEndpoint+route, nil)
		if err != nil {
			t.Fatal(err)
		}
		unauthResp, err := http.DefaultClient.Do(unauthReq)
		if err != nil {
			t.Fatal(err)
		}
		_ = unauthResp.Body.Close()
		if unauthResp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("unauthenticated %s: status = %d, want 401", route, unauthResp.StatusCode)
		}

		authReq, err := http.NewRequest(http.MethodGet, baseEndpoint+route, nil)
		if err != nil {
			t.Fatal(err)
		}
		authReq.Header.Set("Authorization", "Bearer "+token)
		authResp, err := http.DefaultClient.Do(authReq)
		if err != nil {
			t.Fatal(err)
		}
		_ = authResp.Body.Close()
		if authResp.StatusCode == http.StatusNotFound {
			t.Fatalf("authenticated %s was not registered", route)
		}
	}

	status, header, reply := post("", nil, `{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	if status != http.StatusOK {
		t.Fatalf("authenticated initialize: status = %d, body %v", status, reply)
	}
	sessionID := header.Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatal("no Mcp-Session-Id on initialize response")
	}
	post(sessionID, nil, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)

	_, _, reply = post(sessionID, nil, `{"jsonrpc":"2.0","id":"list","method":"tools/list"}`)
	var listed struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(reply["result"], &listed); err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool, len(listed.Tools))
	for _, tl := range listed.Tools {
		got[tl.Name] = true
	}
	for _, want := range []string{"recall", "remember", "entity", "synthesize", "forget"} {
		if !got[want] {
			t.Errorf("tools/list missing %q; got %v", want, got)
		}
	}

	marker := fmt.Sprintf("t421httpconformance%d", time.Now().UnixNano())
	rememberArgs, _ := json.Marshal(map[string]any{"name": "remember", "arguments": map[string]any{"fact": "HTTP round trip fact " + marker, "provenance": "t4-21 test"}})
	_, _, remReply := post(sessionID, nil, fmt.Sprintf(`{"jsonrpc":"2.0","id":"remember","method":"tools/call","params":%s}`, rememberArgs))
	var remResult struct {
		Content []struct{ Text string } `json:"content"`
	}
	if err := json.Unmarshal(remReply["result"], &remResult); err != nil || len(remResult.Content) == 0 {
		t.Fatalf("remember result: %s (%v)", remReply["result"], err)
	}
	var remembered struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(remResult.Content[0].Text), &remembered); err != nil || remembered.ID == "" {
		t.Fatalf("remember did not return an id: %s", remResult.Content[0].Text)
	}

	recallArgs, _ := json.Marshal(map[string]any{"name": "recall", "arguments": map[string]any{"query": marker}})
	_, _, recallReply := post(sessionID, nil, fmt.Sprintf(`{"jsonrpc":"2.0","id":"recall","method":"tools/call","params":%s}`, recallArgs))
	var recallResult struct {
		Content []struct{ Text string } `json:"content"`
	}
	if err := json.Unmarshal(recallReply["result"], &recallResult); err != nil || len(recallResult.Content) == 0 {
		t.Fatalf("recall result: %s (%v)", recallReply["result"], err)
	}
	if !strings.Contains(recallResult.Content[0].Text, marker) {
		t.Fatalf("recall over HTTP did not see the remembered fact: %s", recallResult.Content[0].Text)
	}

	forgetArgs, _ := json.Marshal(map[string]any{"name": "forget", "arguments": map[string]any{"id": remembered.ID}})
	_, _, forgetReply := post(sessionID, nil, fmt.Sprintf(`{"jsonrpc":"2.0","id":"forget","method":"tools/call","params":%s}`, forgetArgs))
	var forgetResult struct {
		Content []struct{ Text string } `json:"content"`
	}
	if err := json.Unmarshal(forgetReply["result"], &forgetResult); err != nil || len(forgetResult.Content) == 0 {
		t.Fatalf("forget result: %s (%v)", forgetReply["result"], err)
	}
	var forgotten struct {
		Expired bool `json:"expired"`
	}
	if err := json.Unmarshal([]byte(forgetResult.Content[0].Text), &forgotten); err != nil || !forgotten.Expired {
		t.Fatalf("forget did not report expired:true over HTTP: %s", forgetResult.Content[0].Text)
	}
}
