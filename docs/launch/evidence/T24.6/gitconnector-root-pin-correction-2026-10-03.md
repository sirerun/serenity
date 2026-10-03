# Corrected connector root pin and skip semantics

Initial source ce5ce63 remains independently HELD for per-file late root opening and uncounted raced-in symlink errors. Corrected source pins one non-symlink directory identity before HEAD/listing, compares the opened root identity, and keeps that root throughout Poll. Per-read identity checks refuse renamed/replaced repository paths before returning any items. Root refusals and leaf ELOOP are mapped to the existing skip sentinels without matching error strings; other I/O errors retain failure behavior. The opaque Root escape error identity is obtained from a guaranteed rejected parent traversal using the opened root's own API.

New public Poll regression uses an owned fake read-only Git executable that replaces the root while listing. On exact ce5ce63 production source it returned the outside sentinel as a source item (genuine RED); corrected source returns no items and a root identity error. Earlier non-failing attempt and first race leak evidence remain preserved. Strengthened leaf-swap test rejects uncounted errors; an intermediate ELOOP-only mapping missed Root's own escape refusal and genuinely failed. Typed refusal mapping corrects it.

Corrected combined controls pass race-enabled repeat10; entire connector package race/vet/lint pass using SSD caches/temp and load<=10. Independent corrected review and combined full-module qualification remain required; initial hold not yet lifted.

- `root-pin-ce5-genuine-red.log` SHA256 `965b41bed5c91d2802cbe875f6e09db6ab39dbccb4a03c51c1381749c70d2f61`
- `root-pin-and-counted-skip-race10.log` SHA256 `e942594d5bb86cafb7d8855bc3970a6391f1ec809711bf1de3191703a4f4e653`
- `corrected-root-and-skip-race10.log` SHA256 `2ba0bec8af60c668516992b29b825dbfe52afe8ec5dfc40c8e057a493c43a70d`
- `corrected-full-package-race.log` SHA256 `ba147868e86ca53df2e730c90a2f2daf21afb50238f4a7d3ed2e956bacb86ce9`
- `corrected-package-vet.log` SHA256 `9b850f15a746ccc07172bfd64d77f2b8c515a12f161a815286c40afd7805b51c`
- `corrected-package-lint.log` SHA256 `e6d89b2adb3e0627b1871ef7f51a52e64558c6063cc79f84604f9b92938e93a0`
