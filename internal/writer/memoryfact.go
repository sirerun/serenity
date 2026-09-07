package writer

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

// MemoryFact is the single sanctioned entry point for writing MEMORY_VERBS
// v1 fact/expiry sources (T4.20, memory-compat-mapping.md): both remember
// and forget's own allocation logic (legacy-id assignment, exact-dedup
// resolution, idempotent-forget resolution) run entirely inside Queue's
// single drain goroutine, the same "no two writes -- even to different
// files -- ever execute concurrently" guarantee Fence/Shard already lean
// on for canonical claim writes, applied here to canonical source writes
// instead. This is why the allocation logic lives in this package, not in
// internal/store: store's own SourceStore.Write has no such serialization
// of its own (its content-addressed dedup is safe unserialized, but a
// caller-chosen legacy id and a caller-chosen "is this a duplicate" verdict
// are not).
// Allocation and semantic deduplication require all canonical writes for a
// brain to use this same queue. Independent processes are not serialized here;
// concurrent writable stdio servers for one brain are outside this guarantee.
type MemoryFact struct {
	Queue   *Queue
	Sources *store.SourceStore
}

// ErrMemoryFactNotFound is ForgetMemoryFact's own not_found signal: no
// memory_fact source exists under the given id at all (as opposed to one
// that exists but is already expired, which is a normal, successful
// idempotent-forget outcome, not this error).
var ErrMemoryFactNotFound = errors.New("writer: no memory fact with this id")

// RememberInput is the durable half of a remember call -- already
// validated by the MCP-layer verb handler (non-empty fact/provenance,
// closed kind/visibility enums, parsed TTL); this package only allocates
// and writes.
type RememberInput struct {
	Fact       string
	Provenance string
	EntitySlug string
	EntityType string
	Kind       store.MemoryFactKind
	Visibility store.MemoryVisibility
	ValidUntil *time.Time
}

// RememberResult is what the caller needs to build MEMORY_VERBS's remember
// response: id/status/entity_slug/valid_until, plus the full record for
// recall-shaped evidence rendering.
type RememberResult struct {
	Record   store.MemoryFactRecord
	Inserted bool // false means an exact duplicate already existed -- Record is the EXISTING one
}

// Remember writes fact as a new canonical memory_fact source (or resolves
// it to an existing exact duplicate), fully inside one Queue.Submit call.
func (w *MemoryFact) Remember(input RememberInput, now time.Time) (RememberResult, error) {
	if w.Queue == nil || w.Sources == nil {
		return RememberResult{}, fmt.Errorf("writer: memory writer dependencies unavailable")
	}
	var result RememberResult
	var innerErr error
	res := w.Queue.Submit(Job{
		Render: func() ([]byte, error) {
			result, innerErr = w.rememberLocked(input, now)
			return nil, innerErr
		},
	})
	if res.Err != nil {
		return RememberResult{}, res.Err
	}
	return result, nil
}

// rememberLocked runs only from inside the queue's drain goroutine (via
// Remember's Job.Render) -- the single-writer window every allocation and
// dedup decision in this method depends on.
func (w *MemoryFact) rememberLocked(input RememberInput, now time.Time) (RememberResult, error) {
	proj, err := store.LoadMemoryProjection(w.Sources)
	if err != nil {
		return RememberResult{}, err
	}

	key := store.DedupKey(input.Fact, input.Provenance, input.EntitySlug, input.Kind, input.Visibility, input.ValidUntil, input.EntityType)
	if dup, ok := proj.FindDuplicate(key, now); ok {
		w.markSource(dup.SHA256)
		return RememberResult{Record: dup, Inserted: false}, nil
	}

	legacyID, err := proj.NextLegacyID()
	if err != nil {
		return RememberResult{}, err
	}
	payload := store.MemoryFactPayload{
		FormatVersion: store.MemoryFactFormatVersion,
		RecordType:    store.SourceKindMemoryFact,
		LegacyID:      legacyID,
		Fact:          input.Fact,
		Provenance:    input.Provenance,
		EntitySlug:    input.EntitySlug,
		EntityType:    input.EntityType,
		Kind:          input.Kind,
		Visibility:    input.Visibility,
		CreatedAt:     now,
		ValidUntil:    input.ValidUntil,
	}
	written, err := w.Sources.WriteMemoryFact(payload)
	if written.SHA256 != "" {
		w.markSource(written.SHA256)
	}
	if err != nil {
		return RememberResult{}, fmt.Errorf("writer: write memory fact: %w", err)
	}

	return RememberResult{Record: store.MemoryFactRecord{SHA256: written.SHA256, Payload: payload}, Inserted: true}, nil
}

// ForgetResult is what the caller needs to build MEMORY_VERBS's forget
// response.
type ForgetResult struct {
	Record  store.MemoryFactRecord // the (now-expired) record
	Expired bool                   // true = this call expired it; false = it already was
}

// Forget expires the memory_fact source named by targetSHA256 through a
// canonical, audit-preserving memory_expiry source (never edits or deletes
// the original -- mapping: "Existing immutable fact is never edited/
// deleted"). Idempotent by construction: a target already expired (by a
// prior forget, or its own TTL having passed) writes nothing and returns
// Expired=false; an unknown target returns ErrMemoryFactNotFound.
func (w *MemoryFact) Forget(targetSHA256, reason string, now time.Time) (ForgetResult, error) {
	if w.Queue == nil || w.Sources == nil {
		return ForgetResult{}, fmt.Errorf("writer: memory writer dependencies unavailable")
	}
	var result ForgetResult
	var innerErr error
	res := w.Queue.Submit(Job{
		Render: func() ([]byte, error) {
			result, innerErr = w.forgetLocked(targetSHA256, reason, now)
			return nil, innerErr
		},
	})
	if res.Err != nil {
		return ForgetResult{}, res.Err
	}
	return result, nil
}

func (w *MemoryFact) forgetLocked(targetSHA256, reason string, now time.Time) (ForgetResult, error) {
	proj, err := store.LoadMemoryProjection(w.Sources)
	if err != nil {
		return ForgetResult{}, err
	}
	rec, ok := proj.Get(targetSHA256)
	if !ok {
		return ForgetResult{}, ErrMemoryFactNotFound
	}
	if rec.Expired(now) {
		w.markSource(rec.SHA256)
		if rec.ExpirySHA256 != "" {
			w.markSource(rec.ExpirySHA256)
		}
		return ForgetResult{Record: rec, Expired: false}, nil
	}

	payload := store.MemoryExpiryPayload{
		FormatVersion: store.MemoryFactFormatVersion,
		RecordType:    store.SourceKindMemoryExpiry,
		TargetSHA256:  targetSHA256,
		Reason:        reason,
		ExpiredAt:     now,
	}
	written, err := w.Sources.WriteMemoryExpiry(payload)
	if written.SHA256 != "" {
		w.markSource(written.SHA256)
	}
	if err != nil {
		return ForgetResult{}, fmt.Errorf("writer: write memory expiry: %w", err)
	}
	rec.ExpirySHA256 = written.SHA256

	rec.ExpiredAt = &now
	rec.ExpiredReason = reason
	return ForgetResult{Record: rec, Expired: true}, nil
}

// ByLegacyOrOpaqueID resolves a MEMORY_VERBS id -- the opaque SHA256
// remember/recall hand back, OR the frozen legacy numeric id (recall.
// facts[].id) rendered as a decimal string -- to the full SHA256 forget
// needs. A candidate that is neither a known opaque id nor a strictly
// well-formed positive decimal integer resolves to ok=false, never a
// partial/loose numeric parse.
func ByLegacyOrOpaqueID(p *store.MemoryProjection, id string) (sha string, ok bool) {
	if rec, found := p.Get(id); found {
		return rec.SHA256, true
	}
	if n, err := strconv.ParseInt(id, 10, 64); err == nil && n > 0 && n <= store.MaxMemoryLegacyID && strconv.FormatInt(n, 10) == id {
		if rec, found := p.ByLegacyID(n); found {
			return rec.SHA256, true
		}
	}
	return "", false
}

// Mark exact files even for a duplicate so retry after a failed commit/restart
// retains the canonical commit obligation without allocating a second fact.
func (w *MemoryFact) markSource(sha string) {
	if !store.ValidSourceSHA(sha) {
		return
	}
	dir := w.Sources.DirFor(sha)
	w.Queue.MarkTouched(filepath.Join(dir, "bytes"))
	w.Queue.MarkTouched(filepath.Join(dir, "meta.yaml"))
}
