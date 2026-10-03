//go:build hostedtest

package backup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/testhooks"
)

func TestSnapshotLeaseCrashRestartAfterCapturedPartialRelease(t *testing.T) {
	if os.Getenv("SERENITY_TEST_SNAPSHOT_RELEASE_CHILD") == "1" {
		root, snapshot, digest := os.Getenv("SERENITY_TEST_LEASE_ROOT"), os.Getenv("SERENITY_TEST_SNAPSHOT"), os.Getenv("SERENITY_TEST_MANIFEST_SHA")
		a := newLeaseTestAuthority()
		s := testLeaseStoreOptions(t, SnapshotLeaseStoreOptions{LeaseRoot: root, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 2 << 20, MaxLeases: 8}, a)
		opts, _ := inspectionOptions(t, snapshot)
		opts.ExpectedManifestSHA256 = digest
		lease, err := s.Stage(context.Background(), snapshot, opts)
		if err != nil {
			t.Fatal(err)
		}
		pin, err := lease.Pin(context.Background(), strings.Repeat("f", 64), 3, digest)
		if err != nil {
			t.Fatal(err)
		}
		auth := PinReleaseAuthorization{PinID: pin.ID(), PlanRef: pin.PlanRef(), ManifestSHA256: pin.ManifestSHA256(), Disposition: PinAbandonedBeforeEffects, RecordVersion: 17}
		a.mu.Lock()
		a.decisions[pin.ID()] = PinReconcileDecision{Action: PinRelease, Authorization: auth}
		a.mu.Unlock()
		if err = s.Reconcile(context.Background()); err != nil {
			t.Fatalf("child release: %v", err)
		}
		t.Fatal("release returned instead of pausing at the armed marker barrier")
	}
	_, snapshot, _ := freshPrivateSnapshot(t, 1)
	_, digest := inspectionOptions(t, snapshot)
	root := filepath.Join(privateTempDir(t), "snapshot-lease-real-crash-restart")
	armR, armW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer armR.Close()
	statusR, statusW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer statusR.Close()
	if _, err = fmt.Fprintf(armW, "arm %s pause\nstart\n", testhooks.PhaseSnapshotReleaseMarkerJournalSynced); err != nil {
		t.Fatal(err)
	}
	if err = armW.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSnapshotLeaseCrashRestartAfterCapturedPartialRelease$")
	cmd.ExtraFiles = []*os.File{armR, statusW}
	cmd.Env = append(os.Environ(), "SERENITY_HOSTED_TESTHOOKS_ARM_FD=3", "SERENITY_HOSTED_TESTHOOKS_STATUS_FD=4", "SERENITY_TEST_SNAPSHOT_RELEASE_CHILD=1", "SERENITY_TEST_LEASE_ROOT="+root, "SERENITY_TEST_SNAPSHOT="+snapshot, "SERENITY_TEST_MANIFEST_SHA="+digest)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = statusW.Close()
	waited := false
	stop := func() {
		if !waited && cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			waited = true
		}
	}
	t.Cleanup(stop)
	statusCh := make(chan struct {
		line string
		err  error
	}, 1)
	go func() {
		line, e := bufio.NewReader(statusR).ReadString('\n')
		statusCh <- struct {
			line string
			err  error
		}{line, e}
	}()
	var line string
	var readErr error
	select {
	case result := <-statusCh:
		line, readErr = result.line, result.err
	case <-ctx.Done():
		stop()
		waited = true
		t.Fatalf("timed out waiting for child crash barrier: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if readErr != nil || line != "paused "+testhooks.PhaseSnapshotReleaseMarkerJournalSynced+"\n" {
		stop()
		waited = true
		t.Fatalf("child pause status=%q err=%v stdout=%q stderr=%q", line, readErr, stdout.String(), stderr.String())
	}
	marker := filepath.Join(root, ".releases")
	entries, err := os.ReadDir(marker)
	if err != nil || len(entries) != 1 {
		stop()
		t.Fatalf("marker inventory=%v err=%v", entries, err)
	}
	record, err := readLeaseRecord(filepath.Join(marker, entries[0].Name()), maxLeaseMetadataBytes)
	if err != nil {
		stop()
		t.Fatal(err)
	}
	live := filepath.Join(root, record.ID)
	if len(record.Files) == 0 {
		stop()
		t.Fatal("missing artifact inventory")
	}
	leaseRoot, err := os.OpenRoot(live)
	if err != nil {
		stop()
		t.Fatal(err)
	}
	leaseInfo, err := leaseRoot.Stat(".")
	if err != nil {
		stop()
		t.Fatal(errors.Join(err, leaseRoot.Close()))
	}
	rootDev, rootIno, err := fileIdentity(leaseInfo)
	if err != nil || rootDev != record.RootDevice || rootIno != record.RootInode {
		stop()
		t.Fatal(errors.Join(ErrSnapshotLeaseInvalid, err, leaseRoot.Close()))
	}
	file := record.Files[0]
	f, err := leaseRoot.Open(file.Name)
	if err != nil {
		stop()
		t.Fatal(errors.Join(err, leaseRoot.Close()))
	}
	fileInfo, err := f.Stat()
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		stop()
		t.Fatal(errors.Join(err, closeErr, leaseRoot.Close()))
	}
	fileDev, fileIno, err := fileIdentity(fileInfo)
	if err != nil || fileDev != file.Device || fileIno != file.Inode || fileInfo.Size() != file.Size {
		stop()
		t.Fatal(errors.Join(ErrSnapshotLeaseInvalid, err, leaseRoot.Close()))
	}
	if err = leaseRoot.Remove(file.Name); err == nil {
		var dir *os.File
		dir, err = leaseRoot.Open(".")
		if err == nil {
			err = dir.Sync()
			err = errors.Join(err, dir.Close())
		}
	}
	if err = errors.Join(err, leaseRoot.Close()); err != nil {
		stop()
		t.Fatal(err)
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	waitErr := cmd.Wait()
	waited = true
	if waitErr == nil {
		t.Fatal("child survived deliberate crash")
	}
	a := newLeaseTestAuthority()
	attempt := PinAttemptRef{PlanRef: record.PlanRef, LeaseID: record.ID, ManifestSHA256: record.ManifestSHA256, ReservationVersion: record.ReservationVersion, AttemptVersion: record.AttemptVersion, State: PinAttemptCommitted}
	a.attempts[attemptKey(attempt.PlanRef, attempt.ManifestSHA256)] = attempt
	a.decisions[record.PinID] = PinReconcileDecision{Action: PinRelease, Authorization: PinReleaseAuthorization{PinID: record.PinID, PlanRef: record.PlanRef, ManifestSHA256: record.ManifestSHA256, Disposition: record.Disposition, RecordVersion: record.RecordVersion}}
	options := SnapshotLeaseStoreOptions{LeaseRoot: root, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 2 << 20, MaxLeases: 8}
	identity, err := PreflightSnapshotStoreIdentity(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := NewSnapshotLeaseStore(context.Background(), options, a, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err = reopened.Reconcile(context.Background()); err != nil {
		t.Fatalf("restart reconciliation: %v", err)
	}
	if _, err = readReleaseTombstone(filepath.Join(root, ".releases", record.ID)); err != nil {
		t.Fatalf("release receipt after restart: %v", err)
	}
	if err = reopened.Reconcile(context.Background()); err != nil {
		t.Fatalf("repeat reconciliation after terminal completion: %v", err)
	}
}

func TestSnapshotLeaseCrashBarrierMatrix(t *testing.T) {
	if os.Getenv("SERENITY_TEST_SNAPSHOT_MATRIX_CHILD") == "1" {
		root, snapshot, digest := os.Getenv("SERENITY_TEST_LEASE_ROOT"), os.Getenv("SERENITY_TEST_SNAPSHOT"), os.Getenv("SERENITY_TEST_MANIFEST_SHA")
		mode := os.Getenv("SERENITY_TEST_SNAPSHOT_MODE")
		a := newLeaseTestAuthority()
		s := testLeaseStoreOptions(t, SnapshotLeaseStoreOptions{LeaseRoot: root, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 2 << 20, MaxLeases: 8}, a)
		opts, _ := inspectionOptions(t, snapshot)
		opts.ExpectedManifestSHA256 = digest
		lease, err := s.Stage(context.Background(), snapshot, opts)
		if err != nil {
			t.Fatal(err)
		}
		pin, err := lease.Pin(context.Background(), strings.Repeat("a", 64), 3, digest)
		if err != nil {
			t.Fatal(err)
		}
		if mode == "release" {
			auth := PinReleaseAuthorization{PinID: pin.ID(), PlanRef: pin.PlanRef(), ManifestSHA256: pin.ManifestSHA256(), Disposition: PinAbandonedBeforeEffects, RecordVersion: 19}
			a.mu.Lock()
			a.decisions[pin.ID()] = PinReconcileDecision{Action: PinRelease, Authorization: auth}
			a.mu.Unlock()
			if err = s.Reconcile(context.Background()); err != nil {
				t.Fatalf("release barrier operation: %v", err)
			}
		}
		t.Fatal("armed crash barrier was not reached")
	}
	type barrier struct{ name, mode string }
	pinPhases := []string{testhooks.PhaseSnapshotPinPendingTempCreated, testhooks.PhaseSnapshotPinPendingTempWritten, testhooks.PhaseSnapshotPinPendingTempSynced, testhooks.PhaseSnapshotPinPendingPublished, testhooks.PhaseSnapshotPinPendingDirectorySynced, testhooks.PhaseSnapshotPinPendingRootSynced, testhooks.PhaseSnapshotPinPendingDurable, testhooks.PhaseSnapshotPinOwnerCommitted, testhooks.PhaseSnapshotPinPinnedTempCreated, testhooks.PhaseSnapshotPinPinnedTempWritten, testhooks.PhaseSnapshotPinPinnedTempSynced, testhooks.PhaseSnapshotPinPinnedPublished, testhooks.PhaseSnapshotPinPinnedDirectorySynced, testhooks.PhaseSnapshotPinPinnedRootSynced, testhooks.PhaseSnapshotPinPromoted}
	releasePhases := []string{testhooks.PhaseSnapshotReleaseOwnerAuthorized, testhooks.PhaseSnapshotReleaseLiveRecordTempCreated, testhooks.PhaseSnapshotReleaseLiveRecordTempWritten, testhooks.PhaseSnapshotReleaseLiveRecordTempSynced, testhooks.PhaseSnapshotReleaseLiveRecordPublished, testhooks.PhaseSnapshotReleaseLiveRecordDirectorySynced, testhooks.PhaseSnapshotReleaseMarkerCreated, testhooks.PhaseSnapshotReleaseMarkerRecordTempCreated, testhooks.PhaseSnapshotReleaseMarkerRecordTempWritten, testhooks.PhaseSnapshotReleaseMarkerRecordTempSynced, testhooks.PhaseSnapshotReleaseMarkerRecordPublished, testhooks.PhaseSnapshotReleaseMarkerRecordDirectorySynced, testhooks.PhaseSnapshotReleaseMarkerJournalSynced, testhooks.PhaseSnapshotReleaseLiveTreeRemoved, testhooks.PhaseSnapshotReleaseLiveTreeRootSynced, testhooks.PhaseSnapshotReleaseTombstoneTempCreated, testhooks.PhaseSnapshotReleaseTombstoneTempWritten, testhooks.PhaseSnapshotReleaseTombstoneTempSynced, testhooks.PhaseSnapshotReleaseTombstonePublished, testhooks.PhaseSnapshotReleaseTombstoneDirectorySynced, testhooks.PhaseSnapshotReleaseMarkerRecordRemoved, testhooks.PhaseSnapshotReleaseMarkerDirectorySynced, testhooks.PhaseSnapshotReleaseTombstoneJournalSynced, testhooks.PhaseSnapshotReleaseOwnerCompletionStarting, testhooks.PhaseSnapshotReleaseOwnerCompleted}
	phases := make([]barrier, 0, len(pinPhases)+len(releasePhases))
	for _, phase := range pinPhases {
		phases = append(phases, barrier{phase, "pin"})
	}
	for _, phase := range releasePhases {
		phases = append(phases, barrier{phase, "release"})
	}
	_, snapshot, _ := freshPrivateSnapshot(t, 1)
	_, digest := inspectionOptions(t, snapshot)
	for _, tc := range phases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			root := filepath.Join(privateTempDir(t), "matrix-"+strings.ReplaceAll(tc.name, "_", "-"))
			armR, armW, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer armR.Close()
			statusR, statusW, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer statusR.Close()
			if _, err = fmt.Fprintf(armW, "arm %s pause\nstart\n", tc.name); err != nil {
				t.Fatal(err)
			}
			if err = armW.Close(); err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSnapshotLeaseCrashBarrierMatrix$")
			cmd.ExtraFiles = []*os.File{armR, statusW}
			cmd.Env = append(os.Environ(), "SERENITY_HOSTED_TESTHOOKS_ARM_FD=3", "SERENITY_HOSTED_TESTHOOKS_STATUS_FD=4", "SERENITY_TEST_SNAPSHOT_MATRIX_CHILD=1", "SERENITY_TEST_SNAPSHOT_MODE="+tc.mode, "SERENITY_TEST_LEASE_ROOT="+root, "SERENITY_TEST_SNAPSHOT="+snapshot, "SERENITY_TEST_MANIFEST_SHA="+digest)
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			_ = statusW.Close()
			waited := false
			stop := func() {
				if !waited && cmd.Process != nil {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
					waited = true
				}
			}
			t.Cleanup(stop)
			type result struct {
				line string
				err  error
			}
			statusCh := make(chan result, 1)
			go func() { line, e := bufio.NewReader(statusR).ReadString('\n'); statusCh <- result{line, e} }()
			select {
			case got := <-statusCh:
				if got.err != nil || got.line != "paused "+tc.name+"\n" {
					stop()
					t.Fatalf("barrier status=%q err=%v", got.line, got.err)
				}
			case <-ctx.Done():
				stop()
				t.Fatalf("timed out waiting for %s barrier", tc.name)
			}
			if err = cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			waitErr := cmd.Wait()
			waited = true
			if waitErr == nil {
				t.Fatal("barrier child survived forced crash")
			}
		})
	}
}
