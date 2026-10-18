//go:build darwin || linux

package writer

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func validationInventory(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[rel] = info.Mode().String()
		if info.Mode()&os.ModeSymlink != 0 {
			target, e := os.Readlink(path)
			if e != nil {
				return e
			}
			out[rel] += target
		} else if info.Mode().IsRegular() {
			b, e := os.ReadFile(path)
			if e != nil {
				return e
			}
			out[rel] += string(b)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMaintenanceValidateForExactOwnerIsReadOnly(t *testing.T) {
	root := maintenanceTestRoot(t)
	id := maintenanceFixtureID()
	owner, err := BeginBrainMaintenance(root, id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close() }()
	before := validationInventory(t, root)
	if err = owner.ValidateFor(root, id); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, validationInventory(t, root)) {
		t.Fatal("validation changed source state")
	}
	foreign := maintenanceTestRoot(t)
	if err = owner.ValidateFor(foreign, id); err == nil {
		t.Fatal("foreign root adopted owner")
	}
	wrong := id
	wrong.OperationID = "another-operation"
	if err = owner.ValidateFor(root, wrong); err == nil {
		t.Fatal("wrong operation adopted owner")
	}
	if !reflect.DeepEqual(before, validationInventory(t, root)) {
		t.Fatal("refusal changed source state")
	}
	if err = owner.Finish(id); err != nil {
		t.Fatal(err)
	}
	if err = owner.ValidateFor(root, id); err == nil {
		t.Fatal("finished owner used as live maintenance")
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err = owner.ValidateFor(root, id); err == nil {
		t.Fatal("closed owner used as live maintenance")
	}
	var missing *BrainMaintenance
	if err = missing.ValidateFor(root, id); err == nil {
		t.Fatal("nil owner accepted")
	}
	if err = (&BrainMaintenance{}).ValidateFor(root, id); err == nil {
		t.Fatal("empty owner accepted")
	}
}

func TestMaintenanceValidateForRejectsChangedNamespace(t *testing.T) {
	for _, mode := range []string{"root-symlink", "runtime-symlink", "runtime-replaced", "writer.lock", "canonical-view.lock", "marker", "view-permissions", "writer-hardlink", "root-permissions"} {
		t.Run(mode, func(t *testing.T) {
			root := maintenanceTestRoot(t)
			id := maintenanceFixtureID()
			owner, err := BeginBrainMaintenance(root, id)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = owner.Close() }()
			supplied := root
			state := filepath.Join(root, ".serenity")
			switch mode {
			case "root-symlink":
				supplied = filepath.Join(t.TempDir(), "alias")
				if err = os.Symlink(root, supplied); err != nil {
					t.Fatal(err)
				}
			case "runtime-symlink", "runtime-replaced":
				old := filepath.Join(root, "retained-runtime")
				if err = os.Rename(state, old); err != nil {
					t.Fatal(err)
				}
				if mode == "runtime-symlink" {
					err = os.Symlink(old, state)
				} else {
					err = os.Mkdir(state, 0700)
				}
				if err != nil {
					t.Fatal(err)
				}
			case "writer.lock", "canonical-view.lock", "marker":
				name := mode
				if mode == "marker" {
					name = maintenanceMarkerName
				}
				path := filepath.Join(state, name)
				bytes, readErr := os.ReadFile(path)
				if readErr != nil {
					t.Fatal(readErr)
				}
				if err = os.Rename(path, path+".retained"); err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, bytes, 0600); err != nil {
					t.Fatal(err)
				}
			case "view-permissions":
				if err = os.Chmod(filepath.Join(state, canonicalViewLockName), 0644); err != nil {
					t.Fatal(err)
				}
			case "writer-hardlink":
				if err = os.Link(filepath.Join(state, "writer.lock"), filepath.Join(state, "writer-alias")); err != nil {
					t.Fatal(err)
				}
			case "root-permissions":
				if err = os.Chmod(root, 0777); err != nil {
					t.Fatal(err)
				}
			default:
				t.Fatalf("unknown fixture %s", mode)
			}
			before := validationInventory(t, root)
			if err = owner.ValidateFor(supplied, id); err == nil {
				t.Fatal("changed namespace adopted maintenance owner")
			}
			if !reflect.DeepEqual(before, validationInventory(t, root)) {
				t.Fatal("validation repaired changed namespace")
			}
		})
	}
}
