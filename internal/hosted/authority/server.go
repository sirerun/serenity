package authority

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/deletion"
)

// ServerConfig is owner-controlled deployment configuration. It is never
// populated from an ordinary runtime request or default environment discovery.
type ServerConfig struct {
	RuntimePrincipal  string
	OwnerPrincipal    string
	ApprovedBuilds    map[string]string // measured executable SHA256 -> source revision
	LeaseLifetime     time.Duration
	ChallengeLifetime time.Duration
	GenesisLifetime   time.Duration
	Now               func() time.Time
}

type Server struct {
	kernel *Kernel
	config ServerConfig
}

func NewServer(k *Kernel, c ServerConfig) (*Server, error) {
	if k == nil || k.validate() != nil || c.RuntimePrincipal == "" || c.OwnerPrincipal == "" || c.RuntimePrincipal == c.OwnerPrincipal || len(c.ApprovedBuilds) == 0 || c.LeaseLifetime < time.Minute || c.LeaseLifetime > time.Hour || c.ChallengeLifetime <= 0 || c.ChallengeLifetime > 5*time.Minute || c.GenesisLifetime <= 0 || c.GenesisLifetime > 24*time.Hour {
		return nil, ErrInvalid
	}
	for digest, sha := range c.ApprovedBuilds {
		if !validID(digest) || len(sha) != 40 {
			return nil, ErrInvalid
		}
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	copyBuilds := make(map[string]string, len(c.ApprovedBuilds))
	for k, v := range c.ApprovedBuilds {
		copyBuilds[k] = v
	}
	c.ApprovedBuilds = copyBuilds
	return &Server{kernel: k, config: c}, nil
}

type protocolState struct {
	Adoption       *Adoption                 `json:"adoption,omitempty"`
	Initialized    bool                      `json:"initialized"`
	Phase          string                    `json:"phase"`
	Epoch          uint64                    `json:"epoch"`
	Position       contracts.JournalPosition `json:"position"`
	GenesisID      string                    `json:"genesis_id"`
	GenesisExpires time.Time                 `json:"genesis_expires"`
	Prepared       *Prepared                 `json:"prepared,omitempty"`
	Lease          *Lease                    `json:"lease,omitempty"`
	Measurement    Measurement               `json:"measurement"`
}

type issuedChallenge struct {
	Challenge Challenge `json:"challenge"`
	Principal string    `json:"principal"`
}
type issuance struct {
	WriterID      string `json:"writer_id"`
	Epoch         uint64 `json:"epoch"`
	Generation    int64  `json:"generation"`
	BuildSHA      string `json:"build_sha"`
	Principal     string `json:"principal"`
	ReservationID string `json:"reservation_id"`
}

func (s *Server) execute(ctx context.Context, principal string, q Request) (Response, error) {
	if principal != s.config.RuntimePrincipal && principal != s.config.OwnerPrincipal {
		return Response{}, ErrForbidden
	}
	if err := validateRequest(q); err != nil {
		return Response{}, err
	}
	owner := principal == s.config.OwnerPrincipal
	if (q.Action == "enroll" || q.Action == "restore" || q.Action == "inspect") && !owner || q.Action == "seal" && !owner {
		return Response{}, ErrForbidden
	}
	if owner && q.Action != "inspect" && q.Action != "enroll" && q.Action != "restore" && q.Action != "seal" && q.Action != "read_finalized_cut" && q.Action != "get" && q.Action != "list" && q.Action != "current" && q.Action != "close" {
		return Response{}, ErrForbidden
	}
	raw, err := json.Marshal(q)
	if err != nil {
		return Response{}, err
	}
	requestHash := digest(raw)
	if q.OperationID != "" {
		receipt, found, e := s.kernel.Store.Receipt(ctx, q.OperationID)
		if e != nil {
			return Response{}, e
		}
		if found {
			if receipt.Principal != principal || receipt.RequestSHA256 != requestHash {
				return Response{}, ErrConflict
			}
			if !receipt.Complete {
				r, ok, e := s.kernel.Store.Load(ctx)
				if e != nil || !ok {
					return Response{}, ErrUnavailable
				}
				if _, e = s.kernel.Resolve(ctx, r); e != nil {
					return Response{}, e
				}
				receipt, found, e = s.kernel.Store.Receipt(ctx, q.OperationID)
				if e != nil || !found || !receipt.Complete {
					return Response{}, ErrUnavailable
				}
			}
			if e := s.revalidateResult(ctx, q, receipt.Response); e != nil {
				return Response{}, e
			}
			return receipt.Response, nil
		}
	}
	if q.Action == "restore" {
		return s.restore(ctx, principal, q, requestHash)
	}
	if q.Action == "enroll" {
		return s.enroll(ctx, principal, q, requestHash)
	}
	rec, err := s.kernel.Current(ctx)
	if err != nil {
		return Response{}, err
	}
	var state protocolState
	if err = strictDecode(rec.State, &state); err != nil {
		return Response{}, err
	}
	now := s.config.Now().UTC()
	apply := func(next protocolState, items []ImmutableItem, out Response) (Response, error) {
		b, e := json.Marshal(next)
		if e != nil {
			return Response{}, e
		}
		result, e := s.kernel.Apply(ctx, rec, b, items, Receipt{OperationID: q.OperationID, RequestSHA256: requestHash, Principal: principal, Response: out}, false)
		if e != nil {
			return Response{}, e
		}
		if e = s.revalidateResult(ctx, q, result); e != nil {
			return Response{}, e
		}
		return result, nil
	}
	switch q.Action {
	case "inspect":
		out := Response{Adoption: state.Adoption, Prepared: state.Prepared, Lease: state.Lease}
		out.Current = &Current{Position: state.Position, Epoch: state.Epoch}
		if state.Lease != nil {
			out.Current.ExpiresAt = state.Lease.ExpiresAt
		} else if state.Prepared != nil {
			out.Current.ExpiresAt = state.Prepared.ExpiresAt
		}
		return out, nil
	case "challenge":
		if state.Phase == "PREPARING" {
			return Response{}, ErrConflict
		}
		id, e := NewOperationID()
		if e != nil {
			return Response{}, e
		}
		nonce, e := NewOperationID()
		if e != nil {
			return Response{}, e
		}
		op, e := NewOperationID()
		if e != nil {
			return Response{}, e
		}
		ch := Challenge{ID: id, Nonce: nonce, OperationID: op, ExpiresAt: now.Add(s.config.ChallengeLifetime)}
		b, e := json.Marshal(issuedChallenge{Challenge: ch, Principal: principal})
		if e != nil {
			return Response{}, e
		}
		return apply(state, []ImmutableItem{{Key: "ATT#" + id, Body: b}}, Response{Challenge: &ch})
	case "prepare":
		if state.Phase == "PREPARING" {
			return Response{}, ErrConflict
		}
		if q.Measurement == nil || q.Measurement.ProcessLifetime == "" || len(q.Measurement.ProcessLifetime) > 256 {
			return Response{}, ErrForbidden
		}
		build, ok := s.config.ApprovedBuilds[q.Measurement.ExecutableSHA256]
		if !ok {
			return Response{}, ErrForbidden
		}
		b, ok, e := s.kernel.Store.Get(ctx, "ATT#"+q.ChallengeID)
		if e != nil {
			return Response{}, e
		}
		if !ok {
			return Response{}, ErrForbidden
		}
		var att issuedChallenge
		if e = strictDecode(b, &att); e != nil {
			return Response{}, e
		}
		if att.Principal != principal || att.Challenge.Nonce != q.Nonce || att.Challenge.OperationID != q.OperationID {
			return Response{}, ErrForbidden
		}
		if !now.Before(att.Challenge.ExpiresAt) {
			return Response{}, ErrExpired
		}
		reservation, e := NewOperationID()
		if e != nil {
			return Response{}, e
		}
		writer, e := NewOperationID()
		if e != nil {
			return Response{}, e
		}
		state.Epoch++
		p := Prepared{RequireEmptyData: !state.Initialized, ReservationID: reservation, WriterID: writer, Epoch: state.Epoch, Position: state.Position, ExpiresAt: now.Add(s.config.LeaseLifetime), BuildSHA: build}
		state.Prepared = &p
		state.Lease = nil
		state.Measurement = *q.Measurement
		items := []ImmutableItem{{Key: "CONSUMED#" + q.ChallengeID, Body: []byte(requestHash)}}
		if state.Phase == "ENROLLED" {
			if !now.Before(state.GenesisExpires) {
				return Response{}, ErrExpired
			}
			items = append(items, ImmutableItem{Key: "GENESIS-CONSUMED#" + state.GenesisID, Body: []byte(requestHash)})
		}
		if !state.Initialized {
			p.Adoption = state.Adoption
		}
		state.Phase = "PREPARING"
		return apply(state, items, Response{Prepared: &p})
	case "activate":
		if state.Phase != "PREPARING" || state.Prepared == nil || state.Prepared.ReservationID != q.ReservationID {
			return Response{}, ErrConflict
		}
		if !now.Before(state.Prepared.ExpiresAt) {
			return Response{}, ErrExpired
		}
		if !state.Initialized && state.Adoption != nil {
			if q.DataInventorySHA256 != state.Adoption.InventorySHA256 {
				return Response{}, ErrForbidden
			}
		} else if q.DataInventorySHA256 != "" {
			return Response{}, ErrInvalid
		}
		reader, e := deletion.NewJournalReader(&boundedReader{store: s.kernel.Store, position: state.Position, strict: true}, state.Position)
		if e != nil {
			return Response{}, e
		}
		observed, e := reader.Observe(ctx)
		if e != nil {
			return Response{}, e
		}
		if observed.HistorySHA256 != q.HistorySHA256 {
			return Response{}, ErrConflict
		}
		for _, use := range observed.WriterUses {
			b, found, e := s.kernel.Store.Get(ctx, "ISS#"+use.WriterID)
			if e != nil {
				return Response{}, e
			}
			if !found {
				return Response{}, ErrForbidden
			}
			var issuance issuance
			if e = strictDecode(b, &issuance); e != nil {
				return Response{}, e
			}
			if issuance.WriterID != use.WriterID || issuance.Generation != use.Generation || issuance.Principal != s.config.RuntimePrincipal && issuance.Principal != s.config.OwnerPrincipal {
				return Response{}, ErrForbidden
			}
		}
		id, e := NewOperationID()
		if e != nil {
			return Response{}, e
		}
		p := state.Prepared
		l := Lease{ID: id, ReservationID: p.ReservationID, WriterID: p.WriterID, Epoch: p.Epoch, Position: p.Position, ExpiresAt: p.ExpiresAt, BuildSHA: p.BuildSHA}
		state.Initialized = true
		state.Phase = "ACTIVE"
		state.Lease = &l
		b, e := json.Marshal(issuance{WriterID: p.WriterID, Epoch: p.Epoch, Generation: p.Position.ActiveGeneration, BuildSHA: p.BuildSHA, Principal: principal, ReservationID: p.ReservationID})
		if e != nil {
			return Response{}, e
		}
		return apply(state, []ImmutableItem{{Key: "ISS#" + p.WriterID, Body: b}}, Response{Lease: &l})
	case "current":
		if e := s.checkLease(state, q.LeaseID, now); e != nil {
			return Response{}, e
		}
		return Response{Current: &Current{Position: state.Position, Epoch: state.Epoch, ExpiresAt: state.Lease.ExpiresAt}}, nil
	case "renew":
		if q.Measurement == nil || *q.Measurement != state.Measurement {
			return Response{}, ErrForbidden
		}
		if e := s.checkLease(state, q.LeaseID, now); e != nil {
			return Response{}, e
		}
		l := *state.Lease
		l.ExpiresAt = now.Add(s.config.LeaseLifetime)
		if !l.ExpiresAt.After(state.Lease.ExpiresAt) {
			return Response{}, ErrConflict
		}
		l.Position = state.Position
		state.Lease = &l
		return apply(state, nil, Response{Lease: &l})
	case "close":
		if q.LeaseID != "" {
			if state.Lease == nil || state.Lease.ID != q.LeaseID {
				return Response{}, ErrConflict
			}
		} else if state.Prepared == nil || state.Prepared.ReservationID != q.ReservationID {
			return Response{}, ErrConflict
		}
		state.Phase = "CLOSED"
		state.Lease = nil
		state.Prepared = nil
		return apply(state, nil, Response{})
	case "append", "seal":
		if e := s.checkLease(state, q.LeaseID, now); e != nil {
			return Response{}, e
		}
		next, e := validateAppend(state, q, owner, now)
		if e != nil {
			return Response{}, e
		}
		return apply(next, []ImmutableItem{{Key: "OBJ#" + q.Key, Body: q.Body}}, Response{Created: true})
	case "get", "list":
		pos, strict, e := s.readPosition(ctx, state, q, owner, now)
		if e != nil {
			return Response{}, e
		}
		reader := boundedReader{store: s.kernel.Store, position: pos, strict: strict}
		if q.Action == "get" {
			b, found, e := reader.Get(ctx, q.Key)
			return Response{Body: b, Found: found}, e
		}
		return reader.listPage(ctx, q.Prefix, q.StartAfter, q.Limit)
	case "cut":
		if e := s.checkLease(state, q.LeaseID, now); e != nil {
			return Response{}, e
		}
		id, e := NewOperationID()
		if e != nil {
			return Response{}, e
		}
		cut := Cut{ID: id, LeaseID: q.LeaseID, Position: state.Position, SnapshotAttemptID: q.SnapshotAttemptID, ExpiresAt: now.Add(s.config.LeaseLifetime), Anchor: rec.Tip}
		b, e := json.Marshal(cut)
		if e != nil {
			return Response{}, e
		}
		return apply(state, []ImmutableItem{{Key: "CUT#" + id, Body: b}}, Response{Cut: &cut})
	case "finalize_cut":
		cut, e := s.loadCut(ctx, q.CutID)
		if e != nil {
			return Response{}, e
		}
		if e = s.checkLease(state, cut.LeaseID, now); e != nil {
			return Response{}, e
		}
		if !now.Before(cut.ExpiresAt) {
			return Response{}, ErrExpired
		}
		final := FinalizedCut{Cut: cut, ManifestSHA256: q.ManifestSHA256, OperationID: q.OperationID}
		b, e := json.Marshal(final)
		if e != nil {
			return Response{}, e
		}
		return apply(state, []ImmutableItem{{Key: "FINAL#" + cut.ID, Body: b}}, Response{FinalizedCut: &final})
	case "read_finalized_cut":
		b, found, e := s.kernel.Store.Get(ctx, "FINAL#"+q.CutID)
		if e != nil {
			return Response{}, e
		}
		if !found {
			return Response{}, ErrInvalid
		}
		var f FinalizedCut
		if e = strictDecode(b, &f); e != nil {
			return Response{}, e
		}
		if !owner {
			if e = s.checkLease(state, f.Cut.LeaseID, now); e != nil {
				return Response{}, e
			}
			if !now.Before(f.Cut.ExpiresAt) {
				return Response{}, ErrExpired
			}
		}
		return Response{FinalizedCut: &f}, nil
	}
	return Response{}, ErrInvalid
}

func (s *Server) enroll(ctx context.Context, principal string, q Request, hash string) (Response, error) {
	r, found, err := s.kernel.Store.Load(ctx)
	if err != nil {
		return Response{}, err
	}
	if found || r.Revision != 0 {
		return Response{}, ErrConflict
	}
	id, err := NewOperationID()
	if err != nil {
		return Response{}, err
	}
	state := protocolState{Adoption: q.Adoption, Phase: "ENROLLED", GenesisID: id, GenesisExpires: s.config.Now().UTC().Add(s.config.GenesisLifetime), Position: contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: id}}
	b, err := json.Marshal(state)
	if err != nil {
		return Response{}, err
	}
	permit, err := json.Marshal(struct {
		ID, Lineage, TableARN, Issuer, Audience, Nonce, PayloadSHA256, OperationID string
		ExpiresAt                                                                  time.Time
	}{id, s.kernel.Lineage, s.kernel.TableARN, principal, s.config.RuntimePrincipal, id, hash, q.OperationID, state.GenesisExpires})
	if err != nil {
		return Response{}, err
	}
	items := []ImmutableItem{{Key: "GENESIS#" + id, Body: permit}}
	if q.Adoption != nil {
		adoption, e := json.Marshal(q.Adoption)
		if e != nil {
			return Response{}, e
		}
		items = append(items, ImmutableItem{Key: "ADOPTION#" + digest(adoption), Body: adoption})
	}
	return s.kernel.Apply(ctx, Record{}, b, items, Receipt{OperationID: q.OperationID, RequestSHA256: hash, Principal: principal}, true)
}
func (s *Server) checkLease(st protocolState, id string, now time.Time) error {
	if st.Phase != "ACTIVE" || st.Lease == nil || st.Lease.ID != id || st.Lease.Epoch != st.Epoch {
		return ErrConflict
	}
	if !now.Before(st.Lease.ExpiresAt) {
		return ErrExpired
	}
	if s.config.ApprovedBuilds[st.Measurement.ExecutableSHA256] != st.Lease.BuildSHA {
		return ErrForbidden
	}
	return nil
}
func (s *Server) loadCut(ctx context.Context, id string) (Cut, error) {
	b, ok, e := s.kernel.Store.Get(ctx, "CUT#"+id)
	if e != nil {
		return Cut{}, e
	}
	if !ok {
		return Cut{}, ErrInvalid
	}
	var cut Cut
	e = strictDecode(b, &cut)
	return cut, e
}
func (s *Server) readPosition(ctx context.Context, st protocolState, q Request, owner bool, now time.Time) (contracts.JournalPosition, bool, error) {
	if q.CutID != "" {
		cut, e := s.loadCut(ctx, q.CutID)
		if e != nil {
			return contracts.JournalPosition{}, false, e
		}
		if !owner {
			if e = s.checkLease(st, cut.LeaseID, now); e != nil {
				return contracts.JournalPosition{}, false, e
			}
			if !now.Before(cut.ExpiresAt) {
				return contracts.JournalPosition{}, false, ErrExpired
			}
		}
		return cut.Position, false, nil
	}
	if q.ReservationID != "" {
		if st.Prepared == nil || st.Prepared.ReservationID != q.ReservationID || st.Phase != "PREPARING" && st.Phase != "ACTIVE" {
			return contracts.JournalPosition{}, false, ErrConflict
		}
		if st.Phase == "PREPARING" && !now.Before(st.Prepared.ExpiresAt) {
			return contracts.JournalPosition{}, false, ErrExpired
		}
		if st.Phase == "ACTIVE" {
			if e := s.checkLease(st, st.Lease.ID, now); e != nil {
				return contracts.JournalPosition{}, false, e
			}
		}
		return st.Prepared.Position, st.Phase == "PREPARING", nil
	}
	if e := s.checkLease(st, q.LeaseID, now); e != nil {
		return contracts.JournalPosition{}, false, e
	}
	return st.Position, true, nil
}
func validateAppend(st protocolState, q Request, owner bool, now time.Time) (protocolState, error) {
	var o contracts.JournalObject
	if len(q.Body) == 0 || len(q.Body) > MaxObjectBytes || strictDecode(q.Body, &o) != nil {
		return st, ErrInvalid
	}
	canonical, err := o.Encode()
	if err != nil || !bytes.Equal(canonical, q.Body) {
		return st, ErrInvalid
	}
	pos := st.Position
	next := pos.LastObjectInGeneration.SequenceID + 1
	prev := pos.LastObjectInGeneration.EntryHash
	if next == 1 {
		prev = pos.PredecessorSeal.EntryHash
	}
	if o.Generation != pos.ActiveGeneration || o.SequenceID != next || next > 9999999999 || o.PrevHash != prev || o.WriterID != st.Lease.WriterID || q.Key != contracts.JournalKey(o.Generation, o.SequenceID) || o.RecordedAt.IsZero() || o.RecordedAt.After(now.Add(time.Minute)) {
		return st, ErrConflict
	}
	switch o.Kind {
	case contracts.JournalKindEntry:
		if q.Action != "append" || o.SubjectID == "" || len(o.SubjectID) > 4096 {
			return st, ErrInvalid
		}
		if o.SubjectType == contracts.DeletionSubjectEpisodeConversation {
			entry := contracts.DeletionEntry{SubjectType: o.SubjectType, SubjectID: o.SubjectID, Outcome: o.Outcome}
			if entry.Validate() != nil {
				return st, ErrInvalid
			}
		} else if o.SubjectType != contracts.DeletionSubjectAccount && o.SubjectType != contracts.DeletionSubjectBrain || o.Outcome != contracts.DeletionIntentRequested && o.Outcome != contracts.DeletionOutcomePurged {
			// Preserve the established legacy account/brain acceptance boundary.
			return st, ErrInvalid
		}
	case contracts.JournalKindSeal:
		if !owner || q.Action != "seal" || o.SubjectType != "" || o.SubjectID != "" || o.Outcome != "" {
			return st, ErrForbidden
		}
	default:
		return st, ErrInvalid
	}
	mark := contracts.DeletionWatermark{Generation: o.Generation, SequenceID: o.SequenceID, EntryHash: contracts.HashObject(q.Body)}
	st.Position.LastObjectInGeneration = mark
	if o.Kind == contracts.JournalKindSeal {
		if st.Position.ActiveGeneration >= 10000 {
			return st, ErrInvalid
		}
		id, e := NewOperationID()
		if e != nil {
			return st, e
		}
		st.Position.ActiveGeneration++
		st.Position.LastObjectInGeneration = contracts.DeletionWatermark{}
		st.Position.PredecessorSeal = mark
		st.Position.SuccessorAllocationID = id
		st.Phase = "CLOSED"
		st.Lease = nil
		st.Prepared = nil
	}
	return st, nil
}

func validateRequest(q Request) error {
	allowed := map[string]string{"inspect": "", "enroll": "operation_id adoption", "restore": "operation_id", "challenge": "operation_id", "prepare": "operation_id challenge_id nonce measurement", "activate": "operation_id reservation_id history_sha256 data_inventory_sha256", "current": "lease_id", "renew": "operation_id lease_id measurement", "close": "operation_id lease_id reservation_id", "append": "operation_id lease_id key body", "seal": "operation_id lease_id key body", "get": "lease_id reservation_id cut_id key", "list": "lease_id reservation_id cut_id prefix start_after limit", "cut": "operation_id lease_id snapshot_attempt_id", "finalize_cut": "operation_id cut_id manifest_sha256", "read_finalized_cut": "cut_id"}
	fields, ok := allowed[q.Action]
	if !ok {
		return ErrInvalid
	}
	b, e := json.Marshal(q)
	if e != nil || len(b) > MaxBodyBytes {
		return ErrInvalid
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil {
		return ErrInvalid
	}
	for key := range m {
		if key != "action" && !strings.Contains(" "+fields+" ", " "+key+" ") {
			return ErrInvalid
		}
	}
	for _, id := range []string{q.OperationID, q.ChallengeID, q.Nonce, q.ReservationID, q.LeaseID, q.CutID, q.HistorySHA256, q.ManifestSHA256, q.SnapshotAttemptID, q.DataInventorySHA256} {
		if id != "" && !validID(id) {
			return ErrInvalid
		}
	}
	if q.Adoption != nil {
		if err := q.Adoption.Validate(); err != nil {
			return err
		}
	}
	if strings.Contains(fields, "operation_id") && q.OperationID == "" {
		return ErrInvalid
	}
	if q.Action == "get" || q.Action == "list" {
		n := 0
		for _, id := range []string{q.LeaseID, q.ReservationID, q.CutID} {
			if id != "" {
				n++
			}
		}
		if n != 1 {
			return ErrInvalid
		}
	}
	if q.Action == "close" && (q.LeaseID == "") == (q.ReservationID == "") {
		return ErrInvalid
	}
	if q.Action == "cut" && q.SnapshotAttemptID == "" || q.Action == "finalize_cut" && (q.ManifestSHA256 == "" || q.CutID == "") {
		return ErrInvalid
	}
	return nil
}

// Re-read immediately before acknowledging a mutation or durable retry. A
// historical receipt is not authority for effects under a superseded lease.
func (s *Server) revalidateResult(ctx context.Context, q Request, out Response) error {
	rec, err := s.kernel.Current(ctx)
	if err != nil {
		return err
	}
	var st protocolState
	if err = strictDecode(rec.State, &st); err != nil {
		return err
	}
	now := s.config.Now().UTC()
	switch q.Action {
	case "append", "renew", "cut":
		return s.checkLease(st, q.LeaseID, now)
	case "activate":
		if out.Lease == nil {
			return ErrUnavailable
		}
		return s.checkLease(st, out.Lease.ID, now)
	case "prepare":
		if out.Prepared == nil || st.Prepared == nil || out.Prepared.ReservationID != st.Prepared.ReservationID || st.Phase != "PREPARING" && st.Phase != "ACTIVE" {
			return ErrConflict
		}
		if !now.Before(st.Prepared.ExpiresAt) {
			return ErrExpired
		}
	case "finalize_cut":
		if out.FinalizedCut == nil {
			return ErrUnavailable
		}
		return s.checkLease(st, out.FinalizedCut.Cut.LeaseID, now)
	}
	return nil
}
