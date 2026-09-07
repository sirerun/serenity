// Independent T4.20 regressions: all four reached runtime failures against
// the original implementation before the storage repair.
package store_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// Two separately committed branches can legitimately allocate the same legacy
// number. A merged projection must reject ambiguity rather than pick a map entry.
func TestMemoryProjectionRejectsMergedLegacyIDCollision(t *testing.T) {
	root := t.TempDir()
	first := independentMemoryPayload("first branch fact", 7)
	second := independentMemoryPayload("second branch fact", 7)
	independentPublishMemoryFixture(t, root, first)
	independentPublishMemoryFixture(t, root, second)
	if _, err := store.LoadMemoryProjection(store.NewSourceStore(root)); err == nil {
		t.Fatal("merged distinct facts with duplicate legacy ID were accepted")
	}
}

// Exercise actual writes so matching keys cannot silently collapse distinct
// attributed inputs. This does not prescribe an allocator or hashing algorithm.
func TestMemoryRememberPreservesDedupTupleAndExpiryPrecision(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	base := writer.RememberInput{Fact: "a", Provenance: "c", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld}
	nulA, nulB := base, base
	nulA.Fact = "a\x00b"
	nulB.Provenance = "b\x00c"
	early, late := now.Add(time.Hour+100*time.Millisecond), now.Add(time.Hour+900*time.Millisecond)
	ttlA, ttlB := base, base
	ttlA.ValidUntil, ttlB.ValidUntil = &early, &late
	for _, tc := range []struct {
		name string
		a, b writer.RememberInput
	}{
		{"embedded NUL preserves attribution boundary", nulA, nulB},
		{"subsecond expiry remains distinct", ttlA, ttlB},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			q := writer.NewQueue(nil)
			defer q.Close()
			sources := store.NewSourceStore(root)
			w := writer.MemoryFact{Queue: q, Sources: sources}
			a, err := w.Remember(tc.a, now)
			if err != nil {
				t.Fatal(err)
			}
			b, err := w.Remember(tc.b, now)
			if err != nil {
				t.Fatal(err)
			}
			if !a.Inserted || !b.Inserted || a.Record.SHA256 == b.Record.SHA256 {
				t.Fatal("distinct attributed inputs collapsed into one record")
			}
			again, err := w.Remember(tc.a, now)
			if err != nil {
				t.Fatal(err)
			}
			if again.Inserted || again.Record.SHA256 != a.Record.SHA256 {
				t.Fatal("identical input no longer deduplicates to its original identity")
			}
			projection, err := store.LoadMemoryProjection(store.NewSourceStore(root))
			if err != nil {
				t.Fatal(err)
			}
			if len(projection.All()) != 2 {
				t.Fatalf("reopened projection contains %d records; want 2", len(projection.All()))
			}
		})
	}
}

// Noncanonical directory entries may be rejected or ignored, but cannot panic
// or enter the authoritative source enumeration. These are not staging names.
func TestMemorySourceEnumerationRejectsInvalidNamespace(t *testing.T) {
	for _, tc := range []struct{ name, prefix, sha string }{
		{"short name", "a", "a"},
		{"invalid hex", "zz", "zz00000000000000000000000000000000000000000000000000000000000000"},
		{"wrong prefix", "ff", "aa00000000000000000000000000000000000000000000000000000000000000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "brain", "sources", tc.prefix, tc.sha)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "meta.yaml"), []byte("kind: memory_fact\nuri: memory://fact\n"), 0644); err != nil {
				t.Fatal(err)
			}
			// A panic naturally fails this subtest; do not recover and convert it to a pass.
			records, err := store.NewSourceStore(root).All()
			if err == nil && len(records) != 0 {
				t.Fatalf("invalid namespace entered source enumeration: %+v", records)
			}
		})
	}
}

func TestMemoryProjectionRejectsContentHashMismatch(t *testing.T) {
	root := t.TempDir()
	original := independentMemoryPayload("original attributed fact", 9)
	sha := independentPublishMemoryFixture(t, root, original)
	sources := store.NewSourceStore(root)
	if _, err := store.LoadMemoryProjection(sources); err != nil {
		t.Fatalf("valid fixture rejected before corruption: %v", err)
	}
	changed := original
	changed.Fact = "different bytes retaining the original identity"
	data, err := store.EncodeMemoryFact(changed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sources.DirFor(sha), "bytes"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadMemoryProjection(sources); err == nil {
		t.Fatal("changed canonical bytes retained their old content identity")
	}
}

func independentMemoryPayload(fact string, id int64) store.MemoryFactPayload {
	return store.MemoryFactPayload{FormatVersion: store.MemoryFactFormatVersion, RecordType: store.SourceKindMemoryFact, LegacyID: id, Fact: fact, Provenance: "independent canonical fixture", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld, CreatedAt: time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)}
}

// Assemble valid canonical files as they would arrive in a Git merge, bypassing
// allocation intentionally. The production codec still defines valid bytes.
func independentPublishMemoryFixture(t *testing.T, root string, p store.MemoryFactPayload) string {
	t.Helper()
	data, err := store.EncodeMemoryFact(p)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	sha := hex.EncodeToString(digest[:])
	dir := filepath.Join(root, "brain", "sources", sha[:2], sha)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bytes"), data, 0644); err != nil {
		t.Fatal(err)
	}
	meta := fmt.Sprintf("kind: memory_fact\nuri: memory://fact\noccurred_at: %q\n", p.CreatedAt.Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(dir, "meta.yaml"), []byte(meta), 0644); err != nil {
		t.Fatal(err)
	}
	return sha
}
