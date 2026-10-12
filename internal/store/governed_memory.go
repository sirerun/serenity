package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/domain"
)

// GovernedMemoryKind contains immutable attributed statements and their command
// receipts. They are not automatically accepted beliefs or extracted claims.
const GovernedMemoryKind = "governed_memory_command"
const GovernedMemorySchema = "serenity.memory/v1"

type GovernedArtifact struct {
	ID     string `json:"id"`
	Digest string `json:"digest"`
}
type GovernedClaim struct {
	ID         string             `json:"id"`
	Version    int64              `json:"version"`
	BrainID    string             `json:"brain_id"`
	Text       string             `json:"text"`
	Sources    []GovernedArtifact `json:"sources"`
	Confidence *int64             `json:"confidence,omitempty"`
	Freshness  time.Time          `json:"freshness"`
	Active     bool               `json:"active"`
}
type GovernedRevision struct {
	BrainID       string    `json:"brain_id"`
	Revision      string    `json:"revision"`
	Digest        string    `json:"digest"`
	ObservedAt    time.Time `json:"observed_at"`
	IndexRevision string    `json:"index_revision"`
}
type GovernedMoney struct {
	Currency   string `json:"currency"`
	MicroUnits int64  `json:"micro_units"`
}
type GovernedResponse struct {
	Schema              string             `json:"schema"`
	ProjectID           string             `json:"project_id"`
	BrainID             string             `json:"brain_id"`
	CommandID           string             `json:"adapter_command_id"`
	Kind                string             `json:"kind"`
	Status              string             `json:"command_status"`
	Claims              []GovernedClaim    `json:"claims"`
	Revisions           []GovernedRevision `json:"brain_revisions"`
	Cost                GovernedMoney      `json:"cost"`
	Destinations        []string           `json:"provider_destinations"`
	WriterOwner         string             `json:"writer_owner,omitempty"`
	LookupAuthoritative bool               `json:"lookup_authoritative"`
	ActiveRecallRemoved *bool              `json:"active_recall_removed,omitempty"`
	HistoricalErasure   *bool              `json:"historical_erasure,omitempty"`
}
type GovernedRecord struct {
	Schema        string           `json:"schema"`
	RequestDigest string           `json:"request_digest"`
	Response      GovernedResponse `json:"response"`
	RecordedAt    time.Time        `json:"recorded_at"`
	// The request preserves attribution, requested removal reason and bounds.
	Request json.RawMessage `json:"request"`
}
type GovernedProjection struct {
	SourceIDs   map[string]string
	WriterOwner string
	Origins     map[string]string
	LegacyIDs   map[string]int64
	BrainID     string
	Commands    map[string]GovernedRecord
	Claims      map[string]GovernedClaim
	Revision    string
	ObservedAt  time.Time
}

func ValidGovernedUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
func ValidGovernedDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil && strings.ToLower(s) == s
}

// LoadGovernedMemory verifies canonical source bytes and reconstructs command
// identity and latest claim versions. An absent command remains unknown, never
// proof that a timed-out request was not admitted.
func LoadGovernedMemory(s *SourceStore) (*GovernedProjection, error) {
	p := &GovernedProjection{Commands: map[string]GovernedRecord{}, Claims: map[string]GovernedClaim{}, Origins: map[string]string{}, SourceIDs: map[string]string{}, LegacyIDs: map[string]int64{}}
	entries, err := s.All()
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, meta := range entries {
		if meta.Kind != GovernedMemoryKind {
			continue
		}
		raw, _, err := s.Read(meta.SHA256)
		if err != nil {
			return nil, err
		}
		var rec GovernedRecord
		if err = json.Unmarshal(raw, &rec); err != nil {
			return nil, fmt.Errorf("decode governed memory: %w", err)
		}
		r := rec.Response
		if rec.Schema != GovernedMemorySchema || r.Schema != GovernedMemorySchema || !ValidGovernedUUID(r.BrainID) || !ValidGovernedUUID(r.CommandID) || !ValidGovernedDigest(rec.RequestDigest) || rec.RecordedAt.IsZero() || r.Status != "completed" {
			return nil, fmt.Errorf("invalid governed memory record")
		}
		sum := sha256.Sum256(rec.Request)
		if hex.EncodeToString(sum[:]) != rec.RequestDigest {
			return nil, fmt.Errorf("governed request digest mismatch")
		}
		if p.BrainID != "" && p.BrainID != r.BrainID {
			return nil, fmt.Errorf("conflicting governed brain binding")
		}
		p.BrainID = r.BrainID
		if _, ok := p.Commands[r.CommandID]; ok {
			return nil, fmt.Errorf("duplicate governed command identity")
		}
		p.Commands[r.CommandID] = rec
		p.SourceIDs[r.CommandID] = meta.SHA256
		if r.WriterOwner != "" {
			if p.WriterOwner != "" && p.WriterOwner != r.WriterOwner {
				return nil, fmt.Errorf("conflicting governed writer owner")
			}
			p.WriterOwner = r.WriterOwner
		}

		if r.Kind == "remember" && len(r.Claims) == 1 {
			c := r.Claims[0]
			if !ValidGovernedUUID(c.ID) {
				return nil, fmt.Errorf("invalid governed claim identity")
			}
			legacy, err := strconv.ParseInt(strings.ReplaceAll(c.ID, "-", "")[:13], 16, 64)
			if err != nil {
				return nil, err
			}
			p.Origins[c.ID] = meta.SHA256
			p.LegacyIDs[c.ID] = legacy + 1
		}
		for _, c := range r.Claims {
			if !ValidGovernedUUID(c.ID) || c.BrainID != r.BrainID || c.Version < 1 || c.Freshness.IsZero() {
				return nil, fmt.Errorf("invalid governed claim")
			}
			previous, ok := p.Claims[c.ID]
			if !ok || c.Version > previous.Version {
				p.Claims[c.ID] = c
			} else if c.Version == previous.Version {
				a, _ := json.Marshal(previous)
				b, _ := json.Marshal(c)
				if string(a) != string(b) {
					return nil, fmt.Errorf("conflicting governed claim version")
				}
			}
		}
		ids = append(ids, meta.SHA256)
		if rec.RecordedAt.After(p.ObservedAt) {
			p.ObservedAt = rec.RecordedAt
		}
	}
	sort.Strings(ids)
	sum := sha256.Sum256([]byte(strings.Join(ids, "\n")))
	p.Revision = hex.EncodeToString(sum[:])
	return p, nil
}

// PutGovernedMemory is reserved for the serialized governed writer.
func (s *SourceStore) PutGovernedMemory(rec GovernedRecord) (domain.Source, error) {
	raw, err := json.Marshal(rec)
	if err != nil {
		return domain.Source{}, err
	}
	return s.writeSource(raw, domain.Source{Kind: GovernedMemoryKind, OccurredAt: rec.RecordedAt, URI: "memory://governed"})
}

// AddGovernedProjection exposes attributed governed memories through existing
// MCP/dashboard reads without converting them to accepted domain beliefs.
func AddGovernedProjection(s *SourceStore, p *MemoryProjection, legacy map[int64]string) error {
	g, err := LoadGovernedMemory(s)
	if err != nil {
		return err
	}
	for id, c := range g.Claims {
		sha := g.Origins[id]
		if sha == "" {
			return fmt.Errorf("governed claim has no origin")
		}
		n := g.LegacyIDs[id]
		if other, ok := legacy[n]; ok && other != sha {
			return fmt.Errorf("governed legacy identity collision")
		}
		legacy[n] = sha
		provenance, _ := json.Marshal(c.Sources)
		rec := &MemoryFactRecord{SHA256: sha, Payload: MemoryFactPayload{FormatVersion: 1, RecordType: SourceKindMemoryFact, LegacyID: n, Fact: c.Text, Provenance: "governed:" + id + " version:" + strconv.FormatInt(c.Version, 10) + " sources:" + string(provenance), Kind: MemoryFactKindFact, Visibility: MemoryVisibilityWorld, CreatedAt: c.Freshness}}
		if !c.Active {
			at := c.Freshness
			rec.ExpiredAt = &at
			rec.ExpiredReason = "governed retraction"
		}
		p.bySHA[sha] = rec
	}
	return nil
}
