package interview

import (
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/domain"
)

func TestDefaultQuestionsParsesTheEmbeddedBank(t *testing.T) {
	qs, err := DefaultQuestions()
	if err != nil {
		t.Fatalf("DefaultQuestions: %v", err)
	}
	if len(qs) < 30 {
		t.Fatalf("got %d questions, want at least 30 (RFC 0001 section 10.4: \"~30 questions\")", len(qs))
	}
}

// TestDefaultQuestionsCoversEveryActionClass proves the bank's own claim
// ("constraints per action class") is real: every domain.ActionSet member
// has at least one constraint question naming it.
func TestDefaultQuestionsCoversEveryActionClass(t *testing.T) {
	qs, err := DefaultQuestions()
	if err != nil {
		t.Fatalf("DefaultQuestions: %v", err)
	}
	covered := make(map[string]bool)
	for _, q := range qs {
		if q.Category == CategoryConstraint {
			covered[q.ActionClass] = true
		}
	}
	for _, action := range domain.ActionSet {
		if !covered[action] {
			t.Errorf("no constraint question covers action_class %q", action)
		}
	}
}

// TestDefaultQuestionsCoversAllThreeCategories proves the bank's own
// claim ("intents, constraints per action class, standing questions") is
// real, not just declared in a doc comment.
func TestDefaultQuestionsCoversAllThreeCategories(t *testing.T) {
	qs, err := DefaultQuestions()
	if err != nil {
		t.Fatalf("DefaultQuestions: %v", err)
	}
	seen := map[Category]bool{}
	for _, q := range qs {
		seen[q.Category] = true
	}
	for _, want := range []Category{CategoryIntent, CategoryConstraint, CategoryStanding} {
		if !seen[want] {
			t.Errorf("no question has category %q", want)
		}
	}
}

func TestLoadQuestionsRejectsEmptyBank(t *testing.T) {
	if _, err := LoadQuestions([]byte("questions: []\n")); err == nil {
		t.Fatal("LoadQuestions: want error for an empty question bank, got nil")
	}
}

func TestLoadQuestionsRejectsDuplicateID(t *testing.T) {
	data := []byte(`
questions:
  - id: dup
    category: standing
    text: "one"
  - id: dup
    category: standing
    text: "two"
`)
	_, err := LoadQuestions(data)
	if err == nil {
		t.Fatal("LoadQuestions: want error for a duplicate id, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate id") {
		t.Errorf("error = %q, want it to mention the duplicate id", err)
	}
}

func TestLoadQuestionsRejectsUnknownCategory(t *testing.T) {
	data := []byte(`
questions:
  - id: bad
    category: whimsy
    text: "one"
`)
	if _, err := LoadQuestions(data); err == nil {
		t.Fatal("LoadQuestions: want error for an unknown category, got nil")
	}
}

func TestLoadQuestionsRejectsActionClassOutsideDomainActionSet(t *testing.T) {
	data := []byte(`
questions:
  - id: bad
    category: constraint
    action_class: launch_the_missiles
    text: "one"
`)
	if _, err := LoadQuestions(data); err == nil {
		t.Fatal("LoadQuestions: want error for an action_class outside domain.ActionSet, got nil")
	}
}

func TestLoadQuestionsRejectsConstraintWithNoActionClass(t *testing.T) {
	data := []byte(`
questions:
  - id: bad
    category: constraint
    text: "one"
`)
	if _, err := LoadQuestions(data); err == nil {
		t.Fatal("LoadQuestions: want error for a constraint question with no action_class, got nil")
	}
}

func TestLoadQuestionsRejectsActionClassOnNonConstraintQuestion(t *testing.T) {
	data := []byte(`
questions:
  - id: bad
    category: intent
    action_class: spend_over
    text: "one"
`)
	if _, err := LoadQuestions(data); err == nil {
		t.Fatal("LoadQuestions: want error for an intent question carrying action_class, got nil")
	}
}

func TestLoadQuestionsRejectsMissingText(t *testing.T) {
	data := []byte(`
questions:
  - id: bad
    category: standing
`)
	if _, err := LoadQuestions(data); err == nil {
		t.Fatal("LoadQuestions: want error for a question with no text, got nil")
	}
}
