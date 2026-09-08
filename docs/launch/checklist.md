# Launch surface checklist

Inventory taken 2026-09-08, 12:45–12:55 UTC for T7.1. Owner and rollback coordinator: **David Ndungu**. This is a dated deployment inventory, not the human launch decision (T7.6).

## Serenity deployment

| Surface | Observed revision and state | Evidence / rollback |
| --- | --- | --- |
| [Public website](https://serenity.sire.run/) | Pages revision `58f58cfb78981b931a9cc7ddebb5d8f1f4140ba8`; HTTPS 200 with certificate validation; last modified 2026-09-08 09:27:35 GMT | [Successful Website run](https://github.com/sirerun/serenity/actions/runs/34210052593). Previous successful revision `89a47c02c449bf257d63a4261692e1e484519ab1`: [run](https://github.com/sirerun/serenity/actions/runs/34209643192). Restore reviewed site changes through a revert PR and rerun Website. |
| DNS | Authoritative `ns-cloud-b1.googledomains.com` returns NOERROR with authoritative answer: `serenity.sire.run. 300 IN CNAME sirerun.github.io.`; public resolution reaches GitHub Pages | [Foundation DNS change](https://github.com/sirerun/foundation/pull/224). David owns rollback through the foundation DNS workflow; do not change unrelated records. |
| Pages security | API custom domain `serenity.sire.run`; certificate approved through 2026-12-06; **`https_enforced: false`**. HTTP currently returns 200 rather than redirecting | [Pages settings](https://github.com/sirerun/serenity/settings/pages). T7.5 must enable enforcement and repeat HTTP/HTTPS checks. |
| Adoption chat | Function/stack `serenity-adoption-chat`, `us-west-2`, Python 3.13, **`$LATEST`** (no immutable published version), modified 2026-09-08 09:00:11 UTC; Active, last update successful, 28-second timeout, 256 MB | [Committed deployment template](https://github.com/sirerun/serenity/blob/66d8c49c92ecdcddbe6ed0d7efc0d49c5fe8a499/deploy/chat/stack.json). Downloaded function ZIP hash matches AWS; handler bytes match this revision. Roll back reviewed handler/template changes through an UPDATE change set, preserving the secret and rate salt; never delete/recreate the stack. See [deployment procedure](https://github.com/sirerun/serenity/blob/66d8c49c92ecdcddbe6ed0d7efc0d49c5fe8a499/deploy/chat/README.md). |

Chat code SHA-256 (AWS base64): `wGw1t54u7WIpmwrIMZNxsVVRuyTQiAVWYAIY+3BAz+4=`. Deployed handler SHA-256: `caa4c6f855ca2e8f04a3e0782fde3f864748c7520cc4c400d2651f40d7b0f49a`. CloudFormation was `UPDATE_COMPLETE`, last updated 2026-09-08 09:00:06 UTC. These identify the mutable deployment without falsely calling `$LATEST` an immutable release.

The public endpoint is recorded in [chat-config.js](https://github.com/sirerun/serenity/blob/58f58cfb78981b931a9cc7ddebb5d8f1f4140ba8/site/assets/chat-config.js). GET `/healthz` returns **200**, `{"status":"ok","documents":13}`. GET `/` correctly returns **405**, `{"error":"Use POST."}`; T7.3 owns explicit health-route CI coverage and operational alarms. This inventory made no model request and did not read or record secret values.

## Ajent sign-in destination

Public browser inspection reached [human sign-in](https://ajent.social/human/login) and its [legacy access-key page](https://ajent.social/login), without signing in. The legacy page renders `/static/juice.png` in a 36×36 header box; its SHA-256 is `4952c76d4cf070e0a3b4d971139516abd2e2366b8937b8e1febee3b60211f267`. The wordmark is visible, but the orange artwork appears clipped in the inspected desktop page. This is an observed asset revision, **not a claim that PR #24 is deployed**. GitHub's repository deployment API returned no deployment records; the exact running Ajent commit remains unverified and belongs to its release owner.

| Change | Classification | Named owner and reason |
| --- | --- | --- |
| [PR #15: contrast](https://github.com/ajent-social/ajent-social/pull/15) | **Merge — already merged** | David Ndungu. Merged 2026-09-07 21:25:41 UTC at `2e3aa5a9b5178bc9bcf1e9407e84d511100fd689`. Live inclusion has no commit attestation in the inspected deployment records; verify it before claiming deployed. |
| [PR #24: browser-login logo](https://github.com/ajent-social/ajent-social/pull/24) | **Defer** | David Ndungu, Ajent release review. Open head `554659e2e343059b19a58c1f3be46601d38b30f4`; 66 changed files, +4,105/−149 lines, including API/infrastructure/schema changes. Requires review of its actual scope and a deployment/rollback revision, not approval based on the logo title or CI alone. |
| [PR #25: consulting footer](https://github.com/ajent-social/ajent-social/pull/25) | **Defer** | David Ndungu. Still draft at `583dcfa93c002af33483d343d6aeceafefeddeab`; one file, +1/−1 line. Await its author's readiness and Ajent review. |

Ajent rollback belongs to David in that repository: use the reviewed deployment mechanism and last confirmed healthy revision. No Ajent merge or deployment was performed by this inventory, and no unverified rollback commit is invented here. The shared Ajent feed was read; its September 7 deployment statements were treated as historical references and checked against current APIs/browser behavior.

## Next acceptance gates

- [x] T7.1: inventory live URLs, exact known deployment/asset revisions, owners, rollback routes, and Ajent PR classifications.
- [ ] T7.2: document and verify `install_cta`, `docs_open`, `chat_started`, `chat_answered`, `chat_failed`; browser coverage and production event observation, excluding prompts and secrets.
- [ ] T7.3: health/CORS/outage/rate-limit CI coverage and an actionable deployed error/latency signal.
- [ ] T7.4: reviewed launch content packet with canonical installation walkthrough; publication remains a separate action.
- [ ] T7.5: enforce HTTPS and retain authoritative DNS, certificate, redirect and Pages API evidence.
- [ ] T7.6: David's go/no-go decision, adoption baseline, known limitations and next review date.

The M4 external-session exit is complete ([report](../evals/m4-report.md)). M5 remains 11/15: naming, the real-mailbox laptop run, name-dependent README, and human release decision remain open ([report](../evals/m5-report.md)).
