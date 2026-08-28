// Package direction loads and reference-scores the T3.13 DIRECTION eval
// corpus (RFC 0001 SS8.3, SS16; ADR 010): golden (plan, constraint set,
// expected verdict) rows under evals/corpora/direction/labels/, one YAML
// file per row.
//
// This is not the production check_plan matcher -- that is
// internal/direction/check's Matcher (T3.5 structured stage 1) and
// Classifier (T3.6 free-text stage 2), both landed and reachable via
// `serenity check` (T3.7). Evaluate here is a small, independent reference
// implementation of the same deterministic stage-1 semantics (RFC SS8.3:
// match a structured action against a constraint's applies_when clause),
// used only to prove this corpus's own labels are internally consistent --
// an adversarial row whose declared expected_verdict doesn't actually
// follow from its declared actions and constraints would be exactly the
// "mislabeled adversarial row" failure mode the corpus exists to avoid.
// It stays independent of the real Matcher deliberately: this corpus's
// Row/Constraint shape (flat comparator-suffix params, see paramsSatisfy)
// is its own convention, not internal/direction's ledger-backed
// applies_when representation, so scoring the corpus never requires
// standing up a real dira ledger.
package direction

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ManifestName is the checksum manifest's filename, kept alongside the label
// files it pins -- the same layout internal/eval's checksum.go documents
// (evals/corpora/<corpus>/labels/checksums.yaml).
const ManifestName = "checksums.yaml"

// Verdicts are the four DIRECTION check_plan outcomes (RFC SS8.3, ADR 010).
// There is no fifth: an error other than "no model available" is a bug, not
// a verdict.
const (
	VerdictPass                    = "pass"
	VerdictViolated                = "violated"
	VerdictNoApplicableConstraints = "no_applicable_constraints"
	VerdictUnverified              = "unverified"
)

// ActionCall is a structured action as check_plan's actions[] parameter
// takes it (RFC SS8.3): one member of domain.ActionSet plus its parameters.
type ActionCall struct {
	Action string         `yaml:"action"`
	Params map[string]any `yaml:"params"`
}

// Constraint is one active constraint precept's applies_when clause plus the
// stored reasoning check_plan must surface verbatim on violation (RFC SS8.3,
// SS7.3). It mirrors a dira constraint entry without being one -- these rows
// never touch a real ledger.
type Constraint struct {
	PreceptID string         `yaml:"precept_id"`
	Action    string         `yaml:"action"`
	Params    map[string]any `yaml:"params"`
	WhyNot    string         `yaml:"why_not"`
	RevisitIf string         `yaml:"revisit_if"`
}

// Row is one golden (plan, constraint set, expected verdict) record (T3.13).
// Category is "matrix" (a plan x constraint combination) or "adversarial"
// (a plan that tries to talk the checker out of an actual violation);
// Labeler and Adjudicated follow ADR-005's labeling-provenance convention
// even though this corpus's rows are not extraction labels.
type Row struct {
	ID                            string       `yaml:"id"`
	Category                      string       `yaml:"category"`
	ActionDomain                  string       `yaml:"action_domain"`
	PlanText                      string       `yaml:"plan_text"`
	Actions                       []ActionCall `yaml:"actions"`
	Constraints                   []Constraint `yaml:"constraints"`
	ExpectedVerdict               string       `yaml:"expected_verdict"`
	ExpectedViolations            []string     `yaml:"expected_violations"`
	ExpectedConstraintsConsidered int          `yaml:"expected_constraints_considered"`
	Adversarial                   bool         `yaml:"adversarial"`
	AdversarialKind               string       `yaml:"adversarial_kind"`
	Rationale                     string       `yaml:"rationale"`
	Labeler                       string       `yaml:"labeler"`
	Adjudicated                   bool         `yaml:"adjudicated"`
}

// LoadRows reads every *.yaml file directly under dir other than
// ManifestName as one Row, sorted by filename for determinism -- the same
// shape as internal/eval.LoadLabels, adapted to this corpus's record type.
func LoadRows(dir string) ([]Row, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Row{}, nil
		}
		return nil, fmt.Errorf("direction: read labels dir %s: %w", dir, err)
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
			return nil, fmt.Errorf("direction: read row file %s: %w", path, err)
		}
		var r Row
		if err := yaml.Unmarshal(b, &r); err != nil {
			return nil, fmt.Errorf("direction: parse row file %s: %w", path, err)
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// Evaluate reference-scores one row's declared actions against its declared
// constraint set using the deterministic stage-1 semantics RFC SS8.3
// describes: a constraint "applies" when a plan action's name matches the
// constraint's applies_when action; it is "violated" when every one of the
// constraint's param conditions also holds. A row with no structured actions
// (free text only) is unverified, matching "no model available" / classifier
// confidence below the ADR 010 floor -- this package never runs a
// classifier, so free-text rows can only be golden-labeled unverified.
//
// It returns the verdict, the sorted list of violated precept ids, and the
// number of constraints considered (always len(row.Constraints), matching
// check_plan's no_applicable_constraints count).
func Evaluate(row Row) (verdict string, violations []string, constraintsConsidered int) {
	constraintsConsidered = len(row.Constraints)
	if len(row.Actions) == 0 {
		return VerdictUnverified, nil, constraintsConsidered
	}

	anyActionMatched := false
	for _, c := range row.Constraints {
		for _, a := range row.Actions {
			if a.Action != c.Action {
				continue
			}
			anyActionMatched = true
			if paramsSatisfy(a.Params, c.Params) {
				violations = append(violations, c.PreceptID)
			}
		}
	}
	sort.Strings(violations)

	switch {
	case len(violations) > 0:
		verdict = VerdictViolated
	case !anyActionMatched:
		verdict = VerdictNoApplicableConstraints
	default:
		verdict = VerdictPass
	}
	return verdict, violations, constraintsConsidered
}

// paramsSatisfy reports whether every condition in constraintParams holds
// against actionParams. A condition key carries its comparator as a suffix
// (_gte, _gt, _lte, _lt, _eq) over the field named by the rest of the key --
// e.g. "amount_gte: 500" reads actionParams["amount"] and requires it >= 500.
// An empty constraintParams (no conditions) trivially holds.
func paramsSatisfy(actionParams, constraintParams map[string]any) bool {
	for key, want := range constraintParams {
		field, op, ok := splitComparator(key)
		if !ok {
			return false // an unrecognized condition key can never be satisfied
		}
		got, present := actionParams[field]
		if !present {
			return false
		}
		if !compare(got, op, want) {
			return false
		}
	}
	return true
}

func splitComparator(key string) (field, op string, ok bool) {
	for _, suffix := range []string{"_gte", "_gt", "_lte", "_lt", "_eq"} {
		if strings.HasSuffix(key, suffix) {
			return strings.TrimSuffix(key, suffix), strings.TrimPrefix(suffix, "_"), true
		}
	}
	return "", "", false
}

func compare(got any, op string, want any) bool {
	if op == "eq" {
		return fmt.Sprint(got) == fmt.Sprint(want)
	}
	g, gok := toFloat64(got)
	w, wok := toFloat64(want)
	if !gok || !wok {
		return false
	}
	switch op {
	case "gte":
		return g >= w
	case "gt":
		return g > w
	case "lte":
		return g <= w
	case "lt":
		return g < w
	default:
		return false
	}
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}
