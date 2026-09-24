package meter_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/meter"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func TestConcurrentAdmission(t *testing.T) {
	ctx := context.Background()
	s, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	a, e := s.CreateAccount(ctx, "a@example.com")
	if e != nil {
		t.Fatal(e)
	}
	m := &meter.Meter{Store: s}
	var admitted atomic.Int32
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := m.Reserve(ctx, a.ID, "writes", 1, 60, "window", time.Now().Add(time.Hour), "")
			if err != nil {
				var limit *meter.LimitError
				if !errors.As(err, &limit) {
					t.Error(err)
				}
				return
			}
			admitted.Add(1)
			if err = m.Finish(ctx, r, true); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if admitted.Load() != 60 {
		t.Fatalf("admitted %d, want 60", admitted.Load())
	}
}
func TestReplayAndLeaseExpiry(t *testing.T) {
	ctx := context.Background()
	s, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	a, e := s.CreateAccount(ctx, "a@example.com")
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	m := &meter.Meter{Store: s, Clock: func() time.Time { return now }}
	r, e := m.Reserve(ctx, a.ID, "writes", 1, 2, "window", now.Add(time.Hour), "first")
	if e != nil {
		t.Fatal(e)
	}
	if e = m.Finish(ctx, r, true); e != nil {
		t.Fatal(e)
	}
	r, e = m.Reserve(ctx, a.ID, "writes", 1, 2, "window", now.Add(time.Hour), "first")
	if e != nil || !r.Replay {
		t.Fatalf("replay %+v %v", r, e)
	}
	r, e = m.Reserve(ctx, a.ID, "writes", 1, 0, "next-window", now.Add(24*time.Hour), "first")
	if e != nil || !r.Replay {
		t.Fatalf("cross-window replay %+v %v", r, e)
	}
	if _, e = m.Reserve(ctx, a.ID, "writes", 1, 2, "window", now.Add(time.Hour), "second"); e != nil {
		t.Fatal(e)
	}
	now = now.Add(6 * time.Minute)
	if _, e = m.Reserve(ctx, a.ID, "writes", 1, 2, "window", now.Add(time.Hour), "third"); e != nil {
		t.Fatal(e)
	}
}
