// Package check implements DIRECTION v1's check_plan operation, stage 1
// (RFC 0001 §8.3, ADR 008, ADR 010): a deterministic matcher of structured
// actions against the ledger's active constraint precepts. It is fully
// offline and NEVER calls a model -- classifying free text into the closed
// action set is stage 2 (a later task), a separate concern this package
// keeps out entirely. Match's own doc comment names the test that proves
// it.
package check

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/direction"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/router"
)

// Status is check_plan's top-level verdict (RFC 0001 §8.3, ADR 010). The
// schema verdict is primary: a plan whose actions trigger at least one
// active constraint is StatusViolated even if others pass; a plan matching
// zero active constraints is StatusNoApplicableConstraints, never a bare
// pass -- that distinction is the whole point of the verdict (a caller can
// tell "checked and clean" from "nothing checked").
type Status string

// The three stage-1 verdicts. ADR 010 maps Pass and NoApplicableConstraints
// to `serenity check` exit code 0, and Violated to exit code 2; exit code 1
// (error) is reserved for genuine errors and the free-text `unverified`
// verdict stage 2 introduces, neither of which this package ever returns
// as a Status.
const (
	StatusPass                    Status = "pass"
	StatusViolated                Status = "violated"
	StatusNoApplicableConstraints Status = "no_applicable_constraints"
)

// Outcome is one constraint's verdict against the checked actions.
type Outcome string

const (
	OutcomePass     Outcome = "pass"
	OutcomeViolated Outcome = "violated"
)

// Action is one structured action from a plan's `actions[]` list (RFC 0001
// §8.3): a member of domain.ActionSet plus whatever concrete parameters it
// carries, e.g. {Action: "spend_over", Params: {"amount": 500}}.
type Action struct {
	Action string
	Params map[string]any
}

// ConstraintVerdict is one active constraint's verdict, present only when
// the constraint is applicable -- i.e. at least one checked action shares
// its applies_when action type. A constraint whose action type never
// appears among the checked actions is not applicable and never produces a
// ConstraintVerdict, not even a pass one; that is what keeps
// StatusNoApplicableConstraints meaningfully distinct from a Result with
// every constraint passing.
//
// WhyNot and RevisitIf are populated only when Outcome is OutcomeViolated,
// copied verbatim from the entry's first recorded alternative -- RFC 0001
// §8.3's `violated(precept_id, why_not, revisit_if)`. ADR 008 keeps
// why_not/revisit_if inside alternatives[] for every entry kind, though
// only decisions are schema-required to carry one; a constraint with no
// alternatives on record still reports Violated, with WhyNot/RevisitIf
// left empty rather than inventing a reason nobody wrote down.
type ConstraintVerdict struct {
	PreceptID string
	Outcome   Outcome
	WhyNot    string
	RevisitIf string
}

// Result is check_plan stage 1's verdict over a set of structured actions.
type Result struct {
	Status Status

	// Constraints holds one entry per applicable active constraint, in
	// ledger id order.
	Constraints []ConstraintVerdict

	// ConsideredCount is the number of active constraint entries the
	// matcher examined, whether or not any were applicable -- the count
	// RFC 0001 §8.3 / ADR 010 attach to a no_applicable_constraints
	// verdict ("naming how many active constraints matched nothing").
	ConsideredCount int
}

// ErrUnknownAction marks an Action naming something outside
// domain.ActionSet. It is direction.ErrUnknownAction, the same error
// applies_when parsing uses for the identical condition on the constraint
// side, so a caller can errors.Is against one sentinel regardless of which
// side of the match produced it.
var ErrUnknownAction = direction.ErrUnknownAction

// Matcher is DIRECTION v1's check_plan stage 1: it matches structured
// actions against the active constraint precepts in a ledger, entirely
// offline.
type Matcher struct {
	store  ledger.Store
	router *router.Router
}

// New builds a Matcher over store. rtr is stored, not called: check_plan's
// eventual free-text stage (RFC 0001 §8.3 stage 2, classification via the
// local-cheap tier) shares this same Matcher and will read it back through
// Router. Match -- stage 1, structured actions only -- never calls it;
// TestMatch_NeverCallsRouter proves this by wiring rtr to a Provider whose
// Send panics unconditionally and confirming Match completes normally.
func New(store ledger.Store, rtr *router.Router) *Matcher {
	return &Matcher{store: store, router: rtr}
}

// Router returns the router.Router passed to New. Match never calls it;
// it is reserved for check_plan's free-text stage.
func (m *Matcher) Router() *router.Router { return m.router }

// Match checks actions against every active constraint precept in the
// ledger, deterministically and fully offline (RFC 0001 §8.3 stage 1). It
// never invokes m.router.
//
// Every action.Action must be a member of domain.ActionSet; Match rejects
// the whole call with an error wrapping ErrUnknownAction otherwise, before
// reading the ledger. A constraint entry is considered only when it is
// kind=constraint and state=active; among those, one is applicable only
// when its applies_when clause names an action type present in actions --
// a constraint whose clause is absent, unparseable in a way that is not
// simply "no clause" (direction.ErrNoAppliesWhenBlock), or whose action
// type is not in play, is otherwise skipped (the first two silently, since
// most entries legitimately carry no machine-checkable clause; the last
// contributes to ConsideredCount but not to Result.Constraints). A
// genuinely malformed applies_when block on an active constraint --
// something Validate at write time should have already excluded -- is
// reported as an error rather than silently treated as inapplicable,
// because an unenforceable active constraint is exactly the failure this
// matcher exists to never hide.
func (m *Matcher) Match(ctx context.Context, actions []Action) (Result, error) {
	for _, a := range actions {
		if !validAction(a.Action) {
			return Result{}, fmt.Errorf("check: %w: %q", ErrUnknownAction, a.Action)
		}
	}

	infos, err := m.store.List(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("check: listing ledger: %w", err)
	}

	result := Result{Status: StatusPass}
	for _, info := range infos {
		entry, err := m.store.Get(ctx, info.ID)
		if err != nil {
			return Result{}, fmt.Errorf("check: reading %s: %w", info.ID, err)
		}
		if entry.Kind != ledger.KindConstraint || entry.State != ledger.StateActive {
			continue
		}
		result.ConsideredCount++

		block, err := direction.ParseAppliesWhen([]byte(entry.Body))
		if err != nil {
			if errors.Is(err, direction.ErrNoAppliesWhenBlock) {
				continue
			}
			return Result{}, fmt.Errorf("check: %s carries a malformed applies_when block: %w", entry.ID, err)
		}

		verdict, applicable, err := matchConstraint(entry, block, actions)
		if err != nil {
			return Result{}, fmt.Errorf("check: %s: %w", entry.ID, err)
		}
		if !applicable {
			continue
		}
		result.Constraints = append(result.Constraints, verdict)
		if verdict.Outcome == OutcomeViolated {
			result.Status = StatusViolated
		}
	}

	if len(result.Constraints) == 0 {
		result.Status = StatusNoApplicableConstraints
	}
	return result, nil
}

// matchConstraint evaluates one constraint entry's applies_when block
// against every checked action. applicable is false when none of actions
// share the clause's action type, in which case verdict is the zero value
// and must not be reported. Otherwise verdict.Outcome is OutcomeViolated
// as soon as any matching action's params trigger the clause, and
// OutcomePass when every matching action's params fail to.
func matchConstraint(entry *ledger.Entry, block *direction.AppliesWhenBlock, actions []Action) (verdict ConstraintVerdict, applicable bool, err error) {
	triggered := false
	for _, a := range actions {
		if a.Action != block.Action {
			continue
		}
		applicable = true
		ok, err := paramsTrigger(block.Params, a.Params)
		if err != nil {
			return ConstraintVerdict{}, false, err
		}
		if ok {
			triggered = true
		}
	}
	if !applicable {
		return ConstraintVerdict{}, false, nil
	}

	verdict = ConstraintVerdict{PreceptID: entry.ID, Outcome: OutcomePass}
	if triggered {
		verdict.Outcome = OutcomeViolated
		if len(entry.Alternatives) > 0 {
			verdict.WhyNot = entry.Alternatives[0].WhyNot
			verdict.RevisitIf = entry.Alternatives[0].RevisitIf
		}
	}
	return verdict, true, nil
}

// paramsTrigger reports whether action's params satisfy every key in
// clause -- an empty or nil clause always triggers, since the constraint
// then fires on the action type alone. A clause key absent from action
// does not trigger (false, nil error): the action simply did not carry
// enough information to match, which is ordinary (not every action names
// every param a constraint cares about), not an error. An unevaluable
// clause -- an unknown operator, or a numeric operator applied to a
// non-numeric operand -- is reported as an error: on an ACTIVE constraint,
// "cannot tell" must never be silently read as "does not apply".
func paramsTrigger(clause, action map[string]any) (bool, error) {
	for key, want := range clause {
		got, present := action[key]
		if !present {
			return false, nil
		}
		ok, err := paramMatches(want, got)
		if err != nil {
			return false, fmt.Errorf("param %q: %w", key, err)
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// paramMatches evaluates one clause param against the action's value for
// the same key. want is either a literal, matched by value equality
// (numeric-aware so YAML's int 500 and a caller's float64(500) agree), or
// a map naming one or more comparison operators (gte, gt, lte, lt, eq, ne)
// that must ALL hold.
func paramMatches(want, got any) (bool, error) {
	ops, isOps := want.(map[string]any)
	if !isOps {
		return valuesEqual(want, got), nil
	}
	if len(ops) == 0 {
		return false, errors.New("comparison map has no operators")
	}
	for op, operand := range ops {
		ok, err := applyOperator(op, operand, got)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// applyOperator evaluates one named comparison of got against operand.
// gte/gt/lte/lt require both sides to be numeric and error otherwise --
// see paramsTrigger's doc comment on why an unevaluable condition must
// never resolve to "does not apply".
func applyOperator(op string, operand, got any) (bool, error) {
	switch op {
	case "eq":
		return valuesEqual(operand, got), nil
	case "ne":
		return !valuesEqual(operand, got), nil
	case "gte", "gt", "lte", "lt":
		wantN, ok := toFloat64(operand)
		if !ok {
			return false, fmt.Errorf("operator %q needs a numeric comparison value, got %T", op, operand)
		}
		gotN, ok := toFloat64(got)
		if !ok {
			return false, fmt.Errorf("operator %q needs a numeric action value, got %T", op, got)
		}
		switch op {
		case "gte":
			return gotN >= wantN, nil
		case "gt":
			return gotN > wantN, nil
		case "lte":
			return gotN <= wantN, nil
		default: // "lt"
			return gotN < wantN, nil
		}
	default:
		return false, fmt.Errorf("unknown comparison operator %q", op)
	}
}

// valuesEqual compares two applies_when/action values. Both numeric
// (regardless of concrete Go type) compare by numeric value, so a YAML-
// decoded int and a caller-constructed float64 representing the same
// number are equal; otherwise it falls back to reflect.DeepEqual, which
// handles strings, bools, and nested maps/slices without risking a panic
// on an uncomparable type.
func valuesEqual(a, b any) bool {
	if af, ok := toFloat64(a); ok {
		bf, ok := toFloat64(b)
		return ok && af == bf
	}
	return reflect.DeepEqual(a, b)
}

// toFloat64 converts v to float64 if it is one of the numeric kinds YAML
// decoding or a caller can plausibly produce. ok is false for anything
// else.
func toFloat64(v any) (f float64, ok bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

// validAction reports whether action is a member of the closed action set
// constraints may guard (RFC 0001 §7.3, domain.ActionSet). It mirrors
// internal/direction's unexported validAction: both packages check the
// same closed set independently rather than share a helper across a
// package boundary neither owns.
func validAction(action string) bool {
	for _, a := range domain.ActionSet {
		if a == action {
			return true
		}
	}
	return false
}
