package serenity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/conformance"
	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/pkg/serenity"
)

// TestCheckPlanDriftAgainstCLI is ADR 012 §2's drift test: for every case,
// the facade's CheckPlan and the built `serenity check --json --actions`
// binary read the same on-disk ledger and their outputs must deep-equal
// after both are normalized through encoding/json into untyped values (so
// field order, indentation and Go types drop out and only the wire shape
// remains). The corpus is testdata/brain-fixture as shipped (no
// applies_when clause on its one constraint: no_applicable_constraints),
// plus seeded constraint and question entries that drive every other
// verdict and the warnings field, so the comparison is not vacuously
// green on an empty shape.
func TestCheckPlanDriftAgainstCLI(t *testing.T) {
	bin := buildSerenityBinary(t)

	type seed func(t *testing.T, root string)
	cases := []struct {
		name    string
		seed    seed
		actions []serenity.Action
		exit    int
	}{
		{
			name:    "fixture as shipped: no applicable constraints",
			actions: []serenity.Action{{Action: "start_project"}},
		},
		{
			name: "seeded constraint passes under threshold",
			seed: func(t *testing.T, root string) {
				seedConstraint(t, root, "cst-9001", "spend_over", "{amount: {gte: 200}}")
			},
			actions: []serenity.Action{
				{Action: "spend_over", Params: map[string]any{"amount": 100}},
			},
		},
		{
			name: "seeded constraint violated with why_not verbatim",
			seed: func(t *testing.T, root string) {
				seedConstraint(t, root, "cst-9001", "spend_over", "{amount: {gte: 200}}")
			},
			actions: []serenity.Action{
				{Action: "spend_over", Params: map[string]any{"amount": 500}},
			},
			exit: 2,
		},
		{
			name: "open question warning alongside a pass",
			seed: func(t *testing.T, root string) {
				seedConstraint(t, root, "cst-9001", "spend_over", "{amount: {gte: 200}}")
				seedOpenQuestion(t, root, "qst-9001", "spend_over")
			},
			actions: []serenity.Action{
				{Action: "spend_over", Params: map[string]any{"amount": 100}},
				{Action: "deploy_to_prod"},
			},
		},
		{
			name:    "empty actions",
			actions: []serenity.Action{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := fixtureBrain(t)
			if tc.seed != nil {
				tc.seed(t, root)
			}

			// CLI side.
			actionsJSON, err := json.Marshal(tc.actions)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(bin, "-C", root, "check", "--json", "--actions", string(actionsJSON))
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err = cmd.Run()
			var exitErr *exec.ExitError
			switch {
			case err == nil && tc.exit == 0:
			case errors.As(err, &exitErr) && exitErr.ExitCode() == tc.exit:
			default:
				t.Fatalf("serenity check: err=%v (want exit %d)\nstdout: %s\nstderr: %s", err, tc.exit, stdout.String(), stderr.String())
			}
			var want any
			if err := json.Unmarshal(stdout.Bytes(), &want); err != nil {
				t.Fatalf("CLI output is not JSON: %v\n%s", err, stdout.String())
			}

			// Facade side.
			b, err := serenity.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			verdict, err := b.CheckPlan(context.Background(), tc.actions)
			if err != nil {
				t.Fatalf("CheckPlan: %v", err)
			}
			got := normalize(t, verdict)

			if !reflect.DeepEqual(got, want) {
				t.Fatalf("facade drifted from `serenity check --json --actions`\n facade: %s\n    cli: %s", mustJSON(t, got), mustJSON(t, want))
			}
		})
	}
}

// normalize round-trips v through encoding/json into an untyped value.
func normalize(t *testing.T, v any) any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// directionTranscriptPath resolves one testdata/conformance/direction/
// transcript file (T4.13), relative to this test file's own package dir.
func directionTranscriptPath(t *testing.T, operation string) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "conformance", "direction", operation+".json")
}

// freshBrain returns a brand-new, empty brain root: a serenity.yml plus
// an empty .dira/entries dir, with no fixture entries copied in -- the
// same starting state testdata/conformance/direction/gen_transcripts.go's
// own newEnv builds for every one of its generated cases (a fresh temp
// root, zero ledger entries), deliberately not fixtureBrain's non-empty
// testdata/brain-fixture copy (that corpus belongs to
// TestCheckPlanDriftAgainstCLI above, not to the T4.13 transcripts,
// which were captured against their own from-empty ledgers).
func freshBrain(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".dira", "entries"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.Default().Save(filepath.Join(root, config.FileName)); err != nil {
		t.Fatal(err)
	}
	return root
}

// seedDirectionConstraint and seedDirectionQuestion reproduce
// testdata/conformance/direction/gen_transcripts.go's own
// identically-named helpers byte-for-byte (title, applies_when body,
// alternative why_not/revisit_if, Created timestamp), so the ledger
// state built here matches the state that produced the frozen T4.13
// transcript this test replays against. Like fixture_test.go's own
// seedEntry, the entry is written directly to disk (ledger.Encode +
// os.WriteFile) rather than through direction.Store.Create/writer.Queue:
// pkg/serenity's tests must not import internal/writer any more than the
// package itself does.
func seedDirectionConstraint(t *testing.T, root, id, title, action, paramsYAML, whyNot, revisitIf string) {
	t.Helper()
	body := "Fixture constraint.\n\n```serenity:applies_when\naction: " + action + "\n"
	if paramsYAML != "" {
		body += "params: " + paramsYAML + "\n"
	}
	body += "```\n"
	seedEntry(t, root, &ledger.Entry{
		ID: id, Kind: ledger.KindConstraint, Title: title, State: ledger.StateActive,
		Created:      "2026-08-28T00:00:00Z",
		Alternatives: []ledger.Alternative{{Option: "no ceiling", WhyNot: whyNot, RevisitIf: revisitIf}},
		Body:         body,
	})
}

func seedDirectionQuestion(t *testing.T, root, id, title string) {
	t.Helper()
	seedEntry(t, root, &ledger.Entry{
		ID: id, Kind: ledger.KindQuestion, Title: title, State: ledger.StateOpen,
		Created: "2026-08-28T00:00:00Z",
	})
}

// briefDirectionSeeds maps a T4.13 brief.json case name to the ledger
// seed that produced it, copied from
// testdata/conformance/direction/gen_transcripts.go's own genBrief. The
// first case ("brief on a fresh ledger...") needs no seed and has no
// entry here.
func briefDirectionSeeds(t *testing.T) map[string]func(t *testing.T, root string) {
	t.Helper()
	return map[string]func(t *testing.T, root string){
		"a zero token_budget returns the minimal valid object: four sections, real candidates all omitted": func(t *testing.T, root string) {
			seedDirectionConstraint(t, root, "cst-0001", "fixture spend ceiling", "spend_over", "{amount: {gte: 200}}",
				"unbounded spend risk", "quarterly review")
			seedDirectionQuestion(t, root, "qst-0001", "should we raise the ceiling?")
		},
		"budget 800 fills sections by priority and drops an overflowing precepts section whole, never starving questions": func(t *testing.T, root string) {
			var longWhyNot strings.Builder
			for i := 0; i < 70; i++ {
				longWhyNot.WriteString("supercalifragilisticexpialidocious ")
			}
			const preceptCap = 12 // internal/server/direction.preceptCap, unexported; duplicated per that package's own gen_transcripts.go precedent.
			for i := 1; i <= preceptCap; i++ {
				id := fmt.Sprintf("cst-%04d", i)
				seedDirectionConstraint(t, root, id, fmt.Sprintf("fixture precept number %d", i), "spend_over", "{amount: {gte: 200}}",
					longWhyNot.String(), "quarterly review")
			}
			seedDirectionQuestion(t, root, "qst-0001", "should we raise the ceiling?")
			seedDirectionQuestion(t, root, "qst-0002", "who owns this precept?")
		},
	}
}

// TestBriefDriftAgainstDirectionTranscripts is T4.19's own acc line: "Brief
// output deep-equals the DIRECTION `brief` wire object on normalized JSON
// for every budget in the T4.13 brief transcripts." There is no `serenity
// brief` CLI verb to shell out to (T4.9's own disclosed scope), so unlike
// TestCheckPlanDriftAgainstCLI above, the comparison target here is the
// frozen recorded response in testdata/conformance/direction/brief.json
// itself: for each case, this test rebuilds the exact ledger state
// gen_transcripts.go's own genBrief built (briefDirectionSeeds above),
// calls the facade's Brief with the transcript's own recorded
// task_hint/token_budget, and asserts the result deep-equals the
// transcript's own recorded response on normalized JSON -- the same
// normalize-then-reflect.DeepEqual technique
// TestCheckPlanDriftAgainstCLI uses, just against a frozen fixture
// instead of a live CLI process.
func TestBriefDriftAgainstDirectionTranscripts(t *testing.T) {
	tr, err := conformance.LoadTranscript(directionTranscriptPath(t, "brief"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Cases) == 0 {
		t.Fatal("testdata/conformance/direction/brief.json has zero cases; the drift corpus would be vacuous")
	}
	seeds := briefDirectionSeeds(t)

	for _, c := range tr.Cases {
		t.Run(c.Name, func(t *testing.T) {
			if len(c.Steps) != 1 {
				t.Fatalf("brief case %q has %d steps, want exactly 1", c.Name, len(c.Steps))
			}
			step := c.Steps[0]
			if step.Method != "POST" || step.Path != "/direction/brief" {
				t.Fatalf("brief case %q: method/path = %s %s, want POST /direction/brief", c.Name, step.Method, step.Path)
			}

			root := freshBrain(t)
			if seed, ok := seeds[c.Name]; ok {
				seed(t, root)
			}

			var req struct {
				TaskHint    string `json:"task_hint"`
				TokenBudget int    `json:"token_budget"`
			}
			if err := json.Unmarshal(step.RequestBody, &req); err != nil {
				t.Fatalf("decode request_body: %v", err)
			}

			b, err := serenity.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			brief, err := b.Brief(context.Background(), serenity.Budget{TaskHint: req.TaskHint, TokenBudget: req.TokenBudget})
			if err != nil {
				t.Fatalf("Brief: %v", err)
			}
			got := normalize(t, brief)

			var want any
			if err := json.Unmarshal([]byte(step.Response.Body), &want); err != nil {
				t.Fatalf("transcript response is not JSON: %v\n%s", err, step.Response.Body)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("facade Brief drifted from testdata/conformance/direction/brief.json case %q\n     facade: %s\ntranscript: %s",
					c.Name, mustJSON(t, got), mustJSON(t, want))
			}
		})
	}
}

// checkPlanDirectionSeeds maps a T4.13 check_plan.json case name to the
// ledger seed that produced it, copied from
// testdata/conformance/direction/gen_transcripts.go's own genCheckPlan.
func checkPlanDirectionSeeds(t *testing.T) map[string]func(t *testing.T, root string) {
	t.Helper()
	return map[string]func(t *testing.T, root string){
		"check_plan with a structured action matching an active constraint returns status violated": func(t *testing.T, root string) {
			seedDirectionConstraint(t, root, "cst-0001", "fixture spend ceiling", "spend_over", "{amount: {gte: 200}}",
				`Unbounded spend risk: "no ceiling" was rejected outright.`, "quarterly budget review")
		},
	}
}

// TestCheckPlanDriftAgainstDirectionTranscripts is T4.19's other acc line:
// "CheckPlan deep-equals `serenity check --json --actions` over every
// T4.13 check_plan transcript" -- extending TestCheckPlanDriftAgainstCLI
// above's CLI-vs-facade comparison to the T4.13 corpus's own scenarios,
// rebuilding each case's ledger state from
// testdata/conformance/direction/gen_transcripts.go's own genCheckPlan
// (checkPlanDirectionSeeds above) rather than inventing new ones.
//
// Two of check_plan.json's four cases carry no actions[] at all -- one
// exercises plan_text (free-text classification, which pkg/serenity's
// CheckPlan has no input for: Action carries no PlanText field, ADR 012's
// own read surface never grew a classifier-router seam) and one exercises
// the plan_text/actions mutual-exclusivity 400 (a wire-layer-only
// validation rule with no facade equivalent to drift-check at all, since
// the facade simply has no such request to reject). Both are explicitly
// skipped rather than silently omitted, and the test fails outright if
// every case in the file turns out to be one of these (a vacuous corpus).
func TestCheckPlanDriftAgainstDirectionTranscripts(t *testing.T) {
	bin := buildSerenityBinary(t)
	tr, err := conformance.LoadTranscript(directionTranscriptPath(t, "check_plan"))
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Cases) == 0 {
		t.Fatal("testdata/conformance/direction/check_plan.json has zero cases; the drift corpus would be vacuous")
	}
	seeds := checkPlanDirectionSeeds(t)

	exercised := 0
	for _, c := range tr.Cases {
		t.Run(c.Name, func(t *testing.T) {
			if len(c.Steps) != 1 {
				t.Fatalf("check_plan case %q has %d steps, want exactly 1", c.Name, len(c.Steps))
			}
			step := c.Steps[0]
			if step.Method != "POST" || step.Path != "/direction/check_plan" {
				t.Fatalf("check_plan case %q: method/path = %s %s, want POST /direction/check_plan", c.Name, step.Method, step.Path)
			}

			var req struct {
				PlanText string            `json:"plan_text"`
				Actions  []serenity.Action `json:"actions"`
			}
			if err := json.Unmarshal(step.RequestBody, &req); err != nil {
				t.Fatalf("decode request_body: %v", err)
			}
			if len(req.Actions) == 0 {
				t.Skipf("case %q carries no structured actions[] (plan_text or neither) -- pkg/serenity's CheckPlan has no plan_text input to drift-check", c.Name)
			}
			exercised++

			root := freshBrain(t)
			if seed, ok := seeds[c.Name]; ok {
				seed(t, root)
			}

			wantExit := 0
			if strings.Contains(step.Response.Body, `"status":"violated"`) {
				wantExit = 2
			}

			actionsJSON, err := json.Marshal(req.Actions)
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(bin, "-C", root, "check", "--json", "--actions", string(actionsJSON))
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err = cmd.Run()
			var exitErr *exec.ExitError
			switch {
			case err == nil && wantExit == 0:
			case errors.As(err, &exitErr) && exitErr.ExitCode() == wantExit:
			default:
				t.Fatalf("serenity check: err=%v (want exit %d)\nstdout: %s\nstderr: %s", err, wantExit, stdout.String(), stderr.String())
			}
			var want any
			if err := json.Unmarshal(stdout.Bytes(), &want); err != nil {
				t.Fatalf("CLI output is not JSON: %v\n%s", err, stdout.String())
			}

			b, err := serenity.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			verdict, err := b.CheckPlan(context.Background(), req.Actions)
			if err != nil {
				t.Fatalf("CheckPlan: %v", err)
			}
			got := normalize(t, verdict)

			if !reflect.DeepEqual(got, want) {
				t.Fatalf("facade drifted from `serenity check --json --actions` over transcript case %q\n facade: %s\n    cli: %s",
					c.Name, mustJSON(t, got), mustJSON(t, want))
			}
		})
	}
	if exercised == 0 {
		t.Fatal("every check_plan.json case was skipped (no structured-actions case found); the drift corpus would be vacuous")
	}
}
