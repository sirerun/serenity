package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/direction"
	"github.com/sirerun/serenity/internal/direction/check"
	"github.com/sirerun/serenity/internal/router"
	"github.com/sirerun/serenity/internal/writer"
)

// newCheckCmd wires `serenity check`, the CLI surface over DIRECTION v1's
// check_plan (RFC 0001 §8.3, ADR 010): either free-text plan prose (stage
// 2, internal/direction/check.Classifier) or structured --actions JSON
// (stage 1 directly, internal/direction/check.Matcher), against every
// active constraint precept in the brain repo's ledger.
//
// Exit codes follow ADR 010 exactly, never improvised: 0 for pass and for
// no_applicable_constraints (the verdict string in stdout/--json tells
// them apart -- no_applicable_constraints is never printed as "pass"), 2
// for violated, 1 for unverified or any other error.
func newCheckCmd() *cobra.Command {
	var actionsJSON string
	var jsonOut bool
	var claudeHook bool
	cmd := &cobra.Command{
		Use:   "check [plan text]",
		Short: "Check a plan against active precept constraints (DIRECTION v1 check_plan)",
		Long: "check evaluates either free-text plan prose (the positional argument) or\n" +
			"structured --actions JSON against every active constraint precept in the\n" +
			"brain repo's ledger, plus open blocking questions.\n\n" +
			"Exit codes (ADR 010): 0 for pass or no_applicable_constraints, 2 for\n" +
			"violated, 1 for unverified or any other error.\n\n" +
			"--claude-hook reads a Claude Code PreToolUse hook envelope from stdin\n" +
			"instead -- this is what `serenity connect claude` installs as Claude\n" +
			"Code's pre-plan gate; a person runs the plan-text or --actions form.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if claudeHook {
				if len(args) != 0 || actionsJSON != "" {
					return claudeGateResult("", fmt.Errorf("--claude-hook is mutually exclusive with plan text and --actions"), cmd.ErrOrStderr())
				}
				return runCheckClaudeHook(cmd.Context(), flagRoot, cmd.InOrStdin(), cmd.ErrOrStderr())
			}
			var planText string
			if len(args) == 1 {
				planText = args[0]
			}
			return runCheck(cmd.Context(), flagRoot, planText, actionsJSON, jsonOut, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&actionsJSON, "actions", "",
		`structured actions as a JSON array, e.g. '[{"action":"spend_over","params":{"amount":500}}]' (mutually exclusive with the plan-text argument)`)
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable JSON output")
	cmd.Flags().BoolVar(&claudeHook, "claude-hook", false,
		"read a Claude Code PreToolUse hook JSON envelope from stdin (installed by `serenity connect claude`)")
	return cmd
}

// runCheckClaudeHook adapts the ordinary check verdict to Claude Code's
// blocking exit contract. Plan content remains data; only an actual pass
// permits the call. The ordinary check command keeps ADR 010 exit semantics.
func runCheckClaudeHook(ctx context.Context, root string, in io.Reader, stderr io.Writer) error {
	const limit = 1 << 20
	raw, err := io.ReadAll(io.LimitReader(in, limit+1))
	if err != nil {
		return claudeGateResult("", fmt.Errorf("read hook input: %w", err), stderr)
	}
	if len(raw) > limit {
		return claudeGateResult("", fmt.Errorf("hook input exceeds %d bytes", limit), stderr)
	}
	var hook struct {
		Event string          `json:"hook_event_name"`
		Tool  string          `json:"tool_name"`
		Input json.RawMessage `json:"tool_input"`
	}
	if err := json.Unmarshal(raw, &hook); err != nil {
		return claudeGateResult("", fmt.Errorf("decode hook input: %w", err), stderr)
	}
	if hook.Event == "" {
		return claudeGateResult("", fmt.Errorf("hook_event_name is required"), stderr)
	}
	if hook.Event != "PreToolUse" {
		return nil
	}
	if hook.Tool == "" {
		return claudeGateResult("", fmt.Errorf("PreToolUse requires tool_name"), stderr)
	}
	if hook.Tool != "ExitPlanMode" {
		return nil
	}
	var input struct {
		Plan string `json:"plan"`
	}
	if err := json.Unmarshal(hook.Input, &input); err != nil {
		return claudeGateResult("", fmt.Errorf("decode tool_input: %w", err), stderr)
	}
	if strings.TrimSpace(input.Plan) == "" {
		return claudeGateResult("", fmt.Errorf("ExitPlanMode requires nonblank tool_input.plan"), stderr)
	}
	var output bytes.Buffer
	checkErr := runCheck(ctx, root, input.Plan, "", true, &output)
	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		if checkErr != nil {
			return claudeGateResult("", checkErr, stderr)
		}
		return claudeGateResult("", fmt.Errorf("decode check verdict: %w", err), stderr)
	}
	return claudeGateResult(result.Status, checkErr, stderr)
}

func claudeGateResult(status string, err error, stderr io.Writer) error {
	if status == "pass" && err == nil {
		return nil
	}
	if status != "" {
		_, _ = fmt.Fprintf(stderr, "Serenity blocked ExitPlanMode: check status %s. Run serenity check --actions for a structured check; free-text classification is not configured.\n", status)
	}
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Serenity blocked ExitPlanMode: %v\n", err)
	}
	if status == "" && err == nil {
		_, _ = fmt.Fprintln(stderr, "Serenity blocked ExitPlanMode: missing check verdict")
	}
	return &ExitError{Code: 2}
}

// runCheck is the check_plan CLI surface. It never returns a bare error for
// a computed verdict -- pass/no_applicable_constraints/unverified/violated
// are all printed to out and then signaled via the returned error's exit
// code (nil for 0, *ExitError for 2 or 1); a plain error return means the
// verdict itself could not be computed at all (bad input, a ledger read
// failure, an action outside domain.ActionSet), which is ADR 010's other
// exit-1 case.
func runCheck(ctx context.Context, root, planText, actionsJSON string, jsonOut bool, out io.Writer) error {
	if planText != "" && actionsJSON != "" {
		return fmt.Errorf("check: provide plan text or --actions, not both")
	}
	if planText == "" && actionsJSON == "" {
		return fmt.Errorf(`check: provide plan text or --actions '<json array>'`)
	}

	if _, err := config.Load(filepath.Join(root, config.FileName)); err != nil {
		return fmt.Errorf("not a brain repo (run `serenity init`?): %w", err)
	}

	queue := writer.NewQueue(nil)
	defer queue.Close()
	store := direction.NewStore(root, queue)
	// No provider-from-config wiring exists yet to build a live classification
	// model for the local-cheap tier (serenity.yml carries no
	// models.classification pin today) -- mirrors runSearch's identical
	// precedent for embeddings. A nil router is not a stub: it is exactly the
	// "no model configured" condition check.Classifier.MatchFreeText already
	// documents as one of the two conditions that yield StatusUnverified, so
	// free-text plan checks correctly and honestly report unverified until a
	// classification provider is wired here.
	matcher := check.New(store, nil)

	var (
		result         check.Result
		matchedActions []check.MatchedAction
		confidence     float64
		haveConfidence bool
	)

	if actionsJSON != "" {
		actions, err := parseCheckActions(actionsJSON)
		if err != nil {
			return fmt.Errorf("check: --actions: %w", err)
		}
		result, err = matcher.Match(ctx, actions)
		if err != nil {
			return fmt.Errorf("check: %w", err)
		}
	} else {
		classifier := check.NewClassifier(matcher, "", nil)
		ftResult, err := classifier.MatchFreeText(ctx, planText, router.Budget{})
		if err != nil {
			return fmt.Errorf("check: %w", err)
		}
		result = ftResult.Result
		matchedActions = ftResult.MatchedActions
		confidence = ftResult.Confidence
		haveConfidence = true
	}

	if jsonOut {
		writeCheckJSON(out, result, matchedActions, confidence, haveConfidence)
	} else {
		writeCheckText(out, result, matchedActions, confidence, haveConfidence)
	}

	switch result.Status {
	case check.StatusPass, check.StatusNoApplicableConstraints:
		return nil
	case check.StatusViolated:
		return &ExitError{Code: 2}
	default: // check.StatusUnverified
		return &ExitError{Code: 1}
	}
}

// checkActionInput is one --actions array element, decoded before it is
// converted into check.Action.
type checkActionInput struct {
	Action string         `json:"action"`
	Params map[string]any `json:"params,omitempty"`
}

// parseCheckActions decodes --actions' JSON array into check.Action values.
// It rejects unknown fields so a caller's typo (e.g. "ammount") fails loudly
// at parse time rather than silently matching no constraint clause.
func parseCheckActions(raw string) ([]check.Action, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var inputs []checkActionInput
	if err := dec.Decode(&inputs); err != nil {
		return nil, fmt.Errorf("invalid JSON array: %w", err)
	}
	actions := make([]check.Action, len(inputs))
	for i, in := range inputs {
		actions[i] = check.Action{Action: in.Action, Params: in.Params}
	}
	return actions, nil
}

// writeCheckText renders the human-readable form. The status line always
// prints result.Status's actual value verbatim -- never a hand-picked
// English word standing in for it -- which is what keeps
// no_applicable_constraints from ever coming out as "pass" (ADR 010).
// Every violated constraint's why_not/revisit_if is printed exactly as
// stored, with no summarizing or reformatting.
func writeCheckText(out io.Writer, result check.Result, matched []check.MatchedAction, confidence float64, haveConfidence bool) {
	_, _ = fmt.Fprintf(out, "status: %s\n", result.Status)
	_, _ = fmt.Fprintf(out, "considered: %d active constraint(s)\n", result.ConsideredCount)

	for _, c := range result.Constraints {
		_, _ = fmt.Fprintf(out, "%s: %s\n", c.Outcome, c.PreceptID)
		if c.Outcome == check.OutcomeViolated {
			_, _ = fmt.Fprintf(out, "  why_not: %s\n", c.WhyNot)
			if c.RevisitIf != "" {
				_, _ = fmt.Fprintf(out, "  revisit_if: %s\n", c.RevisitIf)
			}
		}
	}

	for _, w := range result.Warnings {
		_, _ = fmt.Fprintf(out, "warning: %s blocks action %q: %s\n", w.PreceptID, w.Action, w.Title)
	}

	if haveConfidence {
		_, _ = fmt.Fprintf(out, "confidence: %.2f\n", confidence)
		for _, ma := range matched {
			_, _ = fmt.Fprintf(out, "matched: %s %v (evidence: %q)\n", ma.Action.Action, ma.Action.Params, ma.Span.Text)
		}
	}
}

// writeCheckJSON renders `serenity check --json`'s response via
// check.ToWire (internal/direction/check/wire.go) -- the same converter
// DIRECTION v1's check_plan HTTP handler (internal/server/direction,
// T4.6) uses, so the two surfaces can never drift on field names or
// omission rules. This package no longer declares its own copy of the
// wire shape.
func writeCheckJSON(out io.Writer, result check.Result, matched []check.MatchedAction, confidence float64, haveConfidence bool) {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(check.ToWire(result, matched, confidence, haveConfidence))
}
