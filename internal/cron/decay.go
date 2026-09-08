package cron

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/ingest"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/reconcile"
	"github.com/sirerun/serenity/internal/store"
)

// DecayResult describes review work staged from canonical active claims. Decay
// affects the review signal only; no stored confidence or lifecycle is changed.
type DecayResult struct {
	Claims         int `json:"claims"`
	DistillCreated int `json:"distill_created"`
	AliasCreated   int `json:"alias_created"`
	Existing       int `json:"existing"`
}

// Decay runs the weekly canonical review scan and records only successful runs.
func Decay(ctx context.Context, root string, clock Clock) error {
	_, err := RunDecay(ctx, root, clock)
	return err
}

func RunDecay(ctx context.Context, root string, clock Clock) (DecayResult, error) {
	var result DecayResult
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return result, err
	}
	fw, ss := store.NewFenceWriter(root), store.NewShardStore(root)
	iw := ingest.New(nil, fw, ss, cfg)
	paths, err := filepath.Glob(filepath.Join(root, "brain/entities/*/*.md"))
	if err != nil {
		return result, err
	}
	slugs, err := ss.Slugs()
	if err != nil {
		return result, err
	}
	seen := map[string]bool{}
	for _, slug := range slugs {
		seen[slug] = true
	}
	for _, path := range paths {
		seen[strings.TrimSuffix(filepath.Base(path), ".md")] = true
	}
	slugs = slugs[:0]
	for slug := range seen {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	now := clock.Now().UTC()
	var claims []domain.Claim
	// Validate every subject before staging anything. The existing canonical
	// reader rejects dirty snapshots and resolves shard heads, including rollover.
	for _, slug := range slugs {
		active, err := iw.CanonicalActiveClaims(ctx, slug, now)
		if err != nil {
			return result, fmt.Errorf("cron decay: %w", err)
		}
		claims = append(claims, active...)
	}
	result.Claims = len(claims)
	halfLives := map[string]int{}
	for family, c := range cfg.Families {
		halfLives[family] = c.HalfLifeDays
	}
	sweep := reconcile.Sweep(claims, halfLives, now, 0)
	eng, err := providers.OpenIndex(root)
	if err != nil {
		return result, err
	}
	defer func() { _ = eng.Close() }()
	ds := disposition.NewStore(eng)
	for _, candidate := range sweep.DistillCandidates {
		p := disposition.DecayPayload{Origin: disposition.DecayOrigin, Text: fmt.Sprintf("Review stale claim: %s %s=%q (stored confidence %.2f; aged confidence %.2f). Review does not change the canonical claim.", candidate.Claim.SubjectSlug, candidate.Claim.Predicate, candidate.Claim.Object, candidate.Claim.Confidence, candidate.DecayedConfidence), Claim: candidate.Claim, DecayedConfidence: candidate.DecayedConfidence, HalfLifeDays: halfLives[candidate.Claim.Family], EvaluatedAt: now}
		// Exclude evaluation time/decayed score from identity: aging the same evidence
		// must not reset an earlier human decision. Changed evidence gets a new card.
		identity, err := json.Marshal(struct {
			Claim        domain.Claim
			HalfLifeDays int
		}{candidate.Claim, p.HalfLifeDays})
		if err != nil {
			return result, err
		}
		inserted, err := stageScheduled(ctx, ds, disposition.KindDistill, p, "decay:", identity, now)
		if err != nil {
			return result, err
		}
		if inserted {
			result.DistillCreated++
		} else {
			result.Existing++
		}
	}
	for _, candidate := range sweep.AliasCandidates {
		p := disposition.LexicalAliasPayload{Origin: disposition.LexicalAliasOrigin, A: candidate.A, B: candidate.B, Reason: "similar subject spelling; human review only, no automatic merge"}
		identity, err := json.Marshal(p)
		if err != nil {
			return result, err
		}
		inserted, err := stageScheduled(ctx, ds, disposition.KindEntityMerge, p, "alias:", identity, now)
		if err != nil {
			return result, err
		}
		if inserted {
			result.AliasCreated++
		} else {
			result.Existing++
		}
	}
	return result, recordDetails(root, "decay", now, result)
}

func stageScheduled(ctx context.Context, ds *disposition.Store, kind disposition.Kind, payload any, prefix string, identity []byte, now time.Time) (bool, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	sum := sha256.Sum256(identity)
	_, created, err := ds.CreateOnce(ctx, kind, raw, "", prefix+hex.EncodeToString(sum[:]), now)
	return created, err
}
