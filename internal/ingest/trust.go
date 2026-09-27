package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/reconcile"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// Provenance.Meta keys recorded on every machine claim from an untrusted
// connector (ADR 022, T24.16). Claims from trusted connectors carry neither,
// so their canonical bytes are unchanged.
const (
	MetaTrust     = "trust"
	MetaConnector = "connector"
)

// SourceClass is the connector trust class of one source, plus what a
// reviewer needs to judge a claim_candidate item.
type SourceClass struct {
	Trust     config.Trust
	Connector string
	Kind      string
	URI       string
}

// StoreTrust resolves a source's class from its stored kind and the
// connectors.<name>.trust setting in cfg. A source the store cannot read is
// untrusted (fail closed). Results are cached per sha for the resolver's life.
func StoreTrust(cfg *config.Config, ss *store.SourceStore) func(string) SourceClass {
	var mu sync.Mutex
	cache := map[string]SourceClass{}
	return func(sha string) SourceClass {
		mu.Lock()
		defer mu.Unlock()
		if class, ok := cache[sha]; ok {
			return class
		}
		class := SourceClass{Trust: config.TrustUntrusted}
		if sha != "" {
			if _, src, err := ss.Read(sha); err == nil {
				class = SourceClass{Trust: cfg.SourceTrust(src.Kind), Connector: config.ConnectorForSourceKind(src.Kind), Kind: src.Kind, URI: src.URI}
			}
		}
		cache[sha] = class
		return class
	}
}

func (w *Writer) classify(sha string) SourceClass {
	if w.Trust == nil {
		return SourceClass{Trust: config.TrustTrusted}
	}
	return w.Trust(sha)
}

// classifiedClaim is ClaimFromObservation plus the trust marker an untrusted
// source's claim carries into canonical state and the composer prompt.
func classifiedClaim(o domain.Observation, class SourceClass) domain.Claim {
	c := ClaimFromObservation(o)
	if class.Trust == config.TrustTrusted {
		return c
	}
	c.Provenance.Meta = map[string]string{MetaTrust: string(config.TrustUntrusted)}
	if class.Connector != "" {
		c.Provenance.Meta[MetaConnector] = class.Connector
	}
	return c
}

// WriteReviewed publishes a review plan in one batch: Ready observations as
// active claims (marked untrusted when their connector is) and Pending
// observations as claims in state pending with actor machine.
func (w *Writer) WriteReviewed(plan ReviewPlan) (Stats, error) {
	obs := make([]domain.Observation, 0, len(plan.Ready)+len(plan.Pending))
	claims := make([]domain.Claim, 0, cap(obs))
	for _, o := range plan.Ready {
		obs = append(obs, o)
		claims = append(claims, classifiedClaim(o, w.classify(o.SourceSHA256)))
	}
	for _, o := range plan.Pending {
		c := classifiedClaim(o, w.classify(o.SourceSHA256))
		c.State = domain.StatePending
		obs = append(obs, o)
		claims = append(claims, c)
	}
	return w.writeBatch(obs, claims)
}

// ClaimCandidatePayload is the payload of a disposition.KindClaimCandidate
// item: the pending canonical claim and where it came from.
type ClaimCandidatePayload struct {
	Version    int          `json:"version"`
	Claim      domain.Claim `json:"claim"`
	Trust      config.Trust `json:"trust"`
	Connector  string       `json:"connector,omitempty"`
	SourceKind string       `json:"source_kind,omitempty"`
	SourceURI  string       `json:"source_uri,omitempty"`
}

// ClaimCandidate decodes a claim_candidate item. ok is false for any other
// kind; a malformed candidate payload is an error.
func ClaimCandidate(item disposition.Item) (ClaimCandidatePayload, bool, error) {
	if item.Kind != disposition.KindClaimCandidate {
		return ClaimCandidatePayload{}, false, nil
	}
	var p ClaimCandidatePayload
	if err := json.Unmarshal(item.Payload, &p); err != nil {
		return p, true, fmt.Errorf("claim candidate %s: %w", item.ID, err)
	}
	if p.Version != 1 || p.Claim.State != domain.StatePending || p.Claim.Provenance.Actor != "machine" || !safePart(p.Claim.SubjectSlug) || !safePart(p.Claim.Family) || p.Claim.ID == "" {
		return p, true, fmt.Errorf("claim candidate %s: malformed payload", item.ID)
	}
	return p, true, nil
}

// StageCandidates queues one claim_candidate item per pending observation,
// after WriteReviewed's claims are committed. Keyed insertion keeps an
// existing open, deferred or decided item.
func (w *Writer) StageCandidates(ctx context.Context, ds *disposition.Store, pending []domain.Observation, now time.Time) (created, existing int, err error) {
	if len(pending) == 0 {
		return 0, 0, nil
	}
	_, snapshot, err := w.snapshotObservations(pending)
	if err != nil {
		return 0, 0, err
	}
	if err := writer.CheckSnapshot(w.Fence.Root, snapshot); err != nil {
		return 0, 0, err
	}
	all, _, err := w.canonicalReviewClaims(snapshot, now)
	if err != nil {
		return 0, 0, err
	}
	for _, o := range pending {
		class := w.classify(o.SourceSHA256)
		c := classifiedClaim(o, class)
		c.State = domain.StatePending
		if !all[c.SubjectSlug+"\x00"+c.ID] {
			return created, existing, fmt.Errorf("claim candidate: pending claim is not committed; rerun extraction")
		}
		key, e := observationIdentity(c)
		if e != nil {
			return created, existing, e
		}
		raw, e := json.Marshal(ClaimCandidatePayload{Version: 1, Claim: c, Trust: class.Trust, Connector: class.Connector, SourceKind: class.Kind, SourceURI: class.URI})
		if e != nil {
			return created, existing, e
		}
		_, inserted, e := ds.CreateOnce(ctx, disposition.KindClaimCandidate, raw, "", "claim-candidate:"+key, now)
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

// ActivatedClaim is the claim an accepted candidate publishes: the same
// identity and machine evidence, state active, the accepting human as actor.
func ActivatedClaim(pending domain.Claim, actor string) domain.Claim {
	c := pending
	c.State = domain.StateActive
	c.Provenance.Actor = actor
	c.Provenance.Meta = maps.Clone(pending.Provenance.Meta)
	return c
}

// PlanActivateCandidate prepares the canonical change that activates a
// pending claim for actor, a human. It changes no file. It refuses when the
// canonical row is gone, edited, already active, or when activating would
// conflict with a live claim -- conflicts stay on the reconciliation path.
func (w *Writer) PlanActivateCandidate(ctx context.Context, pending domain.Claim, actor string, now time.Time) ([]writer.FileChange, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(actor, "human:") || actor == "human:" {
		return nil, fmt.Errorf("claim candidate: activation requires a human actor")
	}
	if pending.State != domain.StatePending || !safePart(pending.SubjectSlug) || !safePart(pending.Family) {
		return nil, fmt.Errorf("claim candidate: not a pending claim")
	}
	types, snapshot, err := w.snapshotObservations([]domain.Observation{{SubjectSlug: pending.SubjectSlug, Predicate: pending.Predicate}})
	if err != nil {
		return nil, err
	}
	if err := writer.CheckSnapshot(w.Fence.Root, snapshot); err != nil {
		return nil, err
	}
	_, active, err := w.canonicalReviewClaims(snapshot, now)
	if err != nil {
		return nil, err
	}
	activated := ActivatedClaim(pending, actor)
	for _, c := range active[pending.SubjectSlug] {
		if c.ID == pending.ID {
			return nil, fmt.Errorf("claim candidate: claim %s is already active", pending.ID)
		}
	}
	detection := reconcile.Detect(activated, reconcile.Candidates(activated, active[pending.SubjectSlug]))
	if detection.Verdict == reconcile.VerdictConflict || detection.Verdict == reconcile.VerdictWindowClose {
		return nil, fmt.Errorf("claim candidate: %s would %s live claim %s; reject it or record the change as a human assertion", pending.ID, detection.Verdict, detection.Candidate.ID)
	}
	tier := w.Config.TierOf(pending.Family)
	return writer.PlanFiles(w.Fence.Root, snapshot, func(preview string) error {
		fw, ss := store.NewFenceWriter(preview), store.NewShardStore(preview)
		fw.Vocabulary = w.Fence.Vocabulary
		ss.Vocabulary = w.Shard.Vocabulary
		ss.RolloverBytes = w.Shard.RolloverBytes
		if tier == domain.TierShard {
			return activateShard(ss, pending, activated)
		}
		entityType := types[pending.SubjectSlug]
		return activateFence(fw, fw.PathFor(entityType, pending.SubjectSlug), pending, activated)
	})
}

func sameCandidate(row, pending domain.Claim) bool {
	return row.ID == pending.ID && row.State == domain.StatePending && row.Predicate == pending.Predicate && store.NormalizeKey(row.Object) == store.NormalizeKey(pending.Object) && row.Provenance.SourceSHA256 == pending.Provenance.SourceSHA256
}

func activateShard(ss *store.ShardStore, pending, activated domain.Claim) error {
	lines, err := ss.Lines(pending.SubjectSlug, pending.Family)
	if err != nil {
		return err
	}
	found := false
	for _, row := range lines {
		if row.ID != pending.ID {
			continue
		}
		if row.State == domain.StateActive || row.State == domain.StateRetracted {
			return fmt.Errorf("claim candidate: claim %s is no longer pending", pending.ID)
		}
		found = found || sameCandidate(row, pending)
	}
	if !found {
		return fmt.Errorf("claim candidate: pending claim %s changed or was removed; rerun extraction", pending.ID)
	}
	q := writer.NewQueue(nil)
	defer q.Close()
	_, _, err = writer.Shard(q, ss, activated)
	return err
}

func activateFence(fw *store.FenceWriter, path string, pending, activated domain.Claim) error {
	page, err := fw.ParseEntity(path)
	if err != nil {
		return fmt.Errorf("claim candidate: read canonical entity page: %w", err)
	}
	index := -1
	for i, row := range page.Claims {
		if row.ID == pending.ID {
			if !sameCandidate(row, pending) {
				return fmt.Errorf("claim candidate: pending claim %s changed; review the page before accepting", pending.ID)
			}
			index = i
		}
	}
	if index < 0 {
		return fmt.Errorf("claim candidate: pending claim %s was removed; rerun extraction", pending.ID)
	}
	activated.Object = page.Claims[index].Object
	activated.SourceRef = page.Claims[index].SourceRef
	page.Claims[index] = activated
	q := writer.NewQueue(nil)
	defer q.Close()
	_, _, err = writer.Fence(q, fw, page)
	return err
}
