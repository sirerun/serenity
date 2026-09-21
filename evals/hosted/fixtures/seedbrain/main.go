// Command seedbrain writes a fixed list of entity fence pages into an
// already-initialized brain repo (`serenity init`), then exits without
// syncing. It exists so T23.43's Python eval harness can build a
// disposable, zero-LLM-cost brain for the lexical-only control arm using
// exactly the same primitives internal/cli's own test suite proves safe
// (see internal/cli/search_test.go, TestSearchCLIFindsEntityPageChunkAfterSync):
// store.NewEntityPage + store.NewFenceWriter + writer.Fence. No network
// call, no router, no LLM extraction -- literal fixture text becomes one
// committed entity page per input record, ready for `serenity sync` and
// `serenity search` to index and query over full-text only.
//
// Input is a JSON array on stdin (or via -input) of {slug, type, summary}
// records. Output is one JSON line per record on stdout reporting the
// written page path, so the harness can correlate a corpus fact id (used
// as slug) with its on-disk chunk ref without re-deriving the mapping.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

type record struct {
	Slug    string `json:"slug"`
	Type    string `json:"type"`
	Summary string `json:"summary"`
}

type writtenPage struct {
	Slug string `json:"slug"`
	Path string `json:"path"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "seedbrain:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("seedbrain", flag.ContinueOnError)
	root := fs.String("root", "", "brain repo root (must already exist via `serenity init`)")
	input := fs.String("input", "", "path to a JSON array of {slug,type,summary} records; defaults to stdin")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		return fmt.Errorf("seedbrain: -root is required")
	}

	var raw []byte
	var err error
	if *input == "" {
		raw, err = io.ReadAll(os.Stdin)
	} else {
		raw, err = os.ReadFile(*input)
	}
	if err != nil {
		return fmt.Errorf("seedbrain: read input: %w", err)
	}

	var records []record
	if err := json.Unmarshal(raw, &records); err != nil {
		return fmt.Errorf("seedbrain: decode input: %w", err)
	}
	if len(records) == 0 {
		return fmt.Errorf("seedbrain: input has zero records")
	}

	q := writer.NewQueue(nil)
	defer q.Close()
	fw := store.NewFenceWriter(*root)

	enc := json.NewEncoder(os.Stdout)
	for _, rec := range records {
		if rec.Slug == "" || rec.Type == "" || rec.Summary == "" {
			return fmt.Errorf("seedbrain: record missing slug/type/summary: %+v", rec)
		}
		p := store.NewEntityPage(domain.Entity{Type: rec.Type, Slug: rec.Slug})
		p.Summary = rec.Summary
		path, _, err := writer.Fence(q, fw, p)
		if err != nil {
			return fmt.Errorf("seedbrain: fence %s/%s: %w", rec.Type, rec.Slug, err)
		}
		if err := enc.Encode(writtenPage{Slug: rec.Slug, Path: path}); err != nil {
			return fmt.Errorf("seedbrain: encode result: %w", err)
		}
	}
	return nil
}
