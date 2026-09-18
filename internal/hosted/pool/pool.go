// Package pool bounds the open, single-writer brain runtimes used by hosted MCP.
package pool

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/server/memory"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

var ErrCapacity = errors.New("hosted runtime capacity reached")

type Config struct {
	MaxOpen, MaxInFlight int
	IdleTimeout          time.Duration
	BrainsRoot           string
	Embedder             embed.Embedder
}
type Runtime struct {
	flush     func() error
	Mutations sync.Mutex
	Tools     []mcp.Tool
	Root      string
	close     func() error
	users     int
	last      time.Time
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
func (p *Pool) Acquire(ctx context.Context, id string) (*Runtime, func(), error) {
	if !p.mu.TryLock() {
		return nil, nil, ErrCapacity
	}
	defer p.mu.Unlock()
	if p.closed || p.inFlight >= p.cfg.MaxInFlight {
		return nil, nil, ErrCapacity
	}
	for key, item := range p.open {
		if item.users == 0 && p.cfg.IdleTimeout > 0 && time.Since(item.last) >= p.cfg.IdleTimeout {
			if err := item.close(); err != nil {
				return nil, nil, err
			}
			delete(p.open, key)
		}
	}
	r := p.open[id]
	if r == nil {
		if len(p.open) >= p.cfg.MaxOpen {
			var oldest string
			var candidate *Runtime
			for key, item := range p.open {
				if item.users == 0 && (candidate == nil || item.last.Before(candidate.last)) {
					oldest = key
					candidate = item
				}
			}
			if candidate == nil {
				return nil, nil, ErrCapacity
			}
			if err := candidate.close(); err != nil {
				return nil, nil, err
			}
			delete(p.open, oldest)
		}
		var err error
		r, err = open(ctx, p.cfg, id)
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
		err = errors.Join(err, r.close())
		delete(p.open, id)
	}
	return err
}
func open(ctx context.Context, cfg Config, id string) (*Runtime, error) {
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
	if _, err = os.Stat(filepath.Join(root, ".git")); errors.Is(err, os.ErrNotExist) {
		for _, args := range [][]string{{"init", "--initial-branch=main"}, {"config", "user.name", "Serenity Hosted"}, {"config", "user.email", "hosted@serenity.sire.run"}} {
			if output, e := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...).CombinedOutput(); e != nil {
				return fail(fmt.Errorf("initialize brain git: %w: %s", e, output))
			}
		}
		if err = os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".serenity/\n"), 0600); err != nil {
			return fail(err)
		}
	} else if err != nil {
		return fail(err)
	}
	if _, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--verify", "HEAD").Output(); err != nil {
		if output, e := exec.CommandContext(ctx, "git", "-C", root, "add", "--", config.FileName, ".gitignore").CombinedOutput(); e != nil {
			return fail(fmt.Errorf("stage brain baseline: %w: %s", e, output))
		}
		if output, e := exec.CommandContext(ctx, "git", "-C", root, "commit", "-m", "Initialize hosted brain").CombinedOutput(); e != nil {
			return fail(fmt.Errorf("commit brain baseline: %w: %s", e, output))
		}
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		return fail(err)
	}
	if err = recoverSearch(ctx, root, eng, cfg.Embedder); err != nil {
		return fail(errors.Join(err, eng.Close()))
	}
	q := writer.NewQueue(nil)
	handlers := memory.New(memory.Deps{Root: root, Config: c, Index: eng, Embedder: cfg.Embedder, Queue: q, Sources: store.NewSourceStore(root), Fence: store.NewFenceWriter(root), Shard: store.NewShardStore(root)})
	var tools []mcp.Tool
	for _, tool := range append(handlers.Tools(), handlers.ExtensionTools()...) {
		switch tool.Name {
		case "remember", "recall", "forget", "read_memory_fact":
			tools = append(tools, tool)
		}
	}
	return &Runtime{Root: root, Tools: tools, flush: func() error { _, err := writer.Flush(q, root); return err }, close: func() error {
		q.Close()
		_, flushErr := writer.Flush(q, root)
		return errors.Join(flushErr, eng.Close(), owner.Close())
	}}, nil
}

// Drop joins no new work and closes an idle runtime before its storage is removed.
func (p *Pool) Drop(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	r := p.open[id]
	if r == nil {
		return nil
	}
	if r.users != 0 {
		return ErrCapacity
	}
	if err := r.close(); err != nil {
		return err
	}
	delete(p.open, id)
	return nil
}

func (p *Pool) FlushAll() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, r := range p.open {
		if r.users != 0 {
			return ErrCapacity
		}
		if err := r.flush(); err != nil {
			return err
		}
	}
	return nil
}

func (r *Runtime) Flush() error { return r.flush() }

// recoverSearch scans canonical state once. Calling per-fact projection reloads
// here would make cold opens quadratic in the number of saved memories.
func recoverSearch(ctx context.Context, root string, eng *index.SQLite, embedding embed.Embedder) error {
	projection, err := store.LoadMemoryProjection(store.NewSourceStore(root))
	if err != nil {
		return err
	}
	chunks, err := eng.AllChunks(ctx)
	if err != nil {
		return err
	}
	existing := make(map[string]index.Hit, len(chunks))
	for _, hit := range chunks {
		existing[hit.ChunkRef] = hit
	}
	eligible, err := index.RetrievalEligibility(root, projection, true, true, time.Now())
	if err != nil {
		return err
	}
	for _, fact := range projection.All() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if fact.Expired(time.Now()) {
			continue
		}
		ref := "fact:" + fact.SHA256
		hit := index.Hit{ChunkRef: ref, EntitySlug: fact.Payload.EntitySlug, Text: fact.Payload.Fact, SourceSHA256: fact.SHA256, Kind: store.SourceKindMemoryFact}
		prior, found := existing[ref]
		if !found {
			if err = eng.InsertChunk(ctx, ref, hit.EntitySlug, hit.Text, hit.SourceSHA256, hit.Kind); err != nil {
				return err
			}
		} else if prior != hit {
			if err = index.RefreshMemoryFact(ctx, root, fact.SHA256, eng, time.Now()); err != nil {
				return err
			}
		}
		if !eligible(hit) || fact.Expired(time.Now()) {
			continue
		}
		present, err := eng.HasVector(ctx, ref, embedding.ModelVersion())
		if err != nil {
			return err
		}
		if present {
			continue
		}
		deadline := time.Now().Add(15 * time.Second)
		if fact.Payload.ValidUntil != nil && fact.Payload.ValidUntil.Before(deadline) {
			deadline = *fact.Payload.ValidUntil
		}
		callCtx, cancel := context.WithDeadline(ctx, deadline)
		vec, err := embedding.Embed(callCtx, hit.Text)
		cancel()
		if err != nil {
			return err
		}
		if fact.Expired(time.Now()) {
			continue
		}
		if err = eng.UpsertVector(ctx, ref, embedding.ModelVersion(), vec); err != nil {
			return err
		}
	}
	return nil
}
