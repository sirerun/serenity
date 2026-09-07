package cli

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/cron"
)

// TestCronSweepCLIExitsZeroAndIsIdempotent exercises T2.19's own acc line
// through the built binary (not runCron() in-process): `serenity cron
// sweep` on a fixture exits 0, and a second run is idempotent — no error,
// no duplicate side effect beyond an incremented run count.
func TestCronSweepCLIExitsZeroAndIsIdempotent(t *testing.T) {
	requireGit(t)
	bin := buildSerenityBinary(t)
	root := t.TempDir()

	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatalf("init: %v\n%s", err, initOut.String())
	}

	run := func() string {
		t.Helper()
		cmd := exec.Command(bin, "-C", root, "cron", "sweep")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("cron sweep: %v; output: %s", err, out)
		}
		return string(out)
	}

	first := run()
	if !strings.Contains(first, "sweep") {
		t.Fatalf("expected output to mention the job name, got: %s", first)
	}

	second := run()
	if !strings.Contains(second, "sweep") {
		t.Fatalf("expected output to mention the job name, got: %s", second)
	}

	rec, err := cron.ReadRecord(root, "sweep")
	if err != nil {
		t.Fatalf("read run record: %v", err)
	}
	if rec.RunCount != 2 {
		t.Fatalf("run count = %d, want 2 after two CLI invocations", rec.RunCount)
	}
}

// TestCronUnknownJobCLIExitsNonZero: an unregistered job name is a CLI
// error naming every valid job, not a silent success.
func TestCronUnknownJobCLIExitsNonZero(t *testing.T) {
	requireGit(t)
	bin := buildSerenityBinary(t)
	root := t.TempDir()

	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatalf("init: %v\n%s", err, initOut.String())
	}

	cmd := exec.Command(bin, "-C", root, "cron", "bogus")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected a non-zero exit for an unknown job, got success: %s", out)
	}
	for _, name := range cron.Names() {
		if !strings.Contains(string(out), name) {
			t.Fatalf("expected error output to name valid job %q, got: %s", name, out)
		}
	}
}

// TestCronRequiresExactlyOneArg: no job name and too many job names are
// both usage errors, not a silent default job.
func TestCronRequiresExactlyOneArg(t *testing.T) {
	requireGit(t)
	bin := buildSerenityBinary(t)
	root := t.TempDir()

	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatalf("init: %v\n%s", err, initOut.String())
	}

	for _, args := range [][]string{
		{"-C", root, "cron"},
		{"-C", root, "cron", "sweep", "decay"},
	} {
		cmd := exec.Command(bin, args...)
		if out, err := cmd.CombinedOutput(); err == nil {
			t.Fatalf("args %v: expected a non-zero exit, got success: %s", args, out)
		}
	}
}

// TestCronRecordPathUnderRuntimeState confirms the run record lands under
// .serenity/ (runtime state, never canonical/committed — RFC §7.5), not
// under brain/ or any tracked path.
func TestCronRecordPathUnderRuntimeState(t *testing.T) {
	got := cron.RecordPath("/root", "sweep")
	want := filepath.Join("/root", ".serenity", "cron", "sweep.json")
	if got != want {
		t.Fatalf("RecordPath = %q, want %q", got, want)
	}
}
