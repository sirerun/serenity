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
// T5.10 appends rows like this one to a persistent trend file on a
// results branch; this package only produces a single run's row.
type Row struct {
	SchemaVersion int    `json:"schema_version"`
	Timestamp     string `json:"ts"`
	Commit        string `json:"commit,omitempty"`
	GbrainPin     string `json:"gbrain_pin"`
	Adapter       string `json:"adapter"`
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
