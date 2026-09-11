# Keyed cancellation qualification record

In progress, 2026-09-10. No release or deployment.

The initial implementation extended forget's selector. Store, writer and index
race suites passed, but three pinned protocol checks correctly rejected removal
of forget's required id. Corrected by keeping the five pinned tools unchanged
and adding cancel_memory_operation as an explicitly discoverable extension.

Read-only source review found no blocking semantic defect in the cancellation
projection/writer design. It requested merged-history, codec compatibility and
actual MCP/restart/index proof. Store fixtures and the external proof were added;
results are pending. This review executed no tests. The final code requires full
qualification after the protocol correction.

The local multi-package rerun is deferred while another project's simulator build
holds the shared build lease. Machine load briefly exceeded 40, so no competing
heavy build was started. Hosted CI will also qualify the prepared change; neither
a queued CI run nor this record is a passing result.
