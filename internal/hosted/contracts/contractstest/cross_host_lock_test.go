//go:build unix

package contractstest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/writer"
)

// The restore fence cannot be the local writer lock. writer.AcquireBrain is a
// kernel-local flock on a file inside the brain: it excludes a second process
// on the same filesystem, and it carries nothing across hosts. A snapshot
// restored onto another host brings a copy of the lock file, and acquiring the
// copy succeeds while the original holder is still alive and writing. This is
// the regression interfaces.md requires before task50 starts.
func TestLocalWriterLockCannotFenceAnotherHost(t *testing.T) {
	original, restored := t.TempDir(), t.TempDir()
	held, err := writer.AcquireBrain(original)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Close() }()

	// The lock does its own job: a second acquire of the same brain is refused.
	if second, err := writer.AcquireBrain(original); err == nil {
		_ = second.Close()
		t.Fatal("a second process acquired a brain that is already held")
	}

	// It stores no PID or host identity for another machine to read.
	lockPath := filepath.Join(original, ".serenity", "writer.lock")
	contents, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(contents) != 0 {
		t.Fatalf("lock file holds %d bytes; the fence design assumes it records no holder", len(contents))
	}

	// Snapshot restore: the lock file is copied into another host's brain directory.
	if err := os.MkdirAll(filepath.Join(restored, ".serenity"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(restored, ".serenity", "writer.lock"), contents, 0o600); err != nil {
		t.Fatal(err)
	}
	copyHeld, err := writer.AcquireBrain(restored)
	if err != nil {
		t.Fatalf("acquiring the restored copy failed: %v; the lock unexpectedly carries cross-host state", err)
	}
	defer func() { _ = copyHeld.Close() }()
	// Both "hosts" now believe they own the brain: the local lock proves nothing across hosts.
}
