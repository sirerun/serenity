package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
)

// TestLegacyAbsoluteSourceURIStillResolves pins T24.24's compatibility
// promise: a brain written before source URIs became root-relative keeps
// its historical meta.yaml (absolute file:// uri, no path_hash) and that
// record still reads back unchanged. Nothing rewrites history.
func TestLegacyAbsoluteSourceURIStillResolves(t *testing.T) {
	root := t.TempDir()
	data := []byte("legacy bytes")
	ss := NewSourceStore(root)
	written, err := ss.Write(data, domain.Source{
		Kind: "file", URI: "file:///srv/example/notes/plan.txt",
		OccurredAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Meta:       map[string]string{"path": "notes/plan.txt", "size": "12"},
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Pin the sidecar byte for byte to the pre-T24.24 shape.
	legacy := "kind: file\nuri: file:///srv/example/notes/plan.txt\noccurred_at: \"2026-01-02T03:04:05Z\"\nmeta:\n    path: notes/plan.txt\n    size: \"12\"\n"
	if err := os.WriteFile(filepath.Join(ss.DirFor(written.SHA256), "meta.yaml"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	got, src, err := ss.Read(written.SHA256)
	if err != nil {
		t.Fatalf("Read legacy source: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("bytes = %q, want %q", got, data)
	}
	if src.URI != "file:///srv/example/notes/plan.txt" || src.Meta["path"] != "notes/plan.txt" {
		t.Fatalf("legacy metadata not preserved: %+v", src)
	}
	if _, ok := src.Meta["path_hash"]; ok {
		t.Fatalf("legacy record gained a path_hash on read: %+v", src.Meta)
	}
}
