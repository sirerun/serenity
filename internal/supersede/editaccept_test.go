package supersede

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/reconcile"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// dispositionStoreFixture opens a real disposition.Store backed by the
// same *index.SQLite providers.OpenIndex hands runInbox in production
// (internal/cli/inbox.go, T2.5/T2.7) -- not a synthetic double -- rooted
// at root (a gitRepoFixture's tempdir). No serenity.yml scaffold is
// required: providers.OpenIndex only needs root/.serenity to exist, which
// it creates itself.
func dispositionStoreFixture(t *testing.T, root string) *disposition.Store {
	t.Helper()
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return disposition.NewStore(eng)
}

// seedReconcileItem stages one KindReconcile item shaped exactly like
// internal/reconcile.Engine.Process's own output (T2.2): a is the
// machine-proposed winner (the value a human's edit_accept will later
// override), b is the prior active claim it conflicts with.
func seedReconcileItem(t *testing.T, dispStore *disposition.Store, ctx context.Context, now time.Time, a, b domain.Claim) disposition.Item {
	t.Helper()
	payload, err := json.Marshal(reconcile.ReconcilePayload{Verdict: reconcile.VerdictConflict, Reason: "test fixture", A: a, B: b})
	if err != nil {
		t.Fatalf("marshal ReconcilePayload: %v", err)
	}
	item, err := dispStore.Create(ctx, disposition.KindReconcile, payload, "", now)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	return item
}

// TestApplyDisposedReconcileShardTierEditAcceptProvenanceAndClaimID is
// T2.7's acc line on its shard-tier half: "an edited object lands in ...
// the shard with Provenance.Actor = human:<id>", "the disposition row
// references the new claim id", and "re-render byte-identical."
// "has_balance" is shard-tier in config.Default() (RFC §7.2a) -- the only
// tier where a claim's Provenance is actually persisted to disk at all
// (see the fence-tier test below for why that clause can't be checked
// there).
func TestApplyDisposedReconcileShardTierEditAcceptProvenanceAndClaimID(t *testing.T) {
	root, run := gitRepoFixture(t)
	fw := store.NewFenceWriter(root)
	ss := store.NewShardStore(root)
	q := writer.NewQueue(nil)
	defer q.Close()
	dispStore := dispositionStoreFixture(t, root)
	ctx := context.Background()

	old := claimFixture("claim-b", "acme-corp", "has_balance", "$500")
	if err := ss.Append(old); err != nil {
		t.Fatalf("seed shard: %v", err)
	}
	headRow := old
	headRow.SourceRef = "shard"
	page := store.NewEntityPage(domain.Entity{Type: "topic", Slug: "acme-corp"})
	page.Claims = []domain.Claim{headRow}
	if _, err := fw.WriteEntity(page); err != nil {
		t.Fatalf("seed entity page: %v", err)
	}
	run("add", ".")
	run("commit", "--quiet", "-m", "seed b")

	// draftA is T2.2's own staged proposal -- machine provenance, a value
	// the human is about to overwrite.
	draftA := claimFixture("claim-a-draft", "acme-corp", "has_balance", "$700")
	item := seedReconcileItem(t, dispStore, ctx, fixedNow, draftA, old)

	edited := draftA
	edited.Object = "$900" // the human's actual correction
	editedRaw, err := json.Marshal(edited)
	if err != nil {
		t.Fatalf("marshal edited: %v", err)
	}

	disposeRes, err := dispStore.Dispose(ctx, item.ID, disposition.VerdictEditAccept, editedRaw, "", "human:tester", "", fixedNow)
	if err != nil {
		t.Fatalf("Dispose: %v", err)
	}

	w := New(q, fw, ss, config.Default())
	res, err := w.ApplyDisposedReconcile(ctx, dispStore, disposeRes.Item, fixedNow)
	if err != nil {
		t.Fatalf("ApplyDisposedReconcile: %v", err)
	}
	if res.Tier != domain.TierShard {
		t.Fatalf("Tier = %q, want %q", res.Tier, domain.TierShard)
	}

	if _, err := writer.Flush(q, root); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	lines, err := ss.Lines("acme-corp", "has_balance")
	if err != nil {
		t.Fatalf("Lines: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("shard has %d line(s), want 2 (old + new)", len(lines))
	}
	newLine := lines[1]
	if newLine.Object != "$900" {
		t.Fatalf("appended line Object = %q, want %q (the human's edit, not the machine's draft)", newLine.Object, "$900")
	}
	if newLine.Supersedes != "claim-b" {
		t.Fatalf("appended line Supersedes = %q, want %q", newLine.Supersedes, "claim-b")
	}
	if newLine.Provenance.Actor != "human:tester" {
		t.Fatalf("appended line Provenance.Actor = %q, want %q (T2.7 acc line)", newLine.Provenance.Actor, "human:tester")
	}
	if newLine.ID == "claim-a-draft" {
		t.Fatal("appended line kept the stale pre-edit draft id -- the id must be recomputed from the edited object")
	}

	// "the disposition row references the new claim id" (T2.7 acc line).
	gotItem, err := dispStore.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotItem.AppliedClaimID != newLine.ID {
		t.Fatalf("AppliedClaimID = %q, want %q (the id ApplyDisposedReconcile actually wrote)", gotItem.AppliedClaimID, newLine.ID)
	}

	// "re-render byte-identical": the regenerated head-row fence page
	// parses and re-renders to the exact bytes on disk.
	p, err := fw.ParseEntity(res.FencePath)
	if err != nil {
		t.Fatalf("ParseEntity: %v", err)
	}
	onDisk, err := os.ReadFile(res.FencePath)
	if err != nil {
		t.Fatalf("read fence: %v", err)
	}
	rerendered, err := fw.RenderEntity(p)
	if err != nil {
		t.Fatalf("RenderEntity: %v", err)
	}
	if string(onDisk) != string(rerendered) {
		t.Fatalf("re-render not byte-identical:\non disk:\n%s\nrerendered:\n%s", onDisk, rerendered)
	}
}

// TestApplyDisposedReconcileFenceTierEditAccept is T2.7's acc line on its
// fence-tier half: "an edited object lands in the fence ..." and
// "re-render byte-identical." "works_at" is fence-tier in
// config.Default(). Provenance.Actor is deliberately NOT asserted on the
// on-disk row here -- TestApplyFenceTierStrikethroughAndPointer (T2.3)
// already established that the fence markdown table's 7 columns
// (id/predicate/object/conf/valid/src/state) carry no Provenance column
// at all, so no fence-tier claim's Provenance -- Actor included -- ever
// round-trips through ParseEntity/RenderEntity. That is the existing
// format's design, not something this task changes; the shard-tier test
// above is what genuinely verifies Provenance.Actor = human:<id> on disk.
func TestApplyDisposedReconcileFenceTierEditAccept(t *testing.T) {
	root, run := gitRepoFixture(t)
	fw := store.NewFenceWriter(root)
	ss := store.NewShardStore(root)
	q := writer.NewQueue(nil)
	defer q.Close()
	dispStore := dispositionStoreFixture(t, root)
	ctx := context.Background()

	old := claimFixture("claim-b", "alice-tan", "works_at", "initech")
	page := store.NewEntityPage(domain.Entity{Type: "topic", Slug: "alice-tan"})
	page.Claims = []domain.Claim{old}
	if _, err := fw.WriteEntity(page); err != nil {
		t.Fatalf("seed entity page: %v", err)
	}
	run("add", ".")
	run("commit", "--quiet", "-m", "seed b")

	draftA := claimFixture("claim-a-draft", "alice-tan", "works_at", "acme-corp")
	item := seedReconcileItem(t, dispStore, ctx, fixedNow, draftA, old)

	edited := draftA
	edited.Object = "globex-corp" // the human's real correction
	editedRaw, err := json.Marshal(edited)
	if err != nil {
		t.Fatalf("marshal edited: %v", err)
	}

	disposeRes, err := dispStore.Dispose(ctx, item.ID, disposition.VerdictEditAccept, editedRaw, "", "human:tester", "", fixedNow)
	if err != nil {
		t.Fatalf("Dispose: %v", err)
	}

	w := New(q, fw, ss, config.Default())
	res, err := w.ApplyDisposedReconcile(ctx, dispStore, disposeRes.Item, fixedNow)
	if err != nil {
		t.Fatalf("ApplyDisposedReconcile: %v", err)
	}
	if res.Tier != domain.TierFence {
		t.Fatalf("Tier = %q, want %q", res.Tier, domain.TierFence)
	}

	if _, err := writer.Flush(q, root); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	p, err := fw.ParseEntity(res.FencePath)
	if err != nil {
		t.Fatalf("ParseEntity: %v", err)
	}
	var newRow domain.Claim
	var found bool
	for _, c := range p.Claims {
		if c.State == domain.StateActive && c.Object == "globex-corp" {
			newRow, found = c, true
		}
	}
	if !found {
		t.Fatalf("edited object %q not found as an active row on the fence page: %+v", "globex-corp", p.Claims)
	}
	if newRow.ID == "claim-a-draft" {
		t.Fatal("landed row kept the stale pre-edit draft id -- the id must be recomputed from the edited object")
	}

	gotItem, err := dispStore.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotItem.AppliedClaimID != newRow.ID {
		t.Fatalf("AppliedClaimID = %q, want %q (the id ApplyDisposedReconcile actually wrote)", gotItem.AppliedClaimID, newRow.ID)
	}

	onDisk, err := os.ReadFile(res.FencePath)
	if err != nil {
		t.Fatalf("read fence: %v", err)
	}
	rerendered, err := fw.RenderEntity(p)
	if err != nil {
		t.Fatalf("RenderEntity: %v", err)
	}
	if string(onDisk) != string(rerendered) {
		t.Fatalf("re-render not byte-identical:\non disk:\n%s\nrerendered:\n%s", onDisk, rerendered)
	}
}

func TestApplyDisposedReconcileRejectsWrongKind(t *testing.T) {
	root, _ := gitRepoFixture(t)
	fw := store.NewFenceWriter(root)
	ss := store.NewShardStore(root)
	q := writer.NewQueue(nil)
	defer q.Close()
	dispStore := dispositionStoreFixture(t, root)
	ctx := context.Background()

	payload, err := json.Marshal(disposition.CapturePayload{Text: "not a reconcile item"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	item, err := dispStore.Create(ctx, disposition.KindDistill, payload, "", fixedNow)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	w := New(q, fw, ss, config.Default())
	if _, err := w.ApplyDisposedReconcile(ctx, dispStore, item, fixedNow); err == nil {
		t.Fatal("ApplyDisposedReconcile on a non-reconcile item succeeded, want an error")
	}
}

func TestApplyDisposedReconcileRejectsUndisposedItem(t *testing.T) {
	root, _ := gitRepoFixture(t)
	fw := store.NewFenceWriter(root)
	ss := store.NewShardStore(root)
	q := writer.NewQueue(nil)
	defer q.Close()
	dispStore := dispositionStoreFixture(t, root)
	ctx := context.Background()

	a := claimFixture("claim-a", "carol-diaz", "works_at", "acme-corp")
	b := claimFixture("claim-b", "carol-diaz", "works_at", "initech")
	item := seedReconcileItem(t, dispStore, ctx, fixedNow, a, b) // still pending, never disposed

	w := New(q, fw, ss, config.Default())
	if _, err := w.ApplyDisposedReconcile(ctx, dispStore, item, fixedNow); err == nil {
		t.Fatal("ApplyDisposedReconcile on a still-pending item succeeded, want an error")
	}
}
