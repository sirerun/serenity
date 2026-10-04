//go:build portableplan

// Package portableplan_test exercises pinned interchange examples, not a memory
// writer, authority issuer or scheduler. Run this explicitly with -tags portableplan.
package portableplan_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const contractVersion = "0.0.1"
const contractRevision = "16b66e5eedf20d52e72928bb56a0c19e391e8ce9"
const contractDigest = "sha256:7582512f122d2f2a9c4461facc7541c9887053f137260d6ebe9c6dea611d039d"

type manifestEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type fixtureCase struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Valid bool   `json:"valid"`
	Rule  string `json:"rule"`
}

func readJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, value); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func verifiedBundle(root string) error {
	data, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		return err
	}
	var manifest struct {
		Version string          `json:"contractVersion"`
		Status  string          `json:"status"`
		Digest  string          `json:"contractDigest"`
		Files   []manifestEntry `json:"files"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	if manifest.Version != contractVersion || manifest.Status != "frozen" || manifest.Digest != contractDigest || len(manifest.Files) != 66 {
		return fmt.Errorf("unexpected frozen contract identity")
	}
	seen := make(map[string]bool)
	for _, entry := range manifest.Files {
		if !filepath.IsLocal(entry.Path) || strings.Contains(entry.Path, "\\") || filepath.ToSlash(filepath.Clean(entry.Path)) != entry.Path || seen[entry.Path] {
			return fmt.Errorf("noncanonical or duplicate manifest path %q", entry.Path)
		}
		seen[entry.Path] = true
		path := filepath.Join(root, entry.Path)
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("nonregular contract file %q", entry.Path)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		digest := sha256.Sum256(content)
		if hex.EncodeToString(digest[:]) != entry.SHA256 {
			return fmt.Errorf("contract file digest mismatch: %s", entry.Path)
		}
	}
	sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
	var records []string
	for _, entry := range manifest.Files {
		records = append(records, entry.Path+"\x00"+entry.SHA256+"\n")
	}
	digest := sha256.Sum256([]byte(strings.Join(records, "")))
	if "sha256:"+hex.EncodeToString(digest[:]) != contractDigest {
		return fmt.Errorf("contract manifest digest mismatch")
	}
	return nil
}

func TestPinnedContractIdentity(t *testing.T) {
	var pin struct {
		Version  string `json:"contractVersion"`
		Revision string `json:"revision"`
		Digest   string `json:"contractDigest"`
	}
	readJSON(t, "pin.json", &pin)
	if pin.Version != contractVersion || pin.Revision != contractRevision || pin.Digest != contractDigest {
		t.Fatal("consumer pin does not match the owner-frozen contract")
	}
	if err := verifiedBundle(filepath.Join("testdata", "upstream")); err != nil {
		t.Fatal(err)
	}
}

// No schema reference can fetch a provider, filesystem fallback or remote URL.
type offlineLoader struct{}

func (offlineLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("offline conformance refuses external schema resource %s", url)
}

func pinnedSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	root := filepath.Join("testdata", "upstream")
	if err := verifiedBundle(root); err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	c.UseLoader(offlineLoader{})
	paths, err := filepath.Glob(filepath.Join(root, "*.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		object, ok := doc.(map[string]any)
		if !ok {
			t.Fatalf("schema %s is not an object", path)
		}
		id, ok := object["$id"].(string)
		if !ok {
			t.Fatalf("schema %s has no ID", path)
		}
		if err := c.AddResource(id, doc); err != nil {
			t.Fatal(err)
		}
	}
	sch, err := c.Compile("urn:wazi:plan:0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	return sch
}

func catalog(t *testing.T, root string) []fixtureCase {
	t.Helper()
	var catalog struct {
		Version  string        `json:"contractVersion"`
		Fixtures []fixtureCase `json:"fixtures"`
	}
	readJSON(t, filepath.Join(root, "catalog.json"), &catalog)
	if catalog.Version != contractVersion || len(catalog.Fixtures) == 0 {
		t.Fatal("empty or wrong-version fixture catalog")
	}
	seen := make(map[string]bool)
	for _, fixture := range catalog.Fixtures {
		if fixture.ID == "" || fixture.Rule == "" || seen[fixture.ID] || !filepath.IsLocal(fixture.Path) {
			t.Fatalf("invalid fixture catalog entry: %+v", fixture)
		}
		seen[fixture.ID] = true
	}
	return catalog.Fixtures
}

func TestValidExamplesMatchPinnedStructuralSchema(t *testing.T) {
	sch := pinnedSchema(t)
	for _, root := range []string{filepath.Join("testdata", "upstream", "fixtures"), filepath.Join("testdata", "serenity")} {
		for _, fixture := range catalog(t, root) {
			if !fixture.Valid {
				continue // Negative mappings may have valid shape; the steward checks semantics.
			}
			t.Run(filepath.Base(root)+"/"+fixture.ID, func(t *testing.T) {
				path := fixture.Path
				if filepath.Base(root) == "fixtures" {
					path = strings.TrimPrefix(path, "fixtures/")
				}
				raw, err := os.ReadFile(filepath.Join(root, path))
				if err != nil {
					t.Fatal(err)
				}
				value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
				if err != nil {
					t.Fatal(err)
				}
				if err := sch.Validate(value); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
