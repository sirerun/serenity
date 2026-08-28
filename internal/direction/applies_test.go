package direction

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/dira/schema"
)

const fixtureBody = `Spend above the ceiling requires asking first.

` + "```serenity:applies_when" + `
action: spend_over
params: {amount: 500, currency: usd}
` + "```" + `

More prose after the block, to prove the parser does not swallow it.
`

func TestParseAppliesWhen_RoundTrip(t *testing.T) {
	block, err := ParseAppliesWhen([]byte(fixtureBody))
	if err != nil {
		t.Fatalf("ParseAppliesWhen: %v", err)
	}
	if block.Action != "spend_over" {
		t.Errorf("Action = %q, want spend_over", block.Action)
	}
	if got := block.Params["amount"]; got != 500 {
		t.Errorf("Params[amount] = %v (%T), want 500", got, got)
	}
	if got := block.Params["currency"]; got != "usd" {
		t.Errorf("Params[currency] = %v, want usd", got)
	}

	wantBlock := "```serenity:applies_when\n" +
		"action: spend_over\n" +
		"params: {amount: 500, currency: usd}\n" +
		"```\n"

	rendered := RenderAppliesWhenBlock(block)
	if string(rendered) != wantBlock {
		t.Errorf("RenderAppliesWhenBlock =\n%q\nwant\n%q", rendered, wantBlock)
	}

	// The rendered block must itself parse back to the same clause --
	// round-tripping is stable, not just byte-preserving on the first pass.
	reparsed, err := ParseAppliesWhen(rendered)
	if err != nil {
		t.Fatalf("ParseAppliesWhen(rendered): %v", err)
	}
	if reparsed.Action != block.Action || string(reparsed.Raw) != string(block.Raw) {
		t.Errorf("reparsed block does not match original: %+v vs %+v", reparsed, block)
	}
}

func TestParseAppliesWhen_NoBlock(t *testing.T) {
	_, err := ParseAppliesWhen([]byte("Just prose, no fenced block at all.\n"))
	if !errors.Is(err, ErrNoAppliesWhenBlock) {
		t.Fatalf("err = %v, want ErrNoAppliesWhenBlock", err)
	}
}

func TestParseAppliesWhen_UnopenedNeverClosed(t *testing.T) {
	body := "```serenity:applies_when\naction: spend_over\n"
	_, err := ParseAppliesWhen([]byte(body))
	if !errors.Is(err, ErrNoAppliesWhenBlock) {
		t.Fatalf("err = %v, want ErrNoAppliesWhenBlock (unclosed)", err)
	}
}

func TestParseAppliesWhen_UnknownActionRejected(t *testing.T) {
	body := "```serenity:applies_when\n" +
		"action: launch_the_missiles\n" +
		"```\n"
	_, err := ParseAppliesWhen([]byte(body))
	if !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("err = %v, want ErrUnknownAction", err)
	}
}

func TestParseAppliesWhen_EveryDomainActionAccepted(t *testing.T) {
	for _, action := range []string{
		"start_project", "deploy_to_prod", "spend_over",
		"contact_new_party", "schedule_outside_hours",
	} {
		body := "```serenity:applies_when\naction: " + action + "\n```\n"
		block, err := ParseAppliesWhen([]byte(body))
		if err != nil {
			t.Errorf("action %q: %v", action, err)
			continue
		}
		if block.Action != action {
			t.Errorf("action %q: got %q", action, block.Action)
		}
	}
}

func TestValidateAppliesWhenPlacement_MisplacedInFrontmatter(t *testing.T) {
	entry := `---
id: cst-0099
kind: constraint
title: A constraint that got applies_when wrong
state: active
created: "2026-08-22T00:00:00Z"
applies_when:
  action: spend_over
---
The body has no block; applies_when landed in frontmatter instead, which
dira's schema would reject anyway -- this error names the actual mistake.
`
	err := ValidateAppliesWhenPlacement([]byte(entry))
	if !errors.Is(err, ErrMisplacedAppliesWhen) {
		t.Fatalf("err = %v, want ErrMisplacedAppliesWhen", err)
	}
}

func TestValidateAppliesWhenPlacement_ActionOutsideActionSetRejected(t *testing.T) {
	entry := `---
id: cst-0098
kind: constraint
title: A constraint naming an action nobody defined
state: active
created: "2026-08-22T00:00:00Z"
---
Body block below names an action outside domain.ActionSet.

` + "```serenity:applies_when" + `
action: order_a_pizza
` + "```" + `
`
	err := ValidateAppliesWhenPlacement([]byte(entry))
	if !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("err = %v, want ErrUnknownAction", err)
	}
}

func TestValidateAppliesWhenPlacement_NoBlockIsValid(t *testing.T) {
	entry := `---
id: int-0099
kind: intent
title: An intent with no applies_when block at all
state: active
created: "2026-08-22T00:00:00Z"
---
Intents carry no applies_when clause; this must validate cleanly.
`
	if err := ValidateAppliesWhenPlacement([]byte(entry)); err != nil {
		t.Fatalf("ValidateAppliesWhenPlacement: %v", err)
	}
}

// TestFixtureLedger_PassesVendoredSchemaAndPlacement proves the two
// validations this task adds are independent of, and compatible with,
// dira's own entry.schema.json: every fixture below carries a valid
// applies_when body block (or none at all) and still validates against the
// vendored schema unmodified (ADR 008's central claim).
func TestFixtureLedger_PassesVendoredSchemaAndPlacement(t *testing.T) {
	validator, err := schema.NewValidator()
	if err != nil {
		t.Fatalf("schema.NewValidator: %v", err)
	}

	entries, err := filepath.Glob(filepath.Join("testdata", "fixture-ledger", "*.md"))
	if err != nil {
		t.Fatalf("glob fixture ledger: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("fixture ledger is empty")
	}

	for _, path := range entries {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			if err := validator.Validate(content); err != nil {
				t.Errorf("vendored schema rejected %s: %v", path, err)
			}
			if err := ValidateAppliesWhenPlacement(content); err != nil {
				t.Errorf("ValidateAppliesWhenPlacement rejected %s: %v", path, err)
			}
		})
	}
}
