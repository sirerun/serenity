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

// ValidSourceSHA accepts only canonical lowercase, full SHA-256 identities.
func ValidSourceSHA(sha string) bool {
	if len(sha) != 64 {
		return false
	}
	for _, c := range sha {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// DirFor returns an empty path for invalid identities. I/O methods return an
// error before resolving a path, so untrusted short IDs never panic or traverse.
func (s *SourceStore) DirFor(sha string) string {
	if !ValidSourceSHA(sha) {
		return ""
	}
	return filepath.Join(s.Root, "brain", "sources", sha[:2], sha)
}
func (s *SourceStore) bytesPath(sha string) string { return filepath.Join(s.DirFor(sha), "bytes") }
func (s *SourceStore) metaPath(sha string) string  { return filepath.Join(s.DirFor(sha), "meta.yaml") }

func reservedMemoryKind(kind string) bool { return strings.HasPrefix(strings.ToLower(kind), "memory_") }

// Write publishes ordinary imported material. Memory lifecycle kinds are reserved
// for the typed entry points: importing JSON never grants expiry semantics.
func (s *SourceStore) Write(data []byte, src domain.Source) (domain.Source, error) {
	if reservedMemoryKind(src.Kind) {
		return domain.Source{}, fmt.Errorf("store: reserved source kind %q requires a typed memory write", src.Kind)
	}
	return s.writeSource(data, src)
}

// WriteMemoryFact publishes a validated canonical raw fact. Call via the writer queue.
func (s *SourceStore) WriteMemoryFact(p MemoryFactPayload) (domain.Source, error) {
	data, err := EncodeMemoryFact(p)
	if err != nil {
		return domain.Source{}, err
	}
	return s.writeSource(data, NewMemoryFactSource(p.CreatedAt))
}

// WriteMemoryExpiry publishes an immutable lifecycle event through the writer queue.
func (s *SourceStore) WriteMemoryExpiry(p MemoryExpiryPayload) (domain.Source, error) {
	data, err := EncodeMemoryExpiry(p)
	if err != nil {
		return domain.Source{}, err
	}
	return s.writeSource(data, NewMemoryExpirySource(p.ExpiredAt))
}

// writeSource stages a complete, synced record outside the canonical namespace,
// then publishes its directory with one rename. Readers never see a half record.
func (s *SourceStore) writeSource(data []byte, src domain.Source) (domain.Source, error) {
	digest := sha256.Sum256(data)
	sha := hex.EncodeToString(digest[:])
	src.SHA256 = sha
	if err := validateSourcePayload(data, src); err != nil {
		return domain.Source{}, err
	}
	if err := s.sourceParents(sha, true); err != nil {
		return domain.Source{}, err
	}
	final := s.DirFor(sha)
	if _, err := os.Lstat(final); err == nil {
		_, existing, err := s.Read(sha)
		if errors.Is(err, fs.ErrNotExist) {
			m, metaErr := s.readMeta(sha)
			if metaErr == nil && m.IndexOnly && !reservedMemoryKind(m.Kind) && !reservedMemoryKind(src.Kind) {
				return m, nil
			}
		}
		if err != nil {
			return domain.Source{}, fmt.Errorf("store: existing source is corrupt: %w", err)
		}
		if reservedMemoryKind(src.Kind) && existing.Kind != src.Kind {
			return domain.Source{}, fmt.Errorf("store: memory identity conflicts with existing source kind")
		}
		return existing, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return domain.Source{}, err
	}
	meta, err := marshalSourceMeta(src)
	if err != nil {
		return domain.Source{}, err
	}
	parent := filepath.Dir(final)
	stage, err := os.MkdirTemp(parent, ".source-stage-")
	if err != nil {
		return domain.Source{}, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	for name, content := range map[string][]byte{"bytes": data, "meta.yaml": meta} {
		if err := writeSyncedSourceFile(filepath.Join(stage, name), content); err != nil {
			return domain.Source{}, err
		}
	}
	if err := syncSourceDirectory(stage); err != nil {
		return domain.Source{}, err
	}
	// For index-only imports establish exclusion before their bytes become visible.
	if src.IndexOnly {
		if err := s.ignoreBytes(sha); err != nil {
			return domain.Source{}, err
		}
	}
	if err := os.Rename(stage, final); err != nil {
		// Another serialized owner may have published the same immutable bytes.
		_, existing, readErr := s.Read(sha)
		if readErr == nil && (!reservedMemoryKind(src.Kind) || existing.Kind == src.Kind) {
			return existing, nil
		}
		return domain.Source{}, fmt.Errorf("store: publish source: %w", err)
	}
	if err := syncSourceDirectory(parent); err != nil {
		return src, fmt.Errorf("store: sync published source: %w", err)
	}
	return src, nil
}

func writeSyncedSourceFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	return errors.Join(writeErr, f.Close())
}
func syncSourceDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

// sourceParents refuses symlinks in the canonical source namespace. Root may be
// a caller-selected alias, but no child namespace component may redirect I/O.
func (s *SourceStore) sourceParents(sha string, create bool) error {
	if !ValidSourceSHA(sha) {
		return fmt.Errorf("store: invalid source SHA %q", sha)
	}
	path := s.Root
	for _, part := range []string{"brain", "sources", sha[:2]} {
		parent := path
		path = filepath.Join(path, part)
		if create {
			if err := os.Mkdir(path, 0755); err != nil && !errors.Is(err, fs.ErrExist) {
				return err
			}
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("store: invalid source namespace directory %s", path)
		}
		if create {
			if err := syncSourceDirectory(parent); err != nil {
				return err
			}
		}
	}
	return nil
}

// Exists recognizes complete sources and metadata-only index-only imports.
func (s *SourceStore) Exists(sha string) bool {
	_, _, err := s.Read(sha)
	if err == nil {
		return true
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return false
	}
	src, err := s.readMeta(sha)
	return err == nil && src.IndexOnly && !reservedMemoryKind(src.Kind)
}

// All excludes unpublished staging directories and validates every canonical
// directory. Missing bytes are permitted only for ordinary index-only imports
// whose raw material is intentionally absent from a clone.
func (s *SourceStore) All() ([]domain.Source, error) {
	base := filepath.Join(s.Root, "brain", "sources")
	info, err := os.Lstat(base)
	if errors.Is(err, fs.ErrNotExist) {
		return []domain.Source{}, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("store: invalid sources directory")
	}
	prefixes, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Source, 0)
	for _, prefix := range prefixes {
		if prefix.Name() == ".gitkeep" && prefix.Type().IsRegular() {
			continue
		}
		if strings.HasPrefix(prefix.Name(), ".source-stage-") {
			continue
		}
		if !prefix.IsDir() || len(prefix.Name()) != 2 || !ValidSourceSHA(prefix.Name()+strings.Repeat("0", 62)) {
			return nil, fmt.Errorf("store: invalid source prefix %q", prefix.Name())
		}
		entries, err := os.ReadDir(filepath.Join(base, prefix.Name()))
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".source-stage-") {
				continue
			}
			sha := entry.Name()
			if !entry.IsDir() || !ValidSourceSHA(sha) || sha[:2] != prefix.Name() {
				return nil, fmt.Errorf("store: invalid source directory %q", sha)
			}
			_, src, err := s.Read(sha)
			if err != nil {
				// An index-only clone can contain just meta.yaml. Reserved memory records
				// never use index_only and must always have complete, verified bytes.
				m, metaErr := s.readMeta(sha)
				if !errors.Is(err, fs.ErrNotExist) || metaErr != nil || !m.IndexOnly || reservedMemoryKind(m.Kind) {
					return nil, fmt.Errorf("store: read source %s: %w", sha, err)
				}
				src = m
			}
			out = append(out, src)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SHA256 < out[j].SHA256 })
	return out, nil
}

func (s *SourceStore) Read(sha string) ([]byte, domain.Source, error) {
	if err := s.sourceParents(sha, false); err != nil {
		return nil, domain.Source{}, err
	}
	info, err := os.Lstat(s.DirFor(sha))
	if err != nil {
		return nil, domain.Source{}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, domain.Source{}, fmt.Errorf("store: invalid source directory")
	}
	src, err := s.readMeta(sha)
	if err != nil {
		return nil, domain.Source{}, err
	}
	data, err := readSourceRegularFile(s.bytesPath(sha))
	if err != nil {
		return nil, domain.Source{}, err
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != sha {
		return nil, domain.Source{}, fmt.Errorf("store: source content hash mismatch %s", sha)
	}
	if err := validateSourcePayload(data, src); err != nil {
		return nil, domain.Source{}, err
	}
	return data, src, nil
}

func readSourceRegularFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("store: source path is not a regular file: %s", path)
	}
	return os.ReadFile(path)
}
func marshalSourceMeta(src domain.Source) ([]byte, error) {
	m := sourceMeta{Kind: src.Kind, URI: src.URI, IndexOnly: src.IndexOnly, Meta: src.Meta}
	if !src.OccurredAt.IsZero() {
		m.OccurredAt = src.OccurredAt.UTC().Format(time.RFC3339Nano)
	}
	return yaml.Marshal(m)
}
func (s *SourceStore) readMeta(sha string) (domain.Source, error) {
	if !ValidSourceSHA(sha) {
		return domain.Source{}, fmt.Errorf("store: invalid source SHA %q", sha)
	}
	b, err := readSourceRegularFile(s.metaPath(sha))
	if err != nil {
		return domain.Source{}, err
	}
	var m sourceMeta
	if err := yaml.Unmarshal(b, &m); err != nil {
		return domain.Source{}, err
	}
	if strings.TrimSpace(m.Kind) == "" {
		return domain.Source{}, fmt.Errorf("store: missing source kind")
	}
	src := domain.Source{SHA256: sha, Kind: m.Kind, URI: m.URI, IndexOnly: m.IndexOnly, Meta: m.Meta}
	if m.OccurredAt != "" {
		t, err := parseOccurredAt(m.OccurredAt)
		if err != nil {
			return domain.Source{}, fmt.Errorf("store: invalid source timestamp: %w", err)
		}
		src.OccurredAt = t
	}
	return src, nil
}

func validateSourcePayload(data []byte, src domain.Source) error {
	if strings.TrimSpace(src.Kind) == "" {
		return fmt.Errorf("store: source kind is required")
	}
	if !reservedMemoryKind(src.Kind) {
		return nil
	}
	switch src.Kind {
	case SourceKindMemoryFact:
		p, err := DecodeMemoryFact(data)
		if err != nil {
			return err
		}
		if src.URI != "memory://fact" || !src.OccurredAt.Equal(p.CreatedAt) {
			return fmt.Errorf("store: memory fact metadata disagrees with payload")
		}
	case SourceKindMemoryExpiry:
		p, err := DecodeMemoryExpiry(data)
		if err != nil {
			return err
		}
		if src.URI != "memory://expiry" || !src.OccurredAt.Equal(p.ExpiredAt) {
			return fmt.Errorf("store: memory expiry metadata disagrees with payload")
		}
	default:
		return fmt.Errorf("store: unknown reserved source kind %q", src.Kind)
	}
	return nil
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
	mode := fs.FileMode(0o644)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("store: .gitignore must be a regular file")
		}
		mode = info.Mode().Perm()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
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
	// Replace the directory entry rather than truncating an existing file:
	// even a swapped symlink or a hard link cannot redirect the write.
	tmp, err := os.CreateTemp(s.Root, ".gitignore-source-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	writeErr := tmp.Chmod(mode)
	if writeErr == nil {
		_, writeErr = tmp.WriteString(content)
	}
	if writeErr == nil {
		writeErr = tmp.Sync()
	}
	if err := errors.Join(writeErr, tmp.Close()); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	return syncSourceDirectory(s.Root)
}

func parseOccurredAt(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
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
