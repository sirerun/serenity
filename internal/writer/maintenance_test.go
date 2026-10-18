//go:build darwin || linux

package writer

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func maintenanceFixtureID() MaintenanceIdentity {
	return MaintenanceIdentity{Schema: maintenanceSchema, OperationID: "operation-1", BrainID: "0123456789abcdef", SourceHead: strings.Repeat("a", 40), FenceDigest: strings.Repeat("b", 64)}
}
func markerPath(root string) string { return filepath.Join(root, ".serenity", maintenanceMarkerName) }

func TestMaintenancePersistsAndReconcilesExactOwner(t *testing.T) {
	root := t.TempDir()
	original, err := AcquireBrain(root)
	if err != nil {
		t.Fatal(err)
	}
	lockInfo, err := original.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err = original.Close(); err != nil {
		t.Fatal(err)
	}
	id := maintenanceFixtureID()
	m, err := BeginBrainMaintenance(root, id)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if other, err := AcquireBrain(root); other != nil || !errors.Is(err, ErrBrainOwned) {
		t.Fatalf("competing writer: %v", err)
	}
	if other, err := ResumeBrainMaintenance(root, id); other != nil || !errors.Is(err, ErrBrainOwned) {
		t.Fatalf("competing maintenance: %v", err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	if other, err := AcquireBrain(root); other != nil || !errors.Is(err, ErrMaintenancePending) {
		t.Fatalf("persistent marker admitted writer: %v", err)
	}
	if other, err := BeginBrainMaintenance(root, id); other != nil || !errors.Is(err, ErrMaintenancePending) {
		t.Fatalf("begin adopted marker: %v", err)
	}
	wrong := id
	wrong.OperationID = "other-operation"
	if other, err := ResumeBrainMaintenance(root, wrong); other != nil || !errors.Is(err, ErrMaintenanceIdentity) {
		t.Fatalf("resume wrong operation: %v", err)
	}
	m, err = ResumeBrainMaintenance(root, id)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err = m.Finish(wrong); !errors.Is(err, ErrMaintenanceIdentity) {
		t.Fatal("wrong finish accepted")
	}
	if err = m.Finish(id); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Lstat(markerPath(root)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("finish left marker")
	}
	if other, err := AcquireBrain(root); other != nil || !errors.Is(err, ErrBrainOwned) {
		t.Fatalf("finish released ownership: %v", err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	writer, err := AcquireBrain(root)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	current, err := writer.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(lockInfo, current) {
		t.Fatal("stable lock inode replaced")
	}
}

func TestMaintenancePartialUnsafeAndReplacedMarkersFailClosed(t *testing.T) {
	for _, scenario := range []string{"partial", "symlink", "directory", "mode", "unknown_field", "duplicate_key", "oversized", "replacement", "lock_replacement"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			id := maintenanceFixtureID()
			m, err := BeginBrainMaintenance(root, id)
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			path := markerPath(root)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "replacement" {
				if err = os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				if err = m.Finish(id); !errors.Is(err, ErrMaintenanceIdentity) {
					t.Fatalf("replacement accepted: %v", err)
				}
				got, err := os.ReadFile(path)
				if err != nil || string(got) != string(raw) {
					t.Fatal("finish modified replacement")
				}
				return
			}
			if scenario == "lock_replacement" {
				lockPath := filepath.Join(root, ".serenity", "writer.lock")
				if err = os.Rename(lockPath, lockPath+".original"); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(lockPath, nil, 0600); err != nil {
					t.Fatal(err)
				}
				if err = m.Finish(id); !errors.Is(err, ErrMaintenancePending) {
					t.Fatalf("lock replacement accepted: %v", err)
				}
				return
			}
			if err = m.Close(); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "partial":
				err = os.WriteFile(path, []byte(`{"schema":`), 0600)
			case "symlink":
				if err = os.Remove(path); err == nil {
					err = os.Symlink(filepath.Join(root, "nonexistent"), path)
				}
			case "directory":
				if err = os.Remove(path); err == nil {
					err = os.Mkdir(path, 0700)
				}
			case "mode":
				err = os.Chmod(path, 0644)
			case "unknown_field":
				err = os.WriteFile(path, []byte(strings.TrimSpace(string(raw[:len(raw)-2]))+`,"unexpected":"field"}`), 0600)
			case "duplicate_key":
				err = os.WriteFile(path, []byte(strings.TrimSpace(string(raw[:len(raw)-2]))+`,"schema":"`+maintenanceSchema+`"}`), 0600)
			case "oversized":
				err = os.WriteFile(path, []byte(strings.Repeat("x", maintenanceMaxBytes+1)), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if writer, err := AcquireBrain(root); writer != nil || !errors.Is(err, ErrMaintenancePending) {
				t.Fatalf("unsafe marker admitted: %v", err)
			}
			resumed, err := ResumeBrainMaintenance(root, id)
			if resumed != nil || err == nil {
				if resumed != nil {
					resumed.Close()
				}
				t.Fatalf("unsafe resume: %v", err)
			}
			// A refused ordinary acquisition must release the lock for reconciliation.
			owner, state, err := acquireBrainLock(root)
			if err != nil {
				t.Fatal("refusal leaked writer lock", err)
			}
			if err = errors.Join(owner.Close(), state.Close()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMaintenanceIdentityValidationHasNoEffects(t *testing.T) {
	for _, scenario := range []string{"schema", "operation", "brain", "head", "fence"} {
		t.Run(scenario, func(t *testing.T) {
			id := maintenanceFixtureID()
			switch scenario {
			case "schema":
				id.Schema = "v0"
			case "operation":
				id.OperationID = "../unsafe"
			case "brain":
				id.BrainID = "bad"
			case "head":
				id.SourceHead = strings.Repeat("A", 40)
			case "fence":
				id.FenceDigest = "bad"
			}
			root := t.TempDir()
			if m, err := BeginBrainMaintenance(root, id); m != nil || !errors.Is(err, ErrMaintenanceIdentity) {
				t.Fatalf("invalid accepted: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, ".serenity")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("invalid input created state")
			}
		})
	}
}

func TestMaintenanceProcessExitRetainsGate(t *testing.T) {
	if root := os.Getenv("SERENITY_MAINTENANCE_PROCESS_ROOT"); root != "" {
		switch os.Getenv("SERENITY_MAINTENANCE_PROCESS_ACTION") {
		case "begin_exit":
			if _, err := BeginBrainMaintenance(root, maintenanceFixtureID()); err != nil {
				os.Exit(2)
			}
			// Deliberately skip Close: process exit releases the flock but must leave
			// the durable marker. This is the same lock-release behavior as a crash.
			os.Exit(0)
		case "ordinary_refusal":
			owner, err := AcquireBrain(root)
			if owner != nil {
				_ = owner.Close()
				os.Exit(3)
			}
			if !errors.Is(err, ErrMaintenancePending) {
				os.Exit(4)
			}
			os.Exit(0)
		default:
			os.Exit(5)
		}
	}
	root := t.TempDir()
	run := func(action string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMaintenanceProcessExitRetainsGate$")
		cmd.Env = append(os.Environ(), "SERENITY_MAINTENANCE_PROCESS_ROOT="+root, "SERENITY_MAINTENANCE_PROCESS_ACTION="+action)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("maintenance process %s: %v: %s", action, err, out)
		}
	}
	run("begin_exit")
	run("ordinary_refusal")
	if owner, err := AcquireBrain(root); owner != nil || !errors.Is(err, ErrMaintenancePending) {
		t.Fatalf("process exit removed gate: %v", err)
	}
	m, err := ResumeBrainMaintenance(root, maintenanceFixtureID())
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	// A distinct process still refuses after a clean reconciliation-owner Close.
	run("ordinary_refusal")
	m, err = ResumeBrainMaintenance(root, maintenanceFixtureID())
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err = m.Finish(maintenanceFixtureID()); err != nil {
		t.Fatal(err)
	}
}

func TestMaintenanceDurabilityFailuresNeverReturnUsableOwner(t *testing.T) {
	failure := errors.New("injected durability failure")
	for _, action := range []string{"begin", "resume"} {
		for _, boundary := range []string{"brain_root", "marker_file", "runtime_directory"} {
			t.Run(action+"_"+boundary, func(t *testing.T) {
				root := t.TempDir()
				id := maintenanceFixtureID()
				if action == "resume" {
					m, err := BeginBrainMaintenance(root, id)
					if err != nil {
						t.Fatal(err)
					}
					if err = m.Close(); err != nil {
						t.Fatal(err)
					}
				}
				directoryCalls := 0
				syncs := maintenanceSyncs{
					file: func(file *os.File) error {
						if boundary == "marker_file" {
							return failure
						}
						return file.Sync()
					},
					directory: func(dir *os.Root) error {
						directoryCalls++
						if boundary == "brain_root" && directoryCalls == 1 || boundary == "runtime_directory" && directoryCalls == 2 {
							return failure
						}
						return maintenanceSync(dir)
					},
				}
				var m *BrainMaintenance
				var err error
				if action == "begin" {
					m, err = beginBrainMaintenance(root, id, syncs)
				} else {
					m, err = resumeBrainMaintenance(root, id, syncs)
				}
				if m != nil || !errors.Is(err, failure) {
					if m != nil {
						_ = m.Close()
					}
					t.Fatalf("durability failure returned usable owner: %v", err)
				}
				// Root-entry sync failure precedes first marker creation. Every failure
				// after publication, and every Resume failure, must retain the gate.
				markerExists := action == "resume" || boundary != "brain_root"
				if markerExists {
					if writer, err := AcquireBrain(root); writer != nil || !errors.Is(err, ErrMaintenancePending) {
						if writer != nil {
							_ = writer.Close()
						}
						t.Fatalf("failed durability unblocked writers: %v", err)
					}
					m, err = ResumeBrainMaintenance(root, id)
				} else {
					if _, err := os.Lstat(markerPath(root)); !errors.Is(err, os.ErrNotExist) {
						t.Fatal("root failure published marker")
					}
					m, err = BeginBrainMaintenance(root, id)
				}
				if err != nil {
					t.Fatal("durable retry could not acquire owner", err)
				}
				defer m.Close()
				if err = m.Finish(id); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
