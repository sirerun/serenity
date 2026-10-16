//go:build linux || darwin

package episodeclient

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func testOptions(t *testing.T) Options {
	t.Helper()
	parent, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return Options{Directory: filepath.Join(parent, "queue"), Key: bytes.Repeat([]byte{7}, 32), Policy: testPolicy(), MaxItems: 64, MaxBytes: 2 << 20}
}
func openTest(t *testing.T, o Options) *Queue {
	t.Helper()
	q, e := OpenQueue(o)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { closeTest(t, q) })
	return q
}
func captureTest(t *testing.T, q *Queue, in Input) {
	t.Helper()
	if d, e := q.Capture(in); e != nil || !d.Admitted {
		t.Fatalf("capture: %v %+v", e, d)
	}
}

type reconcileFunc func(context.Context, ReplayKey) (Verification, error)

func (f reconcileFunc) Reconcile(ctx context.Context, k ReplayKey) (Verification, error) {
	return f(ctx, k)
}

type publishFunc func(context.Context, Record, uint64) (Receipt, error)

func (f publishFunc) Publish(ctx context.Context, r Record, g uint64) (Receipt, error) {
	return f(ctx, r, g)
}
func allowed(_ context.Context, k ReplayKey) (Verification, error) {
	now := time.Now()
	return Verification{Binding: k.Binding, Workspace: k.Workspace, Allowed: true, Full: true, GenerationKnown: true, Generation: 4, SyncedAt: now, ExpiresAt: now.Add(30 * time.Second), Conversations: map[string]bool{}}, nil
}
func receiptFor(r Record) Receipt { return Receipt{key(r), DigestRecord(r), true} }
func TestQueueEncryptedRestartBindingAndRedaction(t *testing.T) {
	o := testOptions(t)
	q := openTest(t, o)
	in := testInput()
	in.Text = "private ordinary bytes plus password=synthetic-value"
	captureTest(t, q, in)
	raw, e := os.ReadFile(filepath.Join(o.Directory, "queue.bin"))
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{"private ordinary bytes", "synthetic-value", in.Conversation, o.Policy.Binding.Brain} {
		if bytes.Contains(raw, []byte(s)) {
			t.Fatal("plaintext at rest")
		}
	}
	closeTest(t, q)
	q = openTest(t, o)
	seen := false
	out, e := q.Drain(context.Background(), reconcileFunc(allowed), publishFunc(func(_ context.Context, r Record, _ uint64) (Receipt, error) {
		seen = true
		if r.Text != "private ordinary bytes plus [REDACTED]" {
			t.Fatal("redaction not staged")
		}
		return receiptFor(r), nil
	}), 1)
	if e != nil || !seen || out.Published != 1 || out.Remaining != 0 {
		t.Fatalf("drain %v %+v", e, out)
	}
	captureTest(t, q, testInput())
	closeTest(t, q)
	bad := o
	bad.Key = bytes.Repeat([]byte{8}, 32)
	if q, e := OpenQueue(bad); e != ErrStorage {
		if q != nil {
			closeTest(t, q)
		}
		t.Fatal("wrong key accepted")
	}
	bad = o
	bad.Policy = testPolicy()
	bad.Policy.Binding.Brain = "other"
	if q, e := OpenQueue(bad); e != ErrStorage {
		if q != nil {
			closeTest(t, q)
		}
		t.Fatal("wrong binding accepted")
	}
}
func TestExclusionsNeverCreateDurableContent(t *testing.T) {
	o := testOptions(t)
	q := openTest(t, o)
	for _, class := range []string{"credential", "secret_file", "hidden_reasoning", "recall"} {
		in := testInput()
		in.Class = class
		in.Text = "NEVER-STAGE"
		if _, e := q.Capture(in); e != ErrExcluded {
			t.Fatal(e)
		}
	}
	if _, e := os.Stat(filepath.Join(o.Directory, "queue.bin")); !os.IsNotExist(e) {
		t.Fatal("excluded content caused durable staging")
	}
	st, e := q.Stats()
	if e != nil || st.Items != 0 {
		t.Fatal("exclusion count")
	}
}
func TestQueueCapacityAndStableRetry(t *testing.T) {
	o := testOptions(t)
	o.MaxItems = 1
	q := openTest(t, o)
	in := testInput()
	captureTest(t, q, in)
	captureTest(t, q, in)
	changed := in
	changed.Text = "changed"
	if _, e := q.Capture(changed); e != ErrConflict {
		t.Fatal("identity overwritten")
	}
	changed = in
	changed.Event = "event-2"
	if _, e := q.Capture(changed); e != ErrConflict {
		t.Fatal("duplicate sequence accepted")
	}
	changed.Sequence = 2
	if _, e := q.Capture(changed); e != ErrCapacity {
		t.Fatal("queue silently evicted")
	}
	st, _ := q.Stats()
	if st.Items != 1 {
		t.Fatal("lost original")
	}
	o = testOptions(t)
	o.MaxBytes = 1
	q = openTest(t, o)
	if _, e := q.Capture(in); e != ErrCapacity {
		t.Fatal("byte cap bypassed")
	}
}
func TestPauseClearsOnlyCoveredItemsAndSurvivesRestart(t *testing.T) {
	o := testOptions(t)
	q := openTest(t, o)
	in := testInput()
	captureTest(t, q, in)
	other := in
	other.Conversation = "other"
	captureTest(t, q, other)
	scope := Scope{Workspace: in.Workspace, Conversation: in.Conversation}
	n, e := q.Pause(scope)
	if e != nil || n != 1 {
		t.Fatalf("pause %v %d", e, n)
	}
	closeTest(t, q)
	q = openTest(t, o)
	if _, e = q.Capture(in); e != ErrPaused {
		t.Fatal("pause not durable")
	}
	st, _ := q.Stats()
	if st.Items != 1 {
		t.Fatal("uncovered item lost")
	}
	if e = q.Resume(scope); e != nil {
		t.Fatal(e)
	}
	captureTest(t, q, in)
	n, e = q.Pause(Scope{})
	if e != nil || n != 2 {
		t.Fatal("global pause")
	}
	if e = q.Resume(scope); e != nil {
		t.Fatal(e)
	}
	if _, e = q.Capture(in); e != ErrPaused {
		t.Fatal("narrow resume bypassed global pause")
	}
}
func TestDrainFailsClosedAndRetainsBlockedItems(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*Verification)
	}{
		{"revoked", func(v *Verification) { v.Allowed = false }},
		{"remapped", func(v *Verification) { v.Binding.Brain = "other" }},
		{"policy", func(v *Verification) { v.Binding.Policy = "policy-2" }},
		{"producer", func(v *Verification) { v.Binding.Producer = "other" }},
		{"workspace", func(v *Verification) { v.Workspace = "personal" }},
		{"partial", func(v *Verification) { v.Full = false }},
		{"missing", func(v *Verification) { v.GenerationKnown = false }},
		{"expired", func(v *Verification) { v.ExpiresAt = time.Now().Add(-time.Second) }},
		{"stale", func(v *Verification) { v.SyncedAt = time.Now().Add(-2 * time.Minute) }},
		{"future", func(v *Verification) { v.SyncedAt = time.Now().Add(time.Minute) }},
		{"unbounded_ttl", func(v *Verification) { v.ExpiresAt = time.Now().Add(time.Hour) }},
		{"deleted", func(v *Verification) {
			v.Conversations[ConversationFenceKey(v.Binding.Producer, "conversation-test")] = true
		}},
	}
	for _, tt := range mutations {
		t.Run(tt.name, func(t *testing.T) {
			q := openTest(t, testOptions(t))
			captureTest(t, q, testInput())
			calls := 0
			_, e := q.Drain(context.Background(), reconcileFunc(func(ctx context.Context, k ReplayKey) (Verification, error) {
				v, e := allowed(ctx, k)
				tt.mutate(&v)
				return v, e
			}), publishFunc(func(_ context.Context, r Record, _ uint64) (Receipt, error) { calls++; return receiptFor(r), nil }), 1)
			if e == nil || calls != 0 {
				t.Fatal("unsafe drain")
			}
			st, _ := q.Stats()
			if st.Items != 1 {
				t.Fatal("blocked record lost")
			}
		})
	}
}
func TestGenerationRollbackBlockedAfterRestart(t *testing.T) {
	o := testOptions(t)
	q := openTest(t, o)
	captureTest(t, q, testInput())
	g := reconcileFunc(func(ctx context.Context, k ReplayKey) (Verification, error) {
		v, e := allowed(ctx, k)
		v.Conversations[ConversationFenceKey(k.Binding.Producer, k.Conversation)] = true
		return v, e
	})
	p := publishFunc(func(_ context.Context, r Record, _ uint64) (Receipt, error) {
		t.Fatal("must not publish")
		return receiptFor(r), nil
	})
	if _, e := q.Drain(context.Background(), g, p, 1); e != ErrBlocked {
		t.Fatal(e)
	}
	closeTest(t, q)
	q = openTest(t, o)
	g = reconcileFunc(func(ctx context.Context, k ReplayKey) (Verification, error) {
		v, e := allowed(ctx, k)
		v.Generation = 3
		return v, e
	})
	if _, e := q.Drain(context.Background(), g, p, 1); e != ErrFence {
		t.Fatal("older fence generation accepted")
	}
}
func TestAmbiguousAndIncompletePublicationRetainsSameIdentity(t *testing.T) {
	for _, mode := range []string{"error", "incomplete", "wrong_digest", "wrong_key", "mutated_attachment"} {
		t.Run(mode, func(t *testing.T) {
			q := openTest(t, testOptions(t))
			in := testInput()
			in.Attachments = []AttachmentInput{{"a", "image/png", "exposed", "clear", true, []byte{1, 2, 3}}}
			captureTest(t, q, in)
			_, e := q.Drain(context.Background(), reconcileFunc(allowed), publishFunc(func(_ context.Context, r Record, _ uint64) (Receipt, error) {
				rc := receiptFor(r)
				switch mode {
				case "error":
					return Receipt{}, ErrBlocked
				case "incomplete":
					rc.Complete = false
				case "wrong_digest":
					rc.Digest = "bad"
				case "wrong_key":
					rc.Key.Event = "other"
				case "mutated_attachment":
					r.Attachments[0].Bytes[0] = 9
					rc = receiptFor(r)
				}
				return rc, nil
			}), 1)
			if e != ErrBlocked {
				t.Fatal("invalid receipt accepted")
			}
			st, _ := q.Stats()
			if st.Items != 1 {
				t.Fatal("ambiguous item lost")
			}
			_, e = q.Drain(context.Background(), reconcileFunc(allowed), publishFunc(func(_ context.Context, r Record, _ uint64) (Receipt, error) {
				if r.Event != in.Event || r.Attachments[0].Bytes[0] != 1 {
					t.Fatal("retry changed")
				}
				return receiptFor(r), nil
			}), 1)
			if e != nil {
				t.Fatal(e)
			}
		})
	}
}
func TestFilesystemProtections(t *testing.T) {
	for _, mode := range []string{"directory_mode", "directory_symlink", "data_mode", "data_symlink", "hardlink", "corrupt", "lock_symlink", "pending_symlink"} {
		t.Run(mode, func(t *testing.T) {
			o := testOptions(t)
			q := openTest(t, o)
			captureTest(t, q, testInput())
			closeTest(t, q)
			data := filepath.Join(o.Directory, "queue.bin")
			switch mode {
			case "directory_mode":
				if e := os.Chmod(o.Directory, 0755); e != nil {
					t.Fatal(e)
				}
			case "directory_symlink":
				dest := o.Directory + "-real"
				if e := os.Rename(o.Directory, dest); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(dest, o.Directory); e != nil {
					t.Fatal(e)
				}
			case "data_mode":
				if e := os.Chmod(data, 0644); e != nil {
					t.Fatal(e)
				}
			case "data_symlink":
				dest := filepath.Join(filepath.Dir(o.Directory), "external")
				if e := os.Rename(data, dest); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(dest, data); e != nil {
					t.Fatal(e)
				}
			case "hardlink":
				if e := os.Link(data, filepath.Join(filepath.Dir(o.Directory), "linked")); e != nil {
					t.Fatal(e)
				}
			case "corrupt":
				if e := os.WriteFile(data, []byte("not-authenticated"), 0600); e != nil {
					t.Fatal(e)
				}
			case "lock_symlink":
				lock := filepath.Join(o.Directory, "queue.lock")
				if e := os.Remove(lock); e != nil {
					t.Fatal(e)
				}
				if e := os.Symlink(data, lock); e != nil {
					t.Fatal(e)
				}
			case "pending_symlink":
				if e := os.Symlink(data, filepath.Join(o.Directory, "queue.pending")); e != nil {
					t.Fatal(e)
				}
			}
			if got, e := OpenQueue(o); e != ErrStorage {
				if got != nil {
					closeTest(t, got)
				}
				t.Fatalf("unsafe storage accepted: %v", e)
			}
		})
	}
}
func TestConcurrentHandlesNoLostEnqueue(t *testing.T) {
	o := testOptions(t)
	q1 := openTest(t, o)
	q2 := openTest(t, o)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := testInput()
			in.Event = fmt.Sprintf("event-%d", i)
			in.Sequence = uint64(i + 1)
			q := q1
			if i%2 == 1 {
				q = q2
			}
			_, e := q.Capture(in)
			errs <- e
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	st, e := q1.Stats()
	if e != nil || st.Items != 20 {
		t.Fatalf("lost concurrent input: %v %+v", e, st)
	}
	sequence := uint64(0)
	out, e := q2.Drain(context.Background(), reconcileFunc(allowed), publishFunc(func(_ context.Context, r Record, _ uint64) (Receipt, error) {
		if r.Sequence <= sequence {
			t.Fatal("out of order")
		}
		sequence = r.Sequence
		return receiptFor(r), nil
	}), 64)
	if e != nil || out.Published != 20 {
		t.Fatalf("drain: %v %+v", e, out)
	}
}
func TestPauseSerializesWithInflightPublication(t *testing.T) {
	o := testOptions(t)
	q1 := openTest(t, o)
	q2 := openTest(t, o)
	in := testInput()
	captureTest(t, q1, in)
	in.Event = "event-2"
	in.Sequence = 2
	captureTest(t, q1, in)
	entered := make(chan struct{})
	release := make(chan struct{})
	drained := make(chan error, 1)
	pausedDone := make(chan error, 1)
	go func() {
		_, e := q1.Drain(context.Background(), reconcileFunc(allowed), publishFunc(func(_ context.Context, r Record, _ uint64) (Receipt, error) {
			close(entered)
			<-release
			return receiptFor(r), nil
		}), 1)
		drained <- e
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("drain did not enter")
	}
	go func() { _, e := q2.Pause(Scope{}); pausedDone <- e }()
	select {
	case <-pausedDone:
		t.Fatal("pause passed in-flight publication")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if e := <-drained; e != nil {
		t.Fatal(e)
	}
	if e := <-pausedDone; e != nil {
		t.Fatal(e)
	}
	st, e := q1.Stats()
	if e != nil || st.Items != 0 || st.PausedScopes != 1 {
		t.Fatal("pause did not clear remaining queue")
	}
	if _, e = q1.Capture(in); e != ErrPaused {
		t.Fatal("post-pause capture")
	}
}

func TestInterruptedSnapshotNeverReplacesCommittedQueue(t *testing.T) {
	o := testOptions(t)
	q := openTest(t, o)
	captureTest(t, q, testInput())
	closeTest(t, q)
	if e := os.WriteFile(filepath.Join(o.Directory, "queue.pending"), []byte("interrupted-ciphertext"), 0600); e != nil {
		t.Fatal(e)
	}
	q = openTest(t, o)
	st, e := q.Stats()
	if e != nil || st.Items != 1 {
		t.Fatal("crash residue replaced committed state")
	}
	if _, e = os.Stat(filepath.Join(o.Directory, "queue.pending")); !os.IsNotExist(e) {
		t.Fatal("stale staging residue retained")
	}
}
func TestPermissionChangeAndLockReplacementFailClosed(t *testing.T) {
	for _, mode := range []string{"directory", "data", "lock"} {
		t.Run(mode, func(t *testing.T) {
			o := testOptions(t)
			q := openTest(t, o)
			captureTest(t, q, testInput())
			switch mode {
			case "directory":
				if e := os.Chmod(o.Directory, 0755); e != nil {
					t.Fatal(e)
				}
			case "data":
				if e := os.Chmod(filepath.Join(o.Directory, "queue.bin"), 0644); e != nil {
					t.Fatal(e)
				}
			case "lock":
				p := filepath.Join(o.Directory, "queue.lock")
				if e := os.Remove(p); e != nil {
					t.Fatal(e)
				}
				if e := os.WriteFile(p, nil, 0600); e != nil {
					t.Fatal(e)
				}
			}
			if _, e := q.Stats(); e != ErrStorage {
				t.Fatal("changed permissions/inode accepted")
			}
		})
	}
}
func TestImportQueueRestartUsesSamePendingIdentity(t *testing.T) {
	o := testOptions(t)
	q := openTest(t, o)
	in := testInput()
	in.Event = ""
	in.Sequence = 0
	b := line(in)
	src := LogSource{"source", "one"}
	first, e := Import(bytes.NewReader(b), src, Cursor{}, q.Capture)
	if e != nil || first.Admitted != 1 {
		t.Fatal(e)
	}
	closeTest(t, q)
	q = openTest(t, o)
	// Simulates a crash after Capture but before the adapter persisted its cursor.
	second, e := Import(bytes.NewReader(b), src, Cursor{}, q.Capture)
	if e != nil || second.Cursor != first.Cursor {
		t.Fatal("replay identity changed")
	}
	st, e := q.Stats()
	if e != nil || st.Items != 1 {
		t.Fatal("duplicate queued after interrupted import")
	}
}

func TestKnownFenceBlocksRecaptureAndCannotDisappear(t *testing.T) {
	o := testOptions(t)
	q := openTest(t, o)
	in := testInput()
	captureTest(t, q, in)
	fenced := reconcileFunc(func(ctx context.Context, k ReplayKey) (Verification, error) {
		v, e := allowed(ctx, k)
		v.Conversations[ConversationFenceKey(k.Binding.Producer, k.Conversation)] = true
		return v, e
	})
	never := publishFunc(func(_ context.Context, r Record, _ uint64) (Receipt, error) {
		t.Fatal("fenced content published")
		return receiptFor(r), nil
	})
	if _, e := q.Drain(context.Background(), fenced, never, 1); e != ErrBlocked {
		t.Fatal(e)
	}
	closeTest(t, q)
	q = openTest(t, o)
	in.Event = "new-event"
	in.Sequence = 2
	if _, e := q.Capture(in); e != ErrBlocked {
		t.Fatal("known fence did not stop local recapture")
	}
	for _, generation := range []uint64{4, 5} {
		g := reconcileFunc(func(ctx context.Context, k ReplayKey) (Verification, error) {
			v, e := allowed(ctx, k)
			v.Generation = generation
			return v, e
		})
		if _, e := q.Drain(context.Background(), g, never, 1); e != ErrFence {
			t.Fatal("observed fence disappeared")
		}
	}
	scope := Scope{Conversation: in.Conversation}
	if _, e := q.Pause(scope); e != nil {
		t.Fatal(e)
	}
	if e := q.Resume(scope); e != nil {
		t.Fatal(e)
	}
	if _, e := q.Capture(in); e != ErrBlocked {
		t.Fatal("resume overrode server fence")
	}
}

func TestFullOpaqueFenceSetSurvivesUnrelatedConversationAndRestart(t *testing.T) {
	o := testOptions(t)
	q := openTest(t, o)
	in := testInput()
	fence := ConversationFenceKey(o.Policy.Binding.Producer, "deleted-conversation")
	other := ConversationFenceKey("other-producer", in.Conversation)
	reconcile := reconcileFunc(func(ctx context.Context, k ReplayKey) (Verification, error) {
		v, e := allowed(ctx, k)
		v.Conversations[fence] = true
		v.Conversations[other] = true
		return v, e
	})
	publish := publishFunc(func(_ context.Context, r Record, _ uint64) (Receipt, error) { return receiptFor(r), nil })
	captureTest(t, q, in)
	if r, e := q.Drain(context.Background(), reconcile, publish, 1); e != nil || r.Published != 1 {
		t.Fatalf("first: %+v %v", r, e)
	}
	closeTest(t, q)
	q = openTest(t, o)
	in.Conversation = "another-live-conversation"
	captureTest(t, q, in)
	if r, e := q.Drain(context.Background(), reconcile, publish, 1); e != nil || r.Published != 1 {
		t.Fatalf("restart: %+v %v", r, e)
	}
	in.Conversation = "deleted-conversation"
	if _, e := q.Capture(in); e != ErrBlocked {
		t.Fatalf("forgot known fence: %v", e)
	}
}

func closeTest(t *testing.T, q *Queue) {
	t.Helper()
	if err := q.Close(); err != nil {
		t.Fatal(err)
	}
}
