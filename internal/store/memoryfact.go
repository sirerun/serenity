package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sirerun/serenity/internal/domain"
)

// MEMORY_VERBS v1 (T4.20) persists a raw attributed fact as a typed source
// payload inside the EXISTING content-addressed SourceStore -- a source
// projection, not a second authoritative database (memory-compat-mapping.md,
// coordinator-approved). This file is the codec: two reserved
// domain.Source.Kind values (SourceKindMemoryFact, SourceKindMemoryExpiry),
// each carrying a canonical JSON payload as the source's own bytes. Nothing
// here writes to disk -- that is internal/writer's MemoryFact entry point
// (writer/memoryfact.go), which serializes allocation and writing through
// Queue's single drain goroutine.

// Reserved domain.Source.Kind values for a memory-fact source (this file's
// own record_type strings, duplicated onto Source.Kind so Rebuild and every
// other existing Source enumeration can recognize one without decoding the
// payload first).
const (
	SourceKindMemoryFact   = "memory_fact"
	SourceKindMemoryExpiry = "memory_expiry"
)

// MemoryFactFormatVersion is the payload's own format_version -- independent
// of MEMORY_VERBS_v1's protocol_version (mapping doc: "Keep protocol_version
// out of internal storage unless explicitly useful; format_version is
// independent").
const MemoryFactFormatVersion = 1

// MemoryFactKind mirrors upstream-verbs.ts's FACT_KINDS -- a closed enum,
// frozen by the pinned contract.
type MemoryFactKind string

const (
	MemoryFactKindEvent      MemoryFactKind = "event"
	MemoryFactKindPreference MemoryFactKind = "preference"
	MemoryFactKindCommitment MemoryFactKind = "commitment"
	MemoryFactKindBelief     MemoryFactKind = "belief"
	MemoryFactKindFact       MemoryFactKind = "fact"
)

// ValidMemoryFactKind reports whether k is one of the five pinned kinds.
func ValidMemoryFactKind(k string) bool {
	switch MemoryFactKind(k) {
	case MemoryFactKindEvent, MemoryFactKindPreference, MemoryFactKindCommitment, MemoryFactKindBelief, MemoryFactKindFact:
		return true
	}
	return false
}

// MemoryVisibility mirrors upstream-verbs.ts's visibility enum. This maps
// 1:1 onto domain.Visibility's own private/shared pair at this one
// boundary (mapping doc coordinator note: "Explicitly map domain
// private/shared to private/world at this boundary without changing global
// consent defaults") -- shared==world; nothing about domain.Visibility's
// own semantics elsewhere changes.
type MemoryVisibility string

const (
	MemoryVisibilityWorld   MemoryVisibility = "world"
	MemoryVisibilityPrivate MemoryVisibility = "private"
)

// MemoryFactPayload is the canonical JSON raw-ingress document stored as one
// memory_fact source's bytes. It is original attributed request material,
// never a domain.Claim -- MEMORY_VERBS's remember has no subject/predicate/
// object of its own, and this payload never becomes one (mapping doc:
// "never automatic accepted belief").
type MemoryFactPayload struct {
	OperationKey  string           `json:"operation_key,omitempty"`
	FormatVersion int              `json:"format_version"`
	RecordType    string           `json:"record_type"` // always "memory_fact"
	LegacyID      int64            `json:"legacy_id"`
	Fact          string           `json:"fact"`
	Provenance    string           `json:"provenance"`
	EntitySlug    string           `json:"entity_slug,omitempty"`
	EntityType    string           `json:"entity_type,omitempty"`
	Kind          MemoryFactKind   `json:"kind"`
	Visibility    MemoryVisibility `json:"visibility"`
	CreatedAt     time.Time        `json:"created_at"`
	ValidUntil    *time.Time       `json:"valid_until,omitempty"`
}

// MemoryExpiryPayload is the canonical JSON document for a forget/expiry
// lifecycle event: an immutable source in its own right (Source.Kind ==
// SourceKindMemoryExpiry) that refers to the fact source it closes rather
// than editing or deleting it (mapping doc: "Existing immutable fact is
// never edited/deleted").
type MemoryExpiryPayload struct {
	FormatVersion int       `json:"format_version"`
	RecordType    string    `json:"record_type"` // always "memory_expiry"
	TargetSHA256  string    `json:"target_sha256,omitempty"`
	OperationKey  string    `json:"operation_key,omitempty"`
	Reason        string    `json:"reason,omitempty"`
	ExpiredAt     time.Time `json:"expired_at"`
}

// EncodeMemoryFact renders p as its canonical JSON source bytes, validating
// the closed fields first -- a payload this package itself cannot produce
// never reaches SourceStore.Write.
func EncodeMemoryFact(p MemoryFactPayload) ([]byte, error) {
	p.RecordType = SourceKindMemoryFact
	if p.FormatVersion == 0 {
		p.FormatVersion = MemoryFactFormatVersion
	}
	if err := validateMemoryFact(p); err != nil {
		return nil, err
	}
	return json.Marshal(p)
}

// DecodeMemoryFact recognizes a memory_fact source only through the
// reserved kind AND a validated versioned payload (coordinator refinement
// #3: "imported JSON cannot forge an expiry instruction" -- read
// symmetrically for a forged fact record too): malformed or
// unrecognized-version bytes are a hard error, never silently treated as an
// ordinary source.
func DecodeMemoryFact(data []byte) (MemoryFactPayload, error) {
	if !utf8.Valid(data) {
		return MemoryFactPayload{}, fmt.Errorf("store: memory fact JSON is not UTF-8")
	}
	var p MemoryFactPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return MemoryFactPayload{}, fmt.Errorf("store: corrupt memory_fact payload: %w", err)
	}
	if err := validateMemoryFact(p); err != nil {
		return MemoryFactPayload{}, err
	}
	return p, nil
}

// EncodeMemoryExpiry renders p as its canonical JSON source bytes.
func EncodeMemoryExpiry(p MemoryExpiryPayload) ([]byte, error) {
	if p.OperationKey != "" && p.FormatVersion == 0 {
		p.FormatVersion = 2
	}
	p.RecordType = SourceKindMemoryExpiry
	if p.FormatVersion == 0 {
		p.FormatVersion = MemoryFactFormatVersion
	}
	if err := validateMemoryExpiry(p); err != nil {
		return nil, err
	}
	return json.Marshal(p)
}

// DecodeMemoryExpiry mirrors DecodeMemoryFact's fail-closed contract.
func DecodeMemoryExpiry(data []byte) (MemoryExpiryPayload, error) {
	if !utf8.Valid(data) {
		return MemoryExpiryPayload{}, fmt.Errorf("store: memory expiry JSON is not UTF-8")
	}
	var p MemoryExpiryPayload
	if err := json.Unmarshal(data, &p); err != nil {
		return MemoryExpiryPayload{}, fmt.Errorf("store: corrupt memory_expiry payload: %w", err)
	}
	if err := validateMemoryExpiry(p); err != nil {
		return MemoryExpiryPayload{}, err
	}
	return p, nil
}

// MaxMemoryLegacyID is the largest integer exactly representable by JSON clients.
const MaxMemoryLegacyID int64 = 1<<53 - 1

// ValidMemoryOperationKey accepts bounded opaque ASCII keys. Empty means the
// original unkeyed contract. Keys are brain-scoped identifiers, not credentials.
func ValidMemoryOperationKey(key string) bool {
	if len(key) > 128 {
		return false
	}
	for _, c := range key {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.', c == ':':
		default:
			return false
		}
	}
	return true
}

func validateMemoryFact(p MemoryFactPayload) error {
	if !ValidMemoryOperationKey(p.OperationKey) {
		return fmt.Errorf("store: invalid memory operation key")
	}

	if p.RecordType != SourceKindMemoryFact || p.FormatVersion != MemoryFactFormatVersion {
		return fmt.Errorf("store: invalid memory fact type/version")
	}
	if p.LegacyID <= 0 || p.LegacyID > MaxMemoryLegacyID {
		return fmt.Errorf("store: invalid memory legacy ID %d", p.LegacyID)
	}
	if strings.TrimSpace(p.Fact) == "" || !utf8.ValidString(p.Fact) {
		return fmt.Errorf("store: memory fact is required UTF-8 text")
	}
	if strings.TrimSpace(p.Provenance) == "" || !utf8.ValidString(p.Provenance) || utf8.RuneCountInString(p.Provenance) > 500 {
		return fmt.Errorf("store: invalid memory provenance")
	}
	if !ValidMemoryFactKind(string(p.Kind)) || (p.Visibility != MemoryVisibilityWorld && p.Visibility != MemoryVisibilityPrivate) {
		return fmt.Errorf("store: invalid memory kind/visibility")
	}
	if p.CreatedAt.IsZero() || (p.ValidUntil != nil && p.ValidUntil.IsZero()) {
		return fmt.Errorf("store: missing memory timestamp")
	}
	if !utf8.ValidString(p.EntitySlug) || !utf8.ValidString(p.EntityType) {
		return fmt.Errorf("store: invalid memory entity text")
	}
	return nil
}

func validateMemoryExpiry(p MemoryExpiryPayload) error {
	if p.RecordType != SourceKindMemoryExpiry || p.ExpiredAt.IsZero() || !utf8.ValidString(p.Reason) {
		return fmt.Errorf("store: invalid memory expiry type/timestamp/reason")
	}
	if p.OperationKey != "" {
		if p.FormatVersion != 2 || p.TargetSHA256 != "" || !ValidMemoryOperationKey(p.OperationKey) {
			return fmt.Errorf("store: invalid memory operation cancellation")
		}
	} else if p.FormatVersion != MemoryFactFormatVersion || !ValidSourceSHA(p.TargetSHA256) {
		return fmt.Errorf("store: invalid memory expiry target/version")
	}
	return nil
}

// MemoryFactRecord is one memory_fact source fully resolved against every
// memory_expiry event citing it -- the read shape every MEMORY_VERBS verb
// and the shared search/compose eligibility filter use, never a repeated
// SourceStore.All() scan per call.
type MemoryFactRecord struct {
	SHA256        string // opaque fact_id / forget's id
	Payload       MemoryFactPayload
	ExpiredAt     *time.Time // set once a memory_expiry event targets this fact
	ExpirySHA256  string
	ExpiredReason string
}

// TTLExpired reports whether r's own valid_until has passed as of now --
// independent of whether an explicit forget ever ran (mapping: "Capture one
// query clock for TTL consistency" -- callers pass the single now they
// captured for their whole request).
func (r MemoryFactRecord) TTLExpired(now time.Time) bool {
	return r.Payload.ValidUntil != nil && !r.Payload.ValidUntil.After(now)
}

// Expired reports whether r is unavailable for any reason -- an explicit
// forget, or its own TTL having passed -- the single check every read path
// (recall, entity, search eligibility, forget's own idempotency) uses so
// none of them can diverge on what "still active" means.
func (r MemoryFactRecord) Expired(now time.Time) bool {
	return r.ExpiredAt != nil || r.TTLExpired(now)
}

// MemoryProjection is a point-in-time read over every memory_fact/
// memory_expiry source, loaded once per query or rebuild pass (mapping
// item 5: "a source SHA lookup/projection loaded once per query can
// provide the filter without SQL migrations"). It never allocates identity
// or owns TTL/expiry itself -- purely a derived read of what SourceStore
// already holds, safe to rebuild from scratch at any time.
type MemoryProjection struct {
	bySHA     map[string]*MemoryFactRecord
	canceled  map[string]string
	lifecycle map[string]bool
	indexOnly map[string]bool
}

// LoadMemoryProjection reads every source ss has ever recorded and resolves
// the memory-fact/expiry subset. Corrupt or wrong-version payloads under a
// reserved kind fail the whole load closed (coordinator refinement #1:
// "Unknown or corrupt reserved source state fails closed, never generic-
// source fallback") rather than being silently skipped.
func LoadMemoryProjection(ss *SourceStore) (*MemoryProjection, error) {
	sources, err := ss.All()
	if err != nil {
		return nil, fmt.Errorf("store: load memory projection: %w", err)
	}
	p := &MemoryProjection{canceled: make(map[string]string), bySHA: make(map[string]*MemoryFactRecord), lifecycle: make(map[string]bool), indexOnly: make(map[string]bool)}
	legacy := make(map[int64]string)
	operations := make(map[string]string)

	var expiries []struct {
		sha string
		pl  MemoryExpiryPayload
	}
	for _, src := range sources {
		p.indexOnly[src.SHA256] = src.IndexOnly
		switch src.Kind {
		case SourceKindMemoryFact:
			data, _, err := ss.Read(src.SHA256)
			if err != nil {
				return nil, fmt.Errorf("store: read memory fact %s: %w", src.SHA256, err)
			}
			pl, err := DecodeMemoryFact(data)
			if err != nil {
				return nil, fmt.Errorf("store: memory fact %s: %w", src.SHA256, err)
			}
			if other, exists := legacy[pl.LegacyID]; exists {
				return nil, fmt.Errorf("store: duplicate memory legacy ID %d in %s and %s", pl.LegacyID, other, src.SHA256)
			}
			if pl.OperationKey != "" {
				if _, exists := operations[pl.OperationKey]; exists {
					return nil, fmt.Errorf("store: duplicate memory operation key")
				}
				operations[pl.OperationKey] = src.SHA256
			}
			legacy[pl.LegacyID] = src.SHA256
			p.bySHA[src.SHA256] = &MemoryFactRecord{SHA256: src.SHA256, Payload: pl}
		case SourceKindMemoryExpiry:
			p.lifecycle[src.SHA256] = true
			data, _, err := ss.Read(src.SHA256)
			if err != nil {
				return nil, fmt.Errorf("store: read memory expiry %s: %w", src.SHA256, err)
			}
			pl, err := DecodeMemoryExpiry(data)
			if err != nil {
				return nil, fmt.Errorf("store: memory expiry %s: %w", src.SHA256, err)
			}
			expiries = append(expiries, struct {
				sha string
				pl  MemoryExpiryPayload
			}{src.SHA256, pl})
		}
	}
	for _, e := range expiries {
		target := e.pl.TargetSHA256
		if e.pl.OperationKey != "" {
			// Cancellation is durable even when no matching fact has arrived.
			// Choose a stable source pointer if independent histories merged.
			prior := p.canceled[e.pl.OperationKey]
			if prior == "" || e.sha < prior {
				p.canceled[e.pl.OperationKey] = e.sha
			}
			target = operations[e.pl.OperationKey]
		}
		rec, ok := p.bySHA[target]
		if !ok {
			continue // an expiry citing an unknown/private-to-another-brain target is inert
		}
		if rec.ExpiredAt == nil || e.pl.ExpiredAt.Before(*rec.ExpiredAt) {
			// The FIRST expiry event wins (deterministic under concurrent
			// forgets of the same fact -- writer.MemoryFact's own
			// idempotency check already prevents a second one from being
			// written in the common case; this is the read-side tiebreak
			// for the rare case two independent branches each wrote one
			// before merging, mapping item 6's "collision-check during
			// allocation AND projection after Git merges" read for expiry
			// events).
			at := e.pl.ExpiredAt
			rec.ExpiredAt = &at
			rec.ExpiredReason = e.pl.Reason
			rec.ExpirySHA256 = e.sha
		}
	}
	return p, nil
}

// Get returns the record for sha, if any memory_fact source exists under
// it.
func (p *MemoryProjection) Get(sha string) (MemoryFactRecord, bool) {
	rec, ok := p.bySHA[sha]
	if !ok {
		return MemoryFactRecord{}, false
	}
	return *rec, true
}

// All returns every memory_fact record, sorted by legacy id then SHA256 for
// a stable, deterministic order.
func (p *MemoryProjection) All() []MemoryFactRecord {
	out := make([]MemoryFactRecord, 0, len(p.bySHA))
	for _, rec := range p.bySHA {
		out = append(out, *rec)
	}
	sortMemoryFactRecords(out)
	return out
}

func sortMemoryFactRecords(out []MemoryFactRecord) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Payload.LegacyID != out[j].Payload.LegacyID {
			return out[i].Payload.LegacyID < out[j].Payload.LegacyID
		}
		return out[i].SHA256 < out[j].SHA256
	})
}

// ByEntity returns every record (active or expired -- callers filter) whose
// entity_slug matches slug, in the same deterministic order as All.
func (p *MemoryProjection) ByEntity(slug string) []MemoryFactRecord {
	out := make([]MemoryFactRecord, 0)
	for _, rec := range p.bySHA {
		if rec.Payload.EntitySlug == slug {
			out = append(out, *rec)
		}
	}
	sortMemoryFactRecords(out)
	return out
}

// ByLegacyID returns the record whose legacy_id matches id, if any --
// forget's own "id" wire value may be an opaque SHA256 or, for backward
// compatibility with the frozen legacy numeric id, that integer as a
// string (recall.facts[].id).
func (p *MemoryProjection) ByLegacyID(id int64) (MemoryFactRecord, bool) {
	for _, rec := range p.bySHA {
		if rec.Payload.LegacyID == id {
			return *rec, true
		}
	}
	return MemoryFactRecord{}, false
}

// NextLegacyID allocates a random JSON-safe identity, checking all canonical
// records including expired ones. Projection also rejects collisions after merges.
func (p *MemoryProjection) NextLegacyID() (int64, error) {
	for attempts := 0; attempts < 128; attempts++ {
		n, err := rand.Int(rand.Reader, big.NewInt(MaxMemoryLegacyID))
		if err != nil {
			return 0, fmt.Errorf("store: allocate memory identity: %w", err)
		}
		id := n.Int64() + 1
		if _, exists := p.ByLegacyID(id); !exists {
			return id, nil
		}
	}
	return 0, fmt.Errorf("store: memory identity collision retry limit")
}

// IsLifecycle identifies immutable expiry events, which are never evidence.
func (p *MemoryProjection) IsLifecycle(sha string) bool { return p.lifecycle[sha] }

// SourceIndexOnly preserves ordinary source egress policy in the same snapshot.
func (p *MemoryProjection) SourceIndexOnly(sha string) bool { return p.indexOnly[sha] }

// SourceKnown reports whether canonical source metadata exists. Native claims
// with dangling source attribution cannot authorize remote/provider disclosure.
func (p *MemoryProjection) SourceKnown(sha string) bool { _, ok := p.indexOnly[sha]; return ok }

// DedupKey is the exact-duplicate identity for a candidate remember call --
// same entity + kind + visibility + attribution + effective expiry + fact
// text (mapping item 6: "Define exact dedup key including attribution/
// entity/kind/visibility/effective expiry... do not collapse private and
// world or differing attribution/expiry silently"). validUntil is compared
// by its exact ISO value (nil/omitted TTL is its own key, distinct from any
// concrete expiry) so two facts that are identical except for one carrying
// a different (or no) TTL are never collapsed into one.
func DedupKey(fact, provenance, entitySlug string, kind MemoryFactKind, visibility MemoryVisibility, validUntil *time.Time, entityType ...string) string {
	until := ""
	if validUntil != nil {
		until = validUntil.UTC().Format(time.RFC3339Nano)
	}
	typ := ""
	if len(entityType) > 0 {
		typ = entityType[0]
	}
	data, _ := json.Marshal([]string{fact, provenance, entitySlug, string(kind), string(visibility), until, typ}) // strings are always JSON encodable
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// FindDuplicate returns the active (never expired) record among p's own
// facts whose dedup key exactly matches key, if any -- remember's own
// exact-duplicate check (mapping: "Exact dedup scans active same-scope fact
// record before allocating new ID").
func (p *MemoryProjection) FindDuplicate(key string, now time.Time) (MemoryFactRecord, bool) {
	for _, rec := range p.bySHA {
		if rec.Expired(now) {
			continue
		}
		k := DedupKey(rec.Payload.Fact, rec.Payload.Provenance, rec.Payload.EntitySlug, rec.Payload.Kind, rec.Payload.Visibility, rec.Payload.ValidUntil, rec.Payload.EntityType)
		if k == key {
			return *rec, true
		}
	}
	return MemoryFactRecord{}, false
}

// domain.Source helper constructors, kept here (not inline at every writer
// call site) so the Kind/OccurredAt convention for a memory-lifecycle
// source is defined exactly once.

// NewMemoryFactSource builds the domain.Source metadata (not yet SHA256'd --
// SourceStore.Write fills that in) for a memory_fact payload write.
func NewMemoryFactSource(occurredAt time.Time) domain.Source {
	return domain.Source{Kind: SourceKindMemoryFact, URI: "memory://fact", OccurredAt: occurredAt}
}

// NewMemoryExpirySource builds the domain.Source metadata for a
// memory_expiry payload write.
func NewMemoryExpirySource(occurredAt time.Time) domain.Source {
	return domain.Source{Kind: SourceKindMemoryExpiry, URI: "memory://expiry", OccurredAt: occurredAt}
}

// MemoryEligible is the shared read-time audience filter internal/search,
// internal/compose, and internal/server/memory's recall all apply before a
// chunk backed by sourceSHA256 is allowed to surface (mapping item 5's
// "canonical read filter is still required at query time to suppress
// stale expired/forgotten hits" plus coordinator refinement #4's local-
// vs-remote audience policy). sourceSHA256 not naming any memory_fact
// record (an ordinary source, or an entity-page chunk with no source at
// all) is always eligible -- this filter has nothing to say about it.
// remote=true is MCP's own audience (world-visibility only, RFC's "remote
// callers see visibility=world facts only"); remote=false is local CLI
// search/ask, which may see active private facts too.
func MemoryEligible(p *MemoryProjection, sourceSHA256 string, remote bool, now time.Time) bool {
	if p.IsLifecycle(sourceSHA256) {
		return false
	}
	rec, ok := p.Get(sourceSHA256)
	if !ok {
		return true
	}
	if rec.Expired(now) {
		return false
	}
	if remote && rec.Payload.Visibility != MemoryVisibilityWorld {
		return false
	}
	return true
}

// OperationCancellation returns the immutable cancellation source, if any.
// Absence of a fact does not discard a cancellation fence.
func (p *MemoryProjection) OperationCancellation(key string) (string, bool) {
	sha, ok := p.canceled[key]
	return sha, ok
}
