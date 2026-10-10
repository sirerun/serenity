# Source contract freeze

The owner approved DynamoDB journal authority and KMS release signing implementation. Live infrastructure and trust activation remain separately gated.

Authority contract SHA256: 55f99e3a1ea2012a40e578198edb9cc1f7cdd119615d1b6e652cddbcf7485895. Independent runtime review is CLEAR.
Runtime contract SHA256: 48f205efa72f02f0f5f35af6bb584f8e7196796e45072592d3d1f81962c9b9f9. Independent release review is CLEAR.
Release contract SHA256: bf4e07d185ec78e8da1eba611a720ed58e257d193686a3aced324331a3902b75. Independent coordinator review is CLEAR with mandatory durable pending-install marker and inherited lock validation, incorporated before source release.

All three documents are frozen for source implementation despite historical draft headings. Any implementation-driven interface adjustment must retain the stated invariants and receive independent review before integration. This is design evidence, not source correctness, provider qualification or deployment evidence.

Release review resolved contradictory missing-state handling: missing state always rejects and only explicit genesis initializes it. Prepare must durably create pending intent before installation; verification refuses unresolved intent; commit and recovery require exact pinned transaction identities. One lock spans the entire deployment transaction.

Runtime recovery integration describes required work, not an assertion that a complete production recovery coordinator already exists. Startup activation precedes replay; handlers remain unpublished until replay succeeds, and any replay failure revokes the lease after resource joins.
