# ADR 002: Code complete ends at M5; M6 is a measured soak on finished code

## Status
Accepted

## Date
2026-08-27

## Context
RFC 0001 defines code complete as "every P0/P1 milestone passes its acceptance
criteria on commodity hardware, with the scale profile verified on a
128GB-class box." §17 lists M0-M5 as the ordered v1 critical path and M6 as a
7-day unattended soak on both profiles plus chaos and latency measurements.
Whether M6 sits inside or after "code complete" changes when the plan can be
declared done and what the last milestone's tasks look like.

## Decision
**Code complete = every M0-M5 acceptance criterion in RFC §17 is green on a
laptop.** M6 is planned as an outline epic that starts after code complete: it
measures the finished binary (soak, chaos, p95, repo growth, ladder promotion
on real dispositions) and produces numbers, not features. The 128GB-box scale
profile check belongs to M6.

## Consequences
- The plan's exit gate is objective: the M0-M5 AC checklist in `docs/plan.md`.
- Anything M6 discovers (a lost job under kill, a p95 miss) is a defect against
  code complete and reopens the relevant epic; it does not move the boundary.
- Launch (RFC: "M5's ACs green") coincides with code complete; M6 numbers are
  published after, as the RFC already says for BrainBench ("a trend, not a
  gate").
