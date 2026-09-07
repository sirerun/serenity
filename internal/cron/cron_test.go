package cron

import (
	"context"
	"strings"
	"testing"
	"time"
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

func TestReadRecordMissingIsNotExist(t *testing.T) {
	if _, err := ReadRecord(t.TempDir(), "sweep"); err == nil {
		t.Fatal("ReadRecord on a fresh root = nil error, want a not-exist error")
	}
}
