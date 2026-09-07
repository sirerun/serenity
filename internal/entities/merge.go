package entities

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// ErrShardTierUnsupported is returned by Merge when the entity being
// absorbed (b) has any shard-tier claim family on disk. Scope, disclosed:
// RFC §10.5 says nothing about shard tiers, and §7.2a ties shard tier to a
// different axis entirely ("high-volume machine observations" -- balances,
// transactions -- not entity identity). Merge fully re-points fence-tier
// claims, the human-scale families §7.2a itself associates with identity
// (roles, preferences, commitments, conditions). Silently repointing
// shard-tier claims would mean either duplicating them under two slugs or
// physically relocating an append-only file that other code (ResolveHeads,
// Compact) addresses by (slug, family) path -- both are real engineering
// beyond this task's acc line, so Merge refuses loudly rather than
// orphaning b's shard claims (still on disk under b's slug, but unreachable
// from any entity page once b's page is gone). Checked via ss.Families,
// the shard store's own authoritative listing (§7.2a: "the shard is
// canonical"), only against b -- a's own pre-existing shard claims are
// untouched by a merge (a keeps its slug) and need no check.
var ErrShardTierUnsupported = errors.New("entities: merge: absorbed entity has shard-tier claim families; merge is scoped to fence-tier claims (see ErrShardTierUnsupported doc)")

// ErrEventNotFound is returned by Undo when eventID names no merge event
// on disk.
var ErrEventNotFound = errors.New("entities: merge event not found")

// ErrAlreadyUndone is returned by Undo when eventID's merge event has
// already been undone once.
var ErrAlreadyUndone = errors.New("entities: merge event already undone")

// MergeEvent is the undoable record RFC §10.5 calls for ("auto-merge with
// undoable merge event + audit trail"). The git commit Flush produces over
// APath/BPath's changes (T0.5: "serenity:"-prefixed, one per queue flush)
// is the audit trail proper -- RFC §7 preamble's own stated design ("audit
// history is git log"), automatically satisfied because Merge routes every
// content change through writer.Fence/q.Submit like any other canonical
// write. MergeEvent itself is the narrower, undo-focused half: enough raw
// bytes to restore both paths byte-identically without depending on
// RenderEntity re-producing the exact same output later (a real but
// separate guarantee the deterministic-writer round-trip already provides
// -- MergeEvent does not lean on it).
//
// Persisted at .serenity/entities/merges/<id>.json -- disclosed scope:
// this mirrors T0.4's PendingRecord and T2.19's cron run-records, both
// under the gitignored, machine-local .serenity/ tree (RFC §7.1: "derived
// ... gitignored"). That means Undo is durable across process restarts on
// the machine that ran Merge, but the undo bookkeeping itself does not
// sync via git the way the merge's actual content changes do. A human
// wanting to reverse a merge from a different clone always has the
// git-log audit trail available for a manual git revert of the merge
// commit; MergeEvent's Undo is the fast, in-process path on the same
// machine.
type MergeEvent struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	IntoSlug   string `json:"into_slug"`   // a -- the surviving entity
	MergedSlug string `json:"merged_slug"` // b -- the absorbed entity
	APath      string `json:"a_path"`
	BPath      string `json:"b_path"`
	// ABefore/BBefore are a/b's exact file bytes immediately before Merge
	// wrote anything -- json.Marshal encodes []byte as base64, so these
	// round-trip byte-for-byte through the event file.
	ABefore []byte `json:"a_before"`
	BBefore []byte `json:"b_before,omitempty"`
	// BExisted records whether b had a page at all before Merge -- Undo
	// must delete a freshly-created page it never should have restored,
	// not write zero bytes to it.
	BExisted bool `json:"b_existed"`
	// ClaimsMoved is the count of b's fence-tier claims re-pointed onto a
	// -- observability for the audit record, not used by Undo.
	ClaimsMoved int        `json:"claims_moved"`
	Actor       string     `json:"actor,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UndoneAt    *time.Time `json:"undone_at,omitempty"`
}

func eventsDir(root string) string {
	return filepath.Join(root, ".serenity", "entities", "merges")
}

func eventPath(root, id string) string {
	return filepath.Join(eventsDir(root), id+".json")
}

func writeEvent(root string, ev MergeEvent) error {
	b, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return fmt.Errorf("entities: marshal merge event %s: %w", ev.ID, err)
	}
	if err := os.MkdirAll(eventsDir(root), 0o755); err != nil {
		return fmt.Errorf("entities: merge event dir: %w", err)
	}
	if err := os.WriteFile(eventPath(root, ev.ID), b, 0o644); err != nil {
		return fmt.Errorf("entities: write merge event %s: %w", ev.ID, err)
	}
	return nil
}

func readEvent(root, id string) (MergeEvent, error) {
	b, err := os.ReadFile(eventPath(root, id))
	if err != nil {
		if os.IsNotExist(err) {
			return MergeEvent{}, fmt.Errorf("entities: %w: %s", ErrEventNotFound, id)
		}
		return MergeEvent{}, fmt.Errorf("entities: read merge event %s: %w", id, err)
	}
	var ev MergeEvent
	if err := json.Unmarshal(b, &ev); err != nil {
		return MergeEvent{}, fmt.Errorf("entities: decode merge event %s: %w", id, err)
	}
	return ev, nil
}

// unionAliases dedups and sorts existing plus extra, normalized, dropping
// any entry equal to exclude (an entity never lists itself as its own
// alias) and empty strings. Comparison is case/whitespace-normalized but
// the first-seen surface form is kept (so an alias entered "Alice Tan"
// is not silently lowercased on the page).
func unionAliases(existing []string, extra []string, exclude string) []string {
	excl := normalizeName(exclude)
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		n := normalizeName(s)
		if n == "" || n == excl || seen[n] {
			return
		}
		seen[n] = true
		out = append(out, s)
	}
	for _, s := range existing {
		add(s)
	}
	for _, s := range extra {
		add(s)
	}
	sort.Strings(out)
	return out
}

// Merge folds b's identity into a (RFC §10.5's auto-merge outcome): every
// fence-tier claim on b's page is re-pointed to a.Slug and appended to a's
// page, b's slug and every one of b's own aliases become aliases of a, a's
// page is re-rendered through the writer queue (writer.Fence, so the
// dirty-tree guard and per-file ordering apply exactly as they do to any
// other canonical write), and b's own page is removed. Both pages' exact
// pre-merge bytes are captured into a MergeEvent before anything is
// written, persisted for Undo, and returned.
//
// A claim's re-pointing IS its move to a's page: the fence claims table
// (§7.2) has no subject column -- SubjectSlug is implicit from which
// page's table a row lives on, and store.ParseEntityBytes always
// re-derives it from the page's own frontmatter slug on read. Merge still
// sets each moved claim's in-memory SubjectSlug field to a.Slug before
// appending it (self-consistency for any future reader of the
// EntityPage.Claims value itself, and forward-compatible if the table
// format ever grows an explicit subject column), but that assignment is
// provably inert against the current on-disk format -- confirmed by
// disabling it and observing RenderEntity's output was byte-for-byte
// unchanged. What actually determines a claim's post-merge subject is
// solely its presence in a's claims slice at write time.
//
// Claim identity is deliberately NOT recomputed on repoint: a moved
// claim's ID keeps the hash it was derived under (over b's original
// SubjectSlug, §7.2). This preserves every Supersedes/SupersededBy
// pointer within b's claims unchanged (they still resolve to sibling IDs
// now living on the same page) and keeps the claim's identity stable for
// anything that already referenced it by ID (a disposition item's
// AppliedClaimID, a shard Supersedes pointer). The tradeoff, disclosed: a
// moved claim's ID no longer matches what DerivedID(a.Slug, ...) would
// produce for the same content -- re-deriving it fresh from the same
// source material would mint a different ID. That is an accepted
// consequence of a merge event changing which entity a claim belongs to
// without changing the claim's own recorded history.
//
// a and b must share Type (ErrTypeMismatch) and be distinct entities. b
// must have no shard-tier claim families (ErrShardTierUnsupported) -- see
// that error's doc.
func Merge(q *writer.Queue, fw *store.FenceWriter, ss *store.ShardStore, a, b domain.Entity, now time.Time, actor string) (MergeEvent, error) {
	if a.Type != b.Type {
		return MergeEvent{}, fmt.Errorf("entities: merge: %w: a.Type=%q b.Type=%q", ErrTypeMismatch, a.Type, b.Type)
	}
	if normalizeName(a.Slug) == normalizeName(b.Slug) {
		return MergeEvent{}, fmt.Errorf("entities: merge: a and b must be distinct entities (both %q)", a.Slug)
	}

	bFamilies, err := ss.Families(b.Slug)
	if err != nil {
		return MergeEvent{}, fmt.Errorf("entities: merge: list shard families for %s: %w", b.Slug, err)
	}
	if len(bFamilies) > 0 {
		return MergeEvent{}, fmt.Errorf("entities: merge %s into %s: %w: families=%v", b.Slug, a.Slug, ErrShardTierUnsupported, bFamilies)
	}

	aPath := fw.PathFor(a.Type, a.Slug)
	bPath := fw.PathFor(b.Type, b.Slug)

	aBefore, err := os.ReadFile(aPath)
	if err != nil {
		return MergeEvent{}, fmt.Errorf("entities: merge: read %s: %w", aPath, err)
	}
	bBefore, err := os.ReadFile(bPath)
	bExisted := err == nil
	if err != nil && !os.IsNotExist(err) {
		return MergeEvent{}, fmt.Errorf("entities: merge: read %s: %w", bPath, err)
	}

	aPage, err := store.ParseEntityBytes(aBefore)
	if err != nil {
		return MergeEvent{}, fmt.Errorf("entities: merge: parse %s: %w", aPath, err)
	}

	var bAliases []string
	moved := 0
	if bExisted {
		bPage, err := store.ParseEntityBytes(bBefore)
		if err != nil {
			return MergeEvent{}, fmt.Errorf("entities: merge: parse %s: %w", bPath, err)
		}
		bAliases = bPage.Entity.Aliases
		for _, c := range bPage.Claims {
			c.SubjectSlug = a.Slug
			aPage.Claims = append(aPage.Claims, c)
			moved++
		}
		aPage.Timeline = append(aPage.Timeline, bPage.Timeline...)
	}

	extra := append([]string{b.Slug}, bAliases...)
	extra = append(extra, b.Aliases...)
	aPage.Entity.Aliases = unionAliases(aPage.Entity.Aliases, extra, a.Slug)

	if _, _, err := writer.Fence(q, fw, aPage); err != nil {
		return MergeEvent{}, fmt.Errorf("entities: merge: write %s: %w", aPath, err)
	}

	if bExisted {
		// Not a writer.Fence/Shard entry point (there is no canonical
		// "delete an entity page" primitive in internal/writer today) --
		// submitted as a raw writer.Job through the SAME queue instance so
		// it still serializes against any concurrent write to bPath, and
		// still lands in the touched-set writer.Flush commits alongside
		// a's own change. Disclosed gap: this bypasses T0.4's dirty-tree
		// guard (unexported, and shaped around comparing rendered bytes to
		// disk, which a deletion has no "rendered" form to diff against) --
		// a human edit to b's page mid-merge, not yet committed, is
		// silently lost rather than paused like every other canonical
		// write in this codebase. A follow-up extending internal/writer
		// with a guarded delete primitive would close this.
		res := q.Submit(writer.Job{Path: bPath, Render: func() ([]byte, error) {
			if err := os.Remove(bPath); err != nil {
				return nil, err
			}
			return nil, nil
		}})
		if res.Err != nil {
			return MergeEvent{}, fmt.Errorf("entities: merge: remove %s: %w", bPath, res.Err)
		}
	}

	ev := MergeEvent{
		ID: newID(), Type: a.Type, IntoSlug: a.Slug, MergedSlug: b.Slug,
		APath: aPath, BPath: bPath, ABefore: aBefore, BBefore: bBefore, BExisted: bExisted,
		ClaimsMoved: moved, Actor: actor, CreatedAt: now.UTC(),
	}
	if err := writeEvent(fw.Root, ev); err != nil {
		return MergeEvent{}, err
	}
	return ev, nil
}

// Undo reverses one merge event, restoring both a's and b's pages to their
// exact pre-merge bytes (a's page written back verbatim; b's page
// recreated verbatim if it existed, removed again if Merge itself created
// nothing there). Both restores go through q so they serialize against any
// concurrent write and land in the same writer.Flush commit cycle as any
// other canonical write. An event that has already been undone once
// refuses (ErrAlreadyUndone) rather than restoring a second time.
func Undo(q *writer.Queue, root, eventID string, now time.Time) (MergeEvent, error) {
	ev, err := readEvent(root, eventID)
	if err != nil {
		return MergeEvent{}, err
	}
	if ev.UndoneAt != nil {
		return MergeEvent{}, fmt.Errorf("entities: undo %s: %w", eventID, ErrAlreadyUndone)
	}

	aBefore := ev.ABefore
	res := q.Submit(writer.Job{Path: ev.APath, Render: func() ([]byte, error) {
		if err := os.WriteFile(ev.APath, aBefore, 0o644); err != nil {
			return nil, err
		}
		return aBefore, nil
	}})
	if res.Err != nil {
		return MergeEvent{}, fmt.Errorf("entities: undo %s: restore %s: %w", eventID, ev.APath, res.Err)
	}

	bBefore := ev.BBefore
	if ev.BExisted {
		res = q.Submit(writer.Job{Path: ev.BPath, Render: func() ([]byte, error) {
			if err := os.MkdirAll(filepath.Dir(ev.BPath), 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(ev.BPath, bBefore, 0o644); err != nil {
				return nil, err
			}
			return bBefore, nil
		}})
	} else {
		res = q.Submit(writer.Job{Path: ev.BPath, Render: func() ([]byte, error) {
			if err := os.Remove(ev.BPath); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			return nil, nil
		}})
	}
	if res.Err != nil {
		return MergeEvent{}, fmt.Errorf("entities: undo %s: restore %s: %w", eventID, ev.BPath, res.Err)
	}

	doneAt := now.UTC()
	ev.UndoneAt = &doneAt
	if err := writeEvent(root, ev); err != nil {
		return MergeEvent{}, err
	}
	return ev, nil
}
