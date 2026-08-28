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
		p := &router.OpenAICompatibleProvider{APIKey: os.Getenv("OPENAI_API_KEY"), Model: model, Version: versionTag}
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
}
