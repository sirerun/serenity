package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/direction/interview"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/providers"
)

// newInterviewCmd wires `serenity interview` (RFC 0001 section 10.4, plan
// T3.4): the first-run wizard that walks the question bank in
// internal/direction/interview/questions.yaml (~30 questions across
// intent, constraint, and standing categories) and stages one
// disposition.KindPreceptDraft item per answered question. It writes
// nothing to .dira itself -- review and accept each draft with `serenity
// inbox`, the same review surface T2.5 built and T3.11's KindDecompose
// already extends (internal/cli/inbox.go's runInteractive).
func newInterviewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "interview",
		Short: "Run the first-run interview wizard: answers become staged precept drafts, one disposition each",
		Long: "interview asks the question bank in internal/direction/interview/questions.yaml\n" +
			"one question at a time. Press Enter with no answer to skip a question. Every\n" +
			"non-blank answer becomes exactly one disposition.KindPreceptDraft item --\n" +
			"nothing is written to the ledger by this command; review and accept drafts\n" +
			"with `serenity inbox`.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInterview(cmd.Context(), flagRoot, cmd.InOrStdin(), cmd.OutOrStdout(), time.Now())
		},
	}
	return cmd
}

// runInterview opens root's derived index and a Composer-pinned judgment
// router (BuildComposerRouter -- see interview.buildDraftPrompt's own doc
// comment for why draft synthesis reuses the Composer pin rather than a
// dedicated one) then drives interview.Run over the default question
// bank. An unpinned or uncredentialed Composer model is reported and
// skipped, not an error -- the same graceful-skip contract every other
// Build*Router-consuming command in this package (runAsk, runSync)
// already follows.
func runInterview(ctx context.Context, root string, in io.Reader, out io.Writer, now time.Time) error {
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return fmt.Errorf("not a brain repo (run `serenity init`?): %w", err)
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()
	dispStore := disposition.NewStore(eng)

	ledger := &providers.IndexSpendLedger{Eng: eng}
	r, ok, note := providers.BuildComposerRouter(cfg, ledger)
	if !ok {
		_, _ = fmt.Fprintln(out, "interview: "+note)
		return nil
	}

	questions, err := interview.DefaultQuestions()
	if err != nil {
		return fmt.Errorf("interview: %w", err)
	}

	staged, err := interview.Run(ctx, r, dispStore, questions, in, out, now)
	if err != nil {
		return fmt.Errorf("interview: %w", err)
	}
	_, _ = fmt.Fprintf(out, "interview: staged %d precept draft(s); review with `serenity inbox`\n", staged)
	return nil
}
