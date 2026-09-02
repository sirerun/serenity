package serenity_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sirerun/serenity/internal/direction/check"
	"github.com/sirerun/serenity/pkg/serenity"
)

func TestOpenRejectsNonBrainRepo(t *testing.T) {
	if _, err := serenity.Open(t.TempDir()); err == nil {
		t.Fatal("Open on a directory with no serenity.yml: want error, got nil")
	}
}

func TestOpenOptionErrorIsReturned(t *testing.T) {
	root := fixtureBrain(t)
	boom := errors.New("boom")
	_, err := serenity.Open(root, func(*serenity.Brain) error { return boom })
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want wrapping the option's error", err)
	}
}

func TestCheckPlanUnknownActionIsAnError(t *testing.T) {
	root := fixtureBrain(t)
	b, err := serenity.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.CheckPlan(context.Background(), []serenity.Action{{Action: "launch_rocket"}})
	if !errors.Is(err, check.ErrUnknownAction) {
		t.Fatalf("err = %v, want wrapping check.ErrUnknownAction", err)
	}
}

func TestCheckPlanViolatedCopiesWhyNotVerbatim(t *testing.T) {
	root := fixtureBrain(t)
	seedConstraint(t, root, "cst-9001", "spend_over", "{amount: {gte: 200}}")
	b, err := serenity.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	v, err := b.CheckPlan(context.Background(), []serenity.Action{{Action: "spend_over", Params: map[string]any{"amount": 500}}})
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "violated" || len(v.Constraints) != 1 {
		t.Fatalf("verdict = %+v, want violated with one constraint", v)
	}
	c := v.Constraints[0]
	if c.PreceptID != "cst-9001" || c.Outcome != "violated" || c.WhyNot != fixtureWhyNot || c.RevisitIf != fixtureRevisitIf {
		t.Fatalf("constraint = %+v, want why_not/revisit_if verbatim", c)
	}
}

// TestRecallWithoutComposerReturnsHitsAndNote: with config.Default()'s
// none@v0 pins the facade takes exactly `serenity ask`'s explicit-skip
// path -- no model call, Answer nil, the CLI's own note carried through --
// and `serenity search`'s full-text path over an index that has nothing
// in it yet, which is an empty hit list rather than an error.
func TestRecallWithoutComposerReturnsHitsAndNote(t *testing.T) {
	root := fixtureBrain(t)
	b, err := serenity.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	r, err := b.Recall(context.Background(), "background daemon", serenity.Budget{})
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if r.Answer != nil {
		t.Fatalf("Answer = %+v, want nil with no composer pinned", r.Answer)
	}
	if r.Note == "" {
		t.Fatal("Note is empty; want the CLI's composer-skipped reason")
	}
	if len(r.Hits) != 0 {
		t.Fatalf("Hits = %+v, want none over an unbuilt index", r.Hits)
	}
}
