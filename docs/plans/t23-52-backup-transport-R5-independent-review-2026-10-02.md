# T23.52 R5 proposal — independent rereview

**Verdict: CLEAR for coordinator freeze and a separately explicit source-development assignment.** This is contract approval only; it does not accept T23.52, authorize caller wiring, qualify recovery, or authorize any provider/live action.

**Proposal SHA-256:** `424438e782f41c3b2831c873a8adf155434fe02f15696c4dc297cae28e5e4991`  
**R4 HOLD report SHA-256:** `7da548fd1826c7f68d45e8e97992ad9c87f46a2c66d4abc7c20fdd743dcfd1bc`  
**Source baseline:** `e6dc3f66ee25fe9b9e30b4b3acae6472b812d075`, tree `35cd9a431c1f70e76f56b8bdfda99dbf4b3fe473`; publisher blob `86eb4573a3af5769b615e7e488ad9a386f44a985`, identical to PR346 source commit `68557003d6cbf7b4ec8e01217967793d5743856a`.

R5 resolves the R4 bootstrap-length contradiction. For COMPLETE and manifest, it explicitly treats `max_bytes` as a ceiling, requires a positive typed HEAD length within that ceiling, reads exactly the HEAD-declared body in explicit-version ranges, and validates returned range, length, and version metadata before consuming data. Download then validates COMPLETE structure and manifest digest before parsing the manifest. Publisher checks compare those bounded records with the helper's expected canonical bytes. For artifacts, manifest lengths are passed as ceilings; the existing helper checks final exact length and SHA-256. The current manifest-v2 parser rejects zero-length artifacts, and R5 requires positive object lengths, so no unsupported zero-length artifact path is introduced.

The R4 safeguards remain: complete versions-plus-delete-markers inventory before reads; explicit selected version IDs; final inventory and checksum verification; the 1,003 combined-entry/two-page inventory limit; one pre-COMPLETE and one post-COMPLETE full artifact pass normally, a third pass for ambiguous final-COMPLETE reconciliation, and one pass for download; the stated byte, range, and invocation budgets; a same-PID `RLIMIT_FSIZE` launcher with a test against the actual CLI writer; a private executor independent of the retention runner; all-history destination absence; exclusive-writer, mutation, versioning, and lifecycle gates; and no caller wiring or installed-script/unit, retention, credential, or deployment changes.

The exact e6 baseline and PR346 collision-correction blob remain as recorded in R4. The helper's definite `ObjectExists` path still rethrows before reconciliation, and its existing regression test remains present. No source was changed and no tests or provider requests were run. Ajent MCP was unavailable; repository and local Serenity coordination feeds were checked, with no new applicable hold found.

Minor editorial note: R5 §§11 and 19 retain references to “this R4 proposal” / “The R4 proposal itself” when describing the current proposal's unapproved/freeze status. The R5 header already states the correct status; normalize those references to R5 when freezing the document.
