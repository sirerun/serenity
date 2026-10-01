package writer

import (
	"context"
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
	// Index, when set, loses a forgotten fact's FTS and vector rows inside
	// the same queue job that removes its bytes (ADR 019).
	Index IndexPurger
}

// IndexPurger deletes the derived index rows of one source SHA-256.
// *index.SQLite implements it; the writer never imports the index.
type IndexPurger interface {
	PurgeSource(ctx context.Context, sha string) error
}

// ErrMemoryFactNotFound is ForgetMemoryFact's own not_found signal: no
// memory_fact source exists under the given id at all (as opposed to one
// that exists but is already expired, which is a normal, successful
// idempotent-forget outcome, not this error).
var ErrMemoryFactNotFound = errors.New("writer: no memory fact with this id")

// ErrMemoryOperationConflict means a durable key was reused with different input.
var ErrMemoryOperationCanceled = errors.New("writer: memory operation was canceled")
var ErrMemoryScopeDenied = errors.New("writer: memory fact is outside remote scope")

var ErrMemoryOperationConflict = errors.New("writer: memory operation key conflicts with prior input")

// RememberInput is the durable half of a remember call -- already
// validated by the MCP-layer verb handler (non-empty fact/provenance,
// closed kind/visibility enums, parsed TTL); this package only allocates
// and writes.
type RememberInput struct {
	OperationKey string
	Fact         string
	Provenance   string
	EntitySlug   string
	EntityType   string
	Kind         store.MemoryFactKind
	Visibility   store.MemoryVisibility
	ValidUntil   *time.Time
	// Writer is the principal making the call, recorded as `writer:` in the
	// fact's meta.yaml so forget can be restricted to it (AI-L03). Empty
	// records no writer; such a fact is forgettable only by a human.
	Writer string
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
	return w.RememberContext(context.Background(), input, now)
}

// RememberContext carries trusted operation metadata through the serialized
// queue job without exposing it to request decoding or client operation keys.
func (w *MemoryFact) RememberContext(ctx context.Context, input RememberInput, now time.Time) (RememberResult, error) {
	if w.Queue == nil || w.Sources == nil {
		return RememberResult{}, fmt.Errorf("writer: memory writer dependencies unavailable")
	}
	operation := canonicalOperationFromContext(ctx)
	inlineFlush := operation.AfterFlush != nil
	if inlineFlush && (operation.ID == "" || input.OperationKey != operation.ID || operation.BeforeCommit == nil || !store.ValidMemoryOperationKey(operation.ID)) {
		return RememberResult{}, fmt.Errorf("writer: invalid canonical operation metadata")
	}
	entered := false
	if inlineFlush {
		beforeCommit := operation.BeforeCommit
		operation.BeforeCommit = func(commitCtx context.Context, operationID string) error {
			if err := beforeCommit(commitCtx, operationID); err != nil {
				return err
			}
			entered = true
			return nil
		}
	}
	var result RememberResult
	var innerErr error
	job := Job{
		Render: func() ([]byte, error) {
			result, innerErr = w.rememberLocked(ctx, input, now, operation)
			return nil, innerErr
		},
	}
	var err error
	if inlineFlush {
		flushed := w.Queue.SubmitAndFlush(ctx, w.Sources.Root, job)
		err = flushed.Result.Err
	} else {
		err = w.Queue.Submit(job).Err
	}
	if err != nil {
		return RememberResult{}, err
	}
	if inlineFlush && entered && result.Record.Payload.CanonicalOperationID == operation.ID && result.Record.SHA256 != "" {
		operation.AfterFlush(ctx, operation.ID, result.Record.SHA256)
	}
	return result, nil
}

// rememberLocked runs only from inside the queue's drain goroutine (via
// Remember's Job.Render) -- the single-writer window every allocation and
// dedup decision in this method depends on.
func (w *MemoryFact) rememberLocked(ctx context.Context, input RememberInput, now time.Time, operation CanonicalOperation) (RememberResult, error) {
	if !store.ValidMemoryOperationKey(input.OperationKey) {
		return RememberResult{}, fmt.Errorf("writer: invalid memory operation key")
	}
	proj, err := store.LoadMemoryProjection(w.Sources)
	if err != nil {
		return RememberResult{}, err
	}

	key := store.DedupKey(input.Fact, input.Provenance, input.EntitySlug, input.Kind, input.Visibility, input.ValidUntil, input.EntityType)
	if input.OperationKey != "" {
		// Look through expired records too: retry must never resurrect a withdrawal.
		for _, rec := range proj.All() {
			if rec.Payload.OperationKey != input.OperationKey {
				continue
			}
			p := rec.Payload
			prior := store.DedupKey(p.Fact, p.Provenance, p.EntitySlug, p.Kind, p.Visibility, p.ValidUntil, p.EntityType)
			if prior != key {
				return RememberResult{}, ErrMemoryOperationConflict
			}
			w.markSource(rec.SHA256)
			w.markSource(rec.ExpirySHA256)
			return RememberResult{Record: rec, Inserted: false}, nil
		}
		if sha, canceled := proj.OperationCancellation(input.OperationKey); canceled {
			w.markSource(sha)
			return RememberResult{}, ErrMemoryOperationCanceled
		}
	} else if dup, ok := proj.FindDuplicate(key, now); ok {
		w.markSource(dup.SHA256)
		return RememberResult{Record: dup, Inserted: false}, nil
	}

	legacyID, err := proj.NextLegacyID()
	if err != nil {
		return RememberResult{}, err
	}
	if operation.ID != "" {
		if !store.ValidMemoryOperationKey(operation.ID) || operation.BeforeCommit == nil {
			return RememberResult{}, fmt.Errorf("writer: invalid canonical operation metadata")
		}
		if err := operation.BeforeCommit(ctx, operation.ID); err != nil {
			return RememberResult{}, err
		}
	}
	payload := store.MemoryFactPayload{
		OperationKey:         input.OperationKey,
		CanonicalOperationID: operation.ID,
		FormatVersion:        store.MemoryFactFormatVersion,
		RecordType:           store.SourceKindMemoryFact,
		LegacyID:             legacyID,
		Fact:                 input.Fact,
		Provenance:           input.Provenance,
		EntitySlug:           input.EntitySlug,
		EntityType:           input.EntityType,
		Kind:                 input.Kind,
		Visibility:           input.Visibility,
		CreatedAt:            now,
		ValidUntil:           input.ValidUntil,
	}
	written, err := w.Sources.WriteMemoryFactBy(payload, input.Writer)
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

// Forget erases the memory_fact source named by targetSHA256 (ADR 019): it
// writes a memory_expiry source as the audit record (target SHA-256 and
// reason only, never the fact text), deletes the fact's FTS and vector rows
// through Index, and removes the fact's bytes and meta.yaml, all in one
// queue job so the next Flush commits the deletion and the event together.
// A fact carrying an operation key also gets a cancellation fence, so a
// retried remember under that key can never write the text back.
// Idempotent: a target already expired (by a prior forget, or its own TTL
// having passed) writes no new expiry, finishes any erasure a failed
// attempt left behind, and returns Expired=false; so does a target whose
// erasure already completed. An unknown target returns
// ErrMemoryFactNotFound.
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

// ForgetContext runs the hosted, fenced variant of forget: its source expiry
// and erasure are rendered and flushed under one shared canonical commit
// section. Local callers retain Forget's historical queue-and-later-flush
// behavior.
func (w *MemoryFact) ForgetContext(ctx context.Context, targetSHA256, reason string, now time.Time) (ForgetResult, error) {
	if ctx == nil {
		return ForgetResult{}, ErrNilCommitContext
	}
	if w.Queue == nil || w.Sources == nil {
		return ForgetResult{}, fmt.Errorf("writer: memory writer dependencies unavailable")
	}
	var result ForgetResult
	flushed := w.Queue.SubmitAndFlush(ctx, w.Sources.Root, Job{
		Kind: "hosted memory forget",
		Render: func() ([]byte, error) {
			var err error
			result, err = w.forgetLocked(targetSHA256, reason, now)
			return nil, err
		},
	})
	if flushed.Result.Err != nil {
		return ForgetResult{}, flushed.Result.Err
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
		expiry, erased := proj.ErasedExpiry(targetSHA256)
		if !erased {
			return ForgetResult{}, ErrMemoryFactNotFound
		}
		rec = store.MemoryFactRecord{SHA256: targetSHA256, ExpirySHA256: expiry}
		w.markSource(expiry)
		return ForgetResult{Record: rec, Expired: false}, w.eraseFact(targetSHA256, "", proj, now)
	}
	if rec.ExpirySHA256 != "" {
		w.markSource(rec.ExpirySHA256)
		return ForgetResult{Record: rec, Expired: false}, w.eraseFact(rec.SHA256, rec.Payload.OperationKey, proj, now)
	}
	// A fact past its own TTL is already unavailable, but erasing it still
	// needs the expiry event as its audit record and idempotency trace.
	ttlExpired := rec.TTLExpired(now)

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
	if ttlExpired {
		return ForgetResult{Record: rec, Expired: false}, w.eraseFact(rec.SHA256, rec.Payload.OperationKey, proj, now)
	}

	rec.ExpiredAt = &now
	rec.ExpiredReason = reason
	return ForgetResult{Record: rec, Expired: true}, w.eraseFact(rec.SHA256, rec.Payload.OperationKey, proj, now)
}

// eraseFact runs inside the queue job, after the expiry event exists: it
// fences the fact's operation key, deletes its index rows, then removes its
// bytes and meta.yaml and marks both paths so the next Flush stages the
// deletion. Each step is idempotent, so a retried forget completes a
// partial erasure.
func (w *MemoryFact) eraseFact(sha, operationKey string, proj *store.MemoryProjection, now time.Time) error {
	if operationKey != "" {
		if fence, canceled := proj.OperationCancellation(operationKey); canceled {
			w.markSource(fence)
		} else {
			fence, err := w.Sources.WriteMemoryExpiry(store.MemoryExpiryPayload{OperationKey: operationKey, ExpiredAt: now})
			if fence.SHA256 != "" {
				w.markSource(fence.SHA256)
			}
			if err != nil {
				return fmt.Errorf("writer: fence forgotten operation: %w", err)
			}
		}
	}
	if w.Index != nil {
		if err := w.Index.PurgeSource(context.Background(), sha); err != nil {
			return fmt.Errorf("writer: purge forgotten fact index rows: %w", err)
		}
	}
	w.markSource(sha)
	if _, err := w.Sources.RemoveSource(sha); err != nil {
		return fmt.Errorf("writer: remove forgotten fact: %w", err)
	}
	root, err := filepath.Abs(w.Sources.Root)
	if err != nil {
		return fmt.Errorf("writer: resolve brain root for history rewrite: %w", err)
	}
	sourceDir, err := filepath.Abs(w.Sources.DirFor(sha))
	if err != nil {
		return fmt.Errorf("writer: resolve forgotten source directory: %w", err)
	}
	path, err := filepath.Rel(root, sourceDir)
	if err != nil {
		return fmt.Errorf("writer: resolve forgotten source path: %w", err)
	}
	if err := rewriteForgottenPath(root, path); err != nil {
		return err
	}
	return nil
}

// FactWriter returns the writer principal recorded in the meta.yaml of the
// memory_fact source sha, or "" for a fact written before writers were
// recorded.
func (w *MemoryFact) FactWriter(sha string) (string, error) {
	if w.Sources == nil {
		return "", fmt.Errorf("writer: memory writer dependencies unavailable")
	}
	_, src, err := w.Sources.Read(sha)
	if err != nil {
		return "", err
	}
	if src.Kind != store.SourceKindMemoryFact {
		return "", ErrMemoryFactNotFound
	}
	return src.Meta[store.MemoryFactWriterMetaKey], nil
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

// CancelRemoteOperation durably fences a key and expires any matching world
// fact, without needing its body or an acknowledged fact ID. A later remember
// cannot create a missing canceled operation. Existing facts still recover their
// expired identity. The complete decision is serialized with Remember.
func (w *MemoryFact) CancelRemoteOperation(key, reason string, now time.Time) (ForgetResult, error) {
	if w.Queue == nil || w.Sources == nil || key == "" || !store.ValidMemoryOperationKey(key) {
		return ForgetResult{}, fmt.Errorf("writer: invalid cancellation dependencies/key")
	}
	var result ForgetResult
	res := w.Queue.Submit(Job{Render: func() ([]byte, error) {
		proj, err := store.LoadMemoryProjection(w.Sources)
		if err != nil {
			return nil, err
		}
		for _, rec := range proj.All() {
			if rec.Payload.OperationKey != key {
				continue
			}
			if rec.Payload.Visibility != store.MemoryVisibilityWorld {
				return nil, ErrMemoryScopeDenied
			}
			result.Record = rec
			result.Expired = !rec.Expired(now)
			break
		}
		if sha, canceled := proj.OperationCancellation(key); canceled {
			w.markSource(sha)
			result.Expired = false
			return nil, nil
		}
		written, err := w.Sources.WriteMemoryExpiry(store.MemoryExpiryPayload{OperationKey: key, Reason: reason, ExpiredAt: now})
		if written.SHA256 != "" {
			w.markSource(written.SHA256)
		}
		return nil, err
	}})
	if res.Err != nil {
		return ForgetResult{}, res.Err
	}
	return result, nil
}
