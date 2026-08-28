package ledger

import (
	"context"
	"errors"
	"testing"
)

func newTestEntry(id string) *Entry {
	return &Entry{
		ID:      id,
		Kind:    KindDecision,
		Title:   "vendor dira at a pinned commit",
		State:   StateAccepted,
		Created: "2026-08-28T00:00:00Z",
		Alternatives: []Alternative{
			{Option: "fork and edit dira", WhyNot: "loses upstream fixes silently"},
		},
		Body: "Because RFC 0001 section 7.3 says so.\n",
	}
}

// TestAddAllocatesLowestUnusedID exercises the writer half of the vendored
// reader/writer contract: Add must allocate the lowest unused id for the
// entry's kind and leave e.ID empty on failure.
func TestAddAllocatesLowestUnusedID(t *testing.T) {
	ctx := context.Background()
	s := newMemoryStore()

	first := newTestEntry("")
	if err := Add(ctx, s, first); err != nil {
		t.Fatalf("Add(first): %v", err)
	}
	if first.ID != "dec-0001" {
		t.Fatalf("first.ID = %q, want dec-0001", first.ID)
	}

	second := newTestEntry("")
	if err := Add(ctx, s, second); err != nil {
		t.Fatalf("Add(second): %v", err)
	}
	if second.ID != "dec-0002" {
		t.Fatalf("second.ID = %q, want dec-0002", second.ID)
	}

	// Add rejects an entry that already carries an id.
	third := newTestEntry("dec-0099")
	if err := Add(ctx, s, third); err == nil {
		t.Fatalf("Add(third) with a pre-set id: want error, got nil")
	}
}

// TestStoreRoundTrip exercises the reader half: Create, Get, List, Put and
// Delete against the vendored Store interface.
func TestStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := newMemoryStore()
	e := newTestEntry("dec-0001")

	if err := s.Create(ctx, e); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.Create(ctx, e); !errors.Is(err, ErrExists) {
		t.Fatalf("second Create: got %v, want ErrExists", err)
	}

	got, err := s.Get(ctx, "dec-0001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != e.Title {
		t.Fatalf("Get title = %q, want %q", got.Title, e.Title)
	}

	infos, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(infos) != 1 || infos[0].ID != "dec-0001" {
		t.Fatalf("List = %+v, want one entry dec-0001", infos)
	}

	got.Title = "updated title"
	if err := s.Put(ctx, got); err != nil {
		t.Fatalf("Put: %v", err)
	}
	reread, err := s.Get(ctx, "dec-0001")
	if err != nil {
		t.Fatalf("Get after Put: %v", err)
	}
	if reread.Title != "updated title" {
		t.Fatalf("Get after Put title = %q, want %q", reread.Title, "updated title")
	}

	if err := s.Delete(ctx, "dec-0001"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, "dec-0001"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after Delete: got %v, want ErrNotFound", err)
	}
}

// TestReadOnlyRefusesWrites is cst-0003 rule 1 at the type level: a Store
// opened through ReadOnly must refuse every mutation without touching the
// wrapped store, per readonly.go's own contract.
func TestReadOnlyRefusesWrites(t *testing.T) {
	ctx := context.Background()
	inner := newMemoryStore()
	ro := ReadOnly(inner)

	if err := ro.Create(ctx, newTestEntry("dec-0001")); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Create on ReadOnly: got %v, want ErrReadOnly", err)
	}
	if err := ro.Put(ctx, newTestEntry("dec-0001")); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Put on ReadOnly: got %v, want ErrReadOnly", err)
	}
	if err := ro.Delete(ctx, "dec-0001"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("Delete on ReadOnly: got %v, want ErrReadOnly", err)
	}

	if infos, err := inner.List(ctx); err != nil || len(infos) != 0 {
		t.Fatalf("inner store was written to through ReadOnly: infos=%+v err=%v", infos, err)
	}

	// Get and List pass through unchanged.
	if err := inner.Create(ctx, newTestEntry("dec-0001")); err != nil {
		t.Fatalf("seeding inner: %v", err)
	}
	if _, err := ro.Get(ctx, "dec-0001"); err != nil {
		t.Fatalf("Get through ReadOnly: %v", err)
	}
	if infos, err := ro.List(ctx); err != nil || len(infos) != 1 {
		t.Fatalf("List through ReadOnly = %+v, %v", infos, err)
	}
}

// TestEncodeDecodeRoundTrip proves the codec half of the reader/writer:
// Encode then Decode recovers an equivalent entry.
func TestEncodeDecodeRoundTrip(t *testing.T) {
	e := newTestEntry("dec-0001")
	e.Tags = []string{"vendoring", "dira"}
	e.Edges = []Edge{{Type: EdgeInforms, To: "int-0001", Note: "context"}}

	data, err := Encode(e)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.ID != e.ID || got.Kind != e.Kind || got.Title != e.Title || got.State != e.State {
		t.Fatalf("Decode round trip = %+v, want id/kind/title/state to match %+v", got, e)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "vendoring" {
		t.Fatalf("Decode tags = %v, want [vendoring dira]", got.Tags)
	}
	if len(got.Edges) != 1 || got.Edges[0].To != "int-0001" {
		t.Fatalf("Decode edges = %+v, want one edge to int-0001", got.Edges)
	}

	// Re-encoding an entry nobody edited reproduces the file byte for byte
	// (the style memo's whole reason to exist).
	again, err := Encode(got)
	if err != nil {
		t.Fatalf("re-Encode: %v", err)
	}
	if string(again) != string(data) {
		t.Fatalf("re-Encode is not byte-identical:\n--- first ---\n%s\n--- second ---\n%s", data, again)
	}
}

// TestValidateRejectsMissingAlternative pins cst-0002/entry.schema.json's
// rule that a non-staged decision must record at least one alternative.
func TestValidateRejectsMissingAlternative(t *testing.T) {
	e := newTestEntry("dec-0001")
	e.Alternatives = nil
	if err := e.Validate(); err == nil {
		t.Fatalf("Validate: want an error for a decision with no alternatives, got nil")
	}

	e.State = StateStaged
	if err := e.Validate(); err != nil {
		t.Fatalf("Validate: a staged decision needs no alternative yet, got %v", err)
	}
}
