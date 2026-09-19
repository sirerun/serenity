package contracts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Restore fencing (interfaces.md "Recovery activation barrier", owner task50,
// review owner task41).
//
// STATUS: PROPOSED, not frozen. The chief-architect review must choose this
// mechanism or a distributed generation barrier; this file freezes nothing.
//
// A new writer may start only after the old service can no longer mutate
// shared state. Three independent facts are required, because each defeats a
// different old-writer failure and none implies another:
//
//  1. JournalSealed: the deletion journal generation the old writer used is
//     closed by DeletionJournal.Seal. Stops a cooperating-but-stale writer
//     (a paused process that resumes) from appending behind the reader.
//  2. OldInstanceStopped: the infrastructure provider reports the old
//     instance stopped or terminated. Never a lock file, PID or local check:
//     the writer lock (internal/writer.AcquireBrain) is a kernel-local flock
//     on one host and cannot observe another host at all.
//  3. OldCredentialsRevoked: every credential and session the old instance
//     could still use is revoked or denied, including its live cloud role
//     session, so an unreachable-but-alive instance cannot write to the
//     journal or backups. Stopping an instance does not invalidate a session
//     token already issued to it.
//
// A same-host process restart, where the host does not change, may satisfy
// the instance-stopped fact with the local writer lock (that is what the lock
// actually fences); that narrower case is task50's to specify and is not
// offered for cross-host restore.
type FenceReceipt struct {
	// Generation is the journal generation this fence closes.
	Generation            int64
	JournalSeal           DeletionWatermark // returned by DeletionJournal.Seal(Generation)
	OldInstanceStopped    bool
	OldCredentialsRevoked bool
	VerifiedAt            time.Time
}

// ErrFenceInsufficient means the receipt does not prove the old writer is
// fenced. The message lists every missing fact.
var ErrFenceInsufficient = errors.New("hosted/contracts: old writer is not proven fenced")

// Sufficient returns nil only if all three facts hold. RecoveryApplyResult.
// Unfrozen may be true only after Sufficient returns nil for the plan's
// generation.
func (f FenceReceipt) Sufficient() error {
	var missing []string
	if f.Generation <= 0 || f.JournalSeal.Generation != f.Generation || f.JournalSeal.SequenceID <= 0 || f.JournalSeal.EntryHash == "" {
		missing = append(missing, "journal seal for this generation")
	}
	if !f.OldInstanceStopped {
		missing = append(missing, "provider-verified old instance stop")
	}
	if !f.OldCredentialsRevoked {
		missing = append(missing, "revocation of every old credential and session")
	}
	if f.VerifiedAt.IsZero() {
		missing = append(missing, "verification time")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: missing %s", ErrFenceInsufficient, strings.Join(missing, "; "))
	}
	return nil
}

// WriterFencer establishes a FenceReceipt. Task50 implements it; the provider
// and credential calls behind it are external effects and run only in the
// authorized recovery window, never in unit tests.
type WriterFencer interface {
	Fence(ctx context.Context, generation int64) (FenceReceipt, error)
}
