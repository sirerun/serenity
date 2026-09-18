// Package gateway authenticates every MCP request before selecting a brain.
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/meter"
	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/server/mcp"
)

type entry struct {
	handler *mcp.HTTPHandler
	last    time.Time
}
type Gateway struct {
	Issuer   *credential.Issuer
	Pool     *pool.Pool
	Meter    *meter.Meter
	mu       sync.Mutex
	handlers map[string]*entry
	sessions map[string]string
	closed   bool
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		http.Error(w, "Unauthorized", 401)
		return
	}
	raw := strings.TrimPrefix(auth, "Bearer ")
	binding, err := g.Issuer.Verify(r.Context(), raw)
	if err != nil {
		http.Error(w, "Unauthorized", 401)
		return
	}
	if r.Header.Get("Origin") != "" {
		http.Error(w, "Origin not allowed", 403)
		return
	}
	session := r.Header.Get(mcp.SessionIDHeader)
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		http.Error(w, "Service unavailable", 503)
		return
	}
	if g.handlers == nil {
		g.handlers = map[string]*entry{}
		g.sessions = map[string]string{}
	}
	if session != "" && g.sessions[session] != binding.CredentialID {
		g.mu.Unlock()
		http.Error(w, "Unauthorized session", 401)
		return
	}
	e := g.handlers[binding.CredentialID]
	if e == nil {
		if len(g.handlers) >= 128 {
			g.mu.Unlock()
			http.Error(w, "Connection capacity reached", 503)
			return
		}
		runtime, release, openErr := g.Pool.Acquire(r.Context(), binding.BrainID)
		if openErr != nil {
			g.mu.Unlock()
			http.Error(w, "Memory temporarily unavailable", 503)
			return
		}
		tools := make([]mcp.Tool, len(runtime.Tools))
		copy(tools, runtime.Tools)
		release()
		for idx := range tools {
			name := tools[idx].Name
			tools[idx].Handler = func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
				return g.call(ctx, raw, name, args)
			}
		}
		server, newErr := mcp.New("hosted-v1", tools)
		if newErr != nil {
			g.mu.Unlock()
			http.Error(w, "Memory unavailable", 503)
			return
		}
		e = &entry{handler: mcp.NewHTTPHandlerWithConfig(server, mcp.HTTPConfig{MaxInFlightCalls: 8})}
		g.handlers[binding.CredentialID] = e
	}
	e.last = time.Now()
	g.mu.Unlock()
	recorder := &sessionWriter{ResponseWriter: w, onSession: func(id string) { g.mu.Lock(); g.sessions[id] = binding.CredentialID; g.mu.Unlock() }}
	e.handler.ServeHTTP(recorder, r)
	if r.Method == http.MethodDelete && session != "" {
		g.mu.Lock()
		delete(g.sessions, session)
		g.mu.Unlock()
	}
}

type sessionWriter struct {
	http.ResponseWriter
	onSession func(string)
	wrote     bool
}

func (w *sessionWriter) WriteHeader(status int) {
	if !w.wrote {
		w.wrote = true
		if status >= 200 && status < 300 {
			if id := w.Header().Get(mcp.SessionIDHeader); id != "" {
				w.onSession(id)
			}
		}
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *sessionWriter) Write(p []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(p)
}
func (w *sessionWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func failure(code string, reset time.Time) mcp.Result {
	body := map[string]any{"protocol_version": 1, "error": code, "message": code, "suggestion": "Review your connection and account limits."}
	if !reset.IsZero() {
		body["reset_at"] = reset.UTC().Format(time.RFC3339)
		body["upgrade_url"] = "/billing"
	}
	b, _ := json.Marshal(body)
	return mcp.Result{IsError: true, Content: []mcp.Content{{Type: "text", Text: string(b)}}}
}
func (g *Gateway) call(ctx context.Context, raw, name string, args json.RawMessage) (result mcp.Result, err error) {
	binding, err := g.Issuer.Verify(ctx, raw)
	if err != nil {
		return failure("scope_denied", time.Time{}), nil
	}
	required := "memory:read"
	if name == "remember" || name == "forget" {
		required = "memory:write"
	}
	allowed := false
	for _, s := range binding.Scopes {
		if s == required {
			allowed = true
		}
	}
	if !allowed {
		return failure("scope_denied", time.Time{}), nil
	}
	var input struct {
		Fact         string `json:"fact"`
		OperationKey string `json:"operation_key"`
	}
	if err = json.Unmarshal(args, &input); err != nil {
		return failure("invalid_params", time.Time{}), nil
	}
	if name == "remember" && len(input.Fact) > 4096 {
		return failure("input_too_large", time.Time{}), nil
	}
	runtime, release, err := g.Pool.Acquire(ctx, binding.BrainID)
	if err != nil {
		return failure("capacity", time.Time{}), nil
	}
	defer release()
	var reservation meter.Reservation
	metered := name == "recall" || name == "read_memory_fact" || name == "remember"
	if metered {
		entitlement, e := g.Meter.Entitlement(ctx, binding.AccountID)
		if e != nil {
			return result, e
		}
		metric, limit := "recalls", entitlement.Plan.Recalls
		if name == "remember" {
			metric, limit = "writes", entitlement.Plan.Writes
		}
		reservation, e = g.Meter.Reserve(ctx, binding.AccountID, metric, 1, limit, entitlement.Window, entitlement.ResetAt, input.OperationKey)
		if e != nil {
			var limitErr *meter.LimitError
			if errors.As(e, &limitErr) {
				return failure("limit_exceeded", limitErr.ResetAt), nil
			}
			if errors.Is(e, meter.ErrInProgress) {
				return failure("operation_in_progress", time.Time{}), nil
			}
			return result, e
		}
		defer func() {
			finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			err = errors.Join(err, g.Meter.Finish(finishCtx, reservation, err == nil && !result.IsError))
		}()
	}
	for _, tool := range runtime.Tools {
		if tool.Name == name {
			return tool.Handler(ctx, args)
		}
	}
	return failure("invalid_params", time.Time{}), nil
}

// Call uses the same authorization and quota path for dashboard memory actions.
func (g *Gateway) Call(ctx context.Context, raw, name string, args json.RawMessage) (mcp.Result, error) {
	return g.call(ctx, raw, name, args)
}
func (g *Gateway) Close() {
	g.mu.Lock()
	g.closed = true
	entries := g.handlers
	g.mu.Unlock()
	for _, e := range entries {
		e.handler.Close()
	}
}
