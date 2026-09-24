package backup

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// buildBackupProbe compiles testdata/backupprobe, with or without the
// hostedtest build tag, proving the tag (not just an armed pipe) gates
// activation of the fault barrier this package's own Create call site fires.
func buildBackupProbe(t *testing.T, tag bool) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "backupprobe")
	args := []string{"build", "-o", out}
	if tag {
		args = append(args, "-tags", "hostedtest")
	}
	args = append(args, "./testdata/backupprobe")
	cmd := exec.Command("go", args...)
	cmd.Dir = "."
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build backupprobe (tag=%v): %v: %s", tag, err, stderr.String())
	}
	return out
}

// TestProductionBuildIgnoresArmedCrashAndPublishes proves that in a binary
// built without -tags hostedtest, PhaseBackupManifestWritten never fires as
// an active checkpoint even when a real control pipe is armed to crash there:
// Create runs to completion and the snapshot is published.
func TestProductionBuildIgnoresArmedCrashAndPublishes(t *testing.T) {
	bin := buildBackupProbe(t, false)
	base := t.TempDir()
	armR, armW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = armR.Close() }()
	if _, err = armW.WriteString("arm backup_manifest_written crash\nstart\n"); err != nil {
		t.Fatal(err)
	}
	_ = armW.Close()
	cmd := exec.Command(bin, base)
	cmd.ExtraFiles = []*os.File{armR}
	cmd.Env = append(os.Environ(), "SERENITY_HOSTED_TESTHOOKS_ARM_FD=3")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err = cmd.Run(); err != nil {
		t.Fatalf("untagged probe should exit 0 and ignore the armed crash: %v (stdout=%q)", err, out.String())
	}
	if got := out.String(); got != "reached-published\n" {
		t.Fatalf("untagged probe output = %q, want %q", got, "reached-published\n")
	}
	if _, statErr := os.Stat(filepath.Join(base, "snapshot", "manifest.json")); statErr != nil {
		t.Fatalf("expected a published manifest, stat err = %v", statErr)
	}
}

// TestTaggedBuildCrashesBeforePublication proves the fault barrier activates
// only in a hostedtest-tagged binary with a real armed pipe, and that it
// fires strictly before the destination is published: the crash must leave
// no destination directory at all, even though every artifact was already
// staged.
func TestTaggedBuildCrashesBeforePublication(t *testing.T) {
	bin := buildBackupProbe(t, true)
	base := t.TempDir()
	armR, armW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = armR.Close() }()
	if _, err = armW.WriteString("arm backup_manifest_written crash\nstart\n"); err != nil {
		t.Fatal(err)
	}
	_ = armW.Close()
	cmd := exec.Command(bin, base)
	cmd.ExtraFiles = []*os.File{armR}
	cmd.Env = append(os.Environ(), "SERENITY_HOSTED_TESTHOOKS_ARM_FD=3")
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %v (stdout=%q)", err, out.String())
	}
	if exitErr.ExitCode() != 137 {
		t.Fatalf("exit code = %d, want 137", exitErr.ExitCode())
	}
	if got := out.String(); got != "" {
		t.Fatalf("crashed probe should never print, got %q", got)
	}
	if _, statErr := os.Stat(filepath.Join(base, "snapshot")); !os.IsNotExist(statErr) {
		t.Fatalf("destination must not exist after a crash at the pre-publish checkpoint, stat err = %v", statErr)
	}
}

// TestTaggedBuildPauseHoldsPublicationUntilReleased proves the checkpoint
// sits exactly between staging and publication: while paused, the
// destination does not exist yet; after release, it does.
func TestTaggedBuildPauseHoldsPublicationUntilReleased(t *testing.T) {
	bin := buildBackupProbe(t, true)
	base := t.TempDir()
	armR, armW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = armR.Close() }()
	statusR, statusW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = statusR.Close() }()
	if _, err = armW.WriteString("arm backup_manifest_written pause\nstart\n"); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, base)
	cmd.ExtraFiles = []*os.File{armR, statusW}
	cmd.Env = append(os.Environ(), "SERENITY_HOSTED_TESTHOOKS_ARM_FD=3", "SERENITY_HOSTED_TESTHOOKS_STATUS_FD=4")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = statusW.Close()
	statusBuf := make([]byte, 4096)
	n, err := statusR.Read(statusBuf)
	if err != nil || string(statusBuf[:n]) != "paused backup_manifest_written\n" {
		_ = cmd.Process.Kill()
		t.Fatalf("expected pause status, got %q err=%v", string(statusBuf[:n]), err)
	}
	if _, statErr := os.Stat(filepath.Join(base, "snapshot")); !os.IsNotExist(statErr) {
		t.Fatalf("destination must not exist while paused at the pre-publish checkpoint, stat err = %v", statErr)
	}
	if _, err = armW.WriteString("release backup_manifest_written\n"); err != nil {
		t.Fatal(err)
	}
	_ = armW.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
		if err != nil {
			t.Fatalf("probe should exit 0 after release: %v (stdout=%q)", err, out.String())
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("probe hung past release")
	}
	if got := out.String(); got != "reached-published\n" {
		t.Fatalf("probe output after release = %q, want %q", got, "reached-published\n")
	}
	if _, statErr := os.Stat(filepath.Join(base, "snapshot", "manifest.json")); statErr != nil {
		t.Fatalf("expected a published manifest after release, stat err = %v", statErr)
	}
}
