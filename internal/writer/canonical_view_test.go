//go:build darwin || linux

package writer

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCanonicalViewSharedReadersOrdinaryWriterAndMaintenance(t *testing.T) {
	root := maintenanceTestRoot(t)
	reader, err := AcquireCanonicalRead(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	second, err := AcquireCanonicalRead(root)
	if err != nil {
		t.Fatal(err)
	}
	if err = second.Close(); err != nil {
		t.Fatal(err)
	}
	// Canonical view locks exclude maintenance, not ordinary writer snapshots.
	owner, err := AcquireBrain(root)
	if err != nil {
		t.Fatal("reader blocked ordinary writer", err)
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
	if m, err := BeginBrainMaintenance(root, maintenanceFixtureID()); m != nil || !errors.Is(err, ErrCanonicalViewUnavailable) {
		if m != nil {
			_ = m.Close()
		}
		t.Fatalf("reader did not exclude maintenance: %v", err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	m, err := BeginBrainMaintenance(root, maintenanceFixtureID())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	if r, err := AcquireCanonicalRead(root); r != nil || !errors.Is(err, ErrCanonicalViewUnavailable) {
		if r != nil {
			_ = r.Close()
		}
		t.Fatalf("maintenance admitted read: %v", err)
	}
	if err = m.Finish(maintenanceFixtureID()); err != nil {
		t.Fatal(err)
	}
	// Finish clears only the marker. Its owner still holds exclusive view and
	// writer locks until explicit Close, including between publication phases.
	if r, err := AcquireCanonicalRead(root); r != nil || !errors.Is(err, ErrCanonicalViewUnavailable) {
		if r != nil {
			_ = r.Close()
		}
		t.Fatalf("Finish prematurely released view: %v", err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	r, err := AcquireCanonicalRead(root)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalViewMissingUnsafeAndPendingStateNeverInitializes(t *testing.T) {
	for _, scenario := range []string{"missing_state", "missing_lock", "symlink_state", "symlink_lock", "directory_lock", "mode_lock", "nonempty_lock", "hardlink_lock", "pending_marker"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			state := filepath.Join(root, ".serenity")
			lock := filepath.Join(state, canonicalViewLockName)
			if scenario != "missing_state" {
				if err := os.Mkdir(state, 0700); err != nil {
					t.Fatal(err)
				}
				if scenario != "missing_lock" && scenario != "symlink_state" {
					if err := os.WriteFile(lock, nil, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			switch scenario {
			case "symlink_state":
				if err := os.Remove(state); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), state); err != nil {
					t.Fatal(err)
				}
			case "symlink_lock":
				if err := os.Remove(lock); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, "absent"), lock); err != nil {
					t.Fatal(err)
				}
			case "directory_lock":
				if err := os.Remove(lock); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(lock, 0700); err != nil {
					t.Fatal(err)
				}
			case "mode_lock":
				if err := os.Chmod(lock, 0644); err != nil {
					t.Fatal(err)
				}
			case "nonempty_lock":
				if err := os.WriteFile(lock, []byte("unsafe content"), 0600); err != nil {
					t.Fatal(err)
				}
			case "hardlink_lock":
				if err := os.Link(lock, filepath.Join(root, "alias-lock")); err != nil {
					t.Fatal(err)
				}
			case "pending_marker":
				if err := os.WriteFile(markerPath(root), []byte("partial marker"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before := canonicalViewTree(t, root)
			read, err := AcquireCanonicalRead(root)
			if read != nil || !errors.Is(err, ErrCanonicalViewUnavailable) {
				if read != nil {
					_ = read.Close()
				}
				t.Fatalf("unsafe view accepted: %v", err)
			}
			if scenario == "pending_marker" && !errors.Is(err, ErrMaintenancePending) {
				t.Fatal("pending marker not identified", err)
			}
			if after := canonicalViewTree(t, root); after != before {
				t.Fatalf("refused read mutated files: before=%q after=%q", before, after)
			}
		})
	}
}
func canonicalViewTree(t *testing.T, root string) string {
	t.Helper()
	var rows []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rows = append(rows, fmt.Sprintf("%s %v %d", rel, info.Mode(), info.Size()))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(rows, "\n")
}

func TestCanonicalViewFailuresReleaseBothLocks(t *testing.T) {
	root := maintenanceTestRoot(t)
	owner, err := AcquireBrain(root)
	if err != nil {
		t.Fatal(err)
	}
	if m, err := BeginBrainMaintenance(root, maintenanceFixtureID()); m != nil || !errors.Is(err, ErrBrainOwned) {
		if m != nil {
			_ = m.Close()
		}
		t.Fatalf("maintenance bypassed writer: %v", err)
	}
	// A failed maintenance writer acquisition must release its earlier EX view.
	reader, err := AcquireCanonicalRead(root)
	if err != nil {
		t.Fatal("failed Begin leaked view lock", err)
	}
	if err = reader.Close(); err != nil {
		t.Fatal(err)
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
	m, err := BeginBrainMaintenance(root, maintenanceFixtureID())
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	wrong := maintenanceFixtureID()
	wrong.OperationID = "wrong-operation"
	if resumed, err := ResumeBrainMaintenance(root, wrong); resumed != nil || !errors.Is(err, ErrMaintenanceIdentity) {
		if resumed != nil {
			_ = resumed.Close()
		}
		t.Fatalf("wrong resume: %v", err)
	}
	// Correct retry succeeds only if both locks were released on wrong identity.
	m, err = ResumeBrainMaintenance(root, maintenanceFixtureID())
	if err != nil {
		t.Fatal("failed resume leaked locks", err)
	}
	defer func() { _ = m.Close() }()
	if err = m.Finish(maintenanceFixtureID()); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalViewCrossProcessExclusionAndDeath(t *testing.T) {
	if root := os.Getenv("SERENITY_CANONICAL_VIEW_PROCESS_ROOT"); root != "" {
		switch os.Getenv("SERENITY_CANONICAL_VIEW_PROCESS_ACTION") {
		case "reader_hold":
			reader, err := AcquireCanonicalRead(root)
			if err != nil {
				os.Exit(2)
			}
			_, _ = fmt.Fprintln(os.Stdout, "held")
			_, _ = io.Copy(io.Discard, os.Stdin)
			_ = reader.Close()
			os.Exit(0)
		case "reader_refuse":
			reader, err := AcquireCanonicalRead(root)
			if reader != nil {
				_ = reader.Close()
				os.Exit(3)
			}
			if !errors.Is(err, ErrCanonicalViewUnavailable) {
				os.Exit(4)
			}
			os.Exit(0)
		case "maintenance_exit":
			if _, err := BeginBrainMaintenance(root, maintenanceFixtureID()); err != nil {
				os.Exit(5)
			}
			os.Exit(0)
		default:
			os.Exit(6)
		}
	}
	root := maintenanceTestRoot(t)
	newCommand := func(ctx context.Context, action string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCanonicalViewCrossProcessExclusionAndDeath$")
		cmd.Env = append(os.Environ(), "SERENITY_CANONICAL_VIEW_PROCESS_ROOT="+root, "SERENITY_CANONICAL_VIEW_PROCESS_ACTION="+action)
		return cmd
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := newCommand(ctx, "reader_hold")
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close(); _ = cmd.Wait() }()
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || line != "held\n" {
		t.Fatalf("reader child not ready: %q %v", line, err)
	}
	if m, err := BeginBrainMaintenance(root, maintenanceFixtureID()); m != nil || !errors.Is(err, ErrCanonicalViewUnavailable) {
		if m != nil {
			_ = m.Close()
		}
		t.Fatalf("cross-process reader failed exclusion: %v", err)
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	m, err := BeginBrainMaintenance(root, maintenanceFixtureID())
	if err != nil {
		t.Fatal("reader death retained lock", err)
	}
	refused := newCommand(ctx, "reader_refuse")
	if out, err := refused.CombinedOutput(); err != nil {
		t.Fatalf("exclusive maintenance admitted childread: %v %s", err, out)
	}
	if err = m.Finish(maintenanceFixtureID()); err != nil {
		t.Fatal(err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	died := newCommand(ctx, "maintenance_exit")
	if out, err := died.CombinedOutput(); err != nil {
		t.Fatalf("maintenance child: %v %s", err, out)
	}
	// Process death releases EX locks, but durable marker still refuses readers.
	if reader, err := AcquireCanonicalRead(root); reader != nil || !errors.Is(err, ErrMaintenancePending) {
		if reader != nil {
			_ = reader.Close()
		}
		t.Fatalf("death lost marker gate: %v", err)
	}
	m, err = ResumeBrainMaintenance(root, maintenanceFixtureID())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	if err = m.Finish(maintenanceFixtureID()); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalMaintenanceMissingLegacyViewRefusesWithoutMutation(t *testing.T) {
	root := t.TempDir()
	before := canonicalViewTree(t, root)
	if m, err := BeginBrainMaintenance(root, maintenanceFixtureID()); m != nil || !errors.Is(err, ErrCanonicalViewUnavailable) {
		if m != nil {
			_ = m.Close()
		}
		t.Fatalf("missing legacy view admitted maintenance: %v", err)
	}
	if m, err := ResumeBrainMaintenance(root, maintenanceFixtureID()); m != nil || !errors.Is(err, ErrCanonicalViewUnavailable) {
		if m != nil {
			_ = m.Close()
		}
		t.Fatalf("missing legacy view admitted resume: %v", err)
	}
	if after := canonicalViewTree(t, root); after != before {
		t.Fatal("maintenance initialized missing view state")
	}
	owner, err := AcquireBrain(root)
	if err != nil {
		t.Fatal("ordinary owner could not initialize view", err)
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
	m, err := BeginBrainMaintenance(root, maintenanceFixtureID())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	if err = m.Finish(maintenanceFixtureID()); err != nil {
		t.Fatal(err)
	}
}
