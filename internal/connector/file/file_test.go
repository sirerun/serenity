package file_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/connector"
	"github.com/sirerun/serenity/internal/connector/file"
	"github.com/sirerun/serenity/internal/store"
)

var _ connector.Connector = (*file.Connector)(nil)

// fakeClock is a mutex-guarded Clock so it is safe to advance from a test
// goroutine while the watcher goroutine (watch-mode tests) reads it
// concurrently.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(t0 time.Time) *fakeClock { return &fakeClock{now: t0} }

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// writeFixture creates dir/rel with the given content and backdates its
// mtime by age, so poll-mode's default 2s debounce treats it as already
// stable without any test needing to sleep.
func writeFixture(t *testing.T, dir, rel, content string, age time.Duration) string {
	t.Helper()
	abs := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	stamp := time.Now().Add(-age)
	if err := os.Chtimes(abs, stamp, stamp); err != nil {
		t.Fatalf("chtimes %s: %v", rel, err)
	}
	return abs
}

func pollAll(t *testing.T, c *file.Connector, cur connector.Cursor) ([]connector.RawItem, connector.Cursor) {
	t.Helper()
	items, next, err := c.Poll(context.Background(), cur)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	return items, next
}

// TestGoldenKindsAndURIs proves the plan T1.3 acceptance line "golden
// test asserts kinds/URIs": a fixture directory of known files polled
// once yields exactly the expected {kind, uri} pairs, in the layout
// RawItem/ToSource actually produce.
func TestGoldenKindsAndURIs(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "notes.txt", "hello", time.Hour)
	writeFixture(t, dir, "exports/report.pdf", "%PDF-1.4 fake", time.Hour)

	c := file.NewPoll(dir)
	items, _ := pollAll(t, c, nil)

	type pair struct{ kind, uri string }
	got := make([]pair, 0, len(items))
	for _, it := range items {
		src, err := c.ToSource(it)
		if err != nil {
			t.Fatalf("ToSource: %v", err)
		}
		got = append(got, pair{src.Kind, src.URI})
	}
	sort.Slice(got, func(i, j int) bool { return got[i].uri < got[j].uri })

	want := []pair{
		{"file", "file://" + filepath.ToSlash(filepath.Join(dir, "exports/report.pdf"))},
		{"file", "file://" + filepath.ToSlash(filepath.Join(dir, "notes.txt"))},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d items, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("item %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestEditorTempFilesIgnored proves the acceptance line "editor temp
// files (.*.tmp, .swp) are ignored": neither pattern appears in Poll's
// output, while a normal file alongside them does.
func TestEditorTempFilesIgnored(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "draft.txt", "keep me", time.Hour)
	writeFixture(t, dir, ".draft.txt.tmp", "atomic-save scratch", time.Hour)
	writeFixture(t, dir, ".draft.txt.swp", "vim swap", time.Hour)
	writeFixture(t, dir, "backup.swp", "also a swap file", time.Hour)

	c := file.NewPoll(dir)
	items, _ := pollAll(t, c, nil)

	if len(items) != 1 {
		t.Fatalf("got %d items, want 1 (temp files must be ignored): %+v", len(items), items)
	}
	if items[0].URI != "file://"+filepath.ToSlash(filepath.Join(dir, "draft.txt")) {
		t.Fatalf("unexpected surviving item: %+v", items[0])
	}
}

// TestFixtureIngestedTwiceYieldsIdenticalSourceSet proves the acceptance
// line "fixture dir ingested twice yields an identical source set":
// running the connector -> store pipeline against an unchanged fixture
// directory a second time adds nothing and drops nothing.
func TestFixtureIngestedTwiceYieldsIdenticalSourceSet(t *testing.T) {
	fixture := t.TempDir()
	writeFixture(t, fixture, "a.txt", "alpha", time.Hour)
	writeFixture(t, fixture, "sub/b.txt", "bravo", time.Hour)

	brain := t.TempDir()
	ss := store.NewSourceStore(brain)

	ingestOnce := func() []string {
		c := file.NewPoll(fixture)
		items, _ := pollAll(t, c, nil)
		var shas []string
		for _, it := range items {
			src, err := c.ToSource(it)
			if err != nil {
				t.Fatalf("ToSource: %v", err)
			}
			written, err := ss.Write(it.Bytes, src)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			shas = append(shas, written.SHA256)
		}
		sort.Strings(shas)
		return shas
	}

	first := ingestOnce()
	second := ingestOnce()

	if len(first) != 2 {
		t.Fatalf("first ingest produced %d sources, want 2: %v", len(first), first)
	}
	if len(first) != len(second) {
		t.Fatalf("source set changed size across re-ingest: first=%v second=%v", first, second)
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("source set differs across re-ingest: first=%v second=%v", first, second)
		}
	}
}

// TestPollModeSecondCallWithSameCursorReturnsNothingNew shows the cursor
// itself is doing real work, not just riding on the store's dedup: with
// the cursor carried forward, an unchanged tree yields zero items on the
// second call.
func TestPollModeSecondCallWithSameCursorReturnsNothingNew(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.txt", "alpha", time.Hour)

	c := file.NewPoll(dir)
	first, cur := pollAll(t, c, nil)
	if len(first) != 1 {
		t.Fatalf("first poll returned %d items, want 1", len(first))
	}

	second, _ := pollAll(t, c, cur)
	if len(second) != 0 {
		t.Fatalf("second poll (same cursor, unchanged tree) returned %d items, want 0: %+v", len(second), second)
	}
}

// TestPollModeDebounceWithFakeClock proves the acceptance line "2s
// per-file debounce is tested with a fake clock": a freshly-written file
// is withheld until it has gone DefaultDebounce without a further
// change, purely by advancing an injected clock -- no real sleeping.
func TestPollModeDebounceWithFakeClock(t *testing.T) {
	dir := t.TempDir()
	abs := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(abs, []byte("mid-write"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t0 := time.Now()
	if err := os.Chtimes(abs, t0, t0); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	clock := newFakeClock(t0)
	c := file.NewPoll(dir, file.WithClock(clock))

	items, _ := pollAll(t, c, nil)
	if len(items) != 0 {
		t.Fatalf("at t0 (just written) got %d items, want 0 (not yet settled): %+v", len(items), items)
	}

	clock.Advance(file.DefaultDebounce - time.Millisecond)
	items, _ = pollAll(t, c, nil)
	if len(items) != 0 {
		t.Fatalf("just under the debounce window got %d items, want 0: %+v", len(items), items)
	}

	clock.Advance(2 * time.Millisecond) // now >= DefaultDebounce past t0
	items, _ = pollAll(t, c, nil)
	if len(items) != 1 {
		t.Fatalf("past the debounce window got %d items, want 1", len(items))
	}
	if items[0].URI != "file://"+filepath.ToSlash(abs) {
		t.Fatalf("unexpected item: %+v", items[0])
	}
}

// TestWatchModeDeliversCreatedFile is a smoke test for the fsnotify path
// (debounce is disabled here so the test asserts wiring, not timing --
// TestPollModeDebounceWithFakeClock is the debounce acceptance test).
func TestWatchModeDeliversCreatedFile(t *testing.T) {
	dir := t.TempDir()

	c, err := file.New(dir, file.WithDebounce(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	abs := filepath.Join(dir, "live.txt")
	if err := os.WriteFile(abs, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		items, _, err := c.Poll(context.Background(), nil)
		if err != nil {
			t.Fatalf("Poll: %v", err)
		}
		if len(items) == 1 {
			if items[0].URI != "file://"+filepath.ToSlash(abs) {
				t.Fatalf("unexpected item: %+v", items[0])
			}
			return
		}
		if len(items) > 1 {
			t.Fatalf("got %d items, want at most 1: %+v", len(items), items)
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for fsnotify to deliver the create event")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestToSourceLeavesSHA256ForTheStoreToCompute documents that ToSource
// never sets SHA256 itself -- SourceStore.Write always recomputes it from
// the bytes given, never trusting the caller (internal/store/source.go).
func TestToSourceLeavesSHA256ForTheStoreToCompute(t *testing.T) {
	c := file.NewPoll(t.TempDir())
	src, err := c.ToSource(connector.RawItem{Kind: "file", URI: "file:///x", Bytes: []byte("x")})
	if err != nil {
		t.Fatalf("ToSource: %v", err)
	}
	if src.SHA256 != "" {
		t.Fatalf("ToSource set SHA256 = %q, want empty (store's job)", src.SHA256)
	}
}
