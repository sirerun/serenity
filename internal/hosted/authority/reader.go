package authority

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

type boundedReader struct {
	store    AtomicStore
	position contracts.JournalPosition
	strict   bool
	mu       sync.Mutex
	cache    map[string][]byte
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
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	g, n, ok := journalCoordinates(key)
	if !ok {
		return nil, false, ErrInvalid
	}
	if r.beyond(g, n) {
		return nil, false, nil
	}
	r.mu.Lock()
	cached, found := r.cache[key]
	cached = bytes.Clone(cached)
	r.mu.Unlock()
	if found {
		return cached, true, nil
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
	page, e := r.listPage(ctx, prefix, after, limit)
	return page.Keys, page.More, e
}
func (r *boundedReader) listPage(ctx context.Context, prefix, after string, limit int) (Response, error) {
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	if limit < 1 || limit > MaxPageKeys {
		return Response{}, ErrInvalid
	}
	if prefix != "deletion-journal/" {
		var g int64
		if _, e := fmt.Sscanf(prefix, "deletion-journal/%d/", &g); e != nil || g < 1 || g > 10000 || prefix != contracts.JournalPrefix(g) {
			return Response{}, ErrInvalid
		}
	}
	if after != "" {
		if _, _, ok := journalCoordinates(after); !ok || !strings.HasPrefix(after, prefix) {
			return Response{}, ErrInvalid
		}
	}
	cursor := ""
	if after != "" {
		cursor = "OBJ#" + after
	}
	objects, more, e := r.store.ListObjects(ctx, "OBJ#"+prefix, cursor, limit)
	if e != nil {
		return Response{}, e
	}
	out := Response{More: more}
	encodedBytes := 128 // conservative field/array envelope; final shape checked below
	last := after
	for _, item := range objects {
		if !strings.HasPrefix(item.Key, "OBJ#") {
			return Response{}, ErrInvalid
		}
		key := strings.TrimPrefix(item.Key, "OBJ#")
		g, n, ok := journalCoordinates(key)
		if !ok || key <= last || !strings.HasPrefix(key, prefix) || len(item.Body) > MaxObjectBytes {
			return Response{}, ErrConflict
		}
		last = key
		if r.beyond(g, n) {
			if r.strict {
				return Response{}, ErrConflict
			}
			out.More = false
			break
		}

		encodedItem, err := json.Marshal(ImmutableItem{Key: key, Body: item.Body})
		if err != nil {
			return Response{}, err
		}
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return Response{}, err
		}
		added := len(encodedItem) + len(encodedKey) + 2
		if encodedBytes+added > MaxPageBytes {
			if len(out.Keys) == 0 {
				return Response{}, ErrInvalid
			}
			out.More = true
			break
		}
		encodedBytes += added
		out.Keys = append(out.Keys, key)
		out.Objects = append(out.Objects, ImmutableItem{Key: key, Body: item.Body})

	}
	if out.More && len(out.Keys) == 0 {
		return Response{}, ErrInvalid
	}
	encoded, err := json.Marshal(out)
	if err != nil || len(encoded) > MaxPageBytes {
		return Response{}, ErrInvalid
	}
	cache := make(map[string][]byte, len(out.Objects))
	for _, item := range out.Objects {
		cache[item.Key] = bytes.Clone(item.Body)
	}
	r.mu.Lock()
	r.cache = cache
	r.mu.Unlock()
	return out, nil
}
