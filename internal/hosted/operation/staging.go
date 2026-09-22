package operation

import (
	"context"
	"fmt"
	"sync"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/store"
)

// StagingConfig describes the measured admission envelope. Physical and Free
// must report allocated bytes from the same filesystem as the staging area.
type StagingConfig struct {
	Budget, MaxStage, Headroom int64
	Quota                      map[string]int64
	Physical                   func(account string) int64
	Free                       func() int64
}

type stageState struct {
	ticket   contracts.StageTicket
	admitted int64
	set      bool
}

// StagingGate is an in-process admission gate. Deployments must place the
// staging directory on an OS-enforced size-limited filesystem; this gate is
// the accounting and early-abort layer, not a substitute for that boundary.
type StagingGate struct {
	mu          sync.Mutex
	cfg         StagingConfig
	outstanding int64
	next        uint64
	tickets     map[string]stageState
	pending     map[string]int64
	released    map[string]bool
}

func NewStagingGate(cfg StagingConfig) (*StagingGate, error) {
	if cfg.Budget <= 0 || cfg.MaxStage <= 0 || cfg.Headroom < 0 || cfg.MaxStage > cfg.Budget || cfg.Free == nil || cfg.Physical == nil {
		return nil, fmt.Errorf("invalid staging configuration")
	}
	return &StagingGate{cfg: cfg, tickets: map[string]stageState{}, pending: map[string]int64{}, released: map[string]bool{}}, nil
}

func (g *StagingGate) ReserveStage(ctx context.Context, req contracts.StageRequest) (contracts.StageTicket, error) {
	if err := ctx.Err(); err != nil {
		return contracts.StageTicket{}, err
	}
	if req.AccountID == "" || req.BrainID == "" || req.OperationID == "" {
		return contracts.StageTicket{}, fmt.Errorf("invalid staging request")
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.outstanding+g.cfg.MaxStage > g.cfg.Budget || g.cfg.Free()-g.outstanding-g.cfg.MaxStage < g.cfg.Headroom {
		return contracts.StageTicket{}, contracts.ErrStagingBusy
	}
	g.next++
	id := fmt.Sprintf("stage-%d-%s", g.next, store.ID())
	t := contracts.StageTicket{ID: id, AccountID: req.AccountID, BrainID: req.BrainID, OperationID: req.OperationID, CeilingBytes: g.cfg.MaxStage}
	g.tickets[id] = stageState{ticket: t}
	g.outstanding += g.cfg.MaxStage
	return t, nil
}

func (g *StagingGate) AdmitMeasured(ctx context.Context, t contracts.StageTicket, measured int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	st, ok := g.tickets[t.ID]
	if !ok {
		return contracts.ErrStageTicketUnknown
	}
	if measured < 0 {
		return fmt.Errorf("negative measured bytes")
	}
	if measured > st.ticket.CeilingBytes {
		return contracts.ErrStageCeilingExceeded
	}
	if st.set {
		if st.admitted == measured {
			return nil
		}
		return fmt.Errorf("stage already admitted")
	}
	quota, ok := g.cfg.Quota[t.AccountID]
	if !ok {
		return contracts.ErrStorageQuotaExceeded
	}
	if g.cfg.Physical(t.AccountID)+g.pending[t.AccountID]+measured > quota {
		return contracts.ErrStorageQuotaExceeded
	}
	st.admitted, st.set = measured, true
	g.tickets[t.ID] = st
	g.pending[t.AccountID] += measured
	return nil
}

func (g *StagingGate) Release(ctx context.Context, t contracts.StageTicket, outcome contracts.StageOutcome) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	st, ok := g.tickets[t.ID]
	if !ok {
		if g.released[t.ID] {
			return nil
		}
		return contracts.ErrStageTicketUnknown
	}
	if outcome != contracts.StageDiscarded && outcome != contracts.StagePublished {
		return fmt.Errorf("invalid stage outcome")
	}
	g.outstanding -= st.ticket.CeilingBytes
	g.pending[t.AccountID] -= st.admitted
	delete(g.tickets, t.ID)
	g.released[t.ID] = true
	return nil
}

func (g *StagingGate) Outstanding() int64 { g.mu.Lock(); defer g.mu.Unlock(); return g.outstanding }
