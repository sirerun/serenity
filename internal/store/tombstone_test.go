package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
)

// sourceTombstoneKind is spelled out rather than referenced so the test
// states the on-disk contract independently of the constant's name.
const sourceTombstoneKind = "source_tombstone"

// TestTombstoneRemovesSourceBytesAndMetaAndRecordsEvent is PRIV-01's
// source half: Tombstone removes the source's bytes and meta.yaml from the
// working tree and records a tombstone event that carries neither the
// source bytes nor its URI.
func TestTombstoneRemovesSourceBytesAndMetaAndRecordsEvent(t *testing.T) {
	root := t.TempDir()
	s := NewSourceStore(root)
	const body = "confidential mail body 91c2"
	const uri = "mail://inbox/confidential-91c2"
	src, err := s.Write([]byte(body), domain.Source{Kind: "email", URI: uri, OccurredAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	ss := NewShardStore(root)
	if err := ss.Append(domain.Claim{
		SubjectSlug: "alice-tan", Predicate: "has_balance", Family: "has_balance",
		Object: "500.00 usd", Confidence: 0.9, State: domain.StateActive,
		Provenance: domain.Provenance{SourceSHA256: src.SHA256, ObservedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}); err != nil {
		t.Fatal(err)
	}

	citing, err := s.Tombstone(src.SHA256, ss)
	if err != nil {
		t.Fatal(err)
	}
	if len(citing) != 1 {
		t.Fatalf("citing claims = %d, want 1", len(citing))
	}
	dir := s.DirFor(src.SHA256)
	for _, name := range []string{"bytes", "meta.yaml"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("%s survived tombstone: %v", name, err)
		}
	}
	if s.Exists(src.SHA256) {
		t.Fatal("tombstoned source still exists")
	}
	all, err := s.All()
	if err != nil {
		t.Fatal(err)
	}
	var events int
	for _, got := range all {
		if got.SHA256 == src.SHA256 {
			t.Fatal("tombstoned source still listed")
		}
		if got.Kind != sourceTombstoneKind {
			continue
		}
		events++
		data, meta, err := s.Read(got.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), src.SHA256) {
			t.Fatalf("tombstone event does not name its target: %s", data)
		}
		if strings.Contains(string(data), body) || strings.Contains(string(data), uri) || strings.Contains(meta.URI, uri) {
			t.Fatal("tombstone event carries source bytes or URI")
		}
	}
	if events != 1 {
		t.Fatalf("tombstone events = %d, want 1", events)
	}
}

// TestTombstoneIsIdempotentAndReserved: a retried tombstone reuses the
// same event, an unknown SHA is ErrSourceNotFound, imports cannot forge a
// tombstone event, and memory facts are refused (forget erases those).
func TestTombstoneIsIdempotentAndReserved(t *testing.T) {
	root := t.TempDir()
	s := NewSourceStore(root)
	src, err := s.Write([]byte("tombstone twice"), domain.Source{Kind: "note", URI: "test://twice"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)
	first, err := s.TombstoneAt(src.SHA256, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.TombstoneAt(src.SHA256, nil, now.Add(time.Hour))
	if err != nil || again.Event.SHA256 != first.Event.SHA256 || !again.Event.OccurredAt.Equal(now) || len(again.Removed) != 2 {
		t.Fatalf("retry: %+v %v", again, err)
	}
	missing := strings.Repeat("ab", 32)
	if _, err := s.TombstoneAt(missing, nil, now); !errors.Is(err, ErrSourceNotFound) {
		t.Fatalf("unknown sha: %v", err)
	}
	event, err := EncodeSourceTombstone(missing)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Write(event, domain.Source{Kind: SourceKindTombstone, URI: SourceTombstoneURI}); err == nil {
		t.Fatal("import forged a tombstone event")
	}
	fact, err := s.WriteMemoryFact(MemoryFactPayload{FormatVersion: MemoryFactFormatVersion, RecordType: SourceKindMemoryFact, LegacyID: 1, Fact: "kept by forget", Provenance: "test", Kind: MemoryFactKindFact, Visibility: MemoryVisibilityWorld, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.TombstoneAt(fact.SHA256, nil, now); err == nil {
		t.Fatal("memory fact tombstoned outside forget")
	}
	if _, _, err := s.Read(fact.SHA256); err != nil {
		t.Fatalf("refused tombstone touched the fact: %v", err)
	}
}
