# Frozen source contract

See INS-00 contract freeze in docs/plans/inspector-brand-release.md. Prototype bed37f58 is a visual/reference implementation, not real-data timestamp authority.

Owners: brand_ship site/brand/generators/dashboard page.html excluding explorer and embed; api_ship dashboard.go route additions and new inspector.go/inspector_test.go; root frontend/build/embed/plan records after landed dependencies. Independent release_preflight reviews frozen candidates and validates rollout. No broad lifecycle/recovery ownership.

API brain inventory has no provisioning effect. Filter scope/search/year apply to eligible pool, cursor stable-order and filter-bound; totalMatching is whole matching eligible dataset while client loaded/visible counts are separately labeled. Pagination edges are induced by page but node sourceIds retain eligible links for assembling cross-page edges. Claim/source observed dates remain separate from added-year membership.

### Initial-page graph context extension

Graph responses add contextNodes (at most100 eligible immediate neighbors of the primary page) and contextTruncated. Primary nodes, totalMatching, limit and cursor retain their existing meaning. Edges span the induced primary-plus-context set; context follows the same ownership, visibility, lifecycle, expiry and erasure rules, but can fall outside query/year filters. Frontend must label/dim related context separately and exclude it from filtered totals and loaded-primary counts. Empty primary pages have empty context. All arrays remain nonnull. Stable deterministic context ordering and truncation are verified separately. This unpublished version1 contract extension is approved by coordinator after frontend discovery; it does not add mutations or disclosure privileges.

Validity precision: validUntil is nullable text. Fact values are canonical UTC RFC3339Nano; claims preserve their stored year/month/day/timestamp precision. Eligibility uses domain.Claim.CurrentAt, including malformed-date fail-closed behavior. Display the stored precision; do not derive capture dates from it.
