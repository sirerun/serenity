package recovery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/backup"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/privatefs"
	"github.com/sirerun/serenity/internal/hosted/store"
)

type pinOwnerTestJournal struct{ watermark contracts.DeletionWatermark }

func (j pinOwnerTestJournal) AppendDeletion(context.Context, contracts.DeletionEntry) (contracts.DeletionEntry, error) {
	return contracts.DeletionEntry{}, errors.New("unexpected deletion append")
}
func (j pinOwnerTestJournal) ReadThrough(context.Context, contracts.DeletionWatermark) (contracts.DeletionRead, error) {
	return contracts.DeletionRead{To: j.watermark}, nil
}
func (j pinOwnerTestJournal) Seal(context.Context, int64) (contracts.DeletionWatermark, error) {
	return j.watermark, nil
}

func pinOwnerTestRoot(t *testing.T) string {
	t.Helper()
	base, explicit := os.LookupEnv("SERENITY_RECOVERY_TEST_TMPDIR")
	if runtime.GOOS == "darwin" && !explicit {
		t.Skip("Darwin owner tests require explicit APFS fixture")
	}
	if !explicit {
		base = os.TempDir()
	}
	if err := privatefs.ValidateDirectory(context.Background(), base); err != nil {
		t.Fatalf("validate explicit test fixture: %v", err)
	}
	root, err := os.MkdirTemp(base, "snapshot-pin-owner-test-")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err = privatefs.ValidateDirectory(context.Background(), root); err != nil {
		t.Fatalf("validate created fixture: %v", err)
	}
	created, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if now, e := os.Lstat(root); e == nil && os.SameFile(created, now) && privatefs.ValidateDirectory(context.Background(), root) == nil {
			_ = os.RemoveAll(root)
		}
	})
	return root
}

func pinOwnerTestSnapshot(t *testing.T, root string) (string, string, backup.InspectionOptions) {
	t.Helper()
	ctx := context.Background()
	dataDir := filepath.Join(root, "data")
	if err := os.Mkdir(dataDir, 0700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dataDir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.CreateAccount(ctx, "pin-owner@example.invalid"); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(root, "snapshot")
	watermark := contracts.DeletionWatermark{Generation: 1, SequenceID: 3, EntryHash: strings.Repeat("a", 64)}
	if err = backup.Create(ctx, dataDir, snapshot, "pin-owner-test-build", pinOwnerTestJournal{watermark: watermark}); err != nil {
		t.Fatalf("create fixture snapshot: %v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(snapshot, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(manifest)
	scratch := filepath.Join(root, "inspection-scratch")
	if err = os.Mkdir(scratch, 0700); err != nil {
		t.Fatal(err)
	}
	options := backup.InspectionOptions{ScratchRoot: scratch, ExpectedManifestSHA256: hex.EncodeToString(digest[:]), MaxDeclaredBytes: 1 << 30, MaxAccounts: 100, MaxBrains: 100}
	return snapshot, options.ExpectedManifestSHA256, options
}

func pinOwnerTestPair(t *testing.T, root string) (*SnapshotPinOwner, *backup.SnapshotLeaseStore, string, string, backup.InspectionOptions) {
	t.Helper()
	ownerRoot := filepath.Join(root, "owner")
	leaseRoot := filepath.Join(root, "leases")
	options := backup.SnapshotLeaseStoreOptions{LeaseRoot: leaseRoot, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 8 << 20, MaxLeases: 8}
	ownerOptions := SnapshotPinOwnerOptions{OwnerRoot: ownerRoot, BackupLeaseRoot: leaseRoot, MaxPlans: 8, MaxOwnerMetadataBytes: 16 << 20}
	owner, leases, err := pinOwnerOpenVerifiedPair(context.Background(), ownerOptions, options)
	if err != nil {
		t.Fatalf("open real owner/backup pair: %v", err)
	}
	snapshot, digest, inspection := pinOwnerTestSnapshot(t, root)
	return owner, leases, snapshot, digest, inspection
}

func TestPinOwnerDirectOpenStaysInactiveAndPairGateUsesRealConstructor(t *testing.T) {
	root := pinOwnerTestRoot(t)
	leaseRoot := filepath.Join(root, "leases")
	bo := backup.SnapshotLeaseStoreOptions{LeaseRoot: leaseRoot, MaxArtifactBytesPerLease: 1 << 20, MaxMetadataBytesPerLease: 1 << 16, MaxRestoreScratchBytes: 1 << 20, MaxRetainedArtifactBytes: 1 << 22, MaxRetainedMetadataBytes: 1 << 20, MaxLeases: 2}
	identity, err := backup.PreflightSnapshotStoreIdentity(context.Background(), bo)
	if err != nil {
		t.Fatal(err)
	}
	oo := SnapshotPinOwnerOptions{OwnerRoot: filepath.Join(root, "owner"), BackupLeaseRoot: leaseRoot, MaxPlans: 1, MaxOwnerMetadataBytes: 2 << 20}
	owner, err := OpenSnapshotPinOwner(context.Background(), oo, identity)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.ReservePinPlan(context.Background(), "inactive"); !errors.Is(err, ErrPinOwnerUnavailable) {
		t.Fatalf("direct owner unexpectedly active: %v", err)
	}
	if _, _, err = pinOwnerOpenVerifiedPair(context.Background(), SnapshotPinOwnerOptions{OwnerRoot: oo.OwnerRoot, BackupLeaseRoot: filepath.Join(root, "wrong"), MaxPlans: 1, MaxOwnerMetadataBytes: 2 << 20}, bo); !errors.Is(err, ErrPinOwnerInvalid) {
		t.Fatalf("mismatched roots passed pair gate: %v", err)
	}
	if !owner.activated { /* the direct owner is intentionally not activated */
	} else {
		t.Fatal("direct owner activated without the pair constructor")
	}
}

func TestPinOwnerRealBackupPairStagePinCloseFindAndAbandonRelease(t *testing.T) {
	root := pinOwnerTestRoot(t)
	owner, leases, snapshot, digest, inspection := pinOwnerTestPair(t, root)
	ctx := context.Background()
	reservation, err := owner.ReservePinPlan(ctx, "op-stage-pin-release")
	if err != nil {
		t.Fatal(err)
	}
	if !reservation.Valid() || reservation.RecordVersion() != 1 || reservation.State() != PinOwnerReserved {
		t.Fatalf("initial reservation = %+v", reservation)
	}
	lease, err := leases.Stage(ctx, snapshot, inspection)
	if err != nil {
		t.Fatal(err)
	}
	pin, err := lease.Pin(ctx, reservation.PlanRef(), reservation.ReservationVersion(), digest)
	if err != nil {
		t.Fatalf("producer pin: %v", err)
	}
	if err = lease.Close(ctx); err != nil {
		t.Fatalf("close pinned handle: %v", err)
	}
	found, err := leases.FindPinned(ctx, reservation.PlanRef(), digest)
	if err != nil {
		t.Fatal(err)
	}
	if found.ID() != pin.ID() || found.PlanRef() != pin.PlanRef() || found.ManifestSHA256() != pin.ManifestSHA256() || found.ReservationVersion() != pin.ReservationVersion() {
		t.Fatalf("found pin differs: %+v / %+v", found, pin)
	}
	current, err := owner.ReservePinPlan(ctx, "op-stage-pin-release")
	if err != nil {
		t.Fatal(err)
	}
	if current.State() != PinOwnerPinned || current.RecordVersion() != 3 {
		t.Fatalf("retry did not return current CAS head: %+v", current)
	}
	abandoned, err := owner.MarkAbandonedBeforeEffects(ctx, current, PinAbandonOperatorRequested)
	if err != nil {
		t.Fatal(err)
	}
	if abandoned.State() != PinOwnerAbandonedBeforeEffects || abandoned.RecordVersion() != 4 {
		t.Fatalf("abandoned = %+v", abandoned)
	}
	decision, err := owner.ReconcilePin(ctx, pin)
	if err != nil || decision.Action != backup.PinRelease {
		t.Fatalf("begin exact release: %+v %v", decision, err)
	}
	wrongAuth := decision.Authorization
	wrongAuth.RecordVersion++
	if err = owner.CompletePinRelease(ctx, wrongAuth); !errors.Is(err, ErrPinOwnerConflict) {
		t.Fatalf("wrong release authorization accepted: %v", err)
	}
	if err = leases.Reconcile(ctx); err != nil {
		t.Fatalf("producer release and owner completion: %v", err)
	}
	terminal, err := owner.ReservePinPlan(ctx, "op-stage-pin-release")
	if err != nil {
		t.Fatal(err)
	}
	if terminal.State() != PinOwnerReleased || terminal.RecordVersion() != 6 {
		t.Fatalf("terminal reservation = %+v", terminal)
	}
	if _, err = leases.FindPinned(ctx, reservation.PlanRef(), digest); !errors.Is(err, backup.ErrSnapshotLeaseNotFound) {
		t.Fatalf("released pin remains discoverable: %v", err)
	}
	// Replay the complete release chain after reopening through the actual
	// opaque-identity and four-argument backup-constructor gate.
	bo := backup.SnapshotLeaseStoreOptions{LeaseRoot: owner.options.BackupLeaseRoot, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 8 << 20, MaxLeases: 8}
	reopened, _, err := pinOwnerOpenVerifiedPair(ctx, owner.options, bo)
	if err != nil {
		t.Fatalf("reopen released owner with real backup constructor: %v", err)
	}
	terminal, err = reopened.ReservePinPlan(ctx, "op-stage-pin-release")
	if err != nil || terminal.State() != PinOwnerReleased || terminal.RecordVersion() != 6 {
		t.Fatalf("reopened terminal reservation = %+v, %v", terminal, err)
	}
}

func TestPinOwnerLockReplacementAndHistoryTamperFailClosed(t *testing.T) {
	root := pinOwnerTestRoot(t)
	owner, _, _, _, _ := pinOwnerTestPair(t, root)
	ctx := context.Background()
	if _, err := owner.ReservePinPlan(ctx, "op-lock-replacement"); err != nil {
		t.Fatal(err)
	}
	lockPath := filepath.Join(owner.options.OwnerRoot, pinOwnerLockName)
	// Retain the original inode so the filesystem cannot immediately reuse it.
	// This fixture exercises replacement of the pinned device/inode identity.
	oldInfo, err := os.Stat(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(lockPath, filepath.Join(root, "retained-original-lock")); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	newInfo, err := os.Stat(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(oldInfo, newInfo) {
		t.Fatal("fixture did not replace lock identity")
	}
	if _, err = owner.ReservePinPlan(ctx, "op-must-not-publish"); !errors.Is(err, ErrPinOwnerUnavailable) {
		t.Fatalf("replaced lock accepted: %v", err)
	}
}

func TestPinOwnerBadChecksumBlocksAllOwnerOperations(t *testing.T) {
	root := pinOwnerTestRoot(t)
	owner, _, _, _, _ := pinOwnerTestPair(t, root)
	ctx := context.Background()
	reservation, err := owner.ReservePinPlan(ctx, "op-checksum-tamper")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(owner.options.OwnerRoot, pinOwnerReservationsName, reservation.PlanRef(), pinOwnerRecordName(1))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 4 {
		t.Fatal("short record")
	}
	raw[len(raw)-3] ^= 1
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.ReservePinPlan(ctx, "op-no-silent-repair"); !errors.Is(err, ErrPinOwnerCorrupt) {
		t.Fatalf("tampered history accepted: %v", err)
	}
}

func TestPinOwnerCanceledContextDoesNotAppend(t *testing.T) {
	root := pinOwnerTestRoot(t)
	owner, _, _, _, _ := pinOwnerTestPair(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := owner.ReservePinPlan(ctx, "op-canceled"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled reservation = %v", err)
	}
	if got, err := owner.ReservePinPlan(context.Background(), "op-canceled"); err != nil || got.RecordVersion() != 1 {
		t.Fatalf("canceled context left durable mutation: %+v %v", got, err)
	}
}

func TestPinOwnerCancellationTombstoneDoesNotTouchFreshAttempt(t *testing.T) {
	root := pinOwnerTestRoot(t)
	owner, leases, snapshot, digest, inspection := pinOwnerTestPair(t, root)
	ctx := context.Background()
	reserved, err := owner.ReservePinPlan(ctx, "op-cancel-retry")
	if err != nil {
		t.Fatal(err)
	}
	first, err := leases.Stage(ctx, snapshot, inspection)
	if err != nil {
		t.Fatal(err)
	}
	n, err := owner.BeginPinAttempt(ctx, reserved.PlanRef(), reserved.ReservationVersion(), first.LeaseID(), digest)
	if err != nil {
		t.Fatal(err)
	}
	if n.AttemptVersion != 1 || n.State != backup.PinAttemptPending {
		t.Fatalf("attempt N = %+v", n)
	}
	if err = leases.CancelPin(ctx, n); err != nil {
		t.Fatalf("cancel N with producer proof: %v", err)
	}
	second, err := leases.Stage(ctx, snapshot, inspection)
	if err != nil {
		t.Fatal(err)
	}
	n1, err := owner.BeginPinAttempt(ctx, reserved.PlanRef(), reserved.ReservationVersion(), second.LeaseID(), digest)
	if err != nil {
		t.Fatal(err)
	}
	if n1.AttemptVersion != 2 || n1.LeaseID == n.LeaseID {
		t.Fatalf("fresh attempt N+1 = %+v; N=%+v", n1, n)
	}
	if err = leases.CancelPin(ctx, n); err != nil {
		t.Fatalf("delayed exact Cancel(N): %v", err)
	}
	got, err := owner.FindPinAttempt(ctx, reserved.PlanRef(), digest)
	if err != nil {
		t.Fatal(err)
	}
	if got.AttemptVersion != 2 || got.LeaseID != n1.LeaseID || got.State != backup.PinAttemptPending {
		t.Fatalf("delayed N changed N+1: got %+v want %+v", got, n1)
	}
	if err = leases.CancelPin(ctx, n1); err != nil {
		t.Fatalf("cancel N+1 cleanup: %v", err)
	}
}

func TestPinOwnerCancelRequiresConsumedProducerAbsenceProof(t *testing.T) {
	root := pinOwnerTestRoot(t)
	owner, leases, snapshot, digest, inspection := pinOwnerTestPair(t, root)
	ctx := context.Background()
	reserved, err := owner.ReservePinPlan(ctx, "op-cancel-proof-required")
	if err != nil {
		t.Fatal(err)
	}
	lease, err := leases.Stage(ctx, snapshot, inspection)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := owner.BeginPinAttempt(ctx, reserved.PlanRef(), reserved.ReservationVersion(), lease.LeaseID(), digest)
	if err != nil {
		t.Fatal(err)
	}
	before, err := owner.ReservePinPlan(ctx, "op-cancel-proof-required")
	if err != nil {
		t.Fatal(err)
	}
	if err = owner.CancelPinAttempt(ctx, attempt, backup.VerifiedPinAbsence{}); !errors.Is(err, ErrPinOwnerConflict) {
		t.Fatalf("zero/unconsumed absence proof accepted: %v", err)
	}
	after, err := owner.ReservePinPlan(ctx, "op-cancel-proof-required")
	if err != nil || after != before || after.State() != PinOwnerPinPending {
		t.Fatalf("failed proof changed owner state: before=%+v after=%+v err=%v", before, after, err)
	}
	current, err := owner.FindPinAttempt(ctx, reserved.PlanRef(), digest)
	if err != nil || current != attempt {
		t.Fatalf("failed proof changed attempt: got %+v want %+v err=%v", current, attempt, err)
	}
	if err = leases.CancelPin(ctx, attempt); err != nil {
		t.Fatalf("producer-verified cancellation: %v", err)
	}
}

func TestPinOwnerRejectsMalformedAndUnorderedHistory(t *testing.T) {
	root := pinOwnerTestRoot(t)
	owner, _, _, _, _ := pinOwnerTestPair(t, root)
	res, err := owner.ReservePinPlan(context.Background(), "op-corrupt")
	if err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(owner.options.OwnerRoot, pinOwnerReservationsName, res.PlanRef())
	entries, err := os.ReadDir(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("history records = %d", len(entries))
	}
	if err = os.Rename(filepath.Join(planPath, entries[0].Name()), filepath.Join(planPath, "00000000000000000002.json")); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.ReservePinPlan(context.Background(), "op-other"); !errors.Is(err, ErrPinOwnerCorrupt) {
		t.Fatalf("history gap was accepted: %v", err)
	}
}

func TestPinOwnerBoundedDirectoryScannerUsesFreshSortedDescriptions(t *testing.T) {
	root := pinOwnerTestRoot(t)
	owner, _, _, _, _ := pinOwnerTestPair(t, root)
	fd, err := pinOwnerOpenDirectory(owner.options.OwnerRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := fd.Close(); err != nil {
			t.Errorf("close owner root: %v", err)
		}
	}()
	first, err := pinOwnerReadDirBounded(context.Background(), fd, 3)
	if err != nil {
		t.Fatal(err)
	}
	second, err := pinOwnerReadDirBounded(context.Background(), fd, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != len(second) {
		t.Fatalf("repeated scan changed size: %d != %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Name() != second[i].Name() {
			t.Fatalf("repeated scan order differs at %d: %q != %q", i, first[i].Name(), second[i].Name())
		}
		if i > 0 && first[i-1].Name() > first[i].Name() {
			t.Fatal("directory scanner failed to sort")
		}
	}
	for n := 0; n < 4; n++ {
		if err = pinOwnerMkdirAt(fd, strings.Repeat("x", n+1), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = pinOwnerReadDirBounded(context.Background(), fd, 3); !errors.Is(err, ErrPinOwnerLimit) {
		t.Fatalf("over-limit directory scan = %v", err)
	}
}

func TestPinOwnerFutureRecordVersionUnavailablePreservesBytes(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func([]byte) []byte
		want       error
		wantNotErr error
	}{
		{
			name: "future-version-with-extension",
			mutate: func(raw []byte) []byte {
				return bytes.Replace(raw, []byte(`"version":1,`), []byte(`"version":2,"future_field":{"enabled":true},`), 1)
			},
			want:       ErrPinOwnerUnavailable,
			wantNotErr: ErrPinOwnerCorrupt,
		},
		{
			name: "max-uint64-future-version",
			mutate: func(raw []byte) []byte {
				return bytes.Replace(raw, []byte(`"version":1,`), []byte(`"version":18446744073709551615,`), 1)
			},
			want:       ErrPinOwnerUnavailable,
			wantNotErr: ErrPinOwnerCorrupt,
		},
		{
			name: "overflowing-version",
			mutate: func(raw []byte) []byte {
				return bytes.Replace(raw, []byte(`"version":1,`), []byte(`"version":18446744073709551616,`), 1)
			},
			want: ErrPinOwnerCorrupt,
		},
		{
			name: "oversized-record",
			mutate: func(raw []byte) []byte {
				return append(raw, bytes.Repeat([]byte{' '}, pinOwnerMaxRecordBytes+1-len(raw))...)
			},
			want: ErrPinOwnerCorrupt,
		},
		{
			name: "duplicate-version",
			mutate: func(raw []byte) []byte {
				return bytes.Replace(raw, []byte(`"version":1,`), []byte(`"version":1,"version":2,`), 1)
			},
			want:       ErrPinOwnerCorrupt,
			wantNotErr: ErrPinOwnerUnavailable,
		},
		{
			name: "future-version-with-trailing-garbage",
			mutate: func(raw []byte) []byte {
				future := bytes.Replace(raw, []byte(`"version":1,`), []byte(`"version":2,"future_field":true,`), 1)
				return append(future, 'x')
			},
			want:       ErrPinOwnerCorrupt,
			wantNotErr: ErrPinOwnerUnavailable,
		},
		{
			name: "zero-version",
			mutate: func(raw []byte) []byte {
				return bytes.Replace(raw, []byte(`"version":1,`), []byte(`"version":0,`), 1)
			},
			want: ErrPinOwnerCorrupt,
		},
		{
			name: "null-version",
			mutate: func(raw []byte) []byte {
				return bytes.Replace(raw, []byte(`"version":1,`), []byte(`"version":null,`), 1)
			},
			want: ErrPinOwnerCorrupt,
		},
		{
			name: "fractional-version",
			mutate: func(raw []byte) []byte {
				return bytes.Replace(raw, []byte(`"version":1,`), []byte(`"version":1.5,`), 1)
			},
			want: ErrPinOwnerCorrupt,
		},
		{
			name: "negative-version",
			mutate: func(raw []byte) []byte {
				return bytes.Replace(raw, []byte(`"version":1,`), []byte(`"version":-1,`), 1)
			},
			want: ErrPinOwnerCorrupt,
		},
		{
			name: "string-version",
			mutate: func(raw []byte) []byte {
				return bytes.Replace(raw, []byte(`"version":1,`), []byte(`"version":"2",`), 1)
			},
			want: ErrPinOwnerCorrupt,
		},
		{
			name: "missing-version",
			mutate: func(raw []byte) []byte {
				return bytes.Replace(raw, []byte(`"version":1,`), nil, 1)
			},
			want: ErrPinOwnerCorrupt,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := pinOwnerTestRoot(t)
			owner, _, _, _, _ := pinOwnerTestPair(t, root)
			reservation, err := owner.ReservePinPlan(context.Background(), "op-future-version")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(owner.options.OwnerRoot, pinOwnerReservationsName, reservation.PlanRef(), pinOwnerRecordName(1))
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			mutated := tc.mutate(original)
			if bytes.Equal(mutated, original) {
				t.Fatal("test mutation did not change the fixture record")
			}
			if tc.name == "oversized-record" {
				beforeDecode := append([]byte(nil), mutated...)
				_, _, decodeErr := pinOwnerDecodeRecord(mutated)
				if !errors.Is(decodeErr, tc.want) || !bytes.Equal(mutated, beforeDecode) {
					t.Fatalf("oversized direct decode = %v; want %v and unchanged bytes", decodeErr, tc.want)
				}
				return
			}
			if err = os.WriteFile(path, mutated, 0600); err != nil {
				t.Fatal(err)
			}
			dir, err := os.Open(filepath.Dir(path))
			if err != nil {
				t.Fatal(err)
			}
			err = errors.Join(dir.Sync(), dir.Close())
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = owner.ReservePinPlan(context.Background(), "op-future-version")
			if !errors.Is(err, tc.want) || errors.Is(err, tc.wantNotErr) {
				t.Fatalf("decode result = %v; want %v and not %v", err, tc.want, tc.wantNotErr)
			}
			listed, listErr := owner.ListPinAttempts(context.Background())
			if !errors.Is(listErr, tc.want) || errors.Is(listErr, tc.wantNotErr) {
				t.Fatalf("ListPinAttempts decode result = %v; want %v and not %v", listErr, tc.want, tc.wantNotErr)
			}
			if len(listed) != 0 {
				t.Fatalf("ListPinAttempts returned partial authority on error: %+v", listed)
			}
			_, reopenErr := OpenSnapshotPinOwner(context.Background(), owner.options, owner.expected)
			if !errors.Is(reopenErr, tc.want) || errors.Is(reopenErr, tc.wantNotErr) {
				t.Fatalf("reopen decode result = %v; want %v and not %v", reopenErr, tc.want, tc.wantNotErr)
			}
			after, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(after, before) {
				t.Fatalf("unsupported/malformed record changed: readErr=%v", readErr)
			}
		})
	}
}

func TestPinOwnerFutureSuperblockUnavailablePreservesBytes(t *testing.T) {
	cases := []struct {
		name   string
		mutate func([]byte) []byte
		want   error
	}{
		{
			name: "future-version-extension",
			mutate: func(raw []byte) []byte {
				return bytes.Replace(raw, []byte(`"version":1,`), []byte(`"version":2,"future_field":{"ready":true},`), 1)
			},
			want: ErrPinOwnerUnavailable,
		},
		{
			name: "max-uint64-version",
			mutate: func(raw []byte) []byte {
				return bytes.Replace(raw, []byte(`"version":1,`), []byte(`"version":18446744073709551615,`), 1)
			},
			want: ErrPinOwnerUnavailable,
		},
		{
			name: "version-overflow",
			mutate: func(raw []byte) []byte {
				return bytes.Replace(raw, []byte(`"version":1,`), []byte(`"version":18446744073709551616,`), 1)
			},
			want: ErrPinOwnerCorrupt,
		},
		{
			name: "duplicate-version",
			mutate: func(raw []byte) []byte {
				return bytes.Replace(raw, []byte(`"version":1,`), []byte(`"version":1,"version":2,`), 1)
			},
			want: ErrPinOwnerCorrupt,
		},
		{
			name: "malformed-null-version",
			mutate: func(raw []byte) []byte {
				return bytes.Replace(raw, []byte(`"version":1,`), []byte(`"version":null,`), 1)
			},
			want: ErrPinOwnerCorrupt,
		},
		{
			name: "missing-version",
			mutate: func(raw []byte) []byte {
				return bytes.Replace(raw, []byte(`"version":1,`), nil, 1)
			},
			want: ErrPinOwnerCorrupt,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := pinOwnerTestRoot(t)
			owner, _, _, _, _ := pinOwnerTestPair(t, root)
			path := filepath.Join(owner.options.OwnerRoot, pinOwnerSuperName)
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			mutated := tc.mutate(original)
			if bytes.Equal(original, mutated) {
				t.Fatal("superblock test mutation did not change bytes")
			}
			if err = os.WriteFile(path, mutated, 0600); err != nil {
				t.Fatal(err)
			}
			dir, err := os.Open(owner.options.OwnerRoot)
			if err != nil {
				t.Fatal(err)
			}
			if err = errors.Join(dir.Sync(), dir.Close()); err != nil {
				t.Fatal(err)
			}
			beforeInfo, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			rootEntries := pinOwnerTestRootNames(t, owner.options.OwnerRoot)
			_, openErr := OpenSnapshotPinOwner(context.Background(), owner.options, owner.expected)
			if !errors.Is(openErr, tc.want) {
				t.Fatalf("OpenSnapshotPinOwner = %v; want %v", openErr, tc.want)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, mutated) {
				t.Fatalf("superblock bytes changed: read err=%v", err)
			}
			afterInfo, err := os.Stat(path)
			if err != nil || !os.SameFile(beforeInfo, afterInfo) {
				t.Fatalf("superblock inode changed: stat err=%v", err)
			}
			if gotEntries := pinOwnerTestRootNames(t, owner.options.OwnerRoot); strings.Join(gotEntries, "\x00") != strings.Join(rootEntries, "\x00") {
				t.Fatalf("owner root entries changed: got %v want %v", gotEntries, rootEntries)
			}
		})
	}
	t.Run("oversized-superblock-decode", func(t *testing.T) {
		root := pinOwnerTestRoot(t)
		owner, _, _, _, _ := pinOwnerTestPair(t, root)
		raw, err := os.ReadFile(filepath.Join(owner.options.OwnerRoot, pinOwnerSuperName))
		if err != nil {
			t.Fatal(err)
		}
		oversized := append(raw, bytes.Repeat([]byte{' '}, pinOwnerMaxSuperBytes+1-len(raw))...)
		if _, err = pinOwnerDecodeSuper(oversized); !errors.Is(err, ErrPinOwnerCorrupt) {
			t.Fatalf("oversized direct superblock decode = %v", err)
		}
	})
}

func TestPinOwnerActiveOperationsRefuseChangedSuperblock(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, raw []byte) []byte
		want   error
	}{
		{
			name: "future-extension",
			mutate: func(t *testing.T, raw []byte) []byte {
				t.Helper()
				return bytes.Replace(raw, []byte(`"version":1,`), []byte(`"version":2,"future_field":true,`), 1)
			},
			want: ErrPinOwnerUnavailable,
		},
		{
			name: "bad-checksum",
			mutate: func(t *testing.T, raw []byte) []byte {
				t.Helper()
				changed := append([]byte(nil), raw...)
				changed[len(changed)-3] ^= 1
				return changed
			},
			want: ErrPinOwnerCorrupt,
		},
		{
			name: "mismatched-store-id",
			mutate: func(t *testing.T, raw []byte) []byte {
				t.Helper()
				block, err := pinOwnerDecodeSuper(raw)
				if err != nil {
					t.Fatal(err)
				}
				originalStoreID := block.StoreID
				block.StoreID = strings.Repeat("a", 32)
				if block.StoreID == originalStoreID {
					block.StoreID = strings.Repeat("b", 32)
				}
				encoded, err := pinOwnerEncodeSuper(block)
				if err != nil {
					t.Fatal(err)
				}
				return encoded
			},
			want: ErrPinOwnerUnavailable,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := pinOwnerTestRoot(t)
			owner, leases, snapshot, digest, inspection := pinOwnerTestPair(t, root)
			ctx := context.Background()
			reservation, err := owner.ReservePinPlan(ctx, "op-active-superblock-check")
			if err != nil {
				t.Fatal(err)
			}
			lease, err := leases.Stage(ctx, snapshot, inspection)
			if err != nil {
				t.Fatal(err)
			}
			pin, err := lease.Pin(ctx, reservation.PlanRef(), reservation.ReservationVersion(), digest)
			if err != nil {
				t.Fatal(err)
			}
			superPath := filepath.Join(owner.options.OwnerRoot, pinOwnerSuperName)
			superRaw, err := os.ReadFile(superPath)
			if err != nil {
				t.Fatal(err)
			}
			mutated := tc.mutate(t, superRaw)
			if bytes.Equal(superRaw, mutated) {
				t.Fatal("superblock mutation did not change bytes")
			}
			if err = os.WriteFile(superPath, mutated, 0600); err != nil {
				t.Fatal(err)
			}
			rootDir, err := os.Open(owner.options.OwnerRoot)
			if err != nil {
				t.Fatal(err)
			}
			if err = errors.Join(rootDir.Sync(), rootDir.Close()); err != nil {
				t.Fatal(err)
			}
			planDir := filepath.Join(owner.options.OwnerRoot, pinOwnerReservationsName, reservation.PlanRef())
			recordNames := pinOwnerTestRootNames(t, planDir)
			recordBytes := make(map[string][]byte, len(recordNames))
			recordInfos := make(map[string]os.FileInfo, len(recordNames))
			for _, name := range recordNames {
				path := filepath.Join(planDir, name)
				recordBytes[name], err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				recordInfos[name], err = os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			superInfo, err := os.Stat(superPath)
			if err != nil {
				t.Fatal(err)
			}
			ownerEntries := pinOwnerTestRootNames(t, owner.options.OwnerRoot)
			if _, err = owner.ReservePinPlan(ctx, "op-must-not-append-after-superblock-change"); !errors.Is(err, tc.want) {
				t.Fatalf("active reserve after superblock change = %v; want %v", err, tc.want)
			}
			listed, err := owner.ListPinAttempts(ctx)
			if !errors.Is(err, tc.want) || len(listed) != 0 {
				t.Fatalf("active list after superblock change = %d refs, %v; want no refs and %v", len(listed), err, tc.want)
			}
			decision, err := owner.ReconcilePin(ctx, pin)
			if !errors.Is(err, tc.want) || decision.Action != 0 {
				t.Fatalf("active reconcile after superblock change = %+v, %v; want no decision and %v", decision, err, tc.want)
			}
			superAfter, err := os.ReadFile(superPath)
			if err != nil || !bytes.Equal(superAfter, mutated) {
				t.Fatalf("superblock changed after refusal: read err=%v", err)
			}
			superAfterInfo, err := os.Stat(superPath)
			if err != nil || !os.SameFile(superInfo, superAfterInfo) {
				t.Fatalf("superblock inode changed: stat err=%v", err)
			}
			if got := pinOwnerTestRootNames(t, owner.options.OwnerRoot); strings.Join(got, "\x00") != strings.Join(ownerEntries, "\x00") {
				t.Fatalf("owner root entries changed: got %v want %v", got, ownerEntries)
			}
			if got := pinOwnerTestRootNames(t, planDir); strings.Join(got, "\x00") != strings.Join(recordNames, "\x00") {
				t.Fatalf("owner history entries changed: got %v want %v", got, recordNames)
			}
			for _, name := range recordNames {
				path := filepath.Join(planDir, name)
				got, readErr := os.ReadFile(path)
				info, statErr := os.Stat(path)
				if readErr != nil || statErr != nil || !bytes.Equal(got, recordBytes[name]) || !os.SameFile(recordInfos[name], info) {
					t.Fatalf("owner history record %s changed: read=%v stat=%v", name, readErr, statErr)
				}
			}
		})
	}
}

func pinOwnerTestRootNames(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name()
	}
	return names
}

func TestPinOwnerOpenDoesNotRebindIncompleteOrUnknownRoots(t *testing.T) {
	t.Run("missing-superblock-with-retained-future-history", func(t *testing.T) {
		root := pinOwnerTestRoot(t)
		owner, _, _, _, _ := pinOwnerTestPair(t, root)
		reservation, err := owner.ReservePinPlan(context.Background(), "op-retained-future-history")
		if err != nil {
			t.Fatal(err)
		}
		recordPath := filepath.Join(owner.options.OwnerRoot, pinOwnerReservationsName, reservation.PlanRef(), pinOwnerRecordName(1))
		raw, err := os.ReadFile(recordPath)
		if err != nil {
			t.Fatal(err)
		}
		future := bytes.Replace(raw, []byte(`"version":1,`), []byte(`"version":2,"future_field":true,`), 1)
		if bytes.Equal(raw, future) {
			t.Fatal("future version fixture was unchanged")
		}
		if err = os.WriteFile(recordPath, future, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.Remove(filepath.Join(owner.options.OwnerRoot, pinOwnerSuperName)); err != nil {
			t.Fatal(err)
		}
		ownerRootBefore := pinOwnerTestRootNames(t, owner.options.OwnerRoot)
		lockPath := filepath.Join(owner.options.OwnerRoot, pinOwnerLockName)
		lockBefore, err := os.Stat(lockPath)
		if err != nil {
			t.Fatal(err)
		}
		recordBefore, err := os.Stat(recordPath)
		if err != nil {
			t.Fatal(err)
		}
		_, openErr := OpenSnapshotPinOwner(context.Background(), owner.options, owner.expected)
		if !errors.Is(openErr, ErrPinOwnerUnavailable) {
			t.Fatalf("open with missing superblock and retained future history = %v", openErr)
		}
		got, err := os.ReadFile(recordPath)
		if err != nil || !bytes.Equal(got, future) {
			t.Fatalf("future record changed: read err=%v", err)
		}
		if _, err = os.Lstat(filepath.Join(owner.options.OwnerRoot, pinOwnerSuperName)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("superblock was recreated: %v", err)
		}
		lockAfter, err := os.Stat(lockPath)
		if err != nil || !os.SameFile(lockBefore, lockAfter) {
			t.Fatalf("owner lock was rebound: stat err=%v", err)
		}
		recordAfter, err := os.Stat(recordPath)
		if err != nil || !os.SameFile(recordBefore, recordAfter) {
			t.Fatalf("retained history was replaced: stat err=%v", err)
		}
		if gotNames := pinOwnerTestRootNames(t, owner.options.OwnerRoot); strings.Join(gotNames, "\x00") != strings.Join(ownerRootBefore, "\x00") {
			t.Fatalf("owner root entries changed: got %v want %v", gotNames, ownerRootBefore)
		}
	})

	t.Run("existing-superblock-with-missing-lock", func(t *testing.T) {
		root := pinOwnerTestRoot(t)
		owner, _, _, _, _ := pinOwnerTestPair(t, root)
		superPath := filepath.Join(owner.options.OwnerRoot, pinOwnerSuperName)
		superBefore, err := os.ReadFile(superPath)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.Remove(filepath.Join(owner.options.OwnerRoot, pinOwnerLockName)); err != nil {
			t.Fatal(err)
		}
		ownerRootBefore := pinOwnerTestRootNames(t, owner.options.OwnerRoot)
		_, openErr := OpenSnapshotPinOwner(context.Background(), owner.options, owner.expected)
		if !errors.Is(openErr, ErrPinOwnerUnavailable) {
			t.Fatalf("open with missing bound lock = %v", openErr)
		}
		if _, err = os.Lstat(filepath.Join(owner.options.OwnerRoot, pinOwnerLockName)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing owner lock was recreated: %v", err)
		}
		superAfter, err := os.ReadFile(superPath)
		if err != nil || !bytes.Equal(superAfter, superBefore) {
			t.Fatalf("superblock changed: read err=%v", err)
		}
		if gotNames := pinOwnerTestRootNames(t, owner.options.OwnerRoot); strings.Join(gotNames, "\x00") != strings.Join(ownerRootBefore, "\x00") {
			t.Fatalf("owner root entries changed: got %v want %v", gotNames, ownerRootBefore)
		}
	})

	t.Run("unknown-entry-before-bootstrap", func(t *testing.T) {
		root := pinOwnerTestRoot(t)
		leaseRoot := filepath.Join(root, "leases")
		backupOptions := backup.SnapshotLeaseStoreOptions{LeaseRoot: leaseRoot, MaxArtifactBytesPerLease: 1 << 20, MaxMetadataBytesPerLease: 1 << 16, MaxRestoreScratchBytes: 1 << 20, MaxRetainedArtifactBytes: 1 << 22, MaxRetainedMetadataBytes: 1 << 20, MaxLeases: 2}
		identity, err := backup.PreflightSnapshotStoreIdentity(context.Background(), backupOptions)
		if err != nil {
			t.Fatal(err)
		}
		ownerOptions := SnapshotPinOwnerOptions{OwnerRoot: filepath.Join(root, "owner"), BackupLeaseRoot: leaseRoot, MaxPlans: 1, MaxOwnerMetadataBytes: 2 << 20}
		if err = os.Mkdir(ownerOptions.OwnerRoot, 0700); err != nil {
			t.Fatal(err)
		}
		unknownPath := filepath.Join(ownerOptions.OwnerRoot, "operator-data")
		if err = os.WriteFile(unknownPath, []byte("preserve"), 0600); err != nil {
			t.Fatal(err)
		}
		unknownBefore, err := os.Stat(unknownPath)
		if err != nil {
			t.Fatal(err)
		}
		rootNamesBefore := pinOwnerTestRootNames(t, ownerOptions.OwnerRoot)
		if _, err = OpenSnapshotPinOwner(context.Background(), ownerOptions, identity); !errors.Is(err, ErrPinOwnerUnavailable) {
			t.Fatalf("open with unknown root entry = %v", err)
		}
		unknownAfter, err := os.Stat(unknownPath)
		if err != nil || !os.SameFile(unknownBefore, unknownAfter) {
			t.Fatalf("unknown root entry replaced: stat err=%v", err)
		}
		content, err := os.ReadFile(unknownPath)
		if err != nil || string(content) != "preserve" {
			t.Fatalf("unknown root entry changed: read err=%v content=%q", err, content)
		}
		if gotNames := pinOwnerTestRootNames(t, ownerOptions.OwnerRoot); strings.Join(gotNames, "\x00") != strings.Join(rootNamesBefore, "\x00") {
			t.Fatalf("owner root entries changed: got %v want %v", gotNames, rootNamesBefore)
		}
	})
}

func TestPinOwnerConcurrentFreshOpenAndInitialReservation(t *testing.T) {
	root := pinOwnerTestRoot(t)
	leaseRoot := filepath.Join(root, "leases")
	backupOptions := backup.SnapshotLeaseStoreOptions{LeaseRoot: leaseRoot, MaxArtifactBytesPerLease: 1 << 20, MaxMetadataBytesPerLease: 1 << 16, MaxRestoreScratchBytes: 1 << 20, MaxRetainedArtifactBytes: 1 << 22, MaxRetainedMetadataBytes: 1 << 20, MaxLeases: 4}
	identity, err := backup.PreflightSnapshotStoreIdentity(context.Background(), backupOptions)
	if err != nil {
		t.Fatal(err)
	}
	ownerOptions := SnapshotPinOwnerOptions{OwnerRoot: filepath.Join(root, "owner"), BackupLeaseRoot: leaseRoot, MaxPlans: 4, MaxOwnerMetadataBytes: 2 << 20}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	type openResult struct {
		owner *SnapshotPinOwner
		err   error
	}
	start := make(chan struct{})
	results := make(chan openResult, 2)
	for range 2 {
		go func() {
			<-start
			owner, openErr := OpenSnapshotPinOwner(ctx, ownerOptions, identity)
			results <- openResult{owner: owner, err: openErr}
		}()
	}
	close(start)
	publicOwners := make([]*SnapshotPinOwner, 0, 2)
	for range 2 {
		result := <-results
		if result.err != nil && !errors.Is(result.err, ErrPinOwnerUnavailable) {
			t.Fatalf("concurrent fresh owner open: %v", result.err)
		}
		if result.owner != nil {
			publicOwners = append(publicOwners, result.owner)
		}
	}
	if len(publicOwners) > 1 && (publicOwners[0].storeID != publicOwners[1].storeID || publicOwners[0].ownerDevice != publicOwners[1].ownerDevice || publicOwners[0].ownerInode != publicOwners[1].ownerInode || publicOwners[0].ownerLockDev != publicOwners[1].ownerLockDev || publicOwners[0].ownerLockIno != publicOwners[1].ownerLockIno) {
		t.Fatal("concurrent opens did not bind the same owner root, lock, and store identity")
	}
	if _, err = OpenSnapshotPinOwner(ctx, ownerOptions, identity); err != nil {
		t.Fatalf("retry exact open after concurrent initialization: %v", err)
	}
	if len(publicOwners) > 0 {
		if _, err = publicOwners[0].ReservePinPlan(ctx, "op-inactive-owner-must-not-reserve"); !errors.Is(err, ErrPinOwnerUnavailable) {
			t.Fatalf("unpaired public owner became active: %v", err)
		}
	}
	superBefore, err := os.ReadFile(filepath.Join(ownerOptions.OwnerRoot, pinOwnerSuperName))
	if err != nil {
		t.Fatal(err)
	}
	type reserveResult struct {
		reservation PinOwnerReservation
		err         error
	}
	backupOptions.LeaseRoot = leaseRoot
	activeOwners := make([]*SnapshotPinOwner, 0, 2)
	for range 2 {
		activeOwner, _, openErr := pinOwnerOpenVerifiedPair(ctx, ownerOptions, backupOptions)
		if openErr != nil {
			t.Fatalf("open actual verified owner/backup pair: %v", openErr)
		}
		activeOwners = append(activeOwners, activeOwner)
	}
	if activeOwners[0].storeID != activeOwners[1].storeID || activeOwners[0].ownerLockDev != activeOwners[1].ownerLockDev || activeOwners[0].ownerLockIno != activeOwners[1].ownerLockIno {
		t.Fatal("verified factory pairs did not share one owner identity")
	}
	reserveStart := make(chan struct{})
	reservations := make(chan reserveResult, 2)
	for _, owner := range activeOwners {
		go func(owner *SnapshotPinOwner) {
			<-reserveStart
			reservation, reserveErr := owner.ReservePinPlan(ctx, "op-concurrent-bootstrap")
			reservations <- reserveResult{reservation: reservation, err: reserveErr}
		}(owner)
	}
	close(reserveStart)
	var first PinOwnerReservation
	for range 2 {
		result := <-reservations
		if result.err != nil {
			t.Fatalf("concurrent first reservation: %v", result.err)
		}
		if first.Valid() && result.reservation != first {
			t.Fatalf("concurrent first reservations forked: %+v vs %+v", first, result.reservation)
		}
		first = result.reservation
	}
	superAfter, err := os.ReadFile(filepath.Join(ownerOptions.OwnerRoot, pinOwnerSuperName))
	if err != nil || !bytes.Equal(superAfter, superBefore) {
		t.Fatalf("concurrent initialization rewrote superblock: %v", err)
	}
	ownerRoot, err := pinOwnerOpenDirectory(ownerOptions.OwnerRoot)
	if err != nil {
		t.Fatal(err)
	}
	plans, loadErr := pinOwnerLoadAll(ctx, ownerRoot, activeOwners[0])
	closeErr := ownerRoot.Close()
	if loadErr != nil || closeErr != nil || len(plans) != 1 || plans[0].head.RecordVersion != 1 {
		t.Fatalf("concurrent initialization history = %d plans, load=%v close=%v", len(plans), loadErr, closeErr)
	}
}

func pinOwnerTestTrimHistory(t *testing.T, owner *SnapshotPinOwner, planRef string, keepVersion uint64, changedPendingDigest string) {
	t.Helper()
	planDir := filepath.Join(owner.options.OwnerRoot, pinOwnerReservationsName, planRef)
	for version := uint64(3); version > keepVersion; version-- {
		if err := os.Remove(filepath.Join(planDir, pinOwnerRecordName(version))); err != nil {
			t.Fatalf("remove test history version %d: %v", version, err)
		}
	}
	if changedPendingDigest != "" {
		path := filepath.Join(planDir, pinOwnerRecordName(2))
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		record, _, err := pinOwnerDecodeRecord(raw)
		if err != nil {
			t.Fatal(err)
		}
		record.ManifestSHA256 = changedPendingDigest
		updated, _, err := pinOwnerEncodeRecord(record)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, updated, 0600); err != nil {
			t.Fatal(err)
		}
	}
	dir, err := os.Open(planDir)
	if err != nil {
		t.Fatal(err)
	}
	if err = errors.Join(dir.Sync(), dir.Close()); err != nil {
		t.Fatal(err)
	}
}

func TestPinOwnerReconcileRejectsUnboundActivePinTuples(t *testing.T) {
	cases := []struct {
		name                string
		keepVersion         uint64
		changePendingDigest bool
		wantState           PinOwnerState
		wantRecordVersion   uint64
	}{
		{name: "reserved-has-no-bound-pin", keepVersion: 1, wantState: PinOwnerReserved, wantRecordVersion: 1},
		{name: "pending-pin-id-not-yet-committed", keepVersion: 2, wantState: PinOwnerPinPending, wantRecordVersion: 2},
		{name: "pending-manifest-does-not-match", keepVersion: 2, changePendingDigest: true, wantState: PinOwnerPinPending, wantRecordVersion: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := pinOwnerTestRoot(t)
			owner, leases, snapshot, digest, inspection := pinOwnerTestPair(t, root)
			ctx := context.Background()
			reservation, err := owner.ReservePinPlan(ctx, "op-reconcile-active-mismatch")
			if err != nil {
				t.Fatal(err)
			}
			lease, err := leases.Stage(ctx, snapshot, inspection)
			if err != nil {
				t.Fatal(err)
			}
			stalePin, err := lease.Pin(ctx, reservation.PlanRef(), reservation.ReservationVersion(), digest)
			if err != nil {
				t.Fatal(err)
			}
			changedDigest := ""
			if tc.changePendingDigest {
				changedDigest = strings.Repeat("d", 64)
				if changedDigest == digest {
					changedDigest = strings.Repeat("c", 64)
				}
			}
			pinOwnerTestTrimHistory(t, owner, reservation.PlanRef(), tc.keepVersion, changedDigest)
			headPath := filepath.Join(owner.options.OwnerRoot, pinOwnerReservationsName, reservation.PlanRef(), pinOwnerRecordName(tc.wantRecordVersion))
			before, err := os.ReadFile(headPath)
			if err != nil {
				t.Fatal(err)
			}
			decision, err := owner.ReconcilePin(ctx, stalePin)
			if !errors.Is(err, ErrPinOwnerConflict) || decision.Action != 0 {
				t.Fatalf("ReconcilePin for %s = %+v, %v; want conflict without decision", tc.wantState, decision, err)
			}
			after, readErr := os.ReadFile(headPath)
			if readErr != nil || !bytes.Equal(after, before) {
				t.Fatalf("mismatched pin reconciliation mutated owner head: %v", readErr)
			}
			current, err := owner.ReservePinPlan(ctx, "op-reconcile-active-mismatch")
			if err != nil || current.State() != tc.wantState || current.RecordVersion() != tc.wantRecordVersion {
				t.Fatalf("owner changed after mismatch: %+v %v", current, err)
			}
		})
	}
}

func TestPinOwnerCanonicalGoldenHistoryVectors(t *testing.T) {
	const golden = `
{"version":1,"store_id":"11111111111111111111111111111111","plan_ref":"2222222222222222222222222222222222222222222222222222222222222222","reservation_version":1,"record_version":1,"previous_sha256":"0000000000000000000000000000000000000000000000000000000000000000","operation_id":"op-001","event":"RESERVE","state":"RESERVED","attempt_high_water":0,"attempt_version":0,"lease_id":"","manifest_sha256":"","attempt_state":"","pin_id":"","release_disposition":"","release_record_version":0,"reason_code":"","checksum_sha256":"0d6c13a6cba21bfabe6f84f3ad0ca7060f3843f21e02e7cb44e72b574712d4fa"}
{"version":1,"store_id":"11111111111111111111111111111111","plan_ref":"2222222222222222222222222222222222222222222222222222222222222222","reservation_version":1,"record_version":2,"previous_sha256":"19a19db4826344cb6a09702ce1ce311fb2ec9aa28df21e542ad5582144eaaad7","operation_id":"op-001","event":"PIN_BEGIN","state":"PIN_PENDING","attempt_high_water":1,"attempt_version":1,"lease_id":"4444444444444444444444444444444444444444444444444444444444444444","manifest_sha256":"5555555555555555555555555555555555555555555555555555555555555555","attempt_state":"PENDING","pin_id":"","release_disposition":"","release_record_version":0,"reason_code":"","checksum_sha256":"04675bb0ecb2b826486e72fcf05b99b1132a832886d0157a97df191d545f134d"}
{"version":1,"store_id":"11111111111111111111111111111111","plan_ref":"2222222222222222222222222222222222222222222222222222222222222222","reservation_version":1,"record_version":3,"previous_sha256":"ef5e2a1b21eba195ee1964794a1f3120fe78953f4bec540a590d79803b831116","operation_id":"op-001","event":"PIN_CANCEL","state":"RESERVED","attempt_high_water":1,"attempt_version":1,"lease_id":"4444444444444444444444444444444444444444444444444444444444444444","manifest_sha256":"5555555555555555555555555555555555555555555555555555555555555555","attempt_state":"CANCELED","pin_id":"","release_disposition":"","release_record_version":0,"reason_code":"","checksum_sha256":"9fbd909715a0f2e4058b43477903aba6b6e723be776751ffda633037fe11a251"}
{"version":1,"store_id":"11111111111111111111111111111111","plan_ref":"2222222222222222222222222222222222222222222222222222222222222222","reservation_version":1,"record_version":4,"previous_sha256":"c5d64c664c9e294cbba3be2af08fd2fc578f8b9f483c1a402354c486306d1a8f","operation_id":"op-001","event":"PIN_BEGIN","state":"PIN_PENDING","attempt_high_water":2,"attempt_version":2,"lease_id":"7777777777777777777777777777777777777777777777777777777777777777","manifest_sha256":"5555555555555555555555555555555555555555555555555555555555555555","attempt_state":"PENDING","pin_id":"","release_disposition":"","release_record_version":0,"reason_code":"","checksum_sha256":"38bf57736b47bc106820bd6a784ab6eba8d3727e01037d7509f8df7fafb5fd3e"}
{"version":1,"store_id":"11111111111111111111111111111111","plan_ref":"2222222222222222222222222222222222222222222222222222222222222222","reservation_version":1,"record_version":5,"previous_sha256":"4ab7781d15579d68ad8c69baafaad3840a96d6b1b9dc6d19714f9f2d4fbdf5a3","operation_id":"op-001","event":"PIN_COMMIT","state":"PINNED","attempt_high_water":2,"attempt_version":2,"lease_id":"7777777777777777777777777777777777777777777777777777777777777777","manifest_sha256":"5555555555555555555555555555555555555555555555555555555555555555","attempt_state":"COMMITTED","pin_id":"8888888888888888888888888888888888888888888888888888888888888888","release_disposition":"","release_record_version":0,"reason_code":"","checksum_sha256":"507d37dfd95ab8f39f20f8c33cbfc2cdfb97b81535b1967b370c2492f16831e6"}
{"version":1,"store_id":"11111111111111111111111111111111","plan_ref":"2222222222222222222222222222222222222222222222222222222222222222","reservation_version":1,"record_version":6,"previous_sha256":"0813afd97163c680013c40b573df46b1c85240207d5a42a514aa66326e4a19a3","operation_id":"op-001","event":"ABANDON","state":"ABANDONED_BEFORE_EFFECTS","attempt_high_water":2,"attempt_version":2,"lease_id":"7777777777777777777777777777777777777777777777777777777777777777","manifest_sha256":"5555555555555555555555555555555555555555555555555555555555555555","attempt_state":"COMMITTED","pin_id":"8888888888888888888888888888888888888888888888888888888888888888","release_disposition":"","release_record_version":0,"reason_code":"OPERATOR_ABORT_BEFORE_READY","checksum_sha256":"e9182706582b70d8a1b272a8b00e74dcf4559ed6e3f205c32fa1a7bfbe13428f"}
{"version":1,"store_id":"11111111111111111111111111111111","plan_ref":"2222222222222222222222222222222222222222222222222222222222222222","reservation_version":1,"record_version":7,"previous_sha256":"5341aaf36a79eea01aa9ed2aa1231302f5cbaa36982242fe4e97b7ceb8efaf48","operation_id":"op-001","event":"RELEASE_BEGIN","state":"RELEASING","attempt_high_water":2,"attempt_version":2,"lease_id":"7777777777777777777777777777777777777777777777777777777777777777","manifest_sha256":"5555555555555555555555555555555555555555555555555555555555555555","attempt_state":"COMMITTED","pin_id":"8888888888888888888888888888888888888888888888888888888888888888","release_disposition":"ABANDONED_BEFORE_EFFECTS","release_record_version":7,"reason_code":"","checksum_sha256":"c0abe790755bde24e288a7b7c861747d7313d05c3b457100b4556fd3dd29408e"}
{"version":1,"store_id":"11111111111111111111111111111111","plan_ref":"2222222222222222222222222222222222222222222222222222222222222222","reservation_version":1,"record_version":8,"previous_sha256":"094c14e0feec28a5d05a57450109abd3d246f3d8818dfa7596464785470fd0d8","operation_id":"op-001","event":"RELEASE_COMPLETE","state":"RELEASED","attempt_high_water":2,"attempt_version":2,"lease_id":"7777777777777777777777777777777777777777777777777777777777777777","manifest_sha256":"5555555555555555555555555555555555555555555555555555555555555555","attempt_state":"COMMITTED","pin_id":"8888888888888888888888888888888888888888888888888888888888888888","release_disposition":"ABANDONED_BEFORE_EFFECTS","release_record_version":7,"reason_code":"","checksum_sha256":"d37602def3518a529978d9d3b4e358b0b2dd95f7aa92fef30d4851524c5aca70"}
`
	lines := strings.Split(strings.TrimSpace(golden), "\n")
	if len(lines) != 8 {
		t.Fatalf("golden record count=%d", len(lines))
	}
	wants := []struct{ record, checksum string }{
		{record: "19a19db4826344cb6a09702ce1ce311fb2ec9aa28df21e542ad5582144eaaad7", checksum: "0d6c13a6cba21bfabe6f84f3ad0ca7060f3843f21e02e7cb44e72b574712d4fa"},
		{record: "ef5e2a1b21eba195ee1964794a1f3120fe78953f4bec540a590d79803b831116", checksum: "04675bb0ecb2b826486e72fcf05b99b1132a832886d0157a97df191d545f134d"},
		{record: "c5d64c664c9e294cbba3be2af08fd2fc578f8b9f483c1a402354c486306d1a8f", checksum: "9fbd909715a0f2e4058b43477903aba6b6e723be776751ffda633037fe11a251"},
		{record: "4ab7781d15579d68ad8c69baafaad3840a96d6b1b9dc6d19714f9f2d4fbdf5a3", checksum: "38bf57736b47bc106820bd6a784ab6eba8d3727e01037d7509f8df7fafb5fd3e"},
		{record: "0813afd97163c680013c40b573df46b1c85240207d5a42a514aa66326e4a19a3", checksum: "507d37dfd95ab8f39f20f8c33cbfc2cdfb97b81535b1967b370c2492f16831e6"},
		{record: "5341aaf36a79eea01aa9ed2aa1231302f5cbaa36982242fe4e97b7ceb8efaf48", checksum: "e9182706582b70d8a1b272a8b00e74dcf4559ed6e3f205c32fa1a7bfbe13428f"},
		{record: "094c14e0feec28a5d05a57450109abd3d246f3d8818dfa7596464785470fd0d8", checksum: "c0abe790755bde24e288a7b7c861747d7313d05c3b457100b4556fd3dd29408e"},
		{record: "9874a1678ad667206f8c9f526310e6bc115b2122e9a6f69068c1360e5c6c7997", checksum: "d37602def3518a529978d9d3b4e358b0b2dd95f7aa92fef30d4851524c5aca70"},
	}
	records := make([]pinOwnerRecord, 0, len(lines))
	previous := pinOwnerZeroDigest
	for i, line := range lines {
		raw := []byte(line)
		r, recordHash, err := pinOwnerDecodeRecord(raw)
		if err != nil {
			t.Fatalf("decode golden %d: %v", i+1, err)
		}
		reencoded, gotHash, err := pinOwnerEncodeRecord(r)
		if err != nil {
			t.Fatalf("encode golden %d: %v", i+1, err)
		}
		if string(reencoded) != line || gotHash != wants[i].record || r.ChecksumSHA256 != wants[i].checksum || recordHash != wants[i].record {
			t.Fatalf("golden %d changed: full=%s checksum=%s", i+1, gotHash, r.ChecksumSHA256)
		}
		if r.PreviousSHA256 != previous {
			t.Fatalf("golden %d predecessor=%s want %s", i+1, r.PreviousSHA256, previous)
		}
		previous = recordHash
		records = append(records, r)
	}
	if err := pinOwnerValidateHistory(context.Background(), records); err != nil {
		t.Fatalf("golden full history: %v", err)
	}
	super := pinOwnerSuperblock{Version: 1, StoreID: strings.Repeat("1", 32), OwnerRootDevice: 10, OwnerRootInode: 20, OwnerLockDevice: 10, OwnerLockInode: 21, BackupRootDevice: 10, BackupRootInode: 30, BackupLockDevice: 10, BackupLockInode: 31}
	wantSuper := `{"version":1,"store_id":"11111111111111111111111111111111","owner_root_device":10,"owner_root_inode":20,"owner_lock_device":10,"owner_lock_inode":21,"backup_root_device":10,"backup_root_inode":30,"backup_lock_device":10,"backup_lock_inode":31,"checksum_sha256":"de4868ebd763e062c2effd459f51d35dd0b5eb56d92af560a027aa3f143cddae"}`
	gotSuper, err := pinOwnerEncodeSuper(super)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotSuper) != wantSuper {
		t.Fatalf("superblock golden changed: %s", gotSuper)
	}
}

func TestPinOwnerReservationCapacityIsPreflightedBeforePlanDirectory(t *testing.T) {
	root := pinOwnerTestRoot(t)
	leaseRoot := filepath.Join(root, "leases")
	bo := backup.SnapshotLeaseStoreOptions{LeaseRoot: leaseRoot, MaxArtifactBytesPerLease: 1 << 20, MaxMetadataBytesPerLease: 1 << 16, MaxRestoreScratchBytes: 1 << 20, MaxRetainedArtifactBytes: 1 << 22, MaxRetainedMetadataBytes: 1 << 20, MaxLeases: 2}
	oo := SnapshotPinOwnerOptions{OwnerRoot: filepath.Join(root, "owner"), BackupLeaseRoot: leaseRoot, MaxPlans: 1, MaxOwnerMetadataBytes: pinOwnerMaxSuperBytes}
	owner, _, err := pinOwnerOpenVerifiedPair(context.Background(), oo, bo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.ReservePinPlan(context.Background(), "op-over-capacity"); !errors.Is(err, ErrPinOwnerLimit) {
		t.Fatalf("underbudget reservation = %v", err)
	}
	reservations, err := os.ReadDir(filepath.Join(oo.OwnerRoot, pinOwnerReservationsName))
	if err != nil {
		t.Fatal(err)
	}
	if len(reservations) != 0 {
		t.Fatalf("capacity refusal left reservation entries: %v", reservations)
	}
}

func TestPinOwnerChildCrashTarget(t *testing.T) {
	step := os.Getenv("SERENITY_PIN_OWNER_TEST_CRASH_AT")
	root := os.Getenv("SERENITY_PIN_OWNER_TEST_CRASH_ROOT")
	action := os.Getenv("SERENITY_PIN_OWNER_TEST_CRASH_ACTION")
	if step == "" || root == "" {
		return
	}
	pinOwnerTestCrashHook = func(point string) {
		if point == step {
			os.Exit(76)
		}
	}
	leaseRoot := filepath.Join(root, "leases")
	bo := backup.SnapshotLeaseStoreOptions{LeaseRoot: leaseRoot, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 8 << 20, MaxLeases: 8}
	oo := SnapshotPinOwnerOptions{OwnerRoot: filepath.Join(root, "owner"), BackupLeaseRoot: leaseRoot, MaxPlans: 8, MaxOwnerMetadataBytes: 16 << 20}
	owner, leases, err := pinOwnerOpenVerifiedPair(context.Background(), oo, bo)
	if err != nil {
		t.Fatalf("child open pair: %v", err)
	}
	reservation, err := owner.ReservePinPlan(context.Background(), "op-child-crash")
	if err != nil {
		t.Fatalf("child reserve before injected exit: %v", err)
	}
	ctx := context.Background()
	switch action {
	case "reserve":
	case "pin":
		snapshot := os.Getenv("SERENITY_PIN_OWNER_TEST_SNAPSHOT")
		digest := os.Getenv("SERENITY_PIN_OWNER_TEST_DIGEST")
		inspection := backup.InspectionOptions{ScratchRoot: filepath.Join(root, "inspection-scratch"), ExpectedManifestSHA256: digest, MaxDeclaredBytes: 1 << 30, MaxAccounts: 100, MaxBrains: 100}
		lease, e := leases.Stage(ctx, snapshot, inspection)
		if e != nil {
			t.Fatalf("child stage: %v", e)
		}
		if _, e = lease.Pin(ctx, reservation.PlanRef(), reservation.ReservationVersion(), digest); e != nil {
			t.Fatalf("child pin: %v", e)
		}
	case "cancel-proof":
		snapshot := os.Getenv("SERENITY_PIN_OWNER_TEST_SNAPSHOT")
		digest := os.Getenv("SERENITY_PIN_OWNER_TEST_DIGEST")
		inspection := backup.InspectionOptions{ScratchRoot: filepath.Join(root, "inspection-scratch"), ExpectedManifestSHA256: digest, MaxDeclaredBytes: 1 << 30, MaxAccounts: 100, MaxBrains: 100}
		lease, e := leases.Stage(ctx, snapshot, inspection)
		if e != nil {
			t.Fatalf("child stage: %v", e)
		}
		attempt, e := owner.BeginPinAttempt(ctx, reservation.PlanRef(), reservation.ReservationVersion(), lease.LeaseID(), digest)
		if e != nil {
			t.Fatalf("child begin: %v", e)
		}
		if e = leases.CancelPin(ctx, attempt); e != nil {
			t.Fatalf("child cancel: %v", e)
		}
	case "cancel-event":
		digest := os.Getenv("SERENITY_PIN_OWNER_TEST_DIGEST")
		attempt, e := owner.FindPinAttempt(ctx, reservation.PlanRef(), digest)
		if e != nil {
			t.Fatalf("child find pending attempt: %v", e)
		}
		if e = leases.CancelPin(ctx, attempt); e != nil {
			t.Fatalf("child append cancellation: %v", e)
		}
	case "abandon":
		_, e := owner.MarkAbandonedBeforeEffects(ctx, reservation, PinAbandonOperatorRequested)
		if e != nil {
			t.Fatalf("child abandon: %v", e)
		}
	case "release-begin":
		digest := os.Getenv("SERENITY_PIN_OWNER_TEST_DIGEST")
		pin, e := leases.FindPinned(ctx, reservation.PlanRef(), digest)
		if e != nil {
			t.Fatalf("child find pinned: %v", e)
		}
		if _, e = owner.ReconcilePin(ctx, pin); e != nil {
			t.Fatalf("child begin release: %v", e)
		}
	case "complete-release":
		if e := leases.Reconcile(ctx); e != nil {
			t.Fatalf("child complete release: %v", e)
		}
	default:
		t.Fatalf("unknown child action %q", action)
	}
	t.Fatalf("crash point %q was not reached", step)
}

func TestPinOwnerReservationCASCrashPrefixesFailClosedOrReplayExact(t *testing.T) {
	steps := []string{"before-temp-create", "after-temp-create", "after-partial-write", "after-complete-write", "after-file-fsync", "after-publish", "after-dir-sync"}
	for _, step := range steps {
		t.Run(step, func(t *testing.T) {
			root := pinOwnerTestRoot(t)
			_, _, _, _, _ = pinOwnerTestPair(t, root)
			cmd := exec.Command(os.Args[0], "-test.run=^TestPinOwnerChildCrashTarget$")
			cmd.Env = append(os.Environ(), "SERENITY_PIN_OWNER_TEST_CRASH_AT="+step, "SERENITY_PIN_OWNER_TEST_CRASH_ROOT="+root)
			err := cmd.Run()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 76 {
				t.Fatalf("child crash at %s = %v, want exit 76", step, err)
			}
			leaseRoot := filepath.Join(root, "leases")
			bo := backup.SnapshotLeaseStoreOptions{LeaseRoot: leaseRoot, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 8 << 20, MaxLeases: 8}
			oo := SnapshotPinOwnerOptions{OwnerRoot: filepath.Join(root, "owner"), BackupLeaseRoot: leaseRoot, MaxPlans: 8, MaxOwnerMetadataBytes: 16 << 20}
			reopened, _, openErr := pinOwnerOpenVerifiedPair(context.Background(), oo, bo)
			if step == "after-dir-sync" {
				if openErr != nil {
					t.Fatalf("reopen synced reserve: %v", openErr)
				}
				got, err := reopened.ReservePinPlan(context.Background(), "op-child-crash")
				if err != nil || got.RecordVersion() != 1 || got.State() != PinOwnerReserved {
					t.Fatalf("synced reserve replay = %+v %v", got, err)
				}
				return
			}
			if openErr == nil {
				got, err := reopened.ReservePinPlan(context.Background(), "op-child-crash")
				if err != nil || got.RecordVersion() != 1 || got.State() != PinOwnerReserved {
					t.Fatalf("published reserve replay = %+v %v", got, err)
				}
				return
			}
			if !errors.Is(openErr, ErrPinOwnerCorrupt) && !errors.Is(openErr, ErrPinOwnerUnavailable) {
				t.Fatalf("ambiguous crash prefix returned %v", openErr)
			}
			if _, err = os.Stat(oo.OwnerRoot); err != nil {
				t.Fatalf("owner root disappeared after refusal: %v", err)
			}
		})
	}
}

func TestPinOwnerLifecycleCrashPrefixesReplayExact(t *testing.T) {
	cases := []struct{ step, action string }{
		{"after-event:PIN_BEGIN", "pin"},
		{"after-event:PIN_COMMIT", "pin"},
		{"after-proof-consume", "cancel-proof"},
		{"after-event:PIN_CANCEL", "cancel-event"},
		{"after-event:ABANDON", "abandon"},
		{"after-event:RELEASE_BEGIN", "release-begin"},
		{"after-event:RELEASE_COMPLETE", "complete-release"},
	}
	for _, tc := range cases {
		t.Run(tc.action+"-"+tc.step, func(t *testing.T) {
			root := pinOwnerTestRoot(t)
			owner, leases, snapshot, digest, inspection := pinOwnerTestPair(t, root)
			ctx := context.Background()
			reservation, err := owner.ReservePinPlan(ctx, "op-child-crash")
			if err != nil {
				t.Fatal(err)
			}
			var pin backup.SnapshotPinRef
			var canceledAttempt backup.PinAttemptRef
			if tc.action == "cancel-event" {
				lease, e := leases.Stage(ctx, snapshot, inspection)
				if e != nil {
					t.Fatal(e)
				}
				canceledAttempt, e = owner.BeginPinAttempt(ctx, reservation.PlanRef(), reservation.ReservationVersion(), lease.LeaseID(), digest)
				if e != nil {
					t.Fatal(e)
				}
			}
			if tc.action == "abandon" || tc.action == "release-begin" || tc.action == "complete-release" {
				lease, e := leases.Stage(ctx, snapshot, inspection)
				if e != nil {
					t.Fatal(e)
				}
				pin, e = lease.Pin(ctx, reservation.PlanRef(), reservation.ReservationVersion(), digest)
				if e != nil {
					t.Fatal(e)
				}
				reservation, e = owner.ReservePinPlan(ctx, "op-child-crash")
				if e != nil {
					t.Fatal(e)
				}
				if tc.action == "release-begin" || tc.action == "complete-release" {
					reservation, e = owner.MarkAbandonedBeforeEffects(ctx, reservation, PinAbandonOperatorRequested)
					if e != nil {
						t.Fatal(e)
					}
				}
				if tc.action == "complete-release" {
					if _, e = owner.ReconcilePin(ctx, pin); e != nil {
						t.Fatal(e)
					}
				}
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestPinOwnerChildCrashTarget$")
			cmd.Env = append(os.Environ(), "SERENITY_PIN_OWNER_TEST_CRASH_AT="+tc.step, "SERENITY_PIN_OWNER_TEST_CRASH_ROOT="+root, "SERENITY_PIN_OWNER_TEST_CRASH_ACTION="+tc.action, "SERENITY_PIN_OWNER_TEST_SNAPSHOT="+snapshot, "SERENITY_PIN_OWNER_TEST_DIGEST="+digest)
			err = cmd.Run()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 76 {
				t.Fatalf("child crash at %s = %v, want exit 76", tc.step, err)
			}
			bo := backup.SnapshotLeaseStoreOptions{LeaseRoot: owner.options.BackupLeaseRoot, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 8 << 20, MaxLeases: 8}
			reopened, reopenedLeases, err := pinOwnerOpenVerifiedPair(ctx, owner.options, bo)
			if err != nil {
				t.Fatalf("reopen after %s: %v", tc.step, err)
			}
			switch tc.action {
			case "pin":
				got, e := reopenedLeases.ResumePin(ctx, reservation.PlanRef(), reservation.ReservationVersion(), digest)
				if e != nil {
					t.Fatalf("resume exact pin after %s: %v", tc.step, e)
				}
				found, e := reopenedLeases.FindPinned(ctx, reservation.PlanRef(), digest)
				if e != nil || found.ID() != got.ID() {
					t.Fatalf("replayed pin mismatch: %+v %+v %v", found, got, e)
				}
			case "cancel-proof":
				attempt, e := reopened.FindPinAttempt(ctx, reservation.PlanRef(), digest)
				if e != nil {
					t.Fatal(e)
				}
				if e = reopenedLeases.CancelPin(ctx, attempt); e != nil {
					t.Fatalf("retry exact cancellation after proof consumption: %v", e)
				}
				if _, e = reopened.FindPinAttempt(ctx, reservation.PlanRef(), digest); !errors.Is(e, ErrPinOwnerConflict) {
					t.Fatalf("canceled tuple not tombstoned: %v", e)
				}
			case "cancel-event":
				if e := reopenedLeases.CancelPin(ctx, canceledAttempt); e != nil {
					t.Fatalf("retry durable canceled tuple: %v", e)
				}
				if _, e := reopened.FindPinAttempt(ctx, reservation.PlanRef(), digest); !errors.Is(e, ErrPinOwnerConflict) {
					t.Fatalf("canceled tuple not tombstoned: %v", e)
				}
				beforeNPlusOne, e := reopened.ReservePinPlan(ctx, "op-child-crash")
				if e != nil || beforeNPlusOne.State() != PinOwnerReserved || beforeNPlusOne.RecordVersion() != 3 {
					t.Fatalf("canceled attempt replay = %+v %v", beforeNPlusOne, e)
				}
				nextLease, e := reopenedLeases.Stage(ctx, snapshot, inspection)
				if e != nil {
					t.Fatal(e)
				}
				nextPin, e := nextLease.Pin(ctx, beforeNPlusOne.PlanRef(), beforeNPlusOne.ReservationVersion(), digest)
				if e != nil {
					t.Fatalf("fresh N+1 pin: %v", e)
				}
				nextAttempt, e := reopened.FindPinAttempt(ctx, reservation.PlanRef(), digest)
				if e != nil || nextAttempt.AttemptVersion != 2 || nextAttempt.LeaseID == canceledAttempt.LeaseID || nextPin.ID() == "" {
					t.Fatalf("N+1 attempt = %+v, pin=%q, err=%v", nextAttempt, nextPin.ID(), e)
				}
				if e = reopenedLeases.CancelPin(ctx, canceledAttempt); e != nil {
					t.Fatalf("delayed Cancel(N) after N+1: %v", e)
				}
				stillNPlusOne, e := reopened.FindPinAttempt(ctx, reservation.PlanRef(), digest)
				if e != nil || stillNPlusOne != nextAttempt {
					t.Fatalf("delayed Cancel(N) altered N+1: got %+v want %+v err=%v", stillNPlusOne, nextAttempt, e)
				}
				finalReservation, e := reopened.ReservePinPlan(ctx, "op-child-crash")
				if e != nil || finalReservation.RecordVersion() != 5 || finalReservation.State() != PinOwnerPinned {
					t.Fatalf("N+1 history changed after delayed retry: %+v %v", finalReservation, e)
				}
			case "abandon":
				got, e := reopened.ReservePinPlan(ctx, "op-child-crash")
				if e != nil || got.State() != PinOwnerAbandonedBeforeEffects {
					t.Fatalf("abandon replay = %+v %v", got, e)
				}
			case "release-begin", "complete-release":
				if e := reopenedLeases.Reconcile(ctx); e != nil {
					t.Fatalf("resume exact release after %s: %v", tc.step, e)
				}
				got, e := reopened.ReservePinPlan(ctx, "op-child-crash")
				if e != nil || got.State() != PinOwnerReleased {
					t.Fatalf("release replay = %+v %v", got, e)
				}
			}
		})
	}
}
