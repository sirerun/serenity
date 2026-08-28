package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/sirerun/serenity/internal/domain"
)

// SourceStore holds raw imported material content-addressed under
// brain/sources/<sha256[0:2]>/<sha256>/ (RFC §7.4, plan Layout note): a
// "bytes" file with the exact original content plus a meta.yaml sidecar
// (kind, uri, occurred_at, index_only, connector state). A Source is
// immutable (§7.6's epistemic layer table) — Write on bytes already
// stored is a no-op that returns the original metadata, never a silent
// overwrite by a later, differing call.
type SourceStore struct {
	Root string
}

func NewSourceStore(root string) *SourceStore { return &SourceStore{Root: root} }

// sourceMeta is the meta.yaml sidecar shape (§7.4). SHA256 is not stored
// here — it is the directory name, the single source of that identity.
type sourceMeta struct {
	Kind       string            `yaml:"kind"`
	URI        string            `yaml:"uri"`
	OccurredAt string            `yaml:"occurred_at"`
	IndexOnly  bool              `yaml:"index_only,omitempty"`
	Meta       map[string]string `yaml:"meta,omitempty"`
}

// DirFor returns the content-addressed directory for a sha256 hex digest.
func (s *SourceStore) DirFor(sha string) string {
	return filepath.Join(s.Root, "brain", "sources", sha[:2], sha)
}

func (s *SourceStore) bytesPath(sha string) string { return filepath.Join(s.DirFor(sha), "bytes") }
func (s *SourceStore) metaPath(sha string) string  { return filepath.Join(s.DirFor(sha), "meta.yaml") }

// Write stores data content-addressed and writes its meta.yaml sidecar,
// returning the resolved Source with SHA256 set from the bytes (never
// trusted from the caller). If the sha already exists on disk, Write does
// not touch the filesystem again — it returns the metadata recorded by the
// original write (§7.4/§7.6: Source is immutable; two logical imports that
// happen to carry identical bytes dedup onto the first one's identity).
// An index_only source's raw bytes are excluded from git (its meta.yaml
// stays tracked) so large or sensitive originals never enter version
// control (RFC §7.4, gbrain's db_only pattern).
func (s *SourceStore) Write(data []byte, src domain.Source) (domain.Source, error) {
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])

	if existing, err := s.readMeta(sha); err == nil {
		return existing, nil // content already on disk: immutable, no-op
	} else if !errors.Is(err, fs.ErrNotExist) {
		return domain.Source{}, err
	}

	dir := s.DirFor(sha)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return domain.Source{}, err
	}
	if err := os.WriteFile(s.bytesPath(sha), data, 0o644); err != nil {
		return domain.Source{}, err
	}
	src.SHA256 = sha
	if err := s.writeMeta(src); err != nil {
		return domain.Source{}, err
	}
	if src.IndexOnly {
		if err := s.ignoreBytes(sha); err != nil {
			return domain.Source{}, err
		}
	}
	return src, nil
}

// Exists reports whether content-addressed bytes for sha are already
// stored. Callers that need to know "is this genuinely new" before
// calling Write -- for example `serenity sync` (T1.15), scoping its git
// commit to sources it actually just wrote -- use this rather than
// duplicating Write's own dedup check.
func (s *SourceStore) Exists(sha string) bool {
	_, err := s.readMeta(sha)
	return err == nil
}

// All returns every source recorded in the store, sorted by SHA256 --
// the enumeration primitive a full extract/index pass over "every source
// ever ingested" needs (T1.15), as distinct from Tombstone's per-claim
// shard scan. Reads only meta.yaml sidecars, not bytes; callers needing
// raw content call Read(sha).
func (s *SourceStore) All() ([]domain.Source, error) {
	matches, err := filepath.Glob(filepath.Join(s.Root, "brain", "sources", "*", "*", "meta.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches) // sha is the parent dir name, fixed-width hex -- lexical sort is SHA order
	out := make([]domain.Source, 0, len(matches))
	for _, m := range matches {
		sha := filepath.Base(filepath.Dir(m))
		src, err := s.readMeta(sha)
		if err != nil {
			return nil, fmt.Errorf("store: read source %s: %w", sha, err)
		}
		out = append(out, src)
	}
	return out, nil
}

// Read returns the raw bytes and metadata for a stored source.
func (s *SourceStore) Read(sha string) ([]byte, domain.Source, error) {
	data, err := os.ReadFile(s.bytesPath(sha))
	if err != nil {
		return nil, domain.Source{}, err
	}
	src, err := s.readMeta(sha)
	if err != nil {
		return nil, domain.Source{}, err
	}
	return data, src, nil
}

func (s *SourceStore) writeMeta(src domain.Source) error {
	m := sourceMeta{
		Kind:      src.Kind,
		URI:       src.URI,
		IndexOnly: src.IndexOnly,
		Meta:      src.Meta,
	}
	if !src.OccurredAt.IsZero() {
		m.OccurredAt = src.OccurredAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	b, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(s.metaPath(src.SHA256), b, 0o644)
}

// readMeta returns fs.ErrNotExist (wrapped) when sha has never been
// written, so Write can distinguish "dedup, return existing" from a real
// I/O failure.
func (s *SourceStore) readMeta(sha string) (domain.Source, error) {
	b, err := os.ReadFile(s.metaPath(sha))
	if err != nil {
		return domain.Source{}, err
	}
	var m sourceMeta
	if err := yaml.Unmarshal(b, &m); err != nil {
		return domain.Source{}, err
	}
	src := domain.Source{
		SHA256:    sha,
		Kind:      m.Kind,
		URI:       m.URI,
		IndexOnly: m.IndexOnly,
		Meta:      m.Meta,
	}
	if m.OccurredAt != "" {
		if t, err := parseOccurredAt(m.OccurredAt); err == nil {
			src.OccurredAt = t
		}
	}
	return src, nil
}

// ignoreBytes appends a root .gitignore entry excluding this source's
// bytes file while leaving meta.yaml tracked, so an index_only source's
// existence and metadata stay visible in `git log`/`git show` even though
// its (large or sensitive) bytes never enter version control. Idempotent,
// same append-if-absent shape as the brain-repo .gitignore in `serenity
// init` (internal/cli/init.go's ensureGitignore).
func (s *SourceStore) ignoreBytes(sha string) error {
	rel, err := filepath.Rel(s.Root, s.bytesPath(sha))
	if err != nil {
		return err
	}
	entry := filepath.ToSlash(rel)

	path := filepath.Join(s.Root, ".gitignore")
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	for _, ln := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(ln) == entry {
			return nil
		}
	}
	content := string(b)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += entry + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func parseOccurredAt(s string) (time.Time, error) {
	return time.Parse("2006-01-02T15:04:05Z", s)
}

// Tombstone returns every claim, across every entity's shard families,
// whose provenance cites sha — the read side of §7.4's "deleting a source
// is a tombstone operation that cascades retraction proposals to its
// claims." T1.2 ships only this citing-claim lookup (the task's
// "tombstone stub"); turning the result into retraction proposals through
// the writer/disposition path is later work.
//
// Fence-tier claims render provenance down to a short human-readable
// SourceRef cell (§7.2's table has no sha256 column), so a parsed entity
// page cannot answer "does this cite sha" — only shard-tier claims persist
// full Provenance JSON on disk. This scans shards; fence-page citation
// lookup needs fence rendering to carry provenance first.
func (s *SourceStore) Tombstone(sha string, ss *ShardStore) ([]domain.Claim, error) {
	slugs, err := ss.Slugs()
	if err != nil {
		return nil, err
	}
	var out []domain.Claim
	for _, slug := range slugs {
		families, err := ss.Families(slug)
		if err != nil {
			return nil, err
		}
		for _, family := range families {
			lines, err := ss.Lines(slug, family)
			if err != nil {
				return nil, err
			}
			for _, c := range lines {
				if c.Provenance.SourceSHA256 == sha {
					out = append(out, c)
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}
