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
