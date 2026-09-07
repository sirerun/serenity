package conformance

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// transcriptProtocols is the subset of protocols whose fixtures use the
// Transcript{Cases:[]Case} shape -- memory_verbs' cases.json is gbrain's
// own upstream case format (a bare JSON array, T4.20's own
// TestMemoryV1AllPinnedCases loads it directly), not a Transcript.
var transcriptProtocols = []string{"disposition", "direction"}

// repoRoot locates the repository root relative to this test file's own
// path (internal/conformance/fixtures_test.go -> ../.. -> repo root),
// the same runtime.Caller(0) technique
// internal/eval/brainbench.corpusDir and
// internal/eval/direction.corpusDir already use for their own corpora, so
// this test resolves correctly regardless of the working directory a
// test runner uses.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// protocols is testdata/conformance/'s three subdirectories, RFC 0001
// section 8's own three protocol names (docs/protocol/schemas.Registry
// uses the identical set).
var protocols = []string{"memory_verbs", "disposition", "direction"}

// TestConformanceFixturesExist is the acc line's first clause, literally:
// "testdata/conformance/{memory_verbs,disposition,direction}/ exist with
// a MANIFEST of sha256s".
func TestConformanceFixturesExist(t *testing.T) {
	root := repoRoot(t)
	for _, p := range protocols {
		dir := filepath.Join(root, "testdata", "conformance", p)
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			t.Errorf("testdata/conformance/%s/ does not exist", p)
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err != nil {
			t.Errorf("testdata/conformance/%s/MANIFEST does not exist", p)
		}
	}
}

// TestConformanceFixturesManifestsMatch is the acc line's second clause:
// "CI fails if a transcript changes without its manifest entry". go test
// ./... runs this on every push (ci.yml's test job -- the same mechanism
// internal/eval/ava_corpus_test.go, internal/eval/direction and
// internal/gate/adversarial_corpus_test.go already rely on for their own
// checksum-pinned corpora), so an accidental or malicious edit to any
// vendored case file or captured transcript fails CI here unless its
// MANIFEST is re-pinned in the same change.
func TestConformanceFixturesManifestsMatch(t *testing.T) {
	root := repoRoot(t)
	for _, p := range protocols {
		p := p
		t.Run(p, func(t *testing.T) {
			dir := filepath.Join(root, "testdata", "conformance", p)
			if err := VerifyManifest(dir); err != nil {
				t.Fatalf("VerifyManifest: %v\n(if you edited a fixture deliberately, regenerate its MANIFEST -- see testdata/conformance/README.md)", err)
			}
		})
	}
}

// TestConformanceFixturesManifestNonEmpty guards against a vacuous green:
// a MANIFEST that pins zero files would make
// TestConformanceFixturesManifestsMatch pass trivially without actually
// covering any fixture.
func TestConformanceFixturesManifestNonEmpty(t *testing.T) {
	root := repoRoot(t)
	for _, p := range protocols {
		dir := filepath.Join(root, "testdata", "conformance", p)
		m, err := LoadManifest(dir)
		if err != nil {
			t.Fatalf("%s: LoadManifest: %v", p, err)
		}
		if len(m) == 0 {
			t.Fatalf("%s: MANIFEST pins zero files", p)
		}
	}
}

// TestConformanceTranscriptsLoadAndAreWellFormed guards against a
// vacuous fixture set: every *.json file under disposition/ and
// direction/ must parse as a Transcript, name its own protocol/operation
// correctly, and carry at least one Case with at least one Step -- a
// transcript file present on disk with zero cases would pass every
// checksum check above while covering nothing.
func TestConformanceTranscriptsLoadAndAreWellFormed(t *testing.T) {
	root := repoRoot(t)
	for _, p := range transcriptProtocols {
		dir := filepath.Join(root, "testdata", "conformance", p)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("%s: ReadDir: %v", p, err)
		}
		found := 0
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
				continue
			}
			found++
			path := filepath.Join(dir, e.Name())
			tr, err := LoadTranscript(path)
			if err != nil {
				t.Errorf("%s: LoadTranscript: %v", path, err)
				continue
			}
			if tr.Protocol != p {
				t.Errorf("%s: protocol = %q, want %q", path, tr.Protocol, p)
			}
			wantOperation := e.Name()[:len(e.Name())-len(".json")]
			if tr.Operation != wantOperation {
				t.Errorf("%s: operation = %q, want %q (from filename)", path, tr.Operation, wantOperation)
			}
			if len(tr.Cases) == 0 {
				t.Errorf("%s: zero cases -- vacuous transcript", path)
			}
			for _, c := range tr.Cases {
				if c.Name == "" {
					t.Errorf("%s: a case has no name", path)
				}
				if len(c.Steps) == 0 {
					t.Errorf("%s: case %q has zero steps -- vacuous case", path, c.Name)
				}
				for i, s := range c.Steps {
					if s.Method == "" || s.Path == "" {
						t.Errorf("%s: case %q step %d: missing method/path", path, c.Name, i)
					}
					if s.Response.Status == 0 {
						t.Errorf("%s: case %q step %d: response status is 0", path, c.Name, i)
					}
				}
			}
		}
		if found == 0 {
			t.Fatalf("%s: no *.json transcript files found", p)
		}
	}
}
