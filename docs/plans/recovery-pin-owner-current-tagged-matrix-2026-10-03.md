# Current combined tagged crash qualification

Clean integration source `c916bcd7ed30592efb065c876455d1bb51509523` has Go fingerprint `5176070520a9496599cbf0aabe44cfcc2daae34fa26681c13b231f6c4d1c0425`, exactly matching producer author `f1d9e9eadcaf08d6b71d490af3e1bc6373ec76f2`. The coordinator inspected the actual stage records, source equality and assertion output.

Current three-package tagged race, vet and lint passed for backup, recovery and testhooks. Commands exited 0, source remained unchanged during each command, and exact releases exited 0. Tagged race lease `ffb40811243872e6c2837a7e16bd6e73216dfc67`; vet `4d4539ce987442b04a508160f84160a748b1250f`; lint `2cffededc3dc8c721249ca5c65ed5e7f575e2af1`. The race run includes the corrected distinct-phase inventory and stricter tombstone fixture.

Five compiled current-source regression controls exited 1 at their intended test assertions, with exact released leases and original source restored after each:

- Bypassing fresh release authorization let PinKeep authorize partial deletion. Lease `65728bde60d9bf601b9dadca94207b9c22355464`; mutation diff SHA-256 `71de41929eed8598ea97ffad7aa9e0c91ba097ce8d0c38fcb26cc5da543d5969`.
- Checking only inode after acknowledgement accepted changed receipt content. Lease `43a2ab66ad75593beed9566e965cb2354ef713d1`; mutation diff SHA-256 `97269b75757b17dded18f9248b65310130e0d2a6759a49705e9637d4a359841d`.
- Removing the terminal receipt caused reopened real owner/producer reconciliation to conflict. Lease `4c037923c98d652d615afeec6227fe710b99c70d`; mutation diff SHA-256 `e93ba8c0f573ce0e5c2435703675e95ab3527f59348809afd078d82d4273fa5c`.
- Bypassing pre-owner peak reservation let release succeed when the real budget-gap fixture required refusal. Lease `cc0939bae8c41067392f3c246bc323000ae0648d`; mutation diff SHA-256 `eacce94b13273eec5906565c389f630355ca59be5df340d53882f0d370ba557b`.
- Moving actual tombstone writing before live-tree deletion was rejected by the tombstone fixture, because the live lease still existed. Lease `db9689df467054ec588d3fc6964c4ec4c66ef6c6`; mutation diff SHA-256 `f8c8eae194d1a6748e621982f055a4786716a28bf39fea8c0f2b2f6a7753092b`.

After all five mutations were restored, the complete three-package tagged race suite passed again at the clean integration head and same fingerprint. Restored-run lease `90335cd00af178b3bd356f8209b2b23f509c6368`, command exit 0, exact release exit 0, source unchanged. No compile/setup failure, load hold or lost claim counts as a negative control.

The matrix has 40 actual phase cases plus five canonical partial-temp fixtures (45 total). It checks captured store/directory/temp identities, retained device/inode/mode/inventory/bytes, partial-artifact restart, and exact owner/producer pairing. Owner PENDING with a durable prepublication STAGED lease is an unchanged incomplete boundary. Ordinary production capture remains a no-op. The matrix does not prove power loss or every unlink order inside the unchanged standard-library RemoveAll call.

Historical c754 focused receipts and the first full-module failure remain in the evidence archive. The first full-module snapshot at 1c9 failed only the duplicated phase-list entry, recording 3159 passing test/subtest events and 83 passing packages; it is not full qualification. Current corrections supersede those source checks. Combined full-module qualification and final exact-head independent review must be recorded separately before merge.
