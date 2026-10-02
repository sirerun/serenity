# Hosted Git migration service fixture integration

Coordinator source baseline `1bb11d421ba981a0db51436b221fb0801ba98c1e` includes pool source `b454ee59` on main `e6dc3f66`. `go test -count=1 ./internal/hosted/service` genuinely failed in five tests because warm canonical fixtures had relied on removed pool auto-initialization. No production service change is made.

Only the five positive warm paths now call a new explicit canonical Git fixture helper. It initializes owned Git, fixed local writer identity and the `.serenity/` exclusion; the pool still creates its normal runtime-config baseline. The general deletionFixture is unchanged, including intentionally Git-free cold controls, and all original assertion bodies remain. Existing startup quota/reconciliation, authenticated Unix operator review and cold/inactive/canceled controls pass.

Before correction: fresh load 1.90, command exit 1, immutable `service-fixtures-pre-correction-red.log` SHA-256 `469786b290e1b8b8fd057f428af4996a8e1fc3c3bd575c861aeeb8a8a1547e2c`. After correction: fresh load 1.25, identical package command exit 0, `service-fixtures-corrected-green.log` SHA-256 `34666c1667b54a368050fd3e044aea72f5bf3f095d8250ce77b8f9d86abe4da1`. Both logs are in the external hosted-git-validation evidence directory. The single-package check does not require the shared multi-package build lease; SSD cache/temp and -p=2 were set. R-hosted-service claim `2cf69858606749697a819fb2db597130307adacb` was verified before both checks.

Independent review and combined final full local gates remain required. No full T23.45/46/48 acceptance, provider action, deployment or spend is claimed.
