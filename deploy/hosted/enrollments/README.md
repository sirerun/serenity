# CIMD authority enrollment snapshot

`cimd-6efadae.json` preserves the existing hosted authority resources and immutable
published versions while adding the version, API deployment and exact-route
permissions for the reviewed CIMD source `6efadae23a9572529ad12309ce230ee83ac18b6c`.
It is an IaC candidate, not an applied enrollment or a deployment receipt.

The exact measured executable SHA256 is
`d74d211e3289be433086ffbeb3808226ad0d262c73b6e11c263906d1a38c927a`.
At execution, the `ApprovedBuilds` parameter must preserve all current mappings
and add only this digest-to-source pair; every other parameter uses its current
value. Deployment metadata, signing approvals and source/test receipts remain in
the private release record.

The candidate preserves every existing IAM role, key, journal, code artifact,
principal, namespace and lineage. It changes the Gateway description and points
the existing owner/runtime API integrations and stage at the new immutable
version/deployment. Existing published versions and permissions are retained.

Do not apply this snapshot directly to a moving stack. Reconcile the current
stack/template and exact provider change-set preview, obtain independent
operational review, drain and close writers under an admitted maintenance
transaction, and keep authority concurrency zero through version and policy
qualification. Full resource-policy denies and exact API principal/account/route
restrictions must pass before restoring the previous concurrency. Activation,
backup, compatible rollback and closeout require their own exact reviewed packet;
this template grants none of those effects by itself.
