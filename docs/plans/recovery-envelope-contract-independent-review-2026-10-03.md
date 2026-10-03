# Independent renewed review: recovery envelope pure contract

**Verdict: CLEAR for the bounded pure codec contract**
**Exact reviewed head:** `8e2d5dbd1c4ae90c64697edd8853c730258ac0c6`
**Document SHA-256:** `424a1dfa2159f0e591d830dd21949eedc903edf1858526c61c4add24d677ed75`
**Review clone:** detached at exact head, clean.
**Scope:** pure canonical codec and structural validation only; no builds, source edits, authority-owner claims, or integration claims.

The amendments close all four findings from `final-review-f194ec08.md`:

- `H`, `C`, and `G` remain distinct. The contract allows historical `C` with `C.ActiveGeneration <= OldWriter.Generation`, maps the eligible inner plan generation to the old writer generation, and leaves exact ancestry/store relationships to the owner verifier.
- Complete disposition coverage is scoped to `FROZEN_ONLY`; the eligible wire arm carries the complete account inventory plus eligible IDs and no disposition list.
- Both full-row and summary inventory digest APIs now have a fixed enforceable 1 TiB pure limit. A backup owner can independently enforce a stricter inspection cap before constructing inputs.
- Envelope validation and decoding explicitly recompute `SnapshotInventorySHA256` from fields available in the envelope and reject mismatches; the contract correctly preserves full brain projection/count/byte equality checks for the full-row helper and owner reopening.

I independently recomputed the literal synthetic vectors: brain digest `f1eb7d370f424a93bd89aef88ddf05822fdc1eded3042719339054086d210c3e`; snapshot inventory digest `04a8e01ab69e24a230882daabf374d67eb9bb2bb004c8b81bb2e22dab6f7bc70`. `git diff --check` passes. The older line 132 reference to a stricter “inspection cap” is now explicitly resolved as an owner-side preconstruction limit, with the pure helper's fixed 1 TiB bound stated in the same paragraph and again in the API contract; this does not require the pure API to read external configuration.

This CLEAR applies only to implementing the inert, bounded data codec. It does not establish source ancestry, a journal proof, snapshot pinning, an authority-bearing factory, READY state, provider truth, or production/runtime acceptance.
