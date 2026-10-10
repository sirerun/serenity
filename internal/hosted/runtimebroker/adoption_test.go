package runtimebroker

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/authority"
)

func TestInventoryBindsEveryOperationalEntryAndEphemeralRules(t *testing.T) {
	ctx := context.Background()
	dir, err := os.MkdirTemp("/tmp", "serenity-inventory-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.Mkdir(filepath.Join(dir, "brains"), 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "control.db")
	if err := os.WriteFile(file, []byte("checkpoint"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := InventorySHA256(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".serenity"), 0700); err != nil {
		t.Fatal(err)
	}
	after, err := InventorySHA256(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("directory addition not bound")
	}
	if err := os.WriteFile(filepath.Join(dir, ".serenity", "writer.lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	lockHash, err := InventorySHA256(ctx, dir)
	if err != nil || lockHash != after {
		t.Fatal("empty lock unexpectedly changes inventory")
	}
	listener, err := net.Listen("unix", filepath.Join(dir, ".hosted-admin.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	socketHash, err := InventorySHA256(ctx, dir)
	if err != nil || socketHash != after {
		t.Fatal("genuine admin socket unexpectedly changes inventory")
	}
	for _, name := range []string{"bytes", "mode", "new-file", "new-directory", "lock-content", "socket-regular"} {
		t.Run(name, func(t *testing.T) {
			child := t.TempDir()
			if err := os.WriteFile(filepath.Join(child, "control.db"), []byte("checkpoint"), 0600); err != nil {
				t.Fatal(err)
			}
			initial, err := InventorySHA256(ctx, child)
			if err != nil {
				t.Fatal(err)
			}
			var changeErr error
			switch name {
			case "bytes":
				changeErr = os.WriteFile(filepath.Join(child, "control.db"), []byte("different"), 0600)
			case "mode":
				changeErr = os.Chmod(filepath.Join(child, "control.db"), 0640)
			case "new-file":
				changeErr = os.WriteFile(filepath.Join(child, "old-backup"), nil, 0600)
			case "new-directory":
				changeErr = os.Mkdir(filepath.Join(child, "unknown"), 0700)
			case "lock-content":
				changeErr = os.Mkdir(filepath.Join(child, ".serenity"), 0700)
				if changeErr == nil {
					changeErr = os.WriteFile(filepath.Join(child, ".serenity", "writer.lock"), []byte("nonempty"), 0600)
				}
			case "socket-regular":
				changeErr = os.WriteFile(filepath.Join(child, ".hosted-admin.sock"), nil, 0600)
			}
			if changeErr != nil {
				t.Fatal(changeErr)
			}
			changed, err := InventorySHA256(ctx, child)
			if err == nil && changed == initial {
				t.Fatal("changed data not bound")
			}
		})
	}
}
func TestInventoryRejectsLinksCancellationAndSizeLimit(t *testing.T) {
	for _, name := range []string{"symlink", "hardlink", "large", "root-link"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "control.db")
			if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
				t.Fatal(err)
			}
			var err error
			switch name {
			case "symlink":
				err = os.Symlink(file, filepath.Join(dir, "redirect"))
			case "hardlink":
				err = os.Link(file, filepath.Join(dir, "alias"))
			case "large":
				err = os.Truncate(file, MaxInventoryBytes+1)
			case "root-link":
				link := filepath.Join(t.TempDir(), "link")
				err = os.Symlink(dir, link)
				dir = link
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := InventorySHA256(context.Background(), dir); err == nil {
				t.Fatal("unsafe inventory accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := InventorySHA256(ctx, t.TempDir()); err == nil {
		t.Fatal("canceled inventory completed")
	}
}
func TestInitialAdoptionRequiresExactOwnerInventory(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "control.db"), []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	inventory, err := InventorySHA256(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	a := authority.Adoption{Version: 1, HistoryOrigin: authority.LegacyHistoryOrigin, CheckpointSHA256: strings.Repeat("c", 64), InventorySHA256: inventory, PriorBuildSHA: strings.Repeat("e", 40)}
	if err := requireInitialData(ctx, dir, nil); err == nil {
		t.Fatal("legacy silently admitted")
	}
	if err := requireInitialData(ctx, dir, &a); err != nil {
		t.Fatal(err)
	}
	a.InventorySHA256 = strings.Repeat("f", 64)
	if err := requireInitialData(ctx, dir, &a); err == nil {
		t.Fatal("different legacy data admitted")
	}
}

func TestBrokerAdoptionRequiresRootPinAndRejectsRuntimeInjection(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "control.db"), []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	inventory, err := InventorySHA256(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	a := authority.Adoption{Version: 1, HistoryOrigin: authority.LegacyHistoryOrigin, CheckpointSHA256: strings.Repeat("c", 64), InventorySHA256: inventory, PriorBuildSHA: strings.Repeat("e", 40)}
	pin, err := a.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"", strings.Repeat("f", 64), pin} {
		c := Config{DataDir: dir, AdoptionSHA256: expected}
		got, err := brokerInitialInventory(ctx, c, &a)
		if expected == pin {
			if err != nil || got != inventory {
				t.Fatalf("exact owner pin: %v", err)
			}
		} else if err == nil {
			t.Fatal("unowned adoption admitted")
		}
	}
	for _, q := range []authority.Request{{Action: "activate", DataInventorySHA256: inventory}, {Action: "activate", Adoption: &a}, {Action: "prepare", Measurement: &authority.Measurement{}}} {
		if rejectRuntimeIdentity(q) == nil {
			t.Fatal("runtime identity assertion accepted")
		}
	}
	if err := rejectRuntimeIdentity(authority.Request{Action: "activate"}); err != nil {
		t.Fatal(err)
	}
}
