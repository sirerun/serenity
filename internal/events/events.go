// Package events implements the shared event log (RFC 0001 §7.6/§14):
// a persisted, monotonically-cursored append log that the SSE (T4.4/T4.6)
// and stdio (T4.2) protocol servers will both replay from, so a client
// disconnecting mid-stream and reconnecting with its last-seen cursor
// (Last-Event-ID, in the SSE case) never loses an event -- at-least-once
// delivery. Nothing wires a live producer into this yet (T4.2/T4.4/T4.6
// do not exist), the same disclosed-scope shape T2.19's cron runner and
// T2.12's decay sweep shipped with: this package's own log semantics
// (append, persisted cursor, replay-from) are complete and tested now;
// whichever protocol-server task lands first calls Append at its own
// state-change points.
package events

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Clock is the same seam internal/cron.Clock and internal/index.Clock use
// -- a single Now() method so tests can inject a fake without touching the
// real wall clock.
type Clock interface{ Now() time.Time }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Event is one durable, cursored log entry. Kind is the caller-defined
// event type (e.g. a future DISPOSITION_v1 "item_created"); Payload is
// opaque to this package, the same convention internal/disposition.Item
// uses for its own Payload field.
type Event struct {
	Cursor     int64           `json:"cursor"`
	Kind       string          `json:"kind"`
	Payload    json.RawMessage `json:"payload,omitempty"`
	OccurredAt time.Time       `json:"occurred_at"`
}

// Backend is the persistence boundary Store needs. Declared with only
// primitive types so *index.SQLite (internal/index/events.go) satisfies it
// structurally, with no import in either direction -- the same
// asymmetric-dependency shape internal/disposition.Backend takes over
// internal/index/disposition.go.
type Backend interface {
	AppendEvent(ctx context.Context, id string, payload []byte) error
	EventPayloads(ctx context.Context) ([][]byte, error)
}

// Store is the event log over a Backend.
type Store struct {
	backend Backend
	clock   Clock

	// mu serializes Append's cursor allocation. The next cursor is derived
	// by reading every persisted event and taking max(Cursor)+1 (see
	// nextCursorLocked) rather than a separate counter row, which is what
	// makes cursor persistence trivial across a restart -- a freshly
	// opened Store over the same Backend recomputes the same next cursor
	// from what is already on disk, no separate state to lose or corrupt.
	// But that read-then-write is exactly the shape T2.8 found unguarded
	// in internal/disposition.Store.Dispose: two concurrent Append calls
	// without this lock could both read the same max and both allocate
	// the same cursor, breaking monotonicity. Hold mu across the whole
	// read-allocate-write sequence.
	mu sync.Mutex
}

// Option configures a Store at construction.
type Option func(*Store)

// WithClock overrides the real clock. Test-only hook.
func WithClock(c Clock) Option { return func(s *Store) { s.clock = c } }

// NewStore returns a Store over backend (normally a live *index.SQLite).
func NewStore(backend Backend, opts ...Option) *Store {
	s := &Store{backend: backend, clock: realClock{}}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func newEventID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("events: generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Append records one event of the given kind and opaque payload, allocating
// the next monotonic cursor, and returns the stored Event.
func (s *Store) Append(ctx context.Context, kind string, payload json.RawMessage) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	maxCursor, err := s.maxCursorLocked(ctx)
	if err != nil {
		return Event{}, err
	}

	id, err := newEventID()
	if err != nil {
		return Event{}, err
	}
	ev := Event{
		Cursor:     maxCursor + 1,
		Kind:       kind,
		Payload:    payload,
		OccurredAt: s.clock.Now().UTC(),
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return Event{}, fmt.Errorf("events: marshal event: %w", err)
	}
	if err := s.backend.AppendEvent(ctx, id, data); err != nil {
		return Event{}, fmt.Errorf("events: append: %w", err)
	}
	return ev, nil
}

// maxCursorLocked returns the highest cursor currently persisted, or 0 if
// the log is empty (so the first Append allocates cursor 1). Callers must
// hold mu.
func (s *Store) maxCursorLocked(ctx context.Context) (int64, error) {
	all, err := s.allLocked(ctx)
	if err != nil {
		return 0, err
	}
	var max int64
	for _, ev := range all {
		if ev.Cursor > max {
			max = ev.Cursor
		}
	}
	return max, nil
}

func (s *Store) allLocked(ctx context.Context) ([]Event, error) {
	payloads, err := s.backend.EventPayloads(ctx)
	if err != nil {
		return nil, fmt.Errorf("events: list: %w", err)
	}
	out := make([]Event, 0, len(payloads))
	for _, payload := range payloads {
		var ev Event
		if err := json.Unmarshal(payload, &ev); err != nil {
			return nil, fmt.Errorf("events: decode event: %w", err)
		}
		out = append(out, ev)
	}
	return out, nil
}

// Replay returns every event with Cursor strictly greater than afterCursor,
// in ascending cursor order -- the resume semantics an SSE Last-Event-ID or
// a stdio notification client's own bookmark needs: replaying from a
// dropped connection's last-seen cursor returns exactly what it missed,
// nothing more, nothing lost (at-least-once delivery). Replay(0) returns
// the whole log.
func (s *Store) Replay(ctx context.Context, afterCursor int64) ([]Event, error) {
	s.mu.Lock()
	all, err := s.allLocked(ctx)
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(all))
	for _, ev := range all {
		if ev.Cursor > afterCursor {
			out = append(out, ev)
		}
	}
	sort.Slice(out, func(i, k int) bool { return out[i].Cursor < out[k].Cursor })
	return out, nil
}
