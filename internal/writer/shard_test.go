package writer

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

// TestShardEntryPoint proves writer.Shard is a working queue-backed
// replacement for calling ShardStore.Append directly (T0.3): the line
// lands on disk exactly as returned, IDs get derived, and a second
// append to the same shard is appended rather than dropped.
func TestShardEntryPoint(t *testing.T) {
	root := t.TempDir()
	ss := store.NewShardStore(root)
	q := NewQueue(nil)
	defer q.Close()

	c := domain.Claim{
		SubjectSlug: "checking-acct", Predicate: "has_balance", Family: "has_balance",
		Object: "1200.00 usd", Confidence: 0.9, State: domain.StateActive,
		Provenance: domain.Provenance{ObservedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
	}

	path, line, err := Shard(q, ss, c)
	if err != nil {
		t.Fatalf("Shard: %v", err)
	}
	if path != ss.PathFor("checking-acct", "has_balance") {
		t.Fatalf("path = %q, want the shard store's canonical path", path)
	}

	var got domain.Claim
	if err := json.Unmarshal(line, &got); err != nil {
		t.Fatalf("returned line is not valid JSON: %v", err)
	}
	if got.ID == "" {
		t.Fatal("returned line has no derived id")
	}

	disk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read shard file: %v", err)
	}
	if !bytes.Contains(disk, append(append([]byte{}, line...), '\n')) {
		t.Fatalf("returned line not found on disk verbatim:\n--- line ---\n%s\n--- disk ---\n%s", line, disk)
	}

	lines, err := ss.Lines("checking-acct", "has_balance")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].ID != got.ID {
		t.Fatalf("shard has %d lines, want 1 matching id %s: %+v", len(lines), got.ID, lines)
	}

	// A second submit to the same shard must append, not overwrite.
	c2 := c
	c2.Object = "1300.00 usd"
	c2.Provenance.ObservedAt = c.Provenance.ObservedAt.Add(time.Hour)
	if _, _, err := Shard(q, ss, c2); err != nil {
		t.Fatalf("second Shard: %v", err)
	}
	lines, err = ss.Lines("checking-acct", "has_balance")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 {
		t.Fatalf("shard has %d lines after second append, want 2", len(lines))
	}
}
