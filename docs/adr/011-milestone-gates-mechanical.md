# ADR 011: Milestone gates are mechanically encoded as blocked-by annotations

## Status
Accepted

## Date
2026-08-30

## Context
docs/plan.md's build-in-M-order rule (section 1: "a later epic never starts
before the prior epic's exit task is checked, except where a wave's deps say
otherwise") existed only as prose. The pool dispatcher's mechanical dep check
therefore disagreed with the recorded intent in four places:

- E2 wave 2a (T2.1, T2.10, T2.12, T2.16, T2.19): deps [T0.x, T1.x] all checked,
  so a pool run would dispatch them, while plan.md and roadmap.md record E2 as
  "gated behind T1.23" (the human-only M1 exit verification).
- E3's remaining tasks (T3.4, T3.9, T3.10, T3.11, T3.17): the 2026-08-29 trim
  pass recorded them as "pool-dispatchable, not blocked", but their deps
  reference unchecked E2 tasks (T2.1, T2.5, T2.17, T2.19).
- plan.md section 5 listed T4.7, T4.12, T4.13 as startable during E3, but their
  deps need E2 work (T2.17, T2.1, T4.7 respectively). Only T4.3 is genuinely
  cross-epic-startable.
- T5.9 (E5 wave 5a): deps [T1.4] met, while the E5 epic is milestone-gated
  behind M1+M4.

The contradiction surfaced on 2026-08-30 in an /apply --loop session, which
found the mechanically dispatchable set (5 E2 tasks + T5.9) contradicting the
recorded gates. David was offered early E2 dispatch and directed "refine the
plan first" instead: the recorded M-order gate stands, and the plan must say
so mechanically.

## Decision
1. The M-order milestone gate is authoritative: an epic does not start before
   the prior epic's exit task is checked, and this gate is encoded on task
   lines, not left in prose.
2. E2 wave 2a tasks (T2.1, T2.10, T2.12, T2.16, T2.19) carry
   `blocked-by: [T1.23]`. Later E2 waves are transitively blocked through
   their deps on wave 2a and need no annotation.
3. T5.9 carries `blocked-by: [T4.17]` (T4.17 checked transitively implies
   T1.23 checked, satisfying the MS5 gate's MS1+MS4 precondition).
4. The stale prose is corrected: E3's remaining tasks are deps-blocked on E2;
   only T4.3 is cross-epic-startable during E3.
5. Rule of construction going forward: where prose intent and task deps
   disagree, fix the annotations the same day -- the pool dispatcher reads
   deps and blocked-by fields, not prose.

## Consequences
- The pool's dispatchable set now matches the recorded plan: until T1.23
  (David's real Gmail/repo verification) completes, only T4.3-class
  cross-epic tasks dispatch. The plan's throughput is deliberately bound to
  the M1 exit's honest verification; that is the cost of the gate and it is
  accepted.
- If David later decides to open E2 early, it is a one-line removal of the
  blocked-by annotations plus a roadmap decision line -- not a re-litigation.
- Future plan edits must keep blocked-by annotations and prose gates in sync;
  the /plan trim pass checks for exactly this class of drift.
