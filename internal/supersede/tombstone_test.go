package supersede

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// openTombstoneTestStore mirrors dirtyedit_test.go's own inline disposition
// store setup: a real *index.SQLite backing a *disposition.Store, so
// Create/List/Dispose behave exactly as the CLI/production path does.
func openTombstoneTestStore(t *testing.T, root string) (*disposition.Store, *index.SQLite) {
	t.Helper()
	dbPath := filepath.Join(root, ".serenity", "index.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	eng, err := index.Open(dbPath)
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	return disposition.NewStore(eng), eng
}

// TestTombstoneCascadeSoleProvenanceRetractedAndSurvivesRebuild is T2.21's
// acc line's sole-provenance half: "tombstoning a fixture source creates
// one retraction item per sole-provenance claim; accept marks them
// retracted in files, rebuild drops them from the index."
func TestTombstoneCascadeSoleProvenanceRetractedAndSurvivesRebuild(t *testing.T) {
	root, run := gitRepoFixture(t)
	fw := store.NewFenceWriter(root)
	ss := store.NewShardStore(root)
	q := writer.NewQueue(nil)
	defer q.Close()
	ctx := context.Background()

	// A fixture source, written for real so Tombstone's sha is genuine
	// content-addressed identity, not a made-up string.
	srcStore := store.NewSourceStore(root)
	src, err := srcStore.Write([]byte("acme-corp's Q3 balance statement\n"), domain.Source{Kind: "file", URI: "file:///q3.txt"})
	if err != nil {
		t.Fatalf("write source: %v", err)
	}

	claim := claimFixture("sole-claim", "acme-corp", "has_balance", "$500")
	claim.Provenance.SourceSHA256 = src.SHA256
	claim.ID = store.DerivedID(claim.SubjectSlug, claim.Predicate, claim.ObjectKey, claim.ValidFrom, claim.Provenance.SourceSHA256, store.DefaultIDWidth)
	if err := ss.Append(claim); err != nil {
		t.Fatalf("seed shard: %v", err)
	}
	headRow := claim
	headRow.SourceRef = "shard"
	page := store.NewEntityPage(domain.Entity{Type: "topic", Slug: "acme-corp"})
	page.Claims = []domain.Claim{headRow}
	if _, err := fw.WriteEntity(page); err != nil {
		t.Fatalf("seed entity page: %v", err)
	}
	run("add", ".")
	run("commit", "--quiet", "-m", "seed")

	dispStore, eng := openTombstoneTestStore(t, root)
	w := New(q, fw, ss, config.Default())

	retracted, demoted, err := w.TombstoneCascade(ctx, dispStore, srcStore, src.SHA256, fixedNow)
	if err != nil {
		t.Fatalf("TombstoneCascade: %v", err)
	}
	if retracted != 1 || demoted != 0 {
		t.Fatalf("TombstoneCascade = (retracted=%d, demoted=%d), want (1, 0)", retracted, demoted)
	}

	items, err := dispStore.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var tombItem disposition.Item
	found := 0
	for _, it := range items {
		if it.Kind == disposition.KindTombstone {
			tombItem = it
			found++
		}
	}
	if found != 1 {
		t.Fatalf("found %d KindTombstone item(s) among %d total, want exactly 1", found, len(items))
	}
	var payload TombstonePayload
	if err := json.Unmarshal(tombItem.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if payload.Claim.ID != claim.ID || payload.SourceSHA256 != src.SHA256 {
		t.Fatalf("payload = %+v, want claim %s / source %s", payload, claim.ID, src.SHA256)
	}

	// "accept" -- disposing the item and applying it.
	disposeRes, err := dispStore.Dispose(ctx, tombItem.ID, disposition.VerdictAccept, nil, "", "human:tester", "", fixedNow)
	if err != nil {
		t.Fatalf("Dispose: %v", err)
	}
	result, err := w.ApplyDisposedTombstone(disposeRes.Item, fixedNow)
	if err != nil {
		t.Fatalf("ApplyDisposedTombstone: %v", err)
	}
	if result.Tier != domain.TierShard {
		t.Fatalf("Tier = %q, want %q", result.Tier, domain.TierShard)
	}
	if _, err := writer.Flush(q, root); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// "marks them retracted in files": the shard gains a retraction line
	// reusing the original claim's id.
	lines, err := ss.Lines("acme-corp", "has_balance")
	if err != nil {
		t.Fatalf("Lines: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("shard has %d line(s), want 2 (original + retraction)", len(lines))
	}
	tombLine := lines[1]
	if tombLine.ID != claim.ID {
		t.Fatalf("retraction line ID = %q, want %q (reuses target's id)", tombLine.ID, claim.ID)
	}
	if tombLine.State != domain.StateRetracted {
		t.Fatalf("retraction line State = %q, want %q", tombLine.State, domain.StateRetracted)
	}
	if tombLine.Provenance.Actor != "human:tester" {
		t.Fatalf("retraction line Provenance.Actor = %q, want %q", tombLine.Provenance.Actor, "human:tester")
	}

	heads, err := ss.ResolveHeads("acme-corp", "has_balance")
	if err != nil {
		t.Fatalf("ResolveHeads: %v", err)
	}
	if _, ok := heads[claim.ObjectKey]; ok {
		t.Fatalf("resolved heads still contain the retracted claim's key %q: %+v", claim.ObjectKey, heads)
	}

	if err := eng.Close(); err != nil {
		t.Fatalf("close index: %v", err)
	}

	// "rebuild drops them from the index": wipe .serenity entirely and
	// rebuild purely from committed repo bytes -- the same
	// wipe-and-rebuild move as T2.4's own acc-line test and
	// internal/index/rebuild_test.go's TestWipeAndRebuildInvariant.
	if err := os.RemoveAll(filepath.Join(root, ".serenity")); err != nil {
		t.Fatalf("wipe .serenity: %v", err)
	}
	dbPath := filepath.Join(root, ".serenity", "index.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	eng2, err := index.Open(dbPath)
	if err != nil {
		t.Fatalf("reopen index: %v", err)
	}
	defer func() { _ = eng2.Close() }()
	if err := index.Rebuild(ctx, root, config.Default(), eng2); err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	dump, err := index.DumpString(ctx, eng2)
	if err != nil {
		t.Fatalf("DumpString: %v", err)
	}
	if strings.Contains(dump, "$500") {
		t.Fatalf("retracted claim's value leaked into the post-wipe rebuild:\n%s", dump)
	}
	if strings.Contains(dump, claim.ID) {
		t.Fatalf("retracted claim's id %s leaked into the post-wipe rebuild:\n%s", claim.ID, dump)
	}
}

// TestTombstoneCascadeMultiProvenanceDemotedNotRetracted is T2.21's acc
// line's other half: "multi-provenance claims are demoted not retracted."
// Two independent claims corroborate the same fact from two different
// sources; tombstoning only one source must not retract the (still
// corroborated) claim -- it demotes it via a fresh, lower-confidence
// supersession instead, and stages no disposition item at all.
func TestTombstoneCascadeMultiProvenanceDemotedNotRetracted(t *testing.T) {
	root, run := gitRepoFixture(t)
	fw := store.NewFenceWriter(root)
	ss := store.NewShardStore(root)
	q := writer.NewQueue(nil)
	defer q.Close()
	ctx := context.Background()

	srcStore := store.NewSourceStore(root)
	srcA, err := srcStore.Write([]byte("bob-lee's statement, source A\n"), domain.Source{Kind: "file", URI: "file:///a.txt"})
	if err != nil {
		t.Fatalf("write source A: %v", err)
	}
	srcB, err := srcStore.Write([]byte("bob-lee's statement, source B\n"), domain.Source{Kind: "file", URI: "file:///b.txt"})
	if err != nil {
		t.Fatalf("write source B: %v", err)
	}

	claimA := claimFixture("claim-a", "bob-lee", "has_balance", "$300")
	claimA.Provenance.SourceSHA256 = srcA.SHA256
	claimB := claimFixture("claim-b", "bob-lee", "has_balance", "$300") // same object/key, independent source
	claimB.Provenance.SourceSHA256 = srcB.SHA256
	if err := ss.Append(claimA); err != nil {
		t.Fatalf("seed claim A: %v", err)
	}
	if err := ss.Append(claimB); err != nil {
		t.Fatalf("seed claim B: %v", err)
	}
	run("add", ".")
	run("commit", "--quiet", "-m", "seed")

	dispStore, eng := openTombstoneTestStore(t, root)
	defer func() { _ = eng.Close() }()
	w := New(q, fw, ss, config.Default())

	retracted, demoted, err := w.TombstoneCascade(ctx, dispStore, srcStore, srcA.SHA256, fixedNow)
	if err != nil {
		t.Fatalf("TombstoneCascade: %v", err)
	}
	if retracted != 0 || demoted != 1 {
		t.Fatalf("TombstoneCascade = (retracted=%d, demoted=%d), want (0, 1)", retracted, demoted)
	}

	items, err := dispStore.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, it := range items {
		if it.Kind == disposition.KindTombstone {
			t.Fatalf("unexpected KindTombstone item staged for a multi-provenance claim: %+v", it)
		}
	}

	if _, err := writer.Flush(q, root); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	lines, err := ss.Lines("bob-lee", "has_balance")
	if err != nil {
		t.Fatalf("Lines: %v", err)
	}
	if len(lines) != 3 {
		t.Fatalf("shard has %d line(s), want 3 (claim A, claim B, demoted copy of A)", len(lines))
	}
	demotedLine := lines[2]
	if demotedLine.Supersedes != claimA.ID {
		t.Fatalf("demoted line Supersedes = %q, want %q", demotedLine.Supersedes, claimA.ID)
	}
	if demotedLine.State != domain.StateActive {
		t.Fatalf("demoted line State = %q, want %q (demotion is not retraction)", demotedLine.State, domain.StateActive)
	}
	if got, want := demotedLine.Confidence, claimA.Confidence*demoteFactor; got != want {
		t.Fatalf("demoted line Confidence = %v, want %v (%v * demoteFactor)", got, want, claimA.Confidence)
	}
	if demotedLine.Object != claimA.Object {
		t.Fatalf("demoted line Object = %q, want %q (demotion keeps the same fact)", demotedLine.Object, claimA.Object)
	}
}

func TestApplyDisposedTombstoneRejectsUndisposedItem(t *testing.T) {
	root, _ := gitRepoFixture(t)
	fw := store.NewFenceWriter(root)
	ss := store.NewShardStore(root)
	q := writer.NewQueue(nil)
	defer q.Close()
	w := New(q, fw, ss, config.Default())

	item := disposition.Item{ID: "t1", Kind: disposition.KindTombstone, State: disposition.StatePending}
	if _, err := w.ApplyDisposedTombstone(item, fixedNow); err == nil {
		t.Fatal("expected an error for a pending (undisposed) item")
	}
}

func TestApplyDisposedTombstoneRejectsWrongKind(t *testing.T) {
	root, _ := gitRepoFixture(t)
	fw := store.NewFenceWriter(root)
	ss := store.NewShardStore(root)
	q := writer.NewQueue(nil)
	defer q.Close()
	w := New(q, fw, ss, config.Default())

	item := disposition.Item{
		ID: "r1", Kind: disposition.KindReconcile,
		State: disposition.StateDisposed, Verdict: disposition.VerdictAccept,
	}
	if _, err := w.ApplyDisposedTombstone(item, fixedNow); err == nil {
		t.Fatal("expected an error for a non-tombstone kind")
	}
}
