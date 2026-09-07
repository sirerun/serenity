package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/sirerun/serenity/internal/providers"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/compose"
	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/search"
)

func newAskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ask <question>",
		Short: "Compose a cited answer from the brain's accumulated claims (RFC 0001 section 11)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAsk(cmd.Context(), flagRoot, strings.Join(args, " "), cmd.OutOrStdout())
		},
	}
	return cmd
}

// askAnswerFor is askAnswer's shared core: given an already-resolved
// composer (compose.Completer) and search store/embedder, run the exact
// composition runAsk performs. askAnswer (production) resolves composer
// and embedder from serenity.yml/credentials via
// providers.Build*Router -- a drift test (T4.9) calls this directly with
// an injected fake Completer instead, the same dependency-injection seam
// internal/server/memory.Deps.Composer already gives the protocol side,
// so both paths can be driven identically in tests without live provider
// credentials or network calls.
func askAnswerFor(ctx context.Context, root string, cfg *config.Config, eng search.Store, embedder embed.Embedder, composer compose.Completer, modelVersion, question string) (compose.Answer, error) {
	c := compose.New(root, cfg, eng, embedder, composer, modelVersion)
	return c.Ask(ctx, question)
}

// askAnswer runs the exact composition runAsk prints, returning the
// compose.Answer itself plus an "unavailable" note when no composer model
// is configured (ok=false; answer is the zero value and must not be
// read). Split out so a drift test (T4.9, CLI vs protocol) can invoke the
// identical code path `ask` runs and compare it against MEMORY_VERBS's
// synthesize verb (internal/server/memory) without reimplementing runAsk's
// own logic -- a genuine drift test has to exercise the real CLI
// function, not a hand-rolled duplicate of it.
func askAnswer(ctx context.Context, root, question string) (answer compose.Answer, ok bool, note string, err error) {
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return compose.Answer{}, false, "", fmt.Errorf("not a brain repo (run `serenity init`?): %w", err)
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		return compose.Answer{}, false, "", err
	}
	defer func() { _ = eng.Close() }()

	ledger := &providers.IndexSpendLedger{Eng: eng}

	r, rok, rnote := providers.BuildComposerRouter(cfg, ledger)
	if !rok {
		return compose.Answer{}, false, rnote, nil
	}

	// Query-time embedding widens which subjects chunk search surfaces
	// (compose.Composer.relevantSubjects); an unpinned or uncredentialed
	// embedding model degrades to FTS-only relevance, same fallback
	// runSearch documents for search itself -- never an error, never a
	// silent skip.
	var embedder embed.Embedder
	var embedNote string
	if er, eok, enote := providers.BuildEmbeddingRouter(cfg, ledger); eok {
		embedder = &embed.RouterEmbedder{Router: er, Pin: cfg.Models.Embedding}
	} else {
		embedNote = enote
	}

	a, err := askAnswerFor(ctx, root, cfg, eng, embedder, r, cfg.Models.Composer, question)
	if err != nil {
		return compose.Answer{}, false, "", fmt.Errorf("ask: %w", err)
	}
	return a, true, embedNote, nil
}

// runAsk is the T1.12 CLI surface over internal/compose: RFC section 11's
// composer -- a cited answer with any superseded fact's chain rendered
// alongside it, or an explicit gap statement naming the newest evidence's
// age, never a fabricated answer.
func runAsk(ctx context.Context, root, question string, out io.Writer) error {
	answer, ok, note, err := askAnswer(ctx, root, question)
	if err != nil {
		return err
	}
	if !ok {
		_, _ = fmt.Fprintln(out, note)
		return nil
	}
	if note != "" {
		_, _ = fmt.Fprintf(out, "%s -- widening query relevance to full-text/lexical matching only\n", note)
	}

	if answer.Gap != "" {
		_, _ = fmt.Fprintln(out, answer.Gap)
		return nil
	}
	_, _ = fmt.Fprintln(out, answer.Text)
	return nil
}
