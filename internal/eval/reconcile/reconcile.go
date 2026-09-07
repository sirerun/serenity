// Package reconcile loads and scores plan T2.18's reconcile eval corpus
// (evals/corpora/reconcile/labels/, RFC 0001 SS16/SS17, UC-014/UC-045):
// golden (new claim, active-claim pool, expected verdict) rows.
//
// Unlike internal/eval/direction, which reference-scores a cached
// classifier fixture because the real check_plan needs a live model call,
// this package scores directly against the real production
// internal/reconcile.Detect -- T2.2's own doc names Detect a pure function
// ("no I/O, no mutation"), so there is no reason to freeze a cached
// predictions fixture the way a model-backed corpus needs to: every CI run
// calls the exact function that ships, with no fixture ever able to drift
// from what production code actually does.
package reconcile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/reconcile"
	"github.com/sirerun/serenity/internal/store"
)

// ManifestName is the checksum manifest's filename, kept alongside the
// label files it pins (internal/eval/checksum.go's convention:
// evals/corpora/<corpus>/labels/checksums.yaml).
const ManifestName = "checksums.yaml"

// ClaimFixture is one claim as a corpus row writes it: only the fields
// internal/reconcile.Detect and Candidates actually read (SubjectSlug,
// Predicate, Object, ValidFrom, State) -- Confidence/Visibility/SourceRef/
// Provenance play no role in routing and are left at their zero values.
// State defaults to "active" when omitted, since every fixture claim in
// this corpus is normally an active neighbor; a row that needs to exercise
// Candidates' own superseded-exclusion sets it explicitly.
type ClaimFixture struct {
	ID        string `yaml:"id"`
	Subject   string `yaml:"subject"`
	Predicate string `yaml:"predicate"`
	Object    string `yaml:"object"`
	ValidFrom string `yaml:"valid_from,omitempty"`
	State     string `yaml:"state,omitempty"`
}

// toDomain builds the domain.Claim Detect/Candidates actually consume.
// ObjectKey is derived via store.NormalizeKey exactly as production
// ingestion derives it (internal/ingest), rather than requiring a corpus
// author to hand-compute it -- the same class of fixture-authoring
// footgun internal/eval.Score's own objectHyphenRx comment documents
// avoiding for the ava corpus.
func (f ClaimFixture) toDomain() domain.Claim {
	state := domain.StateActive
	if f.State != "" {
		state = domain.State(f.State)
	}
	return domain.Claim{
		ID:          f.ID,
		SubjectSlug: f.Subject,
		Predicate:   f.Predicate,
		Object:      f.Object,
		ObjectKey:   store.NormalizeKey(f.Object),
		Confidence:  0.9,
		ValidFrom:   f.ValidFrom,
		State:       state,
		Family:      f.Predicate,
	}
}

// Row is one golden (new claim, active-claim pool, expected verdict)
// record. ExpectedVerdict is one of internal/reconcile's six Verdict
// string values (agree, neutral_additive, conflict, window_close, scoped,
// precept_conflict), stored as a plain string here so a mislabeled row
// (a typo, or a value outside the six) is a Score-time error rather than
// a silently-zero-valued Verdict from an unchecked YAML->Verdict cast.
type Row struct {
	ID              string         `yaml:"id"`
	NewClaim        ClaimFixture   `yaml:"new_claim"`
	Active          []ClaimFixture `yaml:"active"`
	ExpectedVerdict string         `yaml:"expected_verdict"`
	Rationale       string         `yaml:"rationale"`
}

// LoadRows reads every *.yaml file directly under dir other than
// ManifestName as one Row, sorted by filename for determinism -- the same
// shape internal/eval/direction.LoadRows uses for its own corpus.
func LoadRows(dir string) ([]Row, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Row{}, nil
		}
		return nil, fmt.Errorf("eval/reconcile: read labels dir %s: %w", dir, err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" || e.Name() == ManifestName {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	rows := make([]Row, 0, len(names))
	for _, name := range names {
		path := filepath.Join(dir, name)
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("eval/reconcile: read row file %s: %w", path, err)
		}
		var r Row
		if err := yaml.Unmarshal(b, &r); err != nil {
			return nil, fmt.Errorf("eval/reconcile: parse row file %s: %w", path, err)
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// Detect runs the real production internal/reconcile.Candidates then
// internal/reconcile.Detect against this row's fixtures -- the exact two
// calls internal/reconcile.Engine.Process itself makes, so scoring this
// corpus measures production routing behavior, not a re-derived
// approximation of it.
func (r Row) Detect() reconcile.Detection {
	newClaim := r.NewClaim.toDomain()
	active := make([]domain.Claim, 0, len(r.Active))
	for _, a := range r.Active {
		active = append(active, a.toDomain())
	}
	return reconcile.Detect(newClaim, reconcile.Candidates(newClaim, active))
}
