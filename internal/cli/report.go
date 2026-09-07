package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/report"
)

func newReportCmd() *cobra.Command {
	var exportPath string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Render the weekly report card: claims by state, corrections, ladder, spend, ingest/reconcile/queue health, repo growth, rebuild timing (RFC section 16)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReport(cmd.Context(), flagRoot, exportPath, cmd.OutOrStdout(), time.Now())
		},
	}
	cmd.Flags().StringVar(&exportPath, "export", "", "write the full report as indented JSON to this path (manual export, RFC section 16 -- shareable only by hand); prints a human-readable summary to stdout when omitted")
	return cmd
}

// runReport is `serenity report`'s implementation (plan T5.11). It opens
// the same *index.SQLite handle every other verb in this package uses
// (providers.OpenIndex), measures the brain repo's own on-disk size and
// records it (RFC section 16's "repo growth/month" needs a history of
// samples, not just one point-in-time number -- see
// index.RecordRepoSizeSample's own doc), then assembles internal/report's
// Report over that state. Nothing here calls a provider, a router, or any
// network-capable code path -- see internal/report's own package doc and
// TestReportNoNetwork.
func runReport(ctx context.Context, root, exportPath string, out io.Writer, now time.Time) error {
	eng, err := providers.OpenIndex(root)
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()

	size, err := report.MeasureRepoSize(root)
	if err != nil {
		return fmt.Errorf("report: measure repo size: %w", err)
	}
	if err := eng.RecordRepoSizeSample(ctx, size, now); err != nil {
		return fmt.Errorf("report: record repo size sample: %w", err)
	}

	rep, err := report.Build(ctx, eng, now)
	if err != nil {
		return err
	}

	if exportPath == "" {
		return printReport(out, rep)
	}

	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return fmt.Errorf("report: marshal: %w", err)
	}
	if err := os.WriteFile(exportPath, b, 0o644); err != nil {
		return fmt.Errorf("report: write %s: %w", exportPath, err)
	}
	_, err = fmt.Fprintf(out, "report exported to %s\n", exportPath)
	return err
}

// printReport renders rep as `serenity status`'s own plain-text lines do
// (internal/cli/status.go) -- one metric per line, "n/a" for a nil
// pointer rather than a misleading zero.
func printReport(out io.Writer, r report.Report) error {
	_, _ = fmt.Fprintf(out, "report     generated=%s\n", r.GeneratedAt.Format(time.RFC3339))

	states := make([]string, 0, len(r.ClaimsByState))
	for s := range r.ClaimsByState {
		states = append(states, s)
	}
	sort.Strings(states)
	if len(states) == 0 {
		_, _ = fmt.Fprintln(out, "claims     none")
	}
	for _, s := range states {
		_, _ = fmt.Fprintf(out, "claims     %s=%d\n", s, r.ClaimsByState[s])
	}

	corrections := "n/a"
	if r.CorrectionsPer100Extractions != nil {
		corrections = fmt.Sprintf("%.2f", *r.CorrectionsPer100Extractions)
	}
	_, _ = fmt.Fprintf(out, "corrections extractions=%d corrections=%d per_100=%s\n", r.Extractions, r.Corrections, corrections)

	_, _ = fmt.Fprintf(out, "reconcile  backlog=%d\n", r.ReconcileBacklog)

	q := r.DispositionQueue
	_, _ = fmt.Fprintf(out, "queue      depth=%d depth_alert=%t p50_age=%s age_alert=%t time_to_dispose=%s abandonment=%s\n",
		q.Depth, q.DepthAlert, secondsOrNA(q.P50AgeSeconds), q.AgeAlert, secondsOrNA(q.TimeToDisposeSeconds), floatOrNA(q.Abandonment))

	_, _ = fmt.Fprintf(out, "spend      month_to_date=$%.4f projected_month=$%.4f ceiling=$%.2f\n",
		r.Spend.MonthToDateUSD, r.Spend.ProjectedMonthUSD, r.Spend.CeilingUSD)

	if len(r.IngestLagByConnector) == 0 {
		_, _ = fmt.Fprintln(out, "connector  none configured (no jobs recorded yet)")
	}
	for _, c := range r.IngestLagByConnector {
		lag := "never"
		if c.LagSeconds != nil {
			lag = (time.Duration(*c.LagSeconds * float64(time.Second))).Round(time.Second).String()
		}
		_, _ = fmt.Fprintf(out, "connector  %s lag=%s\n", c.Connector, lag)
	}
	for _, c := range r.ConnectorHealth {
		mttr := "n/a"
		if c.MTTRSeconds != nil {
			mttr = (time.Duration(*c.MTTRSeconds * float64(time.Second))).Round(time.Second).String()
		}
		_, _ = fmt.Fprintf(out, "connector  %s status=%s mttr=%s\n", c.Connector, c.LastStatus, mttr)
	}

	rg := r.RepoGrowth
	growth := "n/a"
	if rg.BytesPerMonth != nil {
		growth = fmt.Sprintf("%.0f bytes/month", *rg.BytesPerMonth)
	}
	_, _ = fmt.Fprintf(out, "repo       current_bytes=%d samples=%d growth=%s\n", rg.CurrentBytes, rg.SampleCount, growth)

	if r.RebuildTiming.Never {
		_, _ = fmt.Fprintln(out, "rebuild    never (run `serenity sync`)")
	} else {
		_, _ = fmt.Fprintf(out, "rebuild    last=%s took=%s\n",
			r.RebuildTiming.LastAt.Format(time.RFC3339), time.Duration(*r.RebuildTiming.DurationMS*float64(time.Millisecond)).Round(time.Millisecond))
	}

	ladderRate := "n/a"
	if r.TrustLadderPromotionsDemotions.SampledFalseAcceptanceRate != nil {
		ladderRate = fmt.Sprintf("%.4f", *r.TrustLadderPromotionsDemotions.SampledFalseAcceptanceRate)
	}
	_, _ = fmt.Fprintf(out, "ladder     cells=%d false_acceptance_rate=%s\n", len(r.TrustLadderPromotionsDemotions.Cells), ladderRate)

	_, _ = fmt.Fprintf(out, "search     p95=%s\n", secondsOrNA(msToSeconds(r.SearchP95MS)))
	return nil
}

func secondsOrNA(v *float64) string {
	if v == nil {
		return "n/a"
	}
	return time.Duration(*v * float64(time.Second)).Round(time.Second).String()
}

func floatOrNA(v *float64) string {
	if v == nil {
		return "n/a"
	}
	return fmt.Sprintf("%.2f", *v)
}

func msToSeconds(ms *float64) *float64 {
	if ms == nil {
		return nil
	}
	v := *ms / 1000
	return &v
}
