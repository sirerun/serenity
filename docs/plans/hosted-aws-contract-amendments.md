# Reviewed implementation amendments

## Renewable runtime lease

The initial one-hour lease cap requires explicit renewal for a long-running host.
Renew(ctx, Lease, operationID) must compare ACTIVE state, exact epoch, session
and current expiry in the transactional state CAS. The existing lease must
remain unexpired at admission and acknowledgement; the new expiry must be
strictly greater and within the configured maximum lifetime. Renewal preserves
the lease ID, epoch, writer identity and process binding. It cannot revive an
expired, closed, revoked or PREPARING lease.

The operation uses the same durable idempotency receipt and pending-witness
barrier as journal mutation. Success is returned only after the independent TIP
acknowledges the renewal. Current reads the persisted expiry. Runtime closes
admission promptly on renewal failure or expiry; it stops and joins renewal
before releasing journal authority. Independent release reviewer cleared these
invariants; source and concurrency verification remain required.

## Source API name correction

The actual existing constructor is deletion.NewJournalAt. Draft references to
NewWriterJournal are naming errors, not permission to invent another authority
model. Startup uses the existing constructor after verified observation.

## Operator and backup boundaries

The runtime broker continues to require the exact managed systemd MainPID.
Offline restore uses a separately configured explicit owner credential transport
and the owner API route. Historical finalized cuts may be read by that owner
after the old runtime lease expires, but must still be authenticated against
the independent current lineage. Online scheduled backups use the existing
peer-authenticated admin socket and service maintenance lock.

Restoring a snapshot creates only frozen pending-reactivation data. Owner
journal-table disaster rebind and complete sealed recovery are separate gates;
neither a sidecar nor a restored database grants production admission.
