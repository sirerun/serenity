package ingest

import (
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/extract"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// newTestWriter builds a Writer over a fresh temp brain root with a live
// queue; the caller must defer Close().
func newTestWriter(t *testing.T) (*Writer, func()) {
	t.Helper()
	root := t.TempDir()
	q := writer.NewQueue(nil)
	w := New(q, store.NewFenceWriter(root), store.NewShardStore(root), config.Default())
	return w, q.Close
}

func obs(subject, predicate, object, sourceSHA256, span string, confidence float64, createdAt time.Time) domain.Observation {
	return domain.Observation{
		ID:           "unused-in-ingest", // T1.8 stamps a real one; ingest never reads it
		SubjectSlug:  subject,
		Predicate:    predicate,
		Object:       object,
		Confidence:   confidence,
		Model:        "anthropic/claude-fake@2026-08-01",
		SourceSHA256: sourceSHA256,
		Span:         span,
		CreatedAt:    createdAt,
	}
}

// TestClaimFromObservation pins acc line 3: every written claim carries
// SourceSHA256#span, model@version, observed_at. This is the pure
// conversion, checked with no I/O.
func TestClaimFromObservation(t *testing.T) {
	when := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	o := obs("alice-tan", "works_at", "Acme  Corp", "deadbeefcafe0123456789", "10-42", 0.82, when)

	c := ClaimFromObservation(o)

	if c.Provenance.SourceSHA256 != o.SourceSHA256 {
		t.Fatalf("Provenance.SourceSHA256 = %q, want %q", c.Provenance.SourceSHA256, o.SourceSHA256)
	}
	if c.Provenance.Span != o.Span {
		t.Fatalf("Provenance.Span = %q, want %q", c.Provenance.Span, o.Span)
	}
	if c.Provenance.Model != o.Model {
		t.Fatalf("Provenance.Model = %q, want %q", c.Provenance.Model, o.Model)
	}
	if !c.Provenance.ObservedAt.Equal(o.CreatedAt) {
		t.Fatalf("Provenance.ObservedAt = %v, want %v", c.Provenance.ObservedAt, o.CreatedAt)
	}
	if c.Provenance.Actor != "machine" {
		t.Fatalf("Provenance.Actor = %q, want %q", c.Provenance.Actor, "machine")
	}
	if want := "deadbeef#10-42"; c.SourceRef != want {
		t.Fatalf("SourceRef = %q, want %q", c.SourceRef, want)
	}
	if c.State != domain.StateActive {
		t.Fatalf("State = %q, want active (trust 0 never mints any other state)", c.State)
	}
	if c.Family != "works_at" {
		t.Fatalf("Family = %q, want the predicate itself", c.Family)
	}
	if want := store.NormalizeKey("Acme  Corp"); c.ObjectKey != want {
		t.Fatalf("ObjectKey = %q, want %q", c.ObjectKey, want)
	}
	wantID := store.DerivedID("alice-tan", "works_at", c.ObjectKey, "", o.SourceSHA256, store.DefaultIDWidth)
	if c.ID != wantID {
		t.Fatalf("ID = %q, want %q (derived exactly as §7.2 specifies)", c.ID, wantID)
	}
}

// TestWrite_FenceTierDoubleIngestNoop pins acc line 1 for a fence-tier
// family: ingesting one source twice adds zero claims.
func TestWrite_FenceTierDoubleIngestNoop(t *testing.T) {
	w, closeQ := newTestWriter(t)
	defer closeQ()

	o := obs("alice-tan", "works_at", "acme", "src-sha-aaa111", "0-40", 0.9, time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC))

	stats, err := w.Write([]domain.Observation{o})
	if err != nil {
		t.Fatalf("first Write: %v", err)
	}
	if stats != (Stats{Written: 1, Skipped: 0}) {
		t.Fatalf("first Write stats = %+v, want {Written:1 Skipped:0}", stats)
	}

	path := w.Fence.PathFor(DefaultEntityType, "alice-tan")
	page, err := w.Fence.ParseEntity(path)
	if err != nil {
		t.Fatalf("parse page after first write: %v", err)
	}
	if len(page.Claims) != 1 {
		t.Fatalf("claims after first write = %d, want 1", len(page.Claims))
	}

	// Re-ingest: same source, same span/subject/predicate/object -- as
	// T1.8's deterministic pipeline would reproduce -- but a different
	// wall-clock CreatedAt, proving the no-op is id-derived, not
	// timestamp-derived.
	o2 := o
	o2.CreatedAt = time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)

	stats2, err := w.Write([]domain.Observation{o2})
	if err != nil {
		t.Fatalf("second Write: %v", err)
	}
	if stats2 != (Stats{Written: 0, Skipped: 1}) {
		t.Fatalf("second Write stats = %+v, want {Written:0 Skipped:1}", stats2)
	}

	page2, err := w.Fence.ParseEntity(path)
	if err != nil {
		t.Fatalf("parse page after second write: %v", err)
	}
	if len(page2.Claims) != 1 {
		t.Fatalf("claims after re-ingest = %d, want 1 (zero new claims)", len(page2.Claims))
	}
}

// TestWrite_ShardTierDoubleIngestNoop is the shard-tier twin of the above.
func TestWrite_ShardTierDoubleIngestNoop(t *testing.T) {
	w, closeQ := newTestWriter(t)
	defer closeQ()

	o := obs("checking-acct", "has_balance", "1234.56 usd", "src-sha-bbb222", "0-20", 0.95, time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC))

	if _, err := w.Write([]domain.Observation{o}); err != nil {
		t.Fatalf("first Write: %v", err)
	}
	lines, err := w.Shard.Lines("checking-acct", "has_balance")
	if err != nil {
		t.Fatalf("Lines after first write: %v", err)
	}
	if len(lines) != 1 {
		t.Fatalf("shard lines after first write = %d, want 1", len(lines))
	}

	o2 := o
	o2.CreatedAt = time.Date(2026, 8, 29, 9, 0, 0, 0, time.UTC)
	stats2, err := w.Write([]domain.Observation{o2})
	if err != nil {
		t.Fatalf("second Write: %v", err)
	}
	if stats2 != (Stats{Written: 0, Skipped: 1}) {
		t.Fatalf("second Write stats = %+v, want {Written:0 Skipped:1}", stats2)
	}
	lines2, err := w.Shard.Lines("checking-acct", "has_balance")
	if err != nil {
		t.Fatalf("Lines after second write: %v", err)
	}
	if len(lines2) != 1 {
		t.Fatalf("shard lines after re-ingest = %d, want 1 (zero new claims)", len(lines2))
	}
}

// TestWrite_TwoSourcesTwoIDs pins acc line 2: the same logical claim from
// two sources yields two ids -- corroboration, not collapsed, at trust 0.
func TestWrite_TwoSourcesTwoIDs(t *testing.T) {
	w, closeQ := newTestWriter(t)
	defer closeQ()

	when := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	a := obs("alice-tan", "works_at", "acme", "src-sha-source-a", "0-10", 0.9, when)
	b := obs("alice-tan", "works_at", "acme", "src-sha-source-b", "5-15", 0.88, when)

	stats, err := w.Write([]domain.Observation{a, b})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if stats != (Stats{Written: 2, Skipped: 0}) {
		t.Fatalf("stats = %+v, want {Written:2 Skipped:0} -- two sources must both land", stats)
	}

	path := w.Fence.PathFor(DefaultEntityType, "alice-tan")
	page, err := w.Fence.ParseEntity(path)
	if err != nil {
		t.Fatalf("parse page: %v", err)
	}
	if len(page.Claims) != 2 {
		t.Fatalf("claims = %d, want 2", len(page.Claims))
	}
	if page.Claims[0].ID == page.Claims[1].ID {
		t.Fatalf("both claims share id %q, want distinct ids (different source_ref inputs)", page.Claims[0].ID)
	}
	// The fence round-trip does not persist ObjectKey/Provenance (only
	// shard-tier claims carry full Provenance JSON on disk, per
	// internal/store/source.go's Tombstone doc comment); check identity on
	// what a parsed fence row actually carries: predicate and object text.
	for _, c := range page.Claims {
		if c.SubjectSlug != "alice-tan" || c.Predicate != "works_at" || c.Object != "acme" {
			t.Fatalf("claim %+v is not the same logical claim as expected", c)
		}
	}
}

// TestWrite_RejectsSubThreshold proves this package never silently writes
// a distill-eligible observation to a fence or shard -- that split is
// T1.8's Result.Ready/Result.Distill contract, enforced again here.
func TestWrite_RejectsSubThreshold(t *testing.T) {
	w, closeQ := newTestWriter(t)
	defer closeQ()

	below := obs("alice-tan", "works_at", "acme", "src-sha-ccc333", "0-10", extract.DistillThreshold-0.01, time.Now())
	stats, err := w.Write([]domain.Observation{below})
	if err == nil {
		t.Fatalf("want an error for a sub-threshold observation, got stats %+v", stats)
	}
	if stats.Written != 0 {
		t.Fatalf("Written = %d, want 0 -- a rejected batch must write nothing", stats.Written)
	}
}

// TestWrite_MultipleFamiliesSameEntity proves the per-call page cache
// accumulates claims onto one page rather than each write clobbering the
// last (a naive parse-per-observation-without-cache-invalidation bug).
func TestWrite_MultipleFamiliesSameEntity(t *testing.T) {
	w, closeQ := newTestWriter(t)
	defer closeQ()

	when := time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
	worksAt := obs("alice-tan", "works_at", "acme", "src-sha-ddd444", "0-10", 0.9, when)
	committed := obs("alice-tan", "committed_to", "security review by 2026-05-01", "src-sha-eee555", "20-60", 0.85, when)

	stats, err := w.Write([]domain.Observation{worksAt, committed})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if stats != (Stats{Written: 2, Skipped: 0}) {
		t.Fatalf("stats = %+v, want {Written:2 Skipped:0}", stats)
	}

	page, err := w.Fence.ParseEntity(w.Fence.PathFor(DefaultEntityType, "alice-tan"))
	if err != nil {
		t.Fatalf("parse page: %v", err)
	}
	if len(page.Claims) != 2 {
		t.Fatalf("claims = %d, want 2 (both families on one page)", len(page.Claims))
	}
}

// TestWrite_EntityTypeOverride proves the EntityType hook is honored.
func TestWrite_EntityTypeOverride(t *testing.T) {
	w, closeQ := newTestWriter(t)
	defer closeQ()
	w.EntityType = func(slug string) string { return "person" }

	o := obs("alice-tan", "works_at", "acme", "src-sha-fff666", "0-10", 0.9, time.Now())
	if _, err := w.Write([]domain.Observation{o}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := w.Fence.ParseEntity(w.Fence.PathFor("person", "alice-tan")); err != nil {
		t.Fatalf("expected page under the overridden type folder: %v", err)
	}
}
