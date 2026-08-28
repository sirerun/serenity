package extract

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// CacheKey identifies one cached extraction outcome (RFC 0001 §10.1): a
// chunk's content, the pinned model that produced the result, and the
// prompt version that shaped the call. A change to any one of the three
// is a cache miss by design -- a reworded prompt or a migrated model pin
// must never read a result produced under different instructions.
type CacheKey struct {
	ChunkSHA256   string
	ModelVersion  string
	PromptVersion string
}

// hash collapses the key into one filesystem/map-safe token.
func (k CacheKey) hash() string {
	h := sha256.Sum256([]byte(k.ChunkSHA256 + "\x00" + k.ModelVersion + "\x00" + k.PromptVersion))
	return hex.EncodeToString(h[:])
}

// Candidate is one accepted, vocabulary-checked, confidence-clamped
// extraction result for a chunk -- content only, no provenance. It is
// also the shape buildPrompt asks the model to emit (see modelResponse),
// so the same type carries a candidate from raw model JSON all the way
// into the cache.
type Candidate struct {
	Subject    string  `json:"subject"`
	Predicate  string  `json:"predicate"`
	Object     string  `json:"object"`
	Confidence float64 `json:"confidence"`
}

// CachedOutput is what a Cache stores under one CacheKey: the chunk's
// accepted candidates and how many raw candidates were rejected.
// Deliberately provenance-free -- SourceSHA256, Span, CreatedAt, and each
// observation's ID are stamped fresh by ExtractChunk on every call,
// cached or not, so a cache hit for identical chunk text repeated across
// two different sources (a boilerplate signature block, a duplicated
// README paragraph) never leaks the first source's provenance onto the
// second.
type CachedOutput struct {
	Accepted []Candidate `json:"accepted"`
	Rejected int         `json:"rejected"`
}

// Cache stores one CachedOutput per CacheKey. Implementations must be
// safe for concurrent use.
type Cache interface {
	Get(ctx context.Context, key CacheKey) (CachedOutput, bool, error)
	Put(ctx context.Context, key CacheKey, out CachedOutput) error
}

// MemoryCache is a process-lifetime, in-memory Cache. It is the default
// when New is given no cache.
type MemoryCache struct {
	mu      sync.Mutex
	entries map[string]CachedOutput
}

// NewMemoryCache builds an empty MemoryCache.
func NewMemoryCache() *MemoryCache {
	return &MemoryCache{entries: make(map[string]CachedOutput)}
}

func (c *MemoryCache) Get(_ context.Context, key CacheKey) (CachedOutput, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out, ok := c.entries[key.hash()]
	return out, ok, nil
}

func (c *MemoryCache) Put(_ context.Context, key CacheKey, out CachedOutput) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]CachedOutput)
	}
	c.entries[key.hash()] = out
	return nil
}

// FileCache persists extraction outputs as one JSON file per cache key
// under Dir. Chosen alongside MemoryCache, not instead of it: `serenity
// extract` runs as a fresh CLI process per invocation (RFC §10.1), so a
// cache that lives only in one process's memory re-pays for every chunk
// on every run. This is a keyed file store, not a query engine -- no
// premature SQLite dependency, since content-addressed lookup by
// (chunk sha, model@version, prompt version) is all this task's
// acceptance line asks for.
type FileCache struct {
	Dir string
}

// NewFileCache builds a FileCache rooted at dir. dir is created lazily on
// first Put, not by this constructor.
func NewFileCache(dir string) *FileCache { return &FileCache{Dir: dir} }

func (c *FileCache) pathFor(key CacheKey) string {
	return filepath.Join(c.Dir, key.hash()+".json")
}

func (c *FileCache) Get(_ context.Context, key CacheKey) (CachedOutput, bool, error) {
	b, err := os.ReadFile(c.pathFor(key))
	if err != nil {
		if os.IsNotExist(err) {
			return CachedOutput{}, false, nil
		}
		return CachedOutput{}, false, fmt.Errorf("extract: file cache read: %w", err)
	}
	var out CachedOutput
	if err := json.Unmarshal(b, &out); err != nil {
		return CachedOutput{}, false, fmt.Errorf("extract: file cache decode: %w", err)
	}
	return out, true, nil
}

func (c *FileCache) Put(_ context.Context, key CacheKey, out CachedOutput) error {
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return fmt.Errorf("extract: file cache mkdir: %w", err)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return fmt.Errorf("extract: file cache encode: %w", err)
	}
	path := c.pathFor(key)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("extract: file cache write: %w", err)
	}
	return os.Rename(tmp, path)
}
