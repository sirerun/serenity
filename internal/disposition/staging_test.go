package disposition

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/index"
)

func TestCreateOnceAcrossIndependentStoresPreservesDecision(t *testing.T) {
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
	stores := []*Store{NewStore(first), NewStore(second)}
	ctx := context.Background()
	type outcome struct {
		item    Item
		created bool
		err     error
	}
	results := make(chan outcome, 32)
	var group sync.WaitGroup
	for i := 0; i < 32; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			item, created, err := stores[i%2].CreateOnce(ctx, KindReconcile, json.RawMessage(`{"source":"unchanged"}`), "", "source-observation", fixedNow.Add(time.Duration(i)*time.Second))
			results <- outcome{item, created, err}
		}(i)
	}
	group.Wait()
	close(results)
	count := 0
	id := ""
	for result := range results {
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.created {
			count++
		}
		if id == "" {
			id = result.item.ID
		}
		if id != result.item.ID {
			t.Fatal("concurrent staging produced different identities")
		}
	}
	if count != 1 {
		t.Fatalf("fresh insertions=%d, want one", count)
	}
	if _, err := stores[0].Dispose(ctx, id, VerdictReject, nil, "source contradicts reviewed evidence", "human:reviewer", "", fixedNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	replay, created, err := stores[1].CreateOnce(ctx, KindReconcile, json.RawMessage(`{"source":"attempt to replace payload"}`), "", "source-observation", fixedNow.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if created || replay.State != StateDisposed || replay.Verdict != VerdictReject || replay.Actor != "human:reviewer" || string(replay.Payload) != `{"source":"unchanged"}` {
		t.Fatalf("replay overwrote decision: %+v", replay)
	}
	history, err := stores[1].HistoryFor(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("history entries=%d", len(history))
	}
	items, err := stores[1].List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("queue items=%d", len(items))
	}
}
