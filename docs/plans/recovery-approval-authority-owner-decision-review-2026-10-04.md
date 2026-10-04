# Independent review — recovery approval authority decision packet v2

Verdict: **CLEAR for decision use only**. The v2 packet closes the prior packet-specific holds. It remains proposed and grants no source, verifier, factory, provider, PR #358, recovery-effect, or production authority.

- Reviewed file SHA-256: `cb3918b3f97bd2a4bb72b49f2f4d65ff6474f5cb53b5e9e71487f5a2477cfdda`.
- Source tree cited by the packet: `c916bcd7ed30592efb065c876455d1bb51509523`, tree `0b026e37226fd8295f2ca1a350a4f5072816a336`.
- The cited verifier and planner document hashes match the pinned tree. Their own declared baselines remain identified as historical contract provenance.

## Prior HOLDs closed

1. The account-activation binding now includes the direct `SnapshotSHA256` required by `AccountActivationBinding`, alongside `RecoveryEnvelopeHash`, committed `EpochRecordDigest`, one account, operation, nonce, and expiry. Global binding also includes the direct snapshot digest and the complete inventory/scope/writer binding.
2. The packet now identifies the continuation authorization as separate from the three primary approval scopes. It binds the same envelope, current epoch-record digest, exact next phase, operation, nonce, and expiry; it separately assigns issuer-policy ownership and the epoch store's atomic replay/current-phase responsibility. No global-approval retry is treated as continuation authority.
3. The global action set is enumerated exactly as the held planner contract: `quiesce`, `stop`, `revoke`, `seal`, `restore-frozen`, `publish-frozen`, `adopt`.
4. Candidate time and nonce rules are explicit: canonical UTC RFC3339Nano round-trip, `issued_at <= now < expires_at`, expiry after issue, configured maximum age/lifetime, and the proposed 64-lowercase-hex representation of 32 random nonce bytes. The packet leaves common encoding and policy for owner decision and labels the schema/domain candidate rather than frozen.
5. Source attribution now pins the exact review tree and the relevant contract file hashes; it distinguishes the held verifier proposal from the earlier code baselines those documents declare.

## Authority boundary

The packet clearly requires owner assignment and acceptance before verifier source freeze. It creates no issuer, trust root, evidence object, key, verifier, or approved evidence. It keeps pin-owner and envelope-codec scope separate from READY, effect, account activation, startup factory, restore quota, provider, deployment, and production acceptance. The prior v1 HOLD report remains historical; this CLEAR applies only to the v2 decision packet.

No Go commands, build leases, source edits, claims, provider actions, or messages were used for this review.
