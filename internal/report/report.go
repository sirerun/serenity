// Package report assembles RFC 0001 section 16's weekly report card and
// the metrics named earlier in that same section into one JSON-
// serializable snapshot (plan T5.11, `serenity report --export`): "metrics
// -- all local, shareable only by manual export."
//
// Build reads back state that other, already-shipped packages own --
// internal/disposition (T2.1), internal/queue (T2.15), internal/spend
// (T4.10), internal/index's jobs/spend_ledger/claims tables -- rather than
// duplicating their computations. It never calls a provider, a router, or
// any network-capable code path: every source it reads is local to the
// brain repo's own *index.SQLite handle. TestReportNoNetwork (report_test.go)
// guards that with a trapped http.DefaultTransport rather than trusting
// this comment alone.
//
// Two RFC section 16 metrics have no underlying population anywhere in
// this codebase yet, and Build says so explicitly rather than fabricating
// a number:
//
//   - Trust-ladder cell promotions/demotions. internal/ladder (T2.10) is,
//     by its own package doc, "deliberately decoupled from
//     internal/disposition" and keeps trust state in memory only
//     (Engine.state) -- nothing in this codebase durably records a
//     promotion, demotion, or sampled auto-action outcome yet, because
//     nothing wires the reconcile engine to the ladder in production.
//     LadderSection.Cells is an empty (not nil) slice until that wiring
//     exists; SampledFalseAcceptanceRate is nil for the same reason.
//   - Search p95 latency. internal/search (T1.11) has no latency-sampling
//     pipeline; nothing persists a call's duration anywhere. SearchP95MS
//     is nil until one exists.
//
// Both fields are still present on Report -- "every metric named in RFC
// section 16 appears" (plan T5.11's acc line) means the field exists and
// is honestly nil/empty, the same discipline internal/queue.Snapshot
// already applies ("a metric with no underlying population reports n/a,
// never a misleading zero").
package report

import (
	"context"
	"fmt"
	"sort"
	"time"

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
	// domain.State value (active/superseded/retracted) currently in the
	// index. Empty, not nil, on a brain with no claims yet.
	ClaimsByState map[string]int64 `json:"claims_by_state"`

	// Extractions and Corrections back CorrectionsPer100Extractions.
	// Extractions is the total claim count (every claim in the index
	// originates from an extraction, per RFC 0001 §7.6's Source ->
	// Observation -> Claim pipeline). Corrections is the count of
	// disposed KindReconcile disposition items whose verdict was reject
	// or edit_accept -- a human overriding or amending what the machine
	// proposed. This is a disclosed policy choice for what "correction"
	// means (RFC section 16 names the metric, not a formula), the same
	// class of choice T4.10's spend projection and T2.21's demoteFactor
	// already made explicitly.
	Extractions int64 `json:"extractions"`
	Corrections int64 `json:"corrections"`
	// CorrectionsPer100Extractions is nil when Extractions is zero (no
	// population to rate against yet), never a misleading 0.00.
	CorrectionsPer100Extractions *float64 `json:"corrections_per_100_extractions"`

	// TrustLadderPromotionsDemotions is RFC section 16's ladder metric --
	// see the package doc for why Cells is empty in every build today.
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

	// SearchP95MS is RFC section 16's "search p95" -- see the package doc
	// for why it is nil today.
	SearchP95MS *float64 `json:"search_p95_ms"`
}

// LadderSection holds RFC section 16's trust-ladder metric: per-cell
// promotion/demotion history plus the aggregate sampled false-acceptance
// rate across every cell's audited auto actions.
type LadderSection struct {
	Cells []LadderCell `json:"cells"`
	// SampledFalseAcceptanceRate is nil until some caller records a
	// ladder cell's audited outcomes -- see the package doc.
	SampledFalseAcceptanceRate *float64 `json:"sampled_false_acceptance_rate"`
}

// LadderCell is one (connector, predicate-family) cell's ladder state, as
// internal/ladder.Trust names it (LadderSection.Cells is empty until a
// caller persists this).
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

// RepoGrowthSection is RFC section 16's "repo growth/month", derived from
// index.RepoSizeSample history. BytesPerMonth is nil until at least two
// samples spanning a positive amount of time exist -- a single
// measurement has no growth rate to report yet, the same "n/a until a
// population exists" rule every other optional field here follows.
type RepoGrowthSection struct {
	CurrentBytes  int64    `json:"current_bytes"`
	SampleCount   int      `json:"sample_count"`
	BytesPerMonth *float64 `json:"bytes_per_month"`
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

// ConnectorHealthEntry is RFC section 16's "connector health/MTTR":
// LastStatus is that connector's most recent job's status; MTTRSeconds is
// the mean time from the start of an outage (its first
// failed/interrupted job) to the next succeeded job, averaged across
// every such recovery this connector has ever had. Nil when the
// connector has never recovered from an outage (including "never failed
// at all").
type ConnectorHealthEntry struct {
	Connector   string   `json:"connector"`
	LastStatus  string   `json:"last_status"`
	MTTRSeconds *float64 `json:"mttr_seconds"`
}

// Build assembles Report as of now from eng's already-durable state. It
// performs no writes and no network calls -- every value comes from a
// read against eng or a pure computation over what it returned.
func Build(ctx context.Context, eng *index.SQLite, now time.Time) (Report, error) {
	now = now.UTC()
	r := Report{GeneratedAt: now}

	stateCounts, err := eng.ClaimStateCounts(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("report: claim state counts: %w", err)
	}
	r.ClaimsByState = stateCounts
	var totalClaims int64
	for _, n := range stateCounts {
		totalClaims += n
	}
	r.Extractions = totalClaims

	ds := disposition.NewStore(eng)
	items, err := ds.List(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("report: list disposition items: %w", err)
	}
	r.Corrections = correctionsCount(items)
	if totalClaims > 0 {
		rate := float64(r.Corrections) / float64(totalClaims) * 100
		r.CorrectionsPer100Extractions = &rate
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
	r.IngestLagByConnector = ingestLag(jobs, now)
	r.ConnectorHealth = connectorHealth(jobs)

	rebuildSec, err := rebuildSection(ctx, eng)
	if err != nil {
		return Report{}, err
	}
	r.RebuildTiming = rebuildSec

	samples, err := eng.RepoSizeSamples(ctx)
	if err != nil {
		return Report{}, fmt.Errorf("report: read repo size samples: %w", err)
	}
	r.RepoGrowth = repoGrowthSection(samples)

	r.SearchP95MS = nil

	return r, nil
}

// correctionsCount counts disposed KindReconcile items a human rejected or
// amended -- see Report.Corrections' own doc for the disclosed
// definition.
func correctionsCount(items []disposition.Item) int64 {
	var n int64
	for _, it := range items {
		if it.Kind != disposition.KindReconcile || it.State != disposition.StateDisposed {
			continue
		}
		if it.Verdict == disposition.VerdictReject || it.Verdict == disposition.VerdictEditAccept {
			n++
		}
	}
	return n
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
		if j.Status == index.JobSucceeded && !c.HasSucceeded {
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
		sort.Slice(js, func(i, k int) bool { return js[i].StartedAt.Before(js[k].StartedAt) })

		entry := ConnectorHealthEntry{Connector: name, LastStatus: js[len(js)-1].Status}

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
					recoveries = append(recoveries, j.FinishedAt.Sub(outageStart))
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

// repoGrowthSection derives current size and a monthly growth rate from
// samples, oldest-first. BytesPerMonth stays nil with fewer than two
// samples, or when the earliest and latest samples share a timestamp (no
// elapsed time to rate against).
func repoGrowthSection(samples []index.RepoSizeSample) RepoGrowthSection {
	sec := RepoGrowthSection{SampleCount: len(samples)}
	if len(samples) == 0 {
		return sec
	}
	latest := samples[len(samples)-1]
	sec.CurrentBytes = latest.Bytes
	if len(samples) < 2 {
		return sec
	}
	earliest := samples[0]
	days := latest.At.Sub(earliest.At).Hours() / 24
	if days <= 0 {
		return sec
	}
	rate := float64(latest.Bytes-earliest.Bytes) / days * 30
	sec.BytesPerMonth = &rate
	return sec
}
