package contractstest

import (
	"context"
	"fmt"
	"sync"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

// StagingConfig fixes the numbers a StagingGate enforces.
type StagingConfig struct {
	// Budget is the most stage bytes that may be reserved at once, across every
	// account and brain on the host. It is an accounting limit on reservations;
	// it becomes a hard bound only with an OS-enforced size limit under the
	// staging area (see the "Accounted bytes are not enforced bytes" point in
	// contracts/storage.go), which no fixture here provides or proves.
	Budget int64
	// MaxStage is the ceiling granted to every stage: a configured constant
	// task44 must prove, in allocated bytes, at least as large as the biggest
	// mutation the product accepts, and that the stager accounts against with a
	// StageMeter.
	MaxStage int64
	// Headroom is the free space that must remain on the device after every
	// outstanding ceiling is written.
	Headroom int64
	// Quota is each account's physical-byte allowance.
	Quota map[string]int64
	// Free reports observed free device bytes.
	Free func() int64
}

// StagingHarness is what RunStagingSuite needs from an implementation.
type StagingHarness struct {
	Gate contracts.StagingGate
	// SetPhysical sets an account's current physical bytes before a scenario.
	SetPhysical func(account string, bytes int64)
	// Physical reports an account's physical bytes, including published stages.
	Physical func(account string) int64
	// Outstanding reports the sum of ceilings currently reserved.
	Outstanding func() int64
}

// StagingFactory builds a fresh gate from cfg.
type StagingFactory func(cfg StagingConfig) StagingHarness

// MemStagingGate is the reference contracts.StagingGate.
type MemStagingGate struct {
	cfg      StagingConfig
	mu       sync.Mutex
	seq      int
	tickets  map[string]*stageState
	ceilings int64
	pending  map[string]int64 // account → admitted, unpublished bytes
	physical map[string]int64
}

type stageState struct {
	ticket   contracts.StageTicket
	admitted int64
	isAdmit  bool
}

func NewMemStagingGate(cfg StagingConfig) *MemStagingGate {
	return &MemStagingGate{cfg: cfg, tickets: map[string]*stageState{}, pending: map[string]int64{}, physical: map[string]int64{}}
}

// ReferenceStaging is the StagingFactory for MemStagingGate.
func ReferenceStaging(cfg StagingConfig) StagingHarness {
	g := NewMemStagingGate(cfg)
	return StagingHarness{
		Gate:        g,
		SetPhysical: func(a string, n int64) { g.mu.Lock(); g.physical[a] = n; g.mu.Unlock() },
		Physical:    func(a string) int64 { g.mu.Lock(); defer g.mu.Unlock(); return g.physical[a] },
		Outstanding: func() int64 { g.mu.Lock(); defer g.mu.Unlock(); return g.ceilings },
	}
}

func (g *MemStagingGate) ReserveStage(_ context.Context, req contracts.StageRequest) (contracts.StageTicket, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.ceilings+g.cfg.MaxStage > g.cfg.Budget {
		return contracts.StageTicket{}, contracts.ErrStagingBusy
	}
	// Unwritten ceilings are subtracted from observed free space: bytes a stage
	// has reserved but not yet written must still be there when it writes them.
	if g.cfg.Free()-g.ceilings-g.cfg.MaxStage < g.cfg.Headroom {
		return contracts.StageTicket{}, contracts.ErrStagingBusy
	}
	g.seq++
	t := contracts.StageTicket{ID: fmt.Sprintf("stage-%d", g.seq), AccountID: req.AccountID, BrainID: req.BrainID, OperationID: req.OperationID, CeilingBytes: g.cfg.MaxStage}
	g.tickets[t.ID] = &stageState{ticket: t}
	g.ceilings += t.CeilingBytes
	return t, nil
}

func (g *MemStagingGate) AdmitMeasured(_ context.Context, t contracts.StageTicket, measured int64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	st, ok := g.tickets[t.ID]
	if !ok {
		return contracts.ErrStageTicketUnknown
	}
	if measured < 0 || measured > st.ticket.CeilingBytes {
		return contracts.ErrStageCeilingExceeded
	}
	if st.isAdmit {
		if st.admitted == measured {
			return nil
		}
		return fmt.Errorf("hosted/contracts: stage %s already admitted at %d bytes", t.ID, st.admitted)
	}
	acct := st.ticket.AccountID
	quota := g.cfg.Quota[acct]
	if g.physical[acct]+g.pending[acct]+measured > quota {
		return contracts.ErrStorageQuotaExceeded
	}
	g.pending[acct] += measured
	st.admitted, st.isAdmit = measured, true
	return nil
}

func (g *MemStagingGate) Release(_ context.Context, t contracts.StageTicket, outcome contracts.StageOutcome) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	st, ok := g.tickets[t.ID]
	if !ok {
		return nil // idempotent
	}
	g.ceilings -= st.ticket.CeilingBytes
	if st.isAdmit {
		g.pending[st.ticket.AccountID] -= st.admitted
		if outcome == contracts.StagePublished {
			g.physical[st.ticket.AccountID] += st.admitted
		}
	}
	delete(g.tickets, t.ID)
	return nil
}
