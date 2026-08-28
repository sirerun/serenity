package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/index"
)

// newMigrateCmd is `serenity migrate` (RFC 0001 §7.5, §10.1). --models
// selects the one migration mode this repo has a real implementation for:
// a change to serenity.yml's pinned model set. The flag exists (rather than
// --models being implied by just passing --embedding/--extraction) so a
// future migration mode (e.g. a schema migration) has an unambiguous
// sibling flag to add later instead of overloading this command's meaning.
func newMigrateCmd() *cobra.Command {
	var models bool
	var embeddingPin, extractionPin string
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate a pinned model to a new version without rewriting claims in place",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !models {
				return fmt.Errorf("migrate: specify a migration mode (--models)")
			}
			if embeddingPin == "" && extractionPin == "" {
				return fmt.Errorf("migrate --models: specify at least one of --embedding or --extraction")
			}
			return runMigrateModels(cmd.Context(), flagRoot, embeddingPin, extractionPin, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&models, "models", false, "migrate the pinned model set")
	cmd.Flags().StringVar(&embeddingPin, "embedding", "", "new embedding model pin (<model>@<version>)")
	cmd.Flags().StringVar(&extractionPin, "extraction", "", "new extraction model pin (<model>@<version>)")
	return cmd
}

// runMigrateModels is `serenity migrate --models` (RFC §7.5: "Changing a
// pin is a migration"). It is deliberately index-only, unlike `serenity
// extract`: it never calls extractClaims/ingest.Write, so no claim in
// brain/ fences or shards is ever rewritten in place by a model-pin change
// -- the "no claim rewritten in place" invariant holds structurally,
// because this function has no code path that can write one.
//
// Embedding pin: the side this task can actually build. T1.10's vectors
// table already keys by (chunk_ref, model) and never mixes pins in
// search, so migrating is: report how many indexed chunks are
// pending_reembed under the new pin (index.PendingReembed -- every chunk,
// the first time a pin is used), record the new pin in serenity.yml, then
// run the same rebuild+reembed pass `serenity extract` runs. "Staged" in
// the sense the RFC means it: for the entire time between "pin switched"
// and "reembed finished" (one process here, since ReembedMissing runs to
// completion before this returns; an interrupted run leaves it staged
// across the next `sync`/`extract`/`migrate` invocation, which resumes
// from wherever ReembedMissing's own skip-if-already-embedded check left
// off), a chunk lacking the new pin's vector is served by FTS
// (internal/embed.Search) rather than compared against the old pin's
// vector -- old vectors are never searched under the new pin regardless
// of whether the row is still physically present or already wiped by
// index.Rebuild's ResetAll.
//
// Extraction pin: re-extraction as "a new observation pass whose diffs
// flow through reconciliation" (RFC §10.2) needs the reconciler, which is
// E2/M2 work this repo does not have yet. Recording the new pin is real
// (the next `serenity extract` run uses it), but this command does not --
// and, per the invariant above, must not -- perform or simulate that pass
// itself; the gap is reported to the operator, never silently skipped or
// papered over with a reuse of the old extraction output.
func runMigrateModels(ctx context.Context, root, embeddingPin, extractionPin string, out io.Writer) error {
	cfgPath := filepath.Join(root, config.FileName)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("not a brain repo (run `serenity init`?): %w", err)
	}
	eng, err := openIndex(root)
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()

	changedEmbedding := embeddingPin != "" && embeddingPin != cfg.Models.Embedding
	if extractionPin != "" && extractionPin != cfg.Models.Extraction {
		_, _ = fmt.Fprintf(out, "models.extraction: %q -> %q; re-extraction through reconciliation is not yet built (E2/M2) -- no claim is rewritten, the pin is recorded for the next `serenity extract` run only\n",
			cfg.Models.Extraction, extractionPin)
		cfg.Models.Extraction = extractionPin
	}
	if changedEmbedding {
		pending, err := index.PendingReembed(ctx, eng, embeddingPin)
		if err != nil {
			return fmt.Errorf("migrate: count chunks pending_reembed under %q: %w", embeddingPin, err)
		}
		_, _ = fmt.Fprintf(out, "models.embedding: %q -> %q; %d chunk(s) pending_reembed (served by FTS until re-embedded)\n",
			cfg.Models.Embedding, embeddingPin, pending)
		cfg.Models.Embedding = embeddingPin
	}

	if err := cfg.Save(cfgPath); err != nil {
		return fmt.Errorf("migrate: save %s: %w", config.FileName, err)
	}

	if !changedEmbedding {
		// Extraction-only migration: no vector work to stage, and running
		// Rebuild here would still be a harmless index refresh, but doing
		// it unconditionally keeps this command's behavior uniform (a
		// migration always leaves the index derived from the current
		// pinned set) without pretending to embed under an unchanged pin.
		return rebuildAndReport(ctx, root, cfg, eng, out)
	}

	if err := rebuildTimed(ctx, root, cfg, eng); err != nil {
		return err
	}
	ledger := &spendLedgerAdapter{eng: eng}
	if err := reembedChunks(ctx, cfg, ledger, eng, out); err != nil {
		return err
	}
	return printStats(ctx, eng, out)
}

// rebuildAndReport runs a plain rebuild (no re-embed step -- there is
// nothing new to embed when the embedding pin did not change) and prints
// the resulting stats, for the extraction-only migration path.
func rebuildAndReport(ctx context.Context, root string, cfg *config.Config, eng *index.SQLite, out io.Writer) error {
	if err := rebuildTimed(ctx, root, cfg, eng); err != nil {
		return err
	}
	return printStats(ctx, eng, out)
}
