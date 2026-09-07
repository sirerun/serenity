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
type activeCall struct {
	cancel    context.CancelFunc
	cancelled bool
}
type completion struct {
	key   string
	call  *activeCall
	reply response
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
	calls := make(map[string]*activeCall)
	completed := make(chan completion, MaxInFlight)
	state := 0 // new, initialize answered, initialized
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
		case done := <-completed:
			delete(calls, done.key)
			done.call.cancel()
			if !done.call.cancelled {
				if err := write(done.reply); err != nil {
					return err
				}
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
				if req.method == "notifications/initialized" && state == 1 && optionalObject(req.params) {
					state = 2
				}
				if req.method == "notifications/cancelled" && objectParams(req.params) {
					var p struct {
						RequestID json.RawMessage `json:"requestId"`
					}
					if json.Unmarshal(req.params, &p) == nil {
						if key, ok := idKey(p.RequestID); ok {
							if call := calls[key]; call != nil {
								call.cancelled = true
								call.cancel()
							}
						}
					}
				}
				continue
			}
			if _, exists := calls[req.key]; exists {
				return fmt.Errorf("duplicate in-flight MCP request id")
			}
			reply := success(req.id, struct{}{})
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
				if state != 0 {
					reply = failure(req.id, -32600, "Already initialized")
				} else if !objectParams(req.params) || json.Unmarshal(req.params, &p) != nil || p.ProtocolVersion == "" || !objectParams(p.Capabilities) || p.ClientInfo.Name == "" || p.ClientInfo.Version == "" {
					reply = failure(req.id, -32602, "Invalid initialize parameters")
				} else {
					state = 1
					reply = success(req.id, map[string]any{"protocolVersion": ProtocolVersion, "capabilities": map[string]any{"tools": map[string]any{"listChanged": false}}, "serverInfo": map[string]string{"name": "serenity", "version": s.version}})
				}
			case "ping":
				if !optionalObject(req.params) {
					reply = failure(req.id, -32602, "Invalid ping parameters")
				}
			case "tools/list":
				if state != 2 {
					reply = failure(req.id, -32600, "Initialization required")
				} else if !optionalObject(req.params) {
					reply = failure(req.id, -32602, "Invalid tools/list parameters")
				} else {
					var p struct {
						Cursor *string `json:"cursor"`
					}
					if len(req.params) > 0 && json.Unmarshal(req.params, &p) != nil {
						reply = failure(req.id, -32602, "Invalid tools/list parameters")
					} else if p.Cursor != nil {
						reply = failure(req.id, -32602, "Unknown tools cursor")
					} else {
						reply = success(req.id, map[string]any{"tools": s.tools})
					}
				}
			case "tools/call":
				if state != 2 {
					reply = failure(req.id, -32600, "Initialization required")
					break
				}
				var p struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				}
				if !objectParams(req.params) || json.Unmarshal(req.params, &p) != nil || p.Name == "" {
					reply = failure(req.id, -32602, "Invalid tools/call parameters")
					break
				}
				tool, ok := s.byName[p.Name]
				if !ok {
					reply = failure(req.id, -32602, "Unknown tool")
					break
				}
				if len(p.Arguments) == 0 {
					p.Arguments = json.RawMessage("{}")
				}
				args, err := jsonschemaValue(p.Arguments)
				if err != nil || !objectParams(p.Arguments) {
					reply = failure(req.id, -32602, "Invalid tool arguments")
					break
				}
				if tool.schema.Validate(args) != nil {
					reply = success(req.id, toolFailure(tool.tool, InvalidArguments))
					break
				}
				if len(calls) >= MaxInFlight {
					reply = failure(req.id, -32000, "Too many in-flight tool calls")
					break
				}
				callCtx, callCancel := context.WithCancel(ctx)
				call := &activeCall{cancel: callCancel}
				calls[req.key] = call
				workers.Go(func() {
					result := invoke(callCtx, tool.tool, p.Arguments)
					completed <- completion{req.key, call, success(req.id, result)}
				})
				continue
			default:
				reply = failure(req.id, -32601, "Method not found")
			}
			if err := write(reply); err != nil {
				return err
			}
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
