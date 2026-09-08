package gbrain

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// Result counts pages and source fence rows; graph edges are separate from claims.
type Result struct {
	Pages   int         `json:"pages"`
	Claims  int         `json:"claims"`
	Skipped int         `json:"skipped"`
	Audit   FieldReport `json:"audit"`
}

// Import reads a gbrain checkout and durably publishes canonical entity pages.
// Parse/map/vocabulary/destination-collision checks finish before any writes.
// Changed existing pages require an explicit migration resolution, never overwrite.
func Import(ctx context.Context, source, target string, cfg *config.Config) (Result, error) {
	result := Result{Audit: FieldReport{Unmapped: []FieldLoss{}}}
	if cfg == nil {
		return result, fmt.Errorf("gbrain import: missing target config")
	}
	source, err := filepath.Abs(source)
	if err != nil {
		return result, err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return result, err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return result, err
	}
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		return result, err
	}
	if source == target || strings.HasPrefix(target, source+string(filepath.Separator)) || strings.HasPrefix(source, target+string(filepath.Separator)) {
		return result, fmt.Errorf("gbrain import: source and target must be separate directories")
	}
	srcRoot, err := os.OpenRoot(source)
	if err != nil {
		return result, err
	}
	defer func() { _ = srcRoot.Close() }()
	var pages []*store.EntityPage
	seen := map[string]string{}
	targets := map[string]string{}
	sourcePages := map[string]Page{}
	err = filepath.WalkDir(source, func(name string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			if name != source && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("gbrain import: symlink source %s", name)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("gbrain import: nonregular source %s", name)
		}
		if !strings.HasSuffix(name, ".md") {
			return nil
		}
		rel, err := filepath.Rel(source, name)
		if err != nil {
			return err
		}
		// Repository documentation is not an entity page. It must contain YAML
		// frontmatter to participate; a gbrain fence without it is malformed.
		raw, err := srcRoot.ReadFile(rel)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(string(raw), "---\n") && !strings.HasPrefix(string(raw), "---\r\n") {
			if strings.Contains(string(raw), "gbrain:") {
				return fmt.Errorf("%s: fence without frontmatter", rel)
			}
			return nil
		}
		p, err := Parse(filepath.ToSlash(rel), raw)
		if err != nil {
			return err
		}
		mapped, err := Map(p)
		if err != nil {
			return err
		}
		if prior, ok := seen[mapped.Entity.Slug]; ok {
			return fmt.Errorf("gbrain import: target slug collision between %s and %s", prior, rel)
		}
		seen[mapped.Entity.Slug] = rel
		for _, c := range mapped.Claims {
			family, ok := cfg.Families[c.Family]
			if !ok || family.Tier != domain.TierFence {
				return fmt.Errorf("gbrain import: predicate %s requires a configured fence tier", c.Family)
			}
		}
		sourcePages[mapped.Entity.Slug] = p
		targets[p.Slug] = mapped.Entity.Slug
		targets[mapped.Entity.Slug] = mapped.Entity.Slug
		pages = append(pages, mapped)
		return nil
	})
	if err != nil {
		return result, err
	}
	if len(pages) == 0 {
		return result, fmt.Errorf("gbrain import: no entity pages found")
	}
	for _, p := range pages {
		for i, link := range p.Links {
			ref, _, _ := strings.Cut(link.Target, "#")
			p.Links[i].EntitySlug = targets[strings.TrimSuffix(ref, ".md")]
		}
	}
	fw := store.NewFenceWriter(target)
	fw.Vocabulary = map[string]bool{}
	for name := range cfg.Families {
		fw.Vocabulary[name] = true
	}
	// Render all pages up front too: malformed source metadata must not turn
	// into a partial success after an earlier page has already been committed.
	dstRoot, err := os.OpenRoot(target)
	if err != nil {
		return result, err
	}
	defer func() { _ = dstRoot.Close() }()
	for _, p := range pages {
		data, err := fw.RenderEntity(p)
		if err != nil {
			return result, err
		}
		persisted, err := store.ParseEntityBytes(data)
		if err != nil {
			return result, fmt.Errorf("gbrain import: parse rendered page: %w", err)
		}
		audit := Audit(sourcePages[p.Entity.Slug], persisted.Claims)
		result.Audit.Rows += audit.Rows
		result.Audit.Fields += audit.Fields
		result.Audit.Unmapped = append(result.Audit.Unmapped, audit.Unmapped...)
		if len(audit.Unmapped) > 0 {
			return result, fmt.Errorf("gbrain import: %s has %d unmapped fields; no pages written", p.Entity.Slug, len(audit.Unmapped))
		}
		rel := filepath.Join("brain", "entities", p.Entity.Type, p.Entity.Slug+".md")
		if existing, err := dstRoot.ReadFile(rel); err == nil {
			if !bytes.Equal(data, existing) {
				return result, fmt.Errorf("gbrain import: target %s has different content", rel)
			}
		} else if !errors.Is(err, fs.ErrNotExist) {
			return result, err
		}
		existing, err := filepath.Glob(filepath.Join(target, "brain", "entities", "*", p.Entity.Slug+".md"))
		if err != nil {
			return result, err
		}
		for _, name := range existing {
			if name != filepath.Join(target, rel) {
				return result, fmt.Errorf("gbrain import: target slug %s exists under another type", p.Entity.Slug)
			}
		}
	}
	q := writer.NewQueue(nil)
	defer q.Close()
	for _, p := range pages {
		changed, err := writer.ImportEntity(ctx, q, fw, p)
		if err != nil {
			return result, err
		}
		if _, err := writer.Flush(q, target); err != nil {
			return result, fmt.Errorf("gbrain import: commit page %s: %w", p.Entity.Slug, err)
		}
		result.Pages++
		result.Claims += len(p.Claims)
		if !changed {
			result.Skipped++
		}
	}
	return result, nil
}
