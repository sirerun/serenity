package testhooks_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/testhooks"
)

// TestAtIsNoopInThisBuild proves that in the ordinary build under test here
// (no hostedtest tag), calling At never blocks and never exits the process.
func TestAtIsNoopInThisBuild(t *testing.T) {
	done := make(chan struct{})
	go func() {
		testhooks.At(testhooks.PhaseOperationReserved)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("At blocked in a non-hostedtest build")
	}
}

func buildProbe(t *testing.T, tag bool) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "probe")
	args := []string{"build", "-o", out}
	if tag {
		args = append(args, "-tags", "hostedtest")
	}
	args = append(args, "./testdata/probe")
	cmd := exec.Command("go", args...)
	cmd.Dir = "."
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build probe (tag=%v): %v: %s", tag, err, stderr.String())
	}
	return out
}

// TestUntaggedBinaryIgnoresControlPipe proves the fault barrier does not
// exist at all in a binary built without -tags hostedtest, even when an env
// var names a real inherited pipe armed to crash.
func TestUntaggedBinaryIgnoresControlPipe(t *testing.T) {
	bin := buildProbe(t, false)
	armR, armW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = armR.Close() }()
	if _, err = armW.WriteString("arm proof_phase crash\nstart\n"); err != nil {
		t.Fatal(err)
	}
	_ = armW.Close()
	cmd := exec.Command(bin)
	cmd.ExtraFiles = []*os.File{armR}
	cmd.Env = append(os.Environ(), "SERENITY_HOSTED_TESTHOOKS_ARM_FD=3")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err = cmd.Run(); err != nil {
		t.Fatalf("untagged probe should exit 0 and ignore the armed crash: %v (stdout=%q)", err, out.String())
	}
	if got := out.String(); got != "reached\n" {
		t.Fatalf("untagged probe output = %q, want %q", got, "reached\n")
	}
}

// TestTaggedBinaryWithoutPipeIsInert proves that even a hostedtest-tagged
// binary is inert by default: without an inherited control pipe, the env
// var alone activates nothing.
func TestTaggedBinaryWithoutPipeIsInert(t *testing.T) {
	bin := buildProbe(t, true)
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "SERENITY_HOSTED_TESTHOOKS_ARM_FD=99")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("tagged probe without a real pipe should still exit 0: %v (stdout=%q)", err, out.String())
	}
	if got := out.String(); got != "reached\n" {
		t.Fatalf("tagged probe output = %q, want %q", got, "reached\n")
	}
}

// TestTaggedBinaryArmedCrashExitsAtCheckpoint proves the fault barrier
// activates only when both the hostedtest tag and a real armed pipe are
// present, and exits with the documented sentinel code.
func TestTaggedBinaryArmedCrashExitsAtCheckpoint(t *testing.T) {
	bin := buildProbe(t, true)
	armR, armW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = armR.Close() }()
	if _, err = armW.WriteString("arm proof_phase crash\nstart\n"); err != nil {
		t.Fatal(err)
	}
	_ = armW.Close()
	cmd := exec.Command(bin)
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
		t.Fatalf("armed-crash probe should never print, got %q", got)
	}
}

// TestTaggedBinaryPauseReleaseRoundTrip proves the pause instruction blocks
// at the named checkpoint until the harness releases it, using the status
// pipe to observe the pause without a fixed sleep.
func TestTaggedBinaryPauseReleaseRoundTrip(t *testing.T) {
	bin := buildProbe(t, true)
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
	if _, err = armW.WriteString("arm proof_phase pause\nstart\n"); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin)
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
	if err != nil || string(statusBuf[:n]) != "paused proof_phase\n" {
		_ = cmd.Process.Kill()
		t.Fatalf("expected pause status, got %q err=%v", string(statusBuf[:n]), err)
	}
	if _, err = armW.WriteString("release proof_phase\n"); err != nil {
		t.Fatal(err)
	}
	_ = armW.Close()
	if err = cmd.Wait(); err != nil {
		t.Fatalf("probe should exit 0 after release: %v (stdout=%q)", err, out.String())
	}
	if got := out.String(); got != "reached\n" {
		t.Fatalf("probe output after release = %q, want %q", got, "reached\n")
	}
}
