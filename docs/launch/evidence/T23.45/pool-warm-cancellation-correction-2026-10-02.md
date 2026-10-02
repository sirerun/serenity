# Pool warm cancellation correction

Independent source review held pool b454ee59 because pre-canceled cached Acquire/AcquireExisting returned a runtime and an expired-idle Acquire could evict an existing runtime. Coordinator added three actual failing controls against unchanged production source, then checks ctx.Err under the pool mutex before closed/cache/refcount/idle-eviction logic. Existing cold-open cancellation remains. The tests prove the same runtime stays cached and exclusively owned.

Actual pre-fix RED: `go test -run ^TestPreCanceledWarmAcquireKeepsRuntimeAndOwnership$ -count=1 ./internal/hosted/pool`, load 4.47, exit 1; all three controls fail for intended runtime/eviction behavior. Immutable `warm-cancel-pre-fix-red-r2.log` SHA-256 `61d7df343411bf0e05933470d2b478e3447e7f860c84fe33075e2ef707510352`. The first attempted test had an incorrect IdleTTL field and failed compilation; that is preserved separately in warm-cancel-pre-fix-red.log and is not safety evidence. A guarded edit attempt matched more than one site and refused before source mutation; the ensuing warm-cancel-corrected-race.log remained RED and is not a passed check.

After the actual correction, full single-package pool race, vet and lint pass under fresh load <=10 and SSD cache/temp/-p=2. Final passing logs are warm-cancel-corrected-r2-{race,vet,lint}.log; SHA-256 values:

- race: `30244789f60a613998800cbe6db18adcfdebfcd1b11a4f9b33ca447059ea9a8d`
- vet: `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855`
- lint: `e92606b0bf483111dff0a120c315ea165821348f31365020e2468a0059095c47`

Root authoritative source claims were reverified before checks. T23.45 `7418e1d946a51022b862101583e1328343f84304`; R-hosted-runtime `b7e05d8c3509ab9203ab99c02b6aca8ecee5744b`. Original worker claims are now authoritatively released. Its confusing second pair belonged to the shared build-lease remote; no missing authoritative claim or continuous-lease inference is made from that wrong-remote observation.

The first independent reviewer used the author clone rather than a new clone. Tracked production bytes were restored exactly; untracked review artifacts are preserved. A separate dedicated review clone will repeat mutation controls on this corrected source, and final combined full gates remain pending. Full T23.45/46/48 acceptance and live/provider/activation are not claimed.
