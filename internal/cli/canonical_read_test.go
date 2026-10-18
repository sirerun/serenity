package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/writer"
	"github.com/spf13/cobra"
)

func canonicalCLIIdentity() writer.MaintenanceIdentity {
	return writer.MaintenanceIdentity{Schema: "serenity.canonical-maintenance/v1", OperationID: strings.Repeat("a", 64), BrainID: "CanonicalBrain1234", SourceHead: strings.Repeat("b", 40), FenceDigest: strings.Repeat("c", 64)}
}
func canonicalCLIInitialize(t *testing.T, brain string) {
	t.Helper()
	owner, err := writer.AcquireBrain(brain)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestCanonicalCLIReadersRefusePendingBeforeConfiguration(t *testing.T) {
	brain := t.TempDir()
	canonicalCLIInitialize(t, brain)
	maintenance, err := writer.BeginBrainMaintenance(brain, canonicalCLIIdentity())
	if err != nil {
		t.Fatal(err)
	}
	if err := maintenance.Close(); err != nil {
		t.Fatal(err)
	}
	// No config exists: a reader reaching configuration would return a different
	// error. All actual CLI entry points must refuse at the persistent marker.
	for _, args := range [][]string{{"check", "--actions", "[]", "--json"}, {"doctor"}, {"status"}, {"search", "needle"}, {"ask", "question"}, {"report"}} {
		cmd := newRootCmd()
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetErr(&output)
		cmd.SetArgs(append([]string{"-C", brain}, args...))
		if err := cmd.Execute(); !errors.Is(err, writer.ErrMaintenancePending) {
			t.Errorf("%v bypassed pending gate: %v", args, err)
		}
		if output.Len() != 0 {
			t.Errorf("%v disclosed output before gate", args)
		}
	}
}
func TestCanonicalCLIReaderWrapperRetainsLeaseThroughoutRun(t *testing.T) {
	brain := t.TempDir()
	canonicalCLIInitialize(t, brain)
	root := &cobra.Command{Use: "serenity"}
	ran := false
	search := &cobra.Command{Use: "search", RunE: func(*cobra.Command, []string) error {
		ran = true
		maintenance, err := writer.BeginBrainMaintenance(brain, canonicalCLIIdentity())
		if err == nil {
			_ = maintenance.Close()
			t.Fatal("maintenance entered during canonical callback")
		}
		return nil
	}}
	root.AddCommand(search)
	installBrainOwnership(root)
	flagRoot = brain
	if err := search.RunE(search, nil); err != nil || !ran {
		t.Fatalf("reader callback: %v", err)
	}
	maintenance, err := writer.BeginBrainMaintenance(brain, canonicalCLIIdentity())
	if err != nil {
		t.Fatalf("read lease leaked: %v", err)
	}
	if err := maintenance.Finish(canonicalCLIIdentity()); err != nil {
		t.Fatal(err)
	}
	if err := maintenance.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestCanonicalCLIReaderMissingAndUnsafeStateNeverInitializes(t *testing.T) {
	for _, unsafe := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing", true: "symlink"}[unsafe], func(t *testing.T) {
			brain := t.TempDir()
			if err := config.Default().Save(filepath.Join(brain, config.FileName)); err != nil {
				t.Fatal(err)
			}
			if unsafe {
				outside := t.TempDir()
				if err := os.Symlink(outside, filepath.Join(brain, ".serenity")); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadDir(brain)
			if err != nil {
				t.Fatal(err)
			}
			cmd := newRootCmd()
			var output bytes.Buffer
			cmd.SetOut(&output)
			cmd.SetErr(&output)
			cmd.SetArgs([]string{"-C", brain, "doctor"})
			if err := cmd.Execute(); err == nil {
				t.Fatal("unqualified state accepted")
			}
			if output.Len() != 0 {
				t.Fatal("unqualified reader emitted canonical output")
			}
			after, err := os.ReadDir(brain)
			if err != nil || len(after) != len(before) {
				t.Fatal("reader initialized runtime")
			}
		})
	}
}

func TestMemoryToolsConfigAdmissionDoesNotBypassMaintenance(t *testing.T) {
	for _, absent := range []bool{false, true} {
		t.Run(map[bool]string{false: "malformed", true: "absent"}[absent], func(t *testing.T) {
			brain := t.TempDir()
			canonicalCLIInitialize(t, brain)
			if !absent {
				if err := os.WriteFile(filepath.Join(brain, config.FileName), []byte("private_canonical_key: secret\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			maintenance, err := writer.BeginBrainMaintenance(brain, canonicalCLIIdentity())
			if err != nil {
				t.Fatal(err)
			}
			if err = maintenance.Close(); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			_, closeFn, _, _, err := memoryTools(brain, &out)
			if !errors.Is(err, writer.ErrMaintenancePending) || closeFn != nil || out.Len() != 0 {
				t.Fatalf("unfenced config: close=%v err=%v output=%q", closeFn != nil, err, out.String())
			}
			if strings.Contains(err.Error(), "private_canonical_key") {
				t.Fatal("config decoded before ownership")
			}
		})
	}
}

func TestCanonicalHookUnrelatedEventNeedsNoBrain(t *testing.T) {
	brain := t.TempDir()
	cmd := newRootCmd()
	cmd.SetArgs([]string{"-C", brain, "check", "--claude-hook"})
	cmd.SetIn(strings.NewReader(`{"hook_event_name":"SessionStart"}`))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil || out.Len() != 0 {
		t.Fatalf("unrelated hook: %v %q", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(brain, ".serenity")); !os.IsNotExist(err) {
		t.Fatal("unrelated hook initialized state")
	}
	canonicalCLIInitialize(t, brain)
	maintenance, err := writer.BeginBrainMaintenance(brain, canonicalCLIIdentity())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = maintenance.Close() }()
	cmd = newRootCmd()
	cmd.SetArgs([]string{"-C", brain, "check", "--claude-hook"})
	cmd.SetIn(strings.NewReader(`{"hook_event_name":"PreToolUse","tool_name":"ExitPlanMode","tool_input":{"plan":"private"}}`))
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err = cmd.Execute()
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != 2 || !strings.Contains(out.String(), "canonical brain repo view unavailable") {
		t.Fatalf("hook maintenance refusal: %v %q", err, out.String())
	}
}
