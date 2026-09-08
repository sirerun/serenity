package disposition

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/index"
)

func TestDisposeStorageFailureCannotLeaveOrphanHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	eng, err := index.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	s := NewStore(eng)
	ctx := context.Background()
	item, err := s.Create(ctx, KindReconcile, nil, "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	// A real SQLite trigger rejects the state write, independent of statement
	// ordering. History must roll back with it rather than poisoning retries.
	if _, err := db.Exec(`CREATE TRIGGER audit_reject_dispose BEFORE UPDATE ON disposition_items WHEN json_extract(NEW.payload, '$.state') = 'disposed' BEGIN SELECT RAISE(ABORT, 'audit forced state failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Dispose(ctx, item.ID, VerdictAccept, nil, "", "human:fixture", "retry-key", fixedNow); err == nil {
		t.Fatal("injected storage failure was swallowed")
	}
	stored, err := s.Get(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	history, err := s.HistoryFor(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != StatePending || len(history) != 0 {
		t.Fatalf("failed transition split state/history: state=%s history=%d", stored.State, len(history))
	}
	if _, err := db.Exec(`DROP TRIGGER audit_reject_dispose`); err != nil {
		t.Fatal(err)
	}
	res, err := s.Dispose(ctx, item.ID, VerdictAccept, nil, "", "human:fixture", "retry-key", fixedNow)
	if err != nil || res.Replayed || res.AlreadyDisposed || res.Item.State != StateDisposed {
		t.Fatalf("valid retry poisoned by failed transition: %+v %v", res, err)
	}
	history, err = s.HistoryFor(ctx, item.ID)
	if err != nil || len(history) != 1 {
		t.Fatalf("retry history=%d err=%v", len(history), err)
	}
}

func TestBookkeepingAcrossIndependentStoresPreservesOneResult(t *testing.T) {
	for _, mode := range []string{"result-claim", "resurface"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "index.db")
			first, err := index.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = first.Close() }()
			second, err := index.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = second.Close() }()
			ctx := context.Background()
			seed := NewStore(first)
			item, err := seed.Create(ctx, KindReconcile, nil, "", fixedNow)
			if err != nil {
				t.Fatal(err)
			}
			item.State, item.Verdict = StateDisposed, VerdictAccept
			if mode == "resurface" {
				item.State, item.DeferCount = StateParked, MaxDeferCycles
			}
			if err := seed.put(ctx, item); err != nil {
				t.Fatal(err)
			}
			var reads, done sync.WaitGroup
			reads.Add(2)
			gate := make(chan struct{})
			go func() { reads.Wait(); close(gate) }()
			stores := []*Store{NewStore(&synchronizedReadBackend{SQLite: first, reads: &reads, gate: gate}), NewStore(&synchronizedReadBackend{SQLite: second, reads: &reads, gate: gate})}
			var errs [2]error
			for i := range 2 {
				done.Add(1)
				go func(i int) {
					defer done.Done()
					if mode == "resurface" {
						_, errs[i] = stores[i].Resurface(ctx, item.ID, fixedNow.Add(time.Hour))
					} else {
						errs[i] = stores[i].RecordResultClaimID(ctx, item.ID, []string{"first-claim", "second-claim"}[i], fixedNow.Add(time.Hour))
					}
				}(i)
			}
			done.Wait()
			if (errs[0] == nil) == (errs[1] == nil) {
				t.Fatalf("bookkeeping needs one winner: %v", errs)
			}
			stored, err := seed.Get(ctx, item.ID)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "resurface" {
				if stored.State != StatePending || !stored.Resurfaced || stored.DeferCount != MaxDeferCycles {
					t.Fatalf("resurface state lost: %+v", stored)
				}
			} else {
				winner := "first-claim"
				if errs[0] != nil {
					winner = "second-claim"
				}
				if stored.State != StateDisposed || stored.Verdict != VerdictAccept || stored.AppliedClaimID != winner {
					t.Fatalf("winning result overwritten: %+v", stored)
				}
			}
		})
	}
}

func TestDisposeAcceptsExactStoredJSONFormatting(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	item, err := s.Create(ctx, KindReconcile, json.RawMessage(`{"original":"evidence"}`), "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.backend.PutDispositionItem(ctx, item.ID, raw); err != nil {
		t.Fatal(err)
	}
	res, err := s.Dispose(ctx, item.ID, VerdictAccept, nil, "", "human:fixture", "formatted", fixedNow)
	if err != nil || res.Item.State != StateDisposed || string(res.Item.Payload) != string(item.Payload) {
		t.Fatalf("valid formatted snapshot rejected: %+v %v", res, err)
	}
}

type afterListBackend struct {
	Backend
	after func()
}

func (b *afterListBackend) DispositionItems(ctx context.Context) ([][]byte, error) {
	rows, err := b.Backend.DispositionItems(ctx)
	b.after()
	return rows, err
}

func TestSweepCannotOverwriteConcurrentHumanDecision(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	item, err := s.Create(ctx, KindReconcile, nil, "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	backend := &afterListBackend{Backend: s.backend, after: func() {
		if _, err := s.Dispose(ctx, item.ID, VerdictReject, nil, "human decision wins", "human:reviewer", "decision", fixedNow.Add(15*24*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}}
	sweeper := NewStore(backend)
	result, err := Sweep(ctx, sweeper, nil, fixedNow.Add(16*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	stored, err := s.Get(ctx, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.State != StateDisposed || stored.Verdict != VerdictReject || stored.Actor != "human:reviewer" || result.Deferred != 0 || result.Parked != 0 {
		t.Fatalf("aging overwrote a newer decision: item=%+v result=%+v", stored, result)
	}
}

func TestDistillRouteIsPartOfDecisionWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	eng, err := index.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	s := NewStore(eng)
	item, err := Capture(context.Background(), s, "An attributed capture.", "", "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	// A routed verdict must never be observable without its route, even between
	// SQL statements. Ordinary Dispose remains allowed for other call sites.
	if _, err := db.Exec(`CREATE TRIGGER audit_require_route BEFORE UPDATE ON disposition_items WHEN json_extract(NEW.payload, '$.state') = 'disposed' AND COALESCE(json_extract(NEW.payload, '$.route'), '') = '' BEGIN SELECT RAISE(ABORT, 'route must accompany decision'); END`); err != nil {
		t.Fatal(err)
	}
	res, err := s.RouteDistill(context.Background(), item.ID, RouteNote, "human:reviewer", "", "route-key", fixedNow)
	if err != nil || res.Item.Route != RouteNote || res.Item.State != StateDisposed {
		t.Fatalf("route was a separate write: %+v %v", res, err)
	}
}

func TestDistillDraftFailureRecoversRecordedRouteOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	eng, err := index.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	s := NewStore(eng)
	ctx := context.Background()
	item, err := Capture(ctx, s, "Keep the original captured evidence.", "", "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER audit_reject_draft BEFORE INSERT ON disposition_items WHEN json_extract(NEW.payload, '$.kind') = 'precept_draft' BEGIN SELECT RAISE(ABORT, 'draft unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RouteDistill(ctx, item.ID, RoutePreceptDraft, "human:reviewer", "", "route-key", fixedNow); err == nil {
		t.Fatal("follow-on storage failure swallowed")
	}
	stored, err := s.Get(ctx, item.ID)
	if err != nil || stored.Route != RoutePreceptDraft || stored.State != StateDisposed {
		t.Fatalf("committed route not recoverable: %+v %v", stored, err)
	}
	if _, err := db.Exec(`DROP TRIGGER audit_reject_draft`); err != nil {
		t.Fatal(err)
	}
	for _, route := range []DistillRoute{RouteTrash, RouteNote} {
		res, err := s.RouteDistill(ctx, item.ID, route, "human:changed-input", "changed input", "route-key", fixedNow.Add(time.Hour))
		if err != nil || !res.Replayed || res.Item.Route != RoutePreceptDraft || res.Item.Actor != "human:reviewer" {
			t.Fatalf("recorded route lost on retry: %+v %v", res, err)
		}
	}
	items, err := s.List(ctx)
	if err != nil || len(items) != 2 {
		t.Fatalf("follow-on duplicated or lost: count=%d err=%v", len(items), err)
	}
	for _, child := range items {
		if child.ID != item.ID && (child.Kind != KindPreceptDraft || string(child.Payload) != string(item.Payload) || !child.CreatedAt.Equal(fixedNow)) {
			t.Fatalf("recovery rewrote original effect: %+v", child)
		}
	}
	history, err := s.HistoryFor(ctx, item.ID)
	if err != nil || len(history) != 1 {
		t.Fatalf("route re-disposed: history=%d err=%v", len(history), err)
	}
}

type synchronizedReadBackend struct {
	*index.SQLite
	once  sync.Once
	reads *sync.WaitGroup
	gate  <-chan struct{}
}

func (b *synchronizedReadBackend) DispositionItem(ctx context.Context, id string) ([]byte, bool, error) {
	raw, found, err := b.SQLite.DispositionItem(ctx, id)
	b.once.Do(func() { b.reads.Done(); <-b.gate })
	return raw, found, err
}

func TestIndependentStoresCannotBothWinDisposition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.db")
	first, err := index.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Close() }()
	second, err := index.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = second.Close() }()
	ctx := context.Background()
	item, err := NewStore(first).Create(ctx, KindReconcile, nil, "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	var reads sync.WaitGroup
	reads.Add(2)
	gate := make(chan struct{})
	go func() { reads.Wait(); close(gate) }()
	stores := []*Store{NewStore(&synchronizedReadBackend{SQLite: first, reads: &reads, gate: gate}), NewStore(&synchronizedReadBackend{SQLite: second, reads: &reads, gate: gate})}
	var results [2]Result
	var errs [2]error
	var done sync.WaitGroup
	for i := range 2 {
		done.Add(1)
		go func(i int) {
			defer done.Done()
			verdict, actor := VerdictAccept, "human:first"
			if i == 1 {
				verdict, actor = VerdictReject, "human:second"
			}
			results[i], errs[i] = stores[i].Dispose(ctx, item.ID, verdict, nil, "independent choice", actor, actor, fixedNow)
		}(i)
	}
	done.Wait()
	winners := 0
	for i, result := range results {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		if !result.AlreadyDisposed && !result.Replayed {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("independent handles accepted %d conflicting winners: %+v", winners, results)
	}
	if results[0].Item.Verdict != results[1].Item.Verdict || results[0].Item.Actor != results[1].Item.Actor {
		t.Fatalf("loser did not receive winning decision: %+v", results)
	}
	history, err := stores[0].HistoryFor(ctx, item.ID)
	if err != nil || len(history) != 1 {
		t.Fatalf("history=%d err=%v", len(history), err)
	}
}
