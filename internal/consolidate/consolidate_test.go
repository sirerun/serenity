package consolidate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

var fixedNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

// newConsolidator builds a Consolidator against root with a fixed clock,
// so every test's freshness banner is deterministic. Callers must
// q.Close() when done.
func newConsolidator(root string) (*Consolidator, *writer.Queue) {
	q := writer.NewQueue(nil)
	fw := store.NewFenceWriter(root)
	ss := store.NewShardStore(root)
	c := New(q, fw, ss, config.Default())
	c.Now = func() time.Time { return fixedNow }
	return c, q
}

func claimFixture(id, subject, predicate, object string, observed time.Time) domain.Claim {
	return domain.Claim{
		ID:          id,
		SubjectSlug: subject,
		Predicate:   predicate,
		Object:      object,
		ObjectKey:   store.NormalizeKey(object),
		Confidence:  0.9,
		State:       domain.StateActive,
		SourceRef:   "e1#1",
		Family:      predicate,
		Provenance: domain.Provenance{
			SourceSHA256: "sha-" + id,
			Span:         "0-10",
			Model:        "test-model@v1",
			ObservedAt:   observed,
			Actor:        "machine",
		},
	}
}

// gitRepoFixture mirrors internal/supersede/supersede_test.go's helper of
// the same name: a real git repo with one seed commit, so the dirty-tree
// guard has real history to diff against.
func gitRepoFixture(t *testing.T) (root string, run func(args ...string) string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root = t.TempDir()

	run = func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run("init", "--quiet")
	run("config", "user.email", "consolidate-test@example.com")
	run("config", "user.name", "consolidate test")
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "seed.txt")
	run("commit", "--quiet", "-m", "seed")
	return root, run
}

// TestConsolidateAuthority is T2.14's authority-rule acc line: a
// hand-edited summary fence is overwritten (it is always DERIVED, §7.2)
// while a hand-edited fence-tier claims row is preserved (for fence-tier
// families the file is truth).
func TestConsolidateAuthority(t *testing.T) {
	root := t.TempDir()
	c, q := newConsolidator(root)
	defer q.Close()

	claim := claimFixture("claim-a", "alice-tan", "works_at", "Hand-Edited Employer LLC", fixedNow.Add(-48*time.Hour))
	page := store.NewEntityPage(domain.Entity{Type: "topic", Slug: "alice-tan"})
	page.Summary = "Hand-written summary that must be overwritten."
	page.Claims = []domain.Claim{claim}
	if _, err := c.Fence.WriteEntity(page); err != nil {
		t.Fatalf("seed entity page: %v", err)
	}

	if _, err := c.Run(context.Background(), nil, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	p, err := c.Fence.ParseEntity(c.Fence.PathFor("topic", "alice-tan"))
	if err != nil {
		t.Fatalf("ParseEntity: %v", err)
	}
	if p.Summary == "Hand-written summary that must be overwritten." {
		t.Fatal("summary fence was not overwritten -- it must always be DERIVED")
	}
	if !strings.Contains(p.Summary, "1 active claim") {
		t.Fatalf("summary = %q, want it to reflect the one active claim", p.Summary)
	}

	if len(p.Claims) != 1 || p.Claims[0].Object != "Hand-Edited Employer LLC" {
		t.Fatalf("fence-tier claims row not preserved: %+v", p.Claims)
	}
}

// TestConsolidateDirtyGuard proves consolidate never races a human edit:
// a page with an uncommitted modification is paused (writer.ErrDirtyTree,
// recorded in Report.DirtySkipped and at writer.PendingPath) and left on
// disk exactly as the human wrote it, while the rest of the sweep still
// runs.
func TestConsolidateDirtyGuard(t *testing.T) {
	root, run := gitRepoFixture(t)
	c, q := newConsolidator(root)
	defer q.Close()

	dirtyClaim := claimFixture("claim-a", "dirty-co", "works_at", "acme", fixedNow.Add(-time.Hour))
	dirtyPage := store.NewEntityPage(domain.Entity{Type: "topic", Slug: "dirty-co"})
	dirtyPage.Claims = []domain.Claim{dirtyClaim}
	if _, err := c.Fence.WriteEntity(dirtyPage); err != nil {
		t.Fatalf("seed dirty page: %v", err)
	}

	cleanClaim := claimFixture("claim-b", "clean-co", "works_at", "initech", fixedNow.Add(-time.Hour))
	cleanPage := store.NewEntityPage(domain.Entity{Type: "topic", Slug: "clean-co"})
	cleanPage.Claims = []domain.Claim{cleanClaim}
	if _, err := c.Fence.WriteEntity(cleanPage); err != nil {
		t.Fatalf("seed clean page: %v", err)
	}

	run("add", ".")
	run("commit", "--quiet", "-m", "seed both pages")

	dirtyPath := c.Fence.PathFor("topic", "dirty-co")
	humanEdit, err := os.ReadFile(dirtyPath)
	if err != nil {
		t.Fatal(err)
	}
	humanEdit = append(humanEdit, []byte("\n<!-- human note -->\n")...)
	if err := os.WriteFile(dirtyPath, humanEdit, 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := c.Run(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(rep.DirtySkipped) != 1 || rep.DirtySkipped[0] != dirtyPath {
		t.Fatalf("DirtySkipped = %v, want exactly [%s]", rep.DirtySkipped, dirtyPath)
	}

	onDisk, err := os.ReadFile(dirtyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) != string(humanEdit) {
		t.Fatal("dirty page was overwritten -- the human's file state must be left untouched")
	}
	if _, err := os.Stat(writer.PendingPath(root, "dirty-co")); err != nil {
		t.Fatalf("expected a pending record at %s: %v", writer.PendingPath(root, "dirty-co"), err)
	}

	cleanP, err := c.Fence.ParseEntity(c.Fence.PathFor("topic", "clean-co"))
	if err != nil {
		t.Fatalf("ParseEntity(clean-co): %v", err)
	}
	if cleanP.Summary == "" || strings.Contains(cleanP.Summary, "No active claims") {
		t.Fatalf("clean-co was not consolidated despite dirty-co being paused: summary=%q", cleanP.Summary)
	}
}

// TestConsolidateShardHeads proves shard-tier authority (§7.2a): after a
// sweep, an entity page's rows for a shard-tier family are exactly its
// shard's own ResolveHeads output, marked src "shard" -- never a stale or
// hand-written head.
func TestConsolidateShardHeads(t *testing.T) {
	root := t.TempDir()
	c, q := newConsolidator(root)
	defer q.Close()

	old := claimFixture("claim-b", "acme-corp", "has_balance", "$500", fixedNow.Add(-72*time.Hour))
	if err := c.Shard.Append(old); err != nil {
		t.Fatalf("seed shard: %v", err)
	}
	newer := claimFixture("claim-a", "acme-corp", "has_balance", "$700", fixedNow.Add(-time.Hour))
	newer.Supersedes = "claim-b"
	if err := c.Shard.Append(newer); err != nil {
		t.Fatalf("append superseding line: %v", err)
	}

	// A stale hand-written head row sitting on the page must not survive
	// consolidate -- the shard is canonical for this family (§7.2a).
	stalePage := store.NewEntityPage(domain.Entity{Type: "topic", Slug: "acme-corp"})
	stalePage.Claims = []domain.Claim{{
		ID: "stale-head", SubjectSlug: "acme-corp", Predicate: "has_balance",
		Object: "$1", ObjectKey: store.NormalizeKey("$1"), Confidence: 0.5,
		State: domain.StateActive, SourceRef: "shard", Family: "has_balance",
	}}
	if _, err := c.Fence.WriteEntity(stalePage); err != nil {
		t.Fatalf("seed stale head: %v", err)
	}

	if _, err := c.Run(context.Background(), nil, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	wantHeads, err := c.Shard.ResolveHeads("acme-corp", "has_balance")
	if err != nil {
		t.Fatalf("ResolveHeads: %v", err)
	}

	p, err := c.Fence.ParseEntity(c.Fence.PathFor("topic", "acme-corp"))
	if err != nil {
		t.Fatalf("ParseEntity: %v", err)
	}
	if len(p.Claims) != len(wantHeads) {
		t.Fatalf("page has %d claim row(s), want %d (== len(ResolveHeads)): %+v", len(p.Claims), len(wantHeads), p.Claims)
	}
	// The fence table's 7 columns carry no object_key column (store/fence.go),
	// so a round-tripped row has no ObjectKey to key wantHeads by -- index
	// the resolved heads by id instead, the field the table does carry.
	headsByID := map[string]domain.Claim{}
	for _, head := range wantHeads {
		headsByID[head.ID] = head
	}
	for _, row := range p.Claims {
		head, ok := headsByID[row.ID]
		if !ok {
			t.Fatalf("page row %+v has no matching resolved head", row)
		}
		if row.Object != head.Object {
			t.Fatalf("page row %+v does not match resolved head %+v", row, head)
		}
		if row.SourceRef != "shard" {
			t.Fatalf("page row %+v SourceRef = %q, want %q", row, row.SourceRef, "shard")
		}
	}
	if len(wantHeads) != 1 || p.Claims[0].ID != "claim-a" {
		t.Fatalf("expected exactly the superseding claim-a head, got heads=%+v claims=%+v", wantHeads, p.Claims)
	}
}

// fakeEmbedder is a test double for the index.Embedder interface Run
// depends on -- deterministic and call-counted, per the zero-stub policy
// this is test-file-only.
type fakeEmbedder struct {
	pin      string
	calls    int
	lastText string
}

func (e *fakeEmbedder) ModelVersion() string { return e.pin }

func (e *fakeEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	e.calls++
	e.lastText = text
	var v [2]float32
	for i, r := range text {
		v[i%2] += float32(r)
	}
	return v[:], nil
}

// TestConsolidateChangedEmbeddings proves the re-embed phase runs against
// the sweep's own freshly-regenerated content, not a stale summary: after
// Run, the entity-page chunk actually gets embedded and the text handed
// to the embedder reflects the derived (never the hand-written) summary.
func TestConsolidateChangedEmbeddings(t *testing.T) {
	root := t.TempDir()
	c, q := newConsolidator(root)
	defer q.Close()

	claim := claimFixture("claim-a", "acme-corp", "works_at", "acme", fixedNow.Add(-time.Hour))
	page := store.NewEntityPage(domain.Entity{Type: "topic", Slug: "acme-corp"})
	page.Summary = "stale hand-written summary"
	page.Claims = []domain.Claim{claim}
	if _, err := c.Fence.WriteEntity(page); err != nil {
		t.Fatalf("seed entity page: %v", err)
	}

	eng, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	defer func() { _ = eng.Close() }()

	fe := &fakeEmbedder{pin: "fake@v1"}
	rep, err := c.Run(context.Background(), eng, fe)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Embedded == 0 {
		t.Fatal("Run reported 0 embedded chunks, want at least the entity-page chunk")
	}
	if fe.calls == 0 {
		t.Fatal("embedder was never called")
	}
	if strings.Contains(fe.lastText, "stale hand-written summary") {
		t.Fatal("embedded text still carries the stale hand-written summary -- the re-embed step must run after summary regeneration, not before")
	}

	if _, ok, err := eng.VectorFor(context.Background(), "page:acme-corp", "fake@v1"); err != nil || !ok {
		t.Fatalf("VectorFor(page:acme-corp) ok=%v err=%v, want a stored vector", ok, err)
	}
}

// TestConsolidateIdempotent is T2.14's headline acc line: consolidate
// twice on the same brain, same clock, renders byte-identical fence
// pages both times -- WriteEntity's own write-skip contract holds
// end-to-end through a Consolidator sweep, not just for one raw
// FenceWriter call.
func TestConsolidateIdempotent(t *testing.T) {
	root := t.TempDir()
	c, q := newConsolidator(root)
	defer q.Close()

	claim := claimFixture("claim-a", "alice-tan", "works_at", "acme", fixedNow.Add(-time.Hour))
	page := store.NewEntityPage(domain.Entity{Type: "topic", Slug: "alice-tan"})
	page.Claims = []domain.Claim{claim}
	if _, err := c.Fence.WriteEntity(page); err != nil {
		t.Fatalf("seed entity page: %v", err)
	}

	shardClaim := claimFixture("claim-b", "acme-corp", "has_balance", "$500", fixedNow.Add(-time.Hour))
	if err := c.Shard.Append(shardClaim); err != nil {
		t.Fatalf("seed shard: %v", err)
	}

	if _, err := c.Run(context.Background(), nil, nil); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	firstPage, err := os.ReadFile(c.Fence.PathFor("topic", "alice-tan"))
	if err != nil {
		t.Fatal(err)
	}
	firstHead, err := os.ReadFile(c.Fence.PathFor("topic", "acme-corp"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := c.Run(context.Background(), nil, nil); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	secondPage, err := os.ReadFile(c.Fence.PathFor("topic", "alice-tan"))
	if err != nil {
		t.Fatal(err)
	}
	secondHead, err := os.ReadFile(c.Fence.PathFor("topic", "acme-corp"))
	if err != nil {
		t.Fatal(err)
	}

	if string(firstPage) != string(secondPage) {
		t.Fatalf("alice-tan page changed on second consolidate:\nfirst:\n%s\nsecond:\n%s", firstPage, secondPage)
	}
	if string(firstHead) != string(secondHead) {
		t.Fatalf("acme-corp shard-head page changed on second consolidate:\nfirst:\n%s\nsecond:\n%s", firstHead, secondHead)
	}
}
