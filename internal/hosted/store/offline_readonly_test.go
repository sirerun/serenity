//go:build darwin || linux

package store

import (
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func offlineStoreFixture(t *testing.T) (string, string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "control.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateAccount(context.Background(), "offline-readonly@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`PRAGMA journal_mode=DELETE`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	return path, a.ID
}

func TestOfflineReadOnlyPreservesDatabaseAndRejectsWrites(t *testing.T) {
	path, id := offlineStoreFixture(t)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := OpenOfflineReadOnly(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	if err := s.DB().QueryRow(`SELECT id FROM accounts WHERE id=?`, id).Scan(&got); err != nil || got != id {
		t.Fatalf("existing read: %v", err)
	}
	if _, err := s.DB().Exec(`CREATE TABLE forbidden_write (id TEXT)`); err == nil {
		t.Fatal("read-only store accepted a write")
	}
	// The file is read-only independently of the connection's mutable pragma.
	if _, err := s.DB().Exec(`PRAGMA query_only=OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB().Exec(`CREATE TABLE still_forbidden (id TEXT)`); err == nil {
		t.Fatal("disabling query_only made the offline database writable")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("read-only store changed database bytes")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 || entries[0].Name() != "control.db" {
		t.Fatal("read-only store created a sidecar")
	}
}

func TestOfflineReadOnlyRefusesUnsafeOrUnsupportedState(t *testing.T) {
	for _, mode := range []string{"missing", "wal", "shm", "journal", "symlink", "hardlink", "public", "future", "incomplete", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			path, _ := offlineStoreFixture(t)
			directory := filepath.Dir(path)
			snapshot := func() map[string][32]byte {
				t.Helper()
				entries, err := os.ReadDir(directory)
				if err != nil {
					t.Fatal(err)
				}
				result := make(map[string][32]byte)
				for _, entry := range entries {
					name := filepath.Join(directory, entry.Name())
					var data []byte
					if entry.Type()&os.ModeSymlink != 0 {
						link, linkErr := os.Readlink(name)
						err, data = linkErr, []byte(link)
					} else {
						data, err = os.ReadFile(name)
					}
					if err != nil {
						t.Fatal(err)
					}
					result[entry.Name()] = sha256.Sum256(data)
				}
				return result
			}
			ctx := context.Background()
			var err error
			switch mode {
			case "missing":
				path = filepath.Join(filepath.Dir(path), "missing.db")
			case "wal", "shm", "journal":
				err = os.WriteFile(path+"-"+mode, nil, 0600)
			case "symlink":
				link := filepath.Join(filepath.Dir(path), "link.db")
				err = os.Symlink(path, link)
				path = link
			case "hardlink":
				err = os.Link(path, path+".link")
			case "public":
				err = os.Chmod(path, 0644)
			case "future", "incomplete":
				s, openErr := Open(path)
				if openErr != nil {
					t.Fatal(openErr)
				}
				query := `UPDATE schema_migrations SET version=11 WHERE version=10`
				if mode == "incomplete" {
					query = `DELETE FROM schema_migrations WHERE version=9`
				}
				if _, err = s.DB().Exec(query); err != nil {
					t.Fatal(err)
				}
				if _, err = s.DB().Exec(`PRAGMA journal_mode=DELETE`); err != nil {
					t.Fatal(err)
				}
				err = s.Close()
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if err != nil {
				t.Fatal(err)
			}
			before := snapshot()
			s, err := OpenOfflineReadOnly(ctx, path)
			if err == nil || s != nil {
				t.Fatal("unsafe offline database admitted")
			}
			if !reflect.DeepEqual(before, snapshot()) {
				t.Fatal("refused read-only open changed database or sidecars")
			}
			if mode == "missing" {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatal("missing database was created")
				}
			}
		})
	}
}
