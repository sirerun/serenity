package backup

import (
	"context"
	"database/sql"
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
	mu              sync.Mutex
	attempts        map[string]PinAttemptRef
	canceled        map[string]PinAttemptRef
	decisions       map[string]PinReconcileDecision
	completes       int
	identity        SnapshotStoreIdentity
	consumeAttempt  func(PinAttemptRef) PinAttemptRef
	consumeIdentity *SnapshotStoreIdentity
	savedProof      VerifiedPinAbsence
	beforeReconcile func(SnapshotPinRef) error
	beforeComplete  func(PinReleaseAuthorization) error
}

func newLeaseTestAuthority() *leaseTestAuthority {
	return &leaseTestAuthority{attempts: map[string]PinAttemptRef{}, canceled: map[string]PinAttemptRef{}, decisions: map[string]PinReconcileDecision{}}
}
func attemptKey(ref, digest string) string { return ref + ":" + digest }
func attemptVersionKey(v PinAttemptRef) string {
	return fmt.Sprintf("%s:%d:%s:%d", attemptKey(v.PlanRef, v.ManifestSHA256), v.ReservationVersion, v.LeaseID, v.AttemptVersion)
}
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
			if old.PlanRef == ref && old.ManifestSHA256 == digest && old.ReservationVersion == version && old.LeaseID == lease {
				return PinAttemptRef{}, ErrSnapshotLeaseConflict
			}
			if old.PlanRef == ref && old.ManifestSHA256 == digest && old.ReservationVersion == version && old.AttemptVersion >= n {
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
func (a *leaseTestAuthority) CancelPinAttempt(_ context.Context, v PinAttemptRef, proof VerifiedPinAbsence) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.savedProof = proof
	consume := v
	if a.consumeAttempt != nil {
		consume = a.consumeAttempt(v)
	}
	identity := a.identity
	if a.consumeIdentity != nil {
		identity = *a.consumeIdentity
	}
	if err := proof.Consume(consume, identity); err != nil {
		return err
	}
	if old, ok := a.canceled[attemptVersionKey(v)]; ok && sameAttempt(old, v) {
		return nil
	}
	key := attemptKey(v.PlanRef, v.ManifestSHA256)
	cur, ok := a.attempts[key]
	if !ok || !sameAttempt(cur, v) || cur.State != PinAttemptPending {
		return ErrSnapshotLeaseConflict
	}
	delete(a.attempts, key)
	a.canceled[attemptVersionKey(v)] = v
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
		out = append(out, v)
	}
	return out, nil
}
func (a *leaseTestAuthority) ReconcilePin(_ context.Context, p SnapshotPinRef) (PinReconcileDecision, error) {
	a.mu.Lock()
	d, ok := a.decisions[p.ID()]
	before := a.beforeReconcile
	a.mu.Unlock()
	if before != nil {
		if err := before(p); err != nil {
			return PinReconcileDecision{}, err
		}
	}
	if ok {
		return d, nil
	}
	return PinReconcileDecision{Action: PinKeep}, nil
}
func (a *leaseTestAuthority) CompletePinRelease(_ context.Context, authorization PinReleaseAuthorization) error {
	a.mu.Lock()
	a.completes++
	before := a.beforeComplete
	a.mu.Unlock()
	if before != nil {
		return before(authorization)
	}
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
	options := SnapshotLeaseStoreOptions{LeaseRoot: root, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 2 << 20, MaxLeases: 8}
	return testLeaseStoreOptions(t, options, a)
}

func testLeaseStoreOptions(t *testing.T, options SnapshotLeaseStoreOptions, a SnapshotPinLifecycleAuthority) *SnapshotLeaseStore {
	t.Helper()
	identity, err := PreflightSnapshotStoreIdentity(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	if ta, ok := a.(*leaseTestAuthority); ok {
		ta.identity = identity
	}
	s, err := NewSnapshotLeaseStore(context.Background(), options, a, identity)
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

func TestSnapshotLeaseStageDoesNotRetainExpandedBrainRepository(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 1)
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-no-expanded-repo")
	s := testLeaseStore(t, root, newLeaseTestAuthority())
	lease, err := s.Stage(context.Background(), snapshot, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lease.Close(context.Background()) }()

	entries, err := os.ReadDir(lease.path)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{leaseRecordName: true, manifestFile: true, controlDBName: true}
	for _, name := range bundleNames(lease.record.Manifest) {
		allowed[name] = true
	}
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !allowed[entry.Name()] {
			t.Errorf("staged lease retained expanded or unexpected entry %q (mode %s)", entry.Name(), entry.Type())
		}
		delete(allowed, entry.Name())
	}
	for name := range allowed {
		t.Errorf("staged lease is missing expected raw/metadata entry %q", name)
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
	s2, err := NewSnapshotLeaseStore(context.Background(), s.options, a, s.identity)
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

func TestSnapshotLeaseCancelRequiresExactLiveAbsenceProof(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 0)
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-proof")
	a := newLeaseTestAuthority()
	s := testLeaseStore(t, root, a)
	lease, err := s.Stage(context.Background(), snapshot, opts)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := a.BeginPinAttempt(context.Background(), strings.Repeat("b", 64), 3, lease.record.ID, opts.ExpectedManifestSHA256)
	if err != nil {
		t.Fatal(err)
	}
	a.consumeAttempt = func(v PinAttemptRef) PinAttemptRef { v.AttemptVersion++; return v }
	if err = s.CancelPin(context.Background(), attempt); !errors.Is(err, ErrSnapshotLeaseConflict) {
		t.Fatalf("mutated proof tuple accepted: %v", err)
	}
	a.consumeAttempt = nil
	otherRoot := filepath.Join(privateTempDir(t), "snapshot-lease-proof-other")
	if err = os.Mkdir(otherRoot, 0700); err != nil {
		t.Fatal(err)
	}
	otherOptions := s.options
	otherOptions.LeaseRoot = otherRoot
	otherIdentity, identityErr := PreflightSnapshotStoreIdentity(context.Background(), otherOptions)
	if identityErr != nil {
		t.Fatal(identityErr)
	}
	a.consumeIdentity = &otherIdentity
	if err = s.CancelPin(context.Background(), attempt); !errors.Is(err, ErrSnapshotLeaseConflict) {
		t.Fatalf("cross-store proof identity accepted: %v", err)
	}
	a.consumeIdentity = nil
	if err = s.CancelPin(context.Background(), attempt); err != nil {
		t.Fatalf("valid fresh proof rejected: %v", err)
	}
	if err = a.savedProof.Consume(attempt, a.identity); !errors.Is(err, ErrSnapshotLeaseConflict) {
		t.Fatalf("callback-retained proof remained usable: %v", err)
	}
	if err = lease.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotLeaseStoreIdentityRejectsZeroAndCrossRoot(t *testing.T) {
	rootA := filepath.Join(privateTempDir(t), "identity-a")
	if err := os.Mkdir(rootA, 0700); err != nil {
		t.Fatal(err)
	}
	rootB := filepath.Join(privateTempDir(t), "identity-b")
	if err := os.Mkdir(rootB, 0700); err != nil {
		t.Fatal(err)
	}
	optionsA := SnapshotLeaseStoreOptions{LeaseRoot: rootA, MaxArtifactBytesPerLease: 1 << 20, MaxMetadataBytesPerLease: 1 << 16, MaxRestoreScratchBytes: 1 << 20, MaxRetainedArtifactBytes: 2 << 20, MaxRetainedMetadataBytes: 2 << 16, MaxLeases: 2}
	optionsB := optionsA
	optionsB.LeaseRoot = rootB
	identityA, err := PreflightSnapshotStoreIdentity(context.Background(), optionsA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = NewSnapshotLeaseStore(context.Background(), optionsA, newLeaseTestAuthority(), SnapshotStoreIdentity{}); !errors.Is(err, ErrSnapshotLeaseInvalid) {
		t.Fatalf("zero identity accepted: %v", err)
	}
	if _, err = NewSnapshotLeaseStore(context.Background(), optionsB, newLeaseTestAuthority(), identityA); !errors.Is(err, ErrSnapshotLeaseInvalid) {
		t.Fatalf("cross-root identity accepted: %v", err)
	}
	identityB, err := PreflightSnapshotStoreIdentity(context.Background(), optionsB)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(filepath.Join(rootB, ".store.lock"), filepath.Join(rootB, ".store.lock.saved")); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(rootB, ".store.lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = NewSnapshotLeaseStore(context.Background(), optionsB, newLeaseTestAuthority(), identityB); !errors.Is(err, ErrSnapshotLeaseInvalid) {
		t.Fatalf("replaced lock identity accepted: %v", err)
	}
}

func TestSnapshotLeaseStoreLockIdentityAfterBlockingWait(t *testing.T) {
	if os.Getenv("SERENITY_STORE_LOCK_HELPER") == "1" {
		root, ready := os.Getenv("SERENITY_STORE_LOCK_ROOT"), os.Getenv("SERENITY_STORE_LOCK_READY")
		options := SnapshotLeaseStoreOptions{LeaseRoot: root, MaxArtifactBytesPerLease: 1 << 20, MaxMetadataBytesPerLease: 1 << 16, MaxRestoreScratchBytes: 1 << 20, MaxRetainedArtifactBytes: 2 << 20, MaxRetainedMetadataBytes: 2 << 16, MaxLeases: 2}
		s := testLeaseStoreOptions(t, options, newLeaseTestAuthority())
		s.beforeStoreLock = func() {
			if err := os.WriteFile(ready, []byte("waiting"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := s.lock(context.Background()); !errors.Is(err, ErrSnapshotLeaseInvalid) {
			t.Fatalf("waiter accepted substituted lock path: %v", err)
		}
		return
	}
	root := filepath.Join(privateTempDir(t), "store-lock-wait")
	owner := testLeaseStore(t, root, newLeaseTestAuthority())
	_, release, err := owner.lockHandle(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(privateTempDir(t), "store-lock-ready")
	cmd := exec.Command(os.Args[0], "-test.run=^TestSnapshotLeaseStoreLockIdentityAfterBlockingWait$", "-test.count=1")
	cmd.Env = append(os.Environ(), "SERENITY_STORE_LOCK_HELPER=1", "SERENITY_STORE_LOCK_ROOT="+root, "SERENITY_STORE_LOCK_READY="+ready)
	result := make(chan error, 1)
	go func() {
		output, runErr := cmd.CombinedOutput()
		if runErr != nil {
			result <- fmt.Errorf("lock waiter: %w: %s", runErr, output)
			return
		}
		result <- nil
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err = os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = release()
			t.Fatal("store lock waiter did not reach blocking Flock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	lockPath := filepath.Join(root, ".store.lock")
	if err = os.Rename(lockPath, lockPath+".saved"); err != nil {
		_ = release()
		t.Fatal(err)
	}
	if err = os.WriteFile(lockPath, nil, 0600); err != nil {
		_ = release()
		t.Fatal(err)
	}
	if err = release(); err != nil {
		t.Fatal(err)
	}
	if err = <-result; err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotLeasePinAttemptListFailsClosed(t *testing.T) {
	valid := PinAttemptRef{PlanRef: strings.Repeat("c", 64), LeaseID: strings.Repeat("d", 64), ManifestSHA256: strings.Repeat("e", 64), ReservationVersion: 1, AttemptVersion: 1, State: PinAttemptPending}
	if err := validatePinAttemptList([]PinAttemptRef{valid}); err != nil {
		t.Fatalf("valid current attempt rejected: %v", err)
	}
	unknown := valid
	unknown.State = PinAttemptState(255)
	if err := validatePinAttemptList([]PinAttemptRef{unknown}); !errors.Is(err, ErrSnapshotLeaseInvalid) {
		t.Fatalf("unknown state accepted: %v", err)
	}
	if err := validatePinAttemptList([]PinAttemptRef{valid, valid}); !errors.Is(err, ErrSnapshotLeaseConflict) {
		t.Fatalf("duplicate authority entry accepted: %v", err)
	}
	conflict := valid
	conflict.AttemptVersion++
	if err := validatePinAttemptList([]PinAttemptRef{valid, conflict}); !errors.Is(err, ErrSnapshotLeaseConflict) {
		t.Fatalf("conflicting current attempt accepted: %v", err)
	}
}

func TestSnapshotLeaseCommittedOwnerCannotAuthorizeDeletingStagedBytes(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 0)
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-owner-conflict")
	a := newLeaseTestAuthority()
	s := testLeaseStore(t, root, a)
	lease, err := s.Stage(context.Background(), snapshot, opts)
	if err != nil {
		t.Fatal(err)
	}
	attempt := PinAttemptRef{PlanRef: strings.Repeat("f", 64), LeaseID: lease.record.ID, ManifestSHA256: opts.ExpectedManifestSHA256, ReservationVersion: 2, AttemptVersion: 1, State: PinAttemptCommitted}
	a.mu.Lock()
	a.attempts[attemptKey(attempt.PlanRef, attempt.ManifestSHA256)] = attempt
	a.mu.Unlock()
	if err = lease.Close(context.Background()); !errors.Is(err, ErrSnapshotLeaseConflict) {
		t.Fatalf("Close accepted committed owner with local STAGED record: %v", err)
	}
	if err = s.Reconcile(context.Background()); !errors.Is(err, ErrSnapshotLeaseConflict) {
		t.Fatalf("Reconcile accepted committed owner with local STAGED record: %v", err)
	}
	if _, err = os.Stat(lease.path); err != nil {
		t.Fatalf("conflicting staged bytes were deleted: %v", err)
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
	a.mu.Lock()
	a.decisions[pin.ID()] = PinReconcileDecision{Action: PinRelease, Authorization: auth}
	a.mu.Unlock()
	if err = s.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, lease.LeaseID())); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lease remains after release recovery: %v", err)
	}
	marker := filepath.Join(root, ".releases", lease.LeaseID())
	tombstone, err := readReleaseTombstone(marker)
	if err != nil || tombstone.State != "RELEASED" || tombstone.ReservationVersion != record.ReservationVersion || tombstone.AttemptVersion != record.AttemptVersion {
		t.Fatalf("durable RELEASED tombstone=(%+v,%v)", tombstone, err)
	}
	exact := PinAttemptRef{PlanRef: plan, LeaseID: record.ID, ManifestSHA256: record.ManifestSHA256, ReservationVersion: record.ReservationVersion, AttemptVersion: record.AttemptVersion, State: PinAttemptCommitted}
	if err = validateLocalPinAttemptPairs(root, s.options.MaxMetadataBytesPerLease, []PinAttemptRef{exact}); err != nil {
		t.Fatalf("exact RELEASED tuple rejected: %v", err)
	}
	bad := exact
	bad.AttemptVersion++
	if err = validateLocalPinAttemptPairs(root, s.options.MaxMetadataBytesPerLease, []PinAttemptRef{bad}); !errors.Is(err, ErrSnapshotLeaseConflict) {
		t.Fatalf("RELEASED marker accepted another attempt version: %v", err)
	}
	info, err := os.Stat(filepath.Join(marker, releaseRecordName))
	if err != nil || info.Size() > maxReleaseRecordBytes || info.Size() != tombstone.MetadataBytes {
		t.Fatalf("release tombstone size=%v info=%v err=%v", tombstone.MetadataBytes, info, err)
	}
	usage, err := s.usageLocked()
	if err != nil || usage.metadata < tombstone.MetadataBytes {
		t.Fatalf("release tombstone not retained-budget-accounted: usage=%+v record=%d err=%v", usage, tombstone.MetadataBytes, err)
	}
	if err = s.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(marker); err != nil {
		t.Fatalf("completed release receipt was not retained: %v", err)
	}
	if err = s.Reconcile(context.Background()); err != nil {
		t.Fatalf("second reconciliation with retained committed pair: %v", err)
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
	if _, err = a.BeginPinAttempt(context.Background(), plan, 12, lease.LeaseID(), opts.ExpectedManifestSHA256); !errors.Is(err, ErrSnapshotLeaseConflict) {
		t.Fatalf("owner resurrected the permanently canceled exact tuple: %v", err)
	}
	secondLease, err := s.Stage(context.Background(), snapshot, opts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := a.BeginPinAttempt(context.Background(), plan, 12, secondLease.LeaseID(), opts.ExpectedManifestSHA256)
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
	pending := secondLease.record
	pending.State = "PIN_PENDING"
	pending.PlanRef = plan
	pending.PinID = strings.Repeat("d", 64)
	pending.ReservationVersion = second.ReservationVersion
	pending.AttemptVersion = second.AttemptVersion
	if err = writeLeaseRecord(secondLease.path, &pending, s.options.MaxMetadataBytesPerLease); err != nil {
		t.Fatal(err)
	}
	if err = s.CancelPin(context.Background(), second); !errors.Is(err, ErrSnapshotLeaseConflict) {
		t.Fatalf("cancel exact durable pin = %v, want conflict", err)
	}
	if err = lease.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = secondLease.Close(context.Background()); err != nil {
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
	store := testLeaseStore(t, root, newLeaseTestAuthority())
	lease, err := store.Stage(context.Background(), snapshot, opts)
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
	store.options.MaxMetadataBytesPerLease = lease.record.MetadataBytes
	store.options.MaxRetainedMetadataBytes = lease.record.MetadataBytes
	if _, err = lease.Candidate(context.Background(), accountID); !errors.Is(err, ErrSnapshotLeaseLimit) {
		t.Fatalf("Candidate ignored its reserved intent metadata budget: %v", err)
	}
	store.options.MaxMetadataBytesPerLease = 1 << 20
	store.options.MaxRetainedMetadataBytes = 2 << 20
	lease.beforeCandidateOpen = func(f *os.File) error {
		if err := f.Chmod(0600); err != nil {
			return err
		}
		db, err := sql.Open("sqlite", f.Name())
		if err != nil {
			return errors.Join(err, f.Chmod(0400))
		}
		_, err = db.Exec(`PRAGMA journal_mode=DELETE`)
		if err == nil {
			_, err = db.Exec(`UPDATE accounts SET stripe_customer_id='cus_tampered_after_digest' WHERE id=?`, accountID)
		}
		return errors.Join(err, db.Close(), f.Chmod(0400))
	}
	if _, err = lease.Candidate(context.Background(), accountID); err == nil {
		t.Fatalf("Candidate accepted SQL-valid projection modified after digest and before SQLite open")
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

func TestSnapshotLeaseInterruptedStageIntentIsCountedAndReconciled(t *testing.T) {
	root := filepath.Join(privateTempDir(t), "snapshot-lease-stage-intent")
	a := newLeaseTestAuthority()
	s := testLeaseStore(t, root, a)
	parent, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	name, err := createInspectionScratch(parent)
	if err != nil {
		t.Fatal(errors.Join(err, parent.Close()))
	}
	created, err := parent.Stat(name)
	if err != nil {
		t.Fatal(errors.Join(err, parent.Close()))
	}
	dev, ino, err := fileIdentity(created)
	if err != nil {
		t.Fatal(errors.Join(err, parent.Close()))
	}
	id, err := randomID()
	if err != nil {
		t.Fatal(errors.Join(err, parent.Close()))
	}
	if err = writeStageIntent(root, name, stageIntentRecord{Version: 1, Kind: "stage", ID: id, ScratchName: name, RootDevice: s.rootDevice, RootInode: s.rootInode, ScratchDevice: dev, ScratchInode: ino, ArtifactBytes: 9, MetadataBytes: s.options.MaxMetadataBytesPerLease, CreatedUnix: time.Now().Unix()}); err != nil {
		t.Fatal(errors.Join(err, parent.Close()))
	}
	if err = parent.Close(); err != nil {
		t.Fatal(err)
	}
	s.options.MaxRetainedArtifactBytes = 8
	unlock, err := s.lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	usage, usageErr := s.usageLocked()
	unlockErr := unlock()
	if !errors.Is(usageErr, ErrSnapshotLeaseLimit) || unlockErr != nil {
		t.Fatalf("interrupted stage usage = (%+v, %v), unlock %v", usage, usageErr, unlockErr)
	}
	if err = s.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(filepath.Join(root, name)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned interrupted scratch remains: %v", err)
	}
}

func TestSnapshotLeaseReconcileLeavesUnknownScratchUntouched(t *testing.T) {
	root := filepath.Join(privateTempDir(t), "snapshot-lease-unknown-scratch")
	s := testLeaseStore(t, root, newLeaseTestAuthority())
	parent, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	name, err := createInspectionScratch(parent)
	if err != nil {
		t.Fatal(errors.Join(err, parent.Close()))
	}
	if err = parent.WriteFile(filepath.Join(name, "unowned.data"), []byte("keep"), 0600); err != nil {
		t.Fatal(errors.Join(err, parent.Close()))
	}
	if err = parent.Close(); err != nil {
		t.Fatal(err)
	}
	if err = s.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if raw, readErr := os.ReadFile(filepath.Join(root, name, "unowned.data")); readErr != nil || string(raw) != "keep" {
		t.Fatalf("unknown scratch was removed or changed: bytes=%q err=%v", raw, readErr)
	}
}

func TestSnapshotLeaseInterruptedStageChildProcess(t *testing.T) {
	if os.Getenv("SERENITY_TEST_CRASH_STAGE") == "1" {
		root, snapshot, digest := os.Getenv("SERENITY_TEST_LEASE_ROOT"), os.Getenv("SERENITY_TEST_SNAPSHOT"), os.Getenv("SERENITY_TEST_MANIFEST_SHA")
		s := testLeaseStoreOptions(t, SnapshotLeaseStoreOptions{LeaseRoot: root, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 2 << 20, MaxLeases: 8}, newLeaseTestAuthority())
		opts := InspectionOptions{ExpectedManifestSHA256: digest, MaxDeclaredBytes: 1 << 30, MaxAccounts: 100, MaxBrains: 100, stageAfterIntent: func() { os.Exit(77) }}
		_, err := s.Stage(context.Background(), snapshot, opts)
		t.Fatalf("crash hook did not terminate staging: %v", err)
	}
	_, snapshot, _ := freshPrivateSnapshot(t, 1)
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-stage-crash")
	_ = testLeaseStore(t, root, newLeaseTestAuthority())
	cmd := exec.Command(os.Args[0], "-test.run=^TestSnapshotLeaseInterruptedStageChildProcess$")
	cmd.Env = append(os.Environ(), "SERENITY_TEST_CRASH_STAGE=1", "SERENITY_TEST_LEASE_ROOT="+root, "SERENITY_TEST_SNAPSHOT="+snapshot, "SERENITY_TEST_MANIFEST_SHA="+opts.ExpectedManifestSHA256)
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 77 {
		t.Fatalf("child stage exit = %v, want deliberate crash 77", err)
	}
	store := testLeaseStoreOptions(t, SnapshotLeaseStoreOptions{LeaseRoot: root, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 2 << 20, MaxLeases: 1}, newLeaseTestAuthority())
	lock, err := store.lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	usage, usageErr := store.usageLocked()
	unlockErr := lock()
	if usageErr != nil || usage.leases != 1 || unlockErr != nil {
		t.Fatalf("reopened store did not count crashed reservation: usage=%+v err=%v unlock=%v", usage, usageErr, unlockErr)
	}
	if _, err = store.Stage(context.Background(), snapshot, opts); !errors.Is(err, ErrSnapshotLeaseLimit) {
		t.Fatalf("second Stage passed while interrupted intent consumes lease budget: %v", err)
	}
	if err = store.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotLeaseInterruptedCandidateChildProcess(t *testing.T) {
	if os.Getenv("SERENITY_TEST_CRASH_CANDIDATE") == "1" {
		root, snapshot, digest := os.Getenv("SERENITY_TEST_LEASE_ROOT"), os.Getenv("SERENITY_TEST_SNAPSHOT"), os.Getenv("SERENITY_TEST_MANIFEST_SHA")
		s := testLeaseStoreOptions(t, SnapshotLeaseStoreOptions{LeaseRoot: root, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 2 << 20, MaxLeases: 8}, newLeaseTestAuthority())
		lease, err := s.Stage(context.Background(), snapshot, InspectionOptions{ExpectedManifestSHA256: digest, MaxDeclaredBytes: 1 << 30, MaxAccounts: 100, MaxBrains: 100})
		if err != nil {
			t.Fatal(err)
		}
		lease.candidateAfterIntent = func() { os.Exit(78) }
		account := lease.Inspection().Accounts[0].ID
		_, err = lease.Candidate(context.Background(), account)
		t.Fatalf("candidate crash hook did not terminate: %v", err)
	}
	_, snapshot, _ := freshPrivateSnapshot(t, 1)
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-candidate-crash")
	_ = testLeaseStore(t, root, newLeaseTestAuthority())
	cmd := exec.Command(os.Args[0], "-test.run=^TestSnapshotLeaseInterruptedCandidateChildProcess$")
	cmd.Env = append(os.Environ(), "SERENITY_TEST_CRASH_CANDIDATE=1", "SERENITY_TEST_LEASE_ROOT="+root, "SERENITY_TEST_SNAPSHOT="+snapshot, "SERENITY_TEST_MANIFEST_SHA="+opts.ExpectedManifestSHA256)
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 78 {
		t.Fatalf("candidate child exit = %v, want deliberate crash 78", err)
	}
	store := testLeaseStoreOptions(t, SnapshotLeaseStoreOptions{LeaseRoot: root, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 2 << 20, MaxLeases: 8}, newLeaseTestAuthority())
	lock, err := store.lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	usage, usageErr := store.usageLocked()
	unlockErr := lock()
	if usageErr != nil || usage.leases != 1 || usage.artifacts == 0 || unlockErr != nil {
		t.Fatalf("candidate crash reservation not counted: usage=%+v err=%v unlock=%v", usage, usageErr, unlockErr)
	}
	if err = store.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".serenity-snapshot-inspect-") {
			t.Fatalf("owned crashed Candidate projection remains after Reconcile: %s", entry.Name())
		}
	}
}

func TestSnapshotLeaseInterruptedRestoreScratchChildProcess(t *testing.T) {
	if os.Getenv("SERENITY_TEST_CRASH_RESTORE") == "1" {
		root, snapshot, digest := os.Getenv("SERENITY_TEST_LEASE_ROOT"), os.Getenv("SERENITY_TEST_SNAPSHOT"), os.Getenv("SERENITY_TEST_MANIFEST_SHA")
		s := testLeaseStoreOptions(t, SnapshotLeaseStoreOptions{LeaseRoot: root, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 2 << 20, MaxLeases: 8}, newLeaseTestAuthority())
		lease, err := s.Stage(context.Background(), snapshot, InspectionOptions{ExpectedManifestSHA256: digest, MaxDeclaredBytes: 1 << 30, MaxAccounts: 100, MaxBrains: 100})
		if err != nil {
			t.Fatal(err)
		}
		lease.restoreAfterIntent = func() { os.Exit(79) }
		_ = RestoreVerified(context.Background(), lease, filepath.Join(privateTempDir(t), "crash-restore-destination"))
		t.Fatal("restore crash hook did not terminate")
	}
	_, snapshot, _ := freshPrivateSnapshot(t, 1)
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-restore-crash")
	_ = testLeaseStore(t, root, newLeaseTestAuthority())
	cmd := exec.Command(os.Args[0], "-test.run=^TestSnapshotLeaseInterruptedRestoreScratchChildProcess$")
	cmd.Env = append(os.Environ(), "SERENITY_TEST_CRASH_RESTORE=1", "SERENITY_TEST_LEASE_ROOT="+root, "SERENITY_TEST_SNAPSHOT="+snapshot, "SERENITY_TEST_MANIFEST_SHA="+opts.ExpectedManifestSHA256)
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 79 {
		t.Fatalf("restore child exit = %v, want deliberate crash 79", err)
	}
	store := testLeaseStoreOptions(t, SnapshotLeaseStoreOptions{LeaseRoot: root, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 2 << 20, MaxLeases: 8}, newLeaseTestAuthority())
	lock, err := store.lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	usage, usageErr := store.usageLocked()
	unlockErr := lock()
	entries, readErr := os.ReadDir(root)
	var retained leaseDiskRecord
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			retained, readErr = readLeaseRecord(filepath.Join(root, entry.Name()), 1<<20)
			break
		}
	}
	if usageErr != nil || readErr != nil || unlockErr != nil || retained.ArtifactBytes == 0 || usage.artifacts != retained.ArtifactBytes*2 {
		t.Fatalf("reopened store did not account crashed restore raw copy: usage=%+v err=%v unlock=%v", usage, usageErr, unlockErr)
	}
	bounded := store.options
	bounded.MaxArtifactBytesPerLease = retained.ArtifactBytes
	bounded.MaxRetainedArtifactBytes = retained.ArtifactBytes*2 - 1
	limited := testLeaseStoreOptions(t, bounded, newLeaseTestAuthority())
	if _, err = limited.Stage(context.Background(), snapshot, InspectionOptions{ExpectedManifestSHA256: opts.ExpectedManifestSHA256, MaxDeclaredBytes: 1 << 30, MaxAccounts: 100, MaxBrains: 100}); !errors.Is(err, ErrSnapshotLeaseLimit) {
		t.Fatalf("stage bypassed crashed restore aggregate reservation: %v", err)
	}
	if err = store.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	entries, err = os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".serenity-snapshot-inspect-") {
			t.Fatalf("owned restore scratch survived reconciliation: %s", entry.Name())
		}
	}
}

func TestSnapshotLeaseCloseCancellationPreservesBorrowedBytes(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 0)
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-close-cancel")
	lease, err := testLeaseStore(t, root, newLeaseTestAuthority()).Stage(context.Background(), snapshot, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err = lease.beginBorrow(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err = lease.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close during active borrower = %v, want deadline", err)
	}
	if _, err = os.Stat(lease.path); err != nil {
		t.Fatalf("borrowed lease removed after canceled close: %v", err)
	}
	if err = lease.endBorrow(); err != nil {
		t.Fatal(err)
	}
	if err = lease.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(lease.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lease not cleaned after borrower ended: %v", err)
	}
}
