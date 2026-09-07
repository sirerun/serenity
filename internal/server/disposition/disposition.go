// Package disposition implements the DISPOSITION v1 wire protocol (RFC
// 0001 §8.2, T4.4) over HTTP: list_pending, dispose, capture, subscribe.
// It is a thin transport layer over the already-shipped domain packages --
// internal/disposition.Store (T2.1/T2.5/T2.6/T2.20/T2.21) for every
// operation's actual effect, internal/events.Store (T4.12) for
// subscribe's cursor log -- adding no queue logic of its own, the same
// "no privileged internal path" discipline RFC 0001 §7 states for every
// protocol server: the CLI's own `serenity inbox`/`serenity capture`
// consume the identical domain Store methods this package calls.
//
// This is the first live producer wired into internal/events.Store
// (T4.12 shipped with none -- "T4.2/T4.4/T4.6... don't exist" was its own
// disclosed scope at the time): a genuinely NEW dispose (not a replay, not
// an already_disposed conflict) and a successful capture each append one
// event, giving subscribe(cursor) real state to replay.
//
// Disclosed scope: RFC 0001 §8.2 does not specify DISPOSITION v1's exact
// wire shapes (unlike §8.1 MEMORY_VERBS, which the RFC does describe with
// a "gbrain envelope" -- protocol_version, evidence, provenance, etc.);
// docs/protocol/DISPOSITION_v1.md (T4.16) has not shipped yet, so this
// package defines its own JSON request/response shapes, documented
// per-handler below, rather than waiting on a spec that does not exist.
// Not wired into a live `serenity serve` command yet -- T4.1 (`serenityd`
// core) has not shipped, so there is no daemon process to register these
// routes into in production; this mirrors T4.3/T4.10/T4.12's own
// disclosed "package-complete, not yet live-wired" scope. subscribe's
// long-poll fallback and SSE both poll internal/events.Store on a fixed
// interval rather than an event-driven wakeup (no pub/sub primitive
// exists in this codebase yet) -- correct or a caller reads the exact
// wall it published, one poll interval late.
package disposition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	coredisp "github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/events"
)

// Clock is the same seam internal/events.Clock and internal/cron.Clock
// use -- a single Now() method so tests can inject a fake without
// touching the real wall clock.
type Clock interface{ Now() time.Time }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

const (
	// defaultListPendingLimit and maxListPendingLimit bound list_pending's
	// page size when the caller omits or overspecifies limit.
	defaultListPendingLimit = 100
	maxListPendingLimit     = 500

	// defaultPollInterval is how often subscribe's SSE and long-poll loops
	// re-check internal/events.Store for new entries.
	defaultPollInterval = 200 * time.Millisecond
	// defaultLongPollWait bounds how long a long-poll subscribe request
	// blocks before returning an empty batch.
	defaultLongPollWait = 25 * time.Second
)

// Registrar is the subset of *internal/server.Server this package needs.
// Declared locally -- the same pattern internal/direction.Completer uses
// for *router.Router -- so this package has no hard dependency on the
// server package's concrete type; a bare *http.ServeMux satisfies it too
// (its own Handle(pattern string, handler http.Handler) method matches
// exactly), useful for a test that wants to skip auth wrapping.
type Registrar interface {
	Handle(pattern string, h http.Handler)
}

// Handlers implements DISPOSITION v1 over a disposition Store and an
// events Store.
type Handlers struct {
	store        *coredisp.Store
	events       *events.Store
	clock        Clock
	pollInterval time.Duration
	longPollWait time.Duration
}

// Option configures Handlers at construction.
type Option func(*Handlers)

// WithClock overrides the real clock. Test-only hook.
func WithClock(c Clock) Option { return func(h *Handlers) { h.clock = c } }

// WithPollInterval overrides subscribe's poll interval. Test-only hook --
// production keeps defaultPollInterval.
func WithPollInterval(d time.Duration) Option {
	return func(h *Handlers) { h.pollInterval = d }
}

// WithLongPollWait overrides subscribe's long-poll timeout. Test-only
// hook -- production keeps defaultLongPollWait.
func WithLongPollWait(d time.Duration) Option {
	return func(h *Handlers) { h.longPollWait = d }
}

// New builds Handlers over store and ev.
func New(store *coredisp.Store, ev *events.Store, opts ...Option) *Handlers {
	h := &Handlers{
		store:        store,
		events:       ev,
		clock:        realClock{},
		pollInterval: defaultPollInterval,
		longPollWait: defaultLongPollWait,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Register wires all four DISPOSITION v1 routes onto s.
func (h *Handlers) Register(s Registrar) {
	s.Handle("/disposition/list_pending", http.HandlerFunc(h.handleListPending))
	s.Handle("/disposition/dispose", http.HandlerFunc(h.handleDispose))
	s.Handle("/disposition/capture", http.HandlerFunc(h.handleCapture))
	s.Handle("/disposition/subscribe", http.HandlerFunc(h.handleSubscribe))
}

// ProtoError is DISPOSITION v1's error envelope: a stable machine-checkable
// code plus a human message, returned for every 4xx/5xx this package
// emits -- "reject without note -> protocol error" (T4.4's acc line)
// means exactly this shape at 400, code "reject_requires_note".
type ProtoError struct {
	Code    string `json:"error"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, ProtoError{Code: code, Message: msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeJSON(r *http.Request, v any) error {
	defer func() { _ = r.Body.Close() }()
	return json.NewDecoder(r.Body).Decode(v)
}

// --- list_pending -----------------------------------------------------

// ListPendingRequest is list_pending's request body (RFC 0001 §8.2:
// "list_pending(kinds?, expiring_before?, group?)"). Parked and Cursor/
// Limit are this package's own additions, not named in the RFC prose:
// Parked is the wire equivalent of `serenity inbox --parked` (RFC 0001
// §8.2: a parked item is "resurfaced only by explicit filter"; this IS
// that explicit filter), and Cursor/Limit implement the pagination the
// acc line itself requires ("cursor walk of 120 items in pages of 50
// terminates") without which list_pending could not page at all.
type ListPendingRequest struct {
	Kinds          []string `json:"kinds,omitempty"`
	ExpiringBefore string   `json:"expiring_before,omitempty"` // RFC3339
	Group          bool     `json:"group,omitempty"`
	Parked         bool     `json:"parked,omitempty"`
	Cursor         string   `json:"cursor,omitempty"`
	Limit          int      `json:"limit,omitempty"`
}

// ListedItem is one row of list_pending's response. Members is set only
// when Group was requested and the row represents a non-empty GroupID --
// see groupItems.
type ListedItem struct {
	Item    coredisp.Item   `json:"item"`
	Members []coredisp.Item `json:"members,omitempty"`
}

type ListPendingResponse struct {
	Items      []ListedItem `json:"items"`
	NextCursor string       `json:"next_cursor,omitempty"`
}

func (h *Handlers) handleListPending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "list_pending requires POST")
		return
	}
	var req ListPendingRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "decode request: "+err.Error())
		return
	}

	var expiringBefore time.Time
	hasExpiringBefore := false
	if req.ExpiringBefore != "" {
		t, err := time.Parse(time.RFC3339, req.ExpiringBefore)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "expiring_before: "+err.Error())
			return
		}
		expiringBefore = t
		hasExpiringBefore = true
	}

	kindSet := make(map[coredisp.Kind]bool, len(req.Kinds))
	for _, k := range req.Kinds {
		kindSet[coredisp.Kind(k)] = true
	}

	items, err := h.store.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}

	filtered := make([]coredisp.Item, 0, len(items))
	for _, it := range items {
		// "parked items appear only with the parked filter" -- the two
		// visibility modes are mutually exclusive, matching
		// PendingDepth/OldestPendingAge's own pending-or-deferred-only
		// definition (T2.6) for the default (non-parked) view.
		if req.Parked {
			if it.State != coredisp.StateParked {
				continue
			}
		} else if it.State != coredisp.StatePending && it.State != coredisp.StateDeferred {
			continue
		}
		if len(kindSet) > 0 && !kindSet[it.Kind] {
			continue
		}
		if hasExpiringBefore {
			// No per-kind Thresholds config surface is wired to this
			// server (disposition.Thresholds' own doc comment: "every
			// caller today effectively gets DefaultExpiryThreshold for
			// every kind") -- same disclosed gap T2.15 inherited.
			expiry := it.UpdatedAt.Add(coredisp.DefaultExpiryThreshold)
			if !expiry.Before(expiringBefore) {
				continue
			}
		}
		filtered = append(filtered, it)
	}

	rows := groupItems(filtered, req.Group)

	limit := req.Limit
	if limit <= 0 {
		limit = defaultListPendingLimit
	}
	if limit > maxListPendingLimit {
		limit = maxListPendingLimit
	}

	start := 0
	if req.Cursor != "" {
		n, err := strconv.Atoi(req.Cursor)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "malformed cursor")
			return
		}
		start = n
	}
	if start > len(rows) {
		start = len(rows)
	}
	end := start + limit
	if end > len(rows) {
		end = len(rows)
	}
	page := rows[start:end]
	if page == nil {
		page = []ListedItem{}
	}

	resp := ListPendingResponse{Items: page}
	if end < len(rows) {
		resp.NextCursor = strconv.Itoa(end)
	}
	writeJSON(w, http.StatusOK, resp)
}

// groupItems is a disclosed design choice for RFC 0001 §8.2's underspecified
// "with group, similar items ... return as grouped items -- one
// disposition covers all members": Store.List already orders items
// oldest-created-first; when group is requested, items sharing a
// non-empty GroupID collapse into one row at the position of the group's
// first-seen (oldest) member -- Item is that oldest member, Members is
// every item in the group in the same oldest-first order -- so cursor
// pagination over the resulting slice still walks a stable order. Items
// with no GroupID always appear as their own row, Members left nil,
// whether or not group was requested; disposing a group's members
// individually (dispose's own group_id path, "each recorded individually
// for the ladder") is unaffected by how this function presents them.
func groupItems(items []coredisp.Item, group bool) []ListedItem {
	rows := make([]ListedItem, 0, len(items))
	if !group {
		for _, it := range items {
			rows = append(rows, ListedItem{Item: it})
		}
		return rows
	}

	rowIndex := make(map[string]int, len(items)) // GroupID -> index into rows
	for _, it := range items {
		if it.GroupID == "" {
			rows = append(rows, ListedItem{Item: it})
			continue
		}
		if i, ok := rowIndex[it.GroupID]; ok {
			rows[i].Members = append(rows[i].Members, it)
			continue
		}
		rows = append(rows, ListedItem{Item: it, Members: []coredisp.Item{it}})
		rowIndex[it.GroupID] = len(rows) - 1
	}
	return rows
}

// --- dispose ------------------------------------------------------------

// DisposeRequest is dispose's request body (RFC 0001 §8.2: "dispose(item_id
// | group_id, verdict, edited_payload?, note?, idempotency_key)"). Exactly
// one of ItemID/GroupID is required; idempotency_key is required at this
// wire layer (the RFC's own parameter list gives it no `?`, unlike
// edited_payload/note) even though internal/disposition.Store.Dispose
// itself treats an empty idempotency key as "no idempotency check."
type DisposeRequest struct {
	ItemID         string          `json:"item_id,omitempty"`
	GroupID        string          `json:"group_id,omitempty"`
	Verdict        string          `json:"verdict"`
	EditedPayload  json.RawMessage `json:"edited_payload,omitempty"`
	Note           string          `json:"note,omitempty"`
	IdempotencyKey string          `json:"idempotency_key"`
	Actor          string          `json:"actor,omitempty"`
}

type DisposeResultWire struct {
	Item            coredisp.Item `json:"item"`
	AlreadyDisposed bool          `json:"already_disposed,omitempty"`
	Replayed        bool          `json:"replayed,omitempty"`
}

type DisposeResponse struct {
	Results []DisposeResultWire `json:"results"`
}

// handleDispose applies verdict to one item (ItemID) or every item
// sharing a GroupID, each disposed individually through
// coredisp.Store.Dispose ("each recorded individually for the ladder") --
// a group dispose is never one bulk write. A genuinely new dispose (not
// AlreadyDisposed, not Replayed) publishes one "disposition.item_disposed"
// event per item.
//
// Disclosed, narrow gap: if a group has more than one member and a later
// member's Dispose call errors after an earlier member's already
// succeeded (and was already recorded/published), this handler returns
// only the protocol error for the failing member, not a partial-success
// envelope covering the members that already committed -- the earlier
// writes are real and durable regardless of what this response reports;
// building a partial-failure wire shape is out of this task's acc-line
// scope.
func (h *Handlers) handleDispose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "dispose requires POST")
		return
	}
	var req DisposeRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "decode request: "+err.Error())
		return
	}
	if (req.ItemID == "") == (req.GroupID == "") {
		writeError(w, http.StatusBadRequest, "invalid_request", "exactly one of item_id or group_id is required")
		return
	}
	if req.IdempotencyKey == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "idempotency_key is required")
		return
	}
	verdict := coredisp.Verdict(req.Verdict)

	ids := []string{req.ItemID}
	if req.GroupID != "" {
		items, err := h.store.List(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		ids = ids[:0]
		for _, it := range items {
			if it.GroupID == req.GroupID {
				ids = append(ids, it.ID)
			}
		}
		if len(ids) == 0 {
			writeError(w, http.StatusNotFound, "not_found", "no items found for group_id "+req.GroupID)
			return
		}
	}

	now := h.clock.Now()
	results := make([]DisposeResultWire, 0, len(ids))
	for _, id := range ids {
		res, err := h.store.Dispose(r.Context(), id, verdict, req.EditedPayload, req.Note, req.Actor, req.IdempotencyKey, now)
		if err != nil {
			writeDisposeError(w, err)
			return
		}
		results = append(results, DisposeResultWire{
			Item:            res.Item,
			AlreadyDisposed: res.AlreadyDisposed,
			Replayed:        res.Replayed,
		})
		if !res.AlreadyDisposed && !res.Replayed {
			if pubErr := h.publish(r.Context(), "disposition.item_disposed", res.Item); pubErr != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "dispose recorded but event publish failed: "+pubErr.Error())
				return
			}
		}
	}

	writeJSON(w, http.StatusOK, DisposeResponse{Results: results})
}

func writeDisposeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, coredisp.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, coredisp.ErrInvalidVerdict):
		writeError(w, http.StatusBadRequest, "invalid_verdict", err.Error())
	case errors.Is(err, coredisp.ErrRejectRequiresNote):
		writeError(w, http.StatusBadRequest, "reject_requires_note", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
	}
}

// --- capture --------------------------------------------------------------

type CaptureRequest struct {
	Text     string `json:"text,omitempty"`
	AudioRef string `json:"audio_ref,omitempty"`
	Hint     string `json:"hint,omitempty"`
}

type CaptureResponse struct {
	ItemID string `json:"item_id"`
}

func (h *Handlers) handleCapture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "capture requires POST")
		return
	}
	var req CaptureRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "decode request: "+err.Error())
		return
	}
	now := h.clock.Now()
	item, err := coredisp.Capture(r.Context(), h.store, req.Text, req.AudioRef, req.Hint, now)
	if err != nil {
		if errors.Is(err, coredisp.ErrCaptureEmpty) {
			writeError(w, http.StatusBadRequest, "capture_empty", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if err := h.publish(r.Context(), "disposition.item_created", item); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "capture recorded but event publish failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, CaptureResponse{ItemID: item.ID})
}

// --- shared event publishing ----------------------------------------------

// itemEventPayload is the opaque Payload carried by every event this
// package appends -- enough for a subscriber to know what changed
// (ItemID, Kind, State) without re-fetching list_pending; the full item
// is intentionally not embedded, keeping the event log small and
// forcing subscribers to treat it as a change notification, not a
// second copy of queue state.
type itemEventPayload struct {
	ItemID string         `json:"item_id"`
	Kind   coredisp.Kind  `json:"kind"`
	State  coredisp.State `json:"state"`
}

func (h *Handlers) publish(ctx context.Context, eventKind string, item coredisp.Item) error {
	payload, err := json.Marshal(itemEventPayload{ItemID: item.ID, Kind: item.Kind, State: item.State})
	if err != nil {
		return fmt.Errorf("marshal event payload: %w", err)
	}
	_, err = h.events.AppendEvent(ctx, eventKind, payload)
	return err
}

// --- subscribe --------------------------------------------------------

// handleSubscribe implements RFC 0001 §8.2's "subscribe(cursor) --
// server-sent events over HTTP (long-poll fallback), at-least-once with
// monotonic cursors; clients resume from their cursor after disconnect."
// SSE is selected by an `Accept: text/event-stream` request header
// (matching a browser EventSource, which sets it automatically); every
// other request gets the long-poll fallback. Either way, resume prefers
// the `Last-Event-ID` header (SSE's own reconnect mechanism) over the
// `cursor` query parameter when both are present, so an EventSource that
// reconnects with no application code involved still resumes correctly.
func (h *Handlers) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "subscribe requires GET")
		return
	}
	cursor, err := parseCursor(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if wantsSSE(r) {
		h.serveSSE(w, r, cursor)
		return
	}
	h.serveLongPoll(w, r, cursor)
}

func parseCursor(r *http.Request) (int64, error) {
	raw := r.Header.Get("Last-Event-ID")
	if raw == "" {
		raw = r.URL.Query().Get("cursor")
	}
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("malformed cursor %q", raw)
	}
	return n, nil
}

func wantsSSE(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

func (h *Handlers) serveSSE(w http.ResponseWriter, r *http.Request, cursor int64) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal_error", "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	ticker := time.NewTicker(h.pollInterval)
	defer ticker.Stop()

	for {
		evs, err := h.events.Replay(ctx, cursor)
		if err != nil {
			return // best-effort: the client reconnects and resumes from its last-seen cursor
		}
		if len(evs) > 0 {
			for _, ev := range evs {
				if err := writeSSEEvent(w, ev); err != nil {
					return
				}
				cursor = ev.Cursor
			}
			flusher.Flush()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func writeSSEEvent(w http.ResponseWriter, ev events.Event) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "id: %d\ndata: %s\n\n", ev.Cursor, data)
	return err
}

type LongPollResponse struct {
	Events []events.Event `json:"events"`
}

func (h *Handlers) serveLongPoll(w http.ResponseWriter, r *http.Request, cursor int64) {
	ctx := r.Context()
	deadline := h.clock.Now().Add(h.longPollWait)
	ticker := time.NewTicker(h.pollInterval)
	defer ticker.Stop()

	for {
		evs, err := h.events.Replay(ctx, cursor)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		if len(evs) > 0 || !h.clock.Now().Before(deadline) {
			if evs == nil {
				evs = []events.Event{}
			}
			writeJSON(w, http.StatusOK, LongPollResponse{Events: evs})
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
