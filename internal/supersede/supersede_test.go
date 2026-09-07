package supersede

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// gitRepoFixture mirrors internal/writer/commit_test.go's helper of the
// same name: a real git repo with one seed commit, so Apply's downstream
// `writer.Flush` has real history to diff against.
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
	run("config", "user.email", "supersede-test@example.com")
	run("config", "user.name", "supersede test")
	if err := os.WriteFile(filepath.Join(root, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "seed.txt")
	run("commit", "--quiet", "-m", "seed")
	return root, run
}

// countLines reports the number of non-empty lines in path -- a direct
// file read rather than shelling out, since `wc` is not a git subcommand
// and gitRepoFixture's run helper only wraps git.
func countLines(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("read %s: %v", path, err)
	}
	n := 0
	for _, ln := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if strings.TrimSpace(ln) != "" {
			n++
		}
	}
	return n
}

var fixedNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

func claimFixture(id, subject, predicate, object string) domain.Claim {
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
			ObservedAt:   fixedNow,
			Actor:        "machine",
		},
	}
}

// TestApplyFenceTierStrikethroughAndPointer is T2.3's fence-tier half:
// "fence strikethrough + pointer." "works_at" is fence-tier in
// config.Default() (RFC §7.2a).
func TestApplyFenceTierStrikethroughAndPointer(t *testing.T) {
	root, run := gitRepoFixture(t)
	fw := store.NewFenceWriter(root)
	ss := store.NewShardStore(root)
	q := writer.NewQueue(nil)
	defer q.Close()

	old := claimFixture("claim-b", "alice-tan", "works_at", "initech")
	// "topic" matches ingest.DefaultEntityType, the bucket Apply's own
	// entityType() fallback resolves to when Writer.EntityType is nil --
	// the same convention T1.9's write path uses for every subject until
	// entity resolution (T2.13) assigns a real type.
	page := store.NewEntityPage(domain.Entity{Type: "topic", Slug: "alice-tan"})
	page.Claims = []domain.Claim{old}
	if _, err := fw.WriteEntity(page); err != nil {
		t.Fatalf("seed entity page: %v", err)
	}
	run("add", ".")
	run("commit", "--quiet", "-m", "seed b")

	newClaim := claimFixture("claim-a", "alice-tan", "works_at", "acme-corp")

	w := New(q, fw, ss, config.Default())
	res, err := w.Apply(newClaim, old)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Tier != domain.TierFence {
		t.Fatalf("Tier = %q, want %q", res.Tier, domain.TierFence)
	}

	if _, err := writer.Flush(q, root); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	diff := run("show", "HEAD")
	if !strings.Contains(diff, "~~claim-b~~") {
		t.Fatalf("expected the old claim's id struck through in the diff, got:\n%s", diff)
	}
	if !strings.Contains(diff, "superseded→claim-a") {
		t.Fatalf("expected a forward pointer to the new claim in the diff, got:\n%s", diff)
	}
	if !strings.Contains(diff, "claim-a") || !strings.Contains(diff, "acme-corp") {
		t.Fatalf("expected the new active claim's row in the diff, got:\n%s", diff)
	}

	// Old id unchanged: the row is struck through, not renamed or removed.
	p, err := fw.ParseEntity(res.FencePath)
	if err != nil {
		t.Fatalf("ParseEntity: %v", err)
	}
	var foundOld, foundNew bool
	for _, c := range p.Claims {
		switch c.ID {
		case "claim-b":
			foundOld = true
			if c.State != domain.StateSuperseded {
				t.Errorf("old claim State = %q, want %q", c.State, domain.StateSuperseded)
			}
			if c.SupersededBy != "claim-a" {
				t.Errorf("old claim SupersededBy = %q, want %q", c.SupersededBy, "claim-a")
			}
		case "claim-a":
			foundNew = true
			if c.State != domain.StateActive {
				t.Errorf("new claim State = %q, want %q", c.State, domain.StateActive)
			}
			// Not checked here: c.Supersedes. The fence-tier markdown
			// table's 7 columns (id/predicate/object/conf/valid/src/state)
			// have no forward-pointer column -- RenderEntity/ParseEntity
			// only ever read/write State+SupersededBy on the *old* row
			// (store/fence.go), the one-directional "strikethrough +
			// pointer" encoding RFC §7.2 describes. Apply still sets
			// a.Supersedes in memory (it is the field Apply's caller and
			// the shard-tier path rely on -- shard-tier's raw JSON append
			// preserves it verbatim, checked below), but a fence-tier
			// round trip through ParseEntity(RenderEntity(...)) does not
			// carry it forward: that is the existing format's design, not
			// something this task changes.
		}
	}
	if !foundOld {
		t.Error("old claim id claim-b missing after Apply -- it must be struck through in place, not removed")
	}
	if !foundNew {
		t.Error("new claim id claim-a missing after Apply")
	}

	// Re-render byte-identical: parse -> render round-trips exactly.
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

// TestApplyShardTierSupersedingLineAndRegeneratedHead is T2.3's acc-line
// integration test, word for word: "accept the A/B item, git diff shows
// one appended shard line with supersedes=<old id> and the fence head
// row updated; old id unchanged; re-render byte-identical." "has_balance"
// is shard-tier in config.Default() (RFC §7.2a).
func TestApplyShardTierSupersedingLineAndRegeneratedHead(t *testing.T) {
	root, run := gitRepoFixture(t)
	fw := store.NewFenceWriter(root)
	ss := store.NewShardStore(root)
	q := writer.NewQueue(nil)
	defer q.Close()

	old := claimFixture("claim-b", "acme-corp", "has_balance", "$500")
	if err := ss.Append(old); err != nil {
		t.Fatalf("seed shard: %v", err)
	}
	// Seed the fence head row a real prior write (or consolidate) would
	// have left, so the diff below shows exactly the change Apply makes.
	headRow := old
	headRow.SourceRef = "shard"
	page := store.NewEntityPage(domain.Entity{Type: "topic", Slug: "acme-corp"})
	page.Claims = []domain.Claim{headRow}
	if _, err := fw.WriteEntity(page); err != nil {
		t.Fatalf("seed entity page: %v", err)
	}
	run("add", ".")
	run("commit", "--quiet", "-m", "seed b")

	beforeShardLines := countLines(t, ss.PathFor("acme-corp", "has_balance"))

	newClaim := claimFixture("claim-a", "acme-corp", "has_balance", "$700")

	w := New(q, fw, ss, config.Default())
	res, err := w.Apply(newClaim, old)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Tier != domain.TierShard {
		t.Fatalf("Tier = %q, want %q", res.Tier, domain.TierShard)
	}
	if res.ShardPath == "" {
		t.Fatal("ShardPath is empty for a shard-tier apply")
	}

	if _, err := writer.Flush(q, root); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// "git diff shows one appended shard line with supersedes=<old id>":
	shardDiff := run("show", "--", res.ShardPath)
	added := 0
	for _, ln := range strings.Split(shardDiff, "\n") {
		if strings.HasPrefix(ln, "+") && !strings.HasPrefix(ln, "+++") {
			added++
		}
		if strings.HasPrefix(ln, "-") && !strings.HasPrefix(ln, "---") {
			t.Fatalf("shard diff removed a line -- shards are append-only, got:\n%s", shardDiff)
		}
	}
	if added != 1 {
		t.Fatalf("shard diff added %d line(s), want exactly 1:\n%s", added, shardDiff)
	}
	if !strings.Contains(shardDiff, `"supersedes":"claim-b"`) {
		t.Fatalf(`expected the appended line to carry "supersedes":"claim-b", got:\n%s`, shardDiff)
	}

	afterShardLines := countLines(t, ss.PathFor("acme-corp", "has_balance"))
	if afterShardLines != beforeShardLines+1 {
		t.Fatalf("shard line count = %d, want %d (before=%d, Apply must append exactly one line)", afterShardLines, beforeShardLines+1, beforeShardLines)
	}

	// Old id unchanged -- the original line is byte-identical, still
	// first in the file.
	lines, err := ss.Lines("acme-corp", "has_balance")
	if err != nil {
		t.Fatalf("Lines: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("shard has %d line(s), want 2 (old + new)", len(lines))
	}
	if lines[0].ID != "claim-b" || lines[0].Object != "$500" {
		t.Fatalf("old line changed: got %+v", lines[0])
	}
	if lines[1].ID != "claim-a" || lines[1].Supersedes != "claim-b" {
		t.Fatalf("new line wrong: got %+v", lines[1])
	}

	// "the fence head row updated": exactly the new head, marked shard.
	p, err := fw.ParseEntity(res.FencePath)
	if err != nil {
		t.Fatalf("ParseEntity: %v", err)
	}
	if len(p.Claims) != 1 {
		t.Fatalf("fence has %d claim row(s) for has_balance, want exactly 1 (the regenerated head): %+v", len(p.Claims), p.Claims)
	}
	if p.Claims[0].ID != "claim-a" {
		t.Fatalf("fence head row id = %q, want %q (the new resolved head)", p.Claims[0].ID, "claim-a")
	}
	if p.Claims[0].SourceRef != "shard" {
		t.Fatalf("fence head row SourceRef = %q, want %q", p.Claims[0].SourceRef, "shard")
	}

	// Re-render byte-identical.
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

func TestApplyRejectsMismatchedSubjectPredicate(t *testing.T) {
	fw := store.NewFenceWriter(t.TempDir())
	ss := store.NewShardStore(t.TempDir())
	q := writer.NewQueue(nil)
	defer q.Close()
	w := New(q, fw, ss, config.Default())

	a := claimFixture("claim-a", "alice-tan", "works_at", "acme-corp")
	b := claimFixture("claim-b", "bob-lee", "has_role", "engineer")

	if _, err := w.Apply(a, b); err == nil {
		t.Fatal("Apply with mismatched (subject, predicate) succeeded, want an error")
	}
}

func TestApplyFenceTierErrorsWhenBIsNotOnThePage(t *testing.T) {
	root := t.TempDir()
	fw := store.NewFenceWriter(root)
	ss := store.NewShardStore(root)
	q := writer.NewQueue(nil)
	defer q.Close()
	w := New(q, fw, ss, config.Default())

	a := claimFixture("claim-a", "carol-diaz", "works_at", "acme-corp")
	b := claimFixture("claim-b", "carol-diaz", "works_at", "initech")
	// No page ever written for carol-diaz -- b does not actually exist on
	// disk, an inconsistency Apply must surface, not paper over.

	if _, err := w.Apply(a, b); err == nil {
		t.Fatal("Apply succeeded against a b that is not on any existing page, want an error")
	}
}
