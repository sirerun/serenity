package direction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/dira/ledger"
)

// TestNilQueueStore_EveryMutatorErrorsNeverPanics is ADR 012 §3's guard:
// a Store built with a nil writer queue -- the read-only handle
// pkg/serenity's Open builds -- must refuse every write path with an
// error wrapping ErrReadOnly. Each case runs under a deferred recover so
// a nil-pointer dereference is reported as a named failure, not a crashed
// test binary. Reads on the same handle keep working: the guard is on
// writes only.
func TestNilQueueStore_EveryMutatorErrorsNeverPanics(t *testing.T) {
	root := t.TempDir()
	entries := filepath.Join(root, ".dira", "entries")
	if err := os.MkdirAll(entries, 0o755); err != nil {
		t.Fatal(err)
	}
	staged := &ledger.Entry{
		ID: "dec-0001", Kind: ledger.KindDecision, Title: "seed", State: ledger.StateStaged,
		Created:      "2026-09-02T00:00:00Z",
		Alternatives: []ledger.Alternative{{Option: "other", WhyNot: "no", RevisitIf: "later"}},
		Body:         "seed\n",
	}
	data, err := ledger.Encode(staged)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(entries, "dec-0001.md"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	s := NewStore(root, nil)
	ctx := context.Background()
	now := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	fresh := func() *ledger.Entry {
		e := *staged
		e.ID = "dec-0002"
		return &e
	}

	cases := []struct {
		name string
		call func() error
	}{
		{"Create", func() error { return s.Create(ctx, fresh()) }},
		{"Put", func() error { return s.Put(ctx, fresh()) }},
		{"Delete", func() error { return s.Delete(ctx, "dec-0001") }},
		{"CreateDraft", func() error { return s.CreateDraft(ctx, fresh()) }},
		{"Confirm", func() error { _, err := s.Confirm(ctx, "dec-0001", "human", now); return err }},
		{"Supersede", func() error {
			next := fresh()
			next.ID = ""
			_, _, err := s.Supersede(ctx, "dec-0001", next, now)
			return err
		}},
		{"Answer", func() error { _, err := s.Answer(ctx, "dec-0001", "human", now); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("%s panicked on a nil-queue store: %v", tc.name, r)
					}
				}()
				err = tc.call()
			}()
			if err == nil {
				t.Fatalf("%s on a nil-queue store returned nil error", tc.name)
			}
			if !errors.Is(err, ErrReadOnly) {
				t.Fatalf("%s error = %v, want wrapping ErrReadOnly", tc.name, err)
			}
		})
	}

	// The guard is on writes only: the same handle still reads.
	if _, err := s.Get(ctx, "dec-0001"); err != nil {
		t.Fatalf("Get on a nil-queue store: %v", err)
	}
	infos, err := s.List(ctx)
	if err != nil || len(infos) != 1 {
		t.Fatalf("List on a nil-queue store = %v, %v; want 1 entry, nil", infos, err)
	}
	// Nothing was written: the staged entry is byte-identical and no
	// second entry appeared.
	got, err := os.ReadFile(filepath.Join(entries, "dec-0001.md"))
	if err != nil || string(got) != string(data) {
		t.Fatalf("dec-0001.md changed under a read-only store")
	}
}
