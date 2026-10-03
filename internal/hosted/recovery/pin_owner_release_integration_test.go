package recovery

import (
	"context"
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
}
