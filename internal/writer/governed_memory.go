package writer

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/sirerun/serenity/internal/store"
)

// GovernedMemory commits one immutable command receipt through the canonical
// writer and Git publication fence. The caller holds the runtime mutation lock.
func GovernedMemory(ctx context.Context, q *Queue, s *store.SourceStore, root string, rec store.GovernedRecord) error {
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	result := q.SubmitAndFlush(ctx, root, Job{Kind: "governed-memory", Render: func() ([]byte, error) {
		projection, err := store.LoadGovernedMemory(s)
		if err != nil {
			return nil, err
		}
		if old, ok := projection.Commands[rec.Response.CommandID]; ok {
			if old.RequestDigest != rec.RequestDigest {
				return nil, fmt.Errorf("governed command conflict")
			}
			sha := projection.SourceIDs[rec.Response.CommandID]
			q.MarkTouched(filepath.Join(s.DirFor(sha), "bytes"))
			q.MarkTouched(filepath.Join(s.DirFor(sha), "meta.yaml"))
			return nil, nil
		}
		if projection.BrainID != "" && projection.BrainID != rec.Response.BrainID {
			return nil, fmt.Errorf("governed brain binding conflict")
		}
		src, err := s.PutGovernedMemory(rec)
		if err == nil {
			q.MarkTouched(filepath.Join(s.DirFor(src.SHA256), "bytes"))
			q.MarkTouched(filepath.Join(s.DirFor(src.SHA256), "meta.yaml"))
		}
		return raw, err
	}})
	return result.Result.Err
}
