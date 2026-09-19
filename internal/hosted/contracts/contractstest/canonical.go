package contractstest

import (
	"context"
	"fmt"
	"sync"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

// MemCanonical is a fake canonical store for one or more brains. It models the
// two states that matter to absence proofs: committed history, and a fact
// written to the working tree but not yet committed (which a later Flush can
// still commit — see contracts.CanonicalAbsent).
type MemCanonical struct {
	mu      sync.Mutex
	seq     int
	commits map[string]map[string]string // brain → operation ID → commit ref
	pending map[string]map[string]bool   // brain → operation ID → written, uncommitted
}

func NewMemCanonical() *MemCanonical {
	return &MemCanonical{commits: map[string]map[string]string{}, pending: map[string]map[string]bool{}}
}

// WritePending records an uncommitted working-tree write.
func (c *MemCanonical) WritePending(brain, op string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending[brain] == nil {
		c.pending[brain] = map[string]bool{}
	}
	c.pending[brain][op] = true
}

// Commit makes the operation canonical and returns its commit reference.
func (c *MemCanonical) Commit(brain, op string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.commits[brain] == nil {
		c.commits[brain] = map[string]string{}
	}
	if ref, ok := c.commits[brain][op]; ok {
		return ref
	}
	c.seq++
	ref := fmt.Sprintf("commit-%d", c.seq)
	c.commits[brain][op] = ref
	delete(c.pending[brain], op)
	return ref
}

// Landed reports whether the operation is in committed history.
func (c *MemCanonical) Landed(brain, op string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.commits[brain][op]
	return ok
}

// Checker returns the contracts.CanonicalChecker over this store: landed if
// committed, unknown if only pending in the working tree, absent otherwise.
func (c *MemCanonical) Checker() contracts.CanonicalChecker { return canonicalChecker{c} }

type canonicalChecker struct{ c *MemCanonical }

func (k canonicalChecker) Check(_ context.Context, rec contracts.OperationRecord) (contracts.CanonicalVerdict, error) {
	k.c.mu.Lock()
	defer k.c.mu.Unlock()
	if ref, ok := k.c.commits[rec.BrainID][rec.ID]; ok {
		return contracts.CanonicalVerdict{Outcome: contracts.CanonicalLanded, Ref: ref}, nil
	}
	if k.c.pending[rec.BrainID][rec.ID] {
		return contracts.CanonicalVerdict{Outcome: contracts.CanonicalUnknown, Ref: "pending_working_tree"}, nil
	}
	return contracts.CanonicalVerdict{Outcome: contracts.CanonicalAbsent, Ref: fmt.Sprintf("head-%d", k.c.seq)}, nil
}

// FailingChecker always errors, modelling a transient canonical read failure.
type FailingChecker struct{}

func (FailingChecker) Check(context.Context, contracts.OperationRecord) (contracts.CanonicalVerdict, error) {
	return contracts.CanonicalVerdict{}, fmt.Errorf("canonical read failed")
}
