package mcp_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/server/mcp"
)

type session struct {
	in     *io.PipeWriter
	frames chan []byte
	done   chan error
	cancel context.CancelFunc
}

func start(t *testing.T, tools ...mcp.Tool) *session {
	t.Helper()
	srv, err := mcp.New("test", tools)
	if err != nil {
		t.Fatal(err)
	}
	input, in := io.Pipe()
	out, output := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	s := &session{in: in, frames: make(chan []byte, 100), done: make(chan error, 1), cancel: cancel}
	go func() { s.done <- srv.Serve(ctx, input, output); _ = output.Close() }()
	go func() {
		defer close(s.frames)
		scanner := bufio.NewScanner(out)
		for scanner.Scan() {
			s.frames <- bytes.Clone(scanner.Bytes())
		}
		_ = out.Close()
	}()
	t.Cleanup(func() {
		cancel()
		_ = in.Close()
		select {
		case err := <-s.done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("session failed to stop")
		}
	})
	return s
}
func (s *session) send(t *testing.T, frame string) {
	t.Helper()
	if _, err := io.WriteString(s.in, frame+"\n"); err != nil {
		t.Fatal(err)
	}
}
func (s *session) read(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	select {
	case raw := <-s.frames:
		var reply map[string]json.RawMessage
		if json.Unmarshal(raw, &reply) != nil {
			t.Fatalf("invalid frame %q", raw)
		}
		return reply
	case <-time.After(3 * time.Second):
		t.Fatal("response timeout")
		return nil
	}
}
func (s *session) ready(t *testing.T) {
	t.Helper()
	s.send(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	if r := s.read(t); r["error"] != nil {
		t.Fatal(string(r["error"]))
	}
	s.send(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
}
func code(t *testing.T, r map[string]json.RawMessage, want int) {
	t.Helper()
	var e struct{ Code int }
	if err := json.Unmarshal(r["error"], &e); err != nil || e.Code != want {
		t.Fatalf("error %s, want %d", r["error"], want)
	}
}

func TestHandshake(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"2025-11-25", "future-version"} {
		t.Run(version, func(t *testing.T) {
			s := start(t)
			s.send(t, `{"jsonrpc":"2.0","id":123456789012345678901234567890,"method":"initialize","params":{"protocolVersion":"`+version+`","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
			r := s.read(t)
			if string(r["id"]) != "123456789012345678901234567890" || !bytes.Contains(r["result"], []byte(mcp.ProtocolVersion)) {
				t.Fatalf("handshake: %s", r)
			}
			s.send(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
			code(t, s.read(t), -32600)
			s.send(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
			s.send(t, `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`)
			if string(s.read(t)["result"]) != `{"tools":[]}` {
				t.Fatal("nonempty registry")
			}
		})
	}
}
func TestUnknownTool(t *testing.T) {
	t.Parallel()
	s := start(t)
	s.ready(t)
	s.send(t, `{"jsonrpc":"2.0","id":"unknown","method":"tools/call","params":{"name":"missing"}}`)
	code(t, s.read(t), -32602)
}
func TestMalformedJSONRPC(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, frame string
		want        int
	}{
		{"syntax", `{`, -32700}, {"trailing", `{} {}`, -32700}, {"batch", `[]`, -32600}, {"null", `null`, -32600}, {"scalar", `1`, -32600}, {"missing version", `{"id":1,"method":"ping"}`, -32600}, {"null id", `{"jsonrpc":"2.0","id":null,"method":"ping"}`, -32600}, {"fraction id", `{"jsonrpc":"2.0","id":1.1,"method":"ping"}`, -32600}, {"duplicate id", `{"jsonrpc":"2.0","id":1,"id":2,"method":"ping"}`, -32600}, {"unknown method", `{"jsonrpc":"2.0","id":1,"method":"missing"}`, -32601}, {"bad params", `{"jsonrpc":"2.0","id":1,"method":"ping","params":[]}`, -32602}, {"bad initialize", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, -32602}, {"invalid utf8", "{\"bad\":\"\xff\"}", -32700},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := start(t)
			s.send(t, tc.frame)
			code(t, s.read(t), tc.want)
			s.send(t, `{"jsonrpc":"2.0","id":"alive","method":"ping"}`)
			if string(s.read(t)["id"]) != `"alive"` {
				t.Fatal("server did not recover")
			}
		})
	}
}
func TestStdoutFrames(t *testing.T) {
	t.Parallel()
	s := start(t)
	s.ready(t)
	for _, frame := range []string{`{"jsonrpc":"2.0","method":"ping"}`, `{"jsonrpc":"2.0","method":"unknown"}`, `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"missing"}}`} {
		s.send(t, frame)
	}
	s.send(t, `{"jsonrpc":"2.0","id":"only response","method":"ping"}`)
	r := s.read(t)
	if string(r["id"]) != `"only response"` {
		t.Fatalf("notification produced response: %s", r)
	}
}
func TestRegisteredTool(t *testing.T) {
	t.Parallel()
	tool := mcp.Tool{Name: "echo", InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`), Handler: func(_ context.Context, args json.RawMessage) (mcp.Result, error) {
		var p struct{ Text string }
		if err := json.Unmarshal(args, &p); err != nil {
			return mcp.Result{}, err
		}
		return mcp.Result{Content: []mcp.Content{{Type: "text", Text: p.Text}}}, nil
	}}
	s := start(t, tool)
	s.ready(t)
	s.send(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if !bytes.Contains(s.read(t)["result"], []byte(`"name":"echo"`)) {
		t.Fatal("tool not discoverable")
	}
	s.send(t, `{"jsonrpc":"2.0","id":"ok","method":"tools/call","params":{"name":"echo","arguments":{"text":"hello\nworld"}}}`)
	if !bytes.Contains(s.read(t)["result"], []byte(`hello\nworld`)) {
		t.Fatal("tool not invoked")
	}
	for _, args := range []string{`{}`, `{"text":1}`, `[]`, `null`} {
		s.send(t, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"echo","arguments":`+args+`}}`)
		r := s.read(t)
		if args == `[]` || args == `null` {
			code(t, r, -32602)
		} else if !bytes.Contains(r["result"], []byte(`"isError":true`)) {
			t.Fatalf("schema failure: %s", r)
		}
	}
}
func TestCancellation(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	stopped := make(chan struct{})
	s := start(t, mcp.Tool{Name: "wait", InputSchema: json.RawMessage(`{"type":"object"}`), Handler: func(ctx context.Context, _ json.RawMessage) (mcp.Result, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return mcp.Result{}, ctx.Err()
	}})
	s.ready(t)
	s.send(t, `{"jsonrpc":"2.0","id":77,"method":"tools/call","params":{"name":"wait"}}`)
	<-started
	s.send(t, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"77"}}`)
	s.send(t, `{"jsonrpc":"2.0","id":"ping","method":"ping"}`)
	s.read(t)
	select {
	case <-stopped:
		t.Fatal("string ID cancelled numeric request")
	default:
	}
	s.send(t, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":77}}`)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("tool not cancelled")
	}
	s.send(t, `{"jsonrpc":"2.0","id":"barrier","method":"ping"}`)
	if string(s.read(t)["id"]) != `"barrier"` {
		t.Fatal("cancelled request replied")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestOutputFailure(t *testing.T) {
	t.Parallel()
	srv, _ := mcp.New("test", nil)
	err := srv.Serve(context.Background(), io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n")), failingWriter{})
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write error: %v", err)
	}
}
func TestFrameBound(t *testing.T) {
	t.Parallel()
	srv, _ := mcp.New("test", nil)
	err := srv.Serve(context.Background(), io.NopCloser(strings.NewReader(strings.Repeat("x", mcp.MaxFrameBytes+10)+"\n")), io.Discard)
	if err == nil {
		t.Fatal("oversized frame accepted")
	}
}
func TestIdleShutdown(t *testing.T) { t.Parallel(); s := start(t); s.cancel() }
func TestToolFailures(t *testing.T) {
	t.Parallel()
	for _, panics := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "panic"}[panics], func(t *testing.T) {
			s := start(t, mcp.Tool{Name: "fail", InputSchema: json.RawMessage(`{"type":"object"}`), Handler: func(context.Context, json.RawMessage) (mcp.Result, error) {
				if panics {
					panic("secret")
				}
				return mcp.Result{}, errors.New("secret")
			}})
			s.ready(t)
			s.send(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"fail"}}`)
			r := s.read(t)
			if !bytes.Contains(r["result"], []byte(`"isError":true`)) || bytes.Contains(r["result"], []byte("secret")) {
				t.Fatalf("unsafe tool failure %s", r)
			}
		})
	}
}

func TestRegistryValidation(t *testing.T) {
	t.Parallel()
	handler := func(context.Context, json.RawMessage) (mcp.Result, error) { return mcp.Result{}, nil }
	good := mcp.Tool{Name: "good", InputSchema: json.RawMessage(`{"type":"object"}`), Handler: handler}
	cases := []struct {
		name  string
		tools []mcp.Tool
	}{
		{"nil handler", []mcp.Tool{{Name: "a", InputSchema: good.InputSchema}}},
		{"bad name", []mcp.Tool{{Name: "a b", InputSchema: good.InputSchema, Handler: handler}}},
		{"bad schema", []mcp.Tool{{Name: "a", InputSchema: json.RawMessage(`{`), Handler: handler}}},
		{"nonobject schema", []mcp.Tool{{Name: "a", InputSchema: json.RawMessage(`{"type":"string"}`), Handler: handler}}},
		{"invalid schema keyword", []mcp.Tool{{Name: "a", InputSchema: json.RawMessage(`{"type":"object","required":2}`), Handler: handler}}},
		{"duplicate", []mcp.Tool{good, good}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := mcp.New("test", tc.tools); err == nil {
				t.Fatal("invalid registration accepted")
			}
		})
	}
	if _, err := mcp.New("", nil); err == nil {
		t.Fatal("empty version accepted")
	}
}
func TestInFlightBound(t *testing.T) {
	t.Parallel()
	started := make(chan struct{}, mcp.MaxInFlight)
	s := start(t, mcp.Tool{Name: "block", InputSchema: json.RawMessage(`{"type":"object"}`), Handler: func(ctx context.Context, _ json.RawMessage) (mcp.Result, error) {
		started <- struct{}{}
		<-ctx.Done()
		return mcp.Result{}, ctx.Err()
	}})
	s.ready(t)
	for i := 0; i < mcp.MaxInFlight; i++ {
		frame, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": i + 10, "method": "tools/call", "params": map[string]string{"name": "block"}})
		s.send(t, string(frame))
		<-started
	}
	s.send(t, `{"jsonrpc":"2.0","id":"overflow","method":"tools/call","params":{"name":"block"}}`)
	code(t, s.read(t), -32000)
	s.send(t, `{"jsonrpc":"2.0","id":"responsive","method":"ping"}`)
	if string(s.read(t)["id"]) != `"responsive"` {
		t.Fatal("full registry blocked ping")
	}
}
func TestDuplicateInFlightID(t *testing.T) {
	t.Parallel()
	srv, err := mcp.New("test", []mcp.Tool{{Name: "wait", InputSchema: json.RawMessage(`{"type":"object"}`), Handler: func(ctx context.Context, _ json.RawMessage) (mcp.Result, error) {
		<-ctx.Done()
		return mcp.Result{}, ctx.Err()
	}}})
	if err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"wait"}}`, `{"jsonrpc":"2.0","id":2,"method":"ping"}`, ""}, "\n")
	if err := srv.Serve(context.Background(), io.NopCloser(strings.NewReader(input)), io.Discard); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate ID: %v", err)
	}
}
func TestRegistrySnapshot(t *testing.T) {
	t.Parallel()
	schema := json.RawMessage(`{"type":"object"}`)
	srv, err := mcp.New("test", []mcp.Tool{{Name: "original", InputSchema: schema, Handler: func(context.Context, json.RawMessage) (mcp.Result, error) { return mcp.Result{}, nil }}})
	if err != nil {
		t.Fatal(err)
	}
	copy(schema, strings.Repeat("x", len(schema)))
	input := strings.Join([]string{`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, ""}, "\n")
	var out bytes.Buffer
	if err := srv.Serve(context.Background(), io.NopCloser(strings.NewReader(input)), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"inputSchema":{"type":"object"}`) {
		t.Fatalf("registry mutated: %s", out.String())
	}
}

type panicJSON struct{}

func (panicJSON) MarshalJSON() ([]byte, error) { panic("secret marshal panic") }
func TestInvalidToolResult(t *testing.T) {
	t.Parallel()
	cyclic := map[string]any{}
	cyclic["self"] = cyclic
	for name, value := range map[string]any{"channel": make(chan int), "cycle": cyclic, "panic": panicJSON{}} {
		t.Run(name, func(t *testing.T) {
			s := start(t, mcp.Tool{Name: "bad", InputSchema: json.RawMessage(`{"type":"object"}`), Handler: func(context.Context, json.RawMessage) (mcp.Result, error) {
				return mcp.Result{StructuredContent: map[string]any{"value": value}}, nil
			}})
			s.ready(t)
			s.send(t, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"bad"}}`)
			if !bytes.Contains(s.read(t)["result"], []byte(`"isError":true`)) {
				t.Fatal("invalid result not contained")
			}
			s.send(t, `{"jsonrpc":"2.0","id":3,"method":"ping"}`)
			if string(s.read(t)["id"]) != "3" {
				t.Fatal("session failed after invalid result")
			}
		})
	}
}
func TestBlockedOutputShutdown(t *testing.T) {
	t.Parallel()
	srv, _ := mcp.New("test", nil)
	input, send := io.Pipe()
	receive, output := io.Pipe()
	t.Cleanup(func() { _ = send.Close(); _ = receive.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, input, output) }()
	if _, err := io.WriteString(send, `{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked output prevented shutdown")
	}
}

func TestLifecycleAndParameters(t *testing.T) {
	t.Parallel()
	s := start(t)
	s.send(t, `{"jsonrpc":"2.0","id":0,"method":"tools/call","params":{"name":"absent"}}`)
	code(t, s.read(t), -32600)
	s.ready(t)
	for _, tc := range []struct {
		frame string
		want  int
	}{
		{`{"jsonrpc":"2.0","id":2,"method":"initialize","params":{}}`, -32600},
		{`{"jsonrpc":"2.0","id":3,"method":"tools/list","params":[]}`, -32602},
		{`{"jsonrpc":"2.0","id":4,"method":"tools/list","params":{"cursor":2}}`, -32602},
		{`{"jsonrpc":"2.0","id":5,"method":"tools/list","params":{"cursor":"next"}}`, -32602},
		{`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":1}}`, -32602},
		{`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":null}`, -32602},
		{`{"jsonrpc":"2.0","id":8,"method":"ping","result":{}}`, -32600},
		{`{"jsonrpc":"2.0","id":9,"method":"ping","error":{}}`, -32600},
	} {
		s.send(t, tc.frame)
		code(t, s.read(t), tc.want)
	}
}
func TestEmptyToolContent(t *testing.T) {
	t.Parallel()
	s := start(t, mcp.Tool{Name: "empty", InputSchema: json.RawMessage(`{"type":"object"}`), Handler: func(context.Context, json.RawMessage) (mcp.Result, error) { return mcp.Result{}, nil }})
	s.ready(t)
	s.send(t, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"empty"}}`)
	if string(s.read(t)["result"]) != `{"content":[]}` {
		t.Fatal("nil content not normalized")
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestShortWrite(t *testing.T) {
	t.Parallel()
	srv, _ := mcp.New("test", nil)
	err := srv.Serve(context.Background(), io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n")), shortWriter{})
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write: %v", err)
	}
}
func TestMissingStreams(t *testing.T) {
	t.Parallel()
	srv, _ := mcp.New("test", nil)
	if err := srv.Serve(context.Background(), nil, io.Discard); err == nil {
		t.Fatal("nil input accepted")
	}
	if err := srv.Serve(context.Background(), io.NopCloser(strings.NewReader("")), nil); err == nil {
		t.Fatal("nil output accepted")
	}
}
