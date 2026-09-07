package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/ingest"
	"github.com/sirerun/serenity/internal/reconcile"
	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// machineConfidenceCap and defaultMachineConfidence mirror RFC 0001
// §9's own confidence bounds ("machine-assigned confidence is capped at
// 0.90 (local-cheap) / 0.95 (judgment). Only human confirmation can set
// confidence above 0.95"): remember has no tier of its own (it is a
// direct write, not a router.Complete call), so this task reads "machine"
// broadly -- any Provenance.Actor that is not literally "human:<id>" is
// capped at the lower, local-cheap bound, the more conservative of the
// two named ceilings, disclosed rather than silently picking 0.95.
const machineConfidenceCap = 0.90
const defaultMachineConfidence = 0.90

type rememberProvenance struct {
	// Actor is required -- "remember with empty provenance ->
	// provenance_required" (this task's own acc line) is read as: the
	// provenance object itself, or its Actor field, being absent/empty is
	// what triggers the error, since Actor is the one field every other
	// domain.Provenance-producing path in this repo always sets
	// (ClaimFromObservation: "machine"; supersede.Retract's actor
	// parameter; T2.7's edit_accept: the disposing human).
	Actor        string `json:"actor"`
	SourceSHA256 string `json:"source_sha256,omitempty"`
	Span         string `json:"span,omitempty"`
	Model        string `json:"model,omitempty"`
}

type rememberRequest struct {
	Subject    string              `json:"subject"`
	Predicate  string              `json:"predicate"`
	Object     string              `json:"object"`
	Confidence *float64            `json:"confidence,omitempty"`
	ValidFrom  string              `json:"valid_from,omitempty"`
	Provenance *rememberProvenance `json:"provenance,omitempty"`
}

type rememberResponse struct {
	Envelope
	// Status is "remembered" (written to the brain repo) or
	// "staged_for_review" (internal/reconcile, T2.2, found a genuine
	// conflict or a temporal-supersession shape against an active
	// neighbor and staged a KindReconcile disposition item instead of
	// writing -- RFC §10.3: "No automatic supersession at trust level
	// 0"). staged_for_review is a successful response, not an
	// Envelope.Error: the system did exactly what it should when a new
	// claim disagrees with what it already believes.
	Status            string `json:"status"`
	DispositionItemID string `json:"disposition_item_id,omitempty"`
}

func (h *Handlers) rememberTool() mcp.Tool {
	schema := `{
		"type": "object",
		"properties": {
			"subject": {"type": "string"},
			"predicate": {"type": "string"},
			"object": {"type": "string"},
			"confidence": {"type": "number", "minimum": 0, "maximum": 1},
			"valid_from": {"type": "string"},
			"provenance": {
				"type": "object",
				"properties": {
					"actor": {"type": "string"},
					"source_sha256": {"type": "string"},
					"span": {"type": "string"},
					"model": {"type": "string"}
				},
				"required": ["actor"]
			}
		},
		"required": ["subject", "predicate", "object", "provenance"]
	}`
	return mcp.Tool{
		Name:        "remember",
		Description: "Record a new fact (subject, predicate, object) with provenance, reconciled against what the brain already believes.",
		InputSchema: json.RawMessage(schema),
		Handler:     handle(h.remember),
	}
}

// remember writes through the writer queue and reconcile, never
// id-equality dedup (this task's own pitfall note): unlike
// internal/ingest.Writer's trust-0 posture ("no merging, no conflict
// detection... that is the reconcile engine's job, deferred to E2"),
// remember runs the new claim through internal/reconcile (T2.2) before
// ever calling writer.Shard/Fence, because MEMORY_VERBS's remember is an
// interactive write path with no upstream extraction/reconcile stage of
// its own the way ingest has -- this verb is where that stage has to
// live.
//
// Only VerdictConflict/VerdictWindowClose block the write (RFC §10.3's
// "no automatic supersession at trust level 0" -- reconcile.Engine.Process
// already stages exactly one KindReconcile disposition item for either).
// VerdictAgree/VerdictNeutralAdditive/VerdictScoped all write immediately,
// reusing the identical tier-routing (config.TierOf) and writer entry
// points (writer.Shard/Fence) internal/ingest.Writer.writeShardClaim/
// writeFenceClaim already established -- duplicated here in miniature
// rather than imported, since ingest.Writer's own methods are
// unexported and its Write signature takes []domain.Observation, not a
// pre-built domain.Claim.
func (h *Handlers) remember(ctx context.Context, args json.RawMessage) (any, bool, error) {
	var req rememberRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return rememberResponse{Envelope: errorEnvelope("invalid_argument", "remember: malformed request", "send a JSON object with subject, predicate, object, and provenance.actor")}, true, nil
	}
	if req.Subject == "" || req.Predicate == "" || req.Object == "" {
		return rememberResponse{Envelope: errorEnvelope("invalid_argument", "remember: subject, predicate, and object are all required", "pass non-empty subject, predicate, and object strings")}, true, nil
	}
	if req.Provenance == nil || req.Provenance.Actor == "" {
		return rememberResponse{Envelope: errorEnvelope("provenance_required", "remember: provenance is required", "pass provenance.actor (e.g. \"human:you\" or \"machine\"), and provenance.source_sha256/span/model when the fact came from a known source")}, true, nil
	}

	now := h.deps.now()
	confidence := defaultMachineConfidence
	if req.Confidence != nil {
		confidence = *req.Confidence
	}
	human := isHumanActor(req.Provenance.Actor)
	if !human && confidence > machineConfidenceCap {
		confidence = machineConfidenceCap
	}
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	objectKey := store.NormalizeKey(req.Object)
	claim := domain.Claim{
		SubjectSlug: req.Subject,
		Predicate:   req.Predicate,
		Object:      req.Object,
		ObjectKey:   objectKey,
		Confidence:  confidence,
		ValidFrom:   req.ValidFrom,
		State:       domain.StateActive,
		Family:      req.Predicate, // families are 1:1 with predicates in the seed vocabulary (store/fence.go), same as ClaimFromObservation
		Provenance: domain.Provenance{
			SourceSHA256: req.Provenance.SourceSHA256,
			Span:         req.Provenance.Span,
			Model:        req.Provenance.Model,
			ObservedAt:   now,
			Actor:        req.Provenance.Actor,
		},
	}
	claim.ID = store.DerivedID(claim.SubjectSlug, claim.Predicate, claim.ObjectKey, claim.ValidFrom, claim.Provenance.SourceSHA256, store.DefaultIDWidth)

	tier := h.deps.Config.TierOf(claim.Family)
	active, err := h.activeNeighbors(claim, tier)
	if err != nil {
		return nil, false, fmt.Errorf("remember: read active claims: %w", err)
	}

	if h.engine == nil {
		return nil, false, fmt.Errorf("remember: no disposition store configured -- cannot reconcile safely")
	}
	detection, item, err := h.engine.Process(ctx, claim, active, now)
	if err != nil {
		return nil, false, fmt.Errorf("remember: reconcile: %w", err)
	}

	if detection.Verdict == reconcile.VerdictConflict || detection.Verdict == reconcile.VerdictWindowClose {
		env := newEnvelope()
		env.Evidence = []Fact{factOfClaim(claim), factOfClaim(detection.Candidate)}
		resp := rememberResponse{Envelope: env, Status: "staged_for_review"}
		if item != nil {
			resp.DispositionItemID = item.ID
		}
		return resp, false, nil
	}

	if err := h.writeClaim(tier, claim); err != nil {
		return nil, false, fmt.Errorf("remember: write claim: %w", err)
	}

	env := newEnvelope()
	env.Evidence = []Fact{factOfClaim(claim)}
	return rememberResponse{Envelope: env, Status: "remembered"}, false, nil
}

func isHumanActor(actor string) bool {
	return len(actor) >= 6 && actor[:6] == "human:"
}

// activeNeighbors reads the active claims already recorded for claim's
// own (subject, family) -- reconcile.Candidates narrows this down to
// (subject, predicate) itself, so passing every active claim in claim's
// own shard file or fence-page family (family is 1:1 with predicate) is
// already correctly scoped, without internal/compose.AllClaims's
// whole-repo walk.
func (h *Handlers) activeNeighbors(claim domain.Claim, tier domain.Tier) ([]domain.Claim, error) {
	if tier == domain.TierShard {
		lines, err := h.deps.Shard.Lines(claim.SubjectSlug, claim.Family)
		if err != nil {
			return nil, err
		}
		return filterActive(lines), nil
	}
	path := h.deps.Fence.PathFor(ingest.DefaultEntityType, claim.SubjectSlug)
	page, err := h.deps.Fence.ParseEntity(path)
	if err != nil {
		return nil, nil // no existing page yet -- nothing to reconcile against
	}
	var out []domain.Claim
	for _, c := range page.Claims {
		if c.Predicate == claim.Predicate {
			out = append(out, c)
		}
	}
	return filterActive(out), nil
}

func filterActive(claims []domain.Claim) []domain.Claim {
	out := make([]domain.Claim, 0, len(claims))
	for _, c := range claims {
		if c.State == domain.StateActive {
			out = append(out, c)
		}
	}
	return out
}

// writeClaim commits claim through the deterministic writer -- the same
// two entry points (writer.Shard/writer.Fence) and tier split
// internal/ingest.Writer.writeClaim uses, minus that type's per-Write-call
// id-dedup cache (remember commits one claim per call, so there is
// nothing to cache across).
func (h *Handlers) writeClaim(tier domain.Tier, claim domain.Claim) error {
	if tier == domain.TierShard {
		_, _, err := writer.Shard(h.deps.Queue, h.deps.Shard, claim)
		return err
	}
	path := h.deps.Fence.PathFor(ingest.DefaultEntityType, claim.SubjectSlug)
	page, err := h.deps.Fence.ParseEntity(path)
	if err != nil {
		page = store.NewEntityPage(domain.Entity{Type: ingest.DefaultEntityType, Slug: claim.SubjectSlug})
	}
	page.Claims = append(page.Claims, claim)
	_, _, err = writer.Fence(h.deps.Queue, h.deps.Fence, page)
	return err
}
