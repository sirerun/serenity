// Package ingest is the observation-to-claim write path (RFC 0001 §7.6,
// §9, §10.1; T1.9): the seam between extraction (internal/extract, T1.8)
// and reconciliation (internal/reconcile, T2.2, E2).
//
// Trust 0, by design (RFC §10.3's starting posture): every Ready
// observation becomes its own claim. There is no merging, no conflict
// detection, and no supersession here -- that is the reconcile engine's
// job, deferred to E2. The only dedup this package performs is
// identity-level, not semantic: a derived claim id already present in its
// target fence page or shard file is skipped rather than re-appended, so
// re-ingesting an unchanged source through T1.8's deterministic pipeline
// (its own output cache, keyed by chunk content, plus this package's
// content-derived ids) never grows the brain repo. The same logical claim
// observed from two different sources still gets two distinct ids --
// that is corroboration, not a bug (§7.2) -- collapsing those into one
// belief is exactly the semantic dedup E2's reconcile engine performs at
// (subject, predicate) plus embedding-similarity neighbors, never here.
package ingest

import (
	"fmt"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/extract"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// DefaultEntityType is the fence-tier folder every trust-0 claim lands
// under until entity resolution (RFC §10.5, E2) assigns a real type.
// Observations carry only a subject slug, never an entity type, so every
// subject shares this bucket until that resolver exists; Writer.EntityType
// lets a caller override it per slug once one does.
const DefaultEntityType = "topic"

// Writer commits Ready observations to the canonical brain repo through
// the writer queue (RFC §7.7) -- the only sanctioned entry point for
// FenceWriter/ShardStore after M0. Construct with New; the zero value is
// not usable (Queue/Fence/Shard are required).
type Writer struct {
	Queue *writer.Queue
	Fence *store.FenceWriter
	Shard *store.ShardStore
	// Config supplies the tier assignment (config.TierOf) that decides
	// whether a predicate family lands in a fence or a shard (§7.2a). Nil
	// falls back to config.Default().
	Config *config.Config
	// EntityType resolves the fence-tier folder for a subject slug. Nil
	// defaults every slug to DefaultEntityType.
	EntityType func(subjectSlug string) string

	// pages caches parsed entity pages by path for the duration of one
	// Write call, so N observations against the same entity read the file
	// once, not N times, and see each other's just-added claims.
	pages map[string]*store.EntityPage
	// shardIDs caches the set of claim ids already present in a shard file
	// (by path) for the duration of one Write call, for the same reason.
	shardIDs map[string]map[string]bool
}

// New builds a Writer. q, fw, and ss must be non-nil. cfg nil defaults to
// config.Default().
func New(q *writer.Queue, fw *store.FenceWriter, ss *store.ShardStore, cfg *config.Config) *Writer {
	if cfg == nil {
		cfg = config.Default()
	}
	return &Writer{Queue: q, Fence: fw, Shard: ss, Config: cfg}
}

// Stats summarizes one Write call.
type Stats struct {
	// Written counts claims newly committed to a fence or shard.
	Written int
	// Skipped counts observations whose derived claim id already existed
	// at the target -- the source-level re-ingest no-op this task's acc
	// line requires, not an error.
	Skipped int
}

// Write commits every observation in obs as its own claim, in the order
// given. obs must be a Result.Ready slice (internal/extract) -- an
// observation below extract.DistillThreshold aborts the whole call with
// an error, exactly as Extractor.Extract aborts on a per-chunk failure:
// partial, silently-degraded ingestion is never reported as success. A
// caller must route Result.Distill elsewhere; this package never writes a
// sub-threshold observation to a fence or shard.
func (w *Writer) Write(obs []domain.Observation) (Stats, error) {
	var stats Stats
	w.pages = map[string]*store.EntityPage{}
	w.shardIDs = map[string]map[string]bool{}
	defer func() { w.pages, w.shardIDs = nil, nil }()

	for _, o := range obs {
		if o.Confidence < extract.DistillThreshold {
			return stats, fmt.Errorf("ingest: observation %s confidence %.2f below distill threshold %.2f -- pass only Result.Ready, never Result.Distill",
				o.ID, o.Confidence, extract.DistillThreshold)
		}

		c := ClaimFromObservation(o)
		tier := w.Config.TierOf(c.Family)

		written, err := w.writeClaim(tier, c)
		if err != nil {
			return stats, fmt.Errorf("ingest: observation %s: %w", o.ID, err)
		}
		if written {
			stats.Written++
		} else {
			stats.Skipped++
		}
	}
	return stats, nil
}

// writeClaim commits one claim if its id is not already present at the
// target, reporting whether it actually wrote.
func (w *Writer) writeClaim(tier domain.Tier, c domain.Claim) (bool, error) {
	if tier == domain.TierShard {
		return w.writeShardClaim(c)
	}
	return w.writeFenceClaim(c)
}

func (w *Writer) writeShardClaim(c domain.Claim) (bool, error) {
	path := w.Shard.PathFor(c.SubjectSlug, c.Family)
	ids, ok := w.shardIDs[path]
	if !ok {
		lines, err := w.Shard.Lines(c.SubjectSlug, c.Family)
		if err != nil {
			return false, fmt.Errorf("read shard %s: %w", path, err)
		}
		ids = make(map[string]bool, len(lines))
		for _, l := range lines {
			ids[l.ID] = true
		}
		w.shardIDs[path] = ids
	}
	if ids[c.ID] {
		return false, nil
	}
	if _, _, err := writer.Shard(w.Queue, w.Shard, c); err != nil {
		return false, fmt.Errorf("shard write: %w", err)
	}
	ids[c.ID] = true
	return true, nil
}

func (w *Writer) writeFenceClaim(c domain.Claim) (bool, error) {
	entityType := DefaultEntityType
	if w.EntityType != nil {
		if t := w.EntityType(c.SubjectSlug); t != "" {
			entityType = t
		}
	}
	path := w.Fence.PathFor(entityType, c.SubjectSlug)

	p, ok := w.pages[path]
	if !ok {
		parsed, err := w.Fence.ParseEntity(path)
		if err == nil {
			p = parsed
		} else {
			// No existing page (or unreadable): start a fresh one. A
			// genuine read error (bad permissions, corrupt frontmatter on
			// an existing file) also falls here rather than a hard stop --
			// consistent with FenceWriter.WriteEntity's own no-op-on-first-
			// write posture; a truly corrupt page still fails loudly, at
			// RenderEntity/WriteEntity time below, once real content is at
			// stake.
			p = store.NewEntityPage(domain.Entity{Type: entityType, Slug: c.SubjectSlug})
		}
		w.pages[path] = p
	}

	for _, existing := range p.Claims {
		if existing.ID == c.ID {
			return false, nil
		}
	}
	p.Claims = append(p.Claims, c)

	if _, _, err := writer.Fence(w.Queue, w.Fence, p); err != nil {
		return false, fmt.Errorf("fence write: %w", err)
	}
	return true, nil
}

// ClaimFromObservation converts a machine extraction (epistemic layer 2)
// into a trust-0 active claim (layer 3, RFC §7.6) -- a pure, deterministic
// mapping with no I/O, so it is unit-testable independent of the writer
// queue and the acceptance line "every written claim carries
// SourceSHA256#span, model@version, observed_at" is checkable directly on
// its result.
//
// ValidFrom is left empty: observations carry no validity window yet
// (temporal claims are E2 work, RFC §10.2); the claim id derivation
// (§7.2: subject, predicate, normalized object, valid_from, source ref)
// still yields distinct ids across distinct sources with valid_from held
// constant at "".
func ClaimFromObservation(o domain.Observation) domain.Claim {
	objectKey := store.NormalizeKey(o.Object)
	prov := domain.Provenance{
		SourceSHA256: o.SourceSHA256,
		Span:         o.Span,
		Model:        o.Model,
		ObservedAt:   o.CreatedAt,
		Actor:        "machine",
	}
	return domain.Claim{
		ID:          store.DerivedID(o.SubjectSlug, o.Predicate, objectKey, "", o.SourceSHA256, store.DefaultIDWidth),
		SubjectSlug: o.SubjectSlug,
		Predicate:   o.Predicate,
		Object:      o.Object,
		ObjectKey:   objectKey,
		Confidence:  o.Confidence,
		State:       domain.StateActive,
		SourceRef:   shortSourceRef(o.SourceSHA256, o.Span),
		Family:      o.Predicate, // families are 1:1 with predicates in the seed vocabulary (store/fence.go)
		Provenance:  prov,
	}
}

// shortSourceRef renders the human-readable src cell (§7.2's table has no
// sha256 column): the source's first 8 hex characters -- enough to
// eyeball-correlate against `brain/sources/<sha[0:2]>/<sha>/` (§7.4)
// without a full 64-char hash in every row -- plus the exact byte span.
func shortSourceRef(sha, span string) string {
	if len(sha) > 8 {
		sha = sha[:8]
	}
	return sha + "#" + span
}
