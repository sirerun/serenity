package eval

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Manifest pins each label file's content, keyed by its filename relative
// to the labels directory, to a sha256 hex digest -- ADR-005: "Labels are
// checksum-pinned; CI fails when a label file changes without its manifest
// entry."
type Manifest map[string]string

// ChecksumFile returns the sha256 hex digest of a file's bytes.
func ChecksumFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("eval: checksum %s: %w", path, err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// LoadManifest reads a checksum manifest YAML file.
func LoadManifest(path string) (Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("eval: read manifest %s: %w", path, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("eval: parse manifest %s: %w", path, err)
	}
	return m, nil
}

// WriteManifest computes the checksum of every *.yaml file directly under
// labelsDir (other than manifestPath itself, when it lives in labelsDir too
// -- ADR-005's example layout is evals/corpora/<corpus>/labels/checksums.yaml,
// alongside the labels it pins) and writes the resulting manifest to
// manifestPath. It is the tool a labeler runs after a DELIBERATE label
// change to re-pin the manifest; VerifyManifest is what CI runs to catch an
// UNDECLARED one.
func WriteManifest(labelsDir, manifestPath string) error {
	m, err := computeManifest(labelsDir, manifestPath)
	if err != nil {
		return err
	}
	b, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("eval: marshal manifest: %w", err)
	}
	if err := os.WriteFile(manifestPath, b, 0o644); err != nil {
		return fmt.Errorf("eval: write manifest %s: %w", manifestPath, err)
	}
	return nil
}

// computeManifest hashes every *.yaml file directly under labelsDir, except
// the manifest file itself (identified by basename, since the manifest
// conventionally lives inside the directory it pins) -- otherwise the
// manifest would need to include its own checksum from before it was
// written, which is unsatisfiable.
func computeManifest(labelsDir, manifestPath string) (Manifest, error) {
	entries, err := os.ReadDir(labelsDir)
	if err != nil {
		return nil, fmt.Errorf("eval: read labels dir %s: %w", labelsDir, err)
	}
	// Only exclude the manifest by name when it actually lives inside the
	// directory being scanned -- a manifest stored elsewhere must not
	// accidentally shadow a same-named label file.
	var manifestName string
	if filepath.Clean(filepath.Dir(manifestPath)) == filepath.Clean(labelsDir) {
		manifestName = filepath.Base(manifestPath)
	}
	m := make(Manifest)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" || e.Name() == manifestName {
			continue
		}
		sum, err := ChecksumFile(filepath.Join(labelsDir, e.Name()))
		if err != nil {
			return nil, err
		}
		m[e.Name()] = sum
	}
	return m, nil
}

// VerifyManifest checks every *.yaml label file under labelsDir against the
// checksums recorded in manifestPath. Three things all count as "a label
// file changed without its checksum" per ADR-005: a file whose content no
// longer matches its recorded checksum, a label file on disk with no
// manifest entry (an unpinned addition), and a manifest entry naming a file
// that no longer exists (an unpinned removal). Any of these returns a
// non-nil error naming every offending file.
func VerifyManifest(labelsDir, manifestPath string) error {
	want, err := LoadManifest(manifestPath)
	if err != nil {
		return err
	}
	got, err := computeManifest(labelsDir, manifestPath)
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
			problems = append(problems, fmt.Sprintf("%s: checksum mismatch (manifest %s, actual %s) -- label changed without updating the manifest", name, wantSum, gotSum))
		}
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			problems = append(problems, fmt.Sprintf("%s: manifest entry names a file that no longer exists (unpinned removal)", name))
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("eval: manifest verification failed:\n%s", strings.Join(problems, "\n"))
	}
	return nil
}
