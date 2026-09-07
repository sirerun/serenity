package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestVerifyManifestGreenOnMatch(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.json", `{"a":1}`)
	writeFixture(t, dir, "b.json", `{"b":2}`)
	writeFixture(t, dir, "LICENSE", "MIT\n")

	if err := WriteManifest(dir); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	if err := VerifyManifest(dir); err != nil {
		t.Fatalf("VerifyManifest on an untouched fixture set: %v", err)
	}
}

// TestVerifyManifestDetectsTamper is the acc line's "CI fails if a
// transcript changes without its manifest entry", proved directly: pin a
// manifest, mutate a fixture afterward without re-pinning, and assert
// VerifyManifest catches it and names the exact file.
func TestVerifyManifestDetectsTamper(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.json", `{"a":1}`)
	writeFixture(t, dir, "b.json", `{"b":2}`)

	if err := WriteManifest(dir); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	// Tamper: change b.json's content AFTER the manifest was pinned,
	// without touching the manifest.
	writeFixture(t, dir, "b.json", `{"b":3}`)

	err := VerifyManifest(dir)
	if err == nil {
		t.Fatal("VerifyManifest did not detect the tampered fixture file")
	}
	if !strings.Contains(err.Error(), "b.json") || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("VerifyManifest error doesn't name the tampered file/reason: %v", err)
	}
}

func TestVerifyManifestDetectsUnpinnedAddition(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.json", `{"a":1}`)

	if err := WriteManifest(dir); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	writeFixture(t, dir, "c.json", `{"c":3}`) // added after pinning

	err := VerifyManifest(dir)
	if err == nil {
		t.Fatal("VerifyManifest did not detect the unpinned added file")
	}
	if !strings.Contains(err.Error(), "c.json") || !strings.Contains(err.Error(), "unpinned addition") {
		t.Fatalf("VerifyManifest error doesn't name the added file: %v", err)
	}
}

func TestVerifyManifestDetectsRemoval(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.json", `{"a":1}`)
	writeFixture(t, dir, "b.json", `{"b":2}`)

	if err := WriteManifest(dir); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}

	if err := os.Remove(filepath.Join(dir, "b.json")); err != nil {
		t.Fatal(err)
	}

	err := VerifyManifest(dir)
	if err == nil {
		t.Fatal("VerifyManifest did not detect the removed file")
	}
	if !strings.Contains(err.Error(), "b.json") || !strings.Contains(err.Error(), "unpinned removal") {
		t.Fatalf("VerifyManifest error doesn't name the removed file: %v", err)
	}
}

// TestWriteManifestExcludesGeneratorScripts proves a "//go:build ignore"
// regeneration command (e.g. gen_transcripts.go) living beside its own
// fixtures is source, not a pinned fixture -- it must never need its own
// checksum entry, or every edit to the generator itself would spuriously
// break VerifyManifest.
func TestWriteManifestExcludesGeneratorScripts(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.json", `{"a":1}`)
	writeFixture(t, dir, "gen_transcripts.go", "//go:build ignore\n\npackage main\n\nfunc main() {}\n")

	if err := WriteManifest(dir); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	m, err := LoadManifest(dir)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if _, ok := m["gen_transcripts.go"]; ok {
		t.Fatal("manifest pinned the generator script -- it should be excluded")
	}
	if _, ok := m["a.json"]; !ok {
		t.Fatal("manifest is missing the real fixture a.json")
	}

	// Editing the generator script afterward must not trip VerifyManifest.
	writeFixture(t, dir, "gen_transcripts.go", "//go:build ignore\n\npackage main\n\nfunc main() { println(\"changed\") }\n")
	if err := VerifyManifest(dir); err != nil {
		t.Fatalf("VerifyManifest flagged a generator-script edit as a fixture change: %v", err)
	}
}

func TestManifestFileIsShasumCompatible(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.json", `{"a":1}`)
	if err := WriteManifest(dir); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, ManifestFile))
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(string(b))
	sum, err := ChecksumFile(filepath.Join(dir, "a.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := sum + "  a.json"
	if line != want {
		t.Fatalf("MANIFEST line = %q, want %q (sha256sum -c / shasum -a 256 -c compatible format)", line, want)
	}
}
