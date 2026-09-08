# E14 — Disposition state atomicity

An audit on 2026-09-08 forced a real SQLite state-write failure. The item stayed
pending but its history claimed an accepted decision; retrying that key could
then replay a decision that never committed. A second regression synchronized
two independent store handles after reading one pending item. Both returned
successful conflicting verdicts. The existing race test shares one Store mutex,
so it does not cover this boundary.

- [ ] T14.1 Make disposition transitions atomic and preserve competing decisions  Owner: pool  Est: 90m  verifies: [UC-012, UC-026]  deps: [T10.2, T11.3]  acc: [a real SQLite failure leaves both item and history unchanged and a retry can succeed; independent store handles yield one winning terminal verdict and one matching conflict response; queue aging and result bookkeeping cannot overwrite a newer human decision; route metadata is recorded with its decision; regression tests exercise real database transactions and meaningful negative controls with named execution counts and skips]

This task hardens runtime disposition state. It does not authorize multiple
canonical brain writers or change ADR 012's single-writer contract. Filesystem
publication remains governed by the existing inbox journal and writer lock.
