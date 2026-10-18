//go:build darwin || linux

package writer

import (
	"bytes"
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

func initialTestRoot(t *testing.T) string {
	t.Helper()
	return maintenanceTestRoot(t)
}
func initialWritePrefix(t *testing.T, root string, raw []byte) {
	t.Helper()
	if err := os.WriteFile(markerPath(root), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(markerPath(root), 0600); err != nil {
		t.Fatal(err)
	}
}
func initialAllow(context.Context, InitialMaintenanceObservation) error { return nil }
func initialAssertBlocked(t *testing.T, root string) {
	t.Helper()
	if f, err := AcquireBrain(root); f != nil || err == nil {
		if f != nil {
			_ = f.Close()
		}
		t.Fatalf("writer admitted: %v", err)
	}
	if f, err := AcquireCanonicalRead(root); f != nil || err == nil {
		if f != nil {
			_ = f.Close()
		}
		t.Fatalf("reader admitted: %v", err)
	}
}

func TestInitialMaintenanceEveryRepresentativePrefix(t *testing.T) {
	id := maintenanceFixtureID()
	raw, err := maintenanceBytes(id)
	if err != nil {
		t.Fatal(err)
	}
	for _, cut := range []int{0, 1, 17, 64, len(raw) / 2, len(raw) - 1, len(raw)} {
		t.Run(fmt.Sprint(cut), func(t *testing.T) {
			root := initialTestRoot(t)
			initialWritePrefix(t, root, raw[:cut])
			before, _ := os.Stat(markerPath(root))
			p, err := OpenInitialBrainMaintenance(root, id)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = p.Close() }()
			obs, err := p.Observation()
			if err != nil || !obs.Present || obs.Bytes != int64(cut) || !bytes.Equal(obs.Prefix, raw[:cut]) {
				t.Fatalf("observation: %#v %v", obs, err)
			}
			if len(obs.Prefix) > 0 {
				obs.Prefix[0] ^= 1
			}
			if err = p.ValidateFor(root, id); err != nil {
				t.Fatal(err)
			}
			initialAssertBlocked(t, root)
			calls := 0
			m, err := p.Complete(context.Background(), func(_ context.Context, current InitialMaintenanceObservation) error {
				calls++
				want := cut
				if calls == 2 {
					want = len(raw)
				}
				if current.Bytes != int64(want) || !bytes.Equal(current.Prefix, raw[:want]) {
					t.Fatalf("callback%d: %#v", calls, current)
				}
				if len(current.Prefix) > 0 {
					current.Prefix[0] ^= 1
				}
				return nil
			})
			if err != nil || m == nil || calls != 2 {
				t.Fatalf("complete owner=%v calls=%d err=%v", m, calls, err)
			}
			defer func() { _ = m.Close() }()
			after, _ := os.Stat(markerPath(root))
			if !os.SameFile(before, after) {
				t.Fatal("marker inode replaced")
			}
			if err = p.Close(); err != nil {
				t.Fatal(err)
			}
			if err = m.ValidateFor(root, id); err != nil {
				t.Fatal("restricted close closed transferred owner", err)
			}
			if next, err := p.Complete(context.Background(), initialAllow); next != nil || err == nil {
				t.Fatal("transferred twice")
			}
			if _, err = p.Observation(); err == nil {
				t.Fatal("observation after transfer")
			}
			if err = m.Finish(id); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInitialMaintenanceAbsentGateIsRestrictedAndDurable(t *testing.T) {
	root := initialTestRoot(t)
	id := maintenanceFixtureID()
	lockBefore, _ := os.Stat(filepath.Join(root, ".serenity", "writer.lock"))
	p, err := OpenInitialBrainMaintenance(root, id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	obs, err := p.Observation()
	if err != nil || !sameInitialObservation(obs, InitialMaintenanceObservation{}) {
		t.Fatalf("absent: %#v %v", obs, err)
	}
	if _, err = os.Lstat(markerPath(root)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Open created gate")
	}
	calls := 0
	if err = p.EnsureGate(context.Background(), func(_ context.Context, o InitialMaintenanceObservation) error {
		calls++
		if o.Present {
			t.Fatal("predicted inode")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("ensure calls=%d", calls)
	}
	obs, err = p.Observation()
	if err != nil || !obs.Present || obs.Bytes != 0 || obs.Inode == 0 || obs.Links != 1 || obs.Mode != 0600 || obs.UID != uint32(os.Geteuid()) {
		t.Fatalf("created: %#v %v", obs, err)
	}
	initialAssertBlocked(t, root)
	m, err := p.Complete(context.Background(), initialAllow)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	lockAfter, _ := os.Stat(filepath.Join(root, ".serenity", "writer.lock"))
	if !os.SameFile(lockBefore, lockAfter) {
		t.Fatal("stable lock replaced")
	}
}

func TestInitialMaintenanceAdmissionFailuresCloseAndRetain(t *testing.T) {
	for _, which := range []string{"nil", "cancelled", "first", "second", "mutated"} {
		t.Run(which, func(t *testing.T) {
			root := initialTestRoot(t)
			id := maintenanceFixtureID()
			raw, _ := maintenanceBytes(id)
			initialWritePrefix(t, root, raw[:17])
			before, _ := os.Stat(markerPath(root))
			p, err := OpenInitialBrainMaintenance(root, id)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			calls := 0
			verifier := InitialMaintenanceVerifier(func(_ context.Context, _ InitialMaintenanceObservation) error {
				calls++
				if which == "first" && calls == 1 || which == "second" && calls == 2 {
					return errors.New("admission refused")
				}
				if which == "mutated" && calls == 1 {
					if err := os.Chmod(markerPath(root), 0400); err != nil {
						t.Fatal(err)
					}
				}
				return nil
			})
			if which == "nil" {
				verifier = nil
			}
			if which == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			m, err := p.Complete(ctx, verifier)
			if m != nil || err == nil {
				t.Fatalf("admitted: %v %v", m, err)
			}
			if _, err = p.Observation(); err == nil {
				t.Fatal("failed mutation retained usable owner")
			}
			after, _ := os.Stat(markerPath(root))
			if !os.SameFile(before, after) {
				t.Fatal("marker lost/replaced")
			}
			initialAssertBlocked(t, root)
		})
	}
}

func TestInitialMaintenanceFaultsPreserveSameGateAndRetry(t *testing.T) {
	points := []string{"marker-appended", "marker-file-synced", "marker-runtime-synced", "marker-root-synced", "short-write", "file-sync", "runtime-sync", "root-sync"}
	for _, point := range points {
		t.Run(point, func(t *testing.T) {
			root := initialTestRoot(t)
			id := maintenanceFixtureID()
			raw, _ := maintenanceBytes(id)
			initialWritePrefix(t, root, raw[:1])
			before, _ := os.Stat(markerPath(root))
			hooks := durableInitialMaintenanceHooks()
			armed := false
			hooks.checkpoint = func(p string) error {
				if p == point {
					return errors.New("fault")
				}
				return nil
			}
			if point == "short-write" {
				hooks.write = func(f *os.File, p []byte) (int, error) {
					n, err := f.Write(p[:3])
					if err != nil {
						return n, err
					}
					return n, io.ErrShortWrite
				}
			}
			hooks.syncs.file = func(f *os.File) error {
				if armed && point == "file-sync" {
					return errors.New("fault")
				}
				return f.Sync()
			}
			hooks.syncs.directory = func(r *os.Root) error {
				if armed && (point == "runtime-sync" && filepath.Base(r.Name()) == ".serenity" || point == "root-sync" && r.Name() == root) {
					return errors.New("fault")
				}
				return maintenanceSync(r)
			}
			p, err := openInitialBrainMaintenance(root, id, hooks)
			if err != nil {
				t.Fatal(err)
			}
			armed = true
			m, err := p.Complete(context.Background(), initialAllow)
			if m != nil || err == nil {
				t.Fatalf("fault admitted: %v", err)
			}
			after, _ := os.Stat(markerPath(root))
			if !os.SameFile(before, after) {
				t.Fatal("gate replaced")
			}
			initialAssertBlocked(t, root)
			p, err = OpenInitialBrainMaintenance(root, id)
			if err != nil {
				t.Fatal(err)
			}
			m, err = p.Complete(context.Background(), initialAllow)
			if err != nil {
				t.Fatal(err)
			}
			if err = m.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestInitialMaintenanceEnsureFailureNeverCreatesRecoveryEvidence(t *testing.T) {
	for _, point := range []string{"gate-created", "gate-file-synced", "gate-runtime-synced", "gate-root-synced", "file-sync", "runtime-sync", "root-sync", "create"} {
		t.Run(point, func(t *testing.T) {
			root := initialTestRoot(t)
			id := maintenanceFixtureID()
			hooks := durableInitialMaintenanceHooks()
			armed := false
			hooks.checkpoint = func(p string) error {
				if p == point {
					return errors.New("fault")
				}
				return nil
			}
			hooks.syncs.file = func(f *os.File) error {
				if armed && point == "file-sync" {
					return errors.New("fault")
				}
				return f.Sync()
			}
			hooks.syncs.directory = func(r *os.Root) error {
				if armed && (point == "runtime-sync" && filepath.Base(r.Name()) == ".serenity" || point == "root-sync" && r.Name() == root) {
					return errors.New("fault")
				}
				return maintenanceSync(r)
			}
			if point == "create" {
				hooks.create = func(*os.Root, string) (*os.File, error) { return nil, errors.New("fault") }
			}
			p, err := openInitialBrainMaintenance(root, id, hooks)
			if err != nil {
				t.Fatal(err)
			}
			armed = true
			if err = p.EnsureGate(context.Background(), initialAllow); err == nil {
				t.Fatal("expected failure")
			}
			if _, err = p.Observation(); err == nil {
				t.Fatal("failed ensure usable")
			}
			if point != "create" {
				initialAssertBlocked(t, root)
			}
			p, err = OpenInitialBrainMaintenance(root, id)
			if err != nil {
				t.Fatal(err)
			}
			obs, err := p.Observation()
			if err != nil {
				t.Fatal(err)
			}
			if !obs.Present {
				if err = p.EnsureGate(context.Background(), initialAllow); err != nil {
					t.Fatal(err)
				}
			}
			m, err := p.Complete(context.Background(), initialAllow)
			if err != nil {
				t.Fatal(err)
			}
			_ = m.Close()
		})
	}
}

func TestInitialMaintenanceRejectsUnsafeAndChangedState(t *testing.T) {
	for _, which := range []string{"foreign-prefix", "hardlink", "symlink", "mode", "replacement", "root-move", "lock-replace", "foreign-uid"} {
		t.Run(which, func(t *testing.T) {
			root := initialTestRoot(t)
			id := maintenanceFixtureID()
			raw, _ := maintenanceBytes(id)
			initialWritePrefix(t, root, raw[:17])
			switch which {
			case "foreign-prefix":
				initialWritePrefix(t, root, []byte("unknown"))
			case "hardlink":
				if err := os.Link(markerPath(root), filepath.Join(root, "linked")); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Remove(markerPath(root)); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("writer.lock", markerPath(root)); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.Chmod(markerPath(root), 0644); err != nil {
					t.Fatal(err)
				}
			case "foreign-uid":
				if os.Geteuid() != 0 {
					t.Skip("foreign UID fixture requires root")
				}
				if err := os.Chown(markerPath(root), 12345, 12345); err != nil {
					t.Fatal(err)
				}
			}
			p, err := OpenInitialBrainMaintenance(root, id)
			if which == "replacement" || which == "root-move" || which == "lock-replace" {
				if err != nil {
					t.Fatal(err)
				}
				switch which {
				case "replacement":
					if err = os.Rename(markerPath(root), markerPath(root)+".old"); err != nil {
						t.Fatal(err)
					}
					initialWritePrefix(t, root, raw[:17])
				case "root-move":
					if err = os.Rename(root, root+"-moved"); err != nil {
						t.Fatal(err)
					}
					if err = os.Mkdir(root, 0700); err != nil {
						t.Fatal(err)
					}
				case "lock-replace":
					lock := filepath.Join(root, ".serenity", "writer.lock")
					if err = os.Rename(lock, lock+".old"); err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(lock, nil, 0600); err != nil {
						t.Fatal(err)
					}
				}
				if err = p.ValidateFor(root, id); err == nil {
					t.Fatal("changed namespace validated")
				}
				_ = p.Close()
			} else if p != nil || err == nil {
				if p != nil {
					_ = p.Close()
				}
				t.Fatalf("unsafe admitted %v", err)
			}
		})
	}
}

func TestInitialMaintenanceProcessDeathRetainsGate(t *testing.T) {
	if os.Getenv("SERENITY_INITIAL_MAINTENANCE_HELPER") == "1" {
		root := os.Getenv("SERENITY_INITIAL_MAINTENANCE_ROOT")
		point := os.Getenv("SERENITY_INITIAL_MAINTENANCE_POINT")
		hooks := durableInitialMaintenanceHooks()
		hooks.checkpoint = func(p string) error {
			if p == point {
				os.Exit(23)
			}
			return nil
		}
		p, err := openInitialBrainMaintenance(root, maintenanceFixtureID(), hooks)
		if err != nil {
			os.Exit(24)
		}
		if strings.HasPrefix(point, "sync-gate-") {
			err = p.SyncGate(context.Background(), initialAllow)
		} else if strings.HasPrefix(point, "gate-") {
			err = p.EnsureGate(context.Background(), initialAllow)
		} else {
			_, err = p.Complete(context.Background(), initialAllow)
		}
		if err != nil {
			os.Exit(25)
		}
		os.Exit(26)
	}
	for _, point := range []string{"gate-created", "gate-file-synced", "gate-runtime-synced", "gate-root-synced", "marker-appended", "marker-file-synced", "marker-runtime-synced", "marker-root-synced", "sync-gate-file-synced", "sync-gate-runtime-synced", "sync-gate-root-synced"} {
		t.Run(point, func(t *testing.T) {
			root := initialTestRoot(t)
			if strings.HasPrefix(point, "marker-") || strings.HasPrefix(point, "sync-gate-") {
				raw, _ := maintenanceBytes(maintenanceFixtureID())
				initialWritePrefix(t, root, raw[:17])
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInitialMaintenanceProcessDeathRetainsGate$")
			cmd.Env = append(os.Environ(), "SERENITY_INITIAL_MAINTENANCE_HELPER=1", "SERENITY_INITIAL_MAINTENANCE_ROOT="+root, "SERENITY_INITIAL_MAINTENANCE_POINT="+point)
			err := cmd.Run()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 23 {
				t.Fatalf("helper %v", err)
			}
			initialAssertBlocked(t, root)
			p, err := OpenInitialBrainMaintenance(root, maintenanceFixtureID())
			if err != nil {
				t.Fatal(err)
			}
			m, err := p.Complete(context.Background(), initialAllow)
			if err != nil {
				t.Fatal(err)
			}
			_ = m.Close()
		})
	}
}

func TestExistingMaintenanceRejectsHardlinkedAndForeignOwnedMarker(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		t.Run(fmt.Sprint(foreign), func(t *testing.T) {
			root := initialTestRoot(t)
			id := maintenanceFixtureID()
			m, err := BeginBrainMaintenance(root, id)
			if err != nil {
				t.Fatal(err)
			}
			_ = m.Close()
			if foreign {
				if os.Geteuid() != 0 {
					t.Skip("requires root")
				}
				if err = os.Chown(markerPath(root), 12345, 12345); err != nil {
					t.Fatal(err)
				}
			} else {
				if err = os.Link(markerPath(root), filepath.Join(root, "second-link")); err != nil {
					t.Fatal(err)
				}
			}
			if m, err = ResumeBrainMaintenance(root, id); m != nil || err == nil {
				if m != nil {
					_ = m.Close()
				}
				t.Fatal("unsafe legacy marker resumed")
			}
		})
	}
}

func TestInitialMaintenanceSyncGatePreservesPrefixAndRestrictedOwner(t *testing.T) {
	root := initialTestRoot(t)
	id := maintenanceFixtureID()
	raw, _ := maintenanceBytes(id)
	initialWritePrefix(t, root, raw[:17])
	p, err := OpenInitialBrainMaintenance(root, id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	before, err := p.Observation()
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	if err = p.SyncGate(context.Background(), func(_ context.Context, o InitialMaintenanceObservation) error {
		calls++
		if !sameInitialObservation(before, o) {
			t.Fatal("different sync observation")
		}
		o.Prefix[0] ^= 1
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	after, err := p.Observation()
	if err != nil || !sameInitialObservation(before, after) || calls != 1 {
		t.Fatalf("sync changed/closed owner: %v calls=%d", err, calls)
	}
	initialAssertBlocked(t, root)
	m, err := p.Complete(context.Background(), initialAllow)
	if err != nil {
		t.Fatal(err)
	}
	_ = m.Close()
}

func TestInitialMaintenanceSyncGateFailuresCloseAndRetain(t *testing.T) {
	points := []string{"nil", "denied", "cancelled", "file-sync", "runtime-sync", "root-sync", "sync-gate-file-synced", "sync-gate-runtime-synced", "sync-gate-root-synced", "cancel-after-sync", "replace-after-sync"}
	for _, point := range points {
		t.Run(point, func(t *testing.T) {
			root := initialTestRoot(t)
			id := maintenanceFixtureID()
			raw, _ := maintenanceBytes(id)
			initialWritePrefix(t, root, raw[:17])
			before, _ := os.Stat(markerPath(root))
			hooks := durableInitialMaintenanceHooks()
			armed := false
			hooks.syncs.file = func(f *os.File) error {
				if armed && point == "file-sync" {
					return errors.New("fault")
				}
				return f.Sync()
			}
			hooks.syncs.directory = func(r *os.Root) error {
				if armed && (point == "runtime-sync" && filepath.Base(r.Name()) == ".serenity" || point == "root-sync" && r.Name() == root) {
					return errors.New("fault")
				}
				return maintenanceSync(r)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			hooks.checkpoint = func(p string) error {
				if p == point {
					return errors.New("fault")
				}
				if p == "sync-gate-root-synced" && point == "cancel-after-sync" {
					cancel()
				}
				if p == "sync-gate-root-synced" && point == "replace-after-sync" {
					if err := os.Rename(markerPath(root), markerPath(root)+".old"); err != nil {
						t.Fatal(err)
					}
					initialWritePrefix(t, root, raw[:17])
				}
				return nil
			}
			p, err := openInitialBrainMaintenance(root, id, hooks)
			if err != nil {
				t.Fatal(err)
			}
			armed = true
			verifier := InitialMaintenanceVerifier(initialAllow)
			if point == "nil" {
				verifier = nil
			}
			if point == "cancelled" {
				cancel()
			}
			if point == "denied" {
				verifier = func(context.Context, InitialMaintenanceObservation) error { return errors.New("denied") }
			}
			if err = p.SyncGate(ctx, verifier); err == nil {
				t.Fatal("fault admitted")
			}
			if _, err = p.Observation(); err == nil {
				t.Fatal("failed sync retained usable owner")
			}
			current, _ := os.ReadFile(markerPath(root))
			if !bytes.Equal(current, raw[:17]) {
				t.Fatal("sync appended marker")
			}
			after, _ := os.Stat(markerPath(root))
			if point != "replace-after-sync" && !os.SameFile(before, after) {
				t.Fatal("sync replaced marker")
			}
			initialAssertBlocked(t, root)
		})
	}
}

func TestInitialMaintenanceSyncGateRefusesAbsence(t *testing.T) {
	root := initialTestRoot(t)
	p, err := OpenInitialBrainMaintenance(root, maintenanceFixtureID())
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	if err = p.SyncGate(context.Background(), func(context.Context, InitialMaintenanceObservation) error { calls++; return nil }); err == nil {
		t.Fatal("absent sync admitted")
	}
	if calls != 0 {
		t.Fatal("absent sync invoked verifier")
	}
	if _, err = os.Lstat(markerPath(root)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("sync created marker")
	}
}
