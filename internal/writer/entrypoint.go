package writer

import (
	"encoding/json"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

// Fence writes an entity page through the queue -- the only sanctioned
// entry point for FenceWriter.WriteEntity after M0. It returns the page
// path and the rendered bytes that were written (WriteEntity itself
// no-ops when the bytes are unchanged, per store's write-skip contract).
func Fence(q *Queue, fw *store.FenceWriter, p *store.EntityPage) (path string, rendered []byte, err error) {
	path = fw.PathFor(p.Entity.Type, p.Entity.Slug)
	res := q.Submit(Job{
		Path: path,
		Render: func() ([]byte, error) {
			b, err := fw.RenderEntity(p)
			if err != nil {
				return nil, err
			}
			if _, err := fw.WriteEntity(p); err != nil {
				return nil, err
			}
			return b, nil
		},
	})
	return path, res.Bytes, res.Err
}

// Shard appends one claim through the queue -- the only sanctioned entry
// point for ShardStore.Append after M0. It returns the shard path and the
// JSON line that was appended. IDs are derived here (not left to Append)
// so the returned line always reflects exactly what landed on disk.
func Shard(q *Queue, ss *store.ShardStore, c domain.Claim) (path string, line []byte, err error) {
	if c.ObjectKey == "" {
		c.ObjectKey = store.NormalizeKey(c.Object)
	}
	if c.ID == "" {
		c.ID = store.DerivedID(c.SubjectSlug, c.Predicate, c.ObjectKey, c.ValidFrom, c.Provenance.SourceSHA256, store.DefaultIDWidth)
	}
	path = ss.PathFor(c.SubjectSlug, c.Family)
	res := q.Submit(Job{
		Path: path,
		Render: func() ([]byte, error) {
			if err := ss.Append(c); err != nil {
				return nil, err
			}
			return json.Marshal(c)
		},
	})
	return path, res.Bytes, res.Err
}
