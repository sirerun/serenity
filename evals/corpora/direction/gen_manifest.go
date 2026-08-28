//go:build ignore

// Command gen_manifest re-pins evals/corpora/direction/labels/checksums.yaml
// after a deliberate row change, using internal/eval's checksum tooling
// unmodified (ADR-005) -- the same manifest the direction package's tests
// verify with eval.VerifyManifest. Run it from the repo root after adding,
// editing, or removing a row:
//
//	go run evals/corpora/direction/gen_manifest.go
package main

import (
	"log"

	"github.com/sirerun/serenity/internal/eval"
)

func main() {
	const labelsDir = "evals/corpora/direction/labels"
	const manifestPath = "evals/corpora/direction/labels/checksums.yaml"
	if err := eval.WriteManifest(labelsDir, manifestPath); err != nil {
		log.Fatalf("gen_manifest: %v", err)
	}
}
