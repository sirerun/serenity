package ingest

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// Write prepares the entire observation batch before changing canonical files.
// Real tier writers operate in the private snapshot; publication preserves human
// bytes and records every actual shard segment for the caller's Git Flush.
func (w *Writer) Write(obs []domain.Observation) (Stats, error) {
	claims := make([]domain.Claim, len(obs))
	for i, o := range obs {
		claims[i] = ClaimFromObservation(o)
	}
	return w.writeBatch(obs, claims)
}

func (w *Writer) writeBatch(obs []domain.Observation, claims []domain.Claim) (Stats, error) {
	if len(obs) == 0 {
		return Stats{}, nil
	}
	types, snapshot, kept, rejected, err := w.snapshotObservations(obs)
	if err != nil {
		return Stats{}, err
	}
	if len(kept) == 0 {
		return Stats{Rejected: rejected}, nil
	}
	safe := make([]domain.Observation, 0, len(kept))
	safeClaims := make([]domain.Claim, 0, len(kept))
	for _, i := range kept {
		safe = append(safe, obs[i])
		safeClaims = append(safeClaims, claims[i])
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
		stats, err = planned.writeObservations(safe, safeClaims)
		return err
	})
	if err != nil {
		return Stats{}, err
	}
	if err := writer.PublishFiles(w.Queue, w.Fence.Root, changes); err != nil {
		return Stats{}, err
	}
	stats.Rejected = rejected
	return stats, nil
}

// snapshotObservations resolves each observation's entity type and reads
// the canonical files its publication depends on. An observation whose
// subject is not a canonical slug (domain.ValidSlug) or whose predicate is
// not a safe path segment is dropped before any path is built from it and
// counted in rejected; the returned safe slice holds the rest in input
// order (SEC-H03, FUN-03). Errors are reserved for the brain itself being
// in an unsafe state (an ambiguous or non-canonical existing entity type).
func (w *Writer) snapshotObservations(obs []domain.Observation) (types map[string]string, snapshot map[string][]byte, kept []int, rejected int, err error) {
	types = map[string]string{}
	paths := []string{}
	for i, o := range obs {
		if !domain.ValidSlug(o.SubjectSlug) || !safePart(o.Predicate) {
			rejected++
			continue
		}
		kept = append(kept, i)
		if _, ok := types[o.SubjectSlug]; ok {
			continue
		}
		pages, err := filepath.Glob(filepath.Join(w.Fence.Root, "brain", "entities", "*", o.SubjectSlug+".md"))
		if err != nil {
			return nil, nil, nil, 0, err
		}
		if len(pages) > 1 {
			return nil, nil, nil, 0, fmt.Errorf("ingest: ambiguous entity type for %s", o.SubjectSlug)
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
		if !domain.ValidSlug(entityType) {
			return nil, nil, nil, 0, fmt.Errorf("ingest: unsafe entity type")
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
			return nil, nil, nil, 0, err
		}
	}
	rels := make([]string, 0, len(paths))
	for _, path := range paths {
		rel, err := filepath.Rel(w.Fence.Root, path)
		if err != nil {
			return nil, nil, nil, 0, err
		}
		rels = append(rels, filepath.ToSlash(rel))
	}
	snapshot, err = writer.SnapshotFiles(w.Fence.Root, rels)
	return types, snapshot, kept, rejected, err
}

// snapshotStrict is snapshotObservations for callers whose observations
// are not model output -- a human assertion or a proposal built from
// canonical claims -- where a non-canonical identity is a caller error,
// not a candidate to drop.
func (w *Writer) snapshotStrict(obs []domain.Observation) (map[string]string, map[string][]byte, error) {
	types, snapshot, _, rejected, err := w.snapshotObservations(obs)
	if err != nil {
		return nil, nil, err
	}
	if rejected > 0 {
		return nil, nil, fmt.Errorf("ingest: unsafe observation identity")
	}
	return types, snapshot, nil
}

// safePart accepts one literal path segment for a predicate or family
// name: the controlled vocabulary uses underscores, so the slug grammar
// does not apply, but a separator, dot-segment, NUL, glob or control
// character never does.
func safePart(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, "/\\\x00*?[]") && strings.IndexFunc(value, unicode.IsControl) < 0
}
