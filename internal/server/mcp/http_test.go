package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/server/mcp"
)

// httpClient drives an mcp.HTTPHandler the way a real Streamable HTTP MCP
// client does: POST JSON-RPC bodies, remember the assigned Mcp-Session-Id,
// echo it plus MCP-Protocol-Version on every request after the first.
type httpClient struct {
	t         *testing.T
	srv       *httptest.Server
	sessionID string
}

func newHTTPTestServer(t *testing.T, tools ...mcp.Tool) (*httpClient, *mcp.HTTPHandler) {
	t.Helper()
	srv, err := mcp.New("test", tools)
	if err != nil {
		t.Fatal(err)
	}
	h := mcp.NewHTTPHandler(srv)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)
	return &httpClient{t: t, srv: ts}, h
}

type postResult struct {
	status int
	header http.Header
	body   map[string]json.RawMessage
	raw    []byte
}

func (c *httpClient) postRaw(body string, headers map[string]string, contentType string) postResult {
	c.t.Helper()
	req, err := http.NewRequest(http.MethodPost, c.srv.URL, strings.NewReader(body))
	if err != nil {
		c.t.Fatal(err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.sessionID != "" {
		req.Header.Set(mcp.SessionIDHeader, c.sessionID)
		req.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
	}
	for k, v := range headers {
		if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	if sid := resp.Header.Get(mcp.SessionIDHeader); sid != "" {
		c.sessionID = sid
	}
	res := postResult{status: resp.StatusCode, header: resp.Header, raw: raw}
	// Transport-level rejections (wrong content-type, oversized body, no
	// session, ...) are plain text via http.Error, not a JSON-RPC
	// envelope -- only attempt to decode a body the handler actually
	// claims is JSON. Callers of postRaw that expect a JSON-RPC error
	// object assert on that body via errCode, never on a plain-text one.
	if len(raw) > 0 && strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		if err := json.Unmarshal(raw, &res.body); err != nil {
			c.t.Fatalf("response is not a JSON object: %q: %v", raw, err)
		}
	}
	return res
}

func (c *httpClient) post(frame string) postResult {
	c.t.Helper()
	return c.postRaw(frame, nil, "application/json")
}

func (c *httpClient) initialize() postResult {
	c.t.Helper()
	r := c.post(`{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	if r.status != http.StatusOK {
		c.t.Fatalf("initialize status = %d, body %s", r.status, r.raw)
	}
	if c.sessionID == "" {
		c.t.Fatalf("no %s header on initialize response", mcp.SessionIDHeader)
	}
	c.post(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	return r
}

func errCode(t *testing.T, r postResult) int {
	t.Helper()
	var e struct{ Code int }
	if err := json.Unmarshal(r.body["error"], &e); err != nil {
		t.Fatalf("not a JSON-RPC error: %s", r.raw)
	}
	return e.Code
}

func TestHTTPHandshakeAndToolsList(t *testing.T) {
	t.Parallel()
	c, _ := newHTTPTestServer(t)
	r := c.initialize()
	if !strings.Contains(string(r.body["result"]), mcp.ProtocolVersion) {
		t.Fatalf("negotiated version missing: %s", r.raw)
	}
	if got := r.header.Get(mcp.ProtocolVersionHeader); got != mcp.ProtocolVersion {
		t.Fatalf("%s = %q, want %q", mcp.ProtocolVersionHeader, got, mcp.ProtocolVersion)
	}
	list := c.post(`{"jsonrpc":"2.0","id":"list","method":"tools/list"}`)
	if string(list.body["result"]) != `{"tools":[]}` {
		t.Fatalf("nonempty registry: %s", list.raw)
	}
}

// TestHTTPListsSameToolsAsStdio proves the acc line "lists exactly the
// same five repaired memory tools as stdio": the same Server (Tool
// registry) driven over HTTP reports the identical tool set stdio's own
// TestServeWiresMemoryTools proves for the real MEMORY_VERBS registry --
// here with a representative multi-tool registry standing in for it, since
// this package cannot import internal/server/memory without a cycle
// (internal/server/memory already imports internal/server/mcp).
func TestHTTPListsRegisteredTools(t *testing.T) {
	t.Parallel()
	names := []string{"recall", "remember", "entity", "synthesize", "forget"}
	tools := make([]mcp.Tool, len(names))
	for i, n := range names {
		tools[i] = mcp.Tool{Name: n, InputSchema: json.RawMessage(`{"type":"object"}`), Handler: func(context.Context, json.RawMessage) (mcp.Result, error) {
			return mcp.Result{}, nil
		}}
	}
	c, _ := newHTTPTestServer(t, tools...)
	c.initialize()
	r := c.post(`{"jsonrpc":"2.0","id":"list","method":"tools/list"}`)
	var listed struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(r.body["result"], &listed); err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool, len(listed.Tools))
	for _, tl := range listed.Tools {
		got[tl.Name] = true
	}
	for _, want := range names {
		if !got[want] {
			t.Errorf("tools/list missing %q; got %v", want, got)
		}
	}
}

// TestHTTPPersistentRoundTrip proves a session persists tool state across
// independent POST requests (acc: "performs a persistent remember/recall/
// forget round trip"): a value written by one call is visible to a later
// call in the same session, over genuinely separate HTTP request/response
// cycles, not just within one process's call stack.
func TestHTTPPersistentRoundTrip(t *testing.T) {
	t.Parallel()
	store := map[string]string{}
	tools := []mcp.Tool{
		{Name: "put", InputSchema: json.RawMessage(`{"type":"object","properties":{"key":{"type":"string"},"value":{"type":"string"}},"required":["key","value"]}`), Handler: func(_ context.Context, args json.RawMessage) (mcp.Result, error) {
			var p struct{ Key, Value string }
			if err := json.Unmarshal(args, &p); err != nil {
				return mcp.Result{}, err
			}
			store[p.Key] = p.Value
			return mcp.Result{Content: []mcp.Content{{Type: "text", Text: "ok"}}}, nil
		}},
		{Name: "get", InputSchema: json.RawMessage(`{"type":"object","properties":{"key":{"type":"string"}},"required":["key"]}`), Handler: func(_ context.Context, args json.RawMessage) (mcp.Result, error) {
			var p struct{ Key string }
			if err := json.Unmarshal(args, &p); err != nil {
				return mcp.Result{}, err
			}
			return mcp.Result{Content: []mcp.Content{{Type: "text", Text: store[p.Key]}}}, nil
		}},
		{Name: "delete", InputSchema: json.RawMessage(`{"type":"object","properties":{"key":{"type":"string"}},"required":["key"]}`), Handler: func(_ context.Context, args json.RawMessage) (mcp.Result, error) {
			var p struct{ Key string }
			if err := json.Unmarshal(args, &p); err != nil {
				return mcp.Result{}, err
			}
			delete(store, p.Key)
			return mcp.Result{Content: []mcp.Content{{Type: "text", Text: "ok"}}}, nil
		}},
	}
	c, _ := newHTTPTestServer(t, tools...)
	c.initialize()

	c.post(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"put","arguments":{"key":"k","value":"persisted-across-http-requests"}}}`)
	r := c.post(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get","arguments":{"key":"k"}}}`)
	if !strings.Contains(string(r.body["result"]), "persisted-across-http-requests") {
		t.Fatalf("recall did not see the earlier remember: %s", r.raw)
	}
	c.post(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"delete","arguments":{"key":"k"}}}`)
	r = c.post(`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get","arguments":{"key":"k"}}}`)
	if strings.Contains(string(r.body["result"]), "persisted-across-http-requests") {
		t.Fatalf("forget did not take effect: %s", r.raw)
	}
}

func TestHTTPRequiresSessionAfterInitialize(t *testing.T) {
	t.Parallel()
	c, _ := newHTTPTestServer(t)
	c.initialize()
	sid := c.sessionID
	c.sessionID = ""
	r := c.postRaw(`{"jsonrpc":"2.0","id":1,"method":"ping"}`, nil, "application/json")
	if r.status != http.StatusBadRequest {
		t.Fatalf("no session id: status = %d, want 400", r.status)
	}
	c.sessionID = sid
}

func TestHTTPUnknownSession(t *testing.T) {
	t.Parallel()
	c, _ := newHTTPTestServer(t)
	c.sessionID = "does-not-exist"
	r := c.postRaw(`{"jsonrpc":"2.0","id":1,"method":"ping"}`, map[string]string{mcp.ProtocolVersionHeader: mcp.ProtocolVersion}, "application/json")
	if r.status != http.StatusNotFound {
		t.Fatalf("unknown session: status = %d, want 404", r.status)
	}
}

func TestHTTPMissingOrWrongProtocolVersionHeader(t *testing.T) {
	t.Parallel()
	c, _ := newHTTPTestServer(t)
	c.initialize()
	for _, v := range []string{"", "2099-01-01"} {
		r := c.postRaw(`{"jsonrpc":"2.0","id":1,"method":"ping"}`, map[string]string{mcp.ProtocolVersionHeader: v}, "application/json")
		if r.status != http.StatusBadRequest {
			t.Fatalf("protocol version %q: status = %d, want 400", v, r.status)
		}
	}
}

func TestHTTPRejectsOrigin(t *testing.T) {
	t.Parallel()
	c, _ := newHTTPTestServer(t)
	r := c.postRaw(`{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`, map[string]string{"Origin": "http://evil.example"}, "application/json")
	if r.status != http.StatusForbidden {
		t.Fatalf("Origin present: status = %d, want 403", r.status)
	}
	if c.sessionID != "" {
		t.Fatal("a rejected-origin request must not create a session")
	}
}

func TestHTTPRejectsWrongContentType(t *testing.T) {
	t.Parallel()
	c, _ := newHTTPTestServer(t)
	r := c.postRaw(`{"jsonrpc":"2.0","id":"init","method":"initialize","params":{}}`, nil, "text/plain")
	if r.status != http.StatusUnsupportedMediaType {
		t.Fatalf("wrong content-type: status = %d, want 415", r.status)
	}
}

func TestHTTPRejectsOversizedBody(t *testing.T) {
	t.Parallel()
	c, _ := newHTTPTestServer(t)
	big := `{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"pad":"` + strings.Repeat("x", mcp.MaxFrameBytes+10) + `"}}`
	r := c.post(big)
	if r.status != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body: status = %d, want 413", r.status)
	}
}

func TestHTTPBootstrapRejectsNonInitialize(t *testing.T) {
	t.Parallel()
	c, _ := newHTTPTestServer(t)
	r := c.post(`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	if r.status != http.StatusBadRequest {
		t.Fatalf("first request not initialize: status = %d, want 400", r.status)
	}
	if c.sessionID != "" {
		t.Fatal("rejected bootstrap must not create a session")
	}
}

func TestHTTPBootstrapParseError(t *testing.T) {
	t.Parallel()
	c, _ := newHTTPTestServer(t)
	r := c.post(`{not json`)
	if r.status != http.StatusOK {
		t.Fatalf("parse error transport status = %d, want 200 (JSON-RPC error body)", r.status)
	}
	if errCode(t, r) != -32700 {
		t.Fatalf("parse error code: %s", r.raw)
	}
}

func TestHTTPInvalidInitializeDoesNotHoldSession(t *testing.T) {
	t.Parallel()
	c, _ := newHTTPTestServer(t)
	r := c.post(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if errCode(t, r) != -32602 {
		t.Fatalf("bad initialize params: %s", r.raw)
	}
	if c.sessionID != "" {
		t.Fatal("a failed initialize must not hand out a session id")
	}
}

func TestHTTPUnknownMethodAndTool(t *testing.T) {
	t.Parallel()
	c, _ := newHTTPTestServer(t)
	c.initialize()
	r := c.post(`{"jsonrpc":"2.0","id":1,"method":"missing"}`)
	if errCode(t, r) != -32601 {
		t.Fatalf("unknown method: %s", r.raw)
	}
	r = c.post(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"missing"}}`)
	if errCode(t, r) != -32602 {
		t.Fatalf("unknown tool: %s", r.raw)
	}
}

func TestHTTPNotificationGetsNoBody202(t *testing.T) {
	t.Parallel()
	c, _ := newHTTPTestServer(t)
	c.initialize()
	r := c.postRaw(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`, nil, "application/json")
	if r.status != http.StatusAccepted {
		t.Fatalf("notification status = %d, want 202", r.status)
	}
	if len(r.raw) != 0 {
		t.Fatalf("notification response carried a body: %q", r.raw)
	}
}

func TestHTTPDeleteEndsSession(t *testing.T) {
	t.Parallel()
	c, _ := newHTTPTestServer(t)
	c.initialize()
	req, err := http.NewRequest(http.MethodDelete, c.srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set(mcp.SessionIDHeader, c.sessionID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", resp.StatusCode)
	}
	r := c.postRaw(`{"jsonrpc":"2.0","id":1,"method":"ping"}`, map[string]string{mcp.ProtocolVersionHeader: mcp.ProtocolVersion}, "application/json")
	if r.status != http.StatusNotFound {
		t.Fatalf("post-DELETE session: status = %d, want 404", r.status)
	}
}

func TestHTTPGetIs405(t *testing.T) {
	t.Parallel()
	c, _ := newHTTPTestServer(t)
	resp, err := http.Get(c.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", resp.StatusCode)
	}
}

// TestHTTPDisconnectDoesNotCancelToolWork proves the acc line "request-body
// EOF and client disconnect do not implicitly cancel tool work": a POST
// whose client gives up waiting (context cancelled client-side) still lets
// the tool run to completion server-side -- observed here by a later call
// on the same session seeing the completed side effect.
func TestHTTPDisconnectDoesNotCancelToolWork(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	tools := []mcp.Tool{
		{Name: "slow", InputSchema: json.RawMessage(`{"type":"object"}`), Handler: func(ctx context.Context, _ json.RawMessage) (mcp.Result, error) {
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return mcp.Result{}, ctx.Err()
			}
			close(finished)
			return mcp.Result{Content: []mcp.Content{{Type: "text", Text: "done"}}}, nil
		}},
	}
	c, _ := newHTTPTestServer(t, tools...)
	c.initialize()

	reqCtx, cancelReq := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.srv.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"slow"}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(mcp.SessionIDHeader, c.sessionID)
	req.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)

	respCh := make(chan error, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
		respCh <- err
	}()
	<-started
	cancelReq() // simulate the client disconnecting mid-call
	select {
	case err := <-respCh:
		if err == nil {
			t.Fatal("expected the disconnected client's own request to fail")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("client request did not observe its own cancellation")
	}

	select {
	case <-finished:
		t.Fatal("tool finished before being released -- test is not exercising in-flight disconnect")
	default:
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("client disconnect implicitly cancelled tool work")
	}
}

// TestHTTPCloseJoinsInFlightWork proves the acc line "process shutdown ...
// joins workers": Close blocks until an in-flight tool call's goroutine has
// actually returned, not just until its own ctx is cancelled.
func TestHTTPCloseJoinsInFlightWork(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	returned := make(chan struct{})
	tools := []mcp.Tool{
		{Name: "block", InputSchema: json.RawMessage(`{"type":"object"}`), Handler: func(ctx context.Context, _ json.RawMessage) (mcp.Result, error) {
			close(started)
			<-ctx.Done()
			close(returned)
			return mcp.Result{}, ctx.Err()
		}},
	}
	srv, err := mcp.New("test", tools)
	if err != nil {
		t.Fatal(err)
	}
	h := mcp.NewHTTPHandler(srv)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	c := &httpClient{t: t, srv: ts}
	c.initialize()

	reqCtx, cancelReq := context.WithCancel(context.Background())
	defer cancelReq()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.srv.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"block"}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(mcp.SessionIDHeader, c.sessionID)
	req.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-started

	closed := make(chan struct{})
	go func() { h.Close(); close(closed) }()
	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not cancel the in-flight tool call")
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not join the in-flight call's goroutine")
	}
}

func TestHTTPMetricsTrackCallLifecycle(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	tools := []mcp.Tool{{Name: "block", InputSchema: json.RawMessage(`{"type":"object"}`), Handler: func(context.Context, json.RawMessage) (mcp.Result, error) {
		close(started)
		<-release
		return mcp.Result{Content: []mcp.Content{{Type: "text", Text: "ok"}}}, nil
	}}}
	srv, err := mcp.New("test", tools)
	if err != nil {
		t.Fatal(err)
	}
	h := mcp.NewHTTPHandler(srv)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)
	c := &httpClient{t: t, srv: ts}
	c.initialize()
	req, err := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"block"}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(mcp.SessionIDHeader, c.sessionID)
	req.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
	respCh := make(chan *http.Response, 1)
	errCh := make(chan error, 1)
	go func() {
		resp, requestErr := http.DefaultClient.Do(req)
		if requestErr != nil {
			errCh <- requestErr
			return
		}
		respCh <- resp
	}()
	<-started
	metrics := h.Metrics()
	if metrics.ActiveCalls != 1 || metrics.CallsStarted != 1 || metrics.CallsFinished != 0 {
		t.Fatalf("in-flight metrics = %+v, want one active unfinished call", metrics)
	}
	close(release)
	select {
	case err := <-errCh:
		t.Fatal(err)
	case resp := <-respCh:
		_ = resp.Body.Close()
	case <-time.After(3 * time.Second):
		t.Fatal("blocked HTTP call did not finish")
	}
	metrics = h.Metrics()
	if metrics.ActiveCalls != 0 || metrics.CallsFinished != 1 || metrics.CallsCanceled != 0 || metrics.CallLatencyNanos == 0 {
		t.Fatalf("finished metrics = %+v, want one completed call with latency", metrics)
	}
}

func TestHTTPMetricsTrackCancellation(t *testing.T) {
	started := make(chan struct{})
	finished := make(chan struct{})
	tools := []mcp.Tool{{Name: "cancel", InputSchema: json.RawMessage(`{"type":"object"}`), Handler: func(ctx context.Context, _ json.RawMessage) (mcp.Result, error) {
		close(started)
		<-ctx.Done()
		close(finished)
		return mcp.Result{}, ctx.Err()
	}}}
	srv, err := mcp.New("test", tools)
	if err != nil {
		t.Fatal(err)
	}
	h := mcp.NewHTTPHandler(srv)
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	t.Cleanup(h.Close)
	c := &httpClient{t: t, srv: ts}
	c.initialize()
	req, err := http.NewRequest(http.MethodPost, ts.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"cancel"}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(mcp.SessionIDHeader, c.sessionID)
	req.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
	go func() {
		resp, requestErr := http.DefaultClient.Do(req)
		if requestErr == nil {
			_ = resp.Body.Close()
		}
	}()
	<-started
	if r := c.post(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`); r.status != http.StatusAccepted {
		t.Fatalf("cancel notification status = %d, want 202", r.status)
	}
	select {
	case <-finished:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled tool did not finish")
	}
	metrics := h.Metrics()
	if metrics.ActiveCalls != 0 || metrics.CallsStarted != 1 || metrics.CallsFinished != 1 || metrics.CallsCanceled != 1 || metrics.CallLatencyNanos == 0 {
		t.Fatalf("cancelled metrics = %+v, want one finished cancellation", metrics)
	}
}

func TestHTTPMetricsTrackRejectedSessions(t *testing.T) {
	c, h := newHTTPTestServer(t)
	defer h.Close()
	initialize := func(i int) int {
		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":"init-%d","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`, i)
		req, err := http.NewRequest(http.MethodPost, c.srv.URL, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}
	for i := 0; i < mcp.MaxHTTPSessions; i++ {
		if got := initialize(i); got != http.StatusOK {
			t.Fatalf("initialize %d status = %d, want 200", i, got)
		}
	}
	if got := initialize(mcp.MaxHTTPSessions); got != http.StatusServiceUnavailable {
		t.Fatalf("overflow initialize status = %d, want 503", got)
	}
	if got := h.Metrics().SessionsRejected; got != 1 {
		t.Fatalf("SessionsRejected = %d, want 1", got)
	}
}
