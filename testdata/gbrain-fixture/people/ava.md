---
type: person
aliases: [Ava Example, AE]
external_ids:
  fixture: ava-001
custom_note: preserved verbatim
---
# Ava Example

Synthetic person for importer acceptance. Works with [[projects/beacon|Beacon]].

## Facts
<!--- gbrain:facts:begin -->
| # | claim | kind | confidence | visibility | notability | valid_from | valid_until | source | context |
|---|-------|------|------------|------------|------------|------------|-------------|--------|---------|
| 1 | ~~Use a hard cutover~~ | commitment | 0.95 | world | high | 2026-01-01 | 2026-02-01 | meeting: one | superseded by #2 |
| 2 | Prefer feature flags | preference | 0.85 | private | medium | 2026-02-01 | | email: two | choice \| rationale |
| 3 | ~~Old contact detail~~ | fact | 0.70 | private | low | 2025-01-01 | 2026-01-01 | imported notes | forgotten: user request |
| 4 | Presented the plan | event | 0.80 | world | high | 2026-03-01 | 2026-03-01 | calendar | C:\notes\ava |
| 5 | Tests prevent regressions | belief | 0.60 | private | medium | | | conversation | |
<!--- gbrain:facts:end -->

## Takes
<!--- gbrain:takes:begin -->
| # | claim | kind | who | weight | since | source |
|---|-------|------|-----|--------|-------|--------|
| 1 | Migration will finish | bet | people/ava | 0.95 | 2026-02 -> 2026-04 | planning |
| 2 | ~~Obsolete prediction~~ | hunch | brain | 0.20 | 2025-12 | historical |
<!--- gbrain:takes:end -->

## Timeline
- 2026-01-01: Planning began
- 2026-02-01: Feature flags selected
