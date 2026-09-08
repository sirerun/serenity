package disposition

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/writer"
)

// ImportPending transfers complete paused-write records into immutable review
// items. Rename claims an input before staging it, so consuming an older record
// cannot unlink a newer producer write under the same filename. Claimed files
// survive storage errors/crashes and are drained before new arrivals on retry.
func (s *Store) ImportPending(ctx context.Context, root string, now time.Time) (int, error) {
	brain, err := os.OpenRoot(root)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer func() { _ = brain.Close() }()
	for _, name := range []string{".serenity", filepath.Join(".serenity", "pending")} {
		info, err := brain.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		if err != nil {
			return 0, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return 0, fmt.Errorf("disposition: pending path is not a real directory: %s", name)
		}
	}
	pending, err := brain.OpenRoot(filepath.Join(".serenity", "pending"))
	if err != nil {
		return 0, err
	}
	defer func() { _ = pending.Close() }()
	if err := pending.Mkdir(".claimed", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return 0, err
	}
	info, err := pending.Lstat(".claimed")
	if err != nil {
		return 0, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return 0, errors.New("disposition: invalid claimed pending directory")
	}
	if err := syncPendingDir(pending, "."); err != nil {
		return 0, err
	}
	count := 0
	consume := func(dir, name string) error {
		path := filepath.Join(dir, name)
		info, err := pending.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || filepath.Ext(name) != ".json" {
			return fmt.Errorf("disposition: invalid pending record %s", name)
		}
		data, err := pending.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		var rec writer.PendingRecord
		if err := json.Unmarshal(data, &rec); err != nil {
			return fmt.Errorf("disposition: decode pending record %s: %w", name, err)
		}
		key := strings.TrimSuffix(name, ".json")
		if err := s.stagePending(ctx, key, rec, now); err != nil {
			return err
		}
		if err := pending.Remove(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if err := syncPendingDir(pending, dir); err != nil {
			return err
		}
		count++
		return nil
	}
	cleanup := func(dir string) error {
		if err := pending.Remove(dir); err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, fs.ErrExist) {
			return err
		}
		return syncPendingDir(pending, ".claimed")
	}
	// Multiple consumers can encounter the same claimed input. CreateOnce and
	// unlink's not-found handling make that harmless without a process-local lock.
	dirs, err := fs.ReadDir(pending.FS(), ".claimed")
	if err != nil {
		return count, err
	}
	for _, de := range dirs {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		if !de.IsDir() {
			return count, errors.New("disposition: invalid claimed pending entry")
		}
		dir := filepath.Join(".claimed", de.Name())
		files, err := fs.ReadDir(pending.FS(), filepath.ToSlash(dir))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return count, err
		}
		for _, file := range files {
			if err := consume(dir, file.Name()); err != nil {
				return count, err
			}
		}
		if err := cleanup(dir); err != nil {
			return count, err
		}
	}
	entries, err := fs.ReadDir(pending.FS(), ".")
	if err != nil {
		return count, err
	}
	for _, de := range entries {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		if de.IsDir() || filepath.Ext(de.Name()) != ".json" {
			continue
		}
		if !de.Type().IsRegular() {
			return count, fmt.Errorf("disposition: pending input is not regular: %s", de.Name())
		}
		id, err := newEventID()
		if err != nil {
			return count, err
		}
		dir := filepath.Join(".claimed", id)
		if err := pending.Mkdir(dir, 0700); err != nil {
			return count, err
		}
		if err := syncPendingDir(pending, ".claimed"); err != nil {
			return count, err
		}
		if err := pending.Rename(de.Name(), filepath.Join(dir, de.Name())); err != nil {
			cleanupErr := cleanup(dir)
			if errors.Is(err, os.ErrNotExist) && cleanupErr == nil {
				continue
			}
			return count, errors.Join(err, cleanupErr)
		}
		if err := syncPendingDir(pending, dir); err != nil {
			return count, err
		}
		if err := syncPendingDir(pending, "."); err != nil {
			return count, err
		}
		if err := consume(dir, de.Name()); err != nil {
			return count, err
		}
		if err := cleanup(dir); err != nil {
			return count, err
		}
	}
	return count, nil
}

func syncPendingDir(root *os.Root, path string) error {
	dir, err := root.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		// Another importer may already have consumed and removed this claim.
		return nil
	}
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

func pendingIdentity(key string, rec writer.PendingRecord) (string, error) {
	if key == "" || rec.Path == "" {
		return "", errors.New("disposition: pending record needs a key and target path")
	}
	path := filepath.Clean(rec.Path)
	if path == "." {
		return "", errors.New("disposition: pending target must name a file")
	}
	// The recorded target is evidence, never dereferenced by this importer.
	// Preserve old absolute paths when a brain has moved since the write paused.
	if _, err := time.Parse(time.RFC3339, rec.DetectedAt); err != nil {
		return "", fmt.Errorf("disposition: pending detection timestamp: %w", err)
	}
	// Attempt time does not create a new conflict. Both competing contents do.
	raw, err := json.Marshal([]string{"dirty-edit-v1", key, filepath.ToSlash(path), rec.Human, rec.Machine})
	return string(raw), err
}

func (s *Store) stagePending(ctx context.Context, key string, rec writer.PendingRecord, now time.Time) error {
	identity, err := pendingIdentity(key, rec)
	if err != nil {
		return err
	}
	// Preserve decisions created by the old filename-only importer when their
	// evidence matches. Different evidence gets a new content-derived identity.
	prior, err := s.Get(ctx, "dirty_edit:"+key)
	if err == nil {
		if prior.Kind != KindDirtyEdit {
			return errors.New("disposition: legacy pending identity has another kind")
		}
		var original writer.PendingRecord
		if err := json.Unmarshal(prior.Payload, &original); err != nil {
			return err
		}
		oldIdentity, err := pendingIdentity(key, original)
		if err != nil {
			return err
		}
		if oldIdentity == identity {
			return nil
		}
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	payload, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, _, err = s.CreateOnce(ctx, KindDirtyEdit, payload, "", identity, now)
	return err
}
