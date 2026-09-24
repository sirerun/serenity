package deletion

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/contracts/contractstest"
)

func TestJournalContract(t *testing.T) {
	contractstest.RunJournalSuite(t, func(store contracts.JournalObjectStore, writer string, generation int64, now func() time.Time) contracts.DeletionJournal {
		return NewJournal(store, writer, generation, now)
	})
}

func TestFileObjectStoreIsExclusiveAndPersists(t *testing.T) {
	root := filepath.Join(t.TempDir(), "journal")
	store, err := NewFileObjectStore(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	key := contracts.JournalKey(1, 1)
	if created, err := store.PutIfAbsent(ctx, key, []byte("first")); err != nil || !created {
		t.Fatalf("first put = %v, %v", created, err)
	}
	if created, err := store.PutIfAbsent(ctx, key, []byte("second")); err != nil || created {
		t.Fatalf("duplicate put = %v, %v", created, err)
	}
	reopened, err := NewFileObjectStore(root)
	if err != nil {
		t.Fatal(err)
	}
	body, found, err := reopened.Get(ctx, key)
	if err != nil || !found || string(body) != "first" {
		t.Fatalf("reopened get = %q, %v, %v", body, found, err)
	}
	keys, more, err := reopened.ListAfter(ctx, contracts.JournalPrefix(1), "", 1)
	if err != nil || more || len(keys) != 1 || keys[0] != key {
		t.Fatalf("list = %v, %v, %v", keys, more, err)
	}
	if _, err := store.PutIfAbsent(ctx, "deletion-journal/../../escape", []byte("bad")); err == nil {
		t.Fatal("path traversal key accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "escape")); !os.IsNotExist(err) {
		t.Fatalf("traversal created outside journal root: %v", err)
	}
}
