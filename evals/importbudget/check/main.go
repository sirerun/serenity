// Command check validates a measured ingest artifact and its regression budget.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	budget "github.com/sirerun/serenity/internal/eval/importbudget"
)

func readReport(path string) (budget.Report, error) {
	var report budget.Report
	f, err := os.Open(path)
	if err != nil {
		return report, err
	}
	defer func() { _ = f.Close() }()
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return report, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return report, fmt.Errorf("trailing report data")
	}
	return report, budget.Validate(report)
}
func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("import-budget-check", flag.ContinueOnError)
	currentPath := flags.String("current", "", "current measured JSON report")
	previousPath := flags.String("previous", "", "previous report (omit only for first-run bootstrap)")
	markdownPath := flags.String("markdown", "", "write per-stage timing table")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *currentPath == "" || flags.NArg() != 0 {
		return fmt.Errorf("-current is required; positional arguments are not accepted")
	}
	current, err := readReport(*currentPath)
	if err != nil {
		return fmt.Errorf("current report: %w", err)
	}
	if current.Messages != 10000 {
		return fmt.Errorf("release benchmark requires exactly 10000 messages, got %d", current.Messages)
	}
	var previous *budget.Report
	if *previousPath != "" {
		r, err := readReport(*previousPath)
		if err != nil {
			return fmt.Errorf("previous report: %w", err)
		}
		previous = &r
	}
	var table strings.Builder
	fmt.Fprint(&table, "# Synthetic cached-model import timing\n\n")
	fmt.Fprintf(&table, "Messages: %d. Claims: %d. Vectors: %d. Cache hits: %d. Live model calls: %d.\n\n", current.Messages, current.Claims, current.Vectors, current.CacheHits, current.ModelCalls)
	fmt.Fprintln(&table, "| Stage | Items | Seconds |\n| --- | ---: | ---: |")
	for _, stage := range current.Stages {
		fmt.Fprintf(&table, "| %s | %d | %.3f |\n", stage.Name, stage.Items, stage.Seconds)
	}
	fmt.Fprintf(&table, "| **Measured total** | | **%.3f** |\n\n", current.TotalSeconds)
	if previous == nil {
		fmt.Fprintln(&table, "First recorded run: regression comparison is not available; the four-hour ceiling is enforced.")
	} else {
		fmt.Fprintf(&table, "Previous run: %.3f seconds. Change: %+.2f%%.\n", previous.TotalSeconds, 100*(current.TotalSeconds/previous.TotalSeconds-1))
	}
	fmt.Fprintln(&table, "\nCorpus generation, synthetic mailbox loading, and cache preparation are excluded. These fixture outputs measure pipeline overhead; they do not measure live-model quality or network latency.")
	if *markdownPath != "" {
		if err := os.WriteFile(*markdownPath, []byte(table.String()), 0o600); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(out, table.String()); err != nil {
		return err
	}
	return budget.Check(current, previous)
}
func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
