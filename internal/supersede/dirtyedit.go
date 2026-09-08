package supersede

import (
	"context"
	"path/filepath"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
)

// ApplyDisposedDirtyEdit retains the lower-level API using the same exact-byte
// journal as inbox publication. Callers with a disposition store should use
// ApplyAndCommitDirtyEdit so the inbox also receives its completion marker.
func (w *Writer) ApplyDisposedDirtyEdit(item disposition.Item, now time.Time) ([]Result, error) {
	plan, err := w.applyDirtyPublication(context.Background(), nil, item, now)
	if err != nil {
		return nil, err
	}
	results := append([]Result(nil), plan.Results...)
	for i := range results {
		results[i].FencePath = filepath.Join(w.Fence.Root, filepath.FromSlash(results[i].FencePath))
		results[i].ShardPath = filepath.Join(w.Fence.Root, filepath.FromSlash(results[i].ShardPath))
	}
	return results, nil
}
