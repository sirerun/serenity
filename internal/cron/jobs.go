package cron

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/consolidate"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/queue"
	"github.com/sirerun/serenity/internal/writer"
)

// Sweep runs the expiry-sweeper pass (plan T2.6, RFC §0001 §8.2): pending
// disposition items older than the per-kind threshold move to deferred,
// three cycles to parked. The actual transition logic lives in
// disposition.Sweep (internal/disposition/expiry.go, tested directly
// there); this is the scheduled-job wiring T2.19 scaffolded, now filled
// in per its own doc comment ("same name, same signature, same registry
// entry"). Resurfacing a parked item is not part of this scheduled job --
// it happens when new evidence arrives on a specific item, not on a
// sweep timer (disposition.Store.Resurface, called by whichever caller
// owns that correlation).
func Sweep(ctx context.Context, root string, clock Clock) error {
	eng, err := providers.OpenIndex(root)
	if err != nil {
		return fmt.Errorf("cron: sweep: open index: %w", err)
	}
	defer func() { _ = eng.Close() }()

	store := disposition.NewStore(eng)
	if _, err := disposition.Sweep(ctx, store, nil, clock.Now()); err != nil {
		return fmt.Errorf("cron: sweep: %w", err)
	}
	return recordRun(root, "sweep", clock.Now())
}

// Consolidate runs the nightly consolidate pass (plan T2.14): summary
// fences with freshness banners, shard-head refresh, and re-embedding of
// changed chunks. Unpinned embeddings explicitly select FTS-only operation;
// configured-but-unavailable providers fail instead of claiming completion.
func Consolidate(ctx context.Context, root string, clock Clock) error {
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return err
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()
	var embedder index.Embedder
	if cfg.Models.Embedding != "" && cfg.Models.Embedding != "none@v0" {
		r, ok, note := providers.BuildEmbeddingRouter(cfg, &providers.IndexSpendLedger{Eng: eng})
		if !ok {
			return fmt.Errorf("cron: consolidate: %s", note)
		}
		embedder = &embed.RouterEmbedder{Router: r, Pin: cfg.Models.Embedding}
	}
	q := writer.NewQueue(nil)
	defer q.Close()
	if _, err := consolidate.Run(ctx, root, cfg, q, eng, embedder, clock.Now()); err != nil {
		return fmt.Errorf("cron: consolidate: %w", err)
	}
	return recordRun(root, "consolidate", clock.Now())
}

// SLO computes the same live queue metrics used by status and persists the result.
func SLO(ctx context.Context, root string, clock Clock) error {
	eng, err := providers.OpenIndex(root)
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()
	now := clock.Now().UTC()
	snapshot, err := queue.Compute(ctx, disposition.NewStore(eng), queue.Config{}, now)
	if err != nil {
		return err
	}
	return recordDetails(root, "slo", now, snapshot)
}
