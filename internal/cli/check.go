package cli

import (
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
	cmd := &cobra.Command{
		Use:   "check [plan text]",
		Short: "Check a plan against active precept constraints (DIRECTION v1 check_plan)",
		Long: "check evaluates either free-text plan prose (the positional argument) or\n" +
			"structured --actions JSON against every active constraint precept in the\n" +
			"brain repo's ledger, plus open blocking questions.\n\n" +
			"Exit codes (ADR 010): 0 for pass or no_applicable_constraints, 2 for\n" +
			"violated, 1 for unverified or any other error.",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
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
	return cmd
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

// checkJSONOutput is `serenity check --json`'s response shape, field names
// mirroring RFC 0001 §8.3's own vocabulary (precept_id, why_not,
// revisit_if, matched_actions, spans) rather than inventing new ones.
type checkJSONOutput struct {
	Status          string                   `json:"status"`
	ConsideredCount int                      `json:"considered_count"`
	Constraints     []checkJSONConstraint    `json:"constraints,omitempty"`
	Warnings        []checkJSONWarning       `json:"warnings,omitempty"`
	MatchedActions  []checkJSONMatchedAction `json:"matched_actions,omitempty"`
	Confidence      *float64                 `json:"confidence,omitempty"`
}

type checkJSONConstraint struct {
	PreceptID string `json:"precept_id"`
	Outcome   string `json:"outcome"`
	WhyNot    string `json:"why_not,omitempty"`
	RevisitIf string `json:"revisit_if,omitempty"`
}

type checkJSONWarning struct {
	PreceptID string `json:"precept_id"`
	Title     string `json:"title"`
	Action    string `json:"action"`
}

type checkJSONSpan struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Text  string `json:"text"`
}

type checkJSONMatchedAction struct {
	Action string         `json:"action"`
	Params map[string]any `json:"params,omitempty"`
	Span   checkJSONSpan  `json:"span"`
}

func writeCheckJSON(out io.Writer, result check.Result, matched []check.MatchedAction, confidence float64, haveConfidence bool) {
	o := checkJSONOutput{
		Status:          string(result.Status),
		ConsideredCount: result.ConsideredCount,
	}
	for _, c := range result.Constraints {
		o.Constraints = append(o.Constraints, checkJSONConstraint{
			PreceptID: c.PreceptID,
			Outcome:   string(c.Outcome),
			WhyNot:    c.WhyNot,
			RevisitIf: c.RevisitIf,
		})
	}
	for _, w := range result.Warnings {
		o.Warnings = append(o.Warnings, checkJSONWarning{PreceptID: w.PreceptID, Title: w.Title, Action: w.Action})
	}
	if haveConfidence {
		o.Confidence = &confidence
		for _, ma := range matched {
			o.MatchedActions = append(o.MatchedActions, checkJSONMatchedAction{
				Action: ma.Action.Action,
				Params: ma.Action.Params,
				Span:   checkJSONSpan{Start: ma.Span.Start, End: ma.Span.End, Text: ma.Span.Text},
			})
		}
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(o)
}
