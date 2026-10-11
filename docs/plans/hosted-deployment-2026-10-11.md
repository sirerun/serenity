# Hosted deployment receipt — 2026-10-11

The owner-authorized deployment succeeded on the existing production host. The hosted service and privileged broker are active, the broker is enabled, and the backup timer is active. Public readiness returned HTTP 200; an unauthenticated dashboard request redirected with HTTP 303.

## Exact deployed artifact

| Binding | Value |
| --- | --- |
| Source commit | ed8f0b9b76ed235da5c2febc4aebfc2c615a754a |
| Source tree | 4d3f44b64fff0b251f27c2eb85fa26578e32e5ac |
| Canonical manifest SHA-256 | e56744c48f820dfa6e430ed31e2649a8d7cd32d4cab5877346b1d4aca7af2a53 |
| Release archive SHA-256 | 9cf9b2e3b764d1f9d526c7a34175629a74bcfc9e1d56caaf23687c8806c7ecda |
| Executable SHA-256 | f502e0f158bac645590cef6770a39707b18c8774a32d5679d15ecc5805752072 |

The manifest was signed with the approved KMS policy. Root-controlled release verification, prepare and commit completed at release sequence 1. Deployment used a reviewed manual SSM activation path with the reviewed helpers. It did not execute the default deploy.sh path, and no GitHub release was published.

## Evidence boundaries

Source changes received independent review and landed verification. Local source checks are separate from provider observations; hosted GitHub Actions remained unavailable because of the account billing restriction.

Provider qualification exercised private readiness, a real witnessed backup, frozen restore, restart with epoch advancement, and revocation producing HTTP 503. The journal namespace was restored at epoch 3 while preserving its position; owner restore was idempotent. These checks do not prove nonempty historical deletion replay or a complete renewal and rollback campaign.

Production private readiness returned HTTP 200 after activation. In Chrome, the authenticated visual memory explorer visibly loaded its map with 200 notes and displayed 609 total records. List, detail and private-filter behavior were verified. Production backup jobs completed, and the backup timer was enabled and active.

## Preservation and remaining qualification

The original journal table, original data, immutable S3 checkpoint and EBS checkpoint remain retained. Preserved legacy data included 446 operations; this is a legacy operation inventory, not fabricated historical deletion-journal evidence. The temporary qualification IAM policy and session-created local owner credential file were removed; original credential stores were preserved.

The requested deployment is complete. Broader T-HU.6 qualification remains open for nonempty deletion replay, sustained renewal and exact compatible rollback. The canonical plan's composite delivery gates remain open where their full acceptance criteria exceed this deployment receipt. No source, local or synthetic check is presented as provider or production acceptance.
