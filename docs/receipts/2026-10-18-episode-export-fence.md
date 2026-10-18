# Legacy export episode disclosure fence — T5.63–T5.67

Base54e321b843ae75dd11edba2482477adad42d6354; tested source
b9570f54081d19f34c5c07537ca1ce9f32f4f99d, archive SHA256
18a1689182da1d38607ec3b5af661b34fd512b1189c63b6d8e47767e2f1bd16c
matched on the existing Linux ARM64 runner.

Legacy account export creates a complete Git bundle, retaining old episode
objects even after retrieval exclusion. It now refuses unsupported episode
history before creating a bundle or emitting output. It checks current canonical
episode inventory and brain-scoped known intents, then every retained commit/tag
metadata inventory. Deleted HEAD sources and non-default refs cannot bypass the
fence; archive-disabled mode does not bypass it. Unknown/invalid/ref-bound
history and failed inventory refuse with static errors. Existing account,
maintenance and runtime mutation ownership are retained; checks repeat after
flush. No exact episode-export implementation or capability is advertised.

Bounds:256 commits,4096 unique metadata blobs,16MiB aggregate Git stdout,1MiB
per command,64KiB metadata and10second scan deadline. Exceeding them refuses
without a truncated substitute. Ordinary episode-free export within this
qualified envelope remains; unknown kinds or direct non-commit refs refuse.
Explicit ordinary raw index_only metadata is allowed without tracked bytes.
Reserved memory kinds still require complete pairs and episodes always refuse.

Independent EXP-R1 identified index-only compatibility in the initial version.
The correction uses an actual SourceStore.Write/Git fixture. Initial negative
fixtures failed earlier at cold pool projection; repaired current HEAD plus
retained malformed historical snapshots now test the intended guard directly.
All zero-output assertions remain. No production pool contract was changed.

Observed at the exact revised source, Go1.26.9, GOMAXPROCS2, private temporary
storage and bounded serial commands:

- Full `go test -race -count=1 -p=1 -timeout=360s ./internal/hosted/gateway`: pass39.693s.
- Gateway vet: pass; gateway lint: zero issues.
- Scoped govulncheck: zero called vulnerabilities; two module-only advisories outside called code remain.
- Formatting/diff and full Wazi plan checks pass.

This is source/synthetic evidence, not hosted CI, installed/native/provider or
physical-purge acceptance. Full T5.3/portable export remains incomplete. Inherited
current session selection, one implementation outcome under ceiling3 and separate
independent review; token/cost telemetry unavailable. Ajent MCP unavailable;
repository/GitHub coordination checked. ADR024 source fallback applies to billing
blocked Actions without changing protections. Review/landing exact pins follow
in the PR; current root source assignment has separate authority/service owners.
