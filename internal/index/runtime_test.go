package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/config"
)

// seedRuntimeRow inserts a marker row directly into a runtime-only table,
// bypassing the Engine interface (which has no runtime-state writers yet —
// none exist until later milestones own these tables).
func seedRuntimeRow(t *testing.T, s *SQLite, table, id string) {
	t.Helper()
	if _, err := s.db.Exec(`INSERT INTO `+table+`(id, payload) VALUES(?, ?)`, id, "seed"); err != nil {
		t.Fatalf("seed %s: %v", table, err)
	}
}

func countRows(t *testing.T, s *SQLite, table string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// TestRuntimeTablesContents pins the allowlist named in the T0.10
// acceptance criterion: exactly {jobs, disposition_items,
// disposition_history, spend_ledger, caches}, no more, no less.
func TestRuntimeTablesContents(t *testing.T) {
	want := map[string]bool{
		"jobs":                true,
		"disposition_items":   true,
		"disposition_history": true,
		"spend_ledger":        true,
		"caches":              true,
	}
	if len(RuntimeTables) != len(want) {
		t.Fatalf("RuntimeTables has %d entries, want %d: %v", len(RuntimeTables), len(want), RuntimeTables)
	}
	for _, table := range RuntimeTables {
		if !want[table] {
			t.Fatalf("unexpected table %q in RuntimeTables", table)
		}
		delete(want, table)
	}
	if len(want) != 0 {
		t.Fatalf("RuntimeTables missing entries: %v", want)
	}
}

// TestRuntimeTableSchemaShells proves every allowlisted table exists and
// accepts a row from a fresh Open() — the "schema shell" half of T0.10.
func TestRuntimeTableSchemaShells(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "index.db")
	eng, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	for _, table := range RuntimeTables {
		if _, err := eng.db.ExecContext(ctx,
			`INSERT INTO `+table+`(id, payload) VALUES(?, ?)`, "shell-check", "x"); err != nil {
			t.Fatalf("schema shell for %s missing or wrong shape: %v", table, err)
		}
	}
}

// TestRebuildPreservesRuntimeState seeds one row per runtime table, runs a
// full Rebuild over a scaffolded brain, and proves the runtime rows survive
// untouched even though Rebuild starts with ResetAll.
func TestRebuildPreservesRuntimeState(t *testing.T) {
	ctx := context.Background()
	root := scaffoldBrain(t)
	cfg := config.Default()

	dbPath := filepath.Join(root, ".serenity", "index.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	eng, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	for _, table := range RuntimeTables {
		seedRuntimeRow(t, eng, table, "seed-1")
	}

	if err := Rebuild(ctx, root, cfg, eng); err != nil {
		t.Fatal(err)
	}

	for _, table := range RuntimeTables {
		if n := countRows(t, eng, table); n != 1 {
			t.Fatalf("Rebuild touched runtime table %s: want 1 row, got %d", table, n)
		}
	}

	stats, err := eng.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats["entities"] == 0 {
		t.Fatal("rebuild indexed nothing — test fixture or Rebuild is broken")
	}
}

// TestResetAllPreservesRuntimeStateWipesOthers is the ResetAll half of
// T0.10: derived tables are wiped, runtime tables are left exactly as
// found.
func TestResetAllPreservesRuntimeStateWipesOthers(t *testing.T) {
	ctx := context.Background()
	root := scaffoldBrain(t)
	cfg := config.Default()

	dbPath := filepath.Join(root, ".serenity", "index.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	eng, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	if err := Rebuild(ctx, root, cfg, eng); err != nil {
		t.Fatal(err)
	}
	for _, table := range RuntimeTables {
		seedRuntimeRow(t, eng, table, "seed-1")
	}

	if err := eng.ResetAll(ctx); err != nil {
		t.Fatal(err)
	}

	stats, err := eng.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for table, n := range stats {
		if n != 0 {
			t.Fatalf("ResetAll left rows in derived table %s: %d", table, n)
		}
	}
	for _, table := range RuntimeTables {
		if n := countRows(t, eng, table); n != 1 {
			t.Fatalf("ResetAll touched runtime table %s: want 1 row, got %d", table, n)
		}
	}
}
