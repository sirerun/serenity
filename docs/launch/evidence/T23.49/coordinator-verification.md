# Coordinator verification

The independent ready-brain corruption probe failed before the fix at 3f9a9fb: a structurally valid manifest claiming an empty ready brain restored successfully while losing its canonical file. At ef9934a the same mutation was rejected and the restore destination remained absent (probe exit 0 with explicit rejection assertions).

The final coordinator correction syncs the complete staging tree before publication, including intermediate parent directories. The package race suite and scoped lint passed with exit 0. These checks do not simulate power loss or prove assembled concurrent-write behavior. Service/CLI integration and the production deletion journal remain incomplete; this task is not accepted for deployment.

## Restored writer identity regression

Source 95cdfb0 extends the real provision/backup/restore integration test with a synthetic remember and flush. Both subcases failed before the fix because bundle/clone omits repository-local author configuration and the writer used machine fallback identity. Restore now sets the hosted identity before syncing and publishing staging. Exact original HEAD and clean-tree checks precede the new write. The backup package race suite passes; scoped lint reports zero issues. Initial provisioning/opening does not embed; the later write uses deterministic fixture embeddings. This does not reactivate restored accounts or qualify live recovery.
