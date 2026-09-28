package writer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

// AI-L03: a remembered fact records the principal that wrote it in its
// meta.yaml sidecar, and FactWriter reads it back for forget's check.
func TestRememberRecordsWriterPrincipal(t *testing.T) {
	root := t.TempDir()
	q := NewQueue(nil)
	defer q.Close()
	sources := store.NewSourceStore(root)
	w := MemoryFact{Queue: q, Sources: sources}
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

	result, err := w.Remember(RememberInput{Fact: "client a wrote this", Provenance: "test", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld, Writer: "credential:client-a"}, now)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := os.ReadFile(filepath.Join(sources.DirFor(result.Record.SHA256), "meta.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(meta), "writer: credential:client-a") {
		t.Fatalf("meta.yaml does not record the writer:\n%s", meta)
	}
	got, err := w.FactWriter(result.Record.SHA256)
	if err != nil || got != "credential:client-a" {
		t.Fatalf("FactWriter = %q, %v; want credential:client-a", got, err)
	}

	legacy, err := w.Remember(RememberInput{Fact: "written before writers were recorded", Provenance: "test", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := w.FactWriter(legacy.Record.SHA256); err != nil || got != "" {
		t.Fatalf("legacy FactWriter = %q, %v; want empty", got, err)
	}
}
