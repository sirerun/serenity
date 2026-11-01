package serenity

import (
	"context"
	"errors"

	"github.com/kazi-org/dira/engine"
	"github.com/sirerun/serenity/internal/direction"
)

// LexicalVerdict is Dira's canonical privacy-filtered lexical result.
// Conflicts are results, not operational errors. The caller's plan is echoed
// and may be sensitive; source-citation redaction does not make it publishable.
type LexicalVerdict = engine.CheckResult

// CheckLexical evaluates plan through the embedded Dira engine using this
// Brain's existing read-only ledger. It opens no Dira executable or secondary
// ledger and grants no authority. Parent inheritance is not evaluated.
// CheckPlan retains its separate structured-action semantics and result type.
func (b *Brain) CheckLexical(ctx context.Context, plan string) (*LexicalVerdict, error) {
	if b == nil || b.store == nil {
		return nil, errors.New("serenity: lexical check requires an open brain")
	}
	return direction.CheckLexical(ctx, b.store, plan)
}
