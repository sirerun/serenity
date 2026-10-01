package writer

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

// SourceTombstone is the serialized entry point for deleting an ordinary
// source (ADR 019): one queue job records the tombstone event, deletes the
// source's FTS and vector rows, and removes its bytes and meta.yaml, and
// marks every path so the next Flush commits the deletion and the event
// together.
type SourceTombstone struct {
	Queue   *Queue
	Sources *store.SourceStore
	Shards  *store.ShardStore
	Index   IndexPurger
}

// Tombstone deletes the source named by sha and returns the shard claims
// that cite it, for the retraction path to act on.
func (w *SourceTombstone) Tombstone(sha string, now time.Time) ([]domain.Claim, error) {
	if w.Queue == nil || w.Sources == nil {
		return nil, fmt.Errorf("writer: tombstone dependencies unavailable")
	}
	var citing []domain.Claim
	res := w.Queue.Submit(Job{Render: func() ([]byte, error) {
		out, err := w.Sources.TombstoneAt(sha, w.Shards, now)
		if out.Event.SHA256 != "" {
			dir := w.Sources.DirFor(out.Event.SHA256)
			w.Queue.MarkTouched(filepath.Join(dir, "bytes"))
			w.Queue.MarkTouched(filepath.Join(dir, "meta.yaml"))
		}
		for _, path := range out.Removed {
			w.Queue.MarkTouched(path)
		}
		if err != nil {
			return nil, err
		}
		if w.Index != nil {
			if err := w.Index.PurgeSource(context.Background(), sha); err != nil {
				return nil, fmt.Errorf("writer: purge tombstoned source index rows: %w", err)
			}
		}
		root, err := filepath.Abs(w.Sources.Root)
		if err != nil {
			return nil, fmt.Errorf("writer: resolve brain root for source history rewrite: %w", err)
		}
		sourceDir, err := filepath.Abs(w.Sources.DirFor(sha))
		if err != nil {
			return nil, fmt.Errorf("writer: resolve tombstoned source path: %w", err)
		}
		relPath, err := filepath.Rel(root, sourceDir)
		if err != nil {
			return nil, fmt.Errorf("writer: resolve tombstoned source path: %w", err)
		}
		if err := rewriteForgottenPath(root, filepath.ToSlash(relPath)); err != nil {
			return nil, fmt.Errorf("writer: rewrite tombstoned source history: %w", err)
		}
		citing = out.Citing
		return nil, nil
	}})
	if res.Err != nil {
		return nil, res.Err
	}
	return citing, nil
}
