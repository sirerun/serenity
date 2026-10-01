# Initial retention library review — 2026-10-01

Review target741db0aded809fb17604d8b3d49e29ec743cbfda is held for corrections before integration. This review used an isolated extraction of the exact committed library and fake-storage fixture. No AWS calls or object-store deletion occurred.

Runtime control1: approve the generated plan, advance the supplied clock by one second, then apply. The initial library refuses with “approved cutoff changed; create a new plan.” A manually reviewed cutoff must stay fixed rather than requiring review within one second; execution may not broaden the approved deletion set to newly aged objects.

Runtime control2: bind the fake adapter to a different bucket, generate an approved plan naming the configured backup bucket, then apply. The initial library deletes one fake version because bucket identity is metadata only and is not passed to storage calls. Every call must enforce the selected bucket as well as expected owner. The corrected wrong-bucket control must make zero mutations.

Additional requested corrections: reject future timestamps, malformed IsLatest values and duplicate multipart IDs; distinguish verified upload absence from generic API failure; document SDK timestamp normalization accurately. The source author is correcting these before independent qualification. Production IAM/lifecycle boundaries, adapter/private-plan qualification, concurrency control, scheduler, cloud purge and retention-age acceptance remain open.
