//go:build darwin || linux

package index

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/sirerun/serenity/internal/domain"
)

func maintenanceTestDirectory(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	return root
}
func maintenanceTestOriginal(t *testing.T) (string, *SQLite) {
	t.Helper()
	path := filepath.Join(maintenanceTestDirectory(t), "index.db")
	engine, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, engine
}
func maintenanceTestFamily(t *testing.T, path string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		raw, err := os.ReadFile(path + suffix)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		out["index.db"+suffix] = raw
	}
	return out
}
func maintenanceTestCopy(t *testing.T, path string) string {
	t.Helper()
	copyPath := filepath.Join(maintenanceTestDirectory(t), "index.db")
	for name, raw := range maintenanceTestFamily(t, path) {
		if err := os.WriteFile(filepath.Join(filepath.Dir(copyPath), name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return copyPath
}
func maintenanceAssertOriginal(t *testing.T, path string, before map[string][]byte) {
	t.Helper()
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(maintenanceTestFamily(t, path))
	if string(a) != string(b) {
		t.Fatal("maintenance qualification changed original family")
	}
}
func maintenanceAssertRefused(t *testing.T, path string) {
	t.Helper()
	result, err := QualifyMaintenanceCopy(context.Background(), path)
	if !errors.Is(err, ErrMaintenanceCopyUnqualified) || result.Schema != "" {
		t.Fatalf("unqualified copy admitted: %+v %v", result, err)
	}
	if strings.Contains(err.Error(), "payload-secret") {
		t.Fatal("database payload escaped in error")
	}
}

func TestMaintenanceCopyAllowsCurrentDerivedSchemaAndPreservesOriginal(t *testing.T) {
	path, engine := maintenanceTestOriginal(t)
	ctx := context.Background()
	for _, err := range []error{
		engine.UpsertEntity(ctx, domain.Entity{Slug: "project", Type: "project"}),
		engine.UpsertClaim(ctx, domain.Claim{SubjectSlug: "project", ID: "claim", Family: "status", Predicate: "status", Object: "active", ObjectKey: "active", State: domain.StateActive}),
		engine.InsertChunk(ctx, "derived-ref", "project", "retained derived data", "", "file"),
		engine.UpsertVector(ctx, "derived-ref", "fixture@v1", []float32{1, 2}),
		engine.RecordSourcePath(ctx, strings.Repeat("a", 64), "/independent/source/path"),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	before := maintenanceTestFamily(t, path)
	copyPath := maintenanceTestCopy(t, path)
	result, err := QualifyMaintenanceCopy(ctx, copyPath)
	if err != nil {
		t.Fatal(err)
	}
	if result.Schema != "serenity.index-maintenance-cohort/v1" || len(result.SchemaSHA256) != 64 || len(result.EmptyRuntimeTables) != len(RuntimeTables) || result.SidecarEffects != "owned_copy_shm_may_change_empty_wal_may_appear" || len(result.Before) == 0 || len(result.After) == 0 {
		t.Fatalf("bad qualification: %+v", result)
	}
	beforeWAL, afterWAL := false, false
	for _, file := range result.Before {
		if file.Name == "index.db-wal" {
			beforeWAL = true
		}
	}
	for _, file := range result.After {
		if file.Name == "index.db-wal" {
			afterWAL = true
			if file.Bytes != 0 || file.SHA256 != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
				t.Fatal("qualified newly created WAL is not empty")
			}
		}
	}
	if beforeWAL || !afterWAL {
		t.Fatal("fixture did not prove absent-to-empty WAL side effect")
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "retained derived data") || strings.Contains(string(encoded), path) || strings.Contains(string(encoded), "/independent/source/path") {
		t.Fatal("content-bearing qualification")
	}
	maintenanceAssertOriginal(t, path, before)
}

func TestMaintenanceCopyEveryRuntimeTableBlocksEvenCompletedRows(t *testing.T) {
	for _, table := range RuntimeTables {
		t.Run(table, func(t *testing.T) {
			path, engine := maintenanceTestOriginal(t)
			if _, err := engine.db.Exec("INSERT INTO "+table+"(id,payload) VALUES(?,?)", "completed", `{"status":"succeeded","text":"payload-secret"}`); err != nil {
				t.Fatal(err)
			}
			if err := engine.Close(); err != nil {
				t.Fatal(err)
			}
			before := maintenanceTestFamily(t, path)
			maintenanceAssertRefused(t, maintenanceTestCopy(t, path))
			maintenanceAssertOriginal(t, path, before)
		})
	}
}

func TestMaintenanceCopyHonorsWALResidentControlRows(t *testing.T) {
	path, engine := maintenanceTestOriginal(t)
	defer func() { _ = engine.Close() }()
	if _, err := engine.db.Exec("PRAGMA wal_autocheckpoint=0"); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.db.Exec("INSERT INTO spend_ledger(id,payload) VALUES('paid','payload-secret')"); err != nil {
		t.Fatal(err)
	}
	wal, err := os.Stat(path + "-wal")
	if err != nil || wal.Size() == 0 {
		t.Fatal("fixture lacks WAL-resident state", err)
	}
	before := maintenanceTestFamily(t, path)
	maintenanceAssertRefused(t, maintenanceTestCopy(t, path))
	maintenanceAssertOriginal(t, path, before)
}

func TestMaintenanceCopyRejectsForeignAndChangedSchema(t *testing.T) {
	for _, change := range []string{
		"CREATE TABLE foreign_data(payload TEXT)",
		"CREATE VIEW foreign_view AS SELECT * FROM entities",
		"CREATE INDEX foreign_index ON entities(type)",
		"CREATE TRIGGER foreign_trigger AFTER INSERT ON entities BEGIN INSERT INTO caches(id,payload) VALUES('trigger','payload-secret'); END",
		"DROP TABLE chunks",
		"DROP TABLE chunks; CREATE VIRTUAL TABLE chunks USING fts5(chunk_ref UNINDEXED,entity_slug UNINDEXED,text,source_sha256 UNINDEXED,kind UNINDEXED,tokenize='porter')",
		"ALTER TABLE claims ADD COLUMN foreign_column TEXT",
		"PRAGMA user_version=1",
		"PRAGMA application_id=7",
	} {
		t.Run(change, func(t *testing.T) {
			path, engine := maintenanceTestOriginal(t)
			if _, err := engine.db.Exec(change); err != nil {
				t.Fatal(err)
			}
			if err := engine.Close(); err != nil {
				t.Fatal(err)
			}
			before := maintenanceTestFamily(t, path)
			maintenanceAssertRefused(t, maintenanceTestCopy(t, path))
			maintenanceAssertOriginal(t, path, before)
		})
	}
}

func TestMaintenanceCopyRefusesUnsafeBoundsAndCorruption(t *testing.T) {
	for _, scenario := range []string{"missing", "mode", "directory_mode", "symlink", "hardlink", "foreign_entry", "oversized", "corrupt", "wal_mode", "directory_symlink", "fifo"} {
		t.Run(scenario, func(t *testing.T) {
			original, engine := maintenanceTestOriginal(t)
			if err := engine.Close(); err != nil {
				t.Fatal(err)
			}
			before := maintenanceTestFamily(t, original)
			path := maintenanceTestCopy(t, original)
			switch scenario {
			case "missing":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "directory_mode":
				if err := os.Chmod(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(original, path); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(original, path); err != nil {
					t.Fatal(err)
				}
			case "foreign_entry":
				if err := os.WriteFile(filepath.Join(filepath.Dir(path), "unknown"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "oversized":
				if err := os.Truncate(path, maintenanceCopyMaxFamilyBytes+1); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				if err := os.WriteFile(path, []byte("corrupt payload-secret"), 0600); err != nil {
					t.Fatal(err)
				}
			case "directory_symlink":
				alias := filepath.Join(maintenanceTestDirectory(t), "alias")
				if err := os.Symlink(filepath.Dir(path), alias); err != nil {
					t.Fatal(err)
				}
				path = filepath.Join(alias, "index.db")
			case "fifo":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := syscall.Mkfifo(path, 0600); err != nil {
					t.Fatal(err)
				}
			case "wal_mode":
				if err := os.WriteFile(path+"-wal", nil, 0600); err != nil {
					t.Fatal(err)
				}
				// Explicit chmod makes the refusal independent of the test
				// runner's restrictive umask.
				if err := os.Chmod(path+"-wal", 0644); err != nil {
					t.Fatal(err)
				}
			}
			maintenanceAssertRefused(t, path)
			maintenanceAssertOriginal(t, original, before)
		})
	}
}

func TestMaintenanceCopyRejectsInterleavedFileReplacementOrChange(t *testing.T) {
	for _, scenario := range []string{"replace", "change", "directory_replace"} {
		t.Run(scenario, func(t *testing.T) {
			original, engine := maintenanceTestOriginal(t)
			if err := engine.Close(); err != nil {
				t.Fatal(err)
			}
			before := maintenanceTestFamily(t, original)
			path := maintenanceTestCopy(t, original)
			observer := func(stage string) {
				if stage != "closed" {
					return
				}
				if scenario == "directory_replace" {
					parent := filepath.Dir(path)
					if err := os.Rename(parent, parent+"-previous"); err != nil {
						t.Fatal(err)
					}
					if err := os.Mkdir(parent, 0700); err != nil {
						t.Fatal(err)
					}
					raw, err := os.ReadFile(filepath.Join(parent+"-previous", "index.db"))
					if err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(path, raw, 0600); err != nil {
						t.Fatal(err)
					}
				} else if scenario == "replace" {
					raw, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if err = os.Rename(path, path+"-previous"); err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(path, raw, 0600); err != nil {
						t.Fatal(err)
					}
					// Keep the strict family directory clean so inode comparison is the
					// reason for refusal, rather than an unrelated extra-entry failure.
					if err = os.Rename(path+"-previous", filepath.Join(t.TempDir(), "retained-original")); err != nil {
						t.Fatal(err)
					}
				} else {
					file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
					if err != nil {
						t.Fatal(err)
					}
					_, writeErr := file.Write([]byte("changed"))
					closeErr := file.Close()
					if errors.Join(writeErr, closeErr) != nil {
						t.Fatal(writeErr, closeErr)
					}
				}
			}
			result, err := qualifyMaintenanceCopy(context.Background(), path, observer)
			if result.Schema != "" || !errors.Is(err, ErrMaintenanceCopyUnqualified) {
				t.Fatalf("changed copy admitted: %v", err)
			}
			maintenanceAssertOriginal(t, original, before)
		})
	}
}

func TestMaintenanceCopyCancellationAndSchemaBounds(t *testing.T) {
	path, engine := maintenanceTestOriginal(t)
	for i := 0; i < maintenanceCopyMaxSchemaRows; i++ {
		// Unique names are fixed test data, never application-generated SQL.
		if _, err := engine.db.Exec("CREATE TABLE extra_" + strings.Repeat("a", i+1) + "(id TEXT)"); err != nil {
			t.Fatal(err)
		}
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	copyPath := maintenanceTestCopy(t, path)
	maintenanceAssertRefused(t, copyPath)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := QualifyMaintenanceCopy(ctx, copyPath); result.Schema != "" || !errors.Is(err, ErrMaintenanceCopyUnqualified) {
		t.Fatal("cancelled qualification admitted")
	}
}

func TestMaintenanceCopySchemaByteBoundAndMidReadCancellation(t *testing.T) {
	original, engine := maintenanceTestOriginal(t)
	if _, err := engine.db.Exec("CREATE TABLE large_schema(id TEXT DEFAULT '" + strings.Repeat("x", maintenanceCopyMaxSchemaBytes) + "')"); err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	maintenanceAssertRefused(t, maintenanceTestCopy(t, original))
	valid, engine := maintenanceTestOriginal(t)
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	before := maintenanceTestFamily(t, valid)
	copyPath := maintenanceTestCopy(t, valid)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result, err := qualifyMaintenanceCopy(ctx, copyPath, func(stage string) {
		if stage == "opened" {
			cancel()
		}
	})
	if result.Schema != "" || !errors.Is(err, ErrMaintenanceCopyUnqualified) {
		t.Fatal("mid-inspection cancellation admitted")
	}
	maintenanceAssertOriginal(t, valid, before)
}

func TestMaintenanceCopyUnknownCheckSchemaRefusedBeforeIntegrity(t *testing.T) {
	original, engine := maintenanceTestOriginal(t)
	if _, err := engine.db.Exec("CREATE TABLE foreign_check(payload TEXT CHECK(length(payload)<10))"); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.db.Exec("INSERT INTO foreign_check(payload) VALUES('short')"); err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	before := maintenanceTestFamily(t, original)
	copyPath := maintenanceTestCopy(t, original)
	schemaQualified := false
	result, err := qualifyMaintenanceCopy(context.Background(), copyPath, func(stage string) {
		if stage == "schema-qualified" {
			schemaQualified = true
		}
	})
	if result.Schema != "" || !errors.Is(err, ErrMaintenanceCopyUnqualified) || schemaQualified {
		t.Fatalf("foreign CHECK schema reached integrity phase: %v qualified=%v", err, schemaQualified)
	}
	maintenanceAssertOriginal(t, original, before)
}

func TestMaintenanceCopyPreOpenRepinRefusesSubstitutedNamespace(t *testing.T) {
	for _, scenario := range []string{"file_replace", "file_symlink", "directory_replace"} {
		t.Run(scenario, func(t *testing.T) {
			original, engine := maintenanceTestOriginal(t)
			if err := engine.Close(); err != nil {
				t.Fatal(err)
			}
			originalBefore := maintenanceTestFamily(t, original)
			path := maintenanceTestCopy(t, original)
			foreign := maintenanceTestCopy(t, original)
			foreignBefore := maintenanceTestFamily(t, foreign)
			opened := false
			observer := func(stage string) {
				if stage == "opened" {
					opened = true
				}
				if stage != "pinned" {
					return
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "directory_replace" {
					parent := filepath.Dir(path)
					if err = os.Rename(parent, parent+"-old"); err != nil {
						t.Fatal(err)
					}
					if err = os.Mkdir(parent, 0700); err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(path, raw, 0600); err != nil {
						t.Fatal(err)
					}
				} else {
					retained := filepath.Join(maintenanceTestDirectory(t), "retained-index")
					if err = os.Rename(path, retained); err != nil {
						t.Fatal(err)
					}
					if scenario == "file_symlink" {
						err = os.Symlink(foreign, path)
					} else {
						err = os.WriteFile(path, raw, 0600)
					}
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			result, err := qualifyMaintenanceCopy(context.Background(), path, observer)
			if result.Schema != "" || !errors.Is(err, ErrMaintenanceCopyUnqualified) || opened {
				t.Fatalf("substituted copy reached SQLite: %v opened=%v", err, opened)
			}
			maintenanceAssertOriginal(t, original, originalBefore)
			maintenanceAssertOriginal(t, foreign, foreignBefore)
		})
	}
}

func TestMaintenanceCopyRejectsNewNonemptyOrChangedExistingWAL(t *testing.T) {
	for _, scenario := range []string{"new_nonempty", "existing_replaced", "existing_removed", "existing_changed"} {
		t.Run(scenario, func(t *testing.T) {
			original, engine := maintenanceTestOriginal(t)
			if err := engine.Close(); err != nil {
				t.Fatal(err)
			}
			before := maintenanceTestFamily(t, original)
			path := maintenanceTestCopy(t, original)
			if scenario != "new_nonempty" {
				if err := os.WriteFile(path+"-wal", nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			observer := func(stage string) {
				if stage != "closed" {
					return
				}
				wal := path + "-wal"
				switch scenario {
				case "new_nonempty", "existing_changed":
					if err := os.WriteFile(wal, []byte("unexpected WAL frames"), 0600); err != nil {
						t.Fatal(err)
					}
				case "existing_removed":
					if err := os.Remove(wal); err != nil {
						t.Fatal(err)
					}
				case "existing_replaced":
					retained := filepath.Join(maintenanceTestDirectory(t), "retained-wal")
					if err := os.Rename(wal, retained); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(wal, nil, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			result, err := qualifyMaintenanceCopy(context.Background(), path, observer)
			if result.Schema != "" || !errors.Is(err, ErrMaintenanceCopyUnqualified) {
				t.Fatalf("unqualified WAL mutation admitted: %v", err)
			}
			maintenanceAssertOriginal(t, original, before)
		})
	}
}
