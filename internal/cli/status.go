package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/index"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show brain repo status: pins, index counts, ingest health, spend, rebuild timing",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStatus(cmd.Context(), flagRoot, cmd.OutOrStdout(), time.Now())
		},
	}
}

// runStatus renders `serenity status` (RFC section 16, plan T1.17):
// model/engine pins and index counts (M0/v0), plus per-connector ingest
// lag and health, jobs depth, spend to date, and last-rebuild timing
// (v1). now is injected -- rather than read from time.Now() below --
// so the golden test (T1.17 acc: "renders every field deterministically")
// can pin "time since" math to fixed values without a real clock; the
// same shape as internal/index's Clock/WithClock and internal/extract's
// ex.now hook.
func runStatus(ctx context.Context, root string, out io.Writer, now time.Time) error {
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return fmt.Errorf("not a brain repo (run `serenity init`?): %w", err)
	}
	_, _ = fmt.Fprintf(out, "models    embedding=%s extraction=%s\n", cfg.Models.Embedding, cfg.Models.Extraction)
	_, _ = fmt.Fprintf(out, "engine    %s\n", cfg.Index.Engine)

	eng, err := openIndex(root)
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()

	if err := printStats(ctx, eng, out); err != nil {
		return err
	}

	jobs, err := eng.Jobs(ctx)
	if err != nil {
		return fmt.Errorf("status: read jobs: %w", err)
	}
	printConnectorHealth(out, jobs, now)
	printJobsDepth(out, jobs)

	spend, err := eng.SpendRows(ctx)
	if err != nil {
		return fmt.Errorf("status: read spend: %w", err)
	}
	printSpend(out, spend)

	if err := printRebuildTiming(ctx, out, eng, now); err != nil {
		return err
	}
	return nil
}

// connectorHealth is one connector's current health plus ingest lag, the
// two facts plan T1.17 asks `serenity status` to show side by side:
// LastStatus can be unhealthy (failed/interrupted) while an earlier
// successful run still makes Lag small, and vice versa.
type connectorHealth struct {
	Name         string
	LastStatus   string
	HasSucceeded bool
	Lag          time.Duration
}

// summarizeConnectors groups jobs (index.SQLite.Jobs -- most recent
// first) by connector name. LastStatus is that connector's most recent
// job's status; Lag is "now minus the FinishedAt of its most recent
// index.JobSucceeded job" (plan T1.17: "derive from the jobs table's
// timestamps"). A connector that has never succeeded reports
// HasSucceeded=false; Lag is meaningless in that case and callers must
// print "never" instead. Returned sorted by name for deterministic CLI
// output.
func summarizeConnectors(jobs []index.Job, now time.Time) []connectorHealth {
	seen := make(map[string]*connectorHealth)
	for _, j := range jobs {
		h, ok := seen[j.Connector]
		if !ok {
			h = &connectorHealth{Name: j.Connector, LastStatus: j.Status}
			seen[j.Connector] = h
		}
		if j.Status == index.JobSucceeded && !h.HasSucceeded {
			h.HasSucceeded = true
			h.Lag = now.Sub(j.FinishedAt)
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]connectorHealth, 0, len(names))
	for _, name := range names {
		out = append(out, *seen[name])
	}
	return out
}

func printConnectorHealth(out io.Writer, jobs []index.Job, now time.Time) {
	connectors := summarizeConnectors(jobs, now)
	if len(connectors) == 0 {
		_, _ = fmt.Fprintln(out, "connector  none configured (no jobs recorded yet)")
		return
	}
	for _, c := range connectors {
		lag := "never"
		if c.HasSucceeded {
			lag = c.Lag.Round(time.Second).String()
		}
		_, _ = fmt.Fprintf(out, "connector  %s status=%s lag=%s\n", c.Name, c.LastStatus, lag)
	}
}

// printJobsDepth reports the jobs table's status breakdown (plan T1.17
// "jobs depth"). In this single-process CLI model a "running" row past
// the run that started it means the process died before FinishJob --
// index.SQLite.SweepInterrupted reclaims those on the next sync, so a
// nonzero running count here between syncs is a real signal, not noise.
func printJobsDepth(out io.Writer, jobs []index.Job) {
	var running, succeeded, failed, interrupted int
	for _, j := range jobs {
		switch j.Status {
		case index.JobRunning:
			running++
		case index.JobSucceeded:
			succeeded++
		case index.JobFailed:
			failed++
		case index.JobInterrupted:
			interrupted++
		}
	}
	_, _ = fmt.Fprintf(out, "jobs       total=%d running=%d succeeded=%d failed=%d interrupted=%d\n",
		len(jobs), running, succeeded, failed, interrupted)
}

// printSpend reports spend to date (RFC section 16 "spend/day and
// projected month" -- T1.17 scopes this to the running total the spend
// ledger holds; per-day/projected-month rollups are a later milestone).
func printSpend(out io.Writer, rows []index.SpendRow) {
	var totalCost float64
	for _, r := range rows {
		totalCost += r.CostUSD
	}
	_, _ = fmt.Fprintf(out, "spend      calls=%d cost_usd=$%.4f\n", len(rows), totalCost)
}

// printRebuildTiming reports the most recent Rebuild's duration and age
// (RFC section 16 "rebuild timing"). A brain that has only run `serenity
// init` has no rebuild record yet -- reported explicitly, not as an
// error.
func printRebuildTiming(ctx context.Context, out io.Writer, eng *index.SQLite, now time.Time) error {
	rec, err := eng.LastRebuildTiming(ctx)
	if errors.Is(err, index.ErrNoRebuildRecord) {
		_, _ = fmt.Fprintln(out, "rebuild    never (run `serenity sync`)")
		return nil
	}
	if err != nil {
		return fmt.Errorf("status: read rebuild timing: %w", err)
	}
	_, _ = fmt.Fprintf(out, "rebuild    last=%s ago took=%s\n",
		now.Sub(rec.At).Round(time.Second), rec.Duration.Round(time.Millisecond))
	return nil
}
