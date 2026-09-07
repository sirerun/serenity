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

// TestApplyDisposedDirtyEditShardGainsLineHeadRegeneratesAndSurvivesRebuild
// is T2.4's acc line, extending the M0 fence/shard-disagreement fixture
// (internal/index/rebuild_test.go's scaffoldBrain, "the shard is the
// resolved state, RFC §7.2a") with the new step it does not itself cover:
// a human hand-editing the shard-head fence row is not a stale/wrong
// value to be overruled by the shard (scaffoldBrain's own scenario) but a
// genuine correction that, once *accepted* through the DISPOSITION queue,
// becomes durable IN the shard -- "the human edit survives wipe-and-
// rebuild via the shard."
func TestApplyDisposedDirtyEditShardGainsLineHeadRegeneratesAndSurvivesRebuild(t *testing.T) {
	root, run := gitRepoFixture(t)
	fw := store.NewFenceWriter(root)
	ss := store.NewShardStore(root)
	q := writer.NewQueue(nil)
	defer q.Close()
	ctx := context.Background()

	// The shard's real, already-resolved head: $500 -- committed, the
	// last-known-good state a human's edit and a later extract run would
	// both start from.
	orig := claimFixture("orig-head", "acme-corp", "has_balance", "$500")
	if err := ss.Append(orig); err != nil {
		t.Fatalf("seed shard: %v", err)
	}
	headRow := orig
	headRow.SourceRef = "shard"
	page := store.NewEntityPage(domain.Entity{Type: "topic", Slug: "acme-corp"})
	page.Claims = []domain.Claim{headRow}
	if _, err := fw.WriteEntity(page); err != nil {
		t.Fatalf("seed entity page: %v", err)
	}
	run("add", ".")
	run("commit", "--quiet", "-m", "seed b")

	// "edit a shard-head fence row": a direct filesystem write to the
	// already-committed fence page -- not through writer.Fence/the queue,
	// exactly what a human editing the markdown file by hand does -- left
	// uncommitted (git-dirty), correcting the balance to $650.
	fencePath := fw.PathFor("topic", "acme-corp")
	committed, err := fw.ParseEntity(fencePath)
	if err != nil {
		t.Fatalf("re-parse committed page: %v", err)
	}
	edited := *committed
	edited.Claims = append([]domain.Claim(nil), committed.Claims...)
	edited.Claims[0].Object = "$650"
	editedBytes, err := fw.RenderEntity(&edited)
	if err != nil {
		t.Fatalf("render human edit: %v", err)
	}
	if err := os.WriteFile(fencePath, editedBytes, 0o644); err != nil {
		t.Fatalf("write human edit: %v", err)
	}

	// "run extract": a machine write attempt against the same page while
	// it carries the uncommitted human edit hits the dirty-tree guard
	// (T0.4) and pauses rather than overwriting it.
	if _, _, err := writer.Fence(q, fw, committed); err != writer.ErrDirtyTree {
		t.Fatalf("writer.Fence against a dirty page = %v, want ErrDirtyTree", err)
	}
	pendingPath := writer.PendingPath(root, "acme-corp")
	if _, err := os.Stat(pendingPath); err != nil {
		t.Fatalf("expected a pending record at %s: %v", pendingPath, err)
	}

	// T2.1's Store.ImportPending sweeps it into a dirty_edit disposition
	// item.
	dbPath := filepath.Join(root, ".serenity", "index.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	eng, err := index.Open(dbPath)
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	dispStore := disposition.NewStore(eng)

	n, err := dispStore.ImportPending(ctx, root, fixedNow)
	if err != nil {
		t.Fatalf("ImportPending: %v", err)
	}
	if n != 1 {
		t.Fatalf("ImportPending imported %d record(s), want 1", n)
	}
	if _, err := os.Stat(pendingPath); !os.IsNotExist(err) {
		t.Fatalf("pending record %s still on disk after import", pendingPath)
	}

	items, err := dispStore.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var dirtyItem disposition.Item
	found := false
	for _, it := range items {
		if it.Kind == disposition.KindDirtyEdit {
			dirtyItem, found = it, true
		}
	}
	if !found {
		t.Fatalf("no dirty_edit item found among %d item(s)", len(items))
	}

	// "accept -> shard gains a line with actor human:<id> and the head
	// regenerates from the shard":
	disposeRes, err := dispStore.Dispose(ctx, dirtyItem.ID, disposition.VerdictAccept, nil, "", "human:tester", "", fixedNow)
	if err != nil {
		t.Fatalf("Dispose: %v", err)
	}

	w := New(q, fw, ss, config.Default())
	results, err := w.ApplyDisposedDirtyEdit(disposeRes.Item, fixedNow)
	if err != nil {
		t.Fatalf("ApplyDisposedDirtyEdit: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("ApplyDisposedDirtyEdit returned %d result(s), want 1 (only has_balance diverges)", len(results))
	}
	if results[0].Tier != domain.TierShard {
		t.Fatalf("Tier = %q, want %q", results[0].Tier, domain.TierShard)
	}

	if _, err := writer.Flush(q, root); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	lines, err := ss.Lines("acme-corp", "has_balance")
	if err != nil {
		t.Fatalf("Lines: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("shard has %d line(s), want 2 (orig + the accepted human edit)", len(lines))
	}
	newLine := lines[1]
	if newLine.Object != "$650" {
		t.Fatalf("appended line Object = %q, want %q", newLine.Object, "$650")
	}
	if newLine.Supersedes != "orig-head" {
		t.Fatalf("appended line Supersedes = %q, want %q", newLine.Supersedes, "orig-head")
	}
	if newLine.Provenance.Actor != "human:tester" {
		t.Fatalf("appended line Provenance.Actor = %q, want %q", newLine.Provenance.Actor, "human:tester")
	}

	p, err := fw.ParseEntity(results[0].FencePath)
	if err != nil {
		t.Fatalf("ParseEntity: %v", err)
	}
	if len(p.Claims) != 1 || p.Claims[0].Object != "$650" || p.Claims[0].SourceRef != "shard" {
		t.Fatalf("regenerated fence head = %+v, want a single $650/src=shard row", p.Claims)
	}

	if err := eng.Close(); err != nil {
		t.Fatalf("close index: %v", err)
	}

	// "the M0 disagreement fixture extended: the human edit survives
	// wipe-and-rebuild via the shard" -- wipe the entire derived
	// directory (exactly TestWipeAndRebuildInvariant's own move) and
	// rebuild purely from committed repo bytes. The accepted edit is only
	// really durable if it now lives in the shard, not just the fence
	// cache -- this is what actually distinguishes "accepted" from
	// scaffoldBrain's own stale/never-accepted hand edit, which
	// TestShardAuthorityOverFenceHead proves the rebuild must NOT trust.
	if err := os.RemoveAll(filepath.Join(root, ".serenity")); err != nil {
		t.Fatalf("wipe .serenity: %v", err)
	}
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
	if !strings.Contains(dump, "$650") {
		t.Fatalf("accepted human edit missing from the post-wipe rebuild:\n%s", dump)
	}
	if !strings.Contains(dump, newLine.ID) {
		t.Fatalf("accepted line's id %s missing from the post-wipe rebuild:\n%s", newLine.ID, dump)
	}
	if strings.Contains(dump, "$500") {
		t.Fatalf("superseded pre-edit value leaked into the post-wipe rebuild:\n%s", dump)
	}
}

// TestApplyDisposedDirtyEditSkipsFenceTierRows: a fence-tier row on the
// human's edited page is already truth on disk (§7.2) -- there is nothing
// for this function to reconcile there, and it must not try.
func TestApplyDisposedDirtyEditSkipsFenceTierRows(t *testing.T) {
	root, _ := gitRepoFixture(t)
	fw := store.NewFenceWriter(root)
	ss := store.NewShardStore(root)
	q := writer.NewQueue(nil)
	defer q.Close()

	page := store.NewEntityPage(domain.Entity{Type: "topic", Slug: "alice-tan"})
	page.Claims = []domain.Claim{claimFixture("claim-a", "alice-tan", "works_at", "acme-corp")}
	rendered, err := fw.RenderEntity(page)
	if err != nil {
		t.Fatalf("RenderEntity: %v", err)
	}
	fencePath := fw.PathFor("topic", "alice-tan")
	if err := os.MkdirAll(filepath.Dir(fencePath), 0o755); err != nil {
		t.Fatal(err)
	}
	// The human's hand edit really is on disk (uncommitted) -- CommitPath
	// needs a real file to stage, exactly as it would find one from a
	// genuine dirty-tree-guard pause.
	if err := os.WriteFile(fencePath, rendered, 0o644); err != nil {
		t.Fatalf("write fence file: %v", err)
	}

	rec := writer.PendingRecord{Path: fencePath, Human: string(rendered), Machine: string(rendered)}
	payload, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal PendingRecord: %v", err)
	}
	item := disposition.Item{
		ID: "dirty_edit:alice-tan", Kind: disposition.KindDirtyEdit,
		State: disposition.StateDisposed, Verdict: disposition.VerdictAccept,
		Actor: "human:tester", Payload: payload,
	}

	w := New(q, fw, ss, config.Default())
	results, err := w.ApplyDisposedDirtyEdit(item, fixedNow)
	if err != nil {
		t.Fatalf("ApplyDisposedDirtyEdit: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results for an all-fence-tier page, got %d: %+v", len(results), results)
	}
}

func TestApplyDisposedDirtyEditRejectsUndisposedItem(t *testing.T) {
	root, _ := gitRepoFixture(t)
	fw := store.NewFenceWriter(root)
	ss := store.NewShardStore(root)
	q := writer.NewQueue(nil)
	defer q.Close()
	w := New(q, fw, ss, config.Default())

	item := disposition.Item{ID: "dirty_edit:acme-corp", Kind: disposition.KindDirtyEdit, State: disposition.StatePending}
	if _, err := w.ApplyDisposedDirtyEdit(item, fixedNow); err == nil {
		t.Fatal("ApplyDisposedDirtyEdit on a still-pending item succeeded, want an error")
	}
}

func TestApplyDisposedDirtyEditRejectsWrongKind(t *testing.T) {
	root, _ := gitRepoFixture(t)
	fw := store.NewFenceWriter(root)
	ss := store.NewShardStore(root)
	q := writer.NewQueue(nil)
	defer q.Close()
	w := New(q, fw, ss, config.Default())

	item := disposition.Item{ID: "x", Kind: disposition.KindReconcile, State: disposition.StateDisposed, Verdict: disposition.VerdictAccept}
	if _, err := w.ApplyDisposedDirtyEdit(item, fixedNow); err == nil {
		t.Fatal("ApplyDisposedDirtyEdit on a non-dirty_edit item succeeded, want an error")
	}
}
