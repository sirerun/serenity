package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSnapshotLeaseReconcileResumesExactPartialRelease(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 1)
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-partial-release")
	a := newLeaseTestAuthority()
	s := testLeaseStore(t, root, a)
	lease, err := s.Stage(context.Background(), snapshot, opts)
	if err != nil {
		t.Fatal(err)
	}
	plan := strings.Repeat("d", 64)
	pin, err := lease.Pin(context.Background(), plan, 1, opts.ExpectedManifestSHA256)
	if err != nil {
		t.Fatal(err)
	}
	record, err := readLeaseRecord(filepath.Join(root, lease.LeaseID()), s.options.MaxMetadataBytesPerLease)
	if err != nil {
		t.Fatal(err)
	}
	auth := PinReleaseAuthorization{PinID: pin.ID(), PlanRef: pin.PlanRef(), ManifestSHA256: pin.ManifestSHA256(), Disposition: PinAbandonedBeforeEffects, RecordVersion: 12}
	record.State, record.Disposition, record.RecordVersion = "RELEASING", auth.Disposition, auth.RecordVersion
	live := filepath.Join(root, record.ID)
	if err = writeLeaseRecord(live, &record, s.options.MaxMetadataBytesPerLease); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, ".releases", record.ID)
	if err = os.Mkdir(marker, 0700); err != nil {
		t.Fatal(err)
	}
	if err = writeLeaseRecord(marker, &record, maxLeaseMetadataBytes); err != nil {
		t.Fatal(err)
	}
	if len(record.Files) < 2 {
		t.Fatal("fixture lacks staged artifact inventory")
	}
	leaseRoot, err := os.OpenRoot(live)
	if err != nil {
		t.Fatal(err)
	}
	leaseInfo, err := leaseRoot.Stat(".")
	if err != nil {
		t.Fatal(errors.Join(err, leaseRoot.Close()))
	}
	dev, ino, err := fileIdentity(leaseInfo)
	if err != nil || dev != record.RootDevice || ino != record.RootInode {
		t.Fatal(errors.Join(ErrSnapshotLeaseInvalid, err, leaseRoot.Close()))
	}
	artifact, err := leaseRoot.Open(record.Files[0].Name)
	if err != nil {
		t.Fatal(errors.Join(err, leaseRoot.Close()))
	}
	artifactInfo, err := artifact.Stat()
	closeErr := artifact.Close()
	if err != nil || closeErr != nil {
		t.Fatal(errors.Join(err, closeErr, leaseRoot.Close()))
	}
	artifactDev, artifactIno, err := fileIdentity(artifactInfo)
	if err != nil || artifactDev != record.Files[0].Device || artifactIno != record.Files[0].Inode || artifactInfo.Size() != record.Files[0].Size {
		t.Fatal(errors.Join(ErrSnapshotLeaseInvalid, err, leaseRoot.Close()))
	}
	if err = leaseRoot.Remove(record.Files[0].Name); err == nil {
		dir, e := leaseRoot.Open(".")
		if e == nil {
			e = dir.Sync()
			err = errors.Join(e, dir.Close())
		} else {
			err = e
		}
	}
	if err = errors.Join(err, leaseRoot.Close()); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.decisions[pin.ID()] = PinReconcileDecision{Action: PinKeep}
	a.mu.Unlock()
	if err = s.Reconcile(context.Background()); err == nil {
		t.Fatal("PinKeep incorrectly authorized partial deletion")
	}
	if _, err = os.Stat(filepath.Join(live, record.Files[1].Name)); err != nil {
		t.Fatalf("PinKeep removed remaining artifact: %v", err)
	}
	a.mu.Lock()
	a.decisions[pin.ID()] = PinReconcileDecision{Action: PinRelease, Authorization: auth}
	a.mu.Unlock()
	if err = s.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile partial release: %v", err)
	}
	if _, err = os.Lstat(live); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("live lease remains: %v", err)
	}
	if _, err = readReleaseTombstone(marker); err != nil {
		t.Fatalf("retained release receipt: %v", err)
	}
	if err = s.Reconcile(context.Background()); err != nil {
		t.Fatalf("repeat reconcile after terminal owner ack: %v", err)
	}
	if _, err = os.Lstat(marker); err != nil {
		t.Fatalf("terminal receipt removed: %v", err)
	}
}

func TestSnapshotLeaseReleaseRefusesSameBytesWithReplacedLeaseIdentityDuringOwnerCallback(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 0)
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-replaced-callback")
	a := newLeaseTestAuthority()
	s := testLeaseStore(t, root, a)
	lease, err := s.Stage(context.Background(), snapshot, opts)
	if err != nil {
		t.Fatal(err)
	}
	pin, err := lease.Pin(context.Background(), strings.Repeat("e", 64), 1, opts.ExpectedManifestSHA256)
	if err != nil {
		t.Fatal(err)
	}
	auth := PinReleaseAuthorization{PinID: pin.ID(), PlanRef: pin.PlanRef(), ManifestSHA256: pin.ManifestSHA256(), Disposition: PinAbandonedBeforeEffects, RecordVersion: 13}
	a.mu.Lock()
	a.decisions[pin.ID()] = PinReconcileDecision{Action: PinRelease, Authorization: auth}
	a.mu.Unlock()
	live, moved := filepath.Join(root, lease.LeaseID()), filepath.Join(root, lease.LeaseID()+".moved")
	a.mu.Lock()
	a.beforeReconcile = func(SnapshotPinRef) error {
		if err := os.Rename(live, moved); err != nil {
			return err
		}
		if err := os.Mkdir(live, 0700); err != nil {
			return err
		}
		entries, err := os.ReadDir(moved)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				return errors.New("unexpected nested lease entry")
			}
			raw, err := os.ReadFile(filepath.Join(moved, entry.Name()))
			if err != nil {
				return err
			}
			if err = os.WriteFile(filepath.Join(live, entry.Name()), raw, 0600); err != nil {
				return err
			}
		}
		return nil
	}
	a.mu.Unlock()
	if err = s.Reconcile(context.Background()); !errors.Is(err, ErrSnapshotLeaseInvalid) && !errors.Is(err, ErrSnapshotLeaseConflict) {
		t.Fatalf("replacement callback reconcile=%v, want identity refusal", err)
	}
	if _, err = os.Lstat(live); err != nil {
		t.Fatalf("replacement lease unexpectedly removed: %v", err)
	}
	if _, err = os.Lstat(moved); err != nil {
		t.Fatalf("original lease not retained: %v", err)
	}
	a.mu.Lock()
	completes := a.completes
	a.mu.Unlock()
	if completes != 0 {
		t.Fatalf("owner completion called after identity replacement: %d", completes)
	}
}

func TestSnapshotLeaseIncompleteReleaseMarkerIsRetainedAndDenied(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 1)
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-empty-marker")
	a := newLeaseTestAuthority()
	s := testLeaseStore(t, root, a)
	lease, err := s.Stage(context.Background(), snapshot, opts)
	if err != nil {
		t.Fatal(err)
	}
	pin, err := lease.Pin(context.Background(), strings.Repeat("9", 64), 1, opts.ExpectedManifestSHA256)
	if err != nil {
		t.Fatal(err)
	}
	record, err := readLeaseRecord(filepath.Join(root, lease.LeaseID()), s.options.MaxMetadataBytesPerLease)
	if err != nil {
		t.Fatal(err)
	}
	record.State, record.Disposition, record.RecordVersion = "RELEASING", PinAbandonedBeforeEffects, 31
	if err = writeLeaseRecord(filepath.Join(root, record.ID), &record, s.options.MaxMetadataBytesPerLease); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, ".releases", record.ID)
	if err = os.Mkdir(marker, 0700); err != nil {
		t.Fatal(err)
	}
	auth := PinReleaseAuthorization{PinID: pin.ID(), PlanRef: pin.PlanRef(), ManifestSHA256: pin.ManifestSHA256(), Disposition: record.Disposition, RecordVersion: record.RecordVersion}
	a.mu.Lock()
	a.decisions[pin.ID()] = PinReconcileDecision{Action: PinRelease, Authorization: auth}
	a.mu.Unlock()
	if err = s.Reconcile(context.Background()); err == nil {
		t.Fatal("empty marker was silently repaired")
	}
	if _, err = os.Stat(filepath.Join(root, record.ID)); err != nil {
		t.Fatalf("live lease removed after incomplete marker: %v", err)
	}
	if _, err = os.Stat(marker); err != nil {
		t.Fatalf("incomplete marker was removed: %v", err)
	}
	entries, err := os.ReadDir(marker)
	if err != nil || len(entries) != 0 {
		t.Fatalf("incomplete marker changed: entries=%v err=%v", entries, err)
	}
	a.mu.Lock()
	completes := a.completes
	a.mu.Unlock()
	if completes != 0 {
		t.Fatalf("owner completed incomplete marker: %d", completes)
	}
}

func TestSnapshotLeaseSameInodeReceiptMutationDuringTerminalAckIsRejected(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 0)
	opts, _ := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-mutated-receipt")
	a := newLeaseTestAuthority()
	s := testLeaseStore(t, root, a)
	lease, err := s.Stage(context.Background(), snapshot, opts)
	if err != nil {
		t.Fatal(err)
	}
	pin, err := lease.Pin(context.Background(), strings.Repeat("8", 64), 1, opts.ExpectedManifestSHA256)
	if err != nil {
		t.Fatal(err)
	}
	auth := PinReleaseAuthorization{PinID: pin.ID(), PlanRef: pin.PlanRef(), ManifestSHA256: pin.ManifestSHA256(), Disposition: PinAbandonedBeforeEffects, RecordVersion: 37}
	a.mu.Lock()
	a.decisions[pin.ID()] = PinReconcileDecision{Action: PinRelease, Authorization: auth}
	a.mu.Unlock()
	if err = s.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, ".releases", lease.LeaseID())
	receipt := filepath.Join(marker, releaseRecordName)
	before, err := os.Lstat(receipt)
	if err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.beforeComplete = func(PinReleaseAuthorization) error {
		raw, e := os.ReadFile(receipt)
		if e != nil {
			return e
		}
		i := bytes.Index(raw, []byte(`"checksum":"`))
		if i < 0 {
			return errors.New("receipt checksum missing")
		}
		i += len(`"checksum":"`)
		if raw[i] == '0' {
			raw[i] = '1'
		} else {
			raw[i] = '0'
		}
		f, e := os.OpenFile(receipt, os.O_WRONLY|os.O_TRUNC, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write(raw)
		if e == nil {
			e = f.Sync()
		}
		return errors.Join(e, f.Close())
	}
	a.mu.Unlock()
	if err = s.Reconcile(context.Background()); !errors.Is(err, ErrSnapshotLeaseInvalid) {
		t.Fatalf("same-inode receipt mutation result=%v, want invalid receipt", err)
	}
	after, statErr := os.Lstat(receipt)
	if statErr != nil || !os.SameFile(before, after) {
		t.Fatalf("receipt identity changed unexpectedly: %v", statErr)
	}
	if _, statErr = os.Stat(marker); statErr != nil {
		t.Fatalf("receipt marker removed after failed exact acknowledgment: %v", statErr)
	}
}

func TestSnapshotLeaseReleasePeakReservationIncludesTerminalReceiptBeforeEffects(t *testing.T) {
	_, snapshot, _ := freshPrivateSnapshot(t, 1)
	options, digest := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-release-peak-budget")
	a := newLeaseTestAuthority()
	s := testLeaseStoreOptions(t, SnapshotLeaseStoreOptions{LeaseRoot: root, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 2 << 20, MaxLeases: 8}, a)
	options.ExpectedManifestSHA256 = digest
	lease, err := s.Stage(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	pin, err := lease.Pin(context.Background(), strings.Repeat("e", 64), 5, digest)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, lease.LeaseID())
	current, err := readLeaseRecord(path, s.options.MaxMetadataBytesPerLease)
	if err != nil {
		t.Fatal(err)
	}
	auth := PinReleaseAuthorization{PinID: pin.ID(), PlanRef: pin.PlanRef(), ManifestSHA256: digest, Disposition: PinAbandonedBeforeEffects, RecordVersion: 23}
	candidate := current
	candidate.State = "RELEASING"
	candidate.Disposition = auth.Disposition
	candidate.RecordVersion = auth.RecordVersion
	candidateRaw, err := encodeLeaseRecord(&candidate, s.options.MaxMetadataBytesPerLease)
	if err != nil {
		t.Fatal(err)
	}
	_, tombstoneRaw, err := encodeReleaseTombstone(candidate)
	if err != nil {
		t.Fatal(err)
	}
	usage, err := s.usageLocked()
	if err != nil {
		t.Fatal(err)
	}
	base := usage.metadata - current.MetadataBytes
	if base < 0 || candidate.MetadataBytes == 0 || len(tombstoneRaw) == 0 {
		t.Fatalf("invalid release budget fixture: usage=%+v current=%d candidate=%d tomb=%d", usage, current.MetadataBytes, candidate.MetadataBytes, len(tombstoneRaw))
	}
	limit := base + 2*candidate.MetadataBytes + int64(len(tombstoneRaw)) - 1
	if limit < base+2*candidate.MetadataBytes || limit >= s.options.MaxRetainedMetadataBytes {
		t.Fatalf("fixture does not isolate terminal peak: base=%d record=%d tombstone=%d limit=%d configured=%d", base, candidate.MetadataBytes, len(tombstoneRaw), limit, s.options.MaxRetainedMetadataBytes)
	}
	boundedOptions := s.options
	boundedOptions.MaxMetadataBytesPerLease = candidate.MetadataBytes
	boundedOptions.MaxRetainedMetadataBytes = limit
	if err = validateSnapshotLeaseOptions(boundedOptions); err != nil {
		t.Fatalf("bounded policy is not constructor-valid: %v", err)
	}
	identity, err := PreflightSnapshotStoreIdentity(context.Background(), boundedOptions)
	if err != nil {
		t.Fatal(err)
	}
	boundedStore, err := NewSnapshotLeaseStore(context.Background(), boundedOptions, a, identity)
	if err != nil {
		t.Fatal(err)
	}
	beforeRecord, err := os.ReadFile(filepath.Join(path, leaseRecordName))
	if err != nil {
		t.Fatal(err)
	}
	a.decisions[pin.ID()] = PinReconcileDecision{Action: PinRelease, Authorization: auth}
	if err = boundedStore.Reconcile(context.Background()); !errors.Is(err, ErrSnapshotLeaseLimit) {
		t.Fatalf("release peak outside budget was not refused: %v", err)
	}
	afterRecord, readErr := os.ReadFile(filepath.Join(path, leaseRecordName))
	if readErr != nil || !bytes.Equal(beforeRecord, afterRecord) {
		t.Fatalf("failed peak reservation changed live lease record: read=%v", readErr)
	}
	if _, err = os.Lstat(filepath.Join(root, ".releases", lease.LeaseID())); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed peak reservation published marker: %v", err)
	}
	if a.completes != 0 {
		t.Fatalf("failed peak reservation completed owner release %d times", a.completes)
	}
	if int64(len(candidateRaw)) >= candidate.MetadataBytes {
		t.Fatalf("fixture did not carry a separately-accounted manifest: wire=%d metadata=%d", len(candidateRaw), candidate.MetadataBytes)
	}
}
