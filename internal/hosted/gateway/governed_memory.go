package gateway

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sirerun/serenity/internal/gitrun"
	brainstore "github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

type governedRequest struct {
	Schema    string         `json:"schema"`
	ProjectID string         `json:"project_id"`
	Action    governedAction `json:"action"`
}
type governedRef struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
}
type governedAction struct {
	Schema              string                        `json:"schema"`
	BrainID             string                        `json:"brain_id"`
	CommandID           string                        `json:"adapter_command_id"`
	Kind                string                        `json:"kind"`
	Text                string                        `json:"text,omitempty"`
	Sources             []brainstore.GovernedArtifact `json:"sources,omitempty"`
	WriterOwner         string                        `json:"writer_owner,omitempty"`
	MaximumCost         *brainstore.GovernedMoney     `json:"maximum_cost,omitempty"`
	AllowedDestinations []string                      `json:"allowed_provider_destinations,omitempty"`
	Query               string                        `json:"query,omitempty"`
	MinimumFreshness    time.Time                     `json:"minimum_freshness,omitempty"`
	MaxClaims           int                           `json:"max_claims,omitempty"`
	Classification      string                        `json:"classification,omitempty"`
	Claim               *governedRef                  `json:"claim,omitempty"`
	Reason              string                        `json:"reason,omitempty"`
	Removal             string                        `json:"removal,omitempty"`
}

func governedFailure(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"schema": brainstore.GovernedMemorySchema, "error": code}); err != nil {
		return
	}
}
func governedJSON(w http.ResponseWriter, r brainstore.GovernedResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(r); err != nil {
		return
	}
}
func decodeGoverned(raw []byte, dst any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}

// ServeGovernedMemory serves a single bounded request without an MCP handshake.
// Authentication, revocation, maintenance exclusion and account quotas remain
// the hosted gateway's own boundaries. No call invokes an embedding/model API.
func (g *Gateway) ServeGovernedMemory(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != "" {
		governedFailure(w, 403, "origin_denied")
		return
	}
	if !g.admission.allow("ip:"+clientIP(r), 600, time.Now()) {
		governedFailure(w, 429, "rate_limited")
		return
	}
	auth := r.Header.Get("Authorization")
	if len(auth) < 7 || !strings.EqualFold(auth[:7], "Bearer ") {
		governedFailure(w, 401, "unauthorized")
		return
	}
	rawToken := strings.TrimSpace(auth[7:])
	if g.Issuer == nil || g.Pool == nil || g.Meter == nil {
		governedFailure(w, 503, "unavailable")
		return
	}
	binding, err := g.Issuer.Verify(r.Context(), rawToken)
	if err != nil {
		governedFailure(w, 401, "unauthorized")
		return
	}
	if !g.admission.allow("account:"+binding.AccountID, 120, time.Now()) {
		governedFailure(w, 429, "rate_limited")
		return
	}
	var req governedRequest
	lookup := r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/memory/v1/commands/")
	if lookup {
		req.Schema = brainstore.GovernedMemorySchema
		req.ProjectID = r.URL.Query().Get("project_id")
		req.Action.BrainID = r.URL.Query().Get("brain_id")
		req.Action.CommandID = strings.TrimPrefix(r.URL.Path, "/memory/v1/commands/")
	} else if r.Method == http.MethodPost && r.URL.Path == "/memory/v1/commands" {
		typ, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if e != nil || typ != "application/json" {
			governedFailure(w, 415, "content_type")
			return
		}
		raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if e != nil {
			governedFailure(w, 413, "input_too_large")
			return
		}
		if !utf8.Valid(raw) || decodeGoverned(raw, &req) != nil {
			governedFailure(w, 400, "invalid_params")
			return
		}
	} else {
		governedFailure(w, 405, "method_not_allowed")
		return
	}
	a := req.Action
	if req.Schema != brainstore.GovernedMemorySchema || req.ProjectID != binding.BrainID || !brainstore.ValidGovernedUUID(a.BrainID) || !brainstore.ValidGovernedUUID(a.CommandID) {
		governedFailure(w, 403, "binding_mismatch")
		return
	}
	if a.WriterOwner != "" && a.WriterOwner != binding.AccountID {
		governedFailure(w, 403, "writer_owner_mismatch")
		return
	}
	required := "memory:read"
	if !lookup && (a.Kind == "remember" || a.Kind == "retract") {
		required = "memory:write"
	}
	hasScope := false
	for _, scope := range binding.Scopes {
		if scope == required {
			hasScope = true
		}
	}
	if !hasScope {
		governedFailure(w, 403, "scope_denied")
		return
	}
	if !lookup {
		if err := validateGovernedAction(a); err != nil {
			governedFailure(w, 400, err.Error())
			return
		}
	}
	// Match existing hosted lock order: maintenance, account, runtime, mutations.
	g.Maintenance.RLock()
	defer g.Maintenance.RUnlock()
	hash := sha256.Sum256([]byte(binding.AccountID))
	lock := &g.accountLocks[int(hash[0])%len(g.accountLocks)]
	lock.Lock()
	defer lock.Unlock()
	g.mu.Lock()
	closed := g.closed
	g.mu.Unlock()
	if closed {
		governedFailure(w, 503, "unavailable")
		return
	}
	// Recheck after waiting for locks: a queued request must not outrun revocation.
	current, e := g.Issuer.Verify(r.Context(), rawToken)
	if e != nil || current.CredentialID != binding.CredentialID || current.Generation != binding.Generation || current.BrainID != binding.BrainID {
		governedFailure(w, 401, "unauthorized")
		return
	}
	var active bool
	e = g.Issuer.Store.DB().QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM brains b JOIN accounts a ON a.id=b.account_id WHERE b.id=? AND b.account_id=? AND b.state='ready' AND a.status='active')", binding.BrainID, binding.AccountID).Scan(&active)
	if e != nil || !active {
		governedFailure(w, 403, "scope_denied")
		return
	}
	if lookup {
		if _, present, err := validateBrainTree(r.Context(), g.Pool.BrainsRoot(), binding.BrainID); err != nil || !present {
			governedFailure(w, 503, "canonical_unavailable")
			return
		}
		sources := brainstore.NewSourceStore(filepath.Join(g.Pool.BrainsRoot(), binding.BrainID))
		projection, err := brainstore.LoadGovernedMemory(sources)
		if err != nil {
			governedFailure(w, 503, "canonical_unavailable")
			return
		}
		if projection.BrainID != "" && projection.BrainID != a.BrainID {
			governedFailure(w, 403, "binding_mismatch")
			return
		}
		response := newGovernedResponse(req)
		published, err := governedPublished(r.Context(), sources.Root, projection)
		if err != nil {
			governedFailure(w, 503, "canonical_unavailable")
			return
		}
		if !published {
			response.Status = "unknown"
			response.LookupAuthoritative = false
			completeGovernedResponse(&response, projection, time.Now().UTC())
			governedJSON(w, response)
			return
		}
		if rec, ok := projection.Commands[a.CommandID]; ok {
			response = rec.Response
			if rec.Response.Kind != "retract" {
				metric := "recalls"
				if rec.Response.Kind == "remember" {
					metric = "writes"
				}
				settled, err := g.Meter.DurableCommitted(r.Context(), binding.AccountID, metric, "governed:"+binding.BrainID+":"+a.CommandID)
				if err != nil || !settled {
					response = newGovernedResponse(req)
					response.Status = "unknown"
					response.LookupAuthoritative = false
				}
			}
		} else {
			response.Status = "unknown"
			response.LookupAuthoritative = false
		}
		completeGovernedResponse(&response, projection, time.Now().UTC())
		governedJSON(w, response)
		return
	}
	runtime, release, e := g.Pool.AcquireCanonical(r.Context(), binding.BrainID)
	if e != nil {
		governedFailure(w, 503, "capacity")
		return
	}
	defer release()
	runtime.Mutations.Lock()
	defer runtime.Mutations.Unlock()
	projection, e := brainstore.LoadGovernedMemory(runtime.Sources)
	if e != nil {
		governedFailure(w, 503, "canonical_unavailable")
		return
	}
	if projection.BrainID != "" && projection.BrainID != a.BrainID {
		governedFailure(w, 403, "binding_mismatch")
		return
	}
	if a.WriterOwner != "" && projection.WriterOwner != "" && a.WriterOwner != projection.WriterOwner {
		governedFailure(w, 409, "writer_owner_conflict")
		return
	}
	canonical, e := json.Marshal(req)
	if e != nil {
		governedFailure(w, 400, "invalid_params")
		return
	}
	sum := sha256.Sum256(canonical)
	digest := hex.EncodeToString(sum[:])
	if old, ok := projection.Commands[a.CommandID]; ok {
		// Status is read-only and current authority is always required. Replays
		// never mutate, meter twice, or turn a retracted claim active again.
		if !lookup && old.RequestDigest != digest {
			governedFailure(w, 409, "operation_conflict")
			return
		}
		published, err := governedPublished(r.Context(), runtime.Root, projection)
		if err != nil {
			governedFailure(w, 503, "canonical_unavailable")
			return
		}
		if !published {
			// An explicit same-command POST may finish publication of the exact
			// already persisted receipt; status GET never writes or retries it.
			if err = writer.GovernedMemory(r.Context(), runtime.Queue, runtime.Sources, runtime.Root, old); err != nil {
				governedFailure(w, 503, "outcome_unknown")
				return
			}
			published, err = governedPublished(r.Context(), runtime.Root, projection)
			if err != nil || !published {
				governedFailure(w, 503, "outcome_unknown")
				return
			}
		}
		if old.Response.Kind != "retract" {
			metric := "recalls"
			if old.Response.Kind == "remember" {
				metric = "writes"
			}
			if err := g.Meter.CommitDurableOperation(r.Context(), binding.AccountID, metric, "governed:"+binding.BrainID+":"+a.CommandID); err != nil {
				governedFailure(w, 503, "outcome_unknown")
				return
			}
		}
		response := old.Response
		completeGovernedResponse(&response, projection, time.Now().UTC())
		governedJSON(w, response)
		return
	}
	if lookup {
		response := newGovernedResponse(req)
		response.Status = "unknown"
		response.Kind = ""
		response.LookupAuthoritative = false
		completeGovernedResponse(&response, projection, time.Now().UTC())
		governedJSON(w, response)
		return
	}
	published, e := governedPublished(r.Context(), runtime.Root, projection)
	if e != nil || !published {
		governedFailure(w, 503, "publication_pending")
		return
	}
	response := newGovernedResponse(req)
	now := time.Now().UTC()
	switch a.Kind {
	case "remember":
		id, e := governedUUID()
		if e != nil {
			governedFailure(w, 503, "unavailable")
			return
		}
		sources := a.Sources
		if sources == nil {
			sources = []brainstore.GovernedArtifact{}
		}
		response.Claims = []brainstore.GovernedClaim{{ID: id, Version: 1, BrainID: a.BrainID, Text: a.Text, Sources: sources, Freshness: now, Active: true}}
		response.WriterOwner = a.WriterOwner
	case "recall":
		// Canonical scan has no lagging derived index. Minimum freshness constrains
		// the read snapshot, never rewrites the observed time of an old statement.
		if a.MinimumFreshness.After(now) {
			governedFailure(w, 409, "freshness_unavailable")
			return
		}
		terms := strings.Fields(strings.ToLower(a.Query))
		for _, c := range projection.Claims {
			if !c.Active {
				continue
			}
			matched := a.Query == "*"
			for _, term := range terms {
				if strings.Contains(strings.ToLower(c.Text), term) {
					matched = true
				}
			}
			if matched {
				response.Claims = append(response.Claims, c)
			}
		}
		sort.Slice(response.Claims, func(i, j int) bool {
			if response.Claims[i].Freshness.Equal(response.Claims[j].Freshness) {
				return response.Claims[i].ID < response.Claims[j].ID
			}
			return response.Claims[i].Freshness.After(response.Claims[j].Freshness)
		})
		if len(response.Claims) > a.MaxClaims {
			response.Claims = response.Claims[:a.MaxClaims]
		}
	case "inspect", "retract":
		if a.Kind == "inspect" && a.MinimumFreshness.After(now) {
			governedFailure(w, 409, "freshness_unavailable")
			return
		}
		c, ok := projection.Claims[a.Claim.ID]
		if !ok {
			governedFailure(w, 404, "claim_unavailable")
			return
		}
		if c.Version != a.Claim.Version {
			governedFailure(w, 409, "claim_version_conflict")
			return
		}
		if a.Kind == "retract" {
			if !c.Active {
				governedFailure(w, 409, "claim_already_inactive")
				return
			}
			if c.Active {
				c.Version++
				c.Active = false
			}
			response.WriterOwner = a.WriterOwner
			removed, erased := true, false
			response.ActiveRecallRemoved = &removed
			response.HistoricalErasure = &erased
		}
		response.Claims = []brainstore.GovernedClaim{c}
	}
	entitlement, account, e := g.Meter.Subject(r.Context(), binding.AccountID, binding.PartnerID)
	if e != nil {
		governedFailure(w, 503, "meter_unavailable")
		return
	}
	metric, limit := "recalls", entitlement.Plan.Recalls
	if a.Kind == "remember" {
		metric, limit = "writes", entitlement.Plan.Writes
	}

	inventory, e := g.inventory(r.Context(), binding.AccountID, filepath.Dir(runtime.Root))
	if e != nil {
		governedFailure(w, 503, "inventory_unavailable")
		return
	}
	responseBytes, e := json.Marshal(response)
	if e != nil {
		governedFailure(w, 503, "unavailable")
		return
	}
	// Every admitted command persists its receipt, including reads. Bound the
	// request, response, source metadata and Git publication overhead together.
	growth := int64(len(canonical)+len(responseBytes))*4 + 4096
	if a.Kind != "retract" && inventory.StorageBytes+growth >= max(entitlement.Plan.StorageBytes, account.Plan.StorageBytes) {
		governedFailure(w, 429, "limit_exceeded")
		return
	}
	if a.Kind == "remember" && inventory.Memories >= max(entitlement.Plan.Memories, account.Plan.Memories) {
		governedFailure(w, 429, "limit_exceeded")
		return
	}

	if a.Kind != "retract" {
		_, e = g.Meter.ReserveDurable(r.Context(), binding.AccountID, metric, 1, limit, entitlement.Window, entitlement.ResetAt, "governed:"+binding.BrainID+":"+a.CommandID)
		if e != nil {
			governedFailure(w, 429, "limit_exceeded")
			return
		}
	}
	// Once admitted, complete under a bounded server context. A dropped client
	// does not imply absence; the caller can inspect this command after restart.
	commitCtx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 30*time.Second)
	defer cancel()
	rec := brainstore.GovernedRecord{Schema: brainstore.GovernedMemorySchema, RequestDigest: digest, Response: response, RecordedAt: now, Request: canonical}
	e = writer.GovernedMemory(commitCtx, runtime.Queue, runtime.Sources, runtime.Root, rec)
	if e == nil && a.Kind != "retract" {
		e = g.Meter.CommitDurableOperation(commitCtx, binding.AccountID, metric, "governed:"+binding.BrainID+":"+a.CommandID)
	}
	if e != nil {
		governedFailure(w, 503, "outcome_unknown")
		return
	}
	projection, e = brainstore.LoadGovernedMemory(runtime.Sources)
	if e != nil {
		governedFailure(w, 503, "outcome_unknown")
		return
	}
	completeGovernedResponse(&response, projection, now)
	governedJSON(w, response)
}

func newGovernedResponse(req governedRequest) brainstore.GovernedResponse {
	return brainstore.GovernedResponse{Schema: brainstore.GovernedMemorySchema, ProjectID: req.ProjectID, BrainID: req.Action.BrainID, CommandID: req.Action.CommandID, Kind: req.Action.Kind, Status: "completed", Claims: []brainstore.GovernedClaim{}, Revisions: []brainstore.GovernedRevision{}, Cost: brainstore.GovernedMoney{Currency: "USD", MicroUnits: 0}, Destinations: []string{}, LookupAuthoritative: true}
}
func completeGovernedResponse(response *brainstore.GovernedResponse, p *brainstore.GovernedProjection, observed time.Time) {
	// Return current claim state even when recovering an older write receipt.
	for i, c := range response.Claims {
		if latest, ok := p.Claims[c.ID]; ok {
			response.Claims[i] = latest
		}
	}
	response.Revisions = []brainstore.GovernedRevision{{BrainID: response.BrainID, Revision: p.Revision, Digest: p.Revision, IndexRevision: p.Revision, ObservedAt: observed}}
}
func governedUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
func validateGovernedAction(a governedAction) error {
	invalid := errors.New("invalid_params")
	if a.Schema != "zati.serenity.action/v1" {
		return invalid
	}
	switch a.Kind {
	case "remember":
		if strings.TrimSpace(a.Text) == "" || utf8.RuneCountInString(a.Text) > 8192 || a.WriterOwner == "" || len(a.WriterOwner) > 512 || len(a.Sources) > 256 {
			return invalid
		}
		for _, s := range a.Sources {
			if !brainstore.ValidGovernedUUID(s.ID) || !brainstore.ValidGovernedDigest(s.Digest) {
				return invalid
			}
		}
	case "recall":
		if strings.TrimSpace(a.Query) == "" || utf8.RuneCountInString(a.Query) > 8192 || a.MaxClaims < 1 || a.MaxClaims > 200 {
			return invalid
		}
		switch a.Classification {
		case "public", "internal", "restricted":
		default:
			return invalid
		}
	case "inspect", "retract":

		if a.Claim == nil || !brainstore.ValidGovernedUUID(a.Claim.ID) || a.Claim.Version < 1 {
			return invalid
		}
		if a.Kind == "retract" && (a.Reason == "" || len(a.Reason) > 8192 || a.WriterOwner == "" || len(a.WriterOwner) > 512 || a.Removal != "active_recall") {
			return invalid
		}
	default:
		return errors.New("capability_unsupported")
	}
	if a.Kind == "remember" || a.Kind == "recall" {
		if a.MaximumCost == nil || a.MaximumCost.Currency != "USD" || a.MaximumCost.MicroUnits < 0 {
			return invalid
		}
	}
	return nil
}

// governedPublished proves the canonical command sources are included in HEAD.
// A source fsync followed by a failed Git publication remains unknown on GET.
func governedPublished(ctx context.Context, root string, p *brainstore.GovernedProjection) (bool, error) {
	if len(p.SourceIDs) == 0 {
		return true, nil
	}
	out, err := gitrun.CanonicalReadOnly(root).Output(ctx, "ls-tree", "-r", "--name-only", "HEAD", "--", "brain/sources")
	if err != nil {
		return false, err
	}
	paths := map[string]bool{}
	for _, name := range strings.Split(string(out), "\n") {
		paths[name] = true
	}
	for _, sha := range p.SourceIDs {
		base := "brain/sources/" + sha[:2] + "/" + sha + "/"
		if !paths[base+"bytes"] || !paths[base+"meta.yaml"] {
			return false, nil
		}
	}
	return true, nil
}

// Zero UTC is a valid no-floor freshness request. Presence and type, rather
// than time.IsZero, distinguish it from an omitted or null requirement.
func (a *governedAction) UnmarshalJSON(raw []byte) error {
	type actionAlias governedAction
	var decoded actionAlias
	if err := decodeGoverned(raw, &decoded); err != nil {
		return err
	}
	if decoded.Kind == "recall" || decoded.Kind == "inspect" {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return err
		}
		freshness, ok := fields["minimum_freshness"]
		if !ok || string(bytes.TrimSpace(freshness)) == "null" {
			return errors.New("minimum_freshness is required")
		}
	}
	*a = governedAction(decoded)
	return nil
}
