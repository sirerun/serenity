package conformance

import (
	"os"
	"path/filepath"
	"runtime"
)

// DefaultFixturesDir locates testdata/conformance/ relative to this source
// file's own path (internal/conformance/paths.go -> ../.. -> repo root ->
// testdata/conformance) -- the same runtime.Caller(0) technique
// fixtures_test.go's own repoRoot helper and internal/eval's corpus-dir
// helpers already use. It resolves correctly for `go run`/`go test`/`go
// build` invoked from any working directory in a normal source checkout.
// A binary built on one machine and copied to run on another, where that
// baked-in source path no longer exists, falls back to
// "testdata/conformance" relative to the current working directory -- the
// same convention gen_transcripts.go's own doc comment already assumes
// ("GOWORK=off go run testdata/conformance/disposition/gen_transcripts.go"
// run from the repo root).
func DefaultFixturesDir() string {
	if _, file, _, ok := runtime.Caller(0); ok {
		candidate := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "conformance")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return filepath.Join("testdata", "conformance")
}
