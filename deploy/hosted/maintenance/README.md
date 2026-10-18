# Same-release fixture maintenance

This is the versioned source owner for a bounded fixture-maintenance operation
that preserves the installed service pair and capture settings. It replaces
historical private transition templates as source; it is not an installed
controller or an operational approval.

`render_runtime.py` strictly validates an exact declaration and pinned inert
`CONFIG = None` template, producing private manifest/runtime files without
executing them. `runtime.py` owns durable entry, held startup guards, ordered
fixture outcomes and forward closeout recovery. `observe_closed.py` performs
bounded authenticated read-only authority collection. The fixture collectors
and proof renderer produce distinct v3 handoffs to the existing service-UID
operator. Existing v1/v2 admission stays unchanged.

Read [CONTRACT.md](CONTRACT.md) for exact inputs, trust boundaries and recovery
rules. In particular, expiry retains the hold, interrupted children cannot be
reported successful, and operational root code never queries SQLite or reads fixture credentials. The
metadata-only tool mode compares custody; its digests confer no authority.

Source checks require owner-only temporary storage and umask077. Python tests
are discovered under this directory; Go tests live in
`evals/episode-qualification`. `evals/episode-qualification/qualify_linux.py`
exercises the actual producer, Python validator and Go child with a separate
numeric service UID, six inherited descriptors and synthetic storage. Its
optional legacy artifact tests actual old-source adoption, reissue and revoke.
The helper in `testdata/custody_helper` is built separately and never linked
into the operational child. This harness does not exercise the real systemd
wrapper or authenticate provider authority.

Live invocation remains gated by current installed host, service-UID/FD,
systemd and provider qualification plus an independently reviewed finite private
operational packet through the trusted owner channel. Rendering a packet,
passing source tests or renewing a closeout record does not grant fixture
permission. No provider capacity, production credential, capture or canonical
data operation is authorized by these sources alone.
