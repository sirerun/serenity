package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyManifestGreenOnMatch(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.yaml", "span: a\nexpected:\n  predicate: works_at\n  object: acme\nlabeler: model-a\nadjudicated: false\n")
	writeFixture(t, dir, "b.yaml", "span: b\nexpected:\n  predicate: prefers\n  object: tea\nlabeler: model-b\nadjudicated: true\n")

	manifestPath := filepath.Join(dir, "checksums.yaml")
	if err := WriteManifest(dir, manifestPath); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	if err := VerifyManifest(dir, manifestPath); err != nil {
		t.Fatalf("VerifyManifest on an untouched fixture: %v", err)
	}
}

// TestVerifyManifestDetectsTamper is the acc line's "the test fails if a
// label file changes without its checksum": it pins a manifest, mutates a
// label file's content afterward without re-pinning, and asserts
// VerifyManifest catches it and names the exact file.
func TestVerifyManifestDetectsTamper(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.yaml", "span: a\nexpected:\n  predicate: works_at\n  object: acme\nlabeler: model-a\nadjudicated: false\n")
	writeFixture(t, dir, "b.yaml", "span: b\nexpected:\n  predicate: prefers\n  object: tea\nlabeler: model-b\nadjudicated: true\n")

	manifestPath := filepath.Join(dir, "checksums.yaml")
	if err := WriteManifest(dir, manifestPath); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	// Tamper: change b.yaml's object AFTER the manifest was pinned, without
	// touching the manifest.
	writeFixture(t, dir, "b.yaml", "span: b\nexpected:\n  predicate: prefers\n  object: coffee\nlabeler: model-b\nadjudicated: true\n")

	err := VerifyManifest(dir, manifestPath)
	if err == nil {
		t.Fatal("VerifyManifest did not detect the tampered label file")
	}
	if !strings.Contains(err.Error(), "b.yaml") {
		t.Fatalf("error %q does not name the tampered file b.yaml", err.Error())
	}
	if strings.Contains(err.Error(), "a.yaml") {
		t.Fatalf("error %q flags the untouched file a.yaml", err.Error())
	}
}

func TestVerifyManifestDetectsUnpinnedAddition(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.yaml", "span: a\nexpected:\n  predicate: works_at\n  object: acme\nlabeler: model-a\nadjudicated: false\n")
	manifestPath := filepath.Join(dir, "checksums.yaml")
	if err := WriteManifest(dir, manifestPath); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	// A new label file lands after the manifest was pinned, without
	// regenerating it.
	writeFixture(t, dir, "c.yaml", "span: c\nexpected:\n  predicate: works_at\n  object: beta\nlabeler: model-a\nadjudicated: false\n")

	err := VerifyManifest(dir, manifestPath)
	if err == nil {
		t.Fatal("VerifyManifest did not detect the unpinned added file")
	}
	if !strings.Contains(err.Error(), "c.yaml") {
		t.Fatalf("error %q does not name the added file c.yaml", err.Error())
	}
}

func TestVerifyManifestDetectsRemoval(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.yaml", "span: a\nexpected:\n  predicate: works_at\n  object: acme\nlabeler: model-a\nadjudicated: false\n")
	writeFixture(t, dir, "b.yaml", "span: b\nexpected:\n  predicate: prefers\n  object: tea\nlabeler: model-b\nadjudicated: true\n")
	manifestPath := filepath.Join(dir, "checksums.yaml")
	if err := WriteManifest(dir, manifestPath); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	if err := os.Remove(filepath.Join(dir, "b.yaml")); err != nil {
		t.Fatalf("remove fixture: %v", err)
	}

	err := VerifyManifest(dir, manifestPath)
	if err == nil {
		t.Fatal("VerifyManifest did not detect the removed file")
	}
	if !strings.Contains(err.Error(), "b.yaml") {
		t.Fatalf("error %q does not name the removed file b.yaml", err.Error())
	}
}

func TestChecksumFileDeterministic(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.yaml", "span: a\nexpected:\n  predicate: works_at\n  object: acme\n")
	path := filepath.Join(dir, "a.yaml")

	sum1, err := ChecksumFile(path)
	if err != nil {
		t.Fatalf("ChecksumFile: %v", err)
	}
	sum2, err := ChecksumFile(path)
	if err != nil {
		t.Fatalf("ChecksumFile: %v", err)
	}
	if sum1 != sum2 {
		t.Fatalf("ChecksumFile is non-deterministic: %s vs %s", sum1, sum2)
	}
	if len(sum1) != 64 { // sha256 hex digest length.
		t.Fatalf("ChecksumFile returned %q, want a 64-char hex digest", sum1)
	}
}
