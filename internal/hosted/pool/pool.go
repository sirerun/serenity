// Package pool bounds the open, single-writer brain runtimes used by hosted MCP.
package pool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/gitrun"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/server/memory"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

var ErrCapacity = errors.New("hosted runtime capacity reached")
var ErrRuntimeClosing = errors.New("hosted runtime closing; lifecycle retry required")
var ErrNilLifecycleContext = errors.New("hosted pool: lifecycle context required")

type Config struct {
	MaxOpen, MaxInFlight int
	IdleTimeout          time.Duration
	BrainsRoot           string
	Embedder             embed.Embedder
}
type Runtime struct {
	Sources      *store.SourceStore
	Queue        *writer.Queue
	Mutations    sync.Mutex
	Tools        []mcp.Tool
	Root         string
	brainID      string
	queue        *writer.Queue
	close        func() error
	closeContext func(context.Context) error
	closing      bool // guarded by Pool.mu; unavailable until exact close retry succeeds
	users        int
	last         time.Time
}
type Pool struct {
	cfg      Config
	mu       sync.Mutex
	open     map[string]*Runtime
	closed   bool
	inFlight int
	wg       sync.WaitGroup
}

func New(cfg Config) (*Pool, error) {
	if cfg.MaxOpen < 1 || cfg.MaxInFlight < 1 || cfg.BrainsRoot == "" || cfg.Embedder == nil {
		return nil, errors.New("invalid runtime pool configuration")
	}
	return &Pool{cfg: cfg, open: map[string]*Runtime{}}, nil
}

// BrainsRoot returns the configured root used by this pool to open brain
// runtimes. It exposes the immutable configuration value for preflight checks.
func (p *Pool) BrainsRoot() string {
	if p == nil {
		return ""
	}
	return p.cfg.BrainsRoot
}

func (p *Pool) Acquire(ctx context.Context, id string) (*Runtime, func(), error) {
	return p.acquire(ctx, id, false, false)
}

// AcquireCanonical never runs provider-backed index recovery while opening.
// Canonical-only consumers read the current source projection directly.
func (p *Pool) AcquireCanonical(ctx context.Context, id string) (*Runtime, func(), error) {
	return p.acquire(ctx, id, false, true)
}

// AcquireExisting pins an already-open runtime without opening, initializing,
// recovering or evicting brain storage. Absence is a deferred capacity result.
func (p *Pool) AcquireExisting(ctx context.Context, id string) (*Runtime, func(), error) {
	return p.acquire(ctx, id, true, false)
}

func (p *Pool) acquire(ctx context.Context, id string, existingOnly, canonicalOnly bool) (*Runtime, func(), error) {
	if !p.mu.TryLock() {
		return nil, nil, ErrCapacity
	}
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if p.closed || p.inFlight >= p.cfg.MaxInFlight {
		return nil, nil, ErrCapacity
	}
	if current := p.open[id]; current != nil && current.closing {
		return nil, nil, ErrRuntimeClosing
	}
	if !existingOnly {
		for key, item := range p.open {
			if item.closing {
				continue
			}
			if item.users == 0 && p.cfg.IdleTimeout > 0 && time.Since(item.last) >= p.cfg.IdleTimeout {
				if err := closeRuntime(ctx, item); err != nil {
					return nil, nil, err
				}
				delete(p.open, key)
			}
		}
	}
	r := p.open[id]
	if r == nil {
		if existingOnly {
			return nil, nil, ErrCapacity
		}
		if len(p.open) >= p.cfg.MaxOpen {
			var oldest string
			var candidate *Runtime
			for key, item := range p.open {
				if !item.closing && item.users == 0 && (candidate == nil || item.last.Before(candidate.last)) {
					oldest = key
					candidate = item
				}
			}
			if candidate == nil {
				return nil, nil, ErrCapacity
			}
			if err := closeRuntime(ctx, candidate); err != nil {
				return nil, nil, err
			}
			delete(p.open, oldest)
		}
		var err error
		r, err = open(ctx, p.cfg, id, canonicalOnly)
		if err != nil {
			return nil, nil, err
		}
		p.open[id] = r
	}
	r.users++
	r.last = time.Now()
	p.inFlight++
	p.wg.Add(1)
	var once sync.Once
	release := func() {
		once.Do(func() {
			p.mu.Lock()
			r.users--
			r.last = time.Now()
			p.inFlight--
			p.mu.Unlock()
			p.wg.Done()
		})
	}
	return r, release, nil
}
func (p *Pool) Close() error {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	p.wg.Wait()
	p.mu.Lock()
	defer p.mu.Unlock()
	var err error
	for id, r := range p.open {
		closeErr := closeRuntime(context.Background(), r)
		err = errors.Join(err, closeErr)
		if closeErr == nil {
			delete(p.open, id)
		}
	}
	return err
}
func open(ctx context.Context, cfg Config, id string, canonicalOnly bool) (*Runtime, error) {
	if len(id) < 16 || len(id) > 64 {
		return nil, errors.New("invalid brain id")
	}
	for _, c := range id {
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return nil, errors.New("invalid brain id")
		}
	}
	root := filepath.Join(cfg.BrainsRoot, id)
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("unsafe brain root")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	gitInfo, err := os.Lstat(filepath.Join(root, ".git"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("brain Git directory is required: %w", err)
	}
	if err != nil {
		return nil, fmt.Errorf("inspect brain Git directory: %w", err)
	}
	if !gitInfo.IsDir() || gitInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("unsafe brain Git directory")
	}
	owner, err := writer.AcquireBrain(root)
	if err != nil {
		return nil, err
	}
	fail := func(e error) (*Runtime, error) { return nil, errors.Join(e, owner.Close()) }
	for _, dir := range []string{"brain/entities", "brain/sources", "brain/claims", ".dira/entries"} {
		if err = os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
			return fail(err)
		}
	}
	configPath := filepath.Join(root, config.FileName)
	if _, err = os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		c := config.Default()
		c.Models.Embedding = cfg.Embedder.ModelVersion()
		if err = c.Save(configPath); err != nil {
			return fail(err)
		}
	} else if err != nil {
		return fail(err)
	}
	c, err := config.Load(configPath)
	if err != nil {
		return fail(err)
	}
	if c.Models.Embedding != cfg.Embedder.ModelVersion() {
		return fail(embed.ErrPinMismatch)
	}
	// Provisioning may already have committed a baseline before runtime config
	// exists. Check committed content, not merely HEAD or whether Save ran in
	// this process, so a retry after writing config also completes its commit.
	paths := []string{config.FileName, ".gitignore"}
	if _, headErr := gitrun.CanonicalReadOnly(root).Output(ctx, "rev-parse", "--verify", "HEAD"); headErr == nil {
		tracked, listErr := gitrun.CanonicalReadOnly(root).Output(ctx, "ls-tree", "--name-only", "HEAD", "--", config.FileName)
		if listErr != nil {
			return fail(fmt.Errorf("inspect committed brain config: %w", listErr))
		}
		if len(tracked) != 0 {
			paths = nil
		} else {
			paths = []string{config.FileName}
		}
	} else if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if len(paths) != 0 {
		args := append([]string{"add", "--"}, paths...)
		if output, e := gitrun.Brain(root).CombinedOutput(ctx, args...); e != nil {
			return fail(fmt.Errorf("stage brain baseline: %w: %s", e, output))
		}
		args = append([]string{"commit", "--only", "-m", "Initialize hosted brain configuration", "--"}, paths...)
		cmd, e := gitrun.Brain(root).Command(ctx, args...)
		if e != nil {
			return fail(fmt.Errorf("prepare brain baseline commit: %w", e))
		}
		cmd.Env = append(cmd.Env,
			"GIT_AUTHOR_NAME=Serenity Hosted",
			"GIT_AUTHOR_EMAIL=hosted@serenity.sire.run",
			"GIT_COMMITTER_NAME=Serenity Hosted",
			"GIT_COMMITTER_EMAIL=hosted@serenity.sire.run",
		)
		if output, e := cmd.CombinedOutput(); e != nil {
			return fail(fmt.Errorf("commit brain baseline: %w: %s", e, output))
		}
	}

	eng, err := providers.OpenIndex(root)
	if err != nil {
		return fail(err)
	}
	if !canonicalOnly {
		if err = index.RecoverMemorySearch(ctx, root, eng, cfg.Embedder); err != nil {
			return fail(errors.Join(err, eng.Close()))
		}
	}
	q := writer.NewQueue(nil)
	handlers := memory.New(memory.Deps{Root: root, Config: c, Index: eng, Embedder: cfg.Embedder, Queue: q, Sources: store.NewSourceStore(root), Fence: store.NewFenceWriter(root), Shard: store.NewShardStore(root)})
	var tools []mcp.Tool
	for _, tool := range append(handlers.Tools(), handlers.ExtensionTools()...) {
		switch tool.Name {
		case "remember", "recall", "forget", "read_memory_fact", "retract_governed_memory":
			tools = append(tools, tool)
		}
	}
	indexClosed := false
	ownerClosed := false
	closeContext := func(closeCtx context.Context) error {
		if err := q.CloseContext(closeCtx); err != nil {
			return err
		}
		if _, err := writer.FlushContext(closeCtx, q, root); err != nil {
			return err
		}
		// Retain canonical ownership until all actual work and publication have
		// joined. Partial close failures remain unavailable for exact retry.
		if !indexClosed {
			if err := eng.Close(); err != nil {
				return err
			}
			indexClosed = true
		}
		if !ownerClosed {
			if err := owner.Close(); err != nil {
				return err
			}
			ownerClosed = true
		}
		return nil
	}
	return &Runtime{Root: root, Sources: store.NewSourceStore(root), Queue: q, brainID: id, queue: q, Tools: tools,
		closeContext: closeContext, close: func() error { return closeContext(context.Background()) },
	}, nil
}

// Drop joins no new work and closes an idle runtime before its storage is removed.
func (p *Pool) Drop(id string) error { return p.DropContext(context.Background(), id) }

// DropContext retains a closing runtime on cancellation or failure. Every
// acquisition mode refuses that runtime until an exact retry joins and closes it.
func (p *Pool) DropContext(ctx context.Context, id string) error {
	if err := p.lockContext(ctx); err != nil {
		return err
	}
	defer p.mu.Unlock()
	r := p.open[id]
	if r == nil {
		return nil
	}
	if r.users != 0 {
		return ErrCapacity
	}
	if err := closeRuntime(ctx, r); err != nil {
		return err
	}
	delete(p.open, id)
	return nil
}

func closeRuntime(ctx context.Context, r *Runtime) error {
	r.closing = true
	if r.closeContext != nil {
		return r.closeContext(ctx)
	}
	if r.close != nil {
		return r.close()
	}
	return ErrRuntimeClosing
}

func (p *Pool) FlushAll() error { return p.FlushAllContext(context.Background()) }

func (p *Pool) FlushAllContext(ctx context.Context) error {
	if err := p.lockContext(ctx); err != nil {
		return err
	}
	defer p.mu.Unlock()
	for _, r := range p.open {
		if r.users != 0 {
			return ErrCapacity
		}
		if r.closing {
			return ErrRuntimeClosing
		}
		if err := r.FlushContext(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Waiters own no work while waiting for Pool.mu. No helper goroutine can outlive
// cancellation and later acquire the mutex on behalf of a departed caller.
func (p *Pool) lockContext(ctx context.Context) error {
	if ctx == nil {
		return ErrNilLifecycleContext
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if p.mu.TryLock() {
			if err := ctx.Err(); err != nil {
				p.mu.Unlock()
				return err
			}
			return nil
		}
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (r *Runtime) Flush() error { return r.FlushContext(context.Background()) }

// FlushContext publishes pending state while honoring cancellation during
// run-lock or commit-gate waits. Hosted lifecycle callers use this wrapper so
// they cannot bypass the per-brain checker fence.
func (r *Runtime) FlushContext(ctx context.Context) error {
	if r.queue == nil {
		return errors.New("hosted pool: runtime queue unavailable")
	}
	_, err := writer.FlushContext(ctx, r.queue, r.Root)
	return err
}

var ErrBrainFenceID = errors.New("hosted pool: brain fence ID mismatch")

func (r *Runtime) EnterCommit(ctx context.Context, brainID string) (func(), error) {
	if brainID != r.brainID || r.queue == nil {
		return nil, ErrBrainFenceID
	}
	return r.queue.EnterCommit(ctx)
}

func (r *Runtime) Fence(ctx context.Context, brainID string) (func(), error) {
	if brainID != r.brainID || r.queue == nil {
		return nil, ErrBrainFenceID
	}
	release, err := r.queue.AcquireCommitFence(ctx)
	if err != nil {
		return nil, errors.Join(contracts.ErrBrainNotQuiescent, err)
	}
	return release, nil
}
