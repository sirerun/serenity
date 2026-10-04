//go:build hostedtest

package backup

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/testhooks"
)

const maxCrashChildOutput = 64 << 10

type boundedCrashOutput struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (o *boundedCrashOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	n := len(p)
	remaining := maxCrashChildOutput - o.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = o.buf.Write(p)
	}
	return n, nil
}

func (o *boundedCrashOutput) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String()
}

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
	opts, _ := inspectionOptions(t, snapshot)
	digest := opts.ExpectedManifestSHA256
	root := filepath.Join(privateTempDir(t), "snapshot-lease-real-crash-restart")
	armR, armW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = armR.Close() }()
	defer func() { _ = armW.Close() }()
	statusR, statusW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = statusR.Close() }()
	if _, err = fmt.Fprintf(armW, "arm %s pause\nstart\n", testhooks.PhaseSnapshotReleaseMarkerJournalSynced); err != nil {
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
		_ = armW.Close()
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
	_ = armW.Close()
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
		if target := os.Getenv("SERENITY_TEST_SNAPSHOT_PREFIX_KIND"); target != "" {
			captureFile := os.NewFile(6, "captured-lease-temp")
			if captureFile == nil {
				t.Fatal("missing inherited canonical temp capture pipe")
			}
			defer func() { _ = captureFile.Close() }()
			restoreCaptureObserver := setSnapshotLeaseTempCaptureObserverForTest(func(capture snapshotLeaseTempCapture) {
				matches := false
				switch target {
				case "pin-pending":
					matches = capture.Kind == "lease" && capture.Record.State == "PIN_PENDING"
				case "pin-pinned":
					matches = capture.Kind == "lease" && capture.Record.State == "PINNED"
				case "release-live":
					matches = capture.Kind == "lease" && capture.Record.State == "RELEASING" && filepath.Base(filepath.Dir(capture.Path)) != ".releases"
				case "release-marker":
					matches = capture.Kind == "lease" && capture.Record.State == "RELEASING" && filepath.Base(filepath.Dir(capture.Path)) == ".releases"
				case "release-tombstone":
					matches = capture.Kind == "tombstone" && filepath.Base(filepath.Dir(capture.Path)) == ".releases"
				}
				if matches {
					if err := json.NewEncoder(captureFile).Encode(capture); err != nil {
						t.Errorf("write canonical temp capture: %v", err)
					}
				}
			})
			defer restoreCaptureObserver()
		}
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
			attempt := a.attempts[attemptKey(auth.PlanRef, auth.ManifestSHA256)]
			a.mu.Unlock()
			reportLeaseTestAuthorityState(leaseTestAuthorityState{Event: "release", Attempt: attempt, Authorization: auth, HasRelease: true})
			if err = s.Reconcile(context.Background()); err != nil {
				t.Fatalf("release barrier operation: %v", err)
			}
		}
		t.Fatal("armed crash barrier was not reached")
	}
	type barrier struct{ name, mode, prefix string }
	pinPhases := []string{testhooks.PhaseSnapshotPinPendingTempCreated, testhooks.PhaseSnapshotPinPendingTempWritten, testhooks.PhaseSnapshotPinPendingTempSynced, testhooks.PhaseSnapshotPinPendingPublished, testhooks.PhaseSnapshotPinPendingDirectorySynced, testhooks.PhaseSnapshotPinPendingRootSynced, testhooks.PhaseSnapshotPinPendingDurable, testhooks.PhaseSnapshotPinOwnerCommitted, testhooks.PhaseSnapshotPinPinnedTempCreated, testhooks.PhaseSnapshotPinPinnedTempWritten, testhooks.PhaseSnapshotPinPinnedTempSynced, testhooks.PhaseSnapshotPinPinnedPublished, testhooks.PhaseSnapshotPinPinnedDirectorySynced, testhooks.PhaseSnapshotPinPinnedRootSynced, testhooks.PhaseSnapshotPinPromoted}
	releasePhases := []string{testhooks.PhaseSnapshotReleaseOwnerAuthorized, testhooks.PhaseSnapshotReleaseLiveRecordTempCreated, testhooks.PhaseSnapshotReleaseLiveRecordTempWritten, testhooks.PhaseSnapshotReleaseLiveRecordTempSynced, testhooks.PhaseSnapshotReleaseLiveRecordPublished, testhooks.PhaseSnapshotReleaseLiveRecordDirectorySynced, testhooks.PhaseSnapshotReleaseMarkerCreated, testhooks.PhaseSnapshotReleaseMarkerRecordTempCreated, testhooks.PhaseSnapshotReleaseMarkerRecordTempWritten, testhooks.PhaseSnapshotReleaseMarkerRecordTempSynced, testhooks.PhaseSnapshotReleaseMarkerRecordPublished, testhooks.PhaseSnapshotReleaseMarkerRecordDirectorySynced, testhooks.PhaseSnapshotReleaseMarkerJournalSynced, testhooks.PhaseSnapshotReleaseLiveTreeRemoved, testhooks.PhaseSnapshotReleaseLiveTreeRootSynced, testhooks.PhaseSnapshotReleaseTombstoneTempCreated, testhooks.PhaseSnapshotReleaseTombstoneTempWritten, testhooks.PhaseSnapshotReleaseTombstoneTempSynced, testhooks.PhaseSnapshotReleaseTombstonePublished, testhooks.PhaseSnapshotReleaseTombstoneDirectorySynced, testhooks.PhaseSnapshotReleaseMarkerRecordRemoved, testhooks.PhaseSnapshotReleaseMarkerDirectorySynced, testhooks.PhaseSnapshotReleaseTombstoneJournalSynced, testhooks.PhaseSnapshotReleaseOwnerCompletionStarting, testhooks.PhaseSnapshotReleaseOwnerCompleted}
	phases := make([]barrier, 0, len(pinPhases)+len(releasePhases))
	for _, phase := range pinPhases {
		phases = append(phases, barrier{phase, "pin", ""})
	}
	for _, phase := range releasePhases {
		phases = append(phases, barrier{phase, "release", ""})
	}
	phases = append(phases,
		barrier{testhooks.PhaseSnapshotPinPendingTempCreated, "pin", "pin-pending"},
		barrier{testhooks.PhaseSnapshotPinPinnedTempCreated, "pin", "pin-pinned"},
		barrier{testhooks.PhaseSnapshotReleaseLiveRecordTempCreated, "release", "release-live"},
		barrier{testhooks.PhaseSnapshotReleaseMarkerRecordTempCreated, "release", "release-marker"},
		barrier{testhooks.PhaseSnapshotReleaseTombstoneTempCreated, "release", "release-tombstone"},
	)
	_, snapshot, _ := freshPrivateSnapshot(t, 1)
	opts, _ := inspectionOptions(t, snapshot)
	digest := opts.ExpectedManifestSHA256
	for _, tc := range phases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			root := filepath.Join(privateTempDir(t), "matrix-"+strings.ReplaceAll(tc.name, "_", "-"))
			armR, armW, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = armR.Close() }()
			defer func() { _ = armW.Close() }()
			statusR, statusW, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = statusR.Close() }()
			stateR, stateW, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = stateR.Close() }()
			if _, err = fmt.Fprintf(armW, "arm %s pause\nstart\n", tc.name); err != nil {
				t.Fatal(err)
			}
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSnapshotLeaseCrashBarrierMatrix$")
			var stdout, stderr boundedCrashOutput
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			captureR, captureW, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = captureR.Close() }()
			cmd.ExtraFiles = []*os.File{armR, statusW, stateW, captureW}
			cmd.Env = append(os.Environ(), "SERENITY_HOSTED_TESTHOOKS_ARM_FD=3", "SERENITY_HOSTED_TESTHOOKS_STATUS_FD=4", "SERENITY_TEST_AUTHORITY_STATE_FD=5", "SERENITY_TEST_SNAPSHOT_PREFIX_KIND="+tc.prefix, "SERENITY_TEST_SNAPSHOT_MATRIX_CHILD=1", "SERENITY_TEST_SNAPSHOT_MODE="+tc.mode, "SERENITY_TEST_LEASE_ROOT="+root, "SERENITY_TEST_SNAPSHOT="+snapshot, "SERENITY_TEST_MANIFEST_SHA="+digest)
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			_ = statusW.Close()
			_ = stateW.Close()
			_ = captureW.Close()
			waited := false
			stop := func() {
				if !waited && cmd.Process != nil {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
					waited = true
				}
				_ = armW.Close()
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
					t.Fatalf("barrier status=%q err=%v stdout=%q stderr=%q", got.line, got.err, stdout.String(), stderr.String())
				}
			case <-ctx.Done():
				stop()
				t.Fatalf("timed out waiting for %s barrier: stdout=%q stderr=%q", tc.name, stdout.String(), stderr.String())
			}
			var prefixState leaseTestAuthorityState
			var prefixCapture snapshotLeaseTempCapture
			if tc.prefix != "" {
				captureCh := make(chan error, 1)
				go func() {
					decoder := json.NewDecoder(io.LimitReader(captureR, 4<<20))
					captureCh <- decoder.Decode(&prefixCapture)
				}()
				stateCh := make(chan result, 1)
				wantLines := 1
				if tc.prefix == "pin-pinned" {
					wantLines = 2
				} else if strings.HasPrefix(tc.prefix, "release-") {
					wantLines = 3
				}
				go func() {
					reader := bufio.NewReader(stateR)
					var latest string
					for i := 0; i < wantLines; i++ {
						line, e := reader.ReadString('\n')
						if e != nil {
							stateCh <- result{latest, e}
							return
						}
						latest = line
					}
					stateCh <- result{latest, nil}
				}()
				select {
				case got := <-stateCh:
					if got.err != nil {
						stop()
						t.Fatalf("missing authoritative child state before partial-prefix fixture: %v", got.err)
					}
					if err = json.Unmarshal([]byte(got.line), &prefixState); err != nil {
						stop()
						t.Fatal(err)
					}
				case <-ctx.Done():
					stop()
					t.Fatalf("timed out waiting for child authority state at %s", tc.name)
				}
				select {
				case err = <-captureCh:
					if err != nil {
						stop()
						t.Fatalf("receive exact canonical temp capture: %v", err)
					}
				case <-ctx.Done():
					stop()
					t.Fatalf("timed out waiting for canonical temp capture at %s", tc.name)
				}
				if err = writeSnapshotLeaseCrashTempPrefix(root, prefixState, prefixCapture, tc.prefix); err != nil {
					stop()
					t.Fatalf("write reachable %s temp prefix: %v", tc.prefix, err)
				}
			}
			if err = cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = armW.Close()
			waitErr := cmd.Wait()
			waited = true
			if waitErr == nil {
				t.Fatal("barrier child survived forced crash")
			}
			state := prefixState
			if tc.prefix == "" {
				stateBytes, readErr := io.ReadAll(stateR)
				if readErr != nil {
					t.Fatal(readErr)
				}
				lines := strings.Split(strings.TrimSpace(string(stateBytes)), "\n")
				if len(lines) == 0 || lines[0] == "" {
					t.Fatal("child did not report authoritative test state before the crash")
				}
				if err = json.Unmarshal([]byte(lines[len(lines)-1]), &state); err != nil {
					t.Fatalf("decode child authority state: %v", err)
				}
			}
			if state.Attempt.LeaseID == "" || state.Attempt.ManifestSHA256 != digest {
				t.Fatalf("incomplete child authority state: %+v", state)
			}
			restartAuthority := newLeaseTestAuthority()
			restartAuthority.attempts[attemptKey(state.Attempt.PlanRef, state.Attempt.ManifestSHA256)] = state.Attempt
			if state.HasRelease {
				restartAuthority.decisions[state.Authorization.PinID] = PinReconcileDecision{Action: PinRelease, Authorization: state.Authorization}
			}
			beforeRestart, err := snapshotLeaseCrashTree(root)
			if err != nil {
				t.Fatal(err)
			}
			options := SnapshotLeaseStoreOptions{LeaseRoot: root, MaxArtifactBytesPerLease: 1 << 30, MaxMetadataBytesPerLease: 1 << 20, MaxRestoreScratchBytes: 1 << 30, MaxRetainedArtifactBytes: 2 << 30, MaxRetainedMetadataBytes: 2 << 20, MaxLeases: 8}
			identity, err := PreflightSnapshotStoreIdentity(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			reopened, err := NewSnapshotLeaseStore(context.Background(), options, restartAuthority, identity)
			if err != nil {
				t.Fatal(err)
			}
			restartErr := reopened.Reconcile(context.Background())
			if restartErr != nil {
				afterRestart, snapshotErr := snapshotLeaseCrashTree(root)
				if snapshotErr != nil || !reflect.DeepEqual(beforeRestart, afterRestart) {
					t.Fatalf("failed closed after mutating retained crash state: reconcile=%v snapshot=%v", restartErr, snapshotErr)
				}
				current, findErr := restartAuthority.FindPinAttempt(context.Background(), state.Attempt.PlanRef, digest)
				if findErr != nil || !sameAttempt(current, state.Attempt) || current.State != state.Attempt.State {
					t.Fatalf("failed restart changed retained owner attempt: current=%+v err=%v", current, findErr)
				}
				if restartAuthority.completes != 0 {
					t.Fatalf("failed restart performed owner completion %d times", restartAuthority.completes)
				}
				return
			}
			if state.Attempt.State == PinAttemptPending {
				current, findErr := restartAuthority.FindPinAttempt(context.Background(), state.Attempt.PlanRef, digest)
				if findErr != nil || !sameAttempt(current, state.Attempt) || current.State != PinAttemptPending {
					t.Fatalf("pending pin was not preserved after restart: current=%+v err=%v", current, findErr)
				}
				record, readErr := readLeaseRecord(filepath.Join(root, state.Attempt.LeaseID), options.MaxMetadataBytesPerLease)
				if readErr != nil {
					t.Fatalf("read retained local pending pin record: %v", readErr)
				}
				switch record.State {
				case "STAGED":
					afterRestart, snapshotErr := snapshotLeaseCrashTree(root)
					if snapshotErr != nil || !reflect.DeepEqual(beforeRestart, afterRestart) {
						t.Fatalf("pre-publication pending pin changed retained crash state: snapshot=%v", snapshotErr)
					}
				case "PIN_PENDING":
					if record.PinID == "" || record.AttemptVersion != state.Attempt.AttemptVersion || record.ReservationVersion != state.Attempt.ReservationVersion {
						t.Fatalf("published pending pin record does not match owner state: record=%+v attempt=%+v", record, state.Attempt)
					}
				default:
					t.Fatalf("unexpected local state for authoritative pending pin: %s", record.State)
				}
				if restartAuthority.completes != 0 {
					t.Fatalf("pre-publication pending pin performed owner completion %d times", restartAuthority.completes)
				}
				return
			}
			if state.HasRelease {
				if _, err = readReleaseTombstone(filepath.Join(root, ".releases", state.Attempt.LeaseID)); err != nil {
					t.Fatalf("released pin restart lacks terminal receipt: %v", err)
				}
				if _, err = os.Lstat(filepath.Join(root, state.Attempt.LeaseID)); !os.IsNotExist(err) {
					t.Fatalf("released lease remains after restart: %v", err)
				}
			} else {
				current, findErr := restartAuthority.FindPinAttempt(context.Background(), state.Attempt.PlanRef, digest)
				if findErr != nil || !sameAttempt(current, state.Attempt) || current.State != PinAttemptCommitted {
					t.Fatalf("committed pin was not preserved after restart: current=%+v err=%v", current, findErr)
				}
				record, readErr := readLeaseRecord(filepath.Join(root, state.Attempt.LeaseID), options.MaxMetadataBytesPerLease)
				if readErr != nil || record.State != "PINNED" {
					t.Fatalf("committed pin has no corresponding local PINNED record: state=%s err=%v", record.State, readErr)
				}
			}
		})
	}
}

func snapshotLeaseCrashTree(root string) (map[string]string, error) {
	state := make(map[string]string)
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	rootDevice, rootInode, err := fileIdentity(rootInfo)
	if err != nil {
		return nil, err
	}
	state["."] = fmt.Sprintf("root:%o:%d:%d", rootInfo.Mode(), rootDevice, rootInode)
	var visit func(string, string) error
	visit = func(path, rel string) error {
		entries, err := os.ReadDir(path)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			childRel := filepath.Join(rel, entry.Name())
			child := filepath.Join(path, entry.Name())
			info, err := os.Lstat(child)
			if err != nil {
				return err
			}
			device, inode, err := fileIdentity(info)
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				state[childRel] = fmt.Sprintf("symlink:%o:%d:%d", info.Mode(), device, inode)
				continue
			}
			if info.IsDir() {
				state[childRel] = fmt.Sprintf("dir:%o:%d:%d", info.Mode(), device, inode)
				if err = visit(child, childRel); err != nil {
					return err
				}
				continue
			}
			if !info.Mode().IsRegular() {
				state[childRel] = fmt.Sprintf("other:%o:%d:%d", info.Mode(), device, inode)
				continue
			}
			data, readErr := os.ReadFile(child)
			if readErr != nil {
				return readErr
			}
			state[childRel] = fmt.Sprintf("file:%o:%d:%d:%x", info.Mode(), device, inode, data)
		}
		return nil
	}
	if err := visit(root, ""); err != nil {
		return nil, err
	}
	return state, nil
}

func writeSnapshotLeaseCrashTempPrefix(root string, state leaseTestAuthorityState, capture snapshotLeaseTempCapture, kind string) error {
	if strings.HasPrefix(kind, "release-") && (!state.HasRelease || state.Attempt.State != PinAttemptCommitted || state.Authorization.PlanRef != state.Attempt.PlanRef || state.Authorization.ManifestSHA256 != state.Attempt.ManifestSHA256) {
		return ErrSnapshotLeaseConflict
	}
	if kind == "pin-pending" && state.Attempt.State != PinAttemptPending || kind == "pin-pinned" && state.Attempt.State != PinAttemptCommitted {
		return ErrSnapshotLeaseConflict
	}
	expectedDir := filepath.Join(root, state.Attempt.LeaseID)
	tempPrefix := ".lease.tmp-"
	expectedCaptureKind := "lease"
	if kind == "release-marker" || kind == "release-tombstone" {
		expectedDir = filepath.Join(root, ".releases", state.Attempt.LeaseID)
	}
	if kind == "release-tombstone" {
		tempPrefix = ".release.tmp-"
		expectedCaptureKind = "tombstone"
	}
	if capture.Kind != expectedCaptureKind || capture.Path != expectedDir || !strings.HasPrefix(capture.Name, tempPrefix) || len(capture.Raw) < 2 || capture.Record.ID != state.Attempt.LeaseID || capture.Record.ManifestSHA256 != state.Attempt.ManifestSHA256 || capture.Record.PlanRef != state.Attempt.PlanRef || capture.Record.ReservationVersion != state.Attempt.ReservationVersion || capture.Record.AttemptVersion != state.Attempt.AttemptVersion {
		return fmt.Errorf("%w: capture tuple mismatch kind=%s path=%s record=%+v attempt=%+v", ErrSnapshotLeaseConflict, kind, capture.Path, capture.Record, state.Attempt)
	}
	if strings.HasPrefix(kind, "release-") && (capture.Record.PinID != state.Authorization.PinID || capture.Record.RecordVersion != state.Authorization.RecordVersion || capture.Record.Disposition != state.Authorization.Disposition) {
		return ErrSnapshotLeaseConflict
	}
	wantState := map[string]string{
		"pin-pending": "PIN_PENDING", "pin-pinned": "PINNED",
		"release-live": "RELEASING", "release-marker": "RELEASING",
		"release-tombstone": "RELEASED",
	}[kind]
	if capture.Record.State != wantState || kind == "pin-pending" && capture.Record.PinID == "" {
		return ErrSnapshotLeaseConflict
	}
	// RootDevice/RootInode bind the lease directory, not the store root or
	// the separate release-marker directory used by marker/tombstone temps.
	storeDev, storeIno, err := pathIdentity(root)
	if err != nil || storeDev != capture.StoreDevice || storeIno != capture.StoreInode {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	if _, liveErr := os.Lstat(filepath.Join(root, state.Attempt.LeaseID)); liveErr == nil {
		rootDev, rootIno, identityErr := pathIdentity(filepath.Join(root, state.Attempt.LeaseID))
		if identityErr != nil || rootDev != capture.Record.RootDevice || rootIno != capture.Record.RootInode {
			return errors.Join(ErrSnapshotLeaseInvalid, identityErr)
		}
	} else if !errors.Is(liveErr, os.ErrNotExist) || kind != "release-tombstone" {
		return errors.Join(ErrSnapshotLeaseInvalid, liveErr)
	}
	dirInfo, err := os.Lstat(expectedDir)
	if err != nil || !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	dirDev, dirIno, err := fileIdentity(dirInfo)
	if err != nil || dirDev != capture.DirDevice || dirIno != capture.DirInode {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	path := filepath.Join(expectedDir, capture.Name)
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm() != 0600 || before.Size() != 0 {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	fileDev, fileIno, err := fileIdentity(before)
	if err != nil || fileDev != capture.TempDevice || fileIno != capture.TempInode {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	anchored, err := os.OpenRoot(expectedDir)
	if err != nil {
		return err
	}
	openedDir, err := anchored.Stat(".")
	if err != nil || !os.SameFile(dirInfo, openedDir) {
		return errors.Join(ErrSnapshotLeaseInvalid, err, anchored.Close())
	}
	f, err := anchored.OpenFile(capture.Name, os.O_WRONLY, 0)
	if err != nil {
		return errors.Join(err, anchored.Close())
	}
	openedFile, err := f.Stat()
	if err != nil || !os.SameFile(before, openedFile) {
		return errors.Join(ErrSnapshotLeaseInvalid, err, f.Close(), anchored.Close())
	}
	prefix := capture.Raw[:len(capture.Raw)/3]
	n, writeErr := f.Write(prefix)
	if writeErr == nil && n != len(prefix) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		writeErr = f.Sync()
	}
	writeErr = errors.Join(writeErr, f.Close())
	if writeErr == nil {
		dirFile, openErr := anchored.Open(".")
		if openErr == nil {
			writeErr = dirFile.Sync()
			writeErr = errors.Join(writeErr, dirFile.Close())
		} else {
			writeErr = openErr
		}
	}
	current, statErr := os.Lstat(path)
	if statErr != nil || !os.SameFile(before, current) {
		writeErr = errors.Join(writeErr, ErrSnapshotLeaseInvalid, statErr)
	}
	finalRootDev, finalRootIno, rootErr := pathIdentity(root)
	finalDir, dirErr := os.Lstat(expectedDir)
	if rootErr != nil || dirErr != nil || finalRootDev != storeDev || finalRootIno != storeIno || !os.SameFile(dirInfo, finalDir) {
		writeErr = errors.Join(writeErr, ErrSnapshotLeaseInvalid, rootErr, dirErr)
	}
	return errors.Join(writeErr, anchored.Close())
}
