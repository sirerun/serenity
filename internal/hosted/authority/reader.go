package authority

import (
	"context"
	"fmt"
	"strings"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

type boundedReader struct {
	store    AtomicStore
	position contracts.JournalPosition
	strict   bool
}

func journalCoordinates(key string) (int64, int64, bool) {
	var g, n int64
	if _, e := fmt.Sscanf(key, "deletion-journal/%d/%d.json", &g, &n); e != nil || g < 1 || g > 10000 || n < 1 || n > 9999999999 || key != contracts.JournalKey(g, n) {
		return 0, 0, false
	}
	return g, n, true
}
func (r *boundedReader) beyond(g, n int64) bool {
	return g > r.position.ActiveGeneration || g == r.position.ActiveGeneration && n > r.position.LastObjectInGeneration.SequenceID
}
func (r *boundedReader) Get(ctx context.Context, key string) ([]byte, bool, error) {
	g, n, ok := journalCoordinates(key)
	if !ok {
		return nil, false, ErrInvalid
	}
	if r.beyond(g, n) {
		return nil, false, nil
	}
	b, found, e := r.store.Get(ctx, "OBJ#"+key)
	if e != nil {
		return nil, false, e
	}
	if len(b) > MaxObjectBytes {
		return nil, false, ErrInvalid
	}
	return b, found, nil
}
func (r *boundedReader) ListAfter(ctx context.Context, prefix, after string, limit int) ([]string, bool, error) {
	if limit < 1 || limit > MaxPageKeys {
		return nil, false, ErrInvalid
	}
	if prefix != "deletion-journal/" {
		var g int64
		if _, e := fmt.Sscanf(prefix, "deletion-journal/%d/", &g); e != nil || g < 1 || g > 10000 || prefix != contracts.JournalPrefix(g) {
			return nil, false, ErrInvalid
		}
	}
	if after != "" {
		if _, _, ok := journalCoordinates(after); !ok || !strings.HasPrefix(after, prefix) {
			return nil, false, ErrInvalid
		}
	}
	cursor := ""
	if after != "" {
		cursor = "OBJ#" + after
	}
	keys, more, e := r.store.List(ctx, "OBJ#"+prefix, cursor, limit)
	if e != nil {
		return nil, false, e
	}
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if !strings.HasPrefix(key, "OBJ#") {
			return nil, false, ErrInvalid
		}
		key = strings.TrimPrefix(key, "OBJ#")
		g, n, ok := journalCoordinates(key)
		if !ok {
			return nil, false, ErrConflict
		}
		if r.beyond(g, n) {
			if r.strict {
				return nil, false, ErrConflict
			}
			return out, false, nil
		}
		out = append(out, key)
	}
	return out, more, nil
}
