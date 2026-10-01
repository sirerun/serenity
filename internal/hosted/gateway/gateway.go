// Package gateway authenticates every MCP request before selecting a brain.
package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	memoryserver "github.com/sirerun/serenity/internal/server/memory"
	writerpkg "github.com/sirerun/serenity/internal/writer"
)

type requestCredentialKey struct{}

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
	r = r.WithContext(context.WithValue(r.Context(), requestCredentialKey{}, raw))
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
			if strings.HasPrefix(id, "oauth:") && g.Issuer.OAuthActive != nil {
				var ok bool
				ok, queryErr = g.Issuer.OAuthActive(r.Context(), strings.TrimPrefix(id, "oauth:"))
				if ok {
					active = 1
				} else {
					active = 0
				}
			}
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
				// A refreshed access token keeps the grant/session identity, but
				// must use the current request token, never a captured old token.
				current, _ := ctx.Value(requestCredentialKey{}).(string)
				return g.call(ctx, current, name, args)
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
	return failureBody(code, reset, "")
}

// limitFailure is limit_exceeded with the meter bucket that refused the call:
// "account" for the account's plan, "partner" for a partner bucket (ADR 023).
func limitFailure(reset time.Time, bucket string) mcp.Result {
	if bucket == "" {
		bucket = meter.BucketAccount
	}
	return failureBody("limit_exceeded", reset, bucket)
}

func failureBody(code string, reset time.Time, bucket string) mcp.Result {
	body := map[string]any{"protocol_version": 1, "error": code, "message": code, "suggestion": "Review your connection and account limits."}
	if !reset.IsZero() {
		body["reset_at"] = reset.UTC().Format(time.RFC3339)
		body["upgrade_url"] = "/billing"
	}
	if bucket != "" {
		body["bucket"] = bucket
	}
	b, _ := json.Marshal(body)
	return mcp.Result{IsError: true, Content: []mcp.Content{{Type: "text", Text: string(b)}}}
}

func rememberValidationFailure(validation memoryserver.VerbError) (mcp.Result, error) {
	body, err := json.Marshal(validation)
	if err != nil {
		return mcp.Result{}, fmt.Errorf("encode remember validation failure: %w", err)
	}
	return mcp.Result{
		IsError: true,
		Content: []mcp.Content{{Type: "text", Text: string(body)}},
	}, nil
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
		// The subject's window encodes its bucket, so account and partner
		// usage never share a counter even though both rows belong to the
		// same account (ADR 023).
		entitlement, _, e := g.Meter.Subject(ctx, binding.AccountID, binding.PartnerID)
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
				return limitFailure(limitErr.ResetAt, entitlement.Bucket), nil
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
	durableFactID := ""
	if name == "remember" {
		entitlement, accountEntitlement, e := g.Meter.Subject(ctx, binding.AccountID, binding.PartnerID)
		if e != nil {
			return result, e
		}
		// Inventory is physically account-wide. A partner-pro call may fill
		// the brain up to the larger of the account plan and the partner plan;
		// every other call is checked against the account plan (ADR 023).
		memoryLimit := max(entitlement.Plan.Memories, accountEntitlement.Plan.Memories)
		storageLimit := max(entitlement.Plan.StorageBytes, accountEntitlement.Plan.StorageBytes)
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
			normalized, validation, invalid := memoryserver.NormalizeRememberRequest(args, time.Now())
			if invalid {
				return rememberValidationFailure(validation)
			}
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
				if normalized.ValidUntil != nil {
					fields["ttl"], e = json.Marshal(normalized.ValidUntil.UTC().Format(time.RFC3339Nano))
					if e != nil {
						return result, e
					}
				}
				args, e = json.Marshal(fields)
				if e != nil {
					return result, e
				}
			}
			ttl := ""
			if normalized.ValidUntil != nil {
				ttl = normalized.ValidUntil.UTC().Format(time.RFC3339Nano)
			}
			fingerprintFields := []contracts.FingerprintField{
				{Name: "brain", Value: []byte(binding.BrainID)},
				{Name: "fact", Value: []byte(normalized.Fact)},
				{Name: "provenance", Value: []byte(normalized.Provenance)},
				{Name: "visibility", Value: []byte(normalized.Visibility)},
				{Name: "ttl", Value: []byte(ttl)},
				{Name: "entity_type", Value: []byte(normalized.EntityType)},
				{Name: "entity_slug", Value: []byte(normalized.EntitySlug)},
				{Name: "kind", Value: []byte(normalized.Kind)},
			}
			fingerprint, e := contracts.RequestFingerprint("remember", fingerprintFields)
			if e != nil {
				return result, e
			}
			operationRecord, e = g.Operations.Reserve(ctx, contracts.ReserveRequest{AccountID: binding.AccountID, BrainID: binding.BrainID, ClientKey: clientKey, Fingerprint: fingerprint, QuotaPeriod: entitlement.Window, Source: "gateway.remember", LeaseFor: 5 * time.Minute, Deltas: []contracts.ReserveDelta{{Metric: "writes", Units: 1, Limit: entitlement.Plan.Writes}, {Metric: "input_tokens", Units: int64(count), Limit: entitlement.Plan.InputTokens}}})
			if e != nil {
				if errors.Is(e, contracts.ErrOperationKeyReuse) {
					return rememberValidationFailure(memoryserver.RememberOperationConflict())
				}
				if errors.Is(e, contracts.ErrOperationLimitExceeded) {
					return limitFailure(entitlement.ResetAt, entitlement.Bucket), nil
				}
				if errors.Is(e, contracts.ErrOperationInProgress) || errors.Is(e, contracts.ErrOperationPendingReview) {
					return failure("operation_in_progress", time.Time{}), nil
				}
				return result, e
			}
			operationReplay = operationRecord.Phase == contracts.OperationCommitted
			if operationReplay {
				// A committed operation is authoritative. Re-running the tool would
				// re-enter provider/index work and could diverge from the original
				// outcome. Return the stored canonical fact identity instead.
				return replayRememberResult(operationRecord), nil
			}
			if operationRecord.Phase == contracts.OperationReserved {
				defer func() {
					finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
					defer cancel()
					final, evidence := rememberFinalizeOutcome(operationEntered, durableFactID)
					_, finishErr := g.Operations.Finalize(finishCtx, operationRecord.ID, final, evidence)
					err = errors.Join(err, finishErr)
				}()
			}
			// The client key belongs to the ledger's account/brain/period scope.
			// The canonical writer has a brain-wide key namespace, so use this
			// operation's stable internal ID rather than forwarding the raw key.
			var canonicalArgs map[string]json.RawMessage
			if e = json.Unmarshal(args, &canonicalArgs); e != nil {
				return result, e
			}
			canonicalArgs["operation_key"], e = json.Marshal(operationRecord.ID)
			if e != nil {
				return result, e
			}
			args, e = json.Marshal(canonicalArgs)
			if e != nil {
				return result, e
			}
		}
		if !reservation.Replay && (inventory.Memories >= memoryLimit || inventory.StorageBytes >= storageLimit) {
			if !operationReplay {
				return limitFailure(entitlement.ResetAt, entitlement.Bucket), nil
			}
		}
	}
	for _, tool := range runtime.Tools {
		if tool.Name == name {
			callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			if name == "remember" && operationRecord.ID != "" && g.Operations != nil {
				callCtx = writerpkg.WithCanonicalOperation(callCtx, writerpkg.CanonicalOperation{
					ID: operationRecord.ID,
					BeforeCommit: func(commitCtx context.Context, operationID string) error {
						if operationID != operationRecord.ID {
							return errors.New("hosted: canonical operation identity changed")
						}
						if _, enterErr := g.Operations.EnterCanonical(commitCtx, operationRecord.ID); enterErr != nil {
							return enterErr
						}
						operationEntered = true
						return nil
					},
					AfterFlush: func(_ context.Context, operationID, factID string) {
						if operationID == operationRecord.ID && factID != "" {
							durableFactID = factID
						}
					},
				})
			}
			result, err = tool.Handler(callCtx, args)
			if (name == "remember" || name == "forget") && err == nil && !result.IsError {
				// Remember's source has already been committed inline, before its
				// handler performs any optional index/provider work. This flush only
				// publishes separately touched state such as the search cache.
				err = runtime.Flush()
			}
			if name == "remember" && err == nil && !result.IsError {
				err = g.record(ctx, binding, "memory_saved")
			}
			return result, err
		}
	}
	return failure("invalid_params", time.Time{}), nil
}

func rememberFinalizeOutcome(operationEntered bool, durableFactID string) (contracts.OperationPhase, contracts.Evidence) {
	if durableFactID != "" {
		return contracts.OperationCommitted, contracts.Evidence{Kind: contracts.EvidenceCommitted, Ref: "fact:" + durableFactID}
	}
	if operationEntered {
		return contracts.OperationPendingReview, contracts.Evidence{Kind: contracts.EvidenceUnknown, Ref: "gateway_outcome_unknown"}
	}
	return contracts.OperationReleased, contracts.Evidence{Kind: contracts.EvidenceNoCanonicalAttempt}
}

func replayRememberResult(record contracts.OperationRecord) mcp.Result {
	id := strings.TrimPrefix(record.Evidence.Ref, "fact:")
	if id == "" || id == record.Evidence.Ref {
		return failure("operation_replay_unavailable", time.Time{})
	}
	body, _ := json.Marshal(map[string]any{
		"protocol_version": 1,
		"id":               id,
		"status":           "duplicate",
		"status_text":      "already committed this operation",
		"search_state":     "unchanged",
		"entity_slug":      nil,
		"valid_until":      nil,
	})
	return mcp.Result{Content: []mcp.Content{{Type: "text", Text: string(body)}}}
}

// Call uses the same authorization and quota path for dashboard memory actions.
func (g *Gateway) Call(ctx context.Context, raw, name string, args json.RawMessage) (mcp.Result, error) {
	// A refreshed access token keeps the grant/session identity, but
	// must use the current request token, never a captured old token.
	current, _ := ctx.Value(requestCredentialKey{}).(string)
	return g.call(ctx, current, name, args)
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
