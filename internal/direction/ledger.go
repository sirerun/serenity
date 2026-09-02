package direction

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/writer"
)

// Store is the filesystem-backed ledger.Store for .dira/entries/<id>.md.
// dira's own local backend (internal/ledger/local) was deliberately not
// vendored (T3.1); this is Serenity's replacement, per ADR 008. Every
// mutation is submitted through a writer.Queue exactly as FenceWriter and
// ShardStore are wrapped by internal/writer's Fence and Shard for brain/
// canonical files (ADR 004): concurrent Create/Put/Delete calls, even to
// different ids, never interleave. Per the precept-immutability invariant
// (RFC 0001 §7.3/§14, T3.12's AST gate), internal/direction is the only
// package permitted to touch .dira/ at all -- this file is that boundary,
// and CreateDraft/Confirm/Supersede below are its lifecycle, not raw
// Create/Put/Delete: "active precepts are never edited, only superseded".
type Store struct {
	root  string
	queue *writer.Queue
}

// ErrReadOnly is returned by every mutator of a Store built with a nil
// writer queue (ADR 012 §3): the read-only ledger handle pkg/serenity's
// facade opens. Callers branch on it with errors.Is.
var ErrReadOnly = errors.New("direction: store is read-only (opened with no writer queue)")

// NewStore returns a Store rooted at root (a brain repo root; entries live
// under root/.dira/entries). Every write this type performs is submitted
// through queue, never issued directly, so a caller sharing one Queue
// across subsystems gets the same per-path ordering guarantee RFC 0001
// §7.7 asks of every canonical writer. A nil queue yields a read-only
// Store: Get, List and PathFor work, and every mutator (Create, Put,
// Delete, CreateDraft, Confirm, Supersede, Answer) returns an error
// wrapping ErrReadOnly instead of dereferencing nil -- the handle an
// embedding consumer holds (ADR 012: one brain-writer process per brain,
// readers everywhere else).
func NewStore(root string, queue *writer.Queue) *Store {
	return &Store{root: root, queue: queue}
}

// writable returns ErrReadOnly when the Store has no writer queue. Every
// mutator calls it first, before any read or Submit, so a read-only
// handle never reaches the queue and never partially applies a
// multi-step lifecycle transition (Confirm, Supersede, Answer).
func (s *Store) writable(op string) error {
	if s.queue == nil {
		return fmt.Errorf("direction: %s: %w", op, ErrReadOnly)
	}
	return nil
}

// PathFor returns the canonical file path for an entry id.
func (s *Store) PathFor(id string) string {
	return filepath.Join(s.root, ".dira", "entries", id+".md")
}

// Get reads one entry by id, satisfying ledger.Store.
func (s *Store) Get(_ context.Context, id string) (*ledger.Entry, error) {
	path := s.PathFor(id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ledger.ErrNotFound, id)
		}
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return ledger.DecodeStored(data, versionOf(info))
}

// List returns every entry in the ledger, id and version only, sorted by
// id -- it reads no entry bodies (ledger.Store's contract). A ledger with
// no entries directory yet is an empty slice and a nil error, matching
// ledger.Store's "an empty ledger is an empty slice" rule.
func (s *Store) List(_ context.Context) ([]ledger.EntryInfo, error) {
	dir := filepath.Join(s.root, ".dira", "entries")
	des, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []ledger.EntryInfo
	for _, de := range des {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".md") {
			continue
		}
		info, err := de.Info()
		if err != nil {
			return nil, err
		}
		out = append(out, ledger.EntryInfo{
			ID:      strings.TrimSuffix(de.Name(), ".md"),
			Version: versionOf(info),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Create writes a new entry through the writer queue, failing with an
// error wrapping ledger.ErrExists if the id is already taken. It is
// exclusive by construction (O_EXCL) -- the same native primitive dira's
// own local backend uses -- which is what lets ledger.Add (write.go) retry
// the next candidate id on a losing race instead of clobbering the winner.
func (s *Store) Create(_ context.Context, e *ledger.Entry) error {
	if err := s.writable("create"); err != nil {
		return err
	}
	path := s.PathFor(e.ID)
	res := s.queue.Submit(writer.Job{Path: path, Render: func() ([]byte, error) {
		data, err := ledger.Encode(e)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			if errors.Is(err, os.ErrExist) {
				return nil, fmt.Errorf("%w: %s", ledger.ErrExists, e.ID)
			}
			return nil, err
		}
		defer func() { _ = f.Close() }()
		if _, err := f.Write(data); err != nil {
			return nil, err
		}
		return data, f.Close()
	}})
	return res.Err
}

// Put replaces an entry, whatever is already on disk, through the writer
// queue. It satisfies ledger.Store, but nothing outside this file calls it
// directly: Confirm and Supersede are the only callers, and each is held
// to a narrower rule than Put itself expresses -- Confirm only ever flips
// state staged->accepted and Supersede only ever flips state->superseded
// on a copy that carries every other field unchanged from what Get
// returned, so no caller reachable from this package edits an entry's
// title, body, alternatives or edges once it exists.
func (s *Store) Put(_ context.Context, e *ledger.Entry) error {
	if err := s.writable("put"); err != nil {
		return err
	}
	path := s.PathFor(e.ID)
	res := s.queue.Submit(writer.Job{Path: path, Render: func() ([]byte, error) {
		data, err := ledger.Encode(e)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		return data, os.WriteFile(path, data, 0o644)
	}})
	return res.Err
}

// Delete removes an entry through the writer queue, satisfying
// ledger.Store (E7 parity). Nothing in this package's own lifecycle
// (CreateDraft/Confirm/Supersede) ever calls it: precepts are
// supersede-only and never deleted (RFC 0001 §7.3).
func (s *Store) Delete(_ context.Context, id string) error {
	if err := s.writable("delete"); err != nil {
		return err
	}
	path := s.PathFor(id)
	res := s.queue.Submit(writer.Job{Path: path, Render: func() ([]byte, error) {
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("%w: %s", ledger.ErrNotFound, id)
			}
			return nil, err
		}
		return nil, os.Remove(path)
	}})
	return res.Err
}

// versionOf derives the opaque Store version from a file's mtime and
// size -- dira's documented local-backend contract for ledger.EntryInfo.
func versionOf(info os.FileInfo) string {
	return fmt.Sprintf("%d-%d", info.ModTime().UnixNano(), info.Size())
}

// CreateDraft writes e as a new staged precept, allocating its id via
// ledger.Add (write.go's collision-free allocator). e must already carry
// state=staged -- the only state ledger.Kind.States() permits before a
// human has decided anything -- and Created stamped by the caller: the
// clock belongs to whoever is drafting, not to this package (see Add's own
// doc in write.go). Only ledger.KindDecision supports StateStaged, so a
// draft of any other kind is rejected here before it ever reaches Add.
func (s *Store) CreateDraft(ctx context.Context, e *ledger.Entry) error {
	if err := s.writable("create draft"); err != nil {
		return err
	}
	if e.State != ledger.StateStaged {
		return fmt.Errorf("direction: create draft: state must be %q, got %q", ledger.StateStaged, e.State)
	}
	return ledger.Add(ctx, s, e)
}

// Confirm transitions a staged precept to accepted: the only path that may
// set state=accepted. It rejects an entry that is not currently staged, so
// a second Confirm of the same id errors rather than repeating silently.
// The entry must already carry at least one alternative -- T3.4's
// interview always seeds the "do not adopt this" floor -- because Confirm
// does not invent one: a machine-authored why_not would be exactly the
// dishonest floor entry.go's staged exemption exists to avoid. Only state,
// updated and confirmed_by change; title, body, tags, edges and
// alternatives are carried through unmodified from what Get returned.
func (s *Store) Confirm(ctx context.Context, id, confirmedBy string, now time.Time) (*ledger.Entry, error) {
	if err := s.writable("confirm"); err != nil {
		return nil, err
	}
	e, err := s.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("direction: confirm %s: %w", id, err)
	}
	if e.State != ledger.StateStaged {
		return nil, fmt.Errorf("direction: confirm %s: state is %q, not staged -- already confirmed or not a draft", id, e.State)
	}
	if len(e.Alternatives) == 0 {
		return nil, fmt.Errorf("direction: confirm %s: no alternatives recorded; a decision leaving staged needs at least one", id)
	}
	e.State = ledger.StateAccepted
	e.ConfirmedBy = confirmedBy
	e.Updated = now.UTC().Format(time.RFC3339)
	if err := s.Put(ctx, e); err != nil {
		return nil, fmt.Errorf("direction: confirm %s: %w", id, err)
	}
	return e, nil
}

// Supersede replaces oldID with a newly-allocated entry, the one rule
// precepts obey everywhere in this system (RFC 0001 §7.3/§7.6): an active
// precept is never edited, only superseded. next must not carry an id yet
// (Supersede allocates one via ledger.Add) and is written with a
// `supersedes` edge back to oldID, so the successor -- not the
// predecessor -- is where the link lives; that is the one dira edge type
// this relationship has (write.go/entry.go define no reverse
// `superseded_by`). The old entry is then replaced by a copy carrying
// exactly two changes, state and updated: title, body, tags, edges and
// alternatives are the same bytes Get returned, which is what "never
// edited, only superseded" means in code rather than in a comment.
func (s *Store) Supersede(ctx context.Context, oldID string, next *ledger.Entry, now time.Time) (old, created *ledger.Entry, err error) {
	if err := s.writable("supersede"); err != nil {
		return nil, nil, err
	}
	old, err = s.Get(ctx, oldID)
	if err != nil {
		return nil, nil, fmt.Errorf("direction: supersede %s: %w", oldID, err)
	}
	if !supersedable(old) {
		return nil, nil, fmt.Errorf("direction: supersede %s: state %q of kind %q cannot be superseded", oldID, old.State, old.Kind)
	}
	if next.ID != "" {
		return nil, nil, fmt.Errorf("direction: supersede %s: replacement entry must not carry an id yet; Supersede allocates it", oldID)
	}

	if _, err := ledger.AddEdge(next, ledger.Edge{Type: ledger.EdgeSupersedes, To: oldID}); err != nil {
		return nil, nil, fmt.Errorf("direction: supersede %s: %w", oldID, err)
	}
	if next.Created == "" {
		next.Created = now.UTC().Format(time.RFC3339)
	}
	if err := ledger.Add(ctx, s, next); err != nil {
		return nil, nil, fmt.Errorf("direction: supersede %s: writing replacement: %w", oldID, err)
	}

	flipped := *old
	flipped.State = ledger.StateSuperseded
	flipped.Updated = now.UTC().Format(time.RFC3339)
	if err := s.Put(ctx, &flipped); err != nil {
		return nil, nil, fmt.Errorf("direction: supersede %s: flipping old entry: %w", oldID, err)
	}
	return &flipped, next, nil
}

// supersedable reports whether e is in a state its kind allows to move to
// superseded, and isn't superseded already. Only kinds whose state
// vocabulary includes superseded (decision, constraint) can be
// superseded at all; of those, only the "currently in force" state
// (accepted for a decision, active for a constraint) is a legal
// predecessor -- a staged or rejected decision has nothing standing to
// replace.
func supersedable(e *ledger.Entry) bool {
	canSupersede := false
	for _, st := range e.Kind.States() {
		if st == ledger.StateSuperseded {
			canSupersede = true
			break
		}
	}
	if !canSupersede {
		return false
	}
	switch e.State {
	case ledger.StateAccepted, ledger.StateActive:
		return true
	default:
		return false
	}
}
