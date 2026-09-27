package index

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/store"
)

func factRowCounts(t *testing.T, eng *SQLite, sha string) (chunks, vectors int) {
	t.Helper()
	ctx := context.Background()
	ref := "fact:" + sha
	if err := eng.db.QueryRowContext(ctx, "SELECT count(*) FROM chunks WHERE chunk_ref = ?", ref).Scan(&chunks); err != nil {
		t.Fatal(err)
	}
	if err := eng.db.QueryRowContext(ctx, "SELECT count(*) FROM vectors WHERE chunk_ref = ?", ref).Scan(&vectors); err != nil {
		t.Fatal(err)
	}
	return chunks, vectors
}

func seedFactRows(t *testing.T, eng *SQLite, sha string) {
	t.Helper()
	ctx := context.Background()
	if err := eng.InsertChunk(ctx, "fact:"+sha, "", "egress marker", sha, store.SourceKindMemoryFact); err != nil {
		t.Fatal(err)
	}
	if err := eng.UpsertVector(ctx, "fact:"+sha, "memory-boundary@v1", []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
}

func expireFact(t *testing.T, root, sha string) {
	t.Helper()
	if _, err := store.NewSourceStore(root).WriteMemoryExpiry(store.MemoryExpiryPayload{FormatVersion: store.MemoryFactFormatVersion, RecordType: store.SourceKindMemoryExpiry, TargetSHA256: sha, ExpiredAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
}

// TestRecoverMemorySearchDropsExpiredAndErasedFactRows: recovery is the
// hosted repair pass, so it must delete the FTS and vector rows of a fact
// that is expired, or whose canonical directory forget already removed.
func TestRecoverMemorySearchDropsExpiredAndErasedFactRows(t *testing.T) {
	t.Run("expired", func(t *testing.T) {
		root, eng := sourcePolicyIndex(t)
		sha := memorySearchSource(t, root, nil)
		seedFactRows(t, eng, sha)
		expireFact(t, root, sha)
		if err := RecoverMemorySearch(context.Background(), root, eng, nil); err != nil {
			t.Fatal(err)
		}
		if c, v := factRowCounts(t, eng, sha); c != 0 || v != 0 {
			t.Fatalf("expired fact rows survived recovery: chunks=%d vectors=%d", c, v)
		}
	})
	t.Run("erased", func(t *testing.T) {
		root, eng := sourcePolicyIndex(t)
		sources := store.NewSourceStore(root)
		sha := memorySearchSource(t, root, nil)
		seedFactRows(t, eng, sha)
		expireFact(t, root, sha)
		if err := os.RemoveAll(sources.DirFor(sha)); err != nil {
			t.Fatal(err)
		}
		if err := RecoverMemorySearch(context.Background(), root, eng, nil); err != nil {
			t.Fatal(err)
		}
		if c, v := factRowCounts(t, eng, sha); c != 0 || v != 0 {
			t.Fatalf("erased fact rows survived recovery: chunks=%d vectors=%d", c, v)
		}
	})
}

// TestRebuildYieldsNoRowsForExpiredFact is the contract's rebuild case.
func TestRebuildYieldsNoRowsForExpiredFact(t *testing.T) {
	root, eng := sourcePolicyIndex(t)
	sha := memorySearchSource(t, root, nil)
	seedFactRows(t, eng, sha)
	expireFact(t, root, sha)
	if err := Rebuild(context.Background(), root, config.Default(), eng); err != nil {
		t.Fatal(err)
	}
	if c, v := factRowCounts(t, eng, sha); c != 0 || v != 0 {
		t.Fatalf("rebuild kept expired fact rows: chunks=%d vectors=%d", c, v)
	}
}
