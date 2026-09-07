// Package report reads local observations and canonical claims for a manual
// report. Missing instrumentation is reported explicitly, never inferred from
// unrelated populations. Reports neither collect new samples nor send data.
package report

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/sirerun/serenity/internal/config"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/queue"
	"github.com/sirerun/serenity/internal/spend"
)

// Report is one Build call's full weekly report card plus every other RFC
// section 16 metric, ready to be marshaled whole for `serenity report
// --export`.
type Report struct {
	GeneratedAt time.Time `json:"generated_at"`

	// ClaimsByState is RFC section 16's "claims by state": a count per
	// domain.State value (active/superseded/retracted) in retained canonical history, with shard authority and supersession
	// links applied. This is a current snapshot, not an extraction count.
	ClaimsByState map[string]int64 `json:"claims_by_state"`

	// Extraction outcomes and linked corrections are not persisted yet.
	Extractions                  *int64   `json:"extractions"`
	Corrections                  *int64   `json:"corrections"`
	CorrectionsPer100Extractions *float64 `json:"corrections_per_100_extractions"`
	// Reasons travel with exported data so null is not confused with healthy.
	UnavailableReasons map[string]string `json:"unavailable_reasons"`
	Window             string            `json:"window"`

	// TrustLadderPromotionsDemotions is RFC section 16's ladder metric --
	// no transition/audit history is persisted; see UnavailableReasons.
	TrustLadderPromotionsDemotions LadderSection `json:"trust_ladder_promotions_demotions"`

	// Spend is RFC section 16's "spend/day and projected month" plus the
	// weekly report card's plain "spend".
	Spend SpendSection `json:"spend"`

	// IngestLagByConnector is RFC section 16's "ingest lag", one entry per
	// connector that has ever run a job.
	IngestLagByConnector []ConnectorLag `json:"ingest_lag_by_connector"`

	// ReconcileBacklog is RFC section 16's "reconcile backlog": pending or
	// deferred KindReconcile disposition items -- unresolved conflicts a
	// human still owes a verdict on.
	ReconcileBacklog int `json:"reconcile_backlog"`

	// DispositionQueue is RFC section 16's "disposition queue depth/age
	// and time-to-dispose", sourced from internal/queue.Compute (T2.15)
	// unchanged.
	DispositionQueue QueueSection `json:"disposition_queue"`

	// RepoGrowth is RFC section 16's "repo growth/month".
	RepoGrowth RepoGrowthSection `json:"repo_growth"`

	// RebuildTiming is RFC section 16's "rebuild timing".
	RebuildTiming RebuildSection `json:"rebuild_timing"`

	// ConnectorHealth is RFC section 16's "connector health/MTTR", one
	// entry per connector that has ever run a job.
	ConnectorHealth []ConnectorHealthEntry `json:"connector_health"`

	// SearchP95MS has no persisted latency samples; see UnavailableReasons.
	SearchP95MS *float64 `json:"search_p95_ms"`
}

// LadderSection holds RFC section 16's trust-ladder metric: per-cell
// promotion/demotion history plus the aggregate sampled false-acceptance
// rate across every cell's audited auto actions.
type LadderSection struct {
	Cells []LadderCell `json:"cells"`
	// SampledFalseAcceptanceRate is nil until some caller records a
	// ladder cell's audited outcomes; see UnavailableReasons.
	SampledFalseAcceptanceRate *float64 `json:"sampled_false_acceptance_rate"`
}

// LadderCell is one (connector, predicate-family) cell's ladder state, as
// internal/ladder.Trust names it (LadderSection.Cells is empty until a
// caller persists this). No production transition recorder exists yet.
type LadderCell struct {
	Cell                string     `json:"cell"`
	Trust               string     `json:"trust"`
	PromotedAt          *time.Time `json:"promoted_at,omitempty"`
	DemotedAt           *time.Time `json:"demoted_at,omitempty"`
	SampledAutoActions  int        `json:"sampled_auto_actions"`
	FalseAcceptanceRate *float64   `json:"false_acceptance_rate"`
}

// SpendSection is RFC section 16's spend metrics: per-day totals
// (internal/spend.DailyTotals) plus the month-to-date/projected/ceiling
// figures internal/spend.ProjectMonth already computes for `serenity
// status`.
type SpendSection struct {
	PerDayUSD         map[string]float64 `json:"per_day_usd"`
	MonthToDateUSD    float64            `json:"month_to_date_usd"`
	ProjectedMonthUSD float64            `json:"projected_month_usd"`
	CeilingUSD        float64            `json:"ceiling_usd"`
}

// ConnectorLag is one connector's ingest lag: the time since its most
// recent SUCCEEDED job, matching `serenity status`'s own connector-health
// derivation (internal/cli/status.go's summarizeConnectors). LagSeconds is
// nil for a connector that has never succeeded.
type ConnectorLag struct {
	Connector    string   `json:"connector"`
	HasSucceeded bool     `json:"has_succeeded"`
	LagSeconds   *float64 `json:"lag_seconds"`
}

// QueueSection mirrors internal/queue.Snapshot's fields as JSON, keeping
// its own "*OK false means n/a" discipline: an *Seconds/Abandonment
// pointer is nil exactly when Snapshot's own OK flag was false.
type QueueSection struct {
	Depth                int      `json:"depth"`
	DepthAlert           bool     `json:"depth_alert"`
	P50AgeSeconds        *float64 `json:"p50_age_seconds"`
	AgeAlert             bool     `json:"age_alert"`
	TimeToDisposeSeconds *float64 `json:"time_to_dispose_seconds"`
	Abandonment          *float64 `json:"abandonment"`
}

// RepoGrowthSection reports current canonical regular-file bytes separately
// from retained historical samples. Growth requires at least 30 days between
// observations; delta and interval accompany the normalized 30-day rate.
type RepoGrowthSection struct {
	IntervalStart      *time.Time `json:"interval_start"`
	IntervalEnd        *time.Time `json:"interval_end"`
	ObservedDeltaBytes *int64     `json:"observed_delta_bytes"`
	Method             string     `json:"method"`
	CurrentBytes       int64      `json:"current_bytes"`
	SampleCount        int        `json:"sample_count"`
	BytesPerMonth      *float64   `json:"bytes_per_month"`
}

// RebuildSection is RFC section 16's "rebuild timing", mirroring
// `serenity status`'s own printRebuildTiming: Never is true and
// LastAt/DurationMS are both nil for a brain that has only run `serenity
// init`.
type RebuildSection struct {
	Never      bool       `json:"never"`
	LastAt     *time.Time `json:"last_at,omitempty"`
	DurationMS *float64   `json:"duration_ms,omitempty"`
}

// ConnectorHealthEntry uses completion-ordered observed job outcomes. MTTR is
// an explicitly labeled recovery proxy, not knowledge of actual outage onset.
// Unresolved episodes do not enter the mean; CompletedRecoveries is its sample
// count. Connector is an anonymous alias shared with ConnectorLag in this report.
type ConnectorHealthEntry struct {
	CompletedRecoveries int      `json:"completed_recoveries"`
	RecoveryMethod      string   `json:"recovery_method"`
	Connector           string   `json:"connector"`
	LastStatus          string   `json:"last_status"`
	MTTRSeconds         *float64 `json:"mttr_seconds"`
}

// Build assembles a current canonical snapshot and retained local observations.
// It does not write, record samples, rebuild, or use network services. now caps
// spend/job/sample observations; it cannot reconstruct past canonical state.
func Build(ctx context.Context, root string, eng *index.SQLite, now time.Time) (Report, error) {
	now = now.UTC()
	r := Report{GeneratedAt: now, Window: "current snapshot; queue disposal and recovery statistics use retained history; spend through generated_at", UnavailableReasons: map[string]string{
		"corrections_per_100_extractions":   "Extraction events and linked correction outcomes are not persisted; current claims are not an extraction denominator.",
		"trust_ladder_promotions_demotions": "Ladder transitions and completed sampled audits are not persisted.",
		"search_p95_ms":                     "Search latency samples are not persisted.",
	}}
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		return Report{}, fmt.Errorf("report: load brain config: %w", err)
	}
	r.ClaimsByState, err = canonicalClaimCounts(root, cfg)
	if err != nil {
		return Report{}, fmt.Errorf("report: canonical claims: %w", err)
	}

	ds := disposition.NewStore(eng)
	items, err := ds.List(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("report: list disposition items: %w", err)
	}
	r.ReconcileBacklog = reconcileBacklog(items)

	r.TrustLadderPromotionsDemotions = LadderSection{Cells: []LadderCell{}}

	snap, err := queue.Compute(ctx, ds, queue.DefaultConfig(), now)
	if err != nil {
		return Report{}, fmt.Errorf("report: compute queue SLOs: %w", err)
	}
	r.DispositionQueue = toQueueSection(snap)

	spendRows, err := eng.SpendRows(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("report: read spend rows: %w", err)
	}
	r.Spend = toSpendSection(spendRows, now)

	jobs, err := eng.Jobs(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("report: read jobs: %w", err)
	}
	jobs = observedJobs(jobs, now)
	r.IngestLagByConnector = ingestLag(jobs, now)
	r.ConnectorHealth = connectorHealth(jobs)
	anonymizeConnectors(&r)

	rebuildSec, err := rebuildSection(ctx, eng)
	if err != nil {
		return Report{}, err
	}
	r.RebuildTiming = rebuildSec

	samples, err := eng.RepoSizeSamples(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("report: read repo size samples: %w", err)
	}
	r.RepoGrowth = repoGrowthSection(samples, now)
	size, err := MeasureRepoSize(root)
	if err != nil {
		return Report{}, fmt.Errorf("report: measure canonical bytes: %w", err)
	}
	r.RepoGrowth.CurrentBytes = size
	if r.RepoGrowth.BytesPerMonth == nil {
		r.UnavailableReasons["repo_growth"] = "Repo size observations spanning at least 30 days are unavailable; report does not record samples."
	}

	r.SearchP95MS = nil

	return r, nil
}

// reconcileBacklog counts pending/deferred KindReconcile items -- the same
// parked-exclusion rule disposition.Store.PendingDepth applies, narrowed
// to one Kind.
func reconcileBacklog(items []disposition.Item) int {
	n := 0
	for _, it := range items {
		if it.Kind != disposition.KindReconcile {
			continue
		}
		if it.State == disposition.StatePending || it.State == disposition.StateDeferred {
			n++
		}
	}
	return n
}

func toQueueSection(snap queue.Snapshot) QueueSection {
	sec := QueueSection{Depth: snap.Depth, DepthAlert: snap.DepthAlert, AgeAlert: snap.AgeAlert}
	if snap.P50AgeOK {
		v := snap.P50Age.Seconds()
		sec.P50AgeSeconds = &v
	}
	if snap.TimeToDisposeOK {
		v := snap.TimeToDispose.Seconds()
		sec.TimeToDisposeSeconds = &v
	}
	if snap.AbandonmentOK {
		v := snap.Abandonment
		sec.Abandonment = &v
	}
	return sec
}

func toSpendSection(rows []index.SpendRow, now time.Time) SpendSection {
	observed := make([]index.SpendRow, 0, len(rows))
	for _, row := range rows {
		if !row.OccurredAt.After(now) {
			observed = append(observed, row)
		}
	}
	rows = observed
	proj := spend.ProjectMonth(rows, spend.DefaultConfig().MonthlyCeilingUSD, now)
	return SpendSection{
		PerDayUSD:         spend.DailyTotals(rows),
		MonthToDateUSD:    proj.MonthToDateUSD,
		ProjectedMonthUSD: proj.ProjectedUSD,
		CeilingUSD:        proj.CeilingUSD,
	}
}

// ingestLag reports, per connector, the time since its most recent
// SUCCEEDED job -- the same derivation `serenity status`'s
// summarizeConnectors uses (internal/cli/status.go), independently
// reimplemented here so internal/report has no dependency on
// internal/cli (which itself will depend on internal/report).
func ingestLag(jobs []index.Job, now time.Time) []ConnectorLag {
	agg := map[string]*ConnectorLag{}
	for _, j := range jobs {
		c, ok := agg[j.Connector]
		if !ok {
			c = &ConnectorLag{Connector: j.Connector}
			agg[j.Connector] = c
		}
		if j.Status == index.JobSucceeded && (!c.HasSucceeded || now.Sub(j.FinishedAt).Seconds() < *c.LagSeconds) {
			c.HasSucceeded = true
			lag := now.Sub(j.FinishedAt).Seconds()
			c.LagSeconds = &lag
		}
	}
	return connectorLagsSorted(agg)
}

func connectorLagsSorted(agg map[string]*ConnectorLag) []ConnectorLag {
	names := make([]string, 0, len(agg))
	for name := range agg {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]ConnectorLag, 0, len(names))
	for _, name := range names {
		out = append(out, *agg[name])
	}
	return out
}

// connectorHealth groups jobs by connector and computes each one's most
// recent status plus MTTR -- see ConnectorHealthEntry's own doc for the
// MTTR definition.
func connectorHealth(jobs []index.Job) []ConnectorHealthEntry {
	byConnector := map[string][]index.Job{}
	for _, j := range jobs {
		byConnector[j.Connector] = append(byConnector[j.Connector], j)
	}

	names := make([]string, 0, len(byConnector))
	for name := range byConnector {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]ConnectorHealthEntry, 0, len(names))
	for _, name := range names {
		js := byConnector[name]
		sort.SliceStable(js, func(i, k int) bool { return jobObservationAt(js[i]).Before(jobObservationAt(js[k])) })

		entry := ConnectorHealthEntry{Connector: name, LastStatus: js[len(js)-1].Status, RecoveryMethod: "mean failed/interrupted completion to next successful completion; completed episodes only"}

		var recoveries []time.Duration
		var outageStart time.Time
		inOutage := false
		for _, j := range js {
			switch j.Status {
			case index.JobFailed, index.JobInterrupted:
				if !inOutage {
					inOutage = true
					outageStart = j.FinishedAt
					if outageStart.IsZero() {
						outageStart = j.StartedAt
					}
				}
			case index.JobSucceeded:
				if inOutage {
					if !j.FinishedAt.Before(outageStart) {
						recoveries = append(recoveries, j.FinishedAt.Sub(outageStart))
					}
					inOutage = false
				}
			}
		}
		if len(recoveries) > 0 {
			var total time.Duration
			for _, d := range recoveries {
				total += d
			}
			mttr := total.Seconds() / float64(len(recoveries))
			entry.MTTRSeconds = &mttr
			entry.CompletedRecoveries = len(recoveries)
		}
		out = append(out, entry)
	}
	return out
}

func rebuildSection(ctx context.Context, eng *index.SQLite) (RebuildSection, error) {
	rec, err := eng.LastRebuildTiming(ctx)
	if err != nil {
		if err == index.ErrNoRebuildRecord {
			return RebuildSection{Never: true}, nil
		}
		return RebuildSection{}, fmt.Errorf("report: read rebuild timing: %w", err)
	}
	at := rec.At
	ms := float64(rec.Duration) / float64(time.Millisecond)
	return RebuildSection{LastAt: &at, DurationMS: &ms}, nil
}

// repoGrowthSection reports the observed change over at least 30 days,
// normalized to 30 days with the actual interval disclosed. It is not a
// prediction or a calendar-month total. Future samples are excluded.
func repoGrowthSection(samples []index.RepoSizeSample, now time.Time) RepoGrowthSection {
	observed := make([]index.RepoSizeSample, 0, len(samples))
	for _, sample := range samples {
		if !sample.At.After(now) && sample.Bytes >= 0 {
			observed = append(observed, sample)
		}
	}
	sort.Slice(observed, func(i, j int) bool { return observed[i].At.Before(observed[j].At) })
	sec := RepoGrowthSection{SampleCount: len(observed), Method: "canonical regular-file byte delta over observed interval, normalized to 30 days; minimum interval 30 days"}
	if len(observed) < 2 {
		return sec
	}
	first, last := observed[0], observed[len(observed)-1]
	if last.At.Sub(first.At) < 30*24*time.Hour {
		return sec
	}
	sec.IntervalStart, sec.IntervalEnd = &first.At, &last.At
	delta := last.Bytes - first.Bytes
	sec.ObservedDeltaBytes = &delta
	rate := float64(delta) / last.At.Sub(first.At).Hours() * 24 * 30
	sec.BytesPerMonth = &rate
	return sec
}

func jobObservationAt(j index.Job) time.Time {
	if !j.FinishedAt.IsZero() {
		return j.FinishedAt
	}
	return j.StartedAt
}

func observedJobs(jobs []index.Job, now time.Time) []index.Job {
	out := make([]index.Job, 0, len(jobs))
	for _, j := range jobs {
		if j.StartedAt.After(now) {
			continue
		}
		if j.FinishedAt.After(now) {
			j.Status = index.JobRunning
			j.FinishedAt = time.Time{}
		}
		out = append(out, j)
	}
	return out
}

// Connector identifiers can contain email addresses and local paths. Export
// ordinal aliases only. Stable within a report, not an identity across reports.
func anonymizeConnectors(r *Report) {
	aliases := map[string]string{}
	for i := range r.IngestLagByConnector {
		c := &r.IngestLagByConnector[i]
		aliases[c.Connector] = fmt.Sprintf("connector-%d", i+1)
		c.Connector = aliases[c.Connector]
	}
	for i := range r.ConnectorHealth {
		c := &r.ConnectorHealth[i]
		c.Connector = aliases[c.Connector]
	}
}
