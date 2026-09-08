package supersede

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

type dirtyPublication struct {
	Results  []Result            `json:"results,omitempty"`
	Version  int                 `json:"version"`
	Decision string              `json:"decision"`
	Path     string              `json:"path"`
	ClaimIDs []string            `json:"claim_ids"`
	Changes  []writer.FileChange `json:"changes"`
	Complete bool                `json:"complete"`
}

// PreviewDirtyEdit checks the captured human page and all shard dependencies.
// Accept preserves this page's prose/fence claims and promotes changed shard
// objects to human assertions. Unsupported changes are errors before disposal.
func (w *Writer) PreviewDirtyEdit(item disposition.Item, actor string, now time.Time) (string, error) {
	if item.State == disposition.StateDisposed {
		return "", fmt.Errorf("dirty edit: already disposed")
	}
	item.Actor, item.DisposedAt = actor, now
	plan, err := w.prepareDirtyPublication(item)
	if err != nil {
		return "", err
	}
	var rec writer.PendingRecord
	if err := json.Unmarshal(item.Payload, &rec); err != nil {
		return "", err
	}
	return fmt.Sprintf("Review human copy of %s (%d shard correction(s)):\n%s\n", plan.Path, len(plan.ClaimIDs), rec.Human), nil
}

// ApplyAndCommitDirtyEdit journals the exact reviewed byte transitions before
// writing. A retry never replans from partial output or redisposes the item.
func (w *Writer) ApplyAndCommitDirtyEdit(ctx context.Context, ds *disposition.Store, item disposition.Item, now time.Time) (string, error) {
	plan, err := w.applyDirtyPublication(ctx, ds, item, now)
	return plan.Decision, err
}

func (w *Writer) applyDirtyPublication(ctx context.Context, ds *disposition.Store, item disposition.Item, now time.Time) (dirtyPublication, error) {
	runtime, err := openPublicationRuntime(w.Fence.Root)
	if err != nil {
		return dirtyPublication{}, err
	}
	defer func() { _ = runtime.Close() }()
	lock, err := lockPublication(runtime)
	if err != nil {
		return dirtyPublication{}, err
	}
	defer func() { _ = lock.Close() }()
	if ds != nil {
		item, err = ds.Get(ctx, item.ID)
		if err != nil {
			return dirtyPublication{}, err
		}
	}
	if item.Kind != disposition.KindDirtyEdit || item.State != disposition.StateDisposed || item.Verdict != disposition.VerdictAccept {
		return dirtyPublication{}, fmt.Errorf("dirty edit: recorded acceptance required")
	}
	raw, err := json.Marshal(struct {
		ID, Actor string
		At        time.Time
		Payload   json.RawMessage
	}{item.ID, item.Actor, item.DisposedAt, item.Payload})
	if err != nil {
		return dirtyPublication{}, err
	}
	sum := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(sum[:])
	idSum := sha256.Sum256([]byte(item.ID))
	name := "dirty-" + hex.EncodeToString(idSum[:]) + ".json"
	if info, err := runtime.Lstat(name); err == nil && !info.Mode().IsRegular() {
		return dirtyPublication{}, fmt.Errorf("dirty edit: unsafe receipt")
	} else if err != nil && !os.IsNotExist(err) {
		return dirtyPublication{}, err
	}
	raw, err = runtime.ReadFile(name)
	var plan dirtyPublication
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &plan); err != nil {
			return dirtyPublication{}, err
		}
		if plan.Version != 1 || plan.Decision != fingerprint || len(plan.Changes) == 0 || plan.Path == "" {
			return dirtyPublication{}, fmt.Errorf("dirty edit: receipt does not match decision")
		}
	case os.IsNotExist(err):
		plan, err = w.prepareDirtyPublication(item)
		if err != nil {
			return dirtyPublication{}, err
		}
		plan.Version, plan.Decision = 1, fingerprint
		if err := savePublication(runtime, name, plan); err != nil {
			return dirtyPublication{}, err
		}
	default:
		return dirtyPublication{}, err
	}
	if !plan.Complete {
		if err := ctx.Err(); err != nil {
			return dirtyPublication{}, err
		}
		protected := false
		for _, change := range plan.Changes {
			if change.Path == plan.Path {
				protected = true
			}
		}
		if !protected {
			return dirtyPublication{}, fmt.Errorf("dirty edit: reviewed page missing from receipt")
		}
		q := writer.NewQueue(nil)
		defer q.Close()
		if err := writer.PublishFiles(q, w.Fence.Root, plan.Changes); err != nil {
			return dirtyPublication{}, err
		}
		// Prose-only edits have identical before/after bytes but still need a commit.
		q.MarkTouched(filepath.Join(w.Fence.Root, filepath.FromSlash(plan.Path)))
		if _, err := writer.Flush(q, w.Fence.Root); err != nil {
			return dirtyPublication{}, fmt.Errorf("dirty edit: publication awaits commit: %w", err)
		}
		plan.Complete = true
		if err := savePublication(runtime, name, plan); err != nil {
			return dirtyPublication{}, err
		}
	}
	if ds != nil {
		if err := ds.RecordPublicationID(ctx, item.ID, fingerprint, now); err != nil {
			return dirtyPublication{}, err
		}
	}
	return plan, nil
}

func (w *Writer) prepareDirtyPublication(item disposition.Item) (dirtyPublication, error) {
	fail := func(err error) (dirtyPublication, error) { return dirtyPublication{}, err }
	if item.Kind != disposition.KindDirtyEdit || !strings.HasPrefix(item.Actor, "human:") || item.Actor == "human:" || item.DisposedAt.IsZero() {
		return fail(fmt.Errorf("dirty edit: human actor and decision time required"))
	}
	var rec writer.PendingRecord
	if err := json.Unmarshal(item.Payload, &rec); err != nil {
		return fail(err)
	}
	page, err := store.ParseEntityBytes([]byte(rec.Human))
	if err != nil {
		return fail(err)
	}
	if !safeClaimPart(page.Entity.Slug) || !safeClaimPart(page.Entity.Type) {
		return fail(fmt.Errorf("dirty edit: invalid entity identity"))
	}
	root := w.Fence.Root
	expected := filepath.Join("brain", "entities", page.Entity.Type, page.Entity.Slug+".md")
	path := filepath.Clean(rec.Path)
	if filepath.IsAbs(path) {
		path, err = filepath.Rel(root, path)
		if err != nil {
			return fail(err)
		}
	}
	if path != expected {
		return fail(fmt.Errorf("dirty edit: captured path must identify this brain's entity page"))
	}
	path = filepath.ToSlash(path)
	paths := []string{path}
	claimDir := filepath.Join(root, "brain", "claims", page.Entity.Slug)
	err = filepath.WalkDir(claimDir, func(name string, entry fs.DirEntry, walkErr error) error {
		if os.IsNotExist(walkErr) && name == claimDir {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return fail(err)
	}
	snapshot, err := writer.SnapshotFiles(root, paths)
	if err != nil {
		return fail(err)
	}
	if !bytes.Equal(snapshot[path], []byte(rec.Human)) {
		return fail(fmt.Errorf("dirty edit: current page changed since capture; preserve the newer edit"))
	}
	type replacement struct{ a, b domain.Claim }
	var replacements []replacement
	rows := map[string]domain.Claim{}
	for _, c := range page.Claims {
		if c.ID == "" || c.SubjectSlug != page.Entity.Slug || !safeClaimPart(c.Family) {
			return fail(fmt.Errorf("dirty edit: invalid claim identity"))
		}
		if _, ok := rows[c.ID]; ok {
			return fail(fmt.Errorf("dirty edit: duplicate claim identity"))
		}
		rows[c.ID] = c
	}
	families, err := w.Shard.Families(page.Entity.Slug)
	if err != nil {
		return fail(err)
	}
	matched := map[string]bool{}
	for _, family := range families {
		heads, err := w.Shard.ResolveHeads(page.Entity.Slug, family)
		if err != nil {
			return fail(err)
		}
		for _, key := range store.HeadKeys(heads) {
			cur := heads[key]
			c, ok := rows[cur.ID]
			if !ok {
				return fail(fmt.Errorf("dirty edit: current shard head %s is absent; resolve additions, removals or stale heads explicitly", cur.ID))
			}
			expectedRow := cur
			expectedRow.SourceRef = "shard"
			comparable := c
			comparable.Object = cur.Object
			comparable.ObjectKey = cur.ObjectKey
			if !sameCanonicalClaim(w.Fence, comparable, expectedRow, page.Entity.Type) {
				return fail(fmt.Errorf("dirty edit: only the object of an existing shard head may change (%s)", cur.ID))
			}
			matched[c.ID] = true
			if c.Object == cur.Object {
				continue
			}
			if strings.TrimSpace(c.Object) == "" || cur.State != domain.StateActive {
				return fail(fmt.Errorf("dirty edit: nonempty correction of an active head required"))
			}
			nc := cur
			nc.Object = c.Object
			nc.ObjectKey = store.NormalizeKey(c.Object)
			nc.Confidence = 1
			nc.Review = false
			nc.Provenance = domain.Provenance{Actor: item.Actor, ObservedAt: item.DisposedAt.UTC(), Meta: map[string]string{"dirty_edit_item_id": item.ID}}
			nc.SourceRef = item.Actor
			// Include the decision identity so a correction back to an older value is
			// a new historical assertion, not a collision with a prior claim.
			hash := sha256.Sum256([]byte(item.ID + "\x00" + cur.ID + "\x00" + c.Object))
			nc.ID = hex.EncodeToString(hash[:])[:store.DefaultIDWidth]
			lines, err := w.Shard.Lines(cur.SubjectSlug, cur.Family)
			if err != nil {
				return fail(err)
			}
			for _, line := range lines {
				if line.ID == nc.ID {
					return fail(fmt.Errorf("dirty edit: correction identity already exists"))
				}
			}
			replacements = append(replacements, replacement{nc, cur})
		}
	}
	for _, c := range page.Claims {
		if w.Config.TierOf(c.Family) == domain.TierShard && !matched[c.ID] {
			return fail(fmt.Errorf("dirty edit: unknown or stale shard row %s", c.ID))
		}
	}
	var results []Result
	changes, err := writer.PlanReviewedFiles(root, snapshot, path, []byte(rec.Human), func(preview string) error {
		q := writer.NewQueue(nil)
		defer q.Close()
		fw, ss := store.NewFenceWriter(preview), store.NewShardStore(preview)
		fw.Vocabulary = w.Fence.Vocabulary
		ss.Vocabulary = w.Shard.Vocabulary
		ss.RolloverBytes = w.Shard.RolloverBytes
		planned := New(q, fw, ss, w.Config)
		planned.EntityType = func(string) string { return page.Entity.Type }
		for _, r := range replacements {
			result, err := planned.Apply(r.a, r.b)
			if err != nil {
				return err
			}
			fenceRel, err := filepath.Rel(preview, result.FencePath)
			if err != nil {
				return err
			}
			shardRel, err := filepath.Rel(preview, result.ShardPath)
			if err != nil {
				return err
			}
			result.FencePath = filepath.ToSlash(fenceRel)
			result.ShardPath = filepath.ToSlash(shardRel)
			results = append(results, result)
		}
		return nil
	})
	if err != nil {
		return fail(err)
	}
	ids := make([]string, 0, len(replacements))
	for _, r := range replacements {
		ids = append(ids, r.a.ID)
	}
	return dirtyPublication{Path: path, ClaimIDs: ids, Changes: changes, Results: results}, nil
}
