# Operator release authentication

This package implements the opt-in `owner-kms-manifest-v1` policy. Sigstore remains the deployment verifier default. No policy, AWS key, permission or signing call is activated by installing this source.

An independently installed policy pins the repository, exact regional KMS key ARN, ECDSA P256 DER SPKI and fingerprint, and target platform. Artifacts never provide their own trust root. `serenity-release sign` uses only the explicitly named static profile from the explicitly named credentials file, requests `ECDSA_SHA_256` over the SHA256 digest of canonical manifest bytes, and verifies the returned signature offline before writing it. It never loads an environment/default credential chain, credential process or proxy. Runtime identities must not have KMS signing permission.

The manifest binds source commit/tree, tag, archive basename/size/hash, platform, Go version, build-recipe/review/test digests, sequence, validity window and policy/key identity. Review and test digests are operator attestations; a signature does not prove that the tests ran or that the build was hermetic. JSON documents must exactly match the fixed struct serialization: no whitespace, duplicate/unknown/missing fields, nulls or trailing data. `canonicalize --manifest INPUT` authors canonical manifest bytes; the verifier never silently normalizes signed input.

Policy, approval, state, pending marker, lock and all trust-path parents must be root owned, non-symlink and not writable by group or others. Verification runs as root. Downloaded manifest/signature/archive are bounded untrusted inputs. The verifier itself must be installed independently in a root-controlled path, never extracted and run from its candidate archive.

## Deployment transaction

Common arguments are:

```
--archive PATH --manifest PATH --signature PATH --policy PATH
--approval PATH --state PATH --tag TAG [--lock-fd N]
```

`verify` checks real cryptography, streamed archive bytes, exact manifest approval, validity and sequence. An independently installed canonical initial state is required; missing state is never genesis. Initial state has version 1, highest_sequence 0, installed_sequence 0, and a manifest_sha256 of 64 zero characters. All subsequent state transitions require an approval pinning the previous state digest.

The installer holds the state sibling `.lock` across `verify`, `prepare`, installation/readiness, and `commit`, passing the inherited descriptor with `--lock-fd`. The CLI duplicates and validates that descriptor against the root-controlled lock inode and independently acquires the exclusive lock. Without an inherited descriptor each invocation acquires its own lock; this does not by itself serialize a multi-command installer.

`prepare` durably creates an exclusive `.pending` marker before installation. The marker pins manifest and exact previous state. Any pending marker blocks normal verification or another prepare. `commit` repeats verification, matches the pending transition, atomically persists the next state, then removes the marker. The highest sequence never decreases, including an exactly approved rollback. Same-sequence reinstallation is rejected.

After a crash, `recover` requires all common arguments plus `--installed-manifest-sha256 EXACT`. A trusted operator must first inspect the actual installed binary/release and provide that exact pin. Recovery permits only the pending transition, including cleanup after state was already persisted. It does not automatically abort, roll back, infer installed identity or accept expired approvals/manifests. If a manifest has expired, a newly reviewed recovery procedure is required; this tool does not bypass expiry.

Signing arguments are `sign --manifest PATH --policy PATH --region REGION --credentials-file PATH --profile NAME --output PATH`. The policy path retains the same independent root trust requirement. Signature output is exclusive-create. There is no live-key bootstrap or unsigned fallback command.

## Verification

Unit tests use ephemeral software P256 keys and an injected KMS API to exercise the real digest/signature contract. Root filesystem tests additionally cover lock exclusion and inherited descriptors, prepare/commit and both crash boundaries, unsafe ownership/permissions/symlinks/FIFOs, exact recovery and strict shell routing. Those root tests explicitly skip in an ordinary unprivileged test run; acceptance must also run the compiled test binary as root in an isolated Linux environment. Tests make no AWS requests and do not establish provider or production acceptance.
