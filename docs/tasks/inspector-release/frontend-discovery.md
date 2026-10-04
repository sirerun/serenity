# Frontend integration discovery

Prototype bed37f58 uses static fixture globals in App and GraphCanvas. Integration must inject a dataset/context into both; loading real data cannot leave synthetic detail helpers behind. Keep static demo fixtures in the public-demo bundle only; authenticated bootstrap must never fall back to fictitious memories after HTTP failure.

Proposed integration: same built React entry detects /explore/ versus /dashboard/explore. Public path loads explicit synthetic demo; private path loads session-authenticated brain inventory then bounded graph pages. Each brain switch aborts outstanding fetches, clears prior-memory state and remounts App with the selected brain/dataset identity. Session401 clears all memory state and offers same-origin sign-in. Errors and an empty brain are explicit; no WebGL switches to accessible list. Bounded loaded-count labels distinguish loaded records from account totals; rendering cap avoids an unbounded10K scene. Filters/year navigation apply to the loaded projection unless server-authoritative counts become part of the frozen contract. Missing source metadata stays unavailable instead of showing raw paths/URLs.

Vite build currently targets standalone prototype dist and absolute/design assets. Production entry assets must use /assets/explorer/ with site/explore/index.html included in site/embed.go, and authenticated route must supply private no-store headers and same-origin CSP connect-src. Keep prototype routes/gallery and artifacts out of public packaging except accepted mark. Retain existing site tooling/build embed tests.

Implementation waits on INS-A5 and INS-B5 as saved plan requires; this document is source discovery, not speculative implementation.
