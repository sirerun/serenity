package writer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/store"
)

func erasureIndex(t *testing.T, root string) *index.SQLite {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".serenity"), 0o700); err != nil {
		t.Fatal(err)
	}
	eng, err := index.Open(filepath.Join(root, ".serenity", "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return eng
}

func factRows(t *testing.T, eng *index.SQLite, sha, model string) (chunks int, vector bool) {
	t.Helper()
	ctx := context.Background()
	all, err := eng.AllChunks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range all {
		if h.ChunkRef == "fact:"+sha || h.SourceSHA256 == sha {
			chunks++
		}
	}
	vector, err = eng.HasVector(ctx, "fact:"+sha, model)
	if err != nil {
		t.Fatal(err)
	}
	return chunks, vector
}

// TestForgetErasesFactFromTreeAndIndexInSameFlush is PRIV-01's tree and
// index half: after forget and the next flush the fact directory is gone
// from the working tree and from the committed tree, its FTS and vector
// rows are gone, and the expiry audit event never carries the fact text.
func TestForgetErasesFactFromTreeAndIndexInSameFlush(t *testing.T) {
	root, run := gitRepoFixture(t)
	eng := erasureIndex(t, root)
	q := NewQueue(nil)
	defer q.Close()
	sources := store.NewSourceStore(root)
	w := MemoryFact{Queue: q, Sources: sources, Index: eng}
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	const secret = "erasure marker 7f3a: the private phone number"
	res, err := w.Remember(RememberInput{Fact: secret, Provenance: "test", EntitySlug: "alice", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld}, now)
	if err != nil {
		t.Fatal(err)
	}
	sha := res.Record.SHA256
	ctx := context.Background()
	if err := index.RefreshMemoryFact(ctx, root, sha, eng, now); err != nil {
		t.Fatal(err)
	}
	if err := eng.UpsertVector(ctx, "fact:"+sha, "erasure@v1", []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := Flush(q, root); err != nil {
		t.Fatal(err)
	}
	if c, v := factRows(t, eng, sha, "erasure@v1"); c != 1 || !v {
		t.Fatalf("fixture: chunks=%d vector=%v", c, v)
	}

	forgot, err := w.Forget(sha, "user request", now.Add(time.Minute))
	if err != nil || !forgot.Expired {
		t.Fatalf("forget: %+v %v", forgot, err)
	}
	if c, v := factRows(t, eng, sha, "erasure@v1"); c != 0 || v {
		t.Fatalf("index rows survived forget: chunks=%d vector=%v", c, v)
	}
	if _, err := Flush(q, root); err != nil {
		t.Fatal(err)
	}
	dir := sources.DirFor(sha)
	if _, err := os.Lstat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fact directory survived forget: %v", err)
	}
	rel, _ := filepath.Rel(root, dir)
	if out := run("ls-files", "--", filepath.ToSlash(rel)); strings.TrimSpace(out) != "" {
		t.Fatalf("fact path still tracked after flush: %q", out)
	}
	if out := run("status", "--porcelain", "--", filepath.ToSlash(rel)); strings.TrimSpace(out) != "" {
		t.Fatalf("fact deletion not committed in the same flush: %q", out)
	}
	expiry := forgot.Record.ExpirySHA256
	data, _, err := sources.Read(expiry)
	if err != nil {
		t.Fatalf("expiry audit event missing: %v", err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatal("expiry event carries the fact text")
	}
	erel, _ := filepath.Rel(root, sources.DirFor(expiry))
	if out := run("ls-files", "--", filepath.ToSlash(erel)); !strings.Contains(out, "bytes") {
		t.Fatalf("expiry audit event not committed: %q", out)
	}
}

// TestForgetAfterErasureIsIdempotentAndNeverResurrects: a second forget of
// an erased fact still succeeds with Expired=false, and a retried remember
// under the forgotten fact's operation key never writes the text back.
func TestForgetAfterErasureIsIdempotentAndNeverResurrects(t *testing.T) {
	root, _ := gitRepoFixture(t)
	q := NewQueue(nil)
	defer q.Close()
	sources := store.NewSourceStore(root)
	w := MemoryFact{Queue: q, Sources: sources}
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	input := RememberInput{OperationKey: "ajent:erase:1", Fact: "retry must not revive this", Provenance: "test", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld}
	res, err := w.Remember(input, now)
	if err != nil {
		t.Fatal(err)
	}
	sha := res.Record.SHA256
	if _, err := w.Forget(sha, "", now); err != nil {
		t.Fatal(err)
	}
	again, err := w.Forget(sha, "", now.Add(time.Second))
	if err != nil || again.Expired {
		t.Fatalf("re-forget after erasure: %+v %v", again, err)
	}
	if _, err := w.Remember(input, now.Add(time.Hour)); !errors.Is(err, ErrMemoryOperationCanceled) {
		t.Fatalf("retry after erasure: %v", err)
	}
	if _, err := os.Lstat(sources.DirFor(sha)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retry resurrected the erased fact: %v", err)
	}
	proj, err := store.LoadMemoryProjection(sources)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(proj.All()); n != 0 {
		t.Fatalf("projection holds %d facts after erasure", n)
	}
}

// TestSourceTombstoneErasesTreeAndIndexInSameFlush: the writer entry point
// removes a source's bytes and meta, its raw-source index rows, and commits
// the removal with the tombstone event in one flush.
func TestSourceTombstoneErasesTreeAndIndexInSameFlush(t *testing.T) {
	root, run := gitRepoFixture(t)
	eng := erasureIndex(t, root)
	q := NewQueue(nil)
	defer q.Close()
	sources := store.NewSourceStore(root)
	const body = "tombstoned mail body 5e0a"
	src, err := sources.Write([]byte(body), domain.Source{Kind: "email", URI: "mail://inbox/5e0a", OccurredAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	ref := "src:" + src.SHA256 + ":0-26"
	if err := eng.InsertChunk(ctx, ref, "", body, src.SHA256, "email"); err != nil {
		t.Fatal(err)
	}
	if err := eng.UpsertVector(ctx, ref, "erasure@v1", []float32{1, 0}); err != nil {
		t.Fatal(err)
	}
	dir := sources.DirFor(src.SHA256)
	rel, _ := filepath.Rel(root, dir)
	run("add", "--", filepath.ToSlash(rel))
	run("commit", "--quiet", "-m", "fixture source")

	w := SourceTombstone{Queue: q, Sources: sources, Shards: store.NewShardStore(root), Index: eng}
	if _, err := w.Tombstone(src.SHA256, time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if c, _ := factRows(t, eng, src.SHA256, "erasure@v1"); c != 0 {
		t.Fatalf("source chunks survived tombstone: %d", c)
	}
	if has, err := eng.HasVector(ctx, ref, "erasure@v1"); err != nil || has {
		t.Fatalf("source vector survived tombstone: %v %v", has, err)
	}
	if _, err := Flush(q, root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source directory survived tombstone: %v", err)
	}
	if out := run("ls-files", "--", filepath.ToSlash(rel)); strings.TrimSpace(out) != "" {
		t.Fatalf("source still tracked after flush: %q", out)
	}
	if out := run("status", "--porcelain"); strings.TrimSpace(out) != "" && !strings.Contains(out, ".serenity") {
		t.Fatalf("tombstone left uncommitted changes: %q", out)
	}
	if out := run("show", "--name-only", "--format=", "HEAD"); !strings.Contains(out, "bytes") || !strings.Contains(out, filepath.ToSlash(rel)) {
		t.Fatalf("flush did not commit the removal with the event: %q", out)
	}
}
