package entities

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// ErrClaimNotFound is returned by Split when one or more moveIDs is not a
// claim on original's page.
var ErrClaimNotFound = errors.New("entities: claim not found on original entity's page")

// SplitResult reports the two pages Split wrote.
type SplitResult struct {
	OriginalPath string
	NewPath      string
	Moved        int
}

// Split partitions original's fence-tier claims into two pages (RFC
// §10.5: "Splits supported from the client" -- unlike Merge, the RFC does
// not ask for an undo/audit-event record here; reversing a split is
// simply Merge-ing the two resulting pages back together, which does
// carry an undoable event). Every claim whose ID is in moveIDs is
// re-pointed to target.Slug and written to a brand-new page for target;
// every other claim stays on original's page, re-rendered with the moved
// claims removed. Claim IDs are preserved unchanged (see Merge's doc for
// why: a moved claim's ID keeps the hash it was derived under).
//
// original and target must share Type (ErrTypeMismatch) and be distinct
// entities. Every id in moveIDs must name a claim actually on original's
// page (ErrClaimNotFound, listing every missing id) -- checked before
// either page is written, so a bad id set never leaves a partial split on
// disk. moveIDs must select at least one claim (a "split" that moves
// nothing is a caller error, not a no-op).
//
// original's shard-tier claims (if any) are untouched by a fence-tier
// split: they stay filed under original.Slug regardless of how its
// fence-tier claims are partitioned, since original keeps its own slug --
// unlike Merge, there is no shard-tier data-loss risk here, so Split
// carries no ErrShardTierUnsupported check.
func Split(q *writer.Queue, fw *store.FenceWriter, original, target domain.Entity, moveIDs []string, now time.Time) (SplitResult, error) {
	if original.Type != target.Type {
		return SplitResult{}, fmt.Errorf("entities: split: %w: original.Type=%q target.Type=%q", ErrTypeMismatch, original.Type, target.Type)
	}
	if normalizeName(original.Slug) == normalizeName(target.Slug) {
		return SplitResult{}, fmt.Errorf("entities: split: original and target must be distinct entities (both %q)", original.Slug)
	}
	if len(moveIDs) == 0 {
		return SplitResult{}, fmt.Errorf("entities: split: moveIDs must select at least one claim")
	}

	origPath := fw.PathFor(original.Type, original.Slug)
	origPage, err := fw.ParseEntity(origPath)
	if err != nil {
		return SplitResult{}, fmt.Errorf("entities: split: read %s: %w", origPath, err)
	}

	want := make(map[string]bool, len(moveIDs))
	for _, id := range moveIDs {
		want[id] = true
	}

	var kept, moved []domain.Claim
	found := make(map[string]bool, len(moveIDs))
	for _, c := range origPage.Claims {
		if want[c.ID] {
			c.SubjectSlug = target.Slug
			moved = append(moved, c)
			found[c.ID] = true
		} else {
			kept = append(kept, c)
		}
	}
	if len(found) != len(want) {
		var missing []string
		for id := range want {
			if !found[id] {
				missing = append(missing, id)
			}
		}
		sort.Strings(missing)
		return SplitResult{}, fmt.Errorf("entities: split: %w: %s", ErrClaimNotFound, strings.Join(missing, ", "))
	}

	origPage.Claims = kept
	newPage := store.NewEntityPage(target)
	newPage.Claims = moved

	origWritten, _, err := writer.Fence(q, fw, origPage)
	if err != nil {
		return SplitResult{}, fmt.Errorf("entities: split: write %s: %w", origPath, err)
	}
	newWritten, _, err := writer.Fence(q, fw, newPage)
	if err != nil {
		return SplitResult{}, fmt.Errorf("entities: split: write %s: %w", fw.PathFor(target.Type, target.Slug), err)
	}

	return SplitResult{OriginalPath: origWritten, NewPath: newWritten, Moved: len(moved)}, nil
}
