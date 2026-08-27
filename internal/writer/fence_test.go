package writer

import (
	"bytes"
	"os"
	"testing"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

// TestFenceEntryPoint proves writer.Fence is a working queue-backed
// replacement for calling FenceWriter.WriteEntity directly (T0.3): the
// write lands on disk, the returned bytes match what landed, and a
// second write to the same page through the queue is applied cleanly.
func TestFenceEntryPoint(t *testing.T) {
	root := t.TempDir()
	fw := store.NewFenceWriter(root)
	q := NewQueue(nil)
	defer q.Close()

	p := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice-tan"})
	p.Summary = "Runs engineering at Acme."

	path, rendered, err := Fence(q, fw, p)
	if err != nil {
		t.Fatalf("Fence: %v", err)
	}
	if path != fw.PathFor("person", "alice-tan") {
		t.Fatalf("path = %q, want the fence writer's canonical path", path)
	}
	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written page: %v", err)
	}
	if !bytes.Equal(disk, rendered) {
		t.Fatalf("returned bytes do not match what landed on disk:\n--- returned ---\n%s\n--- disk ---\n%s", rendered, disk)
	}

	parsed, err := fw.ParseEntity(path)
	if err != nil {
		t.Fatalf("parse written page: %v", err)
	}
	if parsed.Summary != p.Summary {
		t.Fatalf("summary = %q, want %q", parsed.Summary, p.Summary)
	}

	// A second submit to the same path must be applied, not dropped or
	// interleaved away.
	p2 := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice-tan"})
	p2.Summary = "Runs engineering at Acme; promoted to VP."
	if _, _, err := Fence(q, fw, p2); err != nil {
		t.Fatalf("second Fence: %v", err)
	}
	want, err := fw.RenderEntity(p2)
	if err != nil {
		t.Fatal(err)
	}
	disk2, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(disk2, want) {
		t.Fatalf("second write not applied:\n--- got ---\n%s\n--- want ---\n%s", disk2, want)
	}
}
