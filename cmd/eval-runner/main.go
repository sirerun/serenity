// Command eval-runner is plan T1.22's thin CLI wrapper around
// internal/eval/runner: one call scores either the per-push cached eval
// gate (ci.yml, ModeCached, zero network calls) or the nightly live eval
// (nightly-eval.yml, ModeLive, real internal/router-backed model calls
// bounded by an aggregate USD cap), writing the resulting
// evals/report.json shape to disk and printing a human-readable
// per-family summary to stdout.
//
// This lives outside cmd/serenity and internal/cli deliberately: T1.15 is
// concurrently wiring serenity's own extract/sync commands in that
// package, and an eval-workflow runner is CI tooling, not a brain-repo
// operation a Serenity user would run -- it has no reason to grow into a
// serenity subcommand.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/sirerun/serenity/internal/eval/runner"
	"github.com/sirerun/serenity/internal/extract"
	"github.com/sirerun/serenity/internal/router"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "eval-runner:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("eval-runner", flag.ContinueOnError)
	corpus := fs.String("corpus", "evals/corpora/ava", "corpus directory")
	mode := fs.String("mode", "cached", "cached | live")
	fixture := fs.String("fixture", "evals/fixtures/ava-cached-predictions.yaml", "ModeCached: predictions fixture path")
	directionCorpus := fs.String("direction-corpus", "", "plan T3.16: additionally score this DIRECTION corpus (e.g. evals/corpora/direction), attaching a Direction section; empty skips it")
	directionFixture := fs.String("direction-fixture", "evals/fixtures/direction-cached-predictions.yaml", "-direction-corpus: cached direction.Prediction fixture path (mode cached only, no live DIRECTION classifier exists yet)")
	reconcileCorpus := fs.String("reconcile-corpus", "", "plan T2.18: additionally score this reconcile corpus (e.g. evals/corpora/reconcile) against the real internal/reconcile.Detect, attaching a Reconcile section; empty skips it")
	out := fs.String("out", "evals/report.json", "report output path")
	providerName := fs.String("provider", "anthropic", "ModeLive: anthropic | openai")
	model := fs.String("model", "claude-haiku-4-5-20251001", "ModeLive: model identifier")
	modelVersionTag := fs.String("model-version", "v1", "ModeLive: pinned-model-set version tag (RFC 0001 SS7.5)")
	budgetFlag := fs.Float64("budget-usd", -1, "aggregate USD cap for this run; -1 reads SERENITY_EVAL_BUDGET_USD, unset/0 means unlimited")
	if err := fs.Parse(args); err != nil {
		return err
	}

	budgetUSD, err := resolveBudget(*budgetFlag)
	if err != nil {
		return err
	}

	cfg := runner.Config{
		CorpusDir:   *corpus,
		Mode:        runner.Mode(*mode),
		FixturePath: *fixture,
		BudgetUSD:   budgetUSD,
	}
	if *directionCorpus != "" {
		cfg.DirectionCorpusDir = *directionCorpus
		cfg.DirectionFixturePath = *directionFixture
	}
	if *reconcileCorpus != "" {
		cfg.ReconcileCorpusDir = *reconcileCorpus
	}

	if cfg.Mode == runner.ModeLive {
		provider, modelVersion, err := buildProvider(*providerName, *model, *modelVersionTag)
		if err != nil {
			return err
		}
		ledger := runner.NewTrackingLedger(budgetUSD)
		rt := router.New(map[router.Tier]router.Provider{router.TierLocalCheap: provider}, ledger)
		cfg.Extractor = extract.New(rt, modelVersion, nil, extract.NewMemoryCache())
		cfg.Ledger = ledger
		cfg.ModelVersion = modelVersion
	}

	report, err := runner.Run(context.Background(), cfg)
	if err != nil {
		return err
	}

	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		return fmt.Errorf("write report %s: %w", *out, err)
	}

	printSummary(report)
	return nil
}

// resolveBudget applies flag > env > unlimited precedence. -budget-usd
// defaults to -1 (a real budget can never be negative), which means
// "unset by flag": fall through to SERENITY_EVAL_BUDGET_USD, and if that
// is also unset, the run is unlimited (0).
func resolveBudget(flagValue float64) (float64, error) {
	if flagValue >= 0 {
		return flagValue, nil
	}
	v := os.Getenv("SERENITY_EVAL_BUDGET_USD")
	if v == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("SERENITY_EVAL_BUDGET_USD=%q: %w", v, err)
	}
	return parsed, nil
}

func buildProvider(name, model, versionTag string) (router.Provider, string, error) {
	switch name {
	case "anthropic":
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, "", fmt.Errorf("mode live with -provider anthropic requires ANTHROPIC_API_KEY")
		}
		p := &router.AnthropicProvider{APIKey: key, Model: model, Version: versionTag}
		return p, p.ModelVersion(), nil
	case "openai":
		// BaseURL follows internal/providers.go's own convention: empty
		// OPENAI_BASE_URL leaves the provider's own production-API
		// default in place; setting it points -mode live at a local
		// server (e.g. the DGX Qwen endpoint) instead. Previously
		// unread here, so a live eval-runner run against -provider
		// openai always went to the real OpenAI API regardless of any
		// local server configured elsewhere -- found running T1.23's
		// live eval against the DGX Qwen endpoint.
		//
		// ExtraBody pins temperature=0 (T1.29, found running this task's
		// own acc-line re-verification): the server's default (unset)
		// sampling temperature is non-zero on the DGX endpoint, and
		// repeated live runs against the identical held-out spans and
		// code showed individual spans flipping outcome run to run
		// (e.g. one span's predicate misclassified as "prefers" instead
		// of "said" in one run, correct in the next, no code change
		// between them). Pinning temperature=0 is kept as sound eval
		// methodology -- reproducible scoring is unambiguously wanted
		// here -- but it is NOT a proven fix for the pass/fail bar
		// itself: a controlled same-conditions rerun (identical prompt,
		// identical thinking-mode setting, differing only in this pin)
		// passed the exact same 5 of 12 target families both times. An
		// earlier draft of this comment and docs/lore.md L-0011 claimed
		// a 5/12-to-7/12 swing; that compared two runs that also
		// differed in thinking-mode (a separate, T1.31 setting) --
		// confounded, and corrected once caught. "temperature" is a
		// standard OpenAI chat-completions field (not a local-server-only
		// extension like disable_thinking's chat_template_kwargs), so it
		// is safe to send unconditionally to a real OpenAI/OpenRouter
		// endpoint too -- unlike disable_thinking, this needs no opt-in
		// flag. Deliberately scoped to eval-runner's own provider only:
		// it does NOT touch internal/providers.buildChatProvider's
		// production extraction path, which was not measured here and
		// would need its own task to evaluate (docs/lore.md L-0011).
		p := &router.OpenAICompatibleProvider{
			APIKey:    os.Getenv("OPENAI_API_KEY"),
			BaseURL:   os.Getenv("OPENAI_BASE_URL"),
			Model:     model,
			Version:   versionTag,
			ExtraBody: map[string]any{"temperature": 0},
		}
		return p, p.ModelVersion(), nil
	default:
		return nil, "", fmt.Errorf("unknown -provider %q (want anthropic or openai)", name)
	}
}

func printSummary(r runner.Report) {
	fmt.Printf("eval report: mode=%s corpus=%s generated=%s\n", r.Mode, r.Corpus, r.GeneratedAt.Format(time.RFC3339))

	families := make([]string, 0, len(r.Families))
	for f := range r.Families {
		families = append(families, f)
	}
	sort.Strings(families)
	for _, f := range families {
		s := r.Families[f]
		fmt.Printf("  %-24s P=%.3f R=%.3f F1=%.3f (tp=%d fp=%d fn=%d)\n", f, s.Precision, s.Recall, s.F1, s.TP, s.FP, s.FN)
		// T1.32: the bootstrap recall CI and its pass/fail verdict against
		// runner.RecallFloor -- the point estimate line above is kept
		// exactly as before (T1.29 and earlier tooling still read it), this
		// is an additive second line, printed only when Run computed a CI
		// (always true for a real corpus scoring run; a caller building a
		// bare runner.Report by hand, e.g. in a test, may leave it nil).
		if ci, ok := r.RecallCI[f]; ok {
			verdict := "FAIL"
			if r.RecallFloorPassed[f] {
				verdict = "PASS"
			}
			fmt.Printf("      recall_ci=[%.3f,%.3f] n=%d (%.0f%% CI) floor=%.2f %s\n",
				ci.Lower, ci.Upper, ci.N, ci.Level*100, runner.RecallFloor, verdict)
		}
	}

	if r.Spend != nil {
		fmt.Printf("  spend: $%.4f / $%.4f cap, %d calls, stopped_on_budget=%v\n",
			r.Spend.SpentUSD, r.Spend.BudgetUSD, r.Spend.Calls, r.Spend.StoppedOnBudget)
	}
	if r.Contradiction != nil {
		fmt.Printf("  contradiction: %s\n", r.Contradiction.Status)
	}

	if d := r.Direction; d != nil {
		fmt.Printf("  direction: rows_scored=%d unverified_rate=%.3f false_deny_rate=%.3f adversarial=%d/%d all_caught=%v\n",
			d.RowsScored, d.UnverifiedRate, d.FalseDenyRate, d.Adversarial.Caught, d.Adversarial.Total, d.Adversarial.AllCaught)
		if len(d.Adversarial.Missed) > 0 {
			fmt.Printf("    missed adversarial rows: %v\n", d.Adversarial.Missed)
		}

		domains := make([]string, 0, len(d.VerdictByActionClass))
		for domain := range d.VerdictByActionClass {
			domains = append(domains, domain)
		}
		sort.Strings(domains)
		for _, domain := range domains {
			verdicts := make([]string, 0, len(d.VerdictByActionClass[domain]))
			for v := range d.VerdictByActionClass[domain] {
				verdicts = append(verdicts, v)
			}
			sort.Strings(verdicts)
			for _, v := range verdicts {
				s := d.VerdictByActionClass[domain][v]
				fmt.Printf("    %-24s %-26s P=%.3f R=%.3f F1=%.3f (tp=%d fp=%d fn=%d)\n",
					domain, v, s.Precision, s.Recall, s.F1, s.TP, s.FP, s.FN)
			}
		}
	}

	if rc := r.Reconcile; rc != nil {
		fmt.Printf("  reconcile: rows_scored=%d contradiction P=%.3f R=%.3f F1=%.3f (tp=%d fp=%d fn=%d)\n",
			rc.RowsScored, rc.Contradiction.Precision, rc.Contradiction.Recall, rc.Contradiction.F1,
			rc.Contradiction.TP, rc.Contradiction.FP, rc.Contradiction.FN)

		verdicts := make([]string, 0, len(rc.VerdictConfusion))
		for v := range rc.VerdictConfusion {
			verdicts = append(verdicts, v)
		}
		sort.Strings(verdicts)
		for _, v := range verdicts {
			s := rc.VerdictConfusion[v]
			fmt.Printf("    %-24s P=%.3f R=%.3f F1=%.3f (tp=%d fp=%d fn=%d)\n",
				v, s.Precision, s.Recall, s.F1, s.TP, s.FP, s.FN)
		}
	}
}
