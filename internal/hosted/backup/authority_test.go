package backup

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/authority"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/deletion"
)

type testCutAuthority struct {
	cut         authority.Cut
	finalized   authority.FinalizedCut
	objects     contracts.JournalObjectReader
	finalizeErr error
	lookupErr   error
	changeFinal bool
}

func (a *testCutAuthority) Cut(_ context.Context, l authority.Lease, attempt, _ string) (authority.Cut, error) {
	a.cut.LeaseID = l.ID
	a.cut.SnapshotAttemptID = attempt
	return a.cut, nil
}
func (a *testCutAuthority) FinalizeCut(_ context.Context, c authority.Cut, hash, op string) (authority.FinalizedCut, error) {
	a.finalized = authority.FinalizedCut{Cut: c, ManifestSHA256: hash, OperationID: op}
	if a.changeFinal {
		a.finalized.Cut.ID = strings.Repeat("b", 64)
	}
	return a.finalized, a.finalizeErr
}
func (a *testCutAuthority) ReadFinalizedCut(context.Context, string) (authority.FinalizedCut, error) {
	return a.finalized, a.lookupErr
}
func (a *testCutAuthority) CutReader(authority.Cut) contracts.JournalObjectReader { return a.objects }

func newTestCutAuthority(t *testing.T) *testCutAuthority {
	t.Helper()
	objects, err := deletion.NewFileObjectStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	journal := deletion.NewJournal(objects, "test-authority-writer", 1, nil)
	entry, err := journal.AppendDeletion(context.Background(), contracts.DeletionEntry{
		SubjectType: contracts.DeletionSubjectBrain, SubjectID: "brain-1234567890", Outcome: contracts.DeletionIntentRequested})
	if err != nil {
		t.Fatal(err)
	}
	return &testCutAuthority{objects: objects, cut: authority.Cut{ID: strings.Repeat("a", 64),
		Position: contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis-test", LastObjectInGeneration: entry.Watermark}}}
}
func TestAuthorizedSnapshotRoundTripAndTampering(t *testing.T) {
	for _, mode := range []string{"valid", "manifest", "sidecar", "authority", "history"} {
		t.Run(mode, func(t *testing.T) {
			a := newTestCutAuthority(t)
			data, _ := newTestDataDir(t, 1)
			snapshot := filepath.Join(t.TempDir(), "snapshot")
			ctx := context.Background()
			if err := CreateAuthorized(ctx, data, snapshot, "test-build", a, authority.Lease{ID: "test-lease"}); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "manifest":
				raw, err := os.ReadFile(filepath.Join(snapshot, manifestFile))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(snapshot, manifestFile), append(raw, ' '), 0600); err != nil {
					t.Fatal(err)
				}
			case "sidecar":
				p := filepath.Join(snapshot, authoritySidecar)
				raw, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, append(raw, ' '), 0600); err != nil {
					t.Fatal(err)
				}
			case "authority":
				a.lookupErr = authority.ErrConflict
			case "history":
				objects, err := deletion.NewFileObjectStore(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				a.objects = objects
			}
			destination := filepath.Join(t.TempDir(), "restored")
			err := RestoreAuthorized(ctx, snapshot, destination, a)
			if mode == "valid" {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil {
					t.Fatal("tampered or unavailable authority accepted")
				}
				if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("destination published: %v", err)
				}
			}
		})
	}
}
func TestAuthorizedSnapshotFinalizationFailureNeverPublishes(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		a := newTestCutAuthority(t)
		if mismatch {
			a.changeFinal = true
		} else {
			a.finalizeErr = authority.ErrUnavailable
		}
		data, _ := newTestDataDir(t, 0)
		destination := filepath.Join(t.TempDir(), "snapshot")
		if err := CreateAuthorized(context.Background(), data, destination, "test-build", a, authority.Lease{ID: "test-lease"}); err == nil {
			t.Fatal("invalid finalization accepted")
		}
		if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("destination published: %v", err)
		}
	}
}
func TestCutSidecarRejectsDuplicateUnknownOversizeAndSymlink(t *testing.T) {
	valid := `{"version":1,"cut_id":"` + strings.Repeat("a", 64) + `","manifest_sha256":"` + strings.Repeat("b", 64) + `"}`
	for _, raw := range []string{valid[:len(valid)-1] + `,"version":1}`, valid[:len(valid)-1] + `,"extra":0}`, strings.Repeat(" ", maxAuthoritySidecarBytes+1)} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, authoritySidecar), []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		_, err = readCutSidecar(root)
		_ = root.Close()
		if err == nil {
			t.Fatal("hostile sidecar accepted")
		}
	}
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(target, []byte(valid), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, authoritySidecar)); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	if _, err = readCutSidecar(root); err == nil {
		t.Fatal("sidecar symlink accepted")
	}
}

// TestAuthorizedArchiveRoundTrip crosses the real producer, tar transport,
// unprivileged unpacker and authenticated restore, including a nonempty brain.
func TestAuthorizedArchiveRoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" || os.Geteuid() == 0 {
		t.Skip("Linux unprivileged transport")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	ctx := context.Background()
	a := newTestCutAuthority(t)
	data, _ := newTestDataDir(t, 1)
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(root, "snapshot")
	if err := CreateAuthorized(ctx, data, snapshot, "test-build", a, authority.Lease{ID: "test-lease"}); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(root, "snapshot.tar")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(f)
	entries, err := os.ReadDir(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() {
			t.Fatalf("unexpected producer entry %s", entry.Name())
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			t.Fatal(err)
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		input, err := os.Open(filepath.Join(snapshot, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		_, copyErr := io.Copy(tw, input)
		if err := errors.Join(copyErr, input.Close()); err != nil {
			t.Fatal(err)
		}
	}
	if err := errors.Join(tw.Close(), f.Close()); err != nil {
		t.Fatal(err)
	}
	unpacked := filepath.Join(root, "unpacked")
	if out, err := exec.CommandContext(ctx, python, "../../../deploy/hosted/unpack_snapshot.py", archive, unpacked).CombinedOutput(); err != nil {
		t.Fatalf("unpack: %v: %s", err, out)
	}
	if err := RestoreAuthorized(ctx, unpacked, filepath.Join(root, "restored"), a); err != nil {
		t.Fatal(err)
	}
}
