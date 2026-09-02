package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/sirerun/serenity/internal/providers"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/connector"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/extract"
	"github.com/sirerun/serenity/internal/extract/chunk"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/ingest"
	"github.com/sirerun/serenity/internal/router"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

func newSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Poll connectors, ingest new sources, and rebuild the derived index",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSync(cmd.Context(), flagRoot, cmd.OutOrStdout())
		},
	}
}

func newExtractCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "extract [all]",
		Short: "Run extraction over ingested sources",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 && args[0] != "all" {
				return fmt.Errorf("unknown extract argument %q (only \"all\" is accepted)", args[0])
			}
			return runExtract(cmd.Context(), flagRoot, cmd.OutOrStdout())
		},
	}
}

// runSync is `serenity sync` (RFC 0001 §9, §10.1): the "poll, write, index"
// half of the ingest pipeline. It sweeps any job left "running" by a
// process that died mid-poll (index.SQLite.SweepInterrupted's own doc
// comment names this as its intended call site), polls every connector
// serenity.yml configures, dedups and commits newly-fetched sources, then
// rebuilds the derived index -- wipe-safe and deterministic, per
// index.Rebuild's own contract, which as of T1.15 also indexes every
// stored source's raw text for full-text search (see rebuild.go), not
// only fence/shard-derived claims. Extraction into claims is
// `serenity extract`'s job, not sync's -- unchanged from the pre-T1.15
// division of labor.
func runSync(ctx context.Context, root string, out io.Writer) error {
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return fmt.Errorf("not a brain repo (run `serenity init`?): %w", err)
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()

	if _, err := eng.SweepInterrupted(ctx); err != nil {
		return fmt.Errorf("sync: sweep interrupted jobs: %w", err)
	}

	if err := pollConnectors(ctx, root, cfg, eng, out); err != nil {
		return err
	}

	if err := rebuildTimed(ctx, root, cfg, eng); err != nil {
		return err
	}
	return printStats(ctx, eng, out)
}

// pollConnectors runs every connector serenity.yml configures
// (buildConnectors) through one poll cycle each, writes each fetched item
// into the content-addressed source store (dedup on raw bytes, T1.2), and
// commits every genuinely new source file -- scoped to exactly the paths
// just written, never a blanket `git add .` -- through the same writer
// queue/Flush mechanism fence and shard writes use (RFC §7.7), so a
// human's own uncommitted edits elsewhere in the working tree are never
// swept into this commit.
func pollConnectors(ctx context.Context, root string, cfg *config.Config, eng *index.SQLite, out io.Writer) error {
	connectors, err := buildConnectors(root, cfg)
	if err != nil {
		return fmt.Errorf("sync: %w", err)
	}
	if len(connectors) == 0 {
		_, _ = fmt.Fprintln(out, "no connectors configured (see docs/connectors/README.md); nothing to poll")
		return nil
	}

	priorJobs, err := eng.Jobs(ctx)
	if err != nil {
		return fmt.Errorf("sync: read job history: %w", err)
	}

	ss := store.NewSourceStore(root)
	q := writer.NewQueue(nil)
	defer q.Close()

	for _, c := range connectors {
		cursor := lastCursor(priorJobs, c.Name())
		items, _, pollErr := connector.Run(ctx, eng, c, cursor)
		if pollErr != nil {
			// One connector's failure does not abort the others -- each
			// already recorded its own failed job row via connector.Run,
			// and its cursor did not advance, so the next sync retries it.
			_, _ = fmt.Fprintf(out, "%s: poll failed: %v\n", c.Name(), pollErr)
			continue
		}

		newCount := 0
		for _, item := range items {
			src, err := c.ToSource(item)
			if err != nil {
				return fmt.Errorf("sync: %s: convert item to source: %w", c.Name(), err)
			}
			sum := sha256.Sum256(item.Bytes)
			sha := hex.EncodeToString(sum[:])
			isNew := !ss.Exists(sha)

			written, err := ss.Write(item.Bytes, src)
			if err != nil {
				return fmt.Errorf("sync: %s: write source: %w", c.Name(), err)
			}
			if isNew {
				newCount++
				for _, p := range sourceGitPaths(ss, written) {
					if res := q.Submit(writer.Job{Path: p, Render: sourceAlreadyWritten}); res.Err != nil {
						return fmt.Errorf("sync: %s: mark %s touched: %w", c.Name(), p, res.Err)
					}
				}
			}
		}
		_, _ = fmt.Fprintf(out, "%s: %d item(s) polled, %d new source(s)\n", c.Name(), len(items), newCount)
	}

	committed, err := writer.Flush(q, root)
	if err != nil {
		return fmt.Errorf("sync: commit new sources: %w", err)
	}
	if committed {
		_, _ = fmt.Fprintln(out, "committed new sources")
	}
	return nil
}

// sourceAlreadyWritten is the Render function for a source-file Queue job:
// SourceStore.Write already put the bytes on disk directly (content-
// addressed writes never race a human edit at that path -- a fresh sha256
// directory cannot already exist with different content), so there is
// nothing left to render. Submitting it through the queue anyway exists
// only to mark the path "touched" for writer.Flush's git add scoping.
func sourceAlreadyWritten() ([]byte, error) { return nil, nil }

// lastCursor returns the most recent non-empty cursor named's connector
// recorded, or nil for a connector that has never successfully advanced
// past its start (including one that has only ever been interrupted --
// SweepInterrupted's FinishJob call passes a nil cursor, so an interrupted
// row is skipped here in favor of an earlier one that has a real value).
// jobs must be index.SQLite.Jobs' own return, already most-recent-first.
func lastCursor(jobs []index.Job, name string) connector.Cursor {
	for _, j := range jobs {
		if j.Connector == name && j.Cursor != nil {
			return connector.Cursor(j.Cursor)
		}
	}
	return nil
}

// sourceGitPaths returns the canonical-repo paths one written source
// occupies -- meta.yaml always, plus bytes unless the source is
// index_only (§7.4: index_only bytes stay out of git; SourceStore's own
// ignoreBytes call already added the .gitignore entry for it).
func sourceGitPaths(ss *store.SourceStore, src domain.Source) []string {
	dir := ss.DirFor(src.SHA256)
	paths := []string{filepath.Join(dir, "meta.yaml")}
	if !src.IndexOnly {
		paths = append(paths, filepath.Join(dir, "bytes"))
	}
	return paths
}

// runExtract is `serenity extract` (RFC §9, §10.1): the "chunk, extract,
// write, embed" half of the ingest pipeline, over every source the store
// has ever recorded. `extract` and `extract all` are the same full pass
// (see extractClaims) -- there is no per-source "already extracted" state
// yet to make the distinction the RFC's "extract all" phrasing implies
// meaningful, so this does not invent one silently.
func runExtract(ctx context.Context, root string, out io.Writer) error {
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return fmt.Errorf("not a brain repo (run `serenity init`?): %w", err)
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()

	ledger := &providers.IndexSpendLedger{Eng: eng}

	if err := extractClaims(ctx, root, cfg, ledger, out); err != nil {
		return err
	}
	if err := rebuildTimed(ctx, root, cfg, eng); err != nil {
		return err
	}
	if err := reembedChunks(ctx, cfg, ledger, eng, out); err != nil {
		return err
	}
	return printStats(ctx, eng, out)
}

// extractClaims runs chunk -> extract candidate observations -> write
// claims (RFC §10.1) over every stored source. A source with no
// extraction model pinned, or a pinned model with no credential
// configured (providers.BuildExtractionRouter), is reported once and the whole call
// is a documented no-op -- the same explicit-skip contract the pre-T1.15
// stub used for "models.extraction: none@v0", extended to a real-but-
// uncredentialed pin. Below-distill-threshold observations
// (extract.Result.Distill) are counted and dropped: RFC §10.1's distill
// queue is E2 work (T2.x, not yet built); Extractor.Extract's own
// contract already forbids writing a Distill observation to a fence or
// shard, so v1's honest behavior is "counted, not silently lost, not
// silently promoted."
func extractClaims(ctx context.Context, root string, cfg *config.Config, ledger router.SpendLedger, out io.Writer) error {
	r, ok, note := providers.BuildExtractionRouter(cfg, ledger)
	if !ok {
		_, _ = fmt.Fprintln(out, note)
		return nil
	}

	ss := store.NewSourceStore(root)
	sources, err := ss.All()
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	extractor := extract.New(r, cfg.Models.Extraction, cfg.FamilyNames(), nil)
	q := writer.NewQueue(nil)
	defer q.Close()
	iw := ingest.New(q, store.NewFenceWriter(root), store.NewShardStore(root), cfg)

	var written, skipped, distilled, rejected int
	for _, src := range sources {
		data, _, err := ss.Read(src.SHA256)
		if err != nil {
			return fmt.Errorf("extract: read source %s: %w", src.SHA256, err)
		}
		if !utf8.Valid(data) {
			continue // no text-extraction pipeline for binary sources yet (v1 gap, disclosed)
		}
		chunks := chunk.Split(string(data), chunk.DefaultConfig)
		if len(chunks) == 0 {
			continue
		}

		result, err := extractor.Extract(ctx, src.SHA256, chunks, router.Budget{})
		if err != nil {
			return fmt.Errorf("extract: source %s: %w", src.SHA256, err)
		}
		rejected += result.Rejected
		distilled += len(result.Distill)
		if len(result.Ready) == 0 {
			continue
		}

		stats, err := iw.Write(result.Ready)
		if err != nil {
			return fmt.Errorf("extract: write claims for source %s: %w", src.SHA256, err)
		}
		written += stats.Written
		skipped += stats.Skipped
	}

	committed, err := writer.Flush(q, root)
	if err != nil {
		return fmt.Errorf("extract: commit new claims: %w", err)
	}
	_, _ = fmt.Fprintf(out, "extraction: %d claim(s) written, %d skipped (already present), %d rejected, %d below distill threshold\n",
		written, skipped, rejected, distilled)
	if committed {
		_, _ = fmt.Fprintln(out, "committed new claims")
	}
	return nil
}

// reembedChunks fills in every indexed chunk's vector under the pinned
// embedding model (RFC §10.1's "embed" stage). index.Rebuild wipes and
// fully reconstructs the vectors table on every call (ResetAll) --
// deliberately, since the wipe-and-rebuild invariant requires vectors to
// be reproducible purely from repo bytes plus the pinned model, never
// carried forward as separate state (T1.10's TestVectorsParticipateInRebuildIdentity
// established this exact pattern: Rebuild, then re-embed every chunk from
// scratch). HasVector still gates each call within this one process run:
// a real embedding is a pure function of (pin, text), so this is about
// not paying for a redundant provider call inside a single `extract`
// invocation, not about correctness -- the next invocation re-embeds
// everything again because Rebuild wiped the table again, and that is the
// byte-identity contract working as intended, not an inefficiency this
// task introduced.
func reembedChunks(ctx context.Context, cfg *config.Config, ledger router.SpendLedger, eng *index.SQLite, out io.Writer) error {
	r, ok, note := providers.BuildEmbeddingRouter(cfg, ledger)
	if !ok {
		_, _ = fmt.Fprintln(out, note)
		return nil
	}
	embedder := &embed.RouterEmbedder{Router: r, Pin: cfg.Models.Embedding}

	// UpsertVector is an index-write primitive the file-first CI gate
	// restricts to internal/index/rebuild.go and internal/writer/ -- the
	// CLI never calls it directly, so the actual embed loop lives in
	// index.ReembedMissing.
	embedded, err := index.ReembedMissing(ctx, eng, embedder)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	_, _ = fmt.Fprintf(out, "embedding: %d chunk(s) embedded under %s\n", embedded, embedder.Pin)
	return nil
}

// rebuildTimed runs index.Rebuild and records its wall-clock duration via
// eng.RecordRebuildTiming (RFC section 16 "rebuild timing", plan T1.17) --
// `serenity status` reads this back. Timing lives outside Rebuild itself
// so a bare Rebuild call (as internal/index's own invariant tests use)
// never writes into the caches runtime table.
func rebuildTimed(ctx context.Context, root string, cfg *config.Config, eng *index.SQLite) error {
	start := time.Now()
	if err := index.Rebuild(ctx, root, cfg, eng); err != nil {
		return err
	}
	return eng.RecordRebuildTiming(ctx, time.Since(start), time.Now())
}

func printStats(ctx context.Context, eng index.Engine, out io.Writer) error {
	stats, err := eng.Stats(ctx)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(stats))
	for k := range stats {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = fmt.Fprintf(out, "%-10s %d\n", k, stats[k])
	}
	return nil
}
