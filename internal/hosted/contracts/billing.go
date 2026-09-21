// Package contracts freezes the callable Go interfaces, failure codes and
// shared types named in docs/launch/hosted-completion/interfaces.md, so
// dependent feature tasks (44/46/47/48/49/50/53) can implement and be wired
// against a stable signature. This package defines types only: it holds no
// production logic, no HTTP calls and no database access. Each feature task
// implements its own concrete type satisfying the relevant interface in its
// own package; the integrator wires the concrete type into
// internal/hosted/service assembly once the feature is ready (interfaces.md
// "File-change handshake").
//
// The chief architect approved the operation-accounting, deletion-journal and
// restore-fence designs on PR236 revision
// 218d7234d9abea5964f9d4640d1bfffe5c9f8087; task41 reconciled the narrow
// commit-fence scope. Storage admission remains conditionally approved and
// blocked until task44 proves physical allocation accounting and an
// OS-enforced stage limit. Approved design does not mean production
// implementation, ordinary review, or launch qualification is complete.
package contracts

import (
	"context"
	"errors"
	"time"
)

// Billing truth (interfaces.md "Billing truth", owner task47).
//
// ReconcileCustomer fetches the server-owned provider customer for
// accountID, validates prices/ownership, replaces stale subscription/
// checkout state atomically and returns eligibility without activating a
// frozen account. It never looks up a provider customer by a client-supplied
// customer ID — accountID is the only caller input.
type BillingReconciler interface {
	ReconcileCustomer(ctx context.Context, accountID string) (ReconcileResult, error)
}

// ReconcileResult reports the reconciled entitlement window. Source records
// which provider object the eligibility decision came from, for the
// telemetry/audit trail; it is never empty on a non-error return.
type ReconcileResult struct {
	Eligible           bool
	PlanID             string
	CurrentWindowStart time.Time
	CurrentWindowEnd   time.Time
	GraceUntil         time.Time // zero when no grace window is active
	Source             string    // e.g. "stripe_subscription", "stripe_checkout_complete"
}

var (
	// ErrBillingProviderAmbiguous means the provider returned more owned
	// subscription/checkout state than ReconcileCustomer can safely resolve
	// (interfaces.md: "provider ambiguity leaves deletion pending"). Callers
	// must not activate or deactivate entitlements on this error.
	ErrBillingProviderAmbiguous = errors.New("hosted/contracts: billing provider state is ambiguous")
	// ErrBillingProviderUnavailable means the provider request failed
	// (timeout, 5xx, network). The caller keeps the account's last known
	// reconciled state; it must not treat this as "no subscription".
	ErrBillingProviderUnavailable = errors.New("hosted/contracts: billing provider unavailable")
	// ErrBillingAccountFrozen means accountID is mid-deletion or otherwise
	// frozen: ReconcileCustomer must return this instead of eligibility.
	ErrBillingAccountFrozen = errors.New("hosted/contracts: account is frozen for billing reconciliation")
)

// Billing closure (interfaces.md "Billing closure", owner task47 with
// deletion48).
//
// CloseBillingAccount is resumable: calling it again after a partial failure
// continues from where the previous call left off. It prevents new checkout,
// expires pending checkout sessions, lists and cancels every subscription the
// provider shows against this account's customer (including subscriptions
// the local database has never observed, e.g. from an unprocessed webhook),
// and only returns CloseStatusClosed after re-reconciling that no owned
// subscription remains active.
type BillingCloser interface {
	CloseBillingAccount(ctx context.Context, accountID string) (CloseResult, error)
}

type CloseStatus int

const (
	CloseStatusUnknown CloseStatus = iota
	// CloseStatusClosed: no checkout can start, no subscription remains
	// active or pending at the provider. Safe to proceed to deletion purge.
	CloseStatusClosed
	// CloseStatusPending: closure could not be certified this call (a
	// provider timeout, an ambiguous list result, or a checkout session the
	// provider has not yet resolved to expired/complete). The deletion
	// journal keeps this account's deletion pending rather than purging.
	CloseStatusPending
)

type CloseResult struct {
	Status CloseStatus
	// PendingReason is set and non-empty whenever Status is
	// CloseStatusPending; it names the exact unresolved provider object
	// (never a raw provider error body — interfaces.md/evidence.md forbid
	// provider error bodies in receipts, and the same rule applies to
	// values this type can end up logged/rendered from).
	PendingReason string
}
