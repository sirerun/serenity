package episodeinstall

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func privateTestDirectory(t *testing.T) string {
	t.Helper()
	p, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(p, 0700); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestInventoryCopiesExactFilesModesAndEmptyDirectories(t *testing.T) {
	ctx := context.Background()
	parent := privateTestDirectory(t)
	src := filepath.Join(parent, "source")
	dst := filepath.Join(parent, "copy")
	if err := os.Mkdir(src, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(src, "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	raw := []byte("independent retained bytes")
	if err := os.WriteFile(filepath.Join(src, "record"), raw, 0640); err != nil {
		t.Fatal(err)
	}
	// File creation is subject to the runner's private umask; pin explicit modes.
	if err := os.Chmod(filepath.Join(src, "record"), 0640); err != nil {
		t.Fatal(err)
	}
	inv, err := inventoryPath(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	if !validInventory(inv) {
		t.Fatal("invalid own inventory")
	}
	if err = copyInventory(ctx, src, dst, inv); err != nil {
		t.Fatal(err)
	}
	got, err := inventoryPath(ctx, dst)
	if err != nil || !reflect.DeepEqual(got, inv) {
		t.Fatalf("copy inventory mismatch: %v", err)
	}
	if err = os.WriteFile(filepath.Join(dst, "extra"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if verifyInventory(ctx, dst, inv) == nil {
		t.Fatal("extra file accepted")
	}
}
func TestInventoryRefusesUnsafeObjectsAndCancellation(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "writable", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			root := privateTestDirectory(t)
			file := filepath.Join(root, "file")
			if err := os.WriteFile(file, []byte("retained"), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch kind {
			case "symlink":
				if err := os.Symlink(file, filepath.Join(root, "alias")); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(file, filepath.Join(root, "alias")); err != nil {
					t.Fatal(err)
				}
			case "writable":
				if err := os.Chmod(file, 0660); err != nil {
					t.Fatal(err)
				}
			case "canceled":
				cancel()
			}
			if _, err := inventoryPath(ctx, root); err == nil {
				t.Fatal("unsafe inventory accepted")
			}
		})
	}
}
func TestInventoryAbsenceAndStructuralValidation(t *testing.T) {
	root := privateTestDirectory(t)
	inv, err := inventoryPath(context.Background(), filepath.Join(root, "missing"))
	if err != nil || len(inv) != 1 || inv[0].Kind != "absent" {
		t.Fatalf("absence: %v", err)
	}
	for _, bad := range []inventory{
		nil,
		{{Path: ".", Kind: "absent", Mode: 0700}},
		{{Path: ".", Kind: "directory", Mode: 0700}, {Path: "a/b", Kind: "file", Mode: 0600, SHA256: strings.Repeat("a", 64)}},
		{{Path: ".", Kind: "file", Mode: 0600, SHA256: strings.Repeat("a", 64)}, {Path: "child", Kind: "directory", Mode: 0700}},
		{{Path: ".", Kind: "directory", Mode: 0700}, {Path: "../escape", Kind: "directory", Mode: 0700}},
	} {
		if validInventory(bad) {
			t.Fatal("malformed inventory accepted")
		}
	}
}
func TestRuntimeRefusesUnknownReplayArtifacts(t *testing.T) {
	parent := privateTestDirectory(t)
	if err := os.Mkdir(filepath.Join(parent, ".serenity"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, ".serenity", "writer.lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateRuntime(context.Background(), parent, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(parent, ".serenity", "pending"), 0700); err != nil {
		t.Fatal(err)
	}
	if validateRuntime(context.Background(), parent, true) == nil {
		t.Fatal("unrecognized directory accepted")
	}
}

func TestInventoryFanoutReadsAreBoundedBeforeRejection(t *testing.T) {
	root := privateTestDirectory(t)
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	_, err := inventoryPathBounded(context.Background(), root, 4, func(f *os.File, n int) ([]os.DirEntry, error) {
		calls++
		if n > 4 || n < 1 {
			t.Fatalf("unbounded listing request %d", n)
		}
		return f.ReadDir(n)
	})
	if err == nil || calls != 1 {
		t.Fatalf("overlimit fanout accepted or unbounded reads: %v calls=%d", err, calls)
	}
}

func TestExclusiveRenamePreservesUnexpectedDestination(t *testing.T) {
	root := privateTestDirectory(t)
	from, to := filepath.Join(root, "from"), filepath.Join(root, "to")
	for _, p := range []string{from, to} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(to, "retain"), []byte("unexpected owner bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if renameExclusive(from, to) == nil {
		t.Fatal("overwrote unexpected destination")
	}
	if _, err := os.Stat(from); err != nil {
		t.Fatal("source disappeared")
	}
	got, err := os.ReadFile(filepath.Join(to, "retain"))
	if err != nil || string(got) != "unexpected owner bytes" {
		t.Fatal("destination lost")
	}
}
