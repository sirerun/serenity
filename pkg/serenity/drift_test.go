package serenity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"reflect"
	"testing"

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
