package cron

import (
	"context"
	"github.com/sirerun/serenity/internal/config"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/providers"
)

// fakeClock is a fixed clock: every call returns the same instant. Real
// scheduled behavior (T2.6/T2.12/T2.14/T2.15, once they land) is asserted
// against this rather than time.Now() so tests never flake on wall-clock
// skew.
type fakeClock struct{ at time.Time }

func (f fakeClock) Now() time.Time { return f.at }

func TestNamesIsSortedAndComplete(t *testing.T) {
	want := []string{"consolidate", "decay", "slo", "sweep"}
	got := Names()
	if len(got) != len(want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Names() = %v, want %v", got, want)
		}
	}
}

func TestRunUnknownJobNamesEveryValidJob(t *testing.T) {
	err := Run(context.Background(), "bogus", t.TempDir(), fakeClock{at: time.Now()})
	if err == nil {
		t.Fatal("Run(\"bogus\", ...) = nil, want an error")
	}
	for _, name := range Names() {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("error %q does not name valid job %q", err, name)
		}
	}
}

// TestEachJobExitsCleanAndIsIdempotentWithFakeClock covers T2.19's own acc
// line directly: every registered job (not just sweep) runs against a
// fixture root with an injected clock, exits nil, and running it a second
// time is a genuine no-op — never an error, never a second unrelated
// side effect — with the run record reflecting exactly two runs at the
// clock's fixed instant.
func TestEachJobExitsCleanAndIsIdempotentWithFakeClock(t *testing.T) {
	at := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	clock := fakeClock{at: at}

	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := config.Default().Save(filepath.Join(root, config.FileName)); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()

			if err := Run(ctx, name, root, clock); err != nil {
				t.Fatalf("first run: %v", err)
			}
			rec, err := ReadRecord(root, name)
			if err != nil {
				t.Fatalf("read record after first run: %v", err)
			}
			if rec.Job != name || !rec.LastRun.Equal(at) || rec.RunCount != 1 {
				t.Fatalf("after first run: got %+v, want Job=%s LastRun=%s RunCount=1", rec, name, at)
			}

			if err := Run(ctx, name, root, clock); err != nil {
				t.Fatalf("second run: %v", err)
			}
			rec, err = ReadRecord(root, name)
			if err != nil {
				t.Fatalf("read record after second run: %v", err)
			}
			if rec.RunCount != 2 || !rec.LastRun.Equal(at) {
				t.Fatalf("after second run: got %+v, want RunCount=2 LastRun=%s", rec, at)
			}
		})
	}
}

// TestSweepJobAdvancesExpiredDispositionItem proves `serenity cron sweep`
// is real, not a placeholder (T2.6): a disposition item seeded directly
// against the same brain root's index, aged past the default 14-day
// threshold, actually transitions to deferred when the Sweep job runs
// through the CLI-facing entry point (Run), not just internal/disposition
// package's own tests.
func TestSweepJobAdvancesExpiredDispositionItem(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	start := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	store := disposition.NewStore(eng)
	item, err := store.Create(ctx, disposition.KindReconcile, nil, "", start)
	if err != nil {
		_ = eng.Close()
		t.Fatalf("Create: %v", err)
	}
	if err := eng.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	later := start.Add(15 * 24 * time.Hour)
	if err := Run(ctx, "sweep", root, fakeClock{at: later}); err != nil {
		t.Fatalf("Run(sweep): %v", err)
	}

	eng2, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatalf("OpenIndex (reopen): %v", err)
	}
	defer func() { _ = eng2.Close() }()
	got, err := disposition.NewStore(eng2).Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != disposition.StateDeferred || got.DeferCount != 1 {
		t.Fatalf("after cron sweep: State=%q DeferCount=%d, want deferred/1", got.State, got.DeferCount)
	}
}

func TestReadRecordMissingIsNotExist(t *testing.T) {
	if _, err := ReadRecord(t.TempDir(), "sweep"); err == nil {
		t.Fatal("ReadRecord on a fresh root = nil error, want a not-exist error")
	}
}
