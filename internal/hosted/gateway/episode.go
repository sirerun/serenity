package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sirerun/serenity/internal/episodejson"
	"github.com/sirerun/serenity/internal/hosted/meter"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

type episodeRequest struct {
	Schema         string              `json:"schema"`
	ProjectID      string              `json:"project_id"`
	Operation      string              `json:"operation"`
	Qualification  string              `json:"qualification"`
	Generation     *uint64             `json:"generation,omitempty"`
	Mutation       *store.EpisodeInput `json:"mutation,omitempty"`
	CommandKey     string              `json:"command_key,omitempty"`
	EventKey       string              `json:"event_key,omitempty"`
	Query          string              `json:"query,omitempty"`
	ProducerID     string              `json:"producer_id,omitempty"`
	ConversationID string              `json:"conversation_id,omitempty"`
	WorkspaceLabel string              `json:"workspace_label,omitempty"`
	From           *time.Time          `json:"from,omitempty"`
	Until          *time.Time          `json:"until,omitempty"`
	Limit          int                 `json:"limit,omitempty"`
	MaxBytes       int                 `json:"max_bytes,omitempty"`
	Offset         int                 `json:"offset,omitempty"`
	Revision       string              `json:"revision,omitempty"`
}
type episodeResponse struct {
	Capture        *store.EpisodeCapture        `json:"capture,omitempty"`
	Schema         string                       `json:"schema"`
	ProjectID      string                       `json:"project_id"`
	Status         string                       `json:"status"`
	Generation     uint64                       `json:"generation"`
	Revision       string                       `json:"revision"`
	Receipt        *store.EpisodeReceipt        `json:"receipt,omitempty"`
	Events         []store.ArchivedEpisodeEvent `json:"events,omitempty"`
	Checkpoint     *store.EpisodeCheckpoint     `json:"checkpoint,omitempty"`
	Fences         map[string]uint64            `json:"fences"`
	Capabilities   map[string]any               `json:"capabilities,omitempty"`
	NextOffset     *int                         `json:"next_offset,omitempty"`
	SelectedBytes  int                          `json:"selected_bytes"`
	Omissions      []string                     `json:"omissions"`
	PhysicalPurge  string                       `json:"physical_purge"`
	ManagedRestore string                       `json:"managed_restore"`
	Authority      string                       `json:"authority"`
}

func episodeJSON(w http.ResponseWriter, status int, v any) {
	raw, err := json.Marshal(v)
	if err != nil || len(raw) > store.EpisodeMaxResponseBytes {
		status = 500
		raw = []byte(`{"schema":"serenity.episodes/v1","error":"response_limit"}`)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}
func episodeFailure(w http.ResponseWriter, status int, code string) {
	episodeJSON(w, status, map[string]string{"schema": store.EpisodeSchema, "error": code})
}
func episodeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrEpisodeInvalid):
		episodeFailure(w, 400, "invalid_params")
	case errors.Is(err, store.ErrEpisodeConflict):
		episodeFailure(w, 409, "identity_conflict")
	case errors.Is(err, store.ErrEpisodeFenced):
		episodeFailure(w, 410, "excluded")
	case errors.Is(err, store.ErrEpisodeGeneration):
		episodeFailure(w, 409, "generation_mismatch")
	case errors.Is(err, store.ErrEpisodeLimit):
		episodeFailure(w, 429, "archive_limit_exceeded")
	default:
		episodeFailure(w, 503, "outcome_unknown")
	}
}
func episodeRevision(p *store.EpisodeProjection) string {
	ids := make([]string, 0, len(p.SourceIDs))
	for _, id := range p.SourceIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return store.EpisodeHash(ids)
}
func episodeCapabilities() map[string]any {
	return map[string]any{"synthetic_only": true, "real_history_capture": false, "append": true, "exact_read": true, "attachments": "inline_bounded", "search": "direct_lexical_scan", "context": true, "checkpoint": "client_supplied_evidence", "immediate_exclusion": true, "physical_purge": false, "managed_restore": false, "portable_export": false, "automatic_extraction": false, "model_calls": false, "max_request_bytes": store.EpisodeMaxRequestBytes, "max_record_bytes": store.EpisodeMaxRecordBytes, "max_events": store.EpisodeMaxEvents, "max_event_text_bytes": store.EpisodeMaxTextBytes, "max_event_json_bytes": store.EpisodeMaxEventJSONBytes, "max_attachment_bytes": store.EpisodeMaxAttachmentBytes, "max_attachments_per_event": 8, "max_attachments_per_batch": 16, "max_records": store.EpisodeMaxRecords, "max_fences": store.EpisodeMaxFences, "max_archive_bytes": store.EpisodeMaxArchiveBytes, "max_response_bytes": store.EpisodeMaxResponseBytes, "max_results": 64, "max_scan_entries": 65536, "context_estimator": "serialized_json_bytes", "native_restore": false}
}
func validateEpisodeEnvelope(raw []byte, req episodeRequest) error {
	allowed := map[string]bool{"schema": true, "project_id": true, "operation": true, "qualification": true}
	fieldsByOperation := map[string][]string{
		"append": {"generation", "mutation"}, "checkpoint": {"generation", "mutation"}, "exclude": {"generation", "mutation"},
		"status": {"command_key"}, "read": {"command_key", "event_key", "max_bytes"},
		"search":  {"query", "producer_id", "conversation_id", "workspace_label", "from", "until", "limit", "max_bytes", "offset", "revision"},
		"context": {"query", "producer_id", "conversation_id", "workspace_label", "from", "until", "limit", "max_bytes", "offset", "revision"},
	}
	for _, key := range fieldsByOperation[req.Operation] {
		allowed[key] = true
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	for key, value := range fields {
		if !allowed[key] || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return store.ErrEpisodeInvalid
		}
	}
	return nil
}

func validateEpisodeRequest(r episodeRequest) error {
	if r.Schema != store.EpisodeSchema || r.Qualification != "synthetic" || !store.ValidEpisodeID(r.ProjectID) {
		return store.ErrEpisodeInvalid
	}
	mutation := r.Operation == "append" || r.Operation == "checkpoint" || r.Operation == "exclude"
	if mutation {
		if r.Mutation == nil || r.Generation == nil || r.Mutation.Operation != r.Operation {
			return store.ErrEpisodeInvalid
		}
		if err := store.ValidateEpisodeInput(*r.Mutation); err != nil {
			return err
		}
	} else if r.Mutation != nil || r.Generation != nil {
		return store.ErrEpisodeInvalid
	}
	if len(r.Query) > 512 || !utf8.ValidString(r.Query) || len(r.WorkspaceLabel) > 128 || !utf8.ValidString(r.WorkspaceLabel) || r.Limit < 0 || r.Limit > 64 || r.MaxBytes < 0 || r.MaxBytes > store.EpisodeMaxResponseBytes-32768 || r.Offset < 0 || r.Offset > store.EpisodeMaxRecords*store.EpisodeMaxEvents {
		return store.ErrEpisodeInvalid
	}
	if r.ProducerID != "" && !store.ValidEpisodeID(r.ProducerID) || r.ConversationID != "" && !store.ValidEpisodeID(r.ConversationID) {
		return store.ErrEpisodeInvalid
	}
	if r.From != nil && r.Until != nil && !r.From.Before(*r.Until) {
		return store.ErrEpisodeInvalid
	}
	if r.Offset > 0 && !store.ValidSourceSHA(r.Revision) {
		return store.ErrEpisodeInvalid
	}
	selector := r.CommandKey != "" || r.EventKey != ""
	query := r.Query != "" || r.ProducerID != "" || r.ConversationID != "" || r.WorkspaceLabel != "" || r.From != nil || r.Until != nil || r.Offset != 0 || r.Revision != ""
	switch r.Operation {
	case "append", "checkpoint", "exclude", "capabilities", "fences":
		if selector || query || r.Limit != 0 || r.MaxBytes != 0 {
			return store.ErrEpisodeInvalid
		}
	case "status":
		if !store.ValidSourceSHA(r.CommandKey) || r.EventKey != "" || query || r.Limit != 0 || r.MaxBytes != 0 {
			return store.ErrEpisodeInvalid
		}
	case "read":
		if (r.CommandKey == "") == (r.EventKey == "") || r.CommandKey != "" && !store.ValidSourceSHA(r.CommandKey) || r.EventKey != "" && !store.ValidSourceSHA(r.EventKey) || query || r.Limit != 0 {
			return store.ErrEpisodeInvalid
		}
	case "search", "context":
		if selector {
			return store.ErrEpisodeInvalid
		}
	default:
		return store.ErrEpisodeInvalid
	}
	return nil
}

// ServeEpisodes is deliberately separate from both frozen v1 memory APIs.
// Enabling it requires an explicit synthetic brain allowlist; no production
// history, physical erasure, or managed restore qualification is implied.
func (g *Gateway) ServeEpisodes(w http.ResponseWriter, r *http.Request) {
	if !g.EpisodeArchiveEnabled {
		episodeFailure(w, 404, "archive_disabled")
		return
	}
	if r.URL.Path != "/episodes/v1" || r.Method != http.MethodPost {
		episodeFailure(w, 405, "method_not_allowed")
		return
	}
	if r.Header.Get("Origin") != "" {
		episodeFailure(w, 403, "origin_denied")
		return
	}
	if r.URL.RawQuery != "" {
		episodeFailure(w, 400, "invalid_params")
		return
	}
	if !g.admission.allow("ip:"+clientIP(r), 600, time.Now()) {
		episodeFailure(w, 429, "rate_limited")
		return
	}
	auth := r.Header.Get("Authorization")
	if len(auth) < 7 || !strings.EqualFold(auth[:7], "Bearer ") {
		episodeFailure(w, 401, "unauthorized")
		return
	}
	token := strings.TrimSpace(auth[7:])
	if g.Issuer == nil || g.Pool == nil || g.Meter == nil {
		episodeFailure(w, 503, "unavailable")
		return
	}
	binding, err := g.Issuer.Verify(r.Context(), token)
	if err != nil {
		episodeFailure(w, 401, "unauthorized")
		return
	}
	if !g.admission.allow("account:"+binding.AccountID, 120, time.Now()) {
		episodeFailure(w, 429, "rate_limited")
		return
	}
	allowed := false
	for _, brain := range g.EpisodeSyntheticBrains {
		if brain == binding.BrainID {
			allowed = true
		}
	}
	if !allowed {
		episodeFailure(w, 403, "synthetic_brain_required")
		return
	}
	typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typ != "application/json" {
		episodeFailure(w, 415, "content_type")
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, store.EpisodeMaxRequestBytes))
	if err != nil {
		episodeFailure(w, 413, "input_too_large")
		return
	}
	var req episodeRequest
	if !episodejson.Valid(raw) || decodeGoverned(raw, &req) != nil {
		episodeFailure(w, 400, "invalid_params")
		return
	}
	if req.ProjectID != binding.BrainID {
		episodeFailure(w, 403, "binding_mismatch")
		return
	}
	if err = validateEpisodeEnvelope(raw, req); err != nil {
		episodeError(w, err)
		return
	}
	if err = validateEpisodeRequest(req); err != nil {
		episodeError(w, err)
		return
	}
	mutation := req.Mutation != nil
	scope := "memory:read"
	if mutation {
		scope = "memory:write"
	}
	hasScope := false
	for _, s := range binding.Scopes {
		if s == scope {
			hasScope = true
		}
	}
	if !hasScope {
		episodeFailure(w, 403, "scope_denied")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	g.Maintenance.RLock()
	defer g.Maintenance.RUnlock()
	h := sha256.Sum256([]byte(binding.AccountID))
	lock := &g.accountLocks[int(h[0])%len(g.accountLocks)]
	lock.Lock()
	defer lock.Unlock()
	g.mu.Lock()
	closed := g.closed
	g.mu.Unlock()
	if closed {
		episodeFailure(w, 503, "unavailable")
		return
	}
	recheck := func(ctx context.Context) error {
		current, e := g.Issuer.Verify(ctx, token)
		if e != nil {
			return e
		}
		if current.CredentialID != binding.CredentialID || current.Generation != binding.Generation || current.BrainID != binding.BrainID || current.AccountID != binding.AccountID {
			return errors.New("revoked")
		}
		found := false
		for _, s := range current.Scopes {
			if s == scope {
				found = true
			}
		}
		if !found {
			return errors.New("scope denied")
		}
		var active bool
		e = g.Issuer.Store.DB().QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM brains b JOIN accounts a ON a.id=b.account_id WHERE b.id=? AND b.account_id=? AND b.state='ready' AND a.status='active')", binding.BrainID, binding.AccountID).Scan(&active)
		if e != nil {
			return e
		}
		if !active {
			return errors.New("inactive")
		}
		return nil
	}
	if err = recheck(ctx); err != nil {
		episodeFailure(w, 401, "unauthorized")
		return
	}
	runtime, release, err := g.Pool.AcquireCanonical(ctx, binding.BrainID)
	if err != nil {
		episodeFailure(w, 503, "capacity")
		return
	}
	defer release()
	runtime.Mutations.Lock()
	defer runtime.Mutations.Unlock()
	p, err := store.LoadEpisodes(ctx, runtime.Sources)
	if err != nil {
		episodeError(w, err)
		return
	}
	if p.BrainID != "" && p.BrainID != binding.BrainID {
		episodeFailure(w, 403, "binding_mismatch")
		return
	}
	response := episodeResponse{Schema: store.EpisodeSchema, ProjectID: binding.BrainID, Status: "completed", Generation: p.Generation, Revision: episodeRevision(p), Omissions: []string{"host_coverage_not_verified"}, PhysicalPurge: "unsupported", ManagedRestore: "unsupported", Authority: "attributed_evidence_not_execution_authority"}
	var readReservation *meter.Reservation
	defer func() {
		if readReservation != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = g.Meter.Finish(cleanup, *readReservation, false)
		}
	}()
	if req.Operation == "read" || req.Operation == "search" || req.Operation == "context" {
		entitlement, _, e := g.Meter.Subject(ctx, binding.AccountID, binding.PartnerID)
		if e != nil {
			episodeFailure(w, 503, "meter_unavailable")
			return
		}
		reservation, e := g.Meter.Reserve(ctx, binding.AccountID, "recalls", 1, entitlement.Plan.Recalls, entitlement.Window, entitlement.ResetAt, "")
		if e != nil {
			episodeFailure(w, 429, "limit_exceeded")
			return
		}
		readReservation = &reservation
	}
	if mutation {
		key := store.EpisodeCommandKey(req.Mutation.ProducerID, req.Mutation.CommandID)
		old, exists := p.Records[key]
		if *req.Generation != p.Generation {
			episodeError(w, store.ErrEpisodeGeneration)
			return
		}
		if exists && old.RequestDigest != store.EpisodeHash(*req.Mutation) {
			episodeError(w, store.ErrEpisodeConflict)
			return
		}
		if p.Fences[store.EpisodeConversationKey(req.Mutation.ProducerID, req.Mutation.ConversationID)] != 0 && (!exists || old.Kind != "exclude") {
			episodeError(w, store.ErrEpisodeFenced)
			return
		}
		// Retry may finish its own failed publication. Other unpublished records
		// remain a closed boundary, never an implicit acknowledgment.
		if !exists {
			if err = writer.EpisodesPublished(ctx, runtime.Root, p); err != nil {
				episodeFailure(w, 503, "publication_pending")
				return
			}
		}
		meterKey := "governed:episode:" + binding.BrainID + ":" + key
		var admitted *meter.Reservation
		if req.Operation != "exclude" && !exists {
			entitlement, account, e := g.Meter.Subject(ctx, binding.AccountID, binding.PartnerID)
			if e != nil {
				episodeFailure(w, 503, "meter_unavailable")
				return
			}
			inventory, e := g.inventory(ctx, binding.AccountID, filepath.Dir(runtime.Root))
			if e != nil {
				episodeFailure(w, 503, "inventory_unavailable")
				return
			}
			// Admission must use the same escaping as canonical publication; the
			// caller can send much smaller literal HTML characters on the wire.
			canonical, e := json.Marshal(req.Mutation)
			if e != nil {
				episodeError(w, store.ErrEpisodeInvalid)
				return
			}
			// Reserve canonical copies plus wrapper, metadata and Git overhead.
			growth := int64(len(canonical))*4 + 64<<10
			if inventory.StorageBytes+growth >= max(entitlement.Plan.StorageBytes, account.Plan.StorageBytes) {
				episodeFailure(w, 429, "limit_exceeded")
				return
			}
			reservation, e := g.Meter.ReserveDurable(ctx, binding.AccountID, "writes", 1, entitlement.Plan.Writes, entitlement.Window, entitlement.ResetAt, meterKey)
			if e != nil {
				episodeFailure(w, 429, "limit_exceeded")
				return
			}
			admitted = &reservation
		}
		receipt, e := writer.Episode(ctx, runtime.Queue, runtime.Sources, runtime.Root, binding.BrainID, *req.Generation, *req.Mutation, recheck)
		if e != nil {
			// These errors precede source mutation. Release their quota hold;
			// uncertain publication failures retain the durable reservation.
			if admitted != nil && (errors.Is(e, store.ErrEpisodeInvalid) || errors.Is(e, store.ErrEpisodeLimit) || errors.Is(e, store.ErrEpisodeConflict) || errors.Is(e, store.ErrEpisodeFenced) || errors.Is(e, store.ErrEpisodeGeneration)) {
				_ = g.Meter.Finish(ctx, *admitted, false)
			}
			episodeError(w, e)
			return
		}
		if req.Operation != "exclude" {
			if e = g.Meter.CommitDurableOperation(ctx, binding.AccountID, "writes", meterKey); e != nil {
				episodeFailure(w, 503, "outcome_unknown")
				return
			}
		}
		p, e = store.LoadEpisodes(ctx, runtime.Sources)
		if e != nil {
			episodeFailure(w, 503, "outcome_unknown")
			return
		}
		response.Receipt = &receipt
		response.Generation = p.Generation
		response.Revision = episodeRevision(p)
		if req.Operation == "exclude" {
			response.Status = "excluded"
			response.Omissions = append(response.Omissions, "physical_bytes_and_git_history_retained", "managed_backups_and_offline_copies_not_purged")
		}
	} else {
		if err = writer.EpisodesPublished(ctx, runtime.Root, p); err != nil {
			if req.Operation == "status" {
				response.Status = "unknown"
				response.Revision = ""
				episodeJSON(w, 200, response)
			} else {
				episodeFailure(w, 503, "publication_pending")
			}
			return
		}
		switch req.Operation {
		case "capabilities":
			response.Capabilities = episodeCapabilities()
			response.Capabilities["archive_usage"] = map[string]any{"records": p.DataRecords, "events": len(p.Events), "fences": len(p.Fences), "canonical_json_bytes": p.Bytes}
		case "fences":
			response.Fences = p.Fences
		case "status":
			rec, ok := p.Records[req.CommandKey]
			response.Status = "unknown"
			if ok {
				if rec.Kind != "exclude" && !p.RecordEligible(rec) {
					response.Status = "excluded"
					break
				}
				settled := rec.Kind == "exclude"
				if !settled {
					settled, err = g.Meter.DurableCommitted(ctx, binding.AccountID, "writes", "governed:episode:"+binding.BrainID+":"+req.CommandKey)
					if err != nil {
						episodeFailure(w, 503, "meter_unavailable")
						return
					}
				}
				if settled {
					receipt := p.Receipt(req.CommandKey)
					response.Receipt = &receipt
					response.Status = "completed"
					if rec.Kind == "exclude" {
						response.Status = "excluded"
					}
				}
			}
		case "read":
			if req.EventKey != "" {
				event, ok := p.Events[req.EventKey]
				if !ok || !p.EventEligible(event) {
					episodeFailure(w, 404, "event_unavailable")
					return
				}
				response.Events = []store.ArchivedEpisodeEvent{event}
			} else {
				rec, ok := p.Records[req.CommandKey]
				if !ok || !p.RecordEligible(rec) || rec.Input == nil {
					episodeFailure(w, 404, "record_unavailable")
					return
				}
				receipt := p.Receipt(req.CommandKey)
				response.Receipt = &receipt
				response.Checkpoint = rec.Input.Checkpoint
				capture := rec.Input.Capture()
				response.Capture = &capture
				for _, ref := range rec.Refs {
					event := p.Events[ref.EventKey]
					if !p.EventEligible(event) {
						episodeFailure(w, 404, "event_unavailable")
						return
					}
					response.Events = append(response.Events, event)
				}
			}
			selected, _ := json.Marshal(struct {
				Events     []store.ArchivedEpisodeEvent `json:"events"`
				Checkpoint *store.EpisodeCheckpoint     `json:"checkpoint,omitempty"`
			}{response.Events, response.Checkpoint})
			response.SelectedBytes = len(selected)
			budget := req.MaxBytes
			if budget == 0 {
				budget = store.EpisodeMaxResponseBytes - 32768
			}
			if len(selected) > budget {
				episodeFailure(w, 413, "read_budget_exceeded")
				return
			}
		case "search", "context":
			if req.Revision != "" && req.Revision != response.Revision {
				episodeFailure(w, 409, "revision_changed")
				return
			}
			budget := req.MaxBytes
			if budget == 0 {
				budget = 64 << 10
			}
			limit := req.Limit
			if limit == 0 {
				limit = 20
			}
			events := p.SortedEvents()
			matched := 0
			for _, event := range events {
				if err = ctx.Err(); err != nil {
					episodeFailure(w, 503, "deadline")
					return
				}
				if req.ProducerID != "" && event.ProducerID != req.ProducerID || req.ConversationID != "" && event.ConversationID != req.ConversationID || req.WorkspaceLabel != "" && event.WorkspaceLabel != req.WorkspaceLabel || req.From != nil && event.Event.OccurredAt.Before(*req.From) || req.Until != nil && !event.Event.OccurredAt.Before(*req.Until) || req.Query != "" && !strings.Contains(strings.ToLower(event.Event.Text), strings.ToLower(req.Query)) {
					continue
				}
				if matched < req.Offset {
					matched++
					continue
				}
				b, _ := json.Marshal(event)
				if len(response.Events) >= limit || response.SelectedBytes+len(b) > budget {
					next := matched
					response.NextOffset = &next
					response.Omissions = append(response.Omissions, "result_or_byte_budget")
					break
				}
				response.Events = append(response.Events, event)
				response.SelectedBytes += len(b)
				matched++
			}
			if len(response.Events) == 0 {
				response.Omissions = append(response.Omissions, "no_selected_event_is_not_proof_of_absence")
			}
		}
	}
	if err = recheck(ctx); err != nil {
		episodeFailure(w, 401, "unauthorized")
		return
	}
	if readReservation != nil {
		if err = g.Meter.Finish(ctx, *readReservation, true); err != nil {
			episodeFailure(w, 503, "meter_unavailable")
			return
		}
		readReservation = nil
	}
	episodeJSON(w, 200, response)
}
