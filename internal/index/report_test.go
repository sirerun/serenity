package index

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenReportRequiresExistingIndexAndRefusesWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "index.db")
	if _, err := OpenReport(path); err == nil {
		t.Fatal("missing index opened")
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("report created directory: %v", err)
	}
	path = filepath.Join(t.TempDir(), "index.db")
	seed, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := seed.RecordSpend(context.Background(), SpendRow{ID: "one", CostUSD: 2}); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}
	special := filepath.Join(filepath.Dir(path), "space #? index.db")
	if err := os.Rename(path, special); err != nil {
		t.Fatal(err)
	}
	path = special
	eng, err := OpenReport(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	rows, err := eng.SpendRows(context.Background())
	if err != nil || len(rows) != 1 {
		t.Fatalf("read existing state: %v %v", rows, err)
	}
	if err := eng.RecordSpend(context.Background(), SpendRow{ID: "two"}); err == nil {
		t.Fatal("report handle accepted write")
	}
	if _, err := OpenReport(t.TempDir()); err == nil {
		t.Fatal("directory accepted as index")
	}
}
