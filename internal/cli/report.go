package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/report"
)

func newReportCmd() *cobra.Command {
	var exportJSON bool
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Render the weekly report card: claims by state, corrections, ladder, spend, ingest/reconcile/queue health, repo growth, rebuild timing (RFC section 16)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReport(cmd.Context(), flagRoot, exportJSON, cmd.OutOrStdout(), time.Now())
		},
	}
	cmd.Flags().BoolVar(&exportJSON, "export", false, "print the report as standalone JSON to stdout")
	return cmd
}

// runReport observes existing local state. It neither migrates the index nor
// records samples; export remains an explicit stdout operation.
func runReport(ctx context.Context, root string, exportJSON bool, out io.Writer, now time.Time) error {
	if _, err := config.Load(filepath.Join(root, config.FileName)); err != nil {
		return fmt.Errorf("report: not a brain repo: %w", err)
	}
	eng, err := index.OpenReport(filepath.Join(root, ".serenity", "index.db"))
	if err != nil {
		return err
	}
	defer func() { _ = eng.Close() }()
	rep, err := report.Build(ctx, root, eng, now)
	if err != nil {
		return err
	}
	if !exportJSON {
		return printReport(out, rep)
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(rep); err != nil {
		return fmt.Errorf("report: export: %w", err)
	}
	return nil
}

// printReport renders rep as `serenity status`'s own plain-text lines do
// (internal/cli/status.go) -- one metric per line, "n/a" for a nil
// pointer rather than a misleading zero.
func printReport(destination io.Writer, r report.Report) error {
	var rendered bytes.Buffer
	out := &rendered
	_, _ = fmt.Fprintf(out, "report     generated=%s\nwindow     %s\n", r.GeneratedAt.Format(time.RFC3339), r.Window)

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
	_, _ = fmt.Fprintf(out, "corrections per_100=%s\n", corrections)

	_, _ = fmt.Fprintf(out, "reconcile  backlog=%d\n", r.ReconcileBacklog)

	q := r.DispositionQueue
	_, _ = fmt.Fprintf(out, "queue      depth=%d depth_alert=%t p50_age=%s age_alert=%t time_to_dispose=%s abandonment=%s\n",
		q.Depth, q.DepthAlert, secondsOrNA(q.P50AgeSeconds), q.AgeAlert, secondsOrNA(q.TimeToDisposeSeconds), floatOrNA(q.Abandonment))

	_, _ = fmt.Fprintf(out, "spend      month_to_date=$%.4f projected_month=$%.4f ceiling=$%.2f\n",
		r.Spend.MonthToDateUSD, r.Spend.ProjectedMonthUSD, r.Spend.CeilingUSD)

	if len(r.IngestLagByConnector) == 0 {
		_, _ = fmt.Fprintln(out, "connector  no jobs recorded")
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
		_, _ = fmt.Fprintf(out, "connector  %s status=%s observed_recovery=%s episodes=%d\n", c.Connector, c.LastStatus, mttr, c.CompletedRecoveries)
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
	reasons := make([]string, 0, len(r.UnavailableReasons))
	for metric := range r.UnavailableReasons {
		reasons = append(reasons, metric)
	}
	sort.Strings(reasons)
	for _, metric := range reasons {
		_, _ = fmt.Fprintf(out, "unavailable %s: %s\n", metric, r.UnavailableReasons[metric])
	}
	if n, err := destination.Write(rendered.Bytes()); err != nil {
		return fmt.Errorf("report: render: %w", err)
	} else if n != rendered.Len() {
		return fmt.Errorf("report: render: %w", io.ErrShortWrite)
	}
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
