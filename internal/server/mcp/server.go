package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// MaxFrameBytes and MaxInFlight bound session input memory and tool workers.
const MaxFrameBytes = 1 << 20
const MaxInFlight = 32

type inputFrame struct {
	data []byte
	err  error
}

// activeCall tracks one in-flight tools/call invocation registered against
// a Session, so a later notifications/cancelled naming the same request id
// can cancel it and suppress its reply -- MCP sends no response, success or
// failure, for a cancelled call.
type activeCall struct {
	cancel    context.CancelFunc
	cancelled bool
}

// Session holds one MCP connection's protocol state -- the new/initialize-
// answered/initialized lifecycle and the set of in-flight tools/call
// invocations -- independent of any one transport's I/O. Both the stdio
// transport (Serve, one Session per process-lifetime connection) and the
// Streamable HTTP transport (internal/server/mcp's HTTPHandler, one Session
// per Mcp-Session-Id spanning many independent POSTs) dispatch through
// these same methods: T4.21's own design note is that HTTP must share this
// dispatch logic rather than calling the stream-owning Serve once per HTTP
// request, since Serve's input/output-closing ownership model has no
// meaning for a stateless HTTP POST.
//
// A Session's methods are safe for concurrent use: stdio's own Serve drives
// them from a single goroutine (the mutex is uncontended overhead there),
// but HTTP's multiple concurrent POSTs against one session id need the
// real protection.
type Session struct {
	srv *Server

	mu    sync.Mutex
	state int // 0 new, 1 initialize answered, 2 initialized
	calls map[string]*activeCall
}

// NewSession starts a fresh MCP protocol session over srv's tool registry.
func (s *Server) NewSession() *Session {
	return &Session{srv: s, calls: make(map[string]*activeCall)}
}

// PendingCall is a validated tools/call invocation ready to run. Run
// executes the tool handler and returns the JSON-RPC response to send, or
// ok=false if the call was cancelled before or during execution -- a
// cancelled call gets no response at all, success or failure. Run must be
// called exactly once, from any goroutine; the caller decides how to
// schedule and join it (stdio's own per-Serve worker WaitGroup, HTTP's
// per-listener one).
type PendingCall struct {
	run func() (reply response, ok bool)
}

// Run invokes the pending tool call. See PendingCall's own doc.
func (p *PendingCall) Run() (reply response, ok bool) { return p.run() }

// HandleNotification processes one parsed notification (no id). MCP never
// replies to a notification, so this returns nothing to send.
func (sess *Session) HandleNotification(req request) {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	switch req.method {
	case "notifications/initialized":
		if sess.state == 1 && optionalObject(req.params) {
			sess.state = 2
		}
	case "notifications/cancelled":
		if !objectParams(req.params) {
			return
		}
		var p struct {
			RequestID json.RawMessage `json:"requestId"`
		}
		if json.Unmarshal(req.params, &p) != nil {
			return
		}
		key, ok := idKey(p.RequestID)
		if !ok {
			return
		}
		if call := sess.calls[key]; call != nil {
			call.cancelled = true
			call.cancel()
		}
	}
}

// HandleRequest processes one parsed request carrying an id (a call that
// expects a reply). ctx bounds any spawned tool call's lifetime -- pass the
// long-lived context appropriate to the transport (stdio: Serve's own ctx,
// canceled by EOF/signal; HTTP: the listener's process-lifetime ctx, NEVER
// an individual HTTP request's own context) so that an event scoped to one
// transport request (HTTP: the request body hitting EOF, or the client
// disconnecting) never implicitly cancels tool work -- only an explicit
// notifications/cancelled does, via HandleNotification.
//
// For every method except an accepted "tools/call", it returns the reply
// to send immediately, with pending == nil. For an accepted "tools/call" it
// registers the call (so a concurrent HandleNotification can cancel it) and
// returns pending != nil: the caller must invoke pending.Run (exactly once,
// from any goroutine) and, if it reports ok, send the resulting reply --
// ok=false means the call was cancelled and must not be replied to at all.
//
// fatal reports a session-ending protocol violation (currently: a request
// id reused while still in flight). stdio's Serve treats a fatal error as
// it always has -- ending the whole connection; HTTP has no analogous
// "connection" to end, so it translates fatal into a per-request JSON-RPC
// error instead of tearing down the session.
func (sess *Session) HandleRequest(ctx context.Context, req request) (reply *response, pending *PendingCall, fatal error) {
	sess.mu.Lock()
	defer sess.mu.Unlock()

	if _, exists := sess.calls[req.key]; exists {
		return nil, nil, fmt.Errorf("duplicate in-flight MCP request id")
	}

	switch req.method {
	case "initialize":
		var p struct {
			ProtocolVersion string          `json:"protocolVersion"`
			Capabilities    json.RawMessage `json:"capabilities"`
			ClientInfo      struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"clientInfo"`
		}
		var r response
		switch {
		case sess.state != 0:
			r = failure(req.id, -32600, "Already initialized")
		case !objectParams(req.params) || json.Unmarshal(req.params, &p) != nil || p.ProtocolVersion == "" || !objectParams(p.Capabilities) || p.ClientInfo.Name == "" || p.ClientInfo.Version == "":
			r = failure(req.id, -32602, "Invalid initialize parameters")
		default:
			sess.state = 1
			r = success(req.id, map[string]any{"protocolVersion": ProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{"listChanged": false}}, "serverInfo": map[string]string{"name": "serenity", "version": sess.srv.version}})
		}
		return &r, nil, nil

	case "ping":
		r := success(req.id, struct{}{})
		if !optionalObject(req.params) {
			r = failure(req.id, -32602, "Invalid ping parameters")
		}
		return &r, nil, nil

	case "tools/list":
		var r response
		switch {
		case sess.state != 2:
			r = failure(req.id, -32600, "Initialization required")
		case !optionalObject(req.params):
			r = failure(req.id, -32602, "Invalid tools/list parameters")
		default:
			var p struct {
				Cursor *string `json:"cursor"`
			}
			switch {
			case len(req.params) > 0 && json.Unmarshal(req.params, &p) != nil:
				r = failure(req.id, -32602, "Invalid tools/list parameters")
			case p.Cursor != nil:
				r = failure(req.id, -32602, "Unknown tools cursor")
			default:
				r = success(req.id, map[string]any{"tools": sess.srv.tools})
			}
		}
		return &r, nil, nil

	case "tools/call":
		if sess.state != 2 {
			r := failure(req.id, -32600, "Initialization required")
			return &r, nil, nil
		}
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if !objectParams(req.params) || json.Unmarshal(req.params, &p) != nil || p.Name == "" {
			r := failure(req.id, -32602, "Invalid tools/call parameters")
			return &r, nil, nil
		}
		tool, ok := sess.srv.byName[p.Name]
		if !ok {
			r := failure(req.id, -32602, "Unknown tool")
			return &r, nil, nil
		}
		if len(p.Arguments) == 0 {
			p.Arguments = json.RawMessage("{}")
		}
		args, err := jsonschemaValue(p.Arguments)
		if err != nil || !objectParams(p.Arguments) {
			r := failure(req.id, -32602, "Invalid tool arguments")
			return &r, nil, nil
		}
		if tool.schema.Validate(args) != nil {
			r := success(req.id, toolFailure(tool.tool, InvalidArguments))
			return &r, nil, nil
		}
		if len(sess.calls) >= MaxInFlight {
			r := failure(req.id, -32000, "Too many in-flight tool calls")
			return &r, nil, nil
		}
		callCtx, callCancel := context.WithCancel(ctx)
		call := &activeCall{cancel: callCancel}
		sess.calls[req.key] = call
		id, key, toolCopy, argsCopy := req.id, req.key, tool.tool, p.Arguments
		return nil, &PendingCall{run: func() (response, bool) {
			result := invoke(callCtx, toolCopy, argsCopy)
			cancelled := sess.finish(key)
			callCancel()
			if cancelled {
				return response{}, false
			}
			return success(id, result), true
		}}, nil

	default:
		r := failure(req.id, -32601, "Method not found")
		return &r, nil, nil
	}
}

// finish removes key from the in-flight set and reports whether it was
// cancelled meanwhile.
func (sess *Session) finish(key string) bool {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	call, ok := sess.calls[key]
	delete(sess.calls, key)
	return ok && call.cancelled
}

// Serve owns input until return, closing it on shutdown to unblock Read. Close
// must unblock concurrent Read. Closeable output is also owned and closed on
// shutdown to interrupt a blocked Write. Other writers must return promptly.
// Handlers must return when their context is cancelled.
// EOF or context cancellation stops the session and joins every worker.
func (s *Server) Serve(ctx context.Context, input io.ReadCloser, output io.Writer) error {
	if input == nil || output == nil {
		return fmt.Errorf("MCP input and output are required")
	}
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	workers.Go(func() {
		<-ctx.Done()
		_ = input.Close()
		if closer, ok := output.(io.Closer); ok {
			_ = closer.Close()
		}
	})
	frames := make(chan inputFrame)
	workers.Go(func() {
		scanner := bufio.NewScanner(input)
		scanner.Buffer(make([]byte, 4096), MaxFrameBytes+2)
		for scanner.Scan() {
			data := bytes.Clone(scanner.Bytes())
			if len(data) > MaxFrameBytes {
				select {
				case frames <- inputFrame{err: fmt.Errorf("MCP frame exceeds %d bytes", MaxFrameBytes)}:
				case <-ctx.Done():
				}
				return
			}
			select {
			case frames <- inputFrame{data: data}:
			case <-ctx.Done():
				return
			}
		}
		err := scanner.Err()
		if err == nil {
			err = io.EOF
		}
		select {
		case frames <- inputFrame{err: err}:
		case <-ctx.Done():
		}
	})
	defer func() { cancel(); workers.Wait() }()

	sess := s.NewSession()
	completed := make(chan response, MaxInFlight)
	write := func(reply response) error {
		frame, err := json.Marshal(reply)
		if err != nil {
			return fmt.Errorf("encode MCP response: %w", err)
		}
		frame = append(frame, '\n')
		n, err := output.Write(frame)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("write MCP response: %w", err)
		}
		if n != len(frame) {
			return fmt.Errorf("write MCP response: %w", io.ErrShortWrite)
		}
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case reply := <-completed:
			if err := write(reply); err != nil {
				return err
			}
		case frame := <-frames:
			if frame.err != nil {
				if ctx.Err() != nil || errors.Is(frame.err, io.EOF) {
					return nil
				}
				return fmt.Errorf("read MCP frame: %w", frame.err)
			}
			req, bad := parse(frame.data)
			if bad != nil {
				if err := write(*bad); err != nil {
					return err
				}
				continue
			}
			if len(req.id) == 0 {
				sess.HandleNotification(req)
				continue
			}
			reply, pending, fatal := sess.HandleRequest(ctx, req)
			if fatal != nil {
				return fatal
			}
			if pending == nil {
				if err := write(*reply); err != nil {
					return err
				}
				continue
			}
			workers.Go(func() {
				reply, ok := pending.Run()
				if !ok {
					return
				}
				completed <- reply
			})
		}
	}
}

func jsonschemaValue(raw json.RawMessage) (any, error) {
	var value any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	err := dec.Decode(&value)
	return value, err
}

func invoke(ctx context.Context, tool Tool, args json.RawMessage) (encoded json.RawMessage) {
	failed, _ := json.Marshal(toolFailure(tool, ExecutionFailed))
	encoded = json.RawMessage(failed)
	defer func() {
		if recover() != nil {
			encoded = json.RawMessage(failed)
		}
	}()
	result, err := tool.Handler(ctx, args)
	if err != nil {
		return encoded
	}
	if result.Content == nil {
		result.Content = []Content{}
	}
	for _, c := range result.Content {
		if c.Type != "text" {
			return encoded
		}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return encoded
	}
	return data
}

func toolFailure(tool Tool, kind FailureKind) (result Result) {
	message := "Tool execution failed"
	if kind == InvalidArguments {
		message = "Arguments do not match the tool input schema; check tools/list for required fields and types"
	}
	fallback := Result{Content: []Content{{Type: "text", Text: message}}, IsError: true}
	result = fallback
	defer func() {
		if recover() != nil {
			result = fallback
		}
	}()
	if tool.Failure == nil {
		return result
	}
	candidate := tool.Failure(kind)
	if len(candidate.Content) == 0 {
		return result
	}
	for _, c := range candidate.Content {
		if c.Type != "text" {
			return result
		}
	}
	candidate.IsError = true
	if _, err := json.Marshal(candidate); err != nil {
		return result
	}
	return candidate
}
