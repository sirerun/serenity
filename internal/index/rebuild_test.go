package index

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

// scaffoldBrain builds a small canonical repo: one entity page with
// fence-tier claims plus a DIVERGED shard-head fence row, and a shard
// whose resolution disagrees with that fence row. This is the
// fence/shard-disagreement fixture required by the v2.2 changelog: the
// rebuilt state must derive from the shard (§7.2a authority rule).
func scaffoldBrain(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	fw := store.NewFenceWriter(root)
	p := store.NewEntityPage(domain.Entity{Type: "account", Slug: "checking"})
	p.Summary = "Primary checking account."
	p.Claims = []domain.Claim{
		{ID: "aaaa1111", SubjectSlug: "checking", Predicate: "owns_account", Family: "owns_account",
			Object: "first-national", Confidence: 0.9, State: domain.StateActive, SourceRef: "e1#1"},
		// Shard-tier head row, hand-edited to a STALE value — the shard
		// below says 200.00; rebuild must believe the shard.
		{ID: "bbbb2222", SubjectSlug: "checking", Predicate: "has_balance", Family: "has_balance",
			Object: "999.99 usd", Confidence: 0.9, State: domain.StateActive, SourceRef: "shard"},
	}
	if _, err := fw.WriteEntity(p); err != nil {
		t.Fatal(err)
	}

	ss := store.NewShardStore(root)
	obs := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	c1 := domain.Claim{ID: "bbbb2222", SubjectSlug: "checking", Predicate: "has_balance", Family: "has_balance",
		Object: "100.00 usd", ObjectKey: "balance", Confidence: 0.9, State: domain.StateActive,
		Provenance: domain.Provenance{ObservedAt: obs}}
	c2 := domain.Claim{ID: "cccc3333", SubjectSlug: "checking", Predicate: "has_balance", Family: "has_balance",
		Object: "200.00 usd", ObjectKey: "balance", Confidence: 0.9, State: domain.StateActive, Supersedes: "bbbb2222",
		Provenance: domain.Provenance{ObservedAt: obs.Add(time.Hour)}}
	for _, c := range []domain.Claim{c1, c2} {
		if err := ss.Append(c); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// TestWipeAndRebuildInvariant is the M0 acceptance criterion: wipe
// .serenity/, rebuild from repo bytes, identical derived state (within
// the pinned model set — no models are pinned in this fixture).
func TestWipeAndRebuildInvariant(t *testing.T) {
	ctx := context.Background()
	root := scaffoldBrain(t)
	cfg := config.Default()

	dbPath := filepath.Join(root, ".serenity", "index.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	eng, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := Rebuild(ctx, root, cfg, eng); err != nil {
		t.Fatal(err)
	}
	dump1, err := DumpString(ctx, eng)
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}

	// Wipe the entire derived directory and rebuild from repo bytes.
	if err := os.RemoveAll(filepath.Join(root, ".serenity")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	eng2, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer eng2.Close()
	if err := Rebuild(ctx, root, cfg, eng2); err != nil {
		t.Fatal(err)
	}
	dump2, err := DumpString(ctx, eng2)
	if err != nil {
		t.Fatal(err)
	}

	if dump1 != dump2 {
		t.Fatalf("rebuild not identical\n--- before wipe ---\n%s\n--- after wipe ---\n%s", dump1, dump2)
	}
	if dump1 == "" {
		t.Fatal("dump empty — rebuild indexed nothing")
	}
}

// TestShardAuthorityOverFenceHead: the fence head says 999.99 (stale hand
// edit), the shard resolves to 200.00 — the index must contain the shard's
// value and must NOT contain the fence's diverged value (§7.2a).
func TestShardAuthorityOverFenceHead(t *testing.T) {
	ctx := context.Background()
	root := scaffoldBrain(t)
	cfg := config.Default()

	dbPath := filepath.Join(root, ".serenity", "index.db")
	os.MkdirAll(filepath.Dir(dbPath), 0o755)
	eng, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	if err := Rebuild(ctx, root, cfg, eng); err != nil {
		t.Fatal(err)
	}
	dump, err := DumpString(ctx, eng)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(dump, "999.99") {
		t.Fatalf("diverged fence head leaked into the index:\n%s", dump)
	}
	if !strings.Contains(dump, "200.00 usd") {
		t.Fatalf("shard-resolved head missing from the index:\n%s", dump)
	}
	if !strings.Contains(dump, "cccc3333") {
		t.Fatalf("expected shard head id cccc3333 in index:\n%s", dump)
	}
	// Fence-tier claims still index from the fence (the file is truth).
	if !strings.Contains(dump, "first-national") {
		t.Fatalf("fence-tier claim missing from the index:\n%s", dump)
	}

	stats, err := eng.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats["entities"] != 1 || stats["claims"] != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestSearchFTS(t *testing.T) {
	ctx := context.Background()
	root := scaffoldBrain(t)
	dbPath := filepath.Join(root, ".serenity", "index.db")
	os.MkdirAll(filepath.Dir(dbPath), 0o755)
	eng, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	if err := Rebuild(ctx, root, config.Default(), eng); err != nil {
		t.Fatal(err)
	}
	hits, err := eng.SearchFTS(ctx, "checking", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) == 0 || hits[0].EntitySlug != "checking" {
		t.Fatalf("expected FTS hit for 'checking', got %+v", hits)
	}
}
