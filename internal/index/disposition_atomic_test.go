package index

import (
	"bytes"
	"context"
	"testing"
)

func TestCommitDispositionRollsBackOnHistoryFailure(t *testing.T) {
	s := openDispositionTestDB(t)
	ctx := context.Background()
	before, after := []byte("{\n  \"state\": \"pending\"\n}"), []byte(`{"state":"disposed"}`)
	if err := s.PutDispositionItem(ctx, "item", before); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendDispositionHistory(ctx, "existing", []byte(`{"original":true}`)); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.CommitDisposition(ctx, "item", before, after, "existing", []byte(`{"replacement":true}`)); err == nil || ok {
		t.Fatalf("history collision should fail: committed=%v err=%v", ok, err)
	}
	actual, found, err := s.DispositionItem(ctx, "item")
	if err != nil || !found || !bytes.Equal(actual, before) {
		t.Fatalf("history failure left changed item: %q found=%v err=%v", actual, found, err)
	}
	history, err := s.DispositionHistory(ctx)
	if err != nil || len(history) != 1 || string(history[0]) != `{"original":true}` {
		t.Fatalf("prior history changed: %q %v", history, err)
	}
	if ok, err := s.CommitDisposition(ctx, "item", before, after, "new", []byte(`{"accepted":true}`)); err != nil || !ok {
		t.Fatalf("valid retry failed: %v %v", ok, err)
	}
}

func TestCommitDispositionLostSnapshotWritesNothing(t *testing.T) {
	s := openDispositionTestDB(t)
	ctx := context.Background()
	before := []byte(`{"state":"pending"}`)
	winner := []byte(`{"state":"disposed","verdict":"reject"}`)
	if err := s.PutDispositionItem(ctx, "item", winner); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.CommitDisposition(ctx, "item", before, []byte(`{"state":"disposed","verdict":"accept"}`), "loser", []byte(`{"verdict":"accept"}`)); err != nil || ok {
		t.Fatalf("lost snapshot committed: %v %v", ok, err)
	}
	actual, _, err := s.DispositionItem(ctx, "item")
	if err != nil || !bytes.Equal(actual, winner) {
		t.Fatalf("winner changed: %q %v", actual, err)
	}
	history, err := s.DispositionHistory(ctx)
	if err != nil || len(history) != 0 {
		t.Fatalf("losing history appended: %q %v", history, err)
	}
}

func TestCommitDispositionComparesBytesAcrossSQLiteStorageTypes(t *testing.T) {
	for _, storage := range []string{"blob", "text"} {
		t.Run(storage, func(t *testing.T) {
			s := openDispositionTestDB(t)
			ctx := context.Background()
			before := []byte("{\n  \"state\": \"pending\", \"note\": \"café\"\n}")
			after := []byte(`{"state":"disposed","verdict":"accept"}`)
			if err := s.PutDispositionItem(ctx, "item", before); err != nil {
				t.Fatal(err)
			}
			if storage == "text" {
				if _, err := s.db.ExecContext(ctx, `UPDATE disposition_items SET payload=CAST(payload AS TEXT) WHERE id='item'`); err != nil {
					t.Fatal(err)
				}
			}
			var actualType string
			if err := s.db.QueryRowContext(ctx, `SELECT typeof(payload) FROM disposition_items WHERE id='item'`).Scan(&actualType); err != nil || actualType != storage {
				t.Fatalf("storage=%s err=%v", actualType, err)
			}
			// Identical JSON values with different bytes must still lose the CAS.
			equivalent := []byte(`{"state":"pending","note":"café"}`)
			if ok, err := s.CommitDisposition(ctx, "item", equivalent, after, "wrong", []byte(`{"wrong":true}`)); err != nil || ok {
				t.Fatalf("semantic equality weakened byte CAS: %v %v", ok, err)
			}
			if ok, err := s.CommitDisposition(ctx, "item", before, after, "winner", []byte(`{"winner":true}`)); err != nil || !ok {
				t.Fatalf("readable %s row cannot commit: %v %v", storage, ok, err)
			}
			if ok, err := s.CommitDisposition(ctx, "item", before, []byte(`{"state":"disposed","verdict":"reject"}`), "loser", []byte(`{"loser":true}`)); err != nil || ok {
				t.Fatalf("stale snapshot won: %v %v", ok, err)
			}
			history, err := s.DispositionHistory(ctx)
			if err != nil || len(history) != 1 || string(history[0]) != `{"winner":true}` {
				t.Fatalf("history=%q err=%v", history, err)
			}
		})
	}
}
