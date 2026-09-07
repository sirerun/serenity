//go:build ignore

// Command gen_manifest re-pins evals/corpora/reconcile/labels/checksums.yaml
// after a deliberate row change, using internal/eval's checksum tooling
// unmodified (ADR-005) -- the same manifest internal/eval/reconcile's tests
// verify with eval.VerifyManifest. Run it from the repo root after adding,
// editing, or removing a row:
//
//	go run evals/corpora/reconcile/gen_manifest.go
package main

import (
	"log"

	"github.com/sirerun/serenity/internal/eval"
)

func main() {
	const labelsDir = "evals/corpora/reconcile/labels"
	const manifestPath = "evals/corpora/reconcile/labels/checksums.yaml"
	if err := eval.WriteManifest(labelsDir, manifestPath); err != nil {
		log.Fatalf("gen_manifest: %v", err)
	}
}
