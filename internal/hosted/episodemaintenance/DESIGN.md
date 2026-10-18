# Fresh authoritative episode maintenance history (T5.68)

Source is constructed by the existing trusted hosted factory with its authority
client and live lease callback. Transport independently authenticates principal,
namespace and approved process. Caller-supplied observations, cuts, seals and
booleans cannot substitute for that factory injection. This package creates no
second journal, controller, provider or public route.

Observe validates the current opaque issuance, epoch, source build and expiry,
then requests a fresh authority cut with separately generated attempt/operation
identities. It validates the returned lease/attempt, generation and predecessor
binding, expiry and canonical witness-anchor shape. Authentication of that anchor
belongs to the existing authority client/server/witness protocol; local syntax
checks never authenticate a namespace themselves. Ambiguous cut creation is a
refusal; no observation or callable recheck escapes on failure.

The existing deletion.NewJournalReader observes the complete cut from genesis
under its existing allocation and chain-verification bounds. Its exact position
must match the authenticated cut. Active-generation prefixes are valid immutable
cuts: Read.Sealed is not required. This differs from the sealed recovery path.

Snapshot.Recheck checks the latest local issuance and authenticated Current,
requiring the exact cut position and epoch and matching live expiry. Renewal of
the same lease/reservation/writer/build/epoch is allowed while the cut remains
unexpired. Replacement issuances, expired capability, changed history, missing
objects, cancellation and unknown provider outcomes refuse. A second local lease
read closes a concurrent callback-renewal/replacement race conservatively.

The service owns maintenance exclusion, account/brain binding and canonical
preparation. It must recheck immediately before activation or disclosure. This
package does not activate storage, remove copies, report physical completion or
make exclusion authentication claims from canonical source bytes. Returned
observations are ordinary Go values; only trusted internal factory wiring may
supply Source implementations.

Tests use the real authority Kernel, Server, Gateway, Client and journal reader
with in-memory AtomicStore/WitnessStore and synthetic AWS request identity. They
exercise cut/current/object transport, complete history, same-issuance renewal,
history drift and malformed/expired/missing authority. These are source semantics,
not live AWS, IAM, durability or production qualification evidence.
