package recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/backup"
)

func TestPinOwnerBackupReleaseReceiptSurvivesRepeatedReconcileAndReopen(t *testing.T) {
	ctx := context.Background()
	root := pinOwnerTestRoot(t)
	owner, leases, snapshot, digest, inspection := pinOwnerTestPair(t, root)
	reservation, err := owner.ReservePinPlan(ctx, "release-receipt-reopen")
	if err != nil {
		t.Fatal(err)
	}
	lease, err := leases.Stage(ctx, snapshot, inspection)
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := lease.Pin(ctx, reservation.PlanRef(), reservation.ReservationVersion(), digest)
	if err != nil {
		t.Fatal(err)
	}
	if err = lease.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if pinned.ManifestSHA256() != digest {
		t.Fatal("unexpected pinned snapshot digest")
	}
	leaseID := lease.LeaseID()
	current, err := owner.ReservePinPlan(ctx, "release-receipt-reopen")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.MarkAbandonedBeforeEffects(ctx, current, PinAbandonOperatorRequested); err != nil {
		t.Fatal(err)
	}
	if err = leases.Reconcile(ctx); err != nil {
		t.Fatalf("first producer reconcile: %v", err)
	}
	receiptDir := filepath.Join(owner.options.BackupLeaseRoot, ".releases", leaseID)
	receiptPath := filepath.Join(receiptDir, "release.json")
	receiptBefore, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatalf("read retained terminal receipt: %v", err)
	}
	receiptInfoBefore, err := os.Lstat(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(filepath.Join(owner.options.BackupLeaseRoot, leaseID)); !os.IsNotExist(err) {
		t.Fatalf("released live lease still exists: %v", err)
	}
	historyBefore, err := owner.ListPinAttempts(ctx)
	if err != nil || len(historyBefore) != 1 || historyBefore[0].PlanRef != reservation.PlanRef() || historyBefore[0].LeaseID != leaseID || historyBefore[0].ManifestSHA256 != digest || historyBefore[0].ReservationVersion != reservation.ReservationVersion() || historyBefore[0].State != backup.PinAttemptCommitted {
		t.Fatalf("terminal owner attempt was not retained: history=%+v err=%v", historyBefore, err)
	}
	ownerHeadBefore := pinOwnerReleaseTestHead(t, ctx, owner, reservation.PlanRef())
	if ownerHeadBefore.State != string(PinOwnerReleased) || ownerHeadBefore.AttemptState != pinOwnerAttemptCommitted || ownerHeadBefore.LeaseID != leaseID || ownerHeadBefore.ManifestSHA256 != digest {
		t.Fatalf("owner terminal history is not RELEASED with committed attempt: %+v", ownerHeadBefore)
	}
	if err = leases.Reconcile(ctx); err != nil {
		t.Fatalf("second producer reconcile: %v", err)
	}
	bo := backup.SnapshotLeaseStoreOptions{LeaseRoot: owner.options.BackupLeaseRoot, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 8 << 20, MaxLeases: 8}
	reopened, reopenedLeases, err := pinOwnerOpenVerifiedPair(ctx, owner.options, bo)
	if err != nil {
		t.Fatalf("reopen verified owner/producer pair: %v", err)
	}
	if err = reopenedLeases.Reconcile(ctx); err != nil {
		t.Fatalf("reopened producer reconcile: %v", err)
	}
	assertRetainedTerminal := func(where string) {
		t.Helper()
		receiptAfter, readErr := os.ReadFile(receiptPath)
		receiptInfoAfter, statErr := os.Lstat(receiptPath)
		historyAfter, historyErr := reopened.ListPinAttempts(ctx)
		if readErr != nil || statErr != nil || !os.SameFile(receiptInfoBefore, receiptInfoAfter) || !reflect.DeepEqual(receiptBefore, receiptAfter) || historyErr != nil || !reflect.DeepEqual(historyBefore, historyAfter) {
			t.Fatalf("%s changed terminal receipt/owner history: read=%v stat=%v history=%v", where, readErr, statErr, historyErr)
		}
		if _, statErr = os.Lstat(filepath.Join(owner.options.BackupLeaseRoot, leaseID)); !os.IsNotExist(statErr) {
			t.Fatalf("%s recreated released live lease: %v", where, statErr)
		}
		ownerHeadAfter := pinOwnerReleaseTestHead(t, ctx, reopened, reservation.PlanRef())
		if !reflect.DeepEqual(ownerHeadBefore, ownerHeadAfter) {
			t.Fatalf("%s changed retained RELEASED owner history: before=%+v after=%+v", where, ownerHeadBefore, ownerHeadAfter)
		}
	}
	assertRetainedTerminal("repeated/reopened reconciliation")
	if _, err = reopened.ReservePinPlan(ctx, "post-release-stage"); err != nil {
		t.Fatalf("reserve after retained terminal pair: %v", err)
	}
	additional, err := reopenedLeases.Stage(ctx, snapshot, inspection)
	if err != nil {
		t.Fatalf("stage after retained terminal pair: %v", err)
	}
	if err = additional.Close(ctx); err != nil {
		t.Fatal(err)
	}
	assertRetainedTerminal("additional stage")
}

func pinOwnerReleaseTestHead(t *testing.T, ctx context.Context, owner *SnapshotPinOwner, planRef string) pinOwnerRecord {
	t.Helper()
	head, err := pinOwnerWithLock(owner, ctx, func(root *os.File) (pinOwnerRecord, error) {
		plans, err := pinOwnerLoadAll(ctx, root, owner)
		if err != nil {
			return pinOwnerRecord{}, err
		}
		for _, plan := range plans {
			if plan.head.PlanRef == planRef {
				return plan.head, nil
			}
		}
		return pinOwnerRecord{}, ErrPinOwnerInvalid
	})
	if err != nil {
		t.Fatalf("read exact pin owner terminal head: %v", err)
	}
	return head
}

func TestPinOwnerReleasePeakBudgetRefusalPrecedesOwnerReleaseBegin(t *testing.T) {
	ctx := context.Background()
	root := pinOwnerTestRoot(t)
	owner, leases, snapshot, digest, inspection := pinOwnerTestPair(t, root)
	reservation, err := owner.ReservePinPlan(ctx, "release-capacity-before-owner")
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
	leaseID := lease.LeaseID()
	if err = lease.Close(ctx); err != nil {
		t.Fatal(err)
	}
	reservation, err = owner.ReservePinPlan(ctx, "release-capacity-before-owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.MarkAbandonedBeforeEffects(ctx, reservation, PinAbandonOperatorRequested); err != nil {
		t.Fatal(err)
	}
	headBefore := pinOwnerReleaseTestHead(t, ctx, owner, reservation.PlanRef())
	livePath := filepath.Join(owner.options.BackupLeaseRoot, leaseID)
	recordBytes, err := os.ReadFile(filepath.Join(livePath, "lease.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifestInfo, err := os.Stat(filepath.Join(livePath, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	perLease, tombstoneBytes, err := pinOwnerReleaseWireSizes(recordBytes, manifestInfo.Size(), leaseID)
	if err != nil {
		t.Fatal(err)
	}
	limit := 2*perLease + tombstoneBytes - 1
	if limit < 2*perLease || limit <= perLease {
		t.Fatalf("capacity fixture does not isolate receipt bytes: lease=%d tombstone=%d aggregate=%d", perLease, tombstoneBytes, limit)
	}
	backupOptions := backup.SnapshotLeaseStoreOptions{LeaseRoot: owner.options.BackupLeaseRoot, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: perLease, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: limit, MaxLeases: 8}
	reopened, reopenedLeases, err := pinOwnerOpenVerifiedPair(ctx, owner.options, backupOptions)
	if err != nil {
		t.Fatal(err)
	}
	if reopenedHead := pinOwnerReleaseTestHead(t, ctx, reopened, reservation.PlanRef()); !reflect.DeepEqual(headBefore, reopenedHead) {
		t.Fatalf("reopened owner changed before producer reconciliation: before=%+v after=%+v", headBefore, reopenedHead)
	}
	if err = reopenedLeases.Reconcile(ctx); !errors.Is(err, backup.ErrSnapshotLeaseLimit) {
		t.Fatalf("budget gap did not refuse before release: %v", err)
	}
	recordAfter, readErr := os.ReadFile(filepath.Join(livePath, "lease.json"))
	if readErr != nil || !bytes.Equal(recordBytes, recordAfter) {
		t.Fatalf("producer record changed despite preflight refusal: read=%v", readErr)
	}
	if _, err = os.Lstat(filepath.Join(owner.options.BackupLeaseRoot, ".releases", leaseID)); !os.IsNotExist(err) {
		t.Fatalf("release marker published despite budget refusal: %v", err)
	}
	headAfter := pinOwnerReleaseTestHead(t, ctx, reopened, reservation.PlanRef())
	if !reflect.DeepEqual(headBefore, headAfter) || headAfter.State != string(PinOwnerAbandonedBeforeEffects) || headAfter.AttemptState != pinOwnerAttemptCommitted || headAfter.PinID != pin.ID() {
		t.Fatalf("owner history advanced despite budget refusal: before=%+v after=%+v", headBefore, headAfter)
	}
	attempts, err := reopened.ListPinAttempts(ctx)
	if err != nil || len(attempts) != 1 || attempts[0].LeaseID != leaseID || attempts[0].State != backup.PinAttemptCommitted {
		t.Fatalf("owner committed attempt changed despite budget refusal: attempts=%+v err=%v", attempts, err)
	}
}

func pinOwnerReleaseWireSizes(recordBytes []byte, manifestBytes int64, leaseID string) (int64, int64, error) {
	var record map[string]json.RawMessage
	if err := json.Unmarshal(recordBytes, &record); err != nil {
		return 0, 0, err
	}
	set := func(m map[string]json.RawMessage, key string, value any) error {
		raw, err := json.Marshal(value)
		if err == nil {
			m[key] = raw
		}
		return err
	}
	if err := set(record, "checksum", strings.Repeat("a", 64)); err != nil {
		return 0, 0, err
	}
	if err := set(record, "state", "RELEASING"); err != nil {
		return 0, 0, err
	}
	if err := set(record, "disposition", 2); err != nil {
		return 0, 0, err
	}
	if err := set(record, "record_version", ^uint64(0)); err != nil {
		return 0, 0, err
	}
	var candidateRaw []byte
	var candidateBytes int64
	for i := 0; i < 8; i++ {
		candidateRaw, _ = json.Marshal(record)
		candidateBytes = manifestBytes + int64(len(candidateRaw))
		encodedBytes, _ := json.Marshal(candidateBytes)
		if bytes.Equal(record["metadata_bytes"], encodedBytes) {
			break
		}
		record["metadata_bytes"] = encodedBytes
	}
	if candidateBytes <= manifestBytes || int64(len(candidateRaw)) == 0 {
		return 0, 0, errors.New("failed to compute candidate lease wire length")
	}
	var tombstone map[string]json.RawMessage
	fields := map[string]string{"id": "id", "pin_id": "pin_id", "plan_ref": "plan_ref", "manifest_sha256": "manifest_sha256", "reservation_version": "reservation_version", "attempt_version": "attempt_version"}
	tombstone = make(map[string]json.RawMessage, 13)
	for out, in := range fields {
		tombstone[out] = append([]byte(nil), record[in]...)
	}
	if err := set(tombstone, "version", 1); err != nil {
		return 0, 0, err
	}
	if err := set(tombstone, "checksum", strings.Repeat("a", 64)); err != nil {
		return 0, 0, err
	}
	if err := set(tombstone, "disposition", 2); err != nil {
		return 0, 0, err
	}
	if err := set(tombstone, "record_version", ^uint64(0)); err != nil {
		return 0, 0, err
	}
	if err := set(tombstone, "state", "RELEASED"); err != nil {
		return 0, 0, err
	}
	if err := set(tombstone, "metadata_bytes", 0); err != nil {
		return 0, 0, err
	}
	var tombRaw []byte
	for i := 0; i < 8; i++ {
		tombRaw, _ = json.Marshal(tombstone)
		encodedBytes, _ := json.Marshal(int64(len(tombRaw)))
		if bytes.Equal(tombstone["metadata_bytes"], encodedBytes) {
			break
		}
		tombstone["metadata_bytes"] = encodedBytes
	}
	encodedLeaseID, _ := json.Marshal(leaseID)
	if int64(len(tombRaw)) <= 0 || !bytes.Equal(record["id"], encodedLeaseID) {
		return 0, 0, errors.New("failed to compute release tombstone wire length")
	}
	return candidateBytes, int64(len(tombRaw)), nil
}
