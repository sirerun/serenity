// Package gateway authenticates every MCP request before selecting a brain.
package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	hoststore "github.com/sirerun/serenity/internal/hosted/store"
	brainstore "github.com/sirerun/serenity/internal/store"
	"github.com/tiktoken-go/tokenizer"

	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/meter"
	"github.com/sirerun/serenity/internal/hosted/operation"
	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/server/mcp"
)

type entry struct {
	account string
	handler *mcp.HTTPHandler
	last    time.Time
}
type sessionBinding struct {
	credential string
	last       time.Time
}
type Gateway struct {
	admission    limiter
	Maintenance  sync.RWMutex
	accountLocks [64]sync.Mutex
	Issuer       *credential.Issuer
	Pool         *pool.Pool
	Meter        *meter.Meter
	Operations   *operation.Ledger
	mu           sync.Mutex
	handlers     map[string]*entry
	sessions     map[string]sessionBinding
	closed       bool
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !g.admission.allow("ip:"+clientIP(r), 600, time.Now()) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "Request rate exceeded", http.StatusTooManyRequests)
		return
	}
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	raw := strings.TrimPrefix(auth, "Bearer ")
	binding, err := g.Issuer.Verify(r.Context(), raw)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if !g.admission.allow("account:"+binding.AccountID, 120, time.Now()) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "Account request rate exceeded", http.StatusTooManyRequests)
		return
	}
	if r.Header.Get("Origin") != "" {
		http.Error(w, "Origin not allowed", http.StatusForbidden)
		return
	}
	session := r.Header.Get(mcp.SessionIDHeader)
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}
	if g.handlers == nil {
		g.handlers = map[string]*entry{}
		g.sessions = map[string]sessionBinding{}
	}
	for id, owner := range g.sessions {
		if time.Since(owner.last) > mcp.SessionIdleTimeout {
			delete(g.sessions, id)
		}
	}
	if session != "" && g.sessions[session].credential != binding.CredentialID {
		g.mu.Unlock()
		http.Error(w, "Unauthorized session", http.StatusUnauthorized)
		return
	}
	if session != "" {
		g.sessions[session] = sessionBinding{binding.CredentialID, time.Now()}
	}
	e := g.handlers[binding.CredentialID]
	if e == nil {
		for id, item := range g.handlers {
			var active int
			queryErr := g.Issuer.Store.DB().QueryRowContext(r.Context(), `SELECT count(*) FROM client_credentials WHERE id=? AND revoked_at IS NULL`, id).Scan(&active)
			if time.Since(item.last) > mcp.SessionIdleTimeout || (queryErr == nil && active == 0) {
				item.handler.Close()
				delete(g.handlers, id)
				for session, owner := range g.sessions {
					if owner.credential == id {
						delete(g.sessions, session)
					}
				}
			}
		}
		accountHandlers := 0
		for _, item := range g.handlers {
			if item.account == binding.AccountID {
				accountHandlers++
			}
		}
		if accountHandlers >= credential.MaxActivePerAccount {
			g.mu.Unlock()
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Account connection capacity reached", http.StatusTooManyRequests)
			return
		}
		if len(g.handlers) >= 128 {
			g.mu.Unlock()
			http.Error(w, "Connection capacity reached", http.StatusServiceUnavailable)
			return
		}
		runtime, release, openErr := g.Pool.Acquire(r.Context(), binding.BrainID)
		if openErr != nil {
			g.mu.Unlock()
			http.Error(w, "Memory temporarily unavailable", http.StatusServiceUnavailable)
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
			http.Error(w, "Memory unavailable", http.StatusServiceUnavailable)
			return
		}
		e = &entry{account: binding.AccountID, handler: mcp.NewHTTPHandlerWithConfig(server, mcp.HTTPConfig{MaxInFlightCalls: 8})}
		g.handlers[binding.CredentialID] = e
	}
	e.last = time.Now()
	g.mu.Unlock()
	recorder := &sessionWriter{ResponseWriter: w, onSession: func(id string) {
		g.mu.Lock()
		g.sessions[id] = sessionBinding{binding.CredentialID, time.Now()}
		g.mu.Unlock()
	}}
	var method string
	if r.Method == http.MethodPost {
		body, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if readErr != nil {
			http.Error(w, "Request too large", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		var request struct{ Method string }
		if json.Unmarshal(body, &request) == nil {
			method = request.Method
		}
	}
	e.handler.ServeHTTP(recorder, r)
	var listed struct {
		Result struct {
			Tools []json.RawMessage `json:"tools"`
		} `json:"result"`
	}
	if method == "tools/list" && recorder.status == 200 && json.Unmarshal(recorder.body.Bytes(), &listed) == nil && len(listed.Result.Tools) == 4 {
		_ = g.record(r.Context(), binding, "connected")
	}
	if r.Method == http.MethodDelete && session != "" {
		g.mu.Lock()
		delete(g.sessions, session)
		g.mu.Unlock()
	}
}

type sessionWriter struct {
	body bytes.Buffer
	http.ResponseWriter
	onSession func(string)
	wrote     bool
	status    int
}

func (w *sessionWriter) WriteHeader(status int) {
	if !w.wrote {
		w.wrote = true
		w.status = status
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
	if w.body.Len() < 65536 {
		_, _ = w.body.Write(p)
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
	return g.callBound(ctx, binding, name, args)
}

func (g *Gateway) callBound(ctx context.Context, binding credential.Binding, name string, args json.RawMessage) (result mcp.Result, err error) {
	g.Maintenance.RLock()
	defer g.Maintenance.RUnlock()
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
	if name == "remember" && !brainstore.ValidMemoryOperationKey(input.OperationKey) {
		return failure("invalid_params", time.Time{}), nil
	}
	if name == "remember" && len(input.Fact) > 4096 {
		return failure("input_too_large", time.Time{}), nil
	}
	accountHash := sha256.Sum256([]byte(binding.AccountID))
	lock := &g.accountLocks[int(accountHash[0])%len(g.accountLocks)]
	lock.Lock()
	defer lock.Unlock()
	if _, checkErr := g.Issuer.Store.BrainByID(ctx, binding.AccountID, binding.BrainID); checkErr != nil {
		return failure("scope_denied", time.Time{}), nil
	}
	runtime, release, err := g.Pool.Acquire(ctx, binding.BrainID)
	if err != nil {
		return failure("capacity", time.Time{}), nil
	}
	defer release()
	if name == "remember" || name == "forget" {
		runtime.Mutations.Lock()
		defer runtime.Mutations.Unlock()
	}
	var reservation meter.Reservation
	metered := name == "recall" || name == "read_memory_fact" || (name == "remember" && g.Operations == nil)
	if metered {
		entitlement, e := g.Meter.Entitlement(ctx, binding.AccountID)
		if e != nil {
			return result, e
		}
		metric, limit := "recalls", entitlement.Plan.Recalls
		if name == "remember" {
			metric, limit = "writes", entitlement.Plan.Writes
		}
		reservation, e = g.Meter.Reserve(ctx, binding.AccountID, metric, 1, limit, entitlement.Window, entitlement.ResetAt, operationKey(binding.BrainID, name, input.OperationKey))
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
	var operationRecord contracts.OperationRecord
	operationEntered := false
	operationReplay := false
	canonicalCommitted := false
	if name == "remember" {
		entitlement, e := g.Meter.Entitlement(ctx, binding.AccountID)
		if e != nil {
			return result, e
		}
		inventory, e := g.inventory(ctx, binding.AccountID, filepath.Dir(runtime.Root))
		if e != nil {
			return result, e
		}
		codec, e := tokenizer.Get(tokenizer.Cl100kBase)
		if e != nil {
			return result, e
		}
		count, e := codec.Count(input.Fact)
		if e != nil {
			return result, e
		}
		if g.Operations != nil {
			clientKey := input.OperationKey
			if clientKey == "" {
				clientKey = "hosted:" + hoststore.ID()
				var fields map[string]json.RawMessage
				if e = json.Unmarshal(args, &fields); e != nil {
					return failure("invalid_params", time.Time{}), nil
				}
				fields["operation_key"], e = json.Marshal(clientKey)
				if e != nil {
					return result, e
				}
				args, e = json.Marshal(fields)
				if e != nil {
					return result, e
				}
			}
			fingerprint, e := contracts.RequestFingerprint("remember", []contracts.FingerprintField{{Name: "brain", Value: []byte(binding.BrainID)}, {Name: "fact", Value: []byte(input.Fact)}})
			if e != nil {
				return result, e
			}
			operationRecord, e = g.Operations.Reserve(ctx, contracts.ReserveRequest{AccountID: binding.AccountID, BrainID: binding.BrainID, ClientKey: clientKey, Fingerprint: fingerprint, QuotaPeriod: entitlement.Window, Source: "gateway.remember", LeaseFor: 5 * time.Minute, Deltas: []contracts.ReserveDelta{{Metric: "writes", Units: 1, Limit: entitlement.Plan.Writes}, {Metric: "input_tokens", Units: int64(count), Limit: entitlement.Plan.InputTokens}}})
			if e != nil {
				if errors.Is(e, contracts.ErrOperationLimitExceeded) {
					return failure("limit_exceeded", entitlement.ResetAt), nil
				}
				if errors.Is(e, contracts.ErrOperationInProgress) || errors.Is(e, contracts.ErrOperationPendingReview) {
					return failure("operation_in_progress", time.Time{}), nil
				}
				return result, e
			}
			operationReplay = operationRecord.Phase == contracts.OperationCommitted
			if operationRecord.Phase == contracts.OperationReserved {
				defer func() {
					finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
					defer cancel()
					var final contracts.OperationPhase
					var evidence contracts.Evidence
					if operationEntered {
						if canonicalCommitted && !result.IsError {
							final = contracts.OperationCommitted
							evidence = contracts.Evidence{Kind: contracts.EvidenceCommitted, Ref: canonicalEvidenceRef(result, operationRecord.ID)}
						} else {
							final = contracts.OperationPendingReview
							evidence = contracts.Evidence{Kind: contracts.EvidenceUnknown, Ref: "gateway_outcome_unknown"}
						}
					} else {
						final = contracts.OperationReleased
						evidence = contracts.Evidence{Kind: contracts.EvidenceNoCanonicalAttempt}
					}
					_, finishErr := g.Operations.Finalize(finishCtx, operationRecord.ID, final, evidence)
					err = errors.Join(err, finishErr)
				}()
				if _, e = g.Operations.EnterCanonical(ctx, operationRecord.ID); e != nil {
					return result, e
				}
				operationEntered = true
			}
		}
		if !reservation.Replay && !operationReplay && (inventory.Memories >= entitlement.Plan.Memories || inventory.StorageBytes >= entitlement.Plan.StorageBytes) {
			return failure("limit_exceeded", entitlement.ResetAt), nil
		}
	}
	for _, tool := range runtime.Tools {
		if tool.Name == name {
			callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			result, err = tool.Handler(callCtx, args)
			if (name == "remember" || name == "forget") && err == nil && !result.IsError {
				// Acknowledged writes must already be in the canonical bundle,
				// even if the process dies before its next backup or shutdown.
				err = runtime.Flush()
				if err == nil && name == "remember" {
					canonicalCommitted = true
				}
			}
			if name == "remember" && err == nil && !result.IsError {
				err = g.record(ctx, binding, "memory_saved")
			}
			return result, err
		}
	}
	return failure("invalid_params", time.Time{}), nil
}

func canonicalEvidenceRef(result mcp.Result, operationID string) string {
	for _, content := range result.Content {
		if content.Type != "text" {
			continue
		}
		var body struct {
			ID string `json:"id"`
		}
		if json.Unmarshal([]byte(content.Text), &body) == nil && body.ID != "" {
			return "fact:" + body.ID
		}
	}
	return "gateway:" + operationID
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

func operationKey(brain, name, key string) string {
	if name == "remember" && key != "" {
		return brain + ":" + key
	}
	return ""
}

func (g *Gateway) record(ctx context.Context, b credential.Binding, action string) error {
	return g.Issuer.Store.Transaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO audit_log(account_id,actor,action,created_at,detail) VALUES(?,?,?,?,?)`, b.AccountID, "client", action, hoststore.Stamp(time.Now()), b.BrainID)
		return err
	})
}
func (g *Gateway) CallForAccount(ctx context.Context, account, brain, name string, args json.RawMessage) (mcp.Result, error) {
	if _, err := g.Issuer.Store.BrainByID(ctx, account, brain); err != nil {
		return mcp.Result{}, err
	}
	return g.callBound(ctx, credential.Binding{AccountID: account, BrainID: brain, Scopes: []string{"memory:read", "memory:write"}}, name, args)
}

// Inventory reports current physical storage and live facts across an account.
type Inventory struct{ Brains, Memories, StorageBytes int64 }

func (g *Gateway) Inventory(ctx context.Context, account, root string) (Inventory, error) {
	hash := sha256.Sum256([]byte(account))
	lock := &g.accountLocks[int(hash[0])%len(g.accountLocks)]
	lock.Lock()
	defer lock.Unlock()
	return g.inventory(ctx, account, root)
}
func (g *Gateway) inventory(ctx context.Context, account, root string) (out Inventory, err error) {
	brains, err := g.Issuer.Store.Brains(ctx, account)
	if err != nil {
		return out, err
	}
	out.Brains = int64(len(brains))
	for _, brain := range brains {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		if brain.State != "ready" {
			continue
		}
		if brain.ID != brain.PathKey || filepath.Base(brain.PathKey) != brain.PathKey {
			return out, errors.New("invalid stored brain path")
		}
		path := filepath.Join(root, brain.PathKey)
		projection, e := brainstore.LoadMemoryProjection(brainstore.NewSourceStore(path))
		if e != nil {
			return out, e
		}
		for _, fact := range projection.All() {
			if !fact.Expired(time.Now()) {
				out.Memories++
			}
		}
		err = filepath.WalkDir(path, func(_ string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !entry.IsDir() {
				info, e := entry.Info()
				if e != nil {
					return e
				}
				out.StorageBytes += info.Size()
			}
			return nil
		})
		if err != nil {
			return out, err
		}
	}
	return out, nil
}
