package index

import (
	"context"
	"path/filepath"
	"testing"
)

// TestSourcePathsStayInLocalIndex covers T24.24's local half: the absolute
// path a path_hash stands for is recorded in the derived, never-committed
// index, re-recording the same hash is idempotent, and an unknown hash
// reports not found.
func TestSourcePathsStayInLocalIndex(t *testing.T) {
	ctx := context.Background()
	eng, err := Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })

	const hash = "0000000000000000000000000000000000000000000000000000000000000001"
	for range 2 {
		if err := eng.RecordSourcePath(ctx, hash, "/srv/example/notes/plan.txt"); err != nil {
			t.Fatalf("RecordSourcePath: %v", err)
		}
	}
	got, ok, err := eng.SourcePath(ctx, hash)
	if err != nil || !ok || got != "/srv/example/notes/plan.txt" {
		t.Fatalf("SourcePath = %q, %v, %v", got, ok, err)
	}
	if _, ok, err := eng.SourcePath(ctx, "ff"); err != nil || ok {
		t.Fatalf("unknown hash: ok=%v err=%v", ok, err)
	}
}
