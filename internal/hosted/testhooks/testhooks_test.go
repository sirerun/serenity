package testhooks_test

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	return buildFixture(t, "./testdata/probe", tag, false)
}

func buildFixture(t *testing.T, pkg string, tag, race bool) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), filepath.Base(pkg))
	args := []string{"build", "-o", out}
	if tag {
		args = append(args, "-tags", "hostedtest")
	}
	if race {
		args = append(args, "-race")
	}
	args = append(args, pkg)
	cmd := exec.Command("go", args...)
	cmd.Dir = "."
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build fixture %s (tag=%v, race=%v): %v: %s", pkg, tag, race, err, stderr.String())
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

// TestConcurrentPhasesReverseOrderReleaseAndUnrelatedCheckpoint is the
// regression this task's review requested: it proves, under the race
// detector, that (1) two concurrently paused phases release independently
// in reverse order (phase_b before phase_a, the opposite of program-text
// order) rather than one goroutine stealing and discarding the other's
// release line, and (2) a third, never-armed checkpoint proceeds
// immediately without waiting behind the two paused ones. Built with -race:
// against the prior shared-*bufio.Reader design this both deadlocked
// (phase_b's release line could be consumed and discarded by phase_a's own
// read loop) and reported a real data race.
func TestConcurrentPhasesReverseOrderReleaseAndUnrelatedCheckpoint(t *testing.T) {
	bin := buildFixture(t, "./testdata/concurrent", true, true)
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
	if _, err = armW.WriteString("arm phase_a pause\narm phase_b pause\nstart\n"); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin)
	cmd.ExtraFiles = []*os.File{armR, statusW}
	cmd.Env = append(os.Environ(), "SERENITY_HOSTED_TESTHOOKS_ARM_FD=3", "SERENITY_HOSTED_TESTHOOKS_STATUS_FD=4")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = statusW.Close()
	defer func() { _ = cmd.Process.Kill() }()

	lines := make(chan string, 8)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	nextLine := func(timeout time.Duration) (string, bool) {
		select {
		case l, ok := <-lines:
			return l, ok
		case <-time.After(timeout):
			return "", false
		}
	}

	// Wait for both phases to report paused (order between the two doesn't
	// matter — this only proves both are genuinely blocked concurrently).
	seen := map[string]bool{}
	statusSc := bufio.NewScanner(statusR)
	for len(seen) < 2 {
		if !statusSc.Scan() {
			t.Fatalf("status pipe closed before both phases paused, got %v: %v", seen, statusSc.Err())
		}
		line := statusSc.Text()
		if line == "paused phase_a" || line == "paused phase_b" {
			seen[line] = true
		}
	}

	// Prove the unrelated, never-armed checkpoint is not blocked behind the
	// two paused ones: its release line must arrive before any release
	// instruction has been sent at all.
	if l, ok := nextLine(5 * time.Second); !ok || l != "released phase_c" {
		t.Fatalf("first stdout line = %q ok=%v, want %q (unrelated checkpoint must not block)", l, ok, "released phase_c")
	}

	// Release phase_b first — the reverse of program-text order — and prove
	// only phase_b unblocks.
	if _, err = armW.WriteString("release phase_b\n"); err != nil {
		t.Fatal(err)
	}
	if l, ok := nextLine(5 * time.Second); !ok || l != "released phase_b" {
		t.Fatalf("stdout line after releasing phase_b = %q ok=%v, want %q", l, ok, "released phase_b")
	}
	// phase_a must still be blocked: no line should arrive yet.
	if l, ok := nextLine(300 * time.Millisecond); ok {
		t.Fatalf("phase_a released prematurely: got %q before its own release was sent", l)
	}

	if _, err = armW.WriteString("release phase_a\n"); err != nil {
		t.Fatal(err)
	}
	if l, ok := nextLine(5 * time.Second); !ok || l != "released phase_a" {
		t.Fatalf("stdout line after releasing phase_a = %q ok=%v, want %q", l, ok, "released phase_a")
	}
	if l, ok := nextLine(5 * time.Second); !ok || l != "done" {
		t.Fatalf("final stdout line = %q ok=%v, want %q", l, ok, "done")
	}
	_ = armW.Close()
	if err = cmd.Wait(); err != nil {
		t.Fatalf("probe should exit 0: %v (stderr=%q)", err, stderr.String())
	}
	if race := stderr.String(); strings.Contains(race, "DATA RACE") {
		t.Fatalf("race detector reported a data race:\n%s", race)
	}
}

// TestTaggedBinaryPauseUnblocksOnArmPipeEOF proves the documented EOF
// behavior: closing the harness's write end while a phase is paused
// unblocks it immediately rather than hanging forever.
func TestTaggedBinaryPauseUnblocksOnArmPipeEOF(t *testing.T) {
	bin := buildProbe(t, true)
	armR, armW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = armR.Close() }()
	if _, err = armW.WriteString("arm proof_phase pause\nstart\n"); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin)
	cmd.ExtraFiles = []*os.File{armR}
	cmd.Env = append(os.Environ(), "SERENITY_HOSTED_TESTHOOKS_ARM_FD=3")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Close the write end without ever sending "release proof_phase": the
	// probe must not hang.
	_ = armW.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
		if err != nil {
			t.Fatalf("probe should exit 0 after arm-pipe EOF: %v (stdout=%q)", err, out.String())
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("probe hung past arm-pipe EOF instead of unblocking")
	}
	if got := out.String(); got != "reached\n" {
		t.Fatalf("probe output after EOF = %q, want %q", got, "reached\n")
	}
}
