package index

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

const GBrainClaimChunkKind = "gbrain_claim"

func importedClaim(c domain.Claim) bool {
	return c.ID != "" && strings.HasPrefix(c.SourceRef, "gbrain:") && c.Provenance.Meta["gbrain_page"] != "" && (c.Provenance.Meta["gbrain_fence"] == "facts" || c.Provenance.Meta["gbrain_fence"] == "takes")
}
func importedClaimRef(slug string, c domain.Claim) string { return "gbrain-claim:" + slug + ":" + c.ID }
func importedClaimText(slug string, c domain.Claim) string {
	review := ""
	if c.Review {
		review = "; review required"
	}
	return fmt.Sprintf("Imported claim for %s (%s%s): %s", slug, c.Predicate, review, c.Object)
}

type importedClaimRecord struct {
	claim domain.Claim
	slug  string
}

// importedClaims loads canonical fence rows, not index metadata. A checkpoint,
// SQLite row, or raw source claiming this reserved kind cannot confer authority.
// Resolve replacements before privacy so a private successor cannot resurrect
// an old public claim. Shard-derived rows are not gbrain import targets.
func importedClaims(root string, now time.Time, cfg *config.Config) (map[string]importedClaimRecord, error) {
	paths, err := filepath.Glob(filepath.Join(root, "brain", "entities", "*", "*.md"))
	if err != nil {
		return nil, err
	}
	result := map[string]importedClaimRecord{}
	fw := store.NewFenceWriter(root)
	for _, path := range paths {
		page, err := fw.ParseEntity(path)
		if err != nil {
			return nil, fmt.Errorf("imported claim policy: %w", err)
		}
		replaced := map[string]bool{}
		for _, c := range page.Claims {
			if c.Supersedes != "" {
				replaced[c.Supersedes] = true
			}
		}
		for _, c := range page.Claims {
			if cfg.TierOf(c.Family) != domain.TierFence || !importedClaim(c) || c.State != domain.StateActive || c.SupersededBy != "" || replaced[c.ID] || !c.CurrentAt(now) {
				continue
			}
			ref := importedClaimRef(page.Entity.Slug, c)
			if _, exists := result[ref]; exists {
				return nil, fmt.Errorf("imported claim policy: duplicate identity %s", ref)
			}
			result[ref] = importedClaimRecord{claim: c, slug: page.Entity.Slug}
		}
	}
	return result, nil
}

// RetrievalEligibility combines raw-source and canonical imported-claim policy.
// Canonical bytes are read once per request; stale edited/deleted/retracted rows
// cannot leak through an old FTS/vector index. Local search may read private
// imported claims; remote recall and every embedding/composition egress may not.
func RetrievalEligibility(root string, proj *store.MemoryProjection, remote, egress bool, now time.Time) (func(Hit) bool, error) {
	restricted, err := RestrictedSummaryEntities(root, proj, now)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if errors.Is(err, os.ErrNotExist) {
		cfg = config.Default()
	} else if err != nil {
		return nil, err
	}
	claims, err := importedClaims(root, now, cfg)
	if err != nil {
		return nil, err
	}
	sourceEligible := SourceEligibility(proj, remote, egress, now, restricted)
	return func(hit Hit) bool {
		if hit.Kind != GBrainClaimChunkKind {
			return sourceEligible(hit)
		}
		rec, ok := claims[hit.ChunkRef]
		if !ok || hit.EntitySlug != rec.slug || hit.Text != importedClaimText(rec.slug, rec.claim) || hit.SourceSHA256 != rec.claim.Provenance.SourceSHA256 {
			return false
		}
		if (remote || egress) && rec.claim.Visibility != domain.VisibilityShared {
			return false
		}
		return sourceEligible(Hit{SourceSHA256: rec.claim.Provenance.SourceSHA256})
	}, nil
}
