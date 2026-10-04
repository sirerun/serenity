package gateway

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/pool"
)

func TestInspectorReadColdOwnedBrainDoesNotOpenOrMutateRuntime(t *testing.T) {
	g, brainPool, _, accountID, brainID, brainRoot := newLifecycleExportFixture(t)
	before, err := os.ReadDir(brainRoot)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	err = g.WithInspectorRead(context.Background(), accountID, brainID, func(view InspectorReadView) error {
		called = true
		if view.Root != brainRoot || view.Now.IsZero() {
			t.Fatalf("view = %#v", view)
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("cold fenced read: called=%v err=%v", called, err)
	}
	if _, _, err = brainPool.AcquireExistingForRead(context.Background(), brainID); !errors.Is(err, pool.ErrNotOpen) {
		t.Fatalf("cold read opened runtime: %v", err)
	}
	after, err := os.ReadDir(brainRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("cold read changed brain entries: before=%v after=%v", entryNames(before), entryNames(after))
	}
	for i := range before {
		if before[i].Name() != after[i].Name() {
			t.Fatalf("cold read changed brain entries: before=%v after=%v", entryNames(before), entryNames(after))
		}
	}
}

func TestInspectorReadRejectsCrossOwnerAndUnsafeCanonicalRoot(t *testing.T) {
	g, _, db, accountID, brainID, brainRoot := newLifecycleExportFixture(t)
	other, err := db.CreateAccount(context.Background(), "inspector-other@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err = g.WithInspectorRead(context.Background(), other.ID, brainID, func(InspectorReadView) error { t.Fatal("cross-owner callback ran"); return nil }); !errors.Is(err, ErrInspectorBrainUnavailable) {
		t.Fatalf("cross-owner read error = %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err = os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(brainRoot, outside+"/brain"); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside+"/brain", brainRoot); err != nil {
		t.Fatal(err)
	}
	if err = g.WithInspectorRead(context.Background(), accountID, brainID, func(InspectorReadView) error { t.Fatal("symlink callback ran"); return nil }); err == nil {
		t.Fatal("symlinked canonical root was accepted")
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}
