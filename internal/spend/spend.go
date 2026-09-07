// Package spend implements RFC 0001 section 9's monthly spend ceiling and
// projection (T4.10): "Every judgment-tier call writes to the spend
// ledger; a configurable monthly ceiling (default $50 -- commodity users,
// not lab budgets) queues further judgment calls behind an approval, with
// projected-spend shown in `serenity status` and the briefing before the
// ceiling trips."
//
// The spend ledger itself (the `spend_ledger` runtime table, T1.7) is
// already built and already claimed by internal/index (index.SpendRow,
// SQLite.RecordSpend/SpendRows) -- this package is the policy layer on
// top: per-day/per-month aggregation and projection (ProjectMonth), and
// the transactional gate a caller making a judgment-tier call goes
// through before it runs (Checker.CheckAndRecord). A call that would push
// month-to-date spend over the ceiling is not recorded and does not
// run -- instead it stages one internal/disposition KindEffect approval
// item (RFC's own vocabulary: a judgment call with a real cost is exactly
// what an "effect request" names), the same propose/accept split T2.1's
// DISPOSITION queue applies to every other consequential machine
// proposal. Accepting that item later (ApplyDisposedEffect) records the
// held-back row, "running" the call this one time on explicit human
// sign-off.
package spend

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/index"
)

// DefaultMonthlyCeilingUSD is RFC 0001 section 9's stated default.
const DefaultMonthlyCeilingUSD = 50.0

// Config is this package's own policy configuration. Disclosed scope: it
// is not wired into config.Config/serenity.yml the way T2.10's
// ladder.Config is -- internal/index/rebuild.go already imports
// internal/config, so a Checker needing a live *index.SQLite cannot also
// have its Config type live inside internal/config without an import
// cycle (config -> spend -> index -> config). The RFC's "configurable" is
// satisfied at the Go level (any caller can construct a non-default
// Config); YAML-level configurability is a follow-up, not this task's
// acc line.
type Config struct {
	MonthlyCeilingUSD float64
}

// DefaultConfig returns RFC section 9's published default.
func DefaultConfig() Config {
	return Config{MonthlyCeilingUSD: DefaultMonthlyCeilingUSD}
}

// EffectPayload is a KindEffect disposition item's payload for a
// judgment-tier call CheckAndRecord withheld: the full row it would have
// recorded (so ApplyDisposedEffect can record it verbatim on accept,
// without asking the caller to reconstruct or re-supply it) plus the
// month-to-date and ceiling figures a reviewer needs to make the call.
type EffectPayload struct {
	Row            index.SpendRow `json:"row"`
	MonthToDateUSD float64        `json:"month_to_date_usd"`
	CeilingUSD     float64        `json:"ceiling_usd"`
}

// Checker gates judgment-tier spend against a monthly ceiling and records
// what it allows. Construct with New; the zero value is not usable
// (Index/Disposition are required).
type Checker struct {
	Index       *index.SQLite
	Disposition *disposition.Store
	Config      Config

	// mu serializes CheckAndRecord's read-check-write sequence -- see its
	// own doc for why.
	mu sync.Mutex
}

// New builds a Checker. eng and ds must be non-nil. cfg's zero value
// (MonthlyCeilingUSD == 0) is treated as "use the default," not "ceiling
// of $0" -- a brain that never set one should not silently block every
// judgment call.
func New(eng *index.SQLite, ds *disposition.Store, cfg Config) *Checker {
	if cfg.MonthlyCeilingUSD <= 0 {
		cfg.MonthlyCeilingUSD = DefaultMonthlyCeilingUSD
	}
	return &Checker{Index: eng, Disposition: ds, Config: cfg}
}

// Decision is CheckAndRecord's outcome.
type Decision struct {
	// Recorded is true when row was written to the spend ledger.
	Recorded bool
	// ItemID is set only when Recorded is false: the KindEffect
	// disposition item id staged for human review.
	ItemID string
	// MonthToDateUSD is the ledger's month-to-date total as of this call
	// -- including row's own cost when Recorded is true, excluding it
	// (this is what the ceiling was checked against) when it is false.
	MonthToDateUSD float64
}

// CheckAndRecord is T4.10's transactional check-and-record primitive: given
// a not-yet-recorded judgment-tier spend row, it computes month-to-date
// spend as of row.OccurredAt's calendar month and either records row
// (Decision.Recorded = true) when the running total would stay at or
// under the ceiling, or stages exactly one KindEffect approval item and
// records nothing (Decision.Recorded = false) when it would cross it --
// "a judgment call that would cross the ceiling creates one approval item
// and does not run." The whole read-check-write sequence runs under one
// mutex so two concurrent calls cannot both observe headroom and both
// record, together overshooting the ceiling -- the same shape T2.8
// established for disposition.Store.Dispose's own concurrent-write race.
func (c *Checker) CheckAndRecord(ctx context.Context, row index.SpendRow, now time.Time) (Decision, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	rows, err := c.Index.SpendRows(ctx)
	if err != nil {
		return Decision{}, fmt.Errorf("spend: check and record: read ledger: %w", err)
	}
	mtd := MonthToDate(rows, row.OccurredAt)

	if mtd+row.CostUSD > c.Config.MonthlyCeilingUSD {
		payload := EffectPayload{Row: row, MonthToDateUSD: mtd, CeilingUSD: c.Config.MonthlyCeilingUSD}
		raw, err := json.Marshal(payload)
		if err != nil {
			return Decision{}, fmt.Errorf("spend: check and record: marshal payload: %w", err)
		}
		item, err := c.Disposition.Create(ctx, disposition.KindEffect, raw, "", now)
		if err != nil {
			return Decision{}, fmt.Errorf("spend: check and record: stage approval item: %w", err)
		}
		return Decision{Recorded: false, ItemID: item.ID, MonthToDateUSD: mtd}, nil
	}

	if err := c.Index.RecordSpend(ctx, row); err != nil {
		return Decision{}, fmt.Errorf("spend: check and record: record: %w", err)
	}
	return Decision{Recorded: true, MonthToDateUSD: mtd + row.CostUSD}, nil
}

// ApplyDisposedEffect applies a disposed KindEffect item's accept verdict
// (T4.10's "accepting the item runs it"): the row CheckAndRecord withheld
// is decoded from the item's own payload and recorded now, bypassing the
// ceiling this one time -- a human has explicitly signed off on this
// specific overage. Held under the same mutex as CheckAndRecord so an
// accept cannot interleave with a concurrent CheckAndRecord's own
// read-check-write.
func (c *Checker) ApplyDisposedEffect(ctx context.Context, item disposition.Item) error {
	if item.Kind != disposition.KindEffect {
		return fmt.Errorf("spend: apply disposed effect: item %s is kind %q, not %q", item.ID, item.Kind, disposition.KindEffect)
	}
	if item.State != disposition.StateDisposed || item.Verdict != disposition.VerdictAccept {
		return fmt.Errorf("spend: apply disposed effect: item %s is state=%q verdict=%q, want disposed with accept",
			item.ID, item.State, item.Verdict)
	}
	var payload EffectPayload
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		return fmt.Errorf("spend: apply disposed effect: decode payload: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.Index.RecordSpend(ctx, payload.Row); err != nil {
		return fmt.Errorf("spend: apply disposed effect: record: %w", err)
	}
	return nil
}

// dayKey returns t's UTC calendar-day key, YYYY-MM-DD.
func dayKey(t time.Time) string { return t.UTC().Format("2006-01-02") }

// monthKey returns t's UTC calendar-month key, YYYY-MM.
func monthKey(t time.Time) string { return t.UTC().Format("2006-01") }

// DailyTotals sums CostUSD per UTC calendar day across rows (RFC section
// 16: "spend/day"), keyed YYYY-MM-DD.
func DailyTotals(rows []index.SpendRow) map[string]float64 {
	out := map[string]float64{}
	for _, r := range rows {
		out[dayKey(r.OccurredAt)] += r.CostUSD
	}
	return out
}

// MonthToDate sums CostUSD for every row whose OccurredAt falls in the
// same UTC calendar month as asOf.
func MonthToDate(rows []index.SpendRow, asOf time.Time) float64 {
	key := monthKey(asOf)
	var total float64
	for _, r := range rows {
		if monthKey(r.OccurredAt) == key {
			total += r.CostUSD
		}
	}
	return total
}

// Projection is RFC section 16's "spend/day and projected month": the
// month-to-date total as of now, and a linear projection of that total
// across the rest of the current UTC calendar month (month-to-date /
// days elapsed so far * days in the month) -- the simplest defensible
// projection, and the one a fixture ledger with a known steady daily rate
// makes exactly reproducible in a golden test. This is a disclosed policy
// choice, like T2.21's demoteFactor: the RFC names the required
// output ("projected spend"), not a specific projection method.
type Projection struct {
	MonthToDateUSD float64
	ProjectedUSD   float64
	CeilingUSD     float64
	DaysElapsed    int
	DaysInMonth    int
}

// ProjectMonth computes rows' Projection as of now against ceiling.
// DaysElapsed always counts now's own day (a brain queried at any point
// during day N has N days of data to extrapolate from, never zero, so
// projection is always well-defined for a month with at least one day
// elapsed).
func ProjectMonth(rows []index.SpendRow, ceiling float64, now time.Time) Projection {
	mtd := MonthToDate(rows, now)
	daysElapsed := now.UTC().Day()
	daysInMonth := daysInMonthOf(now)
	var projected float64
	if daysElapsed > 0 {
		projected = mtd / float64(daysElapsed) * float64(daysInMonth)
	}
	return Projection{
		MonthToDateUSD: mtd,
		ProjectedUSD:   projected,
		CeilingUSD:     ceiling,
		DaysElapsed:    daysElapsed,
		DaysInMonth:    daysInMonth,
	}
}

func daysInMonthOf(t time.Time) int {
	t = t.UTC()
	firstOfNextMonth := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	return firstOfNextMonth.AddDate(0, 0, -1).Day()
}
