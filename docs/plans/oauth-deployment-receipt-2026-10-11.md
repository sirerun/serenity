# OAuth interoperability deployment — 2026-10-11

Owner-requested deployment of PR376 completed on the existing Serenity service. Signed release sequence2 is installed and committed, with no pending transaction.

| Binding | Value |
| --- | --- |
| Source commit | da5c346b586b82b8a25f2d8f7a2211e6bdbaf573 |
| Source tree | 531eb0be58fc452d81e1a1be4ebde49d4dfc7fec |
| Archive SHA256 | 8ded1ca99062fbd842b18034e93e0619ac4dd58f7db44cb27ede51bb1562c9e3 |
| Running executable SHA256 | 2b75212f959474a6df1b3a3180d8bc27d9ec817139f1662589ec61a6ab5eaa53 |
| Canonical manifest SHA256 | 844f58e59bc19734a16531fddc7dff839d354243b45adb2bae26f09964893f97 |

The existing pinned KMS key signed the manifest. The installed root verifier checked the signature, archive, exact approval and previous-state pin before downtime. Activation held the release-state lock across prepare, installation, private readiness, actual process executable measurement and commit.

The authority stack gained the new exact approved build, published configuration version and API deployment while retaining the previous versions and build. Invocation concurrency stayed0 through the update and new-version exclusive-policy installation. Full policy readback matched; the base deny policy was unchanged before restoring concurrency1. No authority code, keys, principal grants, data directories, schema or journal namespaces changed. Previous binary and broker configuration remain preserved.

## Verification and limits

Independent source and transaction reviews cleared the candidate. Full local build/vet, affected race tests, lint0, plan conformance and landed checks passed; no hosted CI success is claimed. Fresh backups succeeded before shutdown and after activation. Service, broker and backup timer were active; public readiness returned200.

Public curl authorization probes using Claude and Gumloop metadata and deliberately unsupported response_type returned302 with unsupported_response_type. Claude previously returned500 for the same probe. No redirect was followed or consent/grant created by these probes. A Python urllib attempt returned403 and is not counted as successful verification.

The deployed Claude metadata compatibility repair is confirmed. A complete signed-in Claude/Gumloop connection remains unverified; the earlier Gumloop browser callback abort is not claimed resolved. Fresh authorization is required because old consent requests are single-use. Browser protections remained enabled. No automated data restore or broad rollback qualification is claimed. Existing AWS CLI operator credentials were used without transferring or changing credential stores.
