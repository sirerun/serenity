# Deletion-safe billing assembly — 2026-10-01

Coordinator owns service assembly and the dashboard deletion callback in an isolated external worktree. Fresh resource leases cover these two paths; the legacy lifecycle lease and gateway source remain untouched. Billing and core writer workers retain separate ownership.

The service coordinator will durably set an active account to deleting before calling CloseBillingAccount; provider ambiguity or pending closure leaves the account restricted and retains brain bytes. Only a certified closed result permits the existing gateway purge. Dashboard deletion delegates to this coordinator. Assembly constructs billing before startup deletion recovery and uses the same coordinator for deleting rows, preserving partner routes and existing deleted-brain cleanup. A billing-enabled service without a closer fails closed. No provider activation or deployment occurs during this implementation.

Tests must prove provider observes deleting before closure, pending/error retains data, successful retry purges, startup recovery closes before purge, and partner routing remains intact. This safety assembly does not complete the independent deletion journal, backup retention, restore activation, periodic reconciliation or live provider gates.
