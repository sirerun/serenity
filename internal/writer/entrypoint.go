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
//
// Before submitting, it runs the dirty-tree guard (T0.4, ADR 004): if the
// page has an uncommitted human edit, the write is paused rather than
// raced against it -- err is ErrDirtyTree, rendered is nil, and the file
// on disk is untouched. Both sides land at PendingPath(fw.Root, p.Entity.Slug).
func Fence(q *Queue, fw *store.FenceWriter, p *store.EntityPage) (path string, rendered []byte, err error) {
	path = fw.PathFor(p.Entity.Type, p.Entity.Slug)
	machine, err := fw.RenderEntity(p)
	if err != nil {
		return path, nil, err
	}
	rendered, err = guard(q, fw.Root, path, p.Entity.Slug, machine, func() ([]byte, error) {
		if _, err := fw.WriteEntity(p); err != nil {
			return nil, err
		}
		return machine, nil
	})
	return path, rendered, err
}

// Shard appends one claim through the queue -- the only sanctioned entry
// point for ShardStore.Append after M0. It returns the shard path and the
// JSON line that was appended. IDs are derived here (not left to Append)
// so the returned line always reflects exactly what landed on disk.
//
// Before submitting, it runs the dirty-tree guard (T0.4, ADR 004): see
// Fence. The pending key is "<subject-slug>-<family>" since one entity
// can have several shard families, each independently guarded.
func Shard(q *Queue, ss *store.ShardStore, c domain.Claim) (path string, line []byte, err error) {
	if c.ObjectKey == "" {
		c.ObjectKey = store.NormalizeKey(c.Object)
	}
	if c.ID == "" {
		c.ID = store.DerivedID(c.SubjectSlug, c.Predicate, c.ObjectKey, c.ValidFrom, c.Provenance.SourceSHA256, store.DefaultIDWidth)
	}
	path = ss.PathFor(c.SubjectSlug, c.Family)
	machine, err := json.Marshal(c)
	if err != nil {
		return path, nil, err
	}
	key := c.SubjectSlug + "-" + c.Family
	line, err = guard(q, ss.Root, path, key, machine, func() ([]byte, error) {
		if err := ss.Append(c); err != nil {
			return nil, err
		}
		return machine, nil
	})
	return path, line, err
}
