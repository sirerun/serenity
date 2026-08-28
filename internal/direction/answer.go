package direction

import (
	"context"
	"fmt"
	"time"

	"github.com/sirerun/serenity/internal/dira/ledger"
)

// Answer transitions a question precept from open to answered -- the
// disposition RFC 0001 §8.3's "unanswered blocking questions" clears
// (T3.8, internal/direction/check's matchQuestions stops warning on it the
// next time Match runs, since it reads state fresh from the store).
//
// It mirrors Confirm's shape: it rejects an entry that is not a question,
// and a question that is not currently open, so answering the same id
// twice errors rather than repeating silently. Only state, updated and
// confirmed_by change -- title, body, tags, edges and alternatives are
// carried through unmodified from what Get returned, the same "never
// edited" rule Confirm and Supersede hold to. Answer is question-only: a
// decision's disposition path is Confirm, and neither method accepts the
// other kind.
func (s *Store) Answer(ctx context.Context, id, answeredBy string, now time.Time) (*ledger.Entry, error) {
	e, err := s.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("direction: answer %s: %w", id, err)
	}
	if e.Kind != ledger.KindQuestion {
		return nil, fmt.Errorf("direction: answer %s: kind is %q, not question", id, e.Kind)
	}
	if e.State != ledger.StateOpen {
		return nil, fmt.Errorf("direction: answer %s: state is %q, not open -- already answered or not a question awaiting disposition", id, e.State)
	}
	e.State = ledger.StateAnswered
	e.ConfirmedBy = answeredBy
	e.Updated = now.UTC().Format(time.RFC3339)
	if err := s.Put(ctx, e); err != nil {
		return nil, fmt.Errorf("direction: answer %s: %w", id, err)
	}
	return e, nil
}
