package direction

import (
	"context"
	"errors"
	"fmt"

	"github.com/kazi-org/dira/engine"
	"github.com/kazi-org/dira/ledger"
)

// LexicalReader is the read-only subset of the existing authorized ledger.
// It selects no path, backend, credential, parent ledger or writer.
type LexicalReader interface {
	List(context.Context) ([]ledger.EntryInfo, error)
	Get(context.Context, string) (*ledger.Entry, error)
}

// CheckLexical runs Dira's canonical offline lexical check, including its
// privacy-filtered JSON result. It is distinct from Serenity's structured
// check_plan contract. A compliant lexical verdict is not execution authority
// or proof that unprovided parent ledgers or all policies were checked.
func CheckLexical(ctx context.Context, reader LexicalReader, plan string) (*engine.CheckResult, error) {
	if reader == nil {
		return nil, errors.New("direction: lexical reader is required")
	}
	return engine.Check(ctx, lexicalEntries{reader}, plan)
}

type lexicalEntries struct{ reader LexicalReader }

func (r lexicalEntries) Entries(ctx context.Context) ([]*ledger.Entry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ids, err := r.reader.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list lexical entries: %w", err)
	}
	entries := make([]*ledger.Entry, 0, len(ids))
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entry, err := r.reader.Get(ctx, id.ID)
		if err != nil {
			return nil, fmt.Errorf("read lexical entry: %w", err)
		}
		if entry == nil || entry.ID != id.ID {
			return nil, errors.New("direction: lexical entry identity mismatch")
		}
		entries = append(entries, entry)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
