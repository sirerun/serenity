package direction_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/direction"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
)

var revisitNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

func revisitFixture(t *testing.T, condition string) (string, *index.SQLite) {
	t.Helper()
	root := t.TempDir()
	writeRevisitEntry(t, root, condition, ledger.StateAccepted)
	db, err := index.Open(filepath.Join(root, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return root, db
}
func writeRevisitEntry(t *testing.T, root, condition string, state ledger.State) {
	t.Helper()
	entry := &ledger.Entry{ID: "dec-0001", Kind: ledger.KindDecision, Title: "Keep the choice", State: state, Created: "2026-08-01T00:00:00Z", Body: "Original rationale.\n", Alternatives: []ledger.Alternative{{Option: "Alternative", WhyNot: "Original rejection", RevisitIf: condition}}}
	data, err := ledger.Encode(entry)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".dira", "entries")
	if err = os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, entry.ID+".md"), data, 0644); err != nil {
		t.Fatal(err)
	}
}
func sweep(t *testing.T, root string, db *index.SQLite, now time.Time) direction.RevisitResult {
	t.Helper()
	r, err := direction.SweepRevisit(context.Background(), direction.NewStore(root, nil), db, now)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func reviewItems(t *testing.T, db *index.SQLite) []disposition.Item {
	t.Helper()
	items, err := disposition.NewStore(db).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return items
}
func disposeReview(t *testing.T, db *index.SQLite, item disposition.Item) {
	t.Helper()
	_, err := disposition.NewStore(db).Dispose(context.Background(), item.ID, disposition.VerdictReject, nil, "reviewed", "human", "", revisitNow)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRevisitElapsed(t *testing.T) {
	for _, condition := range []string{"after:2026-09-01T00:00:00Z", "after:0001-01-01T00:00:00Z"} {
		t.Run(condition, func(t *testing.T) {
			root, db := revisitFixture(t, condition)
			path := filepath.Join(root, ".dira", "entries", "dec-0001.md")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := sweep(t, root, db, revisitNow); got.Created != 1 {
				t.Fatalf("result %+v", got)
			}
			items := reviewItems(t, db)
			if len(items) != 1 {
				t.Fatalf("items %v", items)
			}
			if items[0].Kind != disposition.KindPreceptDraft || !bytes.Contains(items[0].Payload, []byte("Original rejection")) {
				t.Fatalf("review %+v", items[0])
			}
			var payload map[string]any
			if err = json.Unmarshal(items[0].Payload, &payload); err != nil {
				t.Fatal(err)
			}
			if payload["action"] != "review" {
				t.Fatal(payload)
			}
			checkpoint, err := db.RevisitCheckpoint(context.Background(), strings.TrimSuffix(items[0].ID, ":1"))
			if err != nil {
				t.Fatal(err)
			}
			if !checkpoint.LastRevisitedAt.Equal(revisitNow) {
				t.Fatalf("checkpoint %+v", checkpoint)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("precept mutated")
			}
		})
	}
}
func TestRevisitRepeatWindow(t *testing.T) {
	root, db := revisitFixture(t, "after:2026-09-01T00:00:00Z")
	sweep(t, root, db, revisitNow)
	other, err := index.Open(filepath.Join(root, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	for _, at := range []time.Time{revisitNow, revisitNow.Add(8 * 24 * time.Hour)} {
		if r := sweep(t, root, other, at); r.Created != 0 {
			t.Fatalf("pending duplicated %+v", r)
		}
	}
	disposeReview(t, other, reviewItems(t, other)[0])
	if r := sweep(t, root, other, revisitNow.Add(time.Hour)); r.Created != 0 {
		t.Fatalf("cooldown %+v", r)
	}
	if r := sweep(t, root, other, revisitNow.Add(7*24*time.Hour)); r.Created != 1 {
		t.Fatalf("eligible %+v", r)
	}
	if len(reviewItems(t, db)) != 2 {
		t.Fatal("missing historical review")
	}
}
func TestRevisitKeyedChange(t *testing.T) {
	root, db := revisitFixture(t, "claim_state_changed:project/status/a%2Fb")
	claim := domain.Claim{ID: "one", SubjectSlug: "project", Predicate: "status", ObjectKey: "a/b", State: domain.StateActive}
	put := func(c domain.Claim) {
		t.Helper()
		if err := db.UpsertClaim(context.Background(), c); err != nil {
			t.Fatal(err)
		}
	}
	put(claim)
	if r := sweep(t, root, db, revisitNow); r.Created != 0 {
		t.Fatal("baseline fired")
	}
	unrelated := claim
	unrelated.ID = "two"
	unrelated.ObjectKey = "elsewhere"
	unrelated.State = domain.StateRetracted
	put(unrelated)
	if r := sweep(t, root, db, revisitNow); r.Created != 0 {
		t.Fatal("unrelated fired")
	}
	claim.State = domain.StateRetracted
	put(claim)
	if r := sweep(t, root, db, revisitNow); r.Created != 1 {
		t.Fatal("change did not fire")
	}
	disposeReview(t, db, reviewItems(t, db)[0])
	claim.State = domain.StateActive
	put(claim)
	if r := sweep(t, root, db, revisitNow.Add(time.Hour)); r.Created != 0 {
		t.Fatal("cooldown ignored")
	}
	// Even a transient observed change remains pending if the key reverts.
	claim.State = domain.StateRetracted
	put(claim)
	if r := sweep(t, root, db, revisitNow.Add(7*24*time.Hour)); r.Created != 1 {
		t.Fatal("suppressed change consumed")
	}
}
func TestRevisitConcurrent(t *testing.T) {
	root, db := revisitFixture(t, "after:2026-09-01T00:00:00Z")
	other, err := index.Open(filepath.Join(root, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 2)
	counts := make(chan int, 2)
	for _, eng := range []*index.SQLite{db, other} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r, err := direction.SweepRevisit(context.Background(), direction.NewStore(root, nil), eng, revisitNow)
			errs <- err
			counts <- r.Created
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	close(counts)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	total := 0
	for n := range counts {
		total += n
	}
	if total != 1 || len(reviewItems(t, db)) != 1 {
		t.Fatalf("concurrent total %d", total)
	}
}
func TestRevisitNegative(t *testing.T) {
	for _, tc := range []struct {
		name, condition string
		state           ledger.State
		unsupported     int
		wantErr         bool
	}{
		{"future", "after:2030-01-01T00:00:00Z", ledger.StateAccepted, 0, false},
		{"inactive", "after:2020-01-01T00:00:00Z", ledger.StateSuperseded, 0, false},
		{"empty", "", ledger.StateAccepted, 0, false},
		{"prose", "when circumstances change", ledger.StateAccepted, 1, false},
		{"bad time", "after:never", ledger.StateAccepted, 0, true},
		{"bad key", "claim_state_changed:a/b", ledger.StateAccepted, 0, true},
		{"bad escape", "claim_state_changed:a/b/%zz", ledger.StateAccepted, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, db := revisitFixture(t, tc.condition)
			writeRevisitEntry(t, root, tc.condition, tc.state)
			r, err := direction.SweepRevisit(context.Background(), direction.NewStore(root, nil), db, revisitNow)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error %v", err)
			}
			if r.Created != 0 || r.Unsupported != tc.unsupported || len(reviewItems(t, db)) != 0 {
				t.Fatalf("result %+v", r)
			}
		})
	}
}
