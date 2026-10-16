//go:build linux || darwin

package episodeclient

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func importFixture(t *testing.T, q *Queue) (ImportProgress, ImportProgress, Input) {
	t.Helper()
	in := testInput()
	k := ImportKey{Format: "codex.0.163.0", Source: "session1", Workspace: in.Workspace, Conversation: in.Conversation}
	old, e := q.ImportProgress(k)
	if e != nil {
		t.Fatal(e)
	}
	next := ImportProgress{Key: k, Cursor: ImportCursor{Records: 3, PrefixSHA256: strings.Repeat("a", 64)}, Gaps: map[string]uint64{"reasoning_excluded": 1}, Complete: true}
	return old, next, in
}
func TestImportPageAtomicCapacityRestartAndCAS(t *testing.T) {
	o := testOptions(t)
	o.MaxItems = 1
	q := openTest(t, o)
	old, next, in := importFixture(t, q)
	other := in
	other.Event = "different"
	other.Sequence++
	if e := q.CapturePage(old, next, []Input{in, other}); e != ErrCapacity {
		t.Fatalf("capacity: %v", e)
	}
	st, _ := q.Stats()
	p, _ := q.ImportProgress(old.Key)
	if st.Items != 0 || p.Cursor.Records != 0 {
		t.Fatal("partial commit")
	}
	in.Text = "visible plus password=synthetic-value"
	if e := q.CapturePage(old, next, []Input{in}); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(filepath.Join(o.Directory, "queue.bin"))
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{in.Text, "synthetic-value", next.Key.Source, "reasoning_excluded"} {
		if bytes.Contains(raw, []byte(s)) {
			t.Fatal("unencrypted state")
		}
	}
	closeTest(t, q)
	q = openTest(t, o)
	got, e := q.ImportProgress(old.Key)
	if e != nil || got.Cursor != next.Cursor || got.Gaps["reasoning_excluded"] != 1 {
		t.Fatalf("restart: %+v %v", got, e)
	}
	if e = q.CapturePage(old, next, []Input{in}); e != ErrConflict {
		t.Fatalf("stale cursor: %v", e)
	}
	if e = q.CapturePage(got, got, nil); e != nil {
		t.Fatalf("same-prefix status: %v", e)
	}
}
func TestImportPageRejectsPauseMappingFenceAndRewind(t *testing.T) {
	q := openTest(t, testOptions(t))
	old, next, in := importFixture(t, q)
	if _, e := q.Pause(Scope{Conversation: in.Conversation}); e != nil {
		t.Fatal(e)
	}
	if e := q.CapturePage(old, next, nil); e != ErrPaused {
		t.Fatalf("empty page bypass: %v", e)
	}
	p, _ := q.ImportProgress(old.Key)
	if p.Cursor.Records != 0 {
		t.Fatal("paused cursor advanced")
	}
	q2 := openTest(t, testOptions(t))
	old, next, in = importFixture(t, q2)
	if e := q2.withLock(func() error {
		s, e := q2.read()
		if e != nil {
			return e
		}
		s.GenerationKnown = true
		s.Generation = 1
		s.Fenced = map[string]bool{ConversationFenceKey(q2.policy.Binding.Producer, in.Conversation): true}
		return q2.write(s)
	}); e != nil {
		t.Fatal(e)
	}
	if e := q2.CapturePage(old, next, nil); e != ErrBlocked {
		t.Fatalf("fenced empty page: %v", e)
	}
	q3 := openTest(t, testOptions(t))
	old, next, in = importFixture(t, q3)
	if e := q3.CapturePage(old, next, []Input{in}); e != nil {
		t.Fatal(e)
	}
	if e := q3.CapturePage(next, old, nil); e != ErrInvalid {
		t.Fatalf("rewind: %v", e)
	}
	q3.policy.Mappings = append(q3.policy.Mappings, q3.policy.Mappings[0])
	if e := q3.CapturePage(next, next, nil); e != ErrMapping {
		t.Fatalf("ambiguous mapping: %v", e)
	}
}
func TestImportPagePolicyFailureRollsBackEarlierAdmissions(t *testing.T) {
	q := openTest(t, testOptions(t))
	old, next, in := importFixture(t, q)
	bad := in
	bad.Event = "private"
	bad.Sequence++
	bad.Privacy = "private"
	if e := q.CapturePage(old, next, []Input{in, bad}); e != ErrExcluded {
		t.Fatalf("privacy: %v", e)
	}
	st, _ := q.Stats()
	p, _ := q.ImportProgress(old.Key)
	if st.Items != 0 || p.Cursor.Records != 0 {
		t.Fatal("partial policy commit")
	}
}

func TestImportProgressReadsLegacyQueueAndRejectsDowngradedMetadata(t *testing.T) {
	o := testOptions(t)
	q := openTest(t, o)
	nonce := make([]byte, q.aead.NonceSize())
	legacy := q.aead.Seal(nonce, nonce, []byte(`{"version":1,"items":null,"paused":null,"generation":0,"generation_known":false}`), q.aad)
	if e := os.WriteFile(filepath.Join(o.Directory, "queue.bin"), legacy, 0600); e != nil {
		t.Fatal(e)
	}
	old, next, in := importFixture(t, q)
	if e := q.CapturePage(old, next, []Input{in}); e != nil {
		t.Fatal(e)
	}
	if e := q.withLock(func() error {
		s, e := q.read()
		if e != nil {
			return e
		}
		if s.Version != 2 {
			t.Fatal("progress not versioned")
		}
		s.Version = 1
		return validateImports(s)
	}); e != ErrStorage {
		t.Fatalf("downgraded progress accepted: %v", e)
	}
}

func TestCaptureIdenticalPendingRetryDoesNotRequireAnotherWrite(t *testing.T) {
	o := testOptions(t)
	q := openTest(t, o)
	in := testInput()
	captureTest(t, q, in)
	before, e := os.ReadFile(filepath.Join(o.Directory, "queue.bin"))
	if e != nil {
		t.Fatal(e)
	}
	// A pending-path obstruction makes any new state publication fail. The
	// already durable identical record remains a successful write-free retry.
	if e = os.Mkdir(filepath.Join(o.Directory, "queue.pending"), 0700); e != nil {
		t.Fatal(e)
	}
	d, e := q.Capture(in)
	if e != nil || !d.Admitted {
		t.Fatalf("durable retry: %+v %v", d, e)
	}
	after, e := os.ReadFile(filepath.Join(o.Directory, "queue.bin"))
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("retry rewrote queue")
	}
	in.Event = "new-event"
	in.Sequence++
	if _, e = q.Capture(in); e != ErrStorage {
		t.Fatalf("new publication unexpectedly succeeded: %v", e)
	}
}
