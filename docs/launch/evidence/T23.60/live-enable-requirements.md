# Live-load enablement requirements

The CLI returns BLOCKED/exit 2 with a zero-call receipt before reading any manifest or credential or opening a socket. Candidate client tests remain useful local evidence, not a supported live-run entry point.

Before removing that unconditional gate, implement and test:

- Failure criteria over all offered requests, not COMPLETE/exit 0 when tool/protocol/network outcomes fail. Check every published threshold, isolation and durability predicate.
- Deadline/cap checks immediately before every socket operation, including queued work, readiness, initialization and notifications; cancel/drain pending work without launching expired requests. Reject nonfinite, boolean, negative and malformed budget values.
- Canonical exact-origin guards and complete redirect rejection; private credentials never enter output or upstream error text.
- Verified seeded account/brain/cardinality/storage state, actual cold-open workload and separate quota saturation; the cold flag alone is not a cold request.
- All three frozen repetitions, all offered outcomes, bounded provider/readiness/rebuild cost, no silent skipped forgets or precharge-only counts represented as executed calls.
- Reviewer-frozen workload and exact deployed source/provider/cost authority, then local HTTP regressions and a separately authorized real run.

Owner: T23.60 follow-up, with T23.41 review and T23.64/T23.68 qualification inputs. No approval or spending is implied by this document.
