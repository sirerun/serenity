// Package disposition implements the DISPOSITION queue (RFC 0001 §8.2,
// §10.2, §10.3, ADR 004): the human-in-the-loop approval queue that every
// consequential machine proposal -- a reconcile A/B pair, a precept draft,
// an effect request, a distill candidate, a source tombstone cascade, a
// dirty-tree conflict, an ambiguous entity-merge candidate, a shard
// compaction pass, or a judgment-tier child-intent decomposition proposal
// -- lands in before it can change canonical state.
//
// disposition_items and disposition_history are runtime-only state (RFC §7
// preamble: "DB-only by design, enumerated in an allowlist"), seeded as
// generic (id, payload) schema shells by T0.10's RuntimeTables. This
// package "claims" that schema the same way T1.1's jobs and T1.7's
// spend_ledger did (internal/index/jobs.go, spend.go): no column
// migration, a typed Go struct marshaled whole into the payload column.
// Because these tables are never canonical, Store bypasses
// internal/writer's queue entirely for its own writes -- the file-first CI
// gate (internal/gate) only restricts UpsertEntity/UpsertClaim/InsertChunk/
// UpsertVector/WriteEntity/Append, none of which this package calls.
package disposition

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Kind enumerates disposition item kinds (RFC 0001 §8.2/§10.2/ADR 004: "Item
// kinds: reconcile, precept_draft, effect, distill, tombstone, dirty_edit").
type Kind string

const (
	KindReconcile    Kind = "reconcile"
	KindPreceptDraft Kind = "precept_draft"
	KindEffect       Kind = "effect"
	KindDistill      Kind = "distill"
	KindTombstone    Kind = "tombstone"
	KindDirtyEdit    Kind = "dirty_edit"
	// KindEntityMerge is an ambiguous entity-merge candidate awaiting human
	// review (internal/entities, T2.13, RFC 0001 §10.5: "Staged: exact/alias
	// match -> embedding similarity within type (auto-merge with undoable
	// merge event + audit trail) -> ambiguous cases as low-priority
	// disposition items"). A pair whose embedding similarity falls short of
	// the auto-merge threshold but is too close to ignore lands here instead
	// of being merged; accept/reject is a human call, not this package's --
	// internal/entities defines the payload shape and never auto-applies a
	// merge for an item of this kind.
	KindEntityMerge Kind = "entity_merge"
	// KindCompact authorizes a `serenity compact` pass (internal/store's
	// ShardStore.Compact, T0.9/T2.9, RFC 0001 §7.7: "an explicit,
	// disposition-approved `serenity compact` run -- never silently").
	// T0.9 originally gated this behind a bare `--confirm` flag "until M2
	// replaces this gate with an approved disposition item"; T2.9 is that
	// replacement. Payload is internal/cli's CompactPayload -- today an
	// empty struct, since there is exactly one compaction scope (every
	// shard family across every entity, T0.9's original sweep). Unlike
	// KindReconcile/KindEntityMerge, nothing in this codebase proposes a
	// KindCompact item automatically -- compaction is periodic
	// maintenance, not a classifier's verdict on new evidence -- so the
	// CLI itself stages one on request (`serenity compact --propose`)
	// for a human to review before accepting it.
	KindCompact Kind = "compact"
	// KindDecompose is one proposed child intent from internal/direction's
	// Decompose (T3.11, RFC 0001 §10.4/§16's judgment-tier decomposition
	// task class): a parent dira intent, run through
	// router.TaskClassDecompositionProposals, may propose several child
	// intents that `derives_from` it. Decompose stages one item per
	// proposed child (never the whole batch as one item -- RFC 0001
	// §8.2's "each recorded individually for the ladder"), all sharing one
	// GroupID so `serenity inbox` reviews and confirms the batch as one
	// row. Unlike every other Kind, a plain accept in `serenity inbox`
	// (not only `e`/edit_accept) writes through to the brain repo for this
	// Kind -- internal/direction.Store.ApplyDisposedDecompose -- because a
	// decompose proposal has no meaningful "accepted but not written"
	// state: accepting it IS the only thing to do with it, there being
	// nothing to edit first. See internal/cli/inbox.go's own doc comment
	// on runInteractive for the disclosed reasoning behind this
	// kind-specific exception to T2.7's "space never touches the brain
	// repo" default.
	KindDecompose Kind = "decompose"
)

// State enumerates disposition item lifecycle states (RFC 0001 §8.2/ADR
// 004: "States: pending, deferred, parked, disposed").
type State string

const (
	StatePending  State = "pending"
	StateDeferred State = "deferred"
	StateParked   State = "parked"
	StateDisposed State = "disposed"
)

// Verdict enumerates the verdicts the DISPOSITION v1 wire's dispose
// operation accepts (RFC 0001 §8.2: "dispose(item_id | group_id, verdict,
// edited_payload?, note?, idempotency_key)" -- "accept | edit_accept |
// reject | defer").
type Verdict string

const (
	VerdictAccept     Verdict = "accept"
	VerdictEditAccept Verdict = "edit_accept"
	VerdictReject     Verdict = "reject"
	VerdictDefer      Verdict = "defer"
)

func (v Verdict) valid() bool {
	switch v {
	case VerdictAccept, VerdictEditAccept, VerdictReject, VerdictDefer:
		return true
	}
	return false
}

// Errors returned by Store's mutators. Wrapped with fmt.Errorf and %w, so
// callers branch with errors.Is.
var (
	// ErrNotFound is returned by Get/Dispose when no item matches the id.
	ErrNotFound = errors.New("disposition: item not found")
	// ErrInvalidVerdict is returned by Dispose for a verdict outside
	// {accept, edit_accept, reject, defer}.
	ErrInvalidVerdict = errors.New("disposition: invalid verdict")
	// ErrRejectRequiresNote is returned by Dispose when verdict is reject
	// and note is empty (RFC 0001 §8.2: "Reject requires a note").
	ErrRejectRequiresNote = errors.New("disposition: reject requires a note")
)

// Item is one DISPOSITION-queue entry (RFC 0001 §8.2). Payload is opaque to
// this package -- the owning subsystem (T2.2's reconcile A/B pair, a
// precept draft body, an effect request, a T2.21 tombstone cascade, T0.4's
// dirty-tree PendingRecord) defines its own shape and decodes it back after
// Get/List. GroupID implements the wire's "grouped items" (RFC 0001 §8.2):
// items sharing one non-empty GroupID are intended to be disposed together
// by a caller iterating List, though each is still disposed individually
// through Dispose ("each recorded individually for the ladder").
type Item struct {
	ID      string          `json:"id"`
	Kind    Kind            `json:"kind"`
	State   State           `json:"state"`
	GroupID string          `json:"group_id,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// DeferCount counts how many times this item has been deferred.
	// Expiry-driven pending->deferred->parked transitions and the
	// "resurface on new evidence" rule are T2.6's scope (Expiry sweeper);
	// this package only records the count so that later logic has
	// something to act on.
	DeferCount int `json:"defer_count,omitempty"`

	// Terminal fields, set once State == StateDisposed. Retained on the
	// item itself (not just in history) so a repeat dispose against an
	// already-disposed item can answer "already_disposed" with the
	// recorded verdict without a history scan (RFC 0001 §8.2: "a second
	// disposition of the same item returns already_disposed with the
	// recorded verdict").
	Verdict        Verdict         `json:"verdict,omitempty"`
	Note           string          `json:"note,omitempty"`
	EditedPayload  json.RawMessage `json:"edited_payload,omitempty"`
	Actor          string          `json:"actor,omitempty"`
	DisposedAt     time.Time       `json:"disposed_at,omitzero"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`

	// Route is set only for a KindDistill item disposed through
	// RouteDistill (capture.go, UC-034) -- empty for every item disposed
	// through the generic Dispose path directly. It distinguishes routes
	// that share the same underlying verdict (note and claim-batch both
	// accept) so the recorded outcome stays observable after the fact.
	Route DistillRoute `json:"route,omitempty"`

	// RouteEffectPending is committed with new precept-draft route decisions
	// and cleared after deterministic staging. Older completed routes have no
	// marker and must not acquire a duplicate child when replayed after upgrade.
	RouteEffectPending bool `json:"route_effect_pending,omitempty"`

	// Resurfaced is set once Resurface (expiry.go, T2.6) has moved this
	// item from parked back to pending. RFC 0001 §8.2: a parked item is
	// "resurfaced only by explicit filter or by new evidence... Nothing
	// resurfaces forever" -- Resurface refuses a second call once this is
	// true, enforcing the one-time cap structurally rather than trusting
	// callers not to invoke it twice.
	Resurfaced bool `json:"resurfaced,omitempty"`

	// AppliedClaimID is the id of the claim a KindReconcile item's
	// accept/edit_accept verdict actually wrote to the canonical brain
	// repo (internal/supersede, T2.3/T2.7), set once RecordResultClaimID
	// is called after that write succeeds -- "the disposition row
	// references the new claim id" (T2.7's acc line). It is empty for
	// every item this package's own Dispose/Create/RouteDistill calls
	// never write to a brain repo (any non-reconcile kind, a reject or
	// defer verdict, or a reconcile item nothing has applied yet):
	// disposition_items/disposition_history remain DB-only runtime state
	// (package doc comment) regardless of this field's presence -- it is
	// a cross-reference into the canonical repo's own claim id space, not
	// a second place canonical state lives.
	AppliedClaimID string `json:"applied_claim_id,omitempty"`
	// AppliedPublicationID identifies a committed dirty-edit receipt (zero or more claims).
	AppliedPublicationID string `json:"applied_publication_id,omitempty"`
}

// HistoryEntry is one append-only disposition_history row (RFC 0001 §8.2:
// "Every disposition is recorded with actor, client, and timestamp;
// disposition history is the training signal for the earned-automation
// ladder"). Rows are never updated once written -- Store.Dispose only ever
// appends, and AppendDispositionHistory (internal/index/disposition.go)
// refuses a colliding id rather than silently overwriting a prior event.
type HistoryEntry struct {
	ID             string    `json:"id"`
	ItemID         string    `json:"item_id"`
	Verdict        Verdict   `json:"verdict"`
	Note           string    `json:"note,omitempty"`
	Actor          string    `json:"actor,omitempty"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	OccurredAt     time.Time `json:"occurred_at"`
}

// Backend is the persistence boundary Store needs. Declared with only
// primitive types (string, []byte) so *index.SQLite (internal/index/
// disposition.go) satisfies it structurally, with no import in either
// direction -- the same asymmetric-dependency shape internal/connector's
// JobStore takes over internal/index/jobs.go.
type Backend interface {
	CommitDisposition(ctx context.Context, id string, before, after []byte, historyID string, history []byte) (bool, error)
	InsertDispositionItem(ctx context.Context, id string, payload []byte) (bool, error)
	PutDispositionItem(ctx context.Context, id string, payload []byte) error
	DispositionItem(ctx context.Context, id string) (payload []byte, found bool, err error)
	DispositionItems(ctx context.Context) ([][]byte, error)
	AppendDispositionHistory(ctx context.Context, id string, payload []byte) error
	DispositionHistory(ctx context.Context) ([][]byte, error)
}

// Store is the DISPOSITION queue (RFC 0001 §8.2) over a Backend.
type Store struct {
	backend Backend

	// mu reduces contention among calls sharing this Store. Correctness across
	// independent handles comes from the backend's exact-snapshot transaction.
	mu sync.Mutex
}

// NewStore returns a Store over backend (normally a live *index.SQLite).
func NewStore(backend Backend) *Store {
	return &Store{backend: backend}
}

// newEventID returns a random 32-hex-char id, the same shape and
// generation method internal/index's newJobID uses for job ids --
// crypto/rand keeps this package stdlib-only per ADR 003, and ids are not
// content-derived (unlike claim ids, ADR 004), so two Create/Dispose calls
// can never legitimately collide.
func newEventID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("disposition: generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Create stages a new pending disposition item of the given kind, with an
// opaque caller-defined payload and an optional group id, and returns it
// with its freshly allocated id.
func (s *Store) Create(ctx context.Context, kind Kind, payload json.RawMessage, groupID string, now time.Time) (Item, error) {
	id, err := newEventID()
	if err != nil {
		return Item{}, err
	}
	return s.createWithID(ctx, id, kind, payload, groupID, now)
}

func (s *Store) createWithID(ctx context.Context, id string, kind Kind, payload json.RawMessage, groupID string, now time.Time) (Item, error) {
	item := Item{
		ID:        id,
		Kind:      kind,
		State:     StatePending,
		GroupID:   groupID,
		Payload:   payload,
		CreatedAt: now.UTC(),
		UpdatedAt: now.UTC(),
	}
	if err := s.put(ctx, item); err != nil {
		return Item{}, err
	}
	return item, nil
}

func (s *Store) put(ctx context.Context, item Item) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("disposition: marshal item %s: %w", item.ID, err)
	}
	if err := s.backend.PutDispositionItem(ctx, item.ID, payload); err != nil {
		return fmt.Errorf("disposition: put item %s: %w", item.ID, err)
	}
	return nil
}

// Get reads back one item by id, wrapping ErrNotFound when it does not
// exist.
func (s *Store) Get(ctx context.Context, id string) (Item, error) {
	item, _, err := s.getSnapshot(ctx, id)
	return item, err
}

func (s *Store) getSnapshot(ctx context.Context, id string) (Item, []byte, error) {
	payload, found, err := s.backend.DispositionItem(ctx, id)
	if err != nil {
		return Item{}, nil, fmt.Errorf("disposition: get %s: %w", id, err)
	}
	if !found {
		return Item{}, nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	var item Item
	if err := json.Unmarshal(payload, &item); err != nil {
		return Item{}, nil, fmt.Errorf("disposition: decode item %s: %w", id, err)
	}
	if item.ID != id {
		return Item{}, nil, fmt.Errorf("disposition: item identity does not match row %s", id)
	}
	return item, payload, nil
}

func (s *Store) commitItem(ctx context.Context, before []byte, item Item, historyID string, history []byte) (bool, error) {
	after, err := json.Marshal(item)
	if err != nil {
		return false, fmt.Errorf("disposition: marshal item %s: %w", item.ID, err)
	}
	committed, err := s.backend.CommitDisposition(ctx, item.ID, before, after, historyID, history)
	if err != nil {
		return false, fmt.Errorf("disposition: commit item %s: %w", item.ID, err)
	}
	return committed, nil
}

// List returns every item, oldest-created first.
func (s *Store) List(ctx context.Context) ([]Item, error) {
	payloads, err := s.backend.DispositionItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("disposition: list items: %w", err)
	}
	out := make([]Item, 0, len(payloads))
	for _, payload := range payloads {
		var item Item
		if err := json.Unmarshal(payload, &item); err != nil {
			return nil, fmt.Errorf("disposition: decode item: %w", err)
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].CreatedAt.Before(out[k].CreatedAt) })
	return out, nil
}

// HistoryFor returns every history row recorded against itemID, oldest
// first.
func (s *Store) HistoryFor(ctx context.Context, itemID string) ([]HistoryEntry, error) {
	all, err := s.allHistory(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]HistoryEntry, 0)
	for _, h := range all {
		if h.ItemID == itemID {
			out = append(out, h)
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].OccurredAt.Before(out[k].OccurredAt) })
	return out, nil
}

func (s *Store) allHistory(ctx context.Context) ([]HistoryEntry, error) {
	payloads, err := s.backend.DispositionHistory(ctx)
	if err != nil {
		return nil, fmt.Errorf("disposition: list history: %w", err)
	}
	out := make([]HistoryEntry, 0, len(payloads))
	for _, payload := range payloads {
		var h HistoryEntry
		if err := json.Unmarshal(payload, &h); err != nil {
			return nil, fmt.Errorf("disposition: decode history row: %w", err)
		}
		out = append(out, h)
	}
	return out, nil
}

// Result is Dispose's outcome: a freshly processed disposition, an
// idempotent replay of one already processed under the same
// idempotency_key (Replayed), or a cross-client conflict against an item
// some other client already disposed (AlreadyDisposed).
type Result struct {
	Item            Item
	AlreadyDisposed bool
	Replayed        bool
}

// Dispose records a verdict against item id (RFC 0001 §8.2: "dispose(item_id
// | group_id, verdict, edited_payload?, note?, idempotency_key)"). In order:
//
//  1. verdict must be one of accept/edit_accept/reject/defer.
//  2. reject requires a non-empty note.
//  3. A retry carrying an idempotencyKey already recorded against this item
//     returns that same original result (Replayed=true) -- no new history
//     row, no state recomputed: "dispose retried with the same
//     idempotency_key returns the original result".
//  4. An item already in StateDisposed, hit by a genuinely new dispose call
//     (no matching idempotency_key), returns AlreadyDisposed=true carrying
//     the recorded verdict rather than erroring or reprocessing: "a second
//     disposition of the same item returns already_disposed with the
//     recorded verdict" -- RFC 0001 §8.2's cross-client conflict rule
//     ("first successful dispose wins").
//  5. Otherwise the verdict is applied, exactly one history row is
//     appended, and the updated item is written back.
func (s *Store) Dispose(ctx context.Context, id string, verdict Verdict, editedPayload json.RawMessage, note, actor, idempotencyKey string, now time.Time) (Result, error) {
	return s.dispose(ctx, id, verdict, editedPayload, note, actor, idempotencyKey, now, "")
}

func (s *Store) dispose(ctx context.Context, id string, verdict Verdict, editedPayload json.RawMessage, note, actor, idempotencyKey string, now time.Time, route DistillRoute) (Result, error) {
	if !verdict.valid() {
		return Result{}, fmt.Errorf("%w: %q", ErrInvalidVerdict, verdict)
	}
	if verdict == VerdictReject && note == "" {
		return Result{}, ErrRejectRequiresNote
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		item, snapshot, err := s.getSnapshot(ctx, id)
		if err != nil {
			return Result{}, err
		}
		if route != "" && item.Kind != KindDistill {
			return Result{}, fmt.Errorf("%w: %s is kind %q", ErrNotDistill, id, item.Kind)
		}

		if idempotencyKey != "" {
			history, err := s.HistoryFor(ctx, id)
			if err != nil {
				return Result{}, err
			}
			for _, h := range history {
				if h.IdempotencyKey == idempotencyKey {
					latest, err := s.Get(ctx, id)
					if err != nil {
						return Result{}, err
					}
					return Result{Item: latest, Replayed: true}, nil
				}
			}
		}

		if item.State == StateDisposed {
			return Result{Item: item, AlreadyDisposed: true}, nil
		}

		now = now.UTC()
		switch verdict {
		case VerdictAccept, VerdictEditAccept, VerdictReject:
			item.State = StateDisposed
			item.DisposedAt = now
		case VerdictDefer:
			item.State = StateDeferred
			item.DeferCount++
		}
		item.Route = route
		item.RouteEffectPending = route == RoutePreceptDraft
		item.Verdict = verdict
		item.Note = note
		item.Actor = actor
		item.IdempotencyKey = idempotencyKey
		if verdict == VerdictEditAccept {
			item.EditedPayload = editedPayload
		}
		item.UpdatedAt = now

		histID, err := newEventID()
		if err != nil {
			return Result{}, err
		}
		entry := HistoryEntry{
			ID:             histID,
			ItemID:         id,
			Verdict:        verdict,
			Note:           note,
			Actor:          actor,
			IdempotencyKey: idempotencyKey,
			OccurredAt:     now,
		}
		histPayload, err := json.Marshal(entry)
		if err != nil {
			return Result{}, fmt.Errorf("disposition: marshal history entry: %w", err)
		}
		committed, err := s.commitItem(ctx, snapshot, item, histID, histPayload)
		if err != nil {
			return Result{}, err
		}
		if !committed {
			continue
		}
		return Result{Item: item}, nil
	}
}

// RecordResultClaimID stamps AppliedClaimID on an already-disposed item
// once a caller outside this package (internal/supersede, T2.3/T2.7) has
// actually written the claim id names to the canonical brain repo. It is
// a separate, later write from Dispose's own -- the disposition_items row
// changes state (pending -> disposed) the instant a human verdicts it,
// which can happen before the corresponding brain-repo write completes
// (a crash, a retry, a batch-applied backlog); this method lets that
// write's result reach the disposition row whenever it does succeed,
// without re-running Dispose's own idempotency/already_disposed machinery
// (id is a fact about a write that already happened, not a new verdict).
func (s *Store) RecordResultClaimID(ctx context.Context, id, claimID string, now time.Time) error {
	if claimID == "" {
		return errors.New("disposition: result claim id is empty")
	}
	_, _, err := s.updateItem(ctx, id, func(item *Item) (bool, error) {
		if item.State != StateDisposed || (item.Verdict != VerdictAccept && item.Verdict != VerdictEditAccept) {
			return false, errors.New("disposition: cannot record a result for an unaccepted decision")
		}
		if item.AppliedClaimID == claimID {
			return false, nil
		}
		if item.AppliedClaimID != "" {
			return false, fmt.Errorf("disposition: result already recorded as %s", item.AppliedClaimID)
		}
		item.AppliedClaimID = claimID
		if now.After(item.UpdatedAt) {
			item.UpdatedAt = now.UTC()
		}
		return true, nil
	})
	return err
}

// RecordPublicationID marks a committed dirty edit without inventing a claim ID.
func (s *Store) RecordPublicationID(ctx context.Context, id, publicationID string, now time.Time) error {
	if publicationID == "" {
		return errors.New("disposition: empty publication id")
	}
	_, _, err := s.updateItem(ctx, id, func(item *Item) (bool, error) {
		if item.Kind != KindDirtyEdit || item.State != StateDisposed || item.Verdict != VerdictAccept {
			return false, errors.New("disposition: accepted dirty edit required")
		}
		if item.AppliedPublicationID == publicationID {
			return false, nil
		}
		if item.AppliedPublicationID != "" {
			return false, errors.New("disposition: different publication already recorded")
		}
		item.AppliedPublicationID = publicationID
		if now.After(item.UpdatedAt) {
			item.UpdatedAt = now.UTC()
		}
		return true, nil
	})
	return err
}

// updateItem retries bookkeeping against the latest item. mutate must be pure:
// a competing writer may make it run again, and only the successful CAS counts.
func (s *Store) updateItem(ctx context.Context, id string, mutate func(*Item) (bool, error)) (Item, bool, error) {
	for {
		if err := ctx.Err(); err != nil {
			return Item{}, false, err
		}
		item, snapshot, err := s.getSnapshot(ctx, id)
		if err != nil {
			return Item{}, false, err
		}
		changed, err := mutate(&item)
		if err != nil {
			return Item{}, false, err
		}
		if !changed {
			return item, false, nil
		}
		committed, err := s.commitItem(ctx, snapshot, item, "", nil)
		if err != nil {
			return Item{}, false, err
		}
		if committed {
			return item, true, nil
		}
	}
}
