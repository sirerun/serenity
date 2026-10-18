package episodeerase

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sirerun/serenity/internal/store"
)

// PathPin is a caller-trusted assertion of independent retention, not proof of
// provenance. It binds an exact canonical path and exact blob bytes.
type PathPin struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// HostedRequest admits the narrowly qualified hosted baseline and explicitly
// pinned independent canonical files. Every historical ordinary file must be
// pinned. .gitignore and serenity.yml are mandatory pins; config may first appear
// after provisioning but must exist at HEAD. No derivative independence is inferred.
type HostedRequest struct {
	Request
	IndependentSources []SourcePin
	IndependentPaths   []PathPin
}

type hostedSources struct {
	pins   []PathPin
	byPath map[string]string
	seen   map[string]bool
	mixed  *mixedSources
}

func safeSegments(path string) bool {
	if path == "" || len(path) > 512 {
		return false
	}
	for _, s := range strings.Split(path, "/") {
		if s == "" || s == "." || s == ".." || s[0] == '.' {
			return false
		}
		for _, c := range s {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.') {
				return false
			}
		}
	}
	return true
}
func canonicalHostedPath(path string) bool {
	if strings.HasPrefix(path, ".dira/entries/") {
		return safeSegments(strings.TrimPrefix(path, ".dira/"))
	}
	if !safeSegments(path) {
		return false
	}
	for _, prefix := range []string{"brain/entities/", "brain/claims/"} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
func hostedPath(path string) bool {
	return validPath(path) || path == ".gitignore" || path == "serenity.yml" || canonicalHostedPath(path)
}
func hostedPathIf(h *hostedSources) func(string) bool {
	if h != nil {
		return hostedPath
	}
	return validPath
}

// BuildHosted prepares an INACTIVE candidate only. All pins and fence freshness
// remain trusted caller inputs. There is no production admission or activation.
func BuildHosted(ctx context.Context, req HostedRequest) (Result, error) {
	if len(req.IndependentPaths) < 2 || len(req.IndependentPaths) > 4096 || len(req.IndependentSources) > 2048 {
		return Result{}, ErrUnsafe
	}
	h := &hostedSources{pins: append([]PathPin(nil), req.IndependentPaths...), byPath: map[string]string{}, seen: map[string]bool{}}
	for _, p := range h.pins {
		if !(p.Path == ".gitignore" || p.Path == "serenity.yml" || canonicalHostedPath(p.Path)) || !store.ValidSourceSHA(p.SHA256) || h.byPath[p.Path] != "" {
			return Result{}, ErrUnsafe
		}
		h.byPath[p.Path] = p.SHA256
	}
	if h.byPath[".gitignore"] != rawDigest([]byte(".serenity/\n")) || h.byPath["serenity.yml"] == "" {
		return Result{}, ErrUnsafe
	}
	sort.Slice(h.pins, func(i, j int) bool { return h.pins[i].Path < h.pins[j].Path })
	m := &mixedSources{pins: append([]SourcePin(nil), req.IndependentSources...), bySHA: map[string]string{}, seen: map[string]bool{}}
	for _, p := range m.pins {
		if !store.ValidSourceSHA(p.SHA256) || !store.ValidSourceSHA(p.MetadataSHA256) || m.bySHA[p.SHA256] != "" {
			return Result{}, ErrUnsafe
		}
		m.bySHA[p.SHA256] = p.MetadataSHA256
	}
	sort.Slice(m.pins, func(i, j int) bool { return m.pins[i].SHA256 < m.pins[j].SHA256 })
	h.mixed = m
	if err := inspectRuntime(ctx, req.Source); err != nil {
		return Result{}, err
	}
	return build(ctx, req.Request, m, h)
}

func (h *hostedSources) validate(root string, files map[string][]byte, episodes map[string]string, head bool) error {
	raw := map[string][]byte{}
	for path, data := range files {
		if validPath(path) {
			raw[path] = data
			continue
		}
		if h.byPath[path] == "" || rawDigest(data) != h.byPath[path] {
			return ErrUnsafe
		}
		h.seen[path] = true
	}
	if string(files[".gitignore"]) != ".serenity/\n" {
		return ErrUnsafe
	}
	if head {
		if _, ok := files["serenity.yml"]; !ok {
			return ErrUnsafe
		}
	}
	return h.mixed.validateSnapshot(root, raw, episodes)
}

// Only the root ignored runtime tree is permitted. It is never copied. Reject
// links, devices, executables, unsafe names and oversized contents even there.
func inspectRuntime(ctx context.Context, root string) error {
	base := filepath.Join(root, ".serenity")
	if _, err := os.Lstat(base); os.IsNotExist(err) {
		return nil
	}
	count := 0
	var total int64
	return filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return ErrUnsafe
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return ErrUnsafe
		}
		count++
		if count > 4096 {
			return ErrLimit
		}
		if path != base {
			rel, err := filepath.Rel(base, path)
			if err != nil || !safeSegments(filepath.ToSlash(rel)) {
				return ErrUnsafe
			}
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0111 != 0 {
			return ErrUnsafe
		}
		total += info.Size()
		if total > 32<<20 {
			return ErrLimit
		}
		return nil
	})
}
