// Package interview implements Serenity's first-run interview wizard (RFC
// 0001 section 10.4: "First-run interview wizard (~30 questions -> draft
// precepts, each confirmed by one disposition) seeds the direction
// layer -- archaeology of old documents explicitly is not the seeding
// mechanism").
//
// Run drives a scripted question/answer loop over the bank in
// questions.yaml (embedded below) and, for each non-blank answer, calls a
// judgment-tier model once to synthesize one candidate precept -- title,
// body, and the mandatory "do not adopt this" floor alternative -- then
// stages it as one disposition.KindPreceptDraft item. Nothing this
// package writes ever reaches .dira: it has no *direction.Store handle at
// all, so "drafts only" (this task's own acc line) is a structural
// property of Run's signature, not a rule it merely follows. Accepting a
// staged draft is internal/direction.Store.ApplyDisposedPreceptDraft's
// job, wired into `serenity inbox`'s accept path the same way T3.11's
// Decompose wired KindDecompose.
package interview

import (
	_ "embed"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sirerun/serenity/internal/domain"
)

// Category is a question's place in the bank (questions.yaml's own doc
// comment names the three).
type Category string

const (
	CategoryIntent     Category = "intent"
	CategoryConstraint Category = "constraint"
	CategoryStanding   Category = "standing"
)

// Question is one entry in the interview's question bank.
type Question struct {
	// ID is a stable kebab-case slug, unrelated to dira's int-/dec-/qst-
	// entry ids -- it identifies a question in this bank, not a ledger
	// entry, and rides on PreceptDraftPayload.QuestionID so a later
	// human reviewing a staged draft in `serenity inbox` can see which
	// question produced it.
	ID string `yaml:"id"`

	// Category is intent, constraint, or standing (questions.yaml's own
	// doc comment). LoadQuestions rejects any other value.
	Category Category `yaml:"category"`

	// ActionClass is set only for a constraint question and, when set,
	// names the domain.ActionSet member the question is scoped to (RFC
	// section 7.3's closed five-action set). LoadQuestions rejects an
	// ActionClass outside that set, and rejects one set on a non-
	// constraint question -- an intent or standing question is not
	// about any single structured action.
	ActionClass string `yaml:"action_class,omitempty"`

	// Text is the question's exact wording, printed verbatim by Run.
	Text string `yaml:"text"`
}

// bankFile is questions.yaml's on-disk shape: one top-level key so the
// file reads as a labeled list rather than a bare YAML sequence, matching
// the style of every other checked-in YAML fixture in this repo (e.g.
// evals/corpora/*/labels/checksums.yaml's own top-level key).
type bankFile struct {
	Questions []Question `yaml:"questions"`
}

//go:embed questions.yaml
var defaultBank []byte

// DefaultQuestions parses the embedded questions.yaml -- the bank
// `serenity interview` uses when built, with no separate file needing to
// ship alongside the binary. Embedding (matching internal/dira/schema's
// own use of go:embed for entry.schema.json) is what makes this true.
func DefaultQuestions() ([]Question, error) {
	return LoadQuestions(defaultBank)
}

// LoadQuestions parses and validates a question bank from raw YAML bytes.
// It rejects a bank with no questions, a duplicate id, a category outside
// {intent, constraint, standing}, an action_class outside
// domain.ActionSet, and an action_class set on a non-constraint question
// -- every failure a hand-edited questions.yaml could introduce that
// would otherwise surface only much later, as a bad disposition payload
// or a silently-ignored field.
func LoadQuestions(data []byte) ([]Question, error) {
	var f bankFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("interview: parse question bank: %w", err)
	}
	if len(f.Questions) == 0 {
		return nil, fmt.Errorf("interview: question bank has no questions")
	}

	seen := make(map[string]bool, len(f.Questions))
	for i, q := range f.Questions {
		if q.ID == "" {
			return nil, fmt.Errorf("interview: question[%d]: id is required", i)
		}
		if seen[q.ID] {
			return nil, fmt.Errorf("interview: question[%d]: duplicate id %q", i, q.ID)
		}
		seen[q.ID] = true

		if q.Text == "" {
			return nil, fmt.Errorf("interview: question %q: text is required", q.ID)
		}

		switch q.Category {
		case CategoryIntent, CategoryStanding:
			if q.ActionClass != "" {
				return nil, fmt.Errorf("interview: question %q: category %q does not take action_class (only constraint questions do)", q.ID, q.Category)
			}
		case CategoryConstraint:
			if q.ActionClass == "" {
				return nil, fmt.Errorf("interview: question %q: category constraint requires action_class", q.ID)
			}
			if !validActionClass(q.ActionClass) {
				return nil, fmt.Errorf("interview: question %q: action_class %q is not in domain.ActionSet (%s)", q.ID, q.ActionClass, strings.Join(domain.ActionSet, ", "))
			}
		default:
			return nil, fmt.Errorf("interview: question %q: category %q is not one of intent, constraint, standing", q.ID, q.Category)
		}
	}
	return f.Questions, nil
}

func validActionClass(action string) bool {
	for _, a := range domain.ActionSet {
		if a == action {
			return true
		}
	}
	return false
}
