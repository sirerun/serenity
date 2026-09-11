# RFC-BRAIN-AUTH-02 — explicit operator credential profiles

Owner: hq-rfc_brain_auth/Sonnet. Root-reviewed implementation scope from corrected
RFC-BRAIN-AUTH-01 design. Isolated serenity-brain-auth branch from qualified57166d6;
Serenity exact-read and all Ajent worktrees read-only. Own outbox/brain-auth code
status/evidence plus isolated worktree. No push/PR/HQ commits until root review.

Implement explicit --credential-profile NAME for HTTP serve and credential lifecycle.
Validate names with a bounded lowercase alphanumeric/hyphen grammar. Never read a
profile selector from brain files/config/.serenity or environment. Absent option
preserves existing behavior. Explicit empty/malformed/missing profile fails closed;
never consult legacy credentials as fallback. Profile+stdio must reject rather
than imply HTTP authentication applies to stdio. Preserve existing core MCP tools.

Keep bare connect/status read-only. Provide explicit --provision-token with a
required profile for new profile creation; mutually exclusive with --rotate-token.
Fresh independent tokens only, never copy legacy value. Per-profile status/rotation
and serve select the same keychain account namespace. TokenSource remains per-request
so rotation applies without restart. Do not expose token values in args/output/logs.
CLI help/docs must explain operator-selected profiles are not persistent per-brain
bindings; status showing a root does not prove a stored binding exists. Duplicate
profile references across private Ajent bindings must be rejected in that future
configuration task, not claimed solved by naming discipline here.

Avoid unrelated doctor/config/auto-install changes unless necessary for correct
profile status. A cloned/preseeded brain cannot select a profile by its content.
Existing mTLS is already possible with AllowLAN=true and explicit loopback bind;
leave it unchanged. Its future client-certificate wiring has its own planned gate.

Tests: profile validation incl explicit-empty, legacy absence path, selected missing
profile refusal, fresh profile separation, scoped rotation, readonly status, flag
conflicts, no secret stdout/stderr. Actual STOCK CLI process proof: two disposable
brain roots and two freshly generated uniquely named profile keychain entries;
A/B own tokens work, cross tokens fail, rotating A invalidates oldA while B works,
clone/preseeded repository metadata never selects a profile. Real profile names
must use an exclusive random task prefix. Creation/read/rotation/deletion of ONLY
those newly owned disposable keychain entries is authorized for this proof. Never
read/rotate/delete an existing user credential or the legacy shared entry. Unit
mock tests cover legacy non-interference; disclose that boundary separately. If
stock CLI init would touch legacy credentials, construct owned brain fixtures
without that init path; do not add a test-only auth backdoor to the shipping binary.
Cleanup must track exact owned profile names and never use prefix-wide deletion.

Use supported filesystem/OS-keychain mechanisms, Go/honest-verdicts skills, bounded
process waits and minimal proof-child environment. No environment inventories,
real gateway dotenv access, paid calls, descendants, deployments or active client
changes. Full race/lint/vet/build needs load≤10 and won shared build lease; exact
own SHA release, BuildOffload caches. Bank actual count/skips/pins/process evidence
and every failure. Reread REVIEW.md and this contract before final.
