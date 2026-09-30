// The checksummed adversarial corpus is driven through the real extractor
// and composer with scripted providers. Its hostile structured candidates
// must be rejected before they can become claim filenames, frontmatter or
// provider context. The AST checks additionally cover every production Go
// source root.
package gate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/compose"
	"github.com/sirerun/serenity/internal/config"
	"gopkg.in/yaml.v3"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/eval"
	"github.com/sirerun/serenity/internal/extract"
	"github.com/sirerun/serenity/internal/extract/chunk"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/router"
	"github.com/sirerun/serenity/internal/store"
)

// adversarialCorpusDir is repo-root-relative (see repoRoot, defined in
// filefirst_test.go, shared by this package's tests).
const adversarialCorpusDir = "evals/corpora/adversarial/documents"

// legalConnectorKinds are the domain.Source.Kind / connector.RawItem.Kind
// values the shipped connectors emit (internal/connector/imap, .../file,
// .../gitrepo) -- the "kinds" the plan's acc line requires the corpus to
// span at least three of.
var legalConnectorKinds = map[string]bool{
	"email":    true,
	"file":     true,
	"git_repo": true,
}

// adversarialDoc mirrors one evals/corpora/adversarial/documents/*.yaml
// fixture. See evals/corpora/adversarial/README.md for the schema.
type adversarialDoc struct {
	Kind                      string   `yaml:"kind"`
	URI                       string   `yaml:"uri"`
	AttackVector              string   `yaml:"attack_vector"`
	Body                      string   `yaml:"body"`
	FabricatedPredicates      []string `yaml:"fabricated_predicates"`
	CamouflagedRealPredicates []string `yaml:"camouflaged_real_predicates"`

	file string // basename, for error messages
}

// loadAdversarialCorpus reads every *.yaml document (excluding the
// checksums.yaml manifest) directly under dir, in sorted filename order.
func loadAdversarialCorpus(dir string) ([]adversarialDoc, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" || e.Name() == "checksums.yaml" {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	docs := make([]adversarialDoc, 0, len(names))
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		var d adversarialDoc
		if err := yaml.Unmarshal(b, &d); err != nil {
			return nil, err
		}
		d.file = name
		docs = append(docs, d)
	}
	return docs, nil
}

// TestAdversarialCorpusManifestPinned is the checksum-manifest half of
// good practice established by T1.13/T1.14: a document that changes
// without its checksum being re-pinned fails CI.
func TestAdversarialCorpusManifestPinned(t *testing.T) {
	dir := filepath.Join(repoRoot, adversarialCorpusDir)
	manifest := filepath.Join(dir, "checksums.yaml")
	if err := eval.VerifyManifest(dir, manifest); err != nil {
		t.Fatalf("adversarial corpus checksum manifest verification failed: %v", err)
	}
}

// TestAdversarialCorpusShape asserts the plan T1.20 acc line's minimums:
// >= 15 documents, spanning at least the three connector kinds this repo
// models (email, file, git_repo), every document well-formed.
func TestAdversarialCorpusShape(t *testing.T) {
	docs, err := loadAdversarialCorpus(filepath.Join(repoRoot, adversarialCorpusDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) < 15 {
		t.Fatalf("want >= 15 adversarial documents, got %d", len(docs))
	}

	byKind := map[string]int{}
	for _, d := range docs {
		if d.Kind == "" || !legalConnectorKinds[d.Kind] {
			t.Errorf("%s: kind %q is not one of the modeled connector kinds (email, file, git_repo)", d.file, d.Kind)
		}
		if d.URI == "" {
			t.Errorf("%s: missing uri", d.file)
		}
		if d.AttackVector == "" {
			t.Errorf("%s: missing attack_vector", d.file)
		}
		if d.Body == "" {
			t.Errorf("%s: missing body", d.file)
		}
		if len(d.FabricatedPredicates) == 0 {
			t.Errorf("%s: no fabricated_predicates declared", d.file)
		}
		byKind[d.Kind]++
	}
	for _, kind := range []string{"email", "file", "git_repo"} {
		if byKind[kind] == 0 {
			t.Errorf("corpus has zero documents of kind %q; want >= 3 distinct kinds represented", kind)
		}
	}
}

type scriptedGateProvider struct {
	name         string
	modelVersion string
	response     string
	calls        int
	prompts      []string
}

func (p *scriptedGateProvider) Name() string         { return p.name }
func (p *scriptedGateProvider) ModelVersion() string { return p.modelVersion }
func (p *scriptedGateProvider) Send(_ context.Context, prompt string) (router.Response, error) {
	p.calls++
	p.prompts = append(p.prompts, prompt)
	return router.Response{Text: p.response}, nil
}

type scriptedGateLedger struct{}

func (scriptedGateLedger) Record(context.Context, router.SpendEntry) error { return nil }

type emptyGateSearchStore struct{}

func (emptyGateSearchStore) SearchVectors(context.Context, string, []float32, int) ([]index.Hit, error) {
	return nil, nil
}
func (emptyGateSearchStore) SearchFTS(context.Context, string, int) ([]index.Hit, error) {
	return nil, nil
}
func (emptyGateSearchStore) VectorFor(context.Context, string, string) ([]float32, bool, error) {
	return nil, false, nil
}

// TestAdversarialCorpusRealExtractAndCompose proves every checked-in corpus
// document crosses Extractor.Extract and Composer.Ask. A scripted extraction
// provider emits both a valid observation and hostile candidates whose slug,
// predicate, URL and metadata payloads must not reach storage or composition.
func TestAdversarialCorpusRealExtractAndCompose(t *testing.T) {
	docs, err := loadAdversarialCorpus(filepath.Join(repoRoot, adversarialCorpusDir))
	if err != nil {
		t.Fatal(err)
	}
	const model = "adversarial-script@v1"
	for _, doc := range docs {
		doc := doc
		t.Run(doc.file, func(t *testing.T) {
			tempRoot := t.TempDir()
			unsafeMarker := "UNTRUSTED-" + strings.TrimSuffix(doc.file, ".yaml")
			wire, err := json.Marshal(map[string]any{"observations": []map[string]any{
				{"subject": "../../" + strings.TrimSuffix(doc.file, ".yaml"), "predicate": "said", "object": unsafeMarker + " https://attacker.invalid/ frontmatter:precept", "confidence": 0.99},
				{"subject": "corpus-record", "predicate": doc.FabricatedPredicates[0], "object": "fabricated predicate", "confidence": 0.99},
				{"subject": "corpus-record", "predicate": "said", "object": "Reviewed source material safely", "confidence": 0.99},
			}})
			if err != nil {
				t.Fatal(err)
			}
			extractProvider := &scriptedGateProvider{name: "adversarial-extract", modelVersion: model, response: string(wire)}
			extractRouter := router.New(map[router.Tier]router.Provider{router.TierLocalCheap: extractProvider}, scriptedGateLedger{})
			extractor := extract.New(extractRouter, model, nil, nil)
			sourceHash := sha256.Sum256([]byte(doc.Body))
			result, err := extractor.Extract(context.Background(), hex.EncodeToString(sourceHash[:]), false, []chunk.Chunk{{
				Span: chunk.Span{Start: 0, End: len(doc.Body)}, Text: doc.Body,
			}}, router.Budget{})
			if err != nil {
				t.Fatalf("extract corpus document: %v", err)
			}
			if extractProvider.calls != 1 || len(extractProvider.prompts) != 1 || !strings.Contains(extractProvider.prompts[0], doc.Body) {
				t.Fatalf("scripted extractor did not receive the real corpus document (calls=%d)", extractProvider.calls)
			}
			if result.Rejected < 2 || len(result.Ready) != 1 || result.Ready[0].SubjectSlug != "corpus-record" {
				t.Fatalf("hostile candidates escaped extraction: rejected=%d ready=%+v", result.Rejected, result.Ready)
			}

			claim := domain.Claim{
				ID: "corpus-safe", SubjectSlug: result.Ready[0].SubjectSlug,
				Predicate: result.Ready[0].Predicate, Family: result.Ready[0].Predicate,
				Object: result.Ready[0].Object, Confidence: result.Ready[0].Confidence,
				State: domain.StateActive, Visibility: domain.VisibilityShared,
				Provenance: domain.Provenance{Actor: "human", ObservedAt: time.Now().UTC()},
			}
			if err := store.NewShardStore(tempRoot).Append(claim); err != nil {
				t.Fatalf("persist only accepted observation: %v", err)
			}
			composeProvider := &scriptedGateProvider{
				name: "adversarial-compose", modelVersion: model,
				response: "Reviewed source material [claim:corpus-safe].",
			}
			composeRouter := router.New(map[router.Tier]router.Provider{router.TierJudgment: composeProvider}, scriptedGateLedger{})
			composer := compose.New(tempRoot, config.Default(), emptyGateSearchStore{}, nil, composeRouter, model)
			answer, err := composer.Ask(context.Background(), "source material")
			if err != nil {
				t.Fatalf("compose accepted corpus evidence: %v", err)
			}
			if composeProvider.calls != 1 || !strings.Contains(composeProvider.prompts[0], "corpus-safe") {
				t.Fatalf("scripted composer did not receive the extracted claim (calls=%d)", composeProvider.calls)
			}
			if len(answer.Citations) != 1 || answer.Citations[0].ClaimID != "corpus-safe" {
				t.Fatalf("composition did not cite the safe extracted claim: %+v", answer.Citations)
			}
			if strings.Contains(composeProvider.prompts[0], unsafeMarker) || strings.Contains(composeProvider.prompts[0], doc.URI) {
				t.Fatalf("rejected filename, URL, or metadata escaped into composition prompt for %s", doc.file)
			}
			if _, err := os.Stat(filepath.Join(tempRoot, ".dira")); !os.IsNotExist(err) {
				t.Fatalf("corpus input minted a precept directory: stat error=%v", err)
			}
			err = filepath.WalkDir(tempRoot, func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if entry.IsDir() {
					return nil
				}
				if strings.Contains(path, strings.TrimSuffix(doc.file, ".yaml")) {
					return fmt.Errorf("corpus filename escaped into a stored path: %s", path)
				}
				b, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if strings.Contains(string(b), unsafeMarker) || strings.Contains(string(b), doc.URI) {
					return fmt.Errorf("rejected URL or frontmatter payload escaped into %s", path)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestASTSafetyScannersCoverEverySourceRoot(t *testing.T) {
	for _, want := range []string{"internal", "pkg", "cmd"} {
		found := false
		for _, got := range astScanSourceRoots {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("AST scanners do not list source root %q: %v", want, astScanSourceRoots)
		}
	}
	for _, sourceRoot := range []string{"pkg", "cmd"} {
		t.Run(sourceRoot, func(t *testing.T) {
			tmp := t.TempDir()
			writeFixture(t, tmp, filepath.Join(sourceRoot, "injected.go"), disallowedCallerSrc)
			violations, err := scanForViolations(tmp)
			if err != nil {
				t.Fatal(err)
			}
			if len(violations) != 1 || violations[0].file != filepath.ToSlash(filepath.Join(sourceRoot, "injected.go")) {
				t.Fatalf("file-first AST scanner did not catch %s: %+v", sourceRoot, violations)
			}
			tmp = t.TempDir()
			writeFixture(t, tmp, filepath.Join(sourceRoot, "inject.go"), diraInjectSrc)
			diraViolations, err := scanForDiraWrites(tmp)
			if err != nil {
				t.Fatal(err)
			}
			if len(diraViolations) != 1 || diraViolations[0].file != filepath.ToSlash(filepath.Join(sourceRoot, "inject.go")) {
				t.Fatalf("precept immutability scanner did not catch %s: %+v", sourceRoot, diraViolations)
			}
		})
	}
}

// TestSeededVulnerableModeIsCaughtByRealExtractor reruns the extractor's
// traversal regression in a test subprocess where its model-boundary slug
// predicate is disabled. This makes the release negative run reproduce an
// actual escaping observation rather than passing because of a synthetic
// Fatalf in the gate itself.
func TestSeededVulnerableModeIsCaughtByRealExtractor(t *testing.T) {
	if os.Getenv("SERENITY_ADVERSARIAL_VULNERABLE") != "1" {
		return
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "-count=1", "-run", "^TestAdversarialSubjectSlugContainment$", "./internal/extract")
	cmd.Dir = root
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("validator-disabled extractor unexpectedly contained the traversal subject:\n%s", output)
	}
	if !strings.Contains(string(output), "unsafe subject escaped model boundary") {
		t.Fatalf("seeded vulnerable run failed for an unrelated reason:\n%s", output)
	}
	t.Fatalf("seeded validator-disabled extractor reproduced the traversal escape:\n%s", output)
}

// TestAdversarialCorpusIngestGateFlagsEveryDocument is (a) from the
// package doc comment: for every corpus document, synthesize the same
// shape of hypothetical extractor T3.12's adversarialExtractorSrcTemplate
// uses (an internal/extract source that writes the document's raw text to
// .dira/ as if it believed it was a confirmed precept) and assert the
// precept-immutability AST gate catches it. This is the "zero writes
// under .dira/" half of the acc line: proven structurally, over the real
// corpus, using the exact mechanism already merged for this purpose.
func TestAdversarialCorpusIngestGateFlagsEveryDocument(t *testing.T) {
	docs, err := loadAdversarialCorpus(filepath.Join(repoRoot, adversarialCorpusDir))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range docs {
		d := d
		t.Run(d.file, func(t *testing.T) {
			tmp := t.TempDir()
			writeFixture(t, tmp, "internal/extract/inject.go", adversarialExtractorSrcTemplate(d.Body))

			violations, err := scanForDiraWrites(tmp)
			if err != nil {
				t.Fatal(err)
			}
			if len(violations) < 1 {
				t.Fatalf("adversarial document %s was NOT caught by the precept-immutability gate", d.file)
			}
		})
	}
}

// TestAdversarialCorpusDiraHashUnchanged mirrors T3.12's
// dira_hash_unchanged_after_processing subtest, over the real corpus: a
// pre-existing, legitimate .dira/ tree's hash is unchanged after every
// document in the corpus has been run through the gate's detection path,
// each in its own isolated temp tree.
func TestAdversarialCorpusDiraHashUnchanged(t *testing.T) {
	docs, err := loadAdversarialCorpus(filepath.Join(repoRoot, adversarialCorpusDir))
	if err != nil {
		t.Fatal(err)
	}

	fixtureRoot := t.TempDir()
	writeFixture(t, fixtureRoot, ".dira/entries/existing.md", "# existing precept\n\ndecision: pre-existing, legitimate entry\n")
	before := hashTree(t, filepath.Join(fixtureRoot, ".dira"))

	for _, d := range docs {
		tmp := t.TempDir()
		writeFixture(t, tmp, "internal/extract/inject.go", adversarialExtractorSrcTemplate(d.Body))
		if _, err := scanForDiraWrites(tmp); err != nil {
			t.Fatalf("processing adversarial document %s: %v", d.file, err)
		}
	}

	after := hashTree(t, filepath.Join(fixtureRoot, ".dira"))
	if before != after {
		t.Fatalf(".dira/ hash changed after processing the adversarial corpus: before=%s after=%s", before, after)
	}
}

// TestAdversarialCorpusFabricatedPredicatesRejected is (b) from the
// package doc comment: every fabricated_predicates token declared by any
// corpus document is fed through internal/store's real controlled-
// vocabulary enforcement (the same check ShardStore.Append and
// FenceWriter.RenderEntity apply in production) and must be rejected as
// store.ErrUnknownPredicate. This is the "zero predicates outside the
// vocabulary" half of the acc line.
func TestAdversarialCorpusFabricatedPredicatesRejected(t *testing.T) {
	docs, err := loadAdversarialCorpus(filepath.Join(repoRoot, adversarialCorpusDir))
	if err != nil {
		t.Fatal(err)
	}

	seen := 0
	for _, d := range docs {
		for _, pred := range d.FabricatedPredicates {
			seen++
			s := store.NewShardStore(t.TempDir())
			claim := domain.Claim{
				SubjectSlug: "corpus-test-subject",
				Predicate:   pred,
				Family:      pred,
				Object:      "adversarial-object",
				State:       domain.StateActive,
			}
			err := s.Append(claim)
			if !errors.Is(err, store.ErrUnknownPredicate) {
				t.Errorf("%s: fabricated predicate %q was NOT rejected as unknown (err=%v) -- it may have leaked into the controlled vocabulary", d.file, pred, err)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no fabricated_predicates found across the corpus -- corpus loading is broken")
	}
}

// TestAdversarialCorpusCamouflagedRealPredicatesAreGenuine is the control
// for the test above: it proves the vocabulary check discriminates rather
// than rejecting everything, by confirming every
// camouflaged_real_predicates entry (a genuine family a document
// disguises its fabricated instruction alongside) is actually accepted.
func TestAdversarialCorpusCamouflagedRealPredicatesAreGenuine(t *testing.T) {
	docs, err := loadAdversarialCorpus(filepath.Join(repoRoot, adversarialCorpusDir))
	if err != nil {
		t.Fatal(err)
	}

	seen := 0
	for _, d := range docs {
		for _, pred := range d.CamouflagedRealPredicates {
			seen++
			s := store.NewShardStore(t.TempDir())
			claim := domain.Claim{
				SubjectSlug: "corpus-test-subject",
				Predicate:   pred,
				Family:      pred,
				Object:      "genuine-object",
				State:       domain.StateActive,
			}
			if err := s.Append(claim); err != nil {
				t.Errorf("%s: camouflaged real predicate %q was rejected (err=%v) -- it should be genuine controlled vocabulary", d.file, pred, err)
			}
		}
	}
	if seen == 0 {
		t.Fatal("no camouflaged_real_predicates found across the corpus -- the contrast case documents are missing their field")
	}
}
