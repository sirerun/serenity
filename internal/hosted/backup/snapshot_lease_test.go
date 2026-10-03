package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/privatefs"
	"github.com/sirerun/serenity/internal/hosted/store"
)

type leaseTestAuthority struct {
	mu        sync.Mutex
	attempts  map[string]PinAttemptRef
	canceled  map[uint64]PinAttemptRef
	decisions map[string]PinReconcileDecision
	completes int
}

func newLeaseTestAuthority() *leaseTestAuthority {
	return &leaseTestAuthority{attempts: map[string]PinAttemptRef{}, canceled: map[uint64]PinAttemptRef{}, decisions: map[string]PinReconcileDecision{}}
}
func attemptKey(ref, digest string) string { return ref + ":" + digest }
func (a *leaseTestAuthority) BeginPinAttempt(_ context.Context, ref string, version uint64, lease, digest string) (PinAttemptRef, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := attemptKey(ref, digest)
	if old, ok := a.attempts[key]; ok && old.State == PinAttemptPending {
		if old.ReservationVersion == version && old.LeaseID == lease {
			return old, nil
		}
		return PinAttemptRef{}, ErrSnapshotLeaseConflict
	}
	n := uint64(1)
	if old, ok := a.attempts[key]; ok {
		n = old.AttemptVersion + 1
	} else {
		for _, old := range a.canceled {
			if old.PlanRef == ref && old.ManifestSHA256 == digest && old.LeaseID == lease && old.ReservationVersion == version && old.AttemptVersion >= n {
				n = old.AttemptVersion + 1
			}
		}
	}
	v := PinAttemptRef{PlanRef: ref, LeaseID: lease, ManifestSHA256: digest, ReservationVersion: version, AttemptVersion: n, State: PinAttemptPending}
	a.attempts[key] = v
	return v, nil
}
func (a *leaseTestAuthority) CommitPinAttempt(_ context.Context, v PinAttemptRef, p SnapshotPinRef) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := attemptKey(v.PlanRef, v.ManifestSHA256)
	cur, ok := a.attempts[key]
	if !ok || !sameAttempt(cur, v) || cur.State != PinAttemptPending || p.PlanRef() != v.PlanRef || p.ManifestSHA256() != v.ManifestSHA256 || p.ReservationVersion() != v.ReservationVersion || !isCanonicalID(p.ID()) {
		return ErrSnapshotLeaseConflict
	}
	cur.State = PinAttemptCommitted
	a.attempts[key] = cur
	return nil
}
func (a *leaseTestAuthority) CancelPinAttempt(_ context.Context, v PinAttemptRef) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if old, ok := a.canceled[v.AttemptVersion]; ok && sameAttempt(old, v) {
		return nil
	}
	key := attemptKey(v.PlanRef, v.ManifestSHA256)
	cur, ok := a.attempts[key]
	if !ok || !sameAttempt(cur, v) || cur.State != PinAttemptPending {
		return ErrSnapshotLeaseConflict
	}
	delete(a.attempts, key)
	a.canceled[v.AttemptVersion] = v
	return nil
}
func (a *leaseTestAuthority) FindPinAttempt(_ context.Context, ref, digest string) (PinAttemptRef, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	v, ok := a.attempts[attemptKey(ref, digest)]
	if !ok {
		return PinAttemptRef{}, ErrSnapshotLeaseNotFound
	}
	return v, nil
}
func (a *leaseTestAuthority) ListPinAttempts(context.Context) ([]PinAttemptRef, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := []PinAttemptRef{}
	for _, v := range a.attempts {
		if v.State == PinAttemptPending {
			out = append(out, v)
		}
	}
	return out, nil
}
func (a *leaseTestAuthority) ReconcilePin(_ context.Context, p SnapshotPinRef) (PinReconcileDecision, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if d, ok := a.decisions[p.ID()]; ok {
		return d, nil
	}
	return PinReconcileDecision{Action: PinKeep}, nil
}
func (a *leaseTestAuthority) CompletePinRelease(context.Context, PinReleaseAuthorization) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.completes++
	return nil
}

func testLeaseStore(t *testing.T, root string, a SnapshotPinLifecycleAuthority) *SnapshotLeaseStore {
	t.Helper()
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := privatefs.ValidateDirectory(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	s, err := NewSnapshotLeaseStore(context.Background(), SnapshotLeaseStoreOptions{LeaseRoot: root, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 2 << 20, MaxLeases: 8}, a)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSnapshotLeaseStagesExactBytesAndRestoresAfterSourceRemoval(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 1)
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-store")
	a := newLeaseTestAuthority()
	s := testLeaseStore(t, root, a)
	lease, err := s.Stage(context.Background(), snapshot, opts)
	if err != nil {
		t.Fatal(err)
	}
	inspection := lease.Inspection()
	if inspection.ManifestSHA256 != opts.ExpectedManifestSHA256 || len(inspection.Accounts) != 1 {
		t.Fatalf("inspection mismatch: %+v", inspection)
	}
	account := inspection.Accounts[0]
	candidate, err := lease.Candidate(context.Background(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.AccountID() != account.ID || candidate.SnapshotStatus() != account.Status || candidate.ManifestSHA256() != opts.ExpectedManifestSHA256 {
		t.Fatalf("candidate mismatch: %+v", candidate)
	}
	if err = os.RemoveAll(snapshot); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(privateTempDir(t), "restored-from-lease")
	if err = RestoreVerified(context.Background(), lease, destination); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(destination, controlDBName)); err != nil {
		t.Fatal(err)
	}
	if err = lease.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotLeasePinSurvivesReopenAndExactCancelAttempt(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 0)
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-pins")
	a := newLeaseTestAuthority()
	s := testLeaseStore(t, root, a)
	lease, err := s.Stage(context.Background(), snapshot, opts)
	if err != nil {
		t.Fatal(err)
	}
	ref := strings.Repeat("a", 64)
	pin, err := lease.Pin(context.Background(), ref, 7, opts.ExpectedManifestSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if !isCanonicalID(pin.ID()) {
		t.Fatalf("noncanonical pin: %q", pin.ID())
	}
	if err = lease.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	s2, err := NewSnapshotLeaseStore(context.Background(), s.options, a)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := s2.ResolvePinned(context.Background(), pin.ID(), pin.ManifestSHA256(), pin.PlanRef())
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := s2.ReopenPinned(context.Background(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.LeaseID() != lease.LeaseID() {
		t.Fatalf("reopened wrong lease: %s", reopened.LeaseID())
	}
	if err = reopened.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, err := s2.FindPinned(context.Background(), ref, opts.ExpectedManifestSHA256); err != nil || got.ID() != pin.ID() {
		t.Fatalf("FindPinned=(%q,%v), want %q", got.ID(), err, pin.ID())
	}
	if err = os.RemoveAll(snapshot); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestSnapshotLeaseRejectsArtifactTamperBeforeRestore(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 1)
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-tamper")
	s := testLeaseStore(t, root, newLeaseTestAuthority())
	lease, err := s.Stage(context.Background(), snapshot, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lease.Close(context.Background()) }()
	name := lease.record.Files[0].Name
	path := filepath.Join(root, lease.LeaseID(), name)
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteAt([]byte{0}, 0); err != nil {
		t.Fatal(errors.Join(err, f.Close()))
	}
	if err = f.Sync(); err != nil {
		t.Fatal(errors.Join(err, f.Close()))
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(privateTempDir(t), "tampered-restore")
	if err = RestoreVerified(context.Background(), lease, destination); !errors.Is(err, ErrSnapshotLeaseInvalid) {
		t.Fatalf("RestoreVerified after tamper = %v, want invalid lease", err)
	}
	if _, err = os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination created after tamper: %v", err)
	}
}

func TestSnapshotLeaseRejectsReplacedStoreRoot(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 0)
	opts, _ := inspectionOptions(t, snapshot)
	parent := privateTempDir(t)
	root := filepath.Join(parent, "snapshot-lease-root")
	s := testLeaseStore(t, root, newLeaseTestAuthority())
	moved := root + ".moved"
	if err := os.Rename(root, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stage(context.Background(), snapshot, opts); !errors.Is(err, ErrSnapshotLeaseInvalid) {
		t.Fatalf("Stage through replaced root = %v, want invalid lease", err)
	}
}

func TestSnapshotLeaseReconcileCompletesInterruptedReleaseWithDurableTombstone(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 0)
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-release-recovery")
	a := newLeaseTestAuthority()
	s := testLeaseStore(t, root, a)
	lease, err := s.Stage(context.Background(), snapshot, opts)
	if err != nil {
		t.Fatal(err)
	}
	plan := strings.Repeat("b", 64)
	pin, err := lease.Pin(context.Background(), plan, 9, opts.ExpectedManifestSHA256)
	if err != nil {
		t.Fatal(err)
	}
	auth := PinReleaseAuthorization{PinID: pin.ID(), PlanRef: pin.PlanRef(), ManifestSHA256: pin.ManifestSHA256(), Disposition: PinAbandonedBeforeEffects, RecordVersion: 11}
	record, err := readLeaseRecord(filepath.Join(root, lease.LeaseID()), s.options.MaxMetadataBytesPerLease)
	if err != nil {
		t.Fatal(err)
	}
	record.State = "RELEASING"
	record.Disposition = auth.Disposition
	record.RecordVersion = auth.RecordVersion
	if err = writeLeaseRecord(filepath.Join(root, lease.LeaseID()), &record, s.options.MaxMetadataBytesPerLease); err != nil {
		t.Fatal(err)
	}
	if err = s.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, lease.LeaseID())); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lease remains after release recovery: %v", err)
	}
	marker := filepath.Join(root, ".releases", lease.LeaseID())
	tombstone, err := readReleaseTombstone(marker)
	if err != nil || tombstone.State != "RELEASED" {
		t.Fatalf("durable RELEASED tombstone=(%+v,%v)", tombstone, err)
	}
	if err = s.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("completed release tombstone was not cleaned: %v", err)
	}
	a.mu.Lock()
	completes := a.completes
	a.mu.Unlock()
	if completes < 1 {
		t.Fatal("release owner completion was not called")
	}
}

func TestSnapshotLeaseCancelExactAttemptTombstoneDoesNotTouchLaterAttempt(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 0)
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-cancel")
	a := newLeaseTestAuthority()
	s := testLeaseStore(t, root, a)
	lease, err := s.Stage(context.Background(), snapshot, opts)
	if err != nil {
		t.Fatal(err)
	}
	plan := strings.Repeat("c", 64)
	first, err := a.BeginPinAttempt(context.Background(), plan, 12, lease.LeaseID(), opts.ExpectedManifestSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CancelPin(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second, err := a.BeginPinAttempt(context.Background(), plan, 12, lease.LeaseID(), opts.ExpectedManifestSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if second.AttemptVersion <= first.AttemptVersion {
		t.Fatalf("retry attempt did not advance: first=%d second=%d", first.AttemptVersion, second.AttemptVersion)
	}
	if err = s.CancelPin(context.Background(), first); err != nil {
		t.Fatalf("retry exact canceled N tombstone while N+1 pending: %v", err)
	}
	current, err := a.FindPinAttempt(context.Background(), plan, opts.ExpectedManifestSHA256)
	if err != nil || !sameAttempt(current, second) || current.State != PinAttemptPending {
		t.Fatalf("stale N retry touched N+1: current=%+v err=%v", current, err)
	}
	pending := lease.record
	pending.State = "PIN_PENDING"
	pending.PlanRef = plan
	pending.PinID = strings.Repeat("d", 64)
	pending.ReservationVersion = second.ReservationVersion
	pending.AttemptVersion = second.AttemptVersion
	if err = writeLeaseRecord(lease.path, &pending, s.options.MaxMetadataBytesPerLease); err != nil {
		t.Fatal(err)
	}
	if err = s.CancelPin(context.Background(), second); !errors.Is(err, ErrSnapshotLeaseConflict) {
		t.Fatalf("cancel exact durable pin = %v, want conflict", err)
	}
	if err = lease.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotLeaseCrossProcessBorrowLock(t *testing.T) {
	if mode := os.Getenv("SERENITY_LEASE_LOCK_HELPER_MODE"); mode != "" {
		path := os.Getenv("SERENITY_LEASE_LOCK_HELPER_PATH")
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		lock, err := lockLeaseContext(ctx, path, false)
		if mode == "blocked" {
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("exclusive subprocess lock while parent holds shared lock = %v, want deadline", err)
			}
			_, _ = os.Stdout.WriteString("blocked\n")
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		if err = errors.Join(unlockFile(lock), lock.Close()); err != nil {
			t.Fatal(err)
		}
		_, _ = os.Stdout.WriteString("acquired\n")
		return
	}
	_, snapshot, _ := freshPrivateSnapshot(t, 0)
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-cross-process-lock")
	lease, err := testLeaseStore(t, root, newLeaseTestAuthority()).Stage(context.Background(), snapshot, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lease.Close(context.Background()) }()
	shared, err := lockLeaseContext(context.Background(), lease.path, true)
	if err != nil {
		t.Fatal(err)
	}
	runHelper := func(mode string) error {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=^TestSnapshotLeaseCrossProcessBorrowLock$", "-test.count=1")
		cmd.Env = append(os.Environ(), "SERENITY_LEASE_LOCK_HELPER_MODE="+mode, "SERENITY_LEASE_LOCK_HELPER_PATH="+lease.path)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("lock helper %s: %w: %s", mode, err, out)
		}
		if !strings.Contains(string(out), mode+"\n") {
			return fmt.Errorf("lock helper %s output=%q missing readiness", mode, out)
		}
		return nil
	}
	if err = runHelper("blocked"); err != nil {
		t.Fatal(err)
	}
	if err = errors.Join(unlockFile(shared), shared.Close()); err != nil {
		t.Fatal(err)
	}
	if err = runHelper("acquired"); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotLeaseCandidatePreservesNullableCheckoutSession(t *testing.T) {
	dataDir, _ := newTestDataDir(t, 0)
	db, err := store.Open(filepath.Join(dataDir, controlDBName))
	if err != nil {
		t.Fatal(err)
	}
	var accountID string
	if err = db.DB().QueryRow(`SELECT id FROM accounts LIMIT 1`).Scan(&accountID); err != nil {
		t.Fatal(errors.Join(err, db.Close()))
	}
	_, err = db.DB().Exec(`INSERT INTO checkout_attempts(account_id,id,price_id,session_id,created_at,request_version,request_body) VALUES(?,?,?,?,?,1,?)`, accountID, "attempt-null", "price_null", nil, store.Stamp(time.Now()), "mode=subscription&customer=cus_null")
	if err != nil {
		t.Fatal(errors.Join(err, db.Close()))
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(privateTempDir(t), "snapshot-null-session")
	if err = Create(context.Background(), dataDir, snapshot, "test-build-sha", fakeJournal{watermark: testWatermark()}); err != nil {
		t.Fatal(err)
	}
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-null-session")
	lease, err := testLeaseStore(t, root, newLeaseTestAuthority()).Stage(context.Background(), snapshot, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lease.Close(context.Background()) }()
	candidate, err := lease.Candidate(context.Background(), accountID)
	if err != nil {
		t.Fatal(err)
	}
	attempts := candidate.CheckoutAttempts()
	if len(attempts) != 1 || attempts[0].HasSessionRef || attempts[0].SessionRef != "" {
		t.Fatalf("NULL checkout session lost its null marker: %+v", attempts)
	}
}

func TestSnapshotLeaseAggregateArtifactAndMetadataBudgetsStopSecondStage(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 0)
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-aggregate-budget")
	s := testLeaseStore(t, root, newLeaseTestAuthority())
	first, err := s.Stage(context.Background(), snapshot, opts)
	if err != nil {
		t.Fatal(err)
	}
	s.options.MaxRetainedArtifactBytes = first.record.ArtifactBytes
	if _, err = s.Stage(context.Background(), snapshot, opts); !errors.Is(err, ErrSnapshotLeaseLimit) {
		t.Fatalf("second stage beyond aggregate artifact budget = %v, want limit", err)
	}
	first.record, err = readLeaseRecord(first.path, s.options.MaxMetadataBytesPerLease)
	if err != nil {
		t.Fatal(err)
	}
	s.options.MaxRetainedArtifactBytes = 2 << 30
	s.options.MaxRetainedMetadataBytes = first.record.MetadataBytes
	if _, err = s.Stage(context.Background(), snapshot, opts); !errors.Is(err, ErrSnapshotLeaseLimit) {
		t.Fatalf("second stage beyond aggregate metadata reservation = %v, want limit", err)
	}
	if err = first.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
