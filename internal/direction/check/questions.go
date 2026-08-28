package check

import (
	"context"
	"errors"
	"fmt"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/direction"
)

// QuestionWarning is check_plan's report of one open question precept that
// blocks a checked action (RFC 0001 §8.3, T3.8). It is deliberately a
// warning, never a verdict: an unanswered blocking question is "kazi
// structurally cannot see" this class of blockage (entry.schema.json's own
// description of the question kind), so it surfaces as extra context on
// the way to a human, not as a StatusViolated the way a triggered
// constraint is.
type QuestionWarning struct {
	// PreceptID is the qst-NNNN entry id, so a caller can look the
	// question up -- e.g. in a brief's "open blocking questions" section,
	// a later consumer of this same Result (RFC 0001 §8.3's brief()).
	PreceptID string

	// Title is the entry's title, copied verbatim so a caller can render
	// the warning without a second ledger read.
	Title string

	// Action is the checked action this question blocks.
	Action string
}

// matchQuestions scans every open question entry in the ledger and
// returns one QuestionWarning per entry whose serenity:applies_when body
// block (ADR 008's mechanism, T3.2 -- shared verbatim with constraints;
// direction.ParseAppliesWhen has no kind-specific behavior) names an
// action present in actions and whose params clause the matching action's
// params trigger (paramsTrigger, the same params DSL Match uses for
// constraints -- a question may scope its block to e.g. {amount: {gte:
// 1000}} exactly as a constraint would).
//
// Answering a question -- direction.Store.Answer flipping its state from
// open to answered, the question-kind analog of Store.Confirm -- removes
// it from this scan on the next call: matchQuestions reads State fresh
// from the store every time, so it never caches "still open" past a
// disposition.
//
// A question with no applies_when block (direction.ErrNoAppliesWhenBlock)
// is simply not machine-checkable yet -- most questions legitimately have
// none -- and is skipped, the same accommodation Match makes for
// constraints. A malformed block is reported as an error rather than
// silently skipped: an unenforceable blocking question must never be
// mistaken for one that was checked and found not to apply, mirroring
// Match's own posture on constraints.
func (m *Matcher) matchQuestions(ctx context.Context, actions []Action) ([]QuestionWarning, error) {
	infos, err := m.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("check: listing ledger for questions: %w", err)
	}

	var warnings []QuestionWarning
	for _, info := range infos {
		entry, err := m.store.Get(ctx, info.ID)
		if err != nil {
			return nil, fmt.Errorf("check: reading %s: %w", info.ID, err)
		}
		if entry.Kind != ledger.KindQuestion || entry.State != ledger.StateOpen {
			continue
		}

		block, err := direction.ParseAppliesWhen([]byte(entry.Body))
		if err != nil {
			if errors.Is(err, direction.ErrNoAppliesWhenBlock) {
				continue
			}
			return nil, fmt.Errorf("check: %s carries a malformed applies_when block: %w", entry.ID, err)
		}

		for _, a := range actions {
			if a.Action != block.Action {
				continue
			}
			triggered, err := paramsTrigger(block.Params, a.Params)
			if err != nil {
				return nil, fmt.Errorf("check: %s: %w", entry.ID, err)
			}
			if triggered {
				warnings = append(warnings, QuestionWarning{
					PreceptID: entry.ID,
					Title:     entry.Title,
					Action:    a.Action,
				})
				break
			}
		}
	}
	return warnings, nil
}
