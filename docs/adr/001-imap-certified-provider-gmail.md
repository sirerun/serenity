# ADR 001: Gmail via app password is the certified IMAP provider for v1

## Status
Accepted

## Date
2026-08-27

## Context
RFC 0001 §10.1 cuts the P0 email connector to "IMAP with one certified provider"
and leaves the provider open (OQ2, "before M1"). The choice sets which mailbox
the M1 acceptance criterion ("import 30 days of real email on a laptop") runs
against, which auth path `serenity connectors auth` must support first, and
which provider quirks the golden fixtures encode.

Candidates weighed: Gmail with an app password, Gmail with OAuth XOAUTH2,
Fastmail, iCloud Mail.

## Decision
Certify **Gmail over IMAP with an app password** (requires 2-step verification
on the Google account). Auth material lives in the OS keychain
(`internal/secrets`), never on disk. The connector is written against the
generic IMAP surface (UID-based cursors, `UIDVALIDITY` invalidation, IDLE or
poll) so that adding a second provider is a fixture plus a doc row, not a code
fork.

Rejected:
- Gmail XOAUTH2: best long-term UX, but an open-source binary would need a
  Google Cloud OAuth client and app verification before anyone but the author
  could authorize it. Revisit when there is a maintained OAuth client id.
- Fastmail: the cleanest IMAP implementation, but a small audience; it becomes
  the second provider once the Gmail fixture exists.
- iCloud: app-specific passwords work, but IMAP behavior is the least
  predictable of the four.

## Consequences
- Widest reachable user base for M1's first-value walkthrough; the maintainer can
  dogfood on a Google Workspace mailbox.
- App passwords are a per-account manual step; the connector guide must walk
  it (RFC §10.1: "your token expired" is the first failure users hit).
- Microsoft Graph stays deferred post-launch as its own project (RFC v2.1
  changelog); nothing here changes that.
