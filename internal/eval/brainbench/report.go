package brainbench

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// RowSchemaVersion is Row's own format version -- bump it and add a
// migration note here if a field's meaning ever changes, mirroring
// result.schema.json's "additive-only within a version" contract upstream
// documents for its own result format.
const RowSchemaVersion = 1

// Row is one CI run's score row -- the shape written to
// evals/brainbench-trend.json (T1.21's acc line: "a per-run score row").
// T5.10 (evals/brainbench/publish_trend.go, trend.go) appends rows like
// this one to a persistent trend file on a results branch; this package
// only produces a single run's row.
//
// BudgetUSD and SpentUSD are T5.10's hard-cap fields, added additively
// (RowSchemaVersion unchanged -- see its own doc comment). SpentUSD is
// always 0 today: Evaluate's search call always passes a nil embedder
// (see its own doc comment), so this adapter makes zero live model calls
// structurally, not by configuration -- there is no code path that could
// spend anything. The fields are still recorded on every row, and
// CheckBudget in trend.go still runs the real comparison every publish,
// so a future change that wires a live embedder into this adapter trips
// the guard instead of silently overspending or silently publishing an
// under-reported number.
type Row struct {
	SchemaVersion int     `json:"schema_version"`
	Timestamp     string  `json:"ts"`
	Commit        string  `json:"commit,omitempty"`
	GbrainPin     string  `json:"gbrain_pin"`
	Adapter       string  `json:"adapter"`
	BudgetUSD     float64 `json:"budget_usd"`
	SpentUSD      float64 `json:"spent_usd"`
	Report
}

// adapterName documents, in the artifact itself, exactly what was scored
// -- see the package doc for the full disclosure.
const adapterName = "serenity-search-fts-only-v1"

// NewRow wraps report into a Row with the given commit and gbrain pin,
// timestamped now.
func NewRow(report Report, commit, gbrainPin string) Row {
	return Row{
		SchemaVersion: RowSchemaVersion,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Commit:        commit,
		GbrainPin:     gbrainPin,
		Adapter:       adapterName,
		Report:        report,
	}
}

// WriteRow marshals row as indented JSON with a trailing newline and
// writes it to path.
func WriteRow(path string, row Row) error {
	b, err := json.MarshalIndent(row, "", "  ")
	if err != nil {
		return fmt.Errorf("brainbench: marshal row: %w", err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("brainbench: write %s: %w", path, err)
	}
	return nil
}
