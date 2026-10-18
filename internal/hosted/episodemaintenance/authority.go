// Package episodemaintenance obtains fresh authoritative, bounded deletion
// history for the existing hosted maintenance owner. It never activates storage.
package episodemaintenance

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/hosted/authority"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/deletion"
)

var ErrAuthorityUnavailable = errors.New("episode maintenance: fresh authoritative history unavailable or changed")

type Source interface {
	Observe(context.Context) (Snapshot, error)
}
type Snapshot struct {
	Observation contracts.JournalObservation
	CutID       string
	Recheck     func(context.Context) error
}

// Authority is supplied only by the trusted existing factory. Its transport
// authenticates namespace and principal; ordinary callers cannot supply cuts.
type Authority interface {
	Cut(context.Context, authority.Lease, string, string) (authority.Cut, error)
	Current(context.Context, authority.Lease) (authority.Current, error)
	CutReader(authority.Cut) contracts.JournalObjectReader
}

type source struct {
	authority Authority
	lease     func() authority.Lease
	now       func() time.Time
}

func NewSource(a Authority, lease func() authority.Lease) (Source, error) {
	if nilCapability(a) || lease == nil {
		return nil, ErrAuthorityUnavailable
	}
	return &source{authority: a, lease: lease, now: time.Now}, nil
}

func (s *source) Observe(ctx context.Context) (Snapshot, error) {
	if ctx == nil || ctx.Err() != nil {
		return Snapshot{}, ErrAuthorityUnavailable
	}
	lease := s.lease()
	if !validLease(lease, s.now()) {
		return Snapshot{}, ErrAuthorityUnavailable
	}
	attempt, err := authority.NewOperationID()
	if err != nil {
		return Snapshot{}, ErrAuthorityUnavailable
	}
	operation, err := authority.NewOperationID()
	if err != nil {
		return Snapshot{}, ErrAuthorityUnavailable
	}
	cut, err := s.authority.Cut(ctx, lease, attempt, operation)
	if err != nil || !validCut(cut, lease, attempt, s.now()) {
		return Snapshot{}, ErrAuthorityUnavailable
	}
	reader, err := deletion.NewJournalReader(s.authority.CutReader(cut), cut.Position)
	if err != nil {
		return Snapshot{}, ErrAuthorityUnavailable
	}
	observed, err := reader.Observe(ctx)
	if err != nil || observed.Position != cut.Position {
		return Snapshot{}, ErrAuthorityUnavailable
	}
	recheck := func(checkCtx context.Context) error {
		if checkCtx == nil || checkCtx.Err() != nil || !cut.ExpiresAt.After(s.now()) {
			return ErrAuthorityUnavailable
		}
		latest := s.lease()
		if !validLease(latest, s.now()) || !sameIssuance(lease, latest) {
			return ErrAuthorityUnavailable
		}
		current, err := s.authority.Current(checkCtx, latest)
		if err != nil || current.Epoch != lease.Epoch || current.Position != cut.Position || !current.ExpiresAt.After(s.now()) || !current.ExpiresAt.Equal(latest.ExpiresAt) {
			return ErrAuthorityUnavailable
		}
		// Close renewal/replacement races in the local lease snapshot as well as
		// remote position drift. Unknown outcomes never return a usable boundary.
		after := s.lease()
		if !sameIssuance(latest, after) || !after.ExpiresAt.Equal(latest.ExpiresAt) || !validLease(after, s.now()) || !cut.ExpiresAt.After(s.now()) {
			return ErrAuthorityUnavailable
		}
		return checkCtx.Err()
	}
	if err = recheck(ctx); err != nil {
		return Snapshot{}, ErrAuthorityUnavailable
	}
	return Snapshot{Observation: observed, CutID: cut.ID, Recheck: recheck}, nil
}

func nilCapability(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan:
		return r.IsNil()
	}
	return false
}
func hexID(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}
func validLease(l authority.Lease, now time.Time) bool {
	return hexID(l.ID, 64) && hexID(l.ReservationID, 64) && hexID(l.WriterID, 64) && hexID(l.BuildSHA, 40) && l.Epoch > 0 && l.ExpiresAt.After(now) && l.Position.ActiveGeneration > 0 && l.Position.ActiveGeneration <= 10000 && positionIDs(l.Position)
}
func positionIDs(p contracts.JournalPosition) bool {
	if p.ActiveGeneration < 1 || p.ActiveGeneration > 10000 {
		return false
	}
	if !p.LastObjectInGeneration.IsZero() && (p.LastObjectInGeneration.Generation != p.ActiveGeneration || p.LastObjectInGeneration.SequenceID < 1 || p.LastObjectInGeneration.SequenceID > 9999999999 || !hexID(p.LastObjectInGeneration.EntryHash, 64)) {
		return false
	}
	if p.ActiveGeneration == 1 {
		return hexID(p.GenesisIssuanceID, 64) && p.SuccessorAllocationID == "" && p.PredecessorSeal.IsZero()
	}
	return p.GenesisIssuanceID == "" && hexID(p.SuccessorAllocationID, 64) && p.PredecessorSeal.Generation == p.ActiveGeneration-1 && p.PredecessorSeal.SequenceID > 0 && hexID(p.PredecessorSeal.EntryHash, 64)
}
func sameIssuance(a, b authority.Lease) bool {
	return a.ID == b.ID && a.ReservationID == b.ReservationID && a.WriterID == b.WriterID && a.Epoch == b.Epoch && a.BuildSHA == b.BuildSHA && sameGeneration(a.Position, b.Position)
}
func sameGeneration(a, b contracts.JournalPosition) bool {
	return a.ActiveGeneration == b.ActiveGeneration && a.GenesisIssuanceID == b.GenesisIssuanceID && a.SuccessorAllocationID == b.SuccessorAllocationID && a.PredecessorSeal == b.PredecessorSeal
}
func validCut(c authority.Cut, l authority.Lease, attempt string, now time.Time) bool {
	a := c.Anchor
	return hexID(c.ID, 64) && c.LeaseID == l.ID && c.SnapshotAttemptID == attempt && c.ExpiresAt.After(now) && c.Position.ActiveGeneration == l.Position.ActiveGeneration && c.Position.GenesisIssuanceID == l.Position.GenesisIssuanceID && c.Position.SuccessorAllocationID == l.Position.SuccessorAllocationID && c.Position.PredecessorSeal == l.Position.PredecessorSeal && positionIDs(c.Position) && a.Sequence > 0 && a.Sequence <= 100000 && a.TotalBytes > 0 && a.TotalBytes <= 64<<20 && hexID(a.SHA256, 64) && a.Key == fmt.Sprintf("%020d/%s.json", a.Sequence, a.SHA256) && opaqueLabel(a.Lineage, 256) && opaqueLabel(a.TableARN, 2048)
}
func opaqueLabel(s string, max int) bool {
	if s == "" || len(s) > max || strings.TrimSpace(s) != s {
		return false
	}
	for _, c := range s {
		if c < 32 || c == 127 {
			return false
		}
	}
	return true
}
