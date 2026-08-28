// Adversarial corpus ingest test (RFC 0001 §14/§16, plan T1.20). The real
// deliverable T3.12's inline adversarialDocs slice explicitly deferred:
// evals/corpora/adversarial/documents/ is a genuinely reusable,
// checksummed corpus of prompt-injection / precept-fabrication documents
// across email/file/git_repo kinds. This file is that corpus's consumer.
//
// Scoping note (disclosed per plan T1.20): there is no complete, wired
// ingest pipeline yet -- extraction (T1.8) and the observation-to-claim
// write path (T1.9) are un-landed siblings of this task. This test does
// not run the corpus through a real connector or extractor; it proves the
// two enforcement mechanisms a real pipeline would sit behind actually
// catch what this corpus contains, using the same technique T3.12 already
// established: (a) the precept-immutability AST gate (this package) would
// catch any code path that tried to persist a document's content as a
// .dira write, and (b) internal/store's real controlled-vocabulary
// enforcement (store.ErrUnknownPredicate, seeded from T0.8's
// internal/config) rejects every predicate-like token this corpus tries
// to smuggle in. When T1.8/T1.9 land a real extraction-to-claim path, a
// follow-up should route this corpus through it directly instead.
package gate

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/eval"
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
