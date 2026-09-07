package index

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/direction"
)

func TestRevisitRollsBackCardAndCheckpoint(t *testing.T) {
	ctx := context.Background()
	eng := openDispositionTestDB(t)
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	request := direction.RevisitRequest{ID: "revisit:rollback", Condition: direction.RevisitCondition{Timed: true, After: now.Add(-time.Hour)}, Payload: json.RawMessage(`{"action":"review"}`), Now: now}
	// Fail after the card INSERT, when the checkpoint UPDATE is attempted.
	// A rollback must remove both the card and the initial checkpoint shell.
	_, err := eng.db.ExecContext(ctx, `CREATE TRIGGER reject_revisit_checkpoint BEFORE UPDATE ON caches BEGIN SELECT RAISE(ABORT, 'injected checkpoint failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if created, err := eng.Revisit(ctx, request); err == nil || created {
		t.Fatalf("got created=%v err=%v", created, err)
	}
	for _, table := range []string{"caches", "disposition_items"} {
		var count int
		if err := eng.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s retained %d rows after failed transaction", table, count)
		}
	}
	if _, err := eng.db.ExecContext(ctx, `DROP TRIGGER reject_revisit_checkpoint`); err != nil {
		t.Fatal(err)
	}
	if created, err := eng.Revisit(ctx, request); err != nil || !created {
		t.Fatalf("retry: created=%v err=%v", created, err)
	}
	cp, err := eng.RevisitCheckpoint(ctx, request.ID)
	if err != nil || cp.Generation != 1 || !cp.LastRevisitedAt.Equal(now) {
		t.Fatalf("checkpoint after retry: %+v %v", cp, err)
	}
	items, err := eng.DispositionItems(ctx)
	if err != nil || len(items) != 1 {
		t.Fatalf("cards after retry: %d %v", len(items), err)
	}
}

func TestRevisitRejectsCorruptCheckpointWithoutCard(t *testing.T) {
	ctx := context.Background()
	eng := openDispositionTestDB(t)
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	id := "revisit:corrupt"
	if _, err := eng.db.ExecContext(ctx, `INSERT INTO caches(id,payload) VALUES(?,?)`, id, []byte(`broken-json`)); err != nil {
		t.Fatal(err)
	}
	request := direction.RevisitRequest{ID: id, Condition: direction.RevisitCondition{Timed: true, After: now}, Payload: json.RawMessage(`{"action":"review"}`), Now: now}
	if created, err := eng.Revisit(ctx, request); err == nil || created {
		t.Fatalf("corrupt state accepted: created=%v err=%v", created, err)
	}
	items, err := eng.DispositionItems(ctx)
	if err != nil || len(items) != 0 {
		t.Fatalf("cards after corrupt checkpoint: %d %v", len(items), err)
	}
	var stored string
	if err := eng.db.QueryRowContext(ctx, `SELECT payload FROM caches WHERE id=?`, id).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "broken-json" {
		t.Fatalf("corrupt state silently replaced: %q", stored)
	}
}
