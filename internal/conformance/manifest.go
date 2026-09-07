// Package conformance loads and checksum-verifies the frozen fixture set
// under testdata/conformance/ (T4.13, RFC 0001 section 8): gbrain's
// vendored MEMORY_VERBS_v1 cases.json, plus Serenity's own DISPOSITION v1
// and DIRECTION v1 transcripts.
//
// The checksum machinery here deliberately mirrors internal/eval's own
// Manifest/WriteManifest/VerifyManifest (ADR-005: "Labels are
// checksum-pinned; CI fails when a label file changes without its
// manifest entry") rather than importing it: that package's
// computeManifest hard-codes a "*.yaml" extension filter, which is right
// for eval label directories (every file in them is a YAML label) but
// wrong here -- a conformance directory holds heterogeneous fixture
// files (cases.json, a vendored LICENSE with no extension,
// *.json transcripts). Generalizing internal/eval's filter to "every
// regular file" would also change its documented behavior for every one
// of its existing production callers (internal/eval/runner.go's live CLI
// path among them) for a use case that package was never scoped to
// cover. This package keeps the identical semantics (mismatch / unpinned
// addition / unpinned removal are all failures) over "every regular
// file, any extension" instead.
package conformance

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ManifestFile is the fixed filename a conformance directory's checksum
// manifest is written under -- unlike internal/eval's checksums.yaml
// (chosen per-corpus by its caller), every conformance directory uses the
// same name so a directory is self-describing: "does it have a MANIFEST"
// is the acc line's own literal question ("testdata/conformance/{...}/
// exist with a MANIFEST of sha256s").
const ManifestFile = "MANIFEST"

// Manifest maps a fixture file's basename (relative to the directory it
// pins) to its sha256 hex digest.
type Manifest map[string]string

// ChecksumFile returns the sha256 hex digest of a file's bytes.
func ChecksumFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("conformance: checksum %s: %w", path, err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// computeManifest hashes every regular, non-hidden file directly under
// dir except the manifest file itself (ManifestFile) and any *_test.go /
// *.go generator script (a directory may carry its own
// "//go:build ignore" regeneration command alongside its fixtures, the
// same way evals/corpora/<corpus>/gen_manifest.go sits beside the labels
// it pins -- that script is source, not a fixture, and must not need to
// pin its own checksum). Subdirectories are not descended into: each
// protocol's fixtures live flat under its own testdata/conformance/<name>/
// directory.
func computeManifest(dir string) (Manifest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("conformance: read dir %s: %w", dir, err)
	}
	m := make(Manifest)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == ManifestFile || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".go") {
			continue
		}
		sum, err := ChecksumFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		m[name] = sum
	}
	return m, nil
}

// WriteManifest computes dir's manifest and writes it to
// dir/ManifestFile in a plain "<sha256>  <filename>" line format (two
// spaces, matching sha256sum/shasum's own "binary mode" digest line) --
// sorted by filename so the file diffs cleanly and can also be checked by
// hand with `shasum -a 256 -c MANIFEST` from inside dir. This is the tool
// an operator runs after a DELIBERATE fixture change to re-pin the
// manifest; VerifyManifest is what CI runs to catch an UNDECLARED one.
func WriteManifest(dir string) error {
	m, err := computeManifest(dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		fmt.Fprintf(&b, "%s  %s\n", m[name], name)
	}
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("conformance: write manifest for %s: %w", dir, err)
	}
	return nil
}

// LoadManifest reads and parses dir/ManifestFile.
func LoadManifest(dir string) (Manifest, error) {
	path := filepath.Join(dir, ManifestFile)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("conformance: read manifest %s: %w", path, err)
	}
	m := make(Manifest)
	for i, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "  ", 2)
		if len(fields) != 2 {
			return nil, fmt.Errorf("conformance: %s line %d: malformed manifest line %q", path, i+1, line)
		}
		m[fields[1]] = fields[0]
	}
	return m, nil
}

// VerifyManifest checks every regular file under dir (per computeManifest's
// own rules -- ManifestFile and *.go generator scripts excluded) against
// the checksums recorded in dir/MANIFEST. Three things all count as "a
// fixture changed without its manifest entry": a file whose content no
// longer matches its recorded checksum, a fixture file on disk with no
// manifest entry (an unpinned addition), and a manifest entry naming a
// file that no longer exists (an unpinned removal). Any of these returns
// a non-nil error naming every offending file.
func VerifyManifest(dir string) error {
	want, err := LoadManifest(dir)
	if err != nil {
		return err
	}
	got, err := computeManifest(dir)
	if err != nil {
		return err
	}

	var problems []string
	for name, gotSum := range got {
		wantSum, ok := want[name]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s: present on disk, no manifest entry (unpinned addition)", name))
		case wantSum != gotSum:
			problems = append(problems, fmt.Sprintf("%s: checksum mismatch (manifest %s, actual %s) -- fixture changed without updating the manifest", name, wantSum, gotSum))
		}
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			problems = append(problems, fmt.Sprintf("%s: manifest entry names a file that no longer exists (unpinned removal)", name))
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("conformance: manifest verification failed for %s:\n%s", dir, strings.Join(problems, "\n"))
	}
	return nil
}
