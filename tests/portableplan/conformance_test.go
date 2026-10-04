//go:build portableplan

// Package portableplan_test exercises pinned interchange examples, not a memory
// writer, authority issuer or scheduler. Run this explicitly with -tags portableplan.
package portableplan_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

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
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !seen[rel] && rel != "manifest.json" {
			return fmt.Errorf("unlisted contract file %q", rel)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in pinned contract %q", rel)
		}
		return nil
	}); err != nil {
		return err
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

func TestPinnedContractRejectsTampering(t *testing.T) {
	for _, change := range []string{"altered-fixture", "extra-schema", "duplicate-path"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join("testdata", "upstream")
			err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				rel, err := filepath.Rel(source, path)
				if err != nil {
					return err
				}
				target := filepath.Join(root, rel)
				if entry.IsDir() {
					return os.MkdirAll(target, 0700)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				return os.WriteFile(target, data, 0600)
			})
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "altered-fixture":
				err = os.WriteFile(filepath.Join(root, "fixtures", "valid", "approved-local-alternative.json"), []byte("{}\n"), 0600)
			case "extra-schema":
				err = os.WriteFile(filepath.Join(root, "unlisted.schema.json"), []byte("{}\n"), 0600)
			case "duplicate-path":
				var manifest map[string]any
				readJSON(t, filepath.Join(root, "manifest.json"), &manifest)
				files := manifest["files"].([]any)
				files[1] = files[0]
				data, marshalErr := json.Marshal(manifest)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				err = os.WriteFile(filepath.Join(root, "manifest.json"), data, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := verifiedBundle(root); err == nil {
				t.Fatal("tampered contract unexpectedly accepted")
			}
		})
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

type validatorReport struct {
	Version   string `json:"contractVersion"`
	Digest    string `json:"contractDigest"`
	Valid     *bool  `json:"valid"`
	Authority *bool  `json:"authorityAuthenticated"`
	Findings  []struct {
		Code string `json:"code"`
	} `json:"findings"`
}

func runValidator(t *testing.T, binary string, args ...string) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, args...).CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("validator timed out: %v", ctx.Err())
	}
	return output, err
}

func TestOwningSemanticConformance(t *testing.T) {
	binary := os.Getenv("WAZI_PLAN_VALIDATOR")
	if !filepath.IsAbs(binary) {
		t.Fatal("WAZI_PLAN_VALIDATOR must name an absolute owner-qualified offline binary")
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(data)
	t.Logf("validator artifact SHA256: %x; source qualification must accompany this result", digest)
	output, err := runValidator(t, binary, "version")
	if err != nil {
		t.Fatalf("validator version: %v: %s", err, output)
	}
	var version validatorReport
	if err := json.Unmarshal(output, &version); err != nil {
		t.Fatalf("validator version JSON: %v", err)
	}
	if version.Version != contractVersion || version.Digest != contractDigest || version.Authority == nil || *version.Authority {
		t.Fatalf("unexpected validator identity/authority report: %s", output)
	}
	for _, root := range []string{"testdata/upstream/fixtures", "testdata/serenity"} {
		for _, fixture := range catalog(t, root) {
			t.Run(filepath.Base(root)+"/"+fixture.ID, func(t *testing.T) {
				path := fixture.Path
				if strings.HasSuffix(root, "fixtures") {
					path = strings.TrimPrefix(path, "fixtures/")
				}
				path, err := filepath.Abs(filepath.Join(root, path))
				if err != nil {
					t.Fatal(err)
				}
				var document map[string]any
				readJSON(t, path, &document)
				output, runErr := runValidator(t, binary, "validate", "--contract-digest", contractDigest, path)
				var report validatorReport
				if err := json.Unmarshal(output, &report); err != nil {
					t.Fatalf("validator report JSON: %v: %s", err, output)
				}
				if report.Version != contractVersion || report.Digest != contractDigest || report.Valid == nil || report.Authority == nil || *report.Authority {
					t.Fatalf("unexpected report identity/authority: %s", output)
				}
				if *report.Valid != fixture.Valid {
					t.Fatalf("%s: expected valid=%t: %s", fixture.Rule, fixture.Valid, output)
				}
				if fixture.Valid {
					if runErr != nil || len(report.Findings) != 0 {
						t.Fatalf("valid fixture failed: %v: %s", runErr, output)
					}
					return
				}
				var exitErr *exec.ExitError
				if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != 1 || len(report.Findings) == 0 {
					t.Fatalf("invalid fixture lacked a validation rejection: %v: %s", runErr, output)
				}
				for _, finding := range report.Findings {
					switch finding.Code {
					case "", "schema_compile", "schema_init", "contract_digest_mismatch", "contract_unavailable", "duplicate_json_member", "invalid_json":
						t.Fatalf("infrastructure/input failure is not conformance: %s", output)
					}
				}
			})
		}
	}
}
