// Package contractstest holds reference models and deterministic conformance
// suites for the PROPOSED contracts in internal/hosted/contracts.
//
// It is test-support code, not production code, and nothing under
// internal/hosted/service or internal/cli imports it. Its purpose is to make
// each proposed decision executable before it is approved: a reviewer can read
// a scenario instead of prose, and the feature owner (task44/48/50) later runs
// the same suite against the real implementation through the exported Run...
// functions. A reference model passing its own suite proves the proposed rules
// are self-consistent and testable; it is not evidence about any production
// code, which does not exist yet, and it is not architecture approval.
//
// Every scenario is single-goroutine and sequenced by explicit calls, a fake
// clock and pre-cancelled contexts, so an interleaving is reproduced exactly
// rather than raced. The concurrency tests assert only order-independent
// invariants and run under the race detector.
package contractstest
