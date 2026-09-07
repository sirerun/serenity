package consolidate

import (
	"bytes"
	"context"
	"errors"
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

var testNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

type fixture struct {
	root string
	cfg  *config.Config
	q    *writer.Queue
	eng  *index.SQLite
	fw   *store.FenceWriter
	page *store.EntityPage
}

func git(t *testing.T, root string, args ...string) {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = root
	c.Env = append(os.Environ(), "GIT_CONFIG_COUNT=0")
	if b, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, b)
	}
}
func setup(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-q")
	git(t, root, "config", "user.email", "test@example.invalid")
	git(t, root, "config", "user.name", "Test")
	git(t, root, "config", "core.hooksPath", "/dev/null")
	cfg := config.Default()
	if err := cfg.Save(filepath.Join(root, config.FileName)); err != nil {
		t.Fatal(err)
	}
	f := &fixture{root: root, cfg: cfg, q: writer.NewQueue(nil), fw: store.NewFenceWriter(root)}
	var err error
	f.eng, err = index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		f.q.Close()
		if err := f.eng.Close(); err != nil {
			t.Error(err)
		}
	})
	f.page = store.NewEntityPage(domain.Entity{Slug: "alice", Type: "person"})
	f.page.Summary = "hand edited summary"
	f.page.Claims = []domain.Claim{{ID: "role1", SubjectSlug: "alice", Predicate: "has_role", Family: "has_role", Object: "Engineer", State: domain.StateActive, Confidence: 0.9, ValidFrom: "2026-03-01"}}
	f.write(t)
	git(t, root, "add", ".")
	git(t, root, "commit", "-qm", "fixture")
	return f
}
func (f *fixture) write(t *testing.T) {
	t.Helper()
	if _, err := f.fw.WriteEntity(f.page); err != nil {
		t.Fatal(err)
	}
}
func (f *fixture) run(t *testing.T, e index.Embedder) Result {
	t.Helper()
	r, err := Run(context.Background(), f.root, f.cfg, f.q, f.eng, e, testNow)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func (f *fixture) read(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(f.fw.PathFor("person", "alice"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestConsolidateIdempotent(t *testing.T) {
	f := setup(t)
	f.run(t, nil)
	a := f.read(t)
	if !bytes.Contains(a, []byte("nothing new on Alice since 2026-03-01")) {
		t.Fatalf("missing freshness: %s", a)
	}
	r, err := Run(context.Background(), f.root, f.cfg, f.q, f.eng, nil, testNow.Add(time.Hour))
	if err != nil || r.Pages != 1 {
		t.Fatalf("repeat %+v %v", r, err)
	}
	if !bytes.Equal(a, f.read(t)) {
		t.Fatal("repeat changed bytes")
	}
}
func TestConsolidateAuthority(t *testing.T) {
	f := setup(t)
	path := f.fw.PathFor("person", "alice")
	b := strings.Replace(string(f.read(t)), "Engineer", "Hand repaired role", 1) + "\nHuman notes must survive.\n"
	if err := os.WriteFile(path, []byte(b), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, f.root, "add", ".")
	git(t, f.root, "commit", "-qm", "human edit")
	f.run(t, nil)
	got := string(f.read(t))
	if strings.Contains(got, "hand edited summary") || !strings.Contains(got, "DERIVED") || !strings.Contains(got, "Hand repaired role") || !strings.HasSuffix(got, "\nHuman notes must survive.\n") {
		t.Fatalf("authority lost: %s", got)
	}
}
func TestConsolidateDirtyGuard(t *testing.T) {
	f := setup(t)
	path := f.fw.PathFor("person", "alice")
	b := append(f.read(t), []byte("\nUncommitted human note\n")...)
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), f.root, f.cfg, f.q, f.eng, nil, testNow)
	if !errors.Is(err, writer.ErrDirtyTree) {
		t.Fatalf("got %v", err)
	}
	if !bytes.Equal(b, f.read(t)) {
		t.Fatal("dirty bytes overwritten")
	}
	if _, err := os.Stat(writer.PendingPath(f.root, "alice")); err != nil {
		t.Fatal(err)
	}
}
func TestConsolidateShardHeads(t *testing.T) {
	f := setup(t)
	ss := store.NewShardStore(f.root)
	old := domain.Claim{ID: "bal1", SubjectSlug: "alice", Predicate: "has_balance", Family: "has_balance", Object: "10", ObjectKey: "balance", State: domain.StateActive, Confidence: 0.9, ValidFrom: "2026-09-01"}
	newer := old
	newer.ID = "bal2"
	newer.Object = "20"
	newer.Supersedes = old.ID
	newer.ValidFrom = "2026-09-02"
	for _, c := range []domain.Claim{old, newer} {
		if err := ss.Append(c); err != nil {
			t.Fatal(err)
		}
	}
	orphan := old
	orphan.SubjectSlug = "orphan"
	orphan.ID = "orphan1"
	if err := ss.Append(orphan); err != nil {
		t.Fatal(err)
	}
	old.SourceRef = "shard"
	f.page.Claims = append(f.page.Claims, old)
	f.write(t)
	git(t, f.root, "add", ".")
	git(t, f.root, "commit", "-qm", "shard fixture")
	f.run(t, nil)
	p, err := f.fw.ParseEntity(f.fw.PathFor("person", "alice"))
	if err != nil {
		t.Fatal(err)
	}
	heads, err := ss.ResolveHeads("alice", "has_balance")
	if err != nil {
		t.Fatal(err)
	}
	var found int
	for _, c := range p.Claims {
		if c.Family == "has_balance" {
			found++
			if c.ID != heads[store.HeadKeys(heads)[0]].ID || c.Object != "20" || c.SourceRef != "shard" {
				t.Fatalf("bad head %+v", c)
			}
		}
	}
	if found != 1 {
		t.Fatalf("heads %d", found)
	}
	paths, err := filepath.Glob(filepath.Join(f.root, "brain/entities/*/orphan.md"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("orphan %v %v", paths, err)
	}
	f.run(t, nil)
	// A committed human modification of a derived head must not be erased.
	b := strings.Replace(string(f.read(t)), "| 20 |", "| 900 |", 1)
	if err := os.WriteFile(f.fw.PathFor("person", "alice"), []byte(b), 0600); err != nil {
		t.Fatal(err)
	}
	git(t, f.root, "add", ".")
	git(t, f.root, "commit", "-qm", "head edit")
	_, err = Run(context.Background(), f.root, f.cfg, f.q, f.eng, nil, testNow)
	if !errors.Is(err, ErrHeadDivergence) {
		t.Fatalf("expected divergence: %v", err)
	}
	if string(f.read(t)) != b {
		t.Fatal("lost head edit")
	}
}

type testEmbedder struct {
	calls int
	fail  bool
	pin   string
}

func (e *testEmbedder) ModelVersion() string { return e.pin }
func (e *testEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	e.calls++
	if e.fail {
		return nil, errors.New("provider unavailable")
	}
	return []float32{float32(len(text)), 1}, nil
}
func TestConsolidateChangedEmbeddings(t *testing.T) {
	f := setup(t)
	e := &testEmbedder{pin: "test@1"}
	if r := f.run(t, e); r.Embedded != 1 {
		t.Fatalf("first %+v", r)
	}
	f.run(t, e)
	if e.calls != 1 {
		t.Fatalf("unchanged called %d", e.calls)
	}
	other := &testEmbedder{pin: "test@2"}
	f.run(t, other)
	if other.calls != 1 {
		t.Fatal("new pin not embedded")
	}
	f.page.Claims[0].Object = "Architect"
	f.write(t)
	git(t, f.root, "add", ".")
	git(t, f.root, "commit", "-qm", "changed claim")
	e.fail = true
	_, err := Run(context.Background(), f.root, f.cfg, f.q, f.eng, e, testNow)
	if err == nil {
		t.Fatal("provider failure swallowed")
	}
	if ok, err := f.eng.HasVector(context.Background(), "page:alice", "test@2"); err != nil || ok {
		t.Fatalf("stale pin survived %v %v", ok, err)
	}
	e.fail = false
	f.run(t, e)
	if e.calls != 3 {
		t.Fatalf("retry calls %d", e.calls)
	}
	git(t, f.root, "rm", "brain/entities/person/alice.md")
	git(t, f.root, "commit", "-qm", "remove page")
	f.run(t, e)
	chunks, err := f.eng.AllChunks(context.Background())
	if err != nil || len(chunks) != 0 {
		t.Fatalf("deleted chunk %v %v", chunks, err)
	}
	if ok, err := f.eng.HasVector(context.Background(), "page:alice", e.pin); err != nil || ok {
		t.Fatalf("deleted vector %v %v", ok, err)
	}
}
