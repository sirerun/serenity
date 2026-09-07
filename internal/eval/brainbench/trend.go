package brainbench

import (
	"encoding/json"
	"fmt"
	"os"
)

// This file is plan T5.10's own half of the BrainBench trend artifact:
// T1.21 (report.go, gen_trend.go) produces one run's Row on cached
// outputs (see Row's own doc comment); this file persists rows like it
// across runs on a durable file (evals/brainbench/publish_trend.go checks
// that file out on a results branch, never on main -- see that command's
// doc comment) and guards the two things a nightly, unattended job must
// never do silently: publish a row that measured nothing real (a "cache
// miss" -- the vendored corpus produced zero scoreable fixtures), and
// spend past a hard cap instead of alerting.

// LoadTrend reads path as a JSON array of Row and returns it. A missing
// file returns an empty, non-nil slice and no error -- the first-ever
// publish on a fresh results branch has nothing to load yet, and that is
// not a failure.
func LoadTrend(path string) ([]Row, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Row{}, nil
		}
		return nil, fmt.Errorf("brainbench: read trend %s: %w", path, err)
	}
	var rows []Row
	if err := json.Unmarshal(b, &rows); err != nil {
		return nil, fmt.Errorf("brainbench: parse trend %s: %w", path, err)
	}
	return rows, nil
}

// AppendRow loads the existing trend at path (see LoadTrend), appends
// row, writes the result back as indented JSON with a trailing newline,
// and returns the full updated slice -- never truncated, never
// reordered: a durable history is only ever grown, matching this
// repo's append-only convention for other durable ledgers (e.g.
// internal/direction's writer-queue store).
func AppendRow(path string, row Row) ([]Row, error) {
	rows, err := LoadTrend(path)
	if err != nil {
		return nil, err
	}
	rows = append(rows, row)

	b, err := json.MarshalIndent(rows, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("brainbench: marshal trend: %w", err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return nil, fmt.Errorf("brainbench: write trend %s: %w", path, err)
	}
	return rows, nil
}

// CheckCacheHit is the nightly job's cache-miss alert: FixturesScored==0
// means the vendored corpus produced not one scoreable gold turn (an
// empty/missing fixtures or gold dir, a schema mismatch, every fixture
// skipped) -- publishing a row in that state would silently record a
// meaningless 0/0/0 point as if it were a real measurement, corrupting
// the trend rather than merely missing an update. Returning an error
// here is the entire "alert instead of re-billing" contract for this
// adapter: there is no live/paid fallback path this package could fall
// back to (Evaluate always makes zero live model calls, see its own doc
// comment) -- the only two possible outcomes on a cache miss are publish
// a false point, or refuse and alert. This picks the latter.
func CheckCacheHit(report Report) error {
	if report.FixturesScored == 0 {
		return fmt.Errorf("brainbench: cache miss -- 0/%d fixtures scored, refusing to publish a misleading trend row (see Report.FixturesSkipped for why)", report.FixturesTotal)
	}
	return nil
}

// CheckBudget is the nightly job's hard-USD-cap guard. budgetUSD<=0 means
// unlimited (never trips). It is a plain, direct comparison rather than a
// running ledger (contrast internal/eval/runner.TrackingLedger, built for
// a mode that issues many metered calls across one run) because this
// adapter has exactly one number to check: the whole run's total spend,
// which is always 0 today (see Row's doc comment) -- the mechanism is
// still real and independently tested (trend_test.go exercises it with a
// synthetic over-budget spend) so it fails loudly the moment that ever
// stops being true, instead of silently letting a run spend past its cap.
func CheckBudget(budgetUSD, spentUSD float64) error {
	if budgetUSD > 0 && spentUSD > budgetUSD {
		return fmt.Errorf("brainbench: spend $%.4f exceeds hard cap $%.4f -- refusing to publish", spentUSD, budgetUSD)
	}
	return nil
}
