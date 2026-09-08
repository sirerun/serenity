package ingest

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// Write prepares the entire observation batch before changing canonical files.
// Real tier writers operate in the private snapshot; publication preserves human
// bytes and records every actual shard segment for the caller's Git Flush.
func (w *Writer) Write(obs []domain.Observation) (Stats, error) {
	if len(obs) == 0 {
		return Stats{}, nil
	}
	types := map[string]string{}
	paths := []string{}
	for _, o := range obs {
		if !safePart(o.SubjectSlug) || !safePart(o.Predicate) {
			return Stats{}, fmt.Errorf("ingest: unsafe observation identity")
		}
		if _, ok := types[o.SubjectSlug]; ok {
			continue
		}
		pages, err := filepath.Glob(filepath.Join(w.Fence.Root, "brain", "entities", "*", o.SubjectSlug+".md"))
		if err != nil {
			return Stats{}, err
		}
		if len(pages) > 1 {
			return Stats{}, fmt.Errorf("ingest: ambiguous entity type for %s", o.SubjectSlug)
		}
		entityType := DefaultEntityType
		if w.EntityType != nil {
			if named := w.EntityType(o.SubjectSlug); named != "" {
				entityType = named
			}
		}
		if len(pages) == 1 {
			entityType = filepath.Base(filepath.Dir(pages[0]))
			paths = append(paths, pages[0])
		}
		if !safePart(entityType) {
			return Stats{}, fmt.Errorf("ingest: unsafe entity type")
		}
		types[o.SubjectSlug] = entityType
		shardDir := filepath.Join(w.Fence.Root, "brain", "claims", o.SubjectSlug)
		err = filepath.WalkDir(shardDir, func(path string, entry fs.DirEntry, walkErr error) error {
			if errors.Is(walkErr, fs.ErrNotExist) && path == shardDir {
				return nil
			}
			if walkErr != nil {
				return walkErr
			}
			if !entry.IsDir() {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			return Stats{}, err
		}
	}
	rels := make([]string, 0, len(paths))
	for _, path := range paths {
		rel, err := filepath.Rel(w.Fence.Root, path)
		if err != nil {
			return Stats{}, err
		}
		rels = append(rels, filepath.ToSlash(rel))
	}
	snapshot, err := writer.SnapshotFiles(w.Fence.Root, rels)
	if err != nil {
		return Stats{}, err
	}
	stats := Stats{}
	changes, err := writer.PlanFiles(w.Fence.Root, snapshot, func(preview string) error {
		q := writer.NewQueue(nil)
		defer q.Close()
		fw, ss := store.NewFenceWriter(preview), store.NewShardStore(preview)
		fw.Vocabulary = w.Fence.Vocabulary
		ss.Vocabulary = w.Shard.Vocabulary
		ss.RolloverBytes = w.Shard.RolloverBytes
		planned := New(q, fw, ss, w.Config)
		planned.EntityType = func(slug string) string { return types[slug] }
		var err error
		stats, err = planned.writeObservations(obs)
		return err
	})
	if err != nil {
		return Stats{}, err
	}
	if err := writer.PublishFiles(w.Queue, w.Fence.Root, changes); err != nil {
		return Stats{}, err
	}
	return stats, nil
}

func safePart(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, "/\\\x00*?[]")
}
