package events

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/index"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	eng, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return NewStore(eng, WithClock(fakeClock{fixedNow}))
}

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

var fixedNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

func TestAppendAllocatesMonotonicCursorsStartingAtOne(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	first, err := s.Append(ctx, "kind-a", json.RawMessage(`{"n":1}`))
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if first.Cursor != 1 {
		t.Fatalf("first Cursor = %d, want 1", first.Cursor)
	}
	second, err := s.Append(ctx, "kind-b", nil)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if second.Cursor != 2 {
		t.Fatalf("second Cursor = %d, want 2", second.Cursor)
	}
	if !second.OccurredAt.Equal(fixedNow) {
		t.Fatalf("OccurredAt = %v, want %v", second.OccurredAt, fixedNow)
	}
}

// TestCursorsSurviveARestart is the acc-line clause: "cursors survive a
// restart." A restart is simulated by opening a fresh Store over the same
// backend (the same thing a fresh process would do against the same
// on-disk SQLite file) -- the new Store must continue the cursor sequence
// from what is already persisted, never reset to 1 or collide.
func TestCursorsSurviveARestart(t *testing.T) {
	ctx := context.Background()
	eng, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	defer func() { _ = eng.Close() }()

	s1 := NewStore(eng, WithClock(fakeClock{fixedNow}))
	if _, err := s1.Append(ctx, "kind-a", nil); err != nil {
		t.Fatalf("Append (before restart): %v", err)
	}
	if _, err := s1.Append(ctx, "kind-a", nil); err != nil {
		t.Fatalf("Append (before restart): %v", err)
	}

	// "Restart": a brand new Store instance over the same backend, the
	// same as a fresh process opening the same on-disk database.
	s2 := NewStore(eng, WithClock(fakeClock{fixedNow.Add(time.Hour)}))
	third, err := s2.Append(ctx, "kind-b", nil)
	if err != nil {
		t.Fatalf("Append (after restart): %v", err)
	}
	if third.Cursor != 3 {
		t.Fatalf("post-restart Cursor = %d, want 3 (continuing the persisted sequence)", third.Cursor)
	}

	all, err := s2.Replay(ctx, 0)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("Replay(0) returned %d events, want 3", len(all))
	}
}

// TestReplayFromCursorReturnsExactlyEventsAfterIt is the acc-line clause:
// "replay from cursor N returns exactly events > N."
func TestReplayFromCursorReturnsExactlyEventsAfterIt(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	var cursors []int64
	for i := range 5 {
		ev, err := s.Append(ctx, "kind", json.RawMessage(fmt.Sprintf(`{"i":%d}`, i)))
		if err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
		cursors = append(cursors, ev.Cursor)
	}

	got, err := s.Replay(ctx, cursors[1]) // after the 2nd event (cursor=2)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("Replay(%d) returned %d events, want 3 (events 3,4,5)", cursors[1], len(got))
	}
	for i, ev := range got {
		wantCursor := cursors[2+i]
		if ev.Cursor != wantCursor {
			t.Fatalf("Replay result[%d].Cursor = %d, want %d", i, ev.Cursor, wantCursor)
		}
		if ev.Cursor <= cursors[1] {
			t.Fatalf("Replay(%d) returned cursor %d, which is not > %d", cursors[1], ev.Cursor, cursors[1])
		}
	}

	everything, err := s.Replay(ctx, 0)
	if err != nil {
		t.Fatalf("Replay(0): %v", err)
	}
	if len(everything) != 5 {
		t.Fatalf("Replay(0) returned %d events, want all 5", len(everything))
	}
}

// TestDroppedConsumerReplayLosesNoEvents is the acc-line clause:
// "at-least-once delivery is asserted by a dropped-consumer test." A
// consumer reads up to some cursor, "drops" (its connection simply ends --
// nothing about Store's state changes, since Store tracks no per-consumer
// session), more events land while it is gone, and reconnecting with its
// last-seen cursor must return every event it missed, none lost.
func TestDroppedConsumerReplayLosesNoEvents(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	for i := 0; i < 3; i++ {
		if _, err := s.Append(ctx, "before-drop", nil); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	firstBatch, err := s.Replay(ctx, 0)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(firstBatch) != 3 {
		t.Fatalf("first batch = %d events, want 3", len(firstBatch))
	}
	lastSeen := firstBatch[len(firstBatch)-1].Cursor

	// Consumer "drops" here -- no call into Store, simulating a closed
	// connection. More events land while it is gone.
	for i := 0; i < 2; i++ {
		if _, err := s.Append(ctx, "while-dropped", nil); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	// Consumer reconnects with its last-seen cursor.
	recovered, err := s.Replay(ctx, lastSeen)
	if err != nil {
		t.Fatalf("Replay on reconnect: %v", err)
	}
	if len(recovered) != 2 {
		t.Fatalf("recovered %d events after reconnect, want exactly 2 (none lost, none duplicated)", len(recovered))
	}
	for _, ev := range recovered {
		if ev.Kind != "while-dropped" {
			t.Fatalf("recovered event Kind = %q, want %q", ev.Kind, "while-dropped")
		}
	}
}

// TestConcurrentAppendNeverCollidesOrSkipsACursor races many goroutines
// appending concurrently and asserts the resulting cursor sequence is a
// gap-free, collision-free permutation of 1..N -- the mutex in Store.Append
// exists precisely to guarantee this (the same class of unguarded
// read-then-write T2.8 found and fixed in internal/disposition.Store).
func TestConcurrentAppendNeverCollidesOrSkipsACursor(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	const n = 50
	var wg sync.WaitGroup
	cursors := make([]int64, n)
	errs := make([]error, n)
	wg.Add(n)
	for i := range n {
		go func() {
			defer wg.Done()
			ev, err := s.Append(ctx, "concurrent", nil)
			cursors[i] = ev.Cursor
			errs[i] = err
		}()
	}
	wg.Wait()

	seen := make(map[int64]bool, n)
	for i, err := range errs {
		if err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
		if seen[cursors[i]] {
			t.Fatalf("cursor %d allocated more than once", cursors[i])
		}
		seen[cursors[i]] = true
	}
	for c := int64(1); c <= n; c++ {
		if !seen[c] {
			t.Fatalf("cursor %d was never allocated -- gap in the sequence", c)
		}
	}
}
