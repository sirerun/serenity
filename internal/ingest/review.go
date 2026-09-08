package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/extract"
	"github.com/sirerun/serenity/internal/reconcile"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// ReviewPlan separates safe additions from proposals requiring human review.
// Proposals are staged only after Ready is published and committed, because a
// proposal can refer to an earlier addition in the same extraction batch.
type ReviewPlan struct {
	Ready           []domain.Observation
	Proposals       []reconcile.ReconcilePayload
	AlreadyPresent  int
	PriorDecision   int
	AwaitingDistill int
}

// ReviewObservations uses canonical rows, not potentially stale index claims.
// Terminal human decisions continue to govern the same source/value observation
// on later extraction attempts, including a human-edited accepted value.
func (w *Writer) ReviewObservations(ctx context.Context, ds *disposition.Store, observations []domain.Observation, now time.Time) (ReviewPlan, error) {
	result := ReviewPlan{}
	_, snapshot, err := w.snapshotObservations(observations)
	if err != nil {
		return result, err
	}
	all, active, err := w.canonicalReviewClaims(snapshot, now)
	if err != nil {
		return result, err
	}
	items, err := ds.List(ctx)
	if err != nil {
		return result, err
	}
	decided := map[string]bool{}
	awaiting := map[string]bool{}
	for _, item := range items {
		observation, isExtraction, err := disposition.ExtractionObservation(item)
		if err != nil {
			return result, err
		}
		if isExtraction {
			key, err := observationIdentity(ClaimFromObservation(observation))
			if err != nil {
				return result, err
			}
			if item.State == disposition.StateDisposed {
				decided[key] = true
			} else {
				awaiting[key] = true
			}
			continue
		}
		if item.Kind != disposition.KindReconcile || item.State != disposition.StateDisposed {
			continue
		}
		var payload reconcile.ReconcilePayload
		if err := json.Unmarshal(item.Payload, &payload); err != nil {
			return result, fmt.Errorf("extract review: corrupt decision %s: %w", item.ID, err)
		}
		key, err := observationIdentity(payload.A)
		if err != nil {
			return result, err
		}
		decided[key] = true
	}
	for _, observation := range observations {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if observation.Confidence < extract.DistillThreshold {
			return result, fmt.Errorf("extract review: low-confidence observation requires distill review")
		}
		claim := ClaimFromObservation(observation)
		identity := claim.SubjectSlug + "\x00" + claim.ID
		if all[identity] {
			result.AlreadyPresent++
			continue
		}
		key, err := observationIdentity(claim)
		if err != nil {
			return result, err
		}
		if decided[key] {
			result.PriorDecision++
			continue
		}
		if awaiting[key] {
			result.AwaitingDistill++
			continue
		}
		candidates := reconcile.Candidates(claim, active[claim.SubjectSlug])
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
		detection := reconcile.Detect(claim, candidates)
		if detection.Verdict == reconcile.VerdictConflict || detection.Verdict == reconcile.VerdictWindowClose {
			result.Proposals = append(result.Proposals, reconcile.ReconcilePayload{Verdict: detection.Verdict, Reason: detection.Reason, A: claim, B: detection.Candidate})
			continue
		}
		result.Ready = append(result.Ready, observation)
		all[identity] = true
		active[claim.SubjectSlug] = append(active[claim.SubjectSlug], claim)
	}
	return result, nil
}

// StageReview records proposals after canonical additions are committed. It
// verifies that each prior claim still exists unchanged, and atomic keyed
// insertion preserves an existing pending, deferred or terminal review.
func (w *Writer) StageReview(ctx context.Context, ds *disposition.Store, proposals []reconcile.ReconcilePayload, now time.Time) (created, existing int, err error) {
	observations := make([]domain.Observation, 0, len(proposals))
	for _, proposal := range proposals {
		observations = append(observations, domain.Observation{SubjectSlug: proposal.B.SubjectSlug, Predicate: proposal.B.Predicate})
	}
	_, snapshot, err := w.snapshotObservations(observations)
	if err != nil {
		return 0, 0, err
	}
	if err := writer.CheckSnapshot(w.Fence.Root, snapshot); err != nil {
		return 0, 0, err
	}
	_, active, err := w.canonicalReviewClaims(snapshot, now)
	if err != nil {
		return 0, 0, err
	}
	// Preflight every prior claim before inserting any queue item.
	for _, proposal := range proposals {
		found := false
		for _, claim := range active[proposal.B.SubjectSlug] {
			if claim.ID != proposal.B.ID {
				continue
			}
			left, e := canonicalClaimBytes(claim)
			if e != nil {
				return 0, 0, e
			}
			right, e := canonicalClaimBytes(proposal.B)
			if e != nil {
				return 0, 0, e
			}
			found = bytes.Equal(left, right)
			break
		}
		if !found {
			return 0, 0, fmt.Errorf("extract review: prior claim changed before staging; rerun extraction against current canonical files")
		}
	}
	for _, proposal := range proposals {
		aKey, e := observationIdentity(proposal.A)
		if e != nil {
			return created, existing, e
		}
		bBytes, e := canonicalClaimBytes(proposal.B)
		if e != nil {
			return created, existing, e
		}
		sum := sha256.Sum256(append(append([]byte(aKey), 0), bBytes...))
		raw, e := json.Marshal(proposal)
		if e != nil {
			return created, existing, e
		}
		_, inserted, e := ds.CreateOnce(ctx, disposition.KindReconcile, raw, "", hex.EncodeToString(sum[:]), now)
		if e != nil {
			return created, existing, e
		}
		if inserted {
			created++
		} else {
			existing++
		}
	}
	return created, existing, nil
}

func observationIdentity(claim domain.Claim) (string, error) {
	// Confidence/model/timestamps can vary between attempts without making the
	// same source's same assertion new evidence or erasing a human rejection.
	raw, err := json.Marshal([]string{claim.SubjectSlug, claim.Predicate, store.NormalizeKey(claim.Object), claim.ValidFrom, claim.Provenance.SourceSHA256})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func canonicalClaimBytes(claim domain.Claim) ([]byte, error) {
	claim.ObjectKey = store.NormalizeKey(claim.Object)
	return json.Marshal(claim)
}

func (w *Writer) canonicalReviewClaims(snapshot map[string][]byte, now time.Time) (map[string]bool, map[string][]domain.Claim, error) {
	all := map[string]bool{}
	active := map[string][]domain.Claim{}
	shards := map[string][]domain.Claim{}
	paths := make([]string, 0, len(snapshot))
	for path := range snapshot {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		raw := snapshot[path]
		if strings.HasPrefix(path, "brain/entities/") {
			page, err := store.ParseEntityBytes(raw)
			if err != nil {
				return nil, nil, err
			}
			if page.Entity.Slug != strings.TrimSuffix(filepath.Base(path), ".md") {
				return nil, nil, fmt.Errorf("extract review: entity identity does not match its path")
			}
			for _, claim := range page.Claims {
				claim.ObjectKey = store.NormalizeKey(claim.Object)
				if w.Config.TierOf(claim.Family) == domain.TierShard {
					continue
				}
				identity := claim.SubjectSlug + "\x00" + claim.ID
				if all[identity] {
					return nil, nil, fmt.Errorf("extract review: duplicate canonical claim identity")
				}
				all[identity] = true
				if claim.State == domain.StateActive && claim.SupersededBy == "" && claim.CurrentAt(now) {
					active[claim.SubjectSlug] = append(active[claim.SubjectSlug], claim)
				}
			}
			continue
		}
		if !strings.HasPrefix(path, "brain/claims/") || !strings.HasSuffix(path, ".jsonl") {
			continue
		}
		rows, err := store.ParseShardBytes(raw)
		if err != nil {
			return nil, nil, err
		}
		slug := filepath.Base(filepath.Dir(path))
		for _, claim := range rows {
			claim.ObjectKey = store.NormalizeKey(claim.Object)
			if claim.SubjectSlug != slug || !safePart(claim.Family) {
				return nil, nil, fmt.Errorf("extract review: shard claim identity does not match its path")
			}
			if w.Config.TierOf(claim.Family) != domain.TierShard {
				continue
			}
			name := filepath.Base(path)
			archived := name == claim.Family+".archive.jsonl"
			if name != claim.Family+".jsonl" && !archived {
				prefix := claim.Family + "."
				segment := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".jsonl")
				number, err := strconv.Atoi(segment)
				if !strings.HasPrefix(name, prefix) || err != nil || number < 0 || strings.ContainsAny(segment, "+-") {
					return nil, nil, fmt.Errorf("extract review: shard family or segment does not match its path")
				}
			}
			all[claim.SubjectSlug+"\x00"+claim.ID] = true
			if archived {
				continue
			}
			key := claim.SubjectSlug + "\x00" + claim.Family
			shards[key] = append(shards[key], claim)
		}
	}
	for _, rows := range shards {
		for _, claim := range store.ResolveHeadLines(rows) {
			if claim.CurrentAt(now) {
				active[claim.SubjectSlug] = append(active[claim.SubjectSlug], claim)
			}
		}
	}
	return all, active, nil
}
