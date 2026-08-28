// Package file implements the file-watcher connector (RFC 0001 §10.1,
// plan T1.3): ingests plain files dropped into a watched directory tree
// (drops, exports, PDFs, screenshots). New uses fsnotify for change
// notification; NewPoll rescans the tree on every Poll call instead, for
// filesystems where inotify-style events are unreliable or unavailable
// (network mounts -- ADR 003's "--poll fallback").
//
// Both modes share the same per-file debounce: a file must go Debounce
// (default 2s) without a further change before Poll treats it as stable
// and safe to read, absorbing the burst of write events an editor or a
// download produces mid-write.
package file

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/sirerun/serenity/internal/connector"
	"github.com/sirerun/serenity/internal/domain"
)

// DefaultDebounce is the per-file settle window a plan T1.3 acceptance
// line pins: "2s per-file debounce is tested with a fake clock".
const DefaultDebounce = 2 * time.Second

// Clock abstracts time.Now so debounce logic is testable without real
// sleeps. Production callers get realClock via New/NewPoll; tests inject
// their own via WithClock.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Option configures a Connector at construction.
type Option func(*Connector)

// WithDebounce overrides DefaultDebounce.
func WithDebounce(d time.Duration) Option { return func(c *Connector) { c.debounce = d } }

// WithClock overrides the real clock. Test-only hook.
func WithClock(clk Clock) Option { return func(c *Connector) { c.clock = clk } }

// Connector is the file-watcher implementation of connector.Connector.
// Construct with New (fsnotify) or NewPoll (directory-scan fallback);
// the zero value is not usable.
type Connector struct {
	root     string
	debounce time.Duration
	clock    Clock
	poll     bool // true selects the --poll fallback instead of fsnotify

	mu       sync.Mutex
	watcher  *fsnotify.Watcher
	touched  map[string]time.Time // relative path -> last event time (watch mode only)
	watchErr error
}

// New constructs a watch-mode Connector: a background fsnotify watcher
// covers root and every subdirectory (re-adding new subdirectories as
// they appear), recording an event time per changed path. Root must
// exist. Call Close when done to stop the background goroutine.
func New(root string, opts ...Option) (*Connector, error) {
	c := newConnector(root, opts...)

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("file: new watcher: %w", err)
	}
	c.watcher = w
	c.touched = map[string]time.Time{}
	if err := c.addTreeWatches(root); err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("file: watch %s: %w", root, err)
	}
	go c.watchLoop()
	return c, nil
}

// NewPoll constructs a poll-mode Connector: Poll rescans the directory
// tree on every call instead of relying on OS-level change notification.
func NewPoll(root string, opts ...Option) *Connector {
	c := newConnector(root, opts...)
	c.poll = true
	return c
}

func newConnector(root string, opts ...Option) *Connector {
	c := &Connector{root: root, debounce: DefaultDebounce, clock: realClock{}}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Name identifies this connector in job rows and RFC §10.1 provenance.
func (c *Connector) Name() string { return "file" }

// Close stops the background fsnotify watcher. No-op in poll mode.
func (c *Connector) Close() error {
	if c.watcher != nil {
		return c.watcher.Close()
	}
	return nil
}

// cursorState is the file connector's Cursor payload: the last content
// seen at each relative path, so Poll only re-reads and re-emits a file
// whose content actually changed since the previous call (RFC §10.1:
// "advancing the cursor never skips or duplicates a source").
type cursorState struct {
	Seen map[string]seenFile `json:"seen"`
}

type seenFile struct {
	SHA256  string    `json:"sha256"`
	ModTime time.Time `json:"mod_time"`
	Size    int64     `json:"size"`
}

func decodeCursor(cur connector.Cursor) (cursorState, error) {
	state := cursorState{Seen: map[string]seenFile{}}
	if len(cur) == 0 {
		return state, nil
	}
	if err := json.Unmarshal(cur, &state); err != nil {
		return cursorState{}, fmt.Errorf("file: decode cursor: %w", err)
	}
	if state.Seen == nil {
		state.Seen = map[string]seenFile{}
	}
	return state, nil
}

func encodeCursor(state cursorState) connector.Cursor {
	b, err := json.Marshal(state)
	if err != nil {
		// state is built entirely from strings, times, and int64s, so
		// Marshal cannot fail in practice; fall back to an empty-but-valid
		// cursor rather than propagating from a Poll that already
		// succeeded.
		return connector.Cursor(`{"seen":{}}`)
	}
	return connector.Cursor(b)
}

// Poll returns one RawItem per file that has settled (stable for at
// least the debounce window) and whose content is new or changed since
// cursor. Poll mode rescans the tree; watch mode drains fsnotify events
// accumulated since the last call.
func (c *Connector) Poll(ctx context.Context, cursor connector.Cursor) ([]connector.RawItem, connector.Cursor, error) {
	state, err := decodeCursor(cursor)
	if err != nil {
		return nil, cursor, err
	}

	var stable []string
	if c.poll {
		stable, err = c.scanStable()
	} else {
		stable, err = c.drainStable()
	}
	if err != nil {
		return nil, cursor, err
	}

	items := make([]connector.RawItem, 0, len(stable))
	for _, rel := range stable {
		if err := ctx.Err(); err != nil {
			return items, encodeCursor(state), err
		}

		item, changed, err := c.buildItem(rel, state)
		if err != nil {
			return items, encodeCursor(state), err
		}
		if changed {
			items = append(items, item)
		}
	}

	sort.Slice(items, func(i, j int) bool { return items[i].URI < items[j].URI })
	return items, encodeCursor(state), nil
}

// buildItem stats and, if needed, reads rel (relative to root), updating
// state.Seen in place. changed is false when the file was removed before
// it could be read, or when its content is identical to what state
// already recorded (only its mtime moved, e.g. a bare `touch`).
func (c *Connector) buildItem(rel string, state cursorState) (connector.RawItem, bool, error) {
	abs := filepath.Join(c.root, filepath.FromSlash(rel))

	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			delete(state.Seen, rel)
			return connector.RawItem{}, false, nil
		}
		return connector.RawItem{}, false, fmt.Errorf("file: stat %s: %w", rel, err)
	}
	if !info.Mode().IsRegular() {
		return connector.RawItem{}, false, nil
	}

	prev, known := state.Seen[rel]
	if known && prev.Size == info.Size() && prev.ModTime.Equal(info.ModTime()) {
		return connector.RawItem{}, false, nil // unchanged since the last poll
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return connector.RawItem{}, false, fmt.Errorf("file: read %s: %w", rel, err)
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])

	state.Seen[rel] = seenFile{SHA256: sha, ModTime: info.ModTime(), Size: info.Size()}
	if known && prev.SHA256 == sha {
		return connector.RawItem{}, false, nil // content identical; only mtime moved
	}

	return connector.RawItem{
		URI:        uriFor(abs),
		Kind:       "file",
		Bytes:      data,
		OccurredAt: info.ModTime(),
		Meta: map[string]string{
			"path": rel,
			"size": strconv.FormatInt(info.Size(), 10),
		},
	}, true, nil
}

// scanStable walks root (poll mode) and returns every non-ignored
// regular file, as a root-relative slash path, whose mtime is at least
// debounce old -- i.e. not still possibly mid-write.
func (c *Connector) scanStable() ([]string, error) {
	var stable []string
	now := c.clock.Now()

	err := filepath.WalkDir(c.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == c.root {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || isEditorTemp(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if now.Sub(info.ModTime()) < c.debounce {
			return nil // possibly still being written; try again next poll
		}
		rel, err := filepath.Rel(c.root, path)
		if err != nil {
			return err
		}
		stable = append(stable, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("file: walk %s: %w", c.root, err)
	}
	sort.Strings(stable)
	return stable, nil
}

// drainStable (watch mode) returns every path whose most recent fsnotify
// event is at least debounce old, clearing it from the pending set.
func (c *Connector) drainStable() ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.watchErr != nil {
		err := c.watchErr
		c.watchErr = nil
		return nil, fmt.Errorf("file: watcher: %w", err)
	}

	now := c.clock.Now()
	var stable []string
	for rel, t := range c.touched {
		if now.Sub(t) >= c.debounce {
			stable = append(stable, rel)
			delete(c.touched, rel)
		}
	}
	sort.Strings(stable)
	return stable, nil
}

// addTreeWatches registers root and every non-hidden subdirectory under
// it with the fsnotify watcher. fsnotify has no recursive-watch mode, so
// new subdirectories are added as they are observed (see handleEvent).
func (c *Connector) addTreeWatches(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		return c.watcher.Add(path)
	})
}

func (c *Connector) watchLoop() {
	for {
		select {
		case ev, ok := <-c.watcher.Events:
			if !ok {
				return
			}
			c.handleEvent(ev)
		case err, ok := <-c.watcher.Errors:
			if !ok {
				return
			}
			c.mu.Lock()
			c.watchErr = err
			c.mu.Unlock()
		}
	}
}

func (c *Connector) handleEvent(ev fsnotify.Event) {
	rel, relErr := filepath.Rel(c.root, ev.Name)
	if relErr != nil {
		return
	}
	rel = filepath.ToSlash(rel)

	info, err := os.Stat(ev.Name)
	if err != nil {
		// Removed or renamed away: drop any pending debounce state so a
		// deleted file is never emitted once it settles.
		c.mu.Lock()
		delete(c.touched, rel)
		c.mu.Unlock()
		return
	}
	if info.IsDir() {
		if ev.Op&fsnotify.Create != 0 {
			_ = c.addTreeWatches(ev.Name) // best-effort: watch the new subtree too
		}
		return
	}
	if !info.Mode().IsRegular() || isEditorTemp(filepath.Base(ev.Name)) {
		return
	}
	if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
		return
	}

	c.mu.Lock()
	c.touched[rel] = c.clock.Now()
	c.mu.Unlock()
}

// isEditorTemp reports whether name matches a common editor scratch-file
// pattern the plan's acceptance line names explicitly: atomic-save temp
// files (".<name>.tmp") and vim swap files ("*.swp").
func isEditorTemp(name string) bool {
	if strings.HasSuffix(name, ".swp") {
		return true
	}
	if ok, _ := filepath.Match(".*.tmp", name); ok {
		return true
	}
	return false
}

func uriFor(abs string) string { return "file://" + filepath.ToSlash(abs) }

// ToSource converts one settled file into the domain.Source the store's
// content-address dedup runs on. SHA256 is left unset -- SourceStore.Write
// always recomputes it from the bytes it is given, never trusting the
// caller (internal/store/source.go).
func (c *Connector) ToSource(item connector.RawItem) (domain.Source, error) {
	return domain.Source{
		Kind:       item.Kind,
		URI:        item.URI,
		OccurredAt: item.OccurredAt,
		Meta:       item.Meta,
	}, nil
}
