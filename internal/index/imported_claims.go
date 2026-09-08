package index

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
const CanonicalClaimChunkKind = "canonical_claim"

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

type canonicalClaimRecord struct {
	claim domain.Claim
	slug  string
	kind  string
	text  string
}

func canonicalClaimProjection(slug string, c domain.Claim) (string, canonicalClaimRecord, error) {
	if importedClaim(c) {
		return importedClaimRef(slug, c), canonicalClaimRecord{claim: c, slug: slug, kind: GBrainClaimChunkKind, text: importedClaimText(slug, c)}, nil
	}
	// The complete canonical claim fingerprints the derived cache identity.
	// Changes in provenance, confidence or validity cannot reuse stale vectors.
	raw, err := json.Marshal(c)
	if err != nil {
		return "", canonicalClaimRecord{}, err
	}
	digest := sha256.Sum256(raw)
	ref := "canonical-claim:" + slug + ":" + c.ID + ":" + hex.EncodeToString(digest[:])
	qualifier := ""
	if c.Review {
		qualifier = "; review required"
	}
	text := fmt.Sprintf("%s [canonical claim: %s %s; confidence %.3g; actor %s%s]", c.Object, slug, c.Predicate, c.Confidence, c.Provenance.Actor, qualifier)
	return ref, canonicalClaimRecord{claim: c, slug: slug, kind: CanonicalClaimChunkKind, text: text}, nil
}

// canonicalClaims resolves native fence/shard facts and imported fence facts.
// Lifecycle and replacement are resolved before visibility, so a private
// successor never resurrects a public predecessor. Index rows confer no authority.
func canonicalClaims(root string, now time.Time, cfg *config.Config) (map[string]canonicalClaimRecord, error) {
	paths, err := filepath.Glob(filepath.Join(root, "brain", "entities", "*", "*.md"))
	if err != nil {
		return nil, err
	}
	result := map[string]canonicalClaimRecord{}
	identities := map[string]bool{}
	add := func(slug string, c domain.Claim) error {
		if c.ID == "" || c.SubjectSlug != slug {
			return fmt.Errorf("canonical claim policy: claim identity does not match entity")
		}
		identity := slug + "\x00" + c.ID
		if identities[identity] {
			return fmt.Errorf("canonical claim policy: duplicate identity")
		}
		identities[identity] = true
		if c.State != domain.StateActive || c.SupersededBy != "" || !c.CurrentAt(now) {
			return nil
		}
		ref, rec, err := canonicalClaimProjection(slug, c)
		if err != nil {
			return err
		}
		result[ref] = rec
		return nil
	}
	fw := store.NewFenceWriter(root)
	subjects := map[string]bool{}
	for _, path := range paths {
		page, err := fw.ParseEntity(path)
		if err != nil {
			return nil, fmt.Errorf("canonical claim policy: %w", err)
		}
		slug := page.Entity.Slug
		if slug != strings.TrimSuffix(filepath.Base(path), ".md") || subjects[slug] {
			return nil, fmt.Errorf("canonical claim policy: ambiguous or mismatched entity identity")
		}
		subjects[slug] = true
		replaced := map[string]bool{}
		for _, c := range page.Claims {
			if c.Supersedes != "" {
				replaced[c.Supersedes] = true
			}
		}
		for _, c := range page.Claims {
			if cfg.TierOf(c.Family) != domain.TierFence || replaced[c.ID] {
				continue
			}
			if err := add(slug, c); err != nil {
				return nil, err
			}
		}
	}
	shards := store.NewShardStore(root)
	slugs, err := shards.Slugs()
	if err != nil {
		return nil, err
	}
	for _, slug := range slugs {
		families, err := shards.Families(slug)
		if err != nil {
			return nil, err
		}
		for _, family := range families {
			if cfg.TierOf(family) != domain.TierShard {
				continue
			}
			rows, err := shards.Lines(slug, family)
			if err != nil {
				return nil, err
			}
			for _, c := range rows {
				if c.SubjectSlug != slug || c.Family != family {
					return nil, fmt.Errorf("canonical claim policy: shard identity does not match path")
				}
			}
			for _, c := range store.ResolveHeadLines(rows) {
				if err := add(slug, c); err != nil {
					return nil, err
				}
			}
		}
	}
	return result, nil
}

// RetrievalEligibility combines raw-source and canonical claim policy.
// Canonical bytes are read once per request; stale edited/deleted/retracted rows
// cannot leak through an old FTS/vector index. Local search may read private
// native/imported claims; remote recall and provider egress may not.
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
	claims, err := canonicalClaims(root, now, cfg)
	if err != nil {
		return nil, err
	}
	sourceEligible := SourceEligibility(proj, remote, egress, now, restricted)
	return func(hit Hit) bool {
		if hit.Kind != GBrainClaimChunkKind && hit.Kind != CanonicalClaimChunkKind {
			return sourceEligible(hit)
		}
		rec, ok := claims[hit.ChunkRef]
		if !ok || hit.Kind != rec.kind || hit.EntitySlug != rec.slug || hit.Text != rec.text || hit.SourceSHA256 != rec.claim.Provenance.SourceSHA256 {
			return false
		}
		return ClaimDisclosureEligible(proj, rec.claim, remote, egress, now)

	}, nil
}

// ClaimDisclosureEligible applies audience/source policy to an already-canonical
// claim. Callers must separately resolve lifecycle and current validity. This is
// also used for directly composed claims, which do not pass through chunk search.
func ClaimDisclosureEligible(proj *store.MemoryProjection, claim domain.Claim, remote, egress bool, now time.Time) bool {
	if remote || egress {
		visibility := claim.Visibility
		// Native legacy claims retain the existing single-principal shared
		// default. Imported visibility remains explicit and fail-closed.
		native := !importedClaim(claim)
		if native && visibility == "" {
			visibility = domain.VisibilityShared
		}
		if visibility != domain.VisibilityShared || proj.SourceIndexOnly(claim.Provenance.SourceSHA256) {
			return false
		}
		if native && claim.Provenance.SourceSHA256 != "" && !proj.SourceKnown(claim.Provenance.SourceSHA256) {
			return false
		}
	}
	return SourceEligibility(proj, remote, egress, now)(Hit{SourceSHA256: claim.Provenance.SourceSHA256})
}
