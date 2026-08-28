package direction

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/dira/ledger"
	diraschema "github.com/sirerun/serenity/internal/dira/schema"
	"github.com/sirerun/serenity/internal/writer"
)

// newTestStore returns a Store rooted at a fresh temp dir with its own
// writer queue, and registers the queue's Close for test cleanup.
func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	root := t.TempDir()
	q := writer.NewQueue(nil)
	t.Cleanup(q.Close)
	return NewStore(root, q), root
}

func stagedDraft(id, title string) *ledger.Entry {
	return &ledger.Entry{
		ID:      id,
		Kind:    ledger.KindDecision,
		Title:   title,
		State:   ledger.StateStaged,
		Created: "2026-08-28T00:00:00Z",
		Alternatives: []ledger.Alternative{
			{Option: "do not adopt this", WhyNot: "the floor every draft carries until confirmed"},
		},
		Body: "Because the interview wizard drafted it.\n",
	}
}

// validateAgainstVendoredSchema is the direct acceptance check: the file
// Confirm/Supersede produced must validate against the vendored
// entry.schema.json, not just against Entry.Validate's Go-side mirror of
// it. Production code in this package never imports internal/dira/schema
// (schema.go's own doc explains why: compiling the JSON Schema costs
// milliseconds and ~21,700 allocations, a cost dira's own command path
// refuses to pay on every invocation) -- but a test proving the
// acceptance criterion is exactly the place that cost belongs.
func validateAgainstVendoredSchema(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	v, err := diraschema.NewValidator()
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	if err := v.Validate(data); err != nil {
		t.Fatalf("%s does not validate against entry.schema.json: %v", path, err)
	}
}

func TestCreateDraftRequiresStagedState(t *testing.T) {
	s, _ := newTestStore(t)
	e := stagedDraft("", "not actually staged")
	e.State = ledger.StateAccepted
	if err := s.CreateDraft(context.Background(), e); err == nil {
		t.Fatal("CreateDraft with state=accepted: want error, got nil")
	}
	if infos, err := s.List(context.Background()); err != nil || len(infos) != 0 {
		t.Fatalf("a rejected draft must not land on disk: infos=%+v err=%v", infos, err)
	}
}

func TestCreateDraftAllocatesIDAndWritesStagedFile(t *testing.T) {
	ctx := context.Background()
	s, root := newTestStore(t)
	e := stagedDraft("", "vendor dira at a pinned commit")

	if err := s.CreateDraft(ctx, e); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if e.ID != "dec-0001" {
		t.Fatalf("e.ID = %q, want dec-0001 (lowest unused id)", e.ID)
	}

	path := filepath.Join(root, ".dira", "entries", "dec-0001.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("staged entry file missing: %v", err)
	}
	validateAgainstVendoredSchema(t, path)

	got, err := s.Get(ctx, "dec-0001")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != ledger.StateStaged {
		t.Fatalf("got.State = %q, want staged", got.State)
	}
}

// TestConfirmWritesValidatingAcceptedEntry is the T3.3 acc's first clause:
// confirm writes .dira/entries/<id>.md that validates against the
// vendored entry schema.
func TestConfirmWritesValidatingAcceptedEntry(t *testing.T) {
	ctx := context.Background()
	s, root := newTestStore(t)
	e := stagedDraft("", "adopt the writer queue for precepts")
	if err := s.CreateDraft(ctx, e); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	confirmed, err := s.Confirm(ctx, e.ID, "david", now)
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if confirmed.State != ledger.StateAccepted {
		t.Fatalf("confirmed.State = %q, want accepted", confirmed.State)
	}
	if confirmed.ConfirmedBy != "david" {
		t.Fatalf("confirmed.ConfirmedBy = %q, want david", confirmed.ConfirmedBy)
	}
	if confirmed.Updated != "2026-08-28T12:00:00Z" {
		t.Fatalf("confirmed.Updated = %q, want 2026-08-28T12:00:00Z", confirmed.Updated)
	}
	// Confirm must not touch anything but state/updated/confirmed_by.
	if confirmed.Title != e.Title || confirmed.Body != e.Body || len(confirmed.Alternatives) != 1 {
		t.Fatalf("Confirm changed fields it must not: %+v", confirmed)
	}

	path := filepath.Join(root, ".dira", "entries", e.ID+".md")
	validateAgainstVendoredSchema(t, path)
}

// TestSecondConfirmErrors is the T3.3 acc's second clause: a second
// confirm of the same id errors.
func TestSecondConfirmErrors(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)
	e := stagedDraft("", "confirm once only")
	if err := s.CreateDraft(ctx, e); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	now := time.Now().UTC()
	if _, err := s.Confirm(ctx, e.ID, "david", now); err != nil {
		t.Fatalf("first Confirm: %v", err)
	}
	if _, err := s.Confirm(ctx, e.ID, "david", now); err == nil {
		t.Fatal("second Confirm: want error, got nil")
	}
}

func TestConfirmRequiresAtLeastOneAlternative(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)
	e := stagedDraft("", "no floor recorded")
	e.Alternatives = nil // valid while staged (entry.go's staged exemption)
	if err := s.CreateDraft(ctx, e); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if _, err := s.Confirm(ctx, e.ID, "david", time.Now().UTC()); err == nil {
		t.Fatal("Confirm with zero alternatives: want error, got nil")
	}
	got, err := s.Get(ctx, e.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != ledger.StateStaged {
		t.Fatalf("a rejected confirm must leave the entry staged, got %q", got.State)
	}
}

func TestConfirmUnknownIDErrors(t *testing.T) {
	s, _ := newTestStore(t)
	if _, err := s.Confirm(context.Background(), "dec-9999", "david", time.Now().UTC()); err == nil {
		t.Fatal("Confirm on an unknown id: want error, got nil")
	}
}

// TestSupersedeFlipsOldAndLinksNew is the T3.3 acc's third clause:
// supersede flips the old entry to superseded and links the new one.
func TestSupersedeFlipsOldAndLinksNew(t *testing.T) {
	ctx := context.Background()
	s, root := newTestStore(t)
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)

	original := stagedDraft("", "spend ceiling is $50/month")
	if err := s.CreateDraft(ctx, original); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if _, err := s.Confirm(ctx, original.ID, "david", now); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	replacement := &ledger.Entry{
		Kind:  ledger.KindDecision,
		Title: "spend ceiling is $100/month",
		State: ledger.StateAccepted,
		Alternatives: []ledger.Alternative{
			{Option: "keep the $50 ceiling", WhyNot: "outgrown by current usage"},
		},
		Body: "Because usage has grown past the old ceiling.\n",
	}

	old, created, err := s.Supersede(ctx, original.ID, replacement, now)
	if err != nil {
		t.Fatalf("Supersede: %v", err)
	}
	if old.State != ledger.StateSuperseded {
		t.Fatalf("old.State = %q, want superseded", old.State)
	}
	if old.Title != "spend ceiling is $50/month" {
		t.Fatalf("Supersede edited the old entry's title: got %q", old.Title)
	}
	if created.ID == "" || created.ID == original.ID {
		t.Fatalf("created.ID = %q, want a fresh id distinct from %q", created.ID, original.ID)
	}

	var supersedesEdge *ledger.Edge
	for i := range created.Edges {
		if created.Edges[i].Type == ledger.EdgeSupersedes {
			supersedesEdge = &created.Edges[i]
		}
	}
	if supersedesEdge == nil || supersedesEdge.To != original.ID {
		t.Fatalf("created entry supersedes edge = %+v, want it to point at %q", created.Edges, original.ID)
	}

	// Both files on disk validate against the vendored schema.
	validateAgainstVendoredSchema(t, filepath.Join(root, ".dira", "entries", original.ID+".md"))
	validateAgainstVendoredSchema(t, filepath.Join(root, ".dira", "entries", created.ID+".md"))

	reread, err := s.Get(ctx, original.ID)
	if err != nil {
		t.Fatalf("Get(original) after Supersede: %v", err)
	}
	if reread.State != ledger.StateSuperseded {
		t.Fatalf("reread.State = %q, want superseded", reread.State)
	}
}

func TestSupersedeRejectsAStagedPredecessor(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)
	draft := stagedDraft("", "still staged, nothing to supersede yet")
	if err := s.CreateDraft(ctx, draft); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	replacement := &ledger.Entry{
		Kind:  ledger.KindDecision,
		Title: "replacement",
		State: ledger.StateAccepted,
		Alternatives: []ledger.Alternative{
			{Option: "leave it staged", WhyNot: "not a real alternative"},
		},
	}
	if _, _, err := s.Supersede(ctx, draft.ID, replacement, time.Now().UTC()); err == nil {
		t.Fatal("Supersede on a staged predecessor: want error, got nil")
	}
}

func TestSupersedeRejectsAReplacementThatAlreadyHasAnID(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)
	original := stagedDraft("", "already has a successor id")
	if err := s.CreateDraft(ctx, original); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if _, err := s.Confirm(ctx, original.ID, "david", time.Now().UTC()); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	replacement := &ledger.Entry{
		ID:    "dec-9999",
		Kind:  ledger.KindDecision,
		Title: "pre-assigned id",
		State: ledger.StateAccepted,
		Alternatives: []ledger.Alternative{
			{Option: "allocate normally", WhyNot: "test exercises the rejection"},
		},
	}
	if _, _, err := s.Supersede(ctx, original.ID, replacement, time.Now().UTC()); err == nil {
		t.Fatal("Supersede with a pre-assigned replacement id: want error, got nil")
	}
}

// TestConfirmedEntryRoundTripsUnmodifiedThroughDirasOwnReader is the
// scoped equivalent of the acc's "dira why <id> on the fixture repo
// prints the entry unmodified" clause. T3.1 vendored only dira's library
// packages (schema, ledger, frontmatter), not the dira CLI binary itself
// -- the real CLI-conformance wiring is T3.14, an un-landed sibling task.
// `dira why` is a pure read: it decodes an entry and prints it back
// without writing anything, so the library-level equivalent this proves
// is that internal/dira/ledger's own reader (Decode) recovers exactly
// what Confirm/Supersede wrote, byte for byte on re-encode -- the same
// round-trip guarantee TestEncodeDecodeRoundTrip pins in the vendored
// package itself.
func TestConfirmedEntryRoundTripsUnmodifiedThroughDirasOwnReader(t *testing.T) {
	ctx := context.Background()
	s, root := newTestStore(t)
	e := stagedDraft("", "why must print this unmodified")
	if err := s.CreateDraft(ctx, e); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if _, err := s.Confirm(ctx, e.ID, "david", time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	path := filepath.Join(root, ".dira", "entries", e.ID+".md")
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	decoded, err := ledger.Decode(onDisk)
	if err != nil {
		t.Fatalf("ledger.Decode: %v", err)
	}
	if decoded.ID != e.ID || decoded.Title != e.Title || decoded.State != ledger.StateAccepted {
		t.Fatalf("decoded = %+v, want a faithful read of the confirmed entry", decoded)
	}

	// dira's own reader must reproduce the file byte for byte on
	// re-encode -- "prints ... unmodified" holds only if nothing was
	// silently reflowed or dropped on the way through.
	reencoded, err := ledger.Encode(decoded)
	if err != nil {
		t.Fatalf("ledger.Encode: %v", err)
	}
	if string(reencoded) != string(onDisk) {
		t.Fatalf("re-encoding the entry dira's reader decoded is not byte-identical:\n--- on disk ---\n%s\n--- re-encoded ---\n%s", onDisk, reencoded)
	}
}

func TestDeleteRemovesEntry(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestStore(t)
	e := stagedDraft("", "deletable")
	if err := s.CreateDraft(ctx, e); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if err := s.Delete(ctx, e.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(ctx, e.ID); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("Get after Delete: got %v, want ErrNotFound", err)
	}
	if err := s.Delete(ctx, e.ID); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("second Delete: got %v, want ErrNotFound", err)
	}
}

// TestStoreImplementsLedgerStore is a compile-time-ish assertion kept as a
// runtime test so a signature drift in ledger.Store fails here with a
// clear message instead of a cryptic error at some other call site.
func TestStoreImplementsLedgerStore(t *testing.T) {
	var _ ledger.Store = (*Store)(nil)
}
