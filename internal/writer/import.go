package writer

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/sirerun/serenity/internal/store"
)

var importSegment = regexp.MustCompile(`^[\pL\pN][\pL\pN._-]*$`)

// ImportEntity atomically publishes a new complete page. Existing identical
// bytes are a no-op; any different page (even untracked human work) is a
// conflict. It never merges by silently replacing a human's canonical page.
// os.Root confines publication even when a destination directory is a symlink.
func ImportEntity(ctx context.Context, q *Queue, fw *store.FenceWriter, p *store.EntityPage) (bool, error) {
	if !importSegment.MatchString(p.Entity.Type) || !importSegment.MatchString(p.Entity.Slug) {
		return false, fmt.Errorf("import entity: unsafe type or slug")
	}
	data, err := fw.RenderEntity(p)
	if err != nil {
		return false, err
	}
	changed := false
	result := q.Submit(Job{Render: func() ([]byte, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		root, err := os.OpenRoot(fw.Root)
		if err != nil {
			return nil, err
		}
		defer func() { _ = root.Close() }()
		rel := filepath.Join("brain", "entities", p.Entity.Type, p.Entity.Slug+".md")
		if existing, err := root.ReadFile(rel); err == nil {
			if !bytes.Equal(existing, data) {
				return nil, fmt.Errorf("import entity: %s already exists with different content", rel)
			}
			q.MarkTouched(filepath.Join(fw.Root, rel))
			return data, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		if err := root.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
			return nil, err
		}
		// The temporary file is inside the confined root. Its name is unique per
		// process attempt; a crash cannot leave a partial canonical .md file.
		name := ".serenity-import-" + rand.Text()
		tmp, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, err
		}
		defer func() { _ = root.Remove(name) }()
		if _, err := tmp.Write(data); err != nil {
			_ = tmp.Close()
			return nil, err
		}
		if err := tmp.Sync(); err != nil {
			_ = tmp.Close()
			return nil, err
		}
		if err := tmp.Close(); err != nil {
			return nil, err
		}
		// Link is exclusive: a concurrent new human page cannot be overwritten.
		if err := root.Link(name, rel); err != nil {
			return nil, fmt.Errorf("import entity: publish %s: %w", rel, err)
		}
		dir, err := root.Open(filepath.Dir(rel))
		if err != nil {
			return nil, err
		}
		err = dir.Sync()
		closeErr := dir.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
		q.MarkTouched(filepath.Join(fw.Root, rel))
		changed = true
		return data, nil
	}})
	return changed, result.Err
}
