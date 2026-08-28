package brainbench

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/search"
)

// counts is the running confusion counts this package accumulates per
// query, then per category and overall -- the same micro-averaging style
// internal/eval/score.go uses (sum raw counts, derive one precision/
// recall/F1 at the end) rather than averaging per-query ratios.
//
// Precision and recall are scored against deliberately different
// numerators, per the BrainBench README's own rule ("injected = fine,
// missed = no penalty" for acceptable_slugs): an acceptable-slug hit
// counts toward precision (it was a reasonable thing to retrieve) but
// never toward recall (it does not satisfy the requirement a gold_slug
// represents). Conflating the two would let a retrieved acceptable_slug
// mask a missed required gold_slug's effect on recall.
//
//	PrecisionHits = |retrieved ∩ (gold_slugs ∪ acceptable_slugs)|
//	FalsePositives = |retrieved \ (gold_slugs ∪ acceptable_slugs)|
//	RecallHits     = |gold_slugs ∩ retrieved|
//	Missed         = |gold_slugs \ retrieved|
type counts struct {
	PrecisionHits, FalsePositives int
	RecallHits, Missed            int
	Queries                       int
}

func (c *counts) add(o counts) {
	c.PrecisionHits += o.PrecisionHits
	c.FalsePositives += o.FalsePositives
	c.RecallHits += o.RecallHits
	c.Missed += o.Missed
	c.Queries += o.Queries
}

// CategoryMetrics is one category's (or the overall run's) scored result.
type CategoryMetrics struct {
	Queries        int     `json:"queries"`
	PrecisionHits  int     `json:"precision_hits"`
	FalsePositives int     `json:"false_positives"`
	RecallHits     int     `json:"recall_hits"`
	Missed         int     `json:"missed"`
	Precision      float64 `json:"precision"`
	Recall         float64 `json:"recall"`
	F1             float64 `json:"f1"`
}

func (c counts) metrics() CategoryMetrics {
	m := CategoryMetrics{
		Queries: c.Queries, PrecisionHits: c.PrecisionHits,
		FalsePositives: c.FalsePositives, RecallHits: c.RecallHits, Missed: c.Missed,
	}
	if c.PrecisionHits+c.FalsePositives > 0 {
		m.Precision = float64(c.PrecisionHits) / float64(c.PrecisionHits+c.FalsePositives)
	}
	if c.RecallHits+c.Missed > 0 {
		m.Recall = float64(c.RecallHits) / float64(c.RecallHits+c.Missed)
	}
	if m.Precision+m.Recall > 0 {
		m.F1 = 2 * m.Precision * m.Recall / (m.Precision + m.Recall)
	}
	return m
}

// SkippedFixture records why a fixture contributed zero scored queries --
// never silently dropped.
type SkippedFixture struct {
	FixtureID string `json:"fixture_id"`
	Reason    string `json:"reason"`
}

// Report is Evaluate's result.
type Report struct {
	Limit           int                        `json:"limit"`
	FixturesTotal   int                        `json:"fixtures_total"`
	FixturesScored  int                        `json:"fixtures_scored"`
	FixturesSkipped []SkippedFixture           `json:"fixtures_skipped"`
	Overall         CategoryMetrics            `json:"overall"`
	ByCategory      map[string]CategoryMetrics `json:"by_category"`
}

// Evaluate runs every should_retrieve:true gold turn in fixtures through
// Serenity's real hybrid search (internal/search.Search, T1.11) against a
// fresh, fixture-scoped index built from that fixture's own seed_pages,
// and scores the retrieved entity slugs against BrainBench's sealed gold.
// See the package doc for the scope this represents.
//
// The embedder passed to search.Search is always nil -- T1.11's honest
// FTS-only degraded mode -- so this makes zero live model calls,
// satisfying the "on cached outputs" requirement structurally rather than
// by configuration.
//
// A fixture is skipped (recorded in Report.FixturesSkipped, never silently
// dropped) when: it has no gold file; it has no seed_pages (write-back and
// continuity-reader fixtures seed their content through a write-back
// pipeline this adapter does not have, T1.9); or none of its gold turns
// are should_retrieve:true (write-back fixtures' own turns are all
// should_retrieve:false -- the benchmark is asking a different question
// there than retrieval quality).
//
// Multi-source fixtures (ms-*) are scored on the union of all seed_pages
// regardless of source_id -- this adapter does not model per-source
// visibility (no "active brain" concept exists in Serenity's search
// surface yet), so a should_retrieve:false turn whose point is cross-
// source suppression is skipped at the per-turn level rather than
// silently scored as a pass.
func Evaluate(ctx context.Context, fixtures []Fixture, gold map[string]Gold, limit int) (Report, error) {
	report := Report{
		Limit:         limit,
		FixturesTotal: len(fixtures),
		ByCategory:    map[string]CategoryMetrics{},
	}
	categoryCounts := map[string]*counts{}
	var overall counts

	for _, f := range fixtures {
		g, ok := gold[f.FixtureID]
		if !ok {
			report.FixturesSkipped = append(report.FixturesSkipped, SkippedFixture{f.FixtureID, "no gold file"})
			continue
		}
		if len(f.SeedPages) == 0 {
			report.FixturesSkipped = append(report.FixturesSkipped, SkippedFixture{
				f.FixtureID, "no seed_pages (requires a write-back pipeline not yet wired, T1.9)",
			})
			continue
		}

		scoreable := make([]Turn, 0, len(f.Turns))
		for _, t := range f.Turns {
			gt, ok := g.Turns[strconv.Itoa(t.TurnID)]
			if ok && gt.ShouldRetrieve {
				scoreable = append(scoreable, t)
			}
		}
		if len(scoreable) == 0 {
			report.FixturesSkipped = append(report.FixturesSkipped, SkippedFixture{f.FixtureID, "no should_retrieve:true gold turns"})
			continue
		}

		fc, err := evaluateFixture(ctx, f, g, scoreable, limit)
		if err != nil {
			return Report{}, fmt.Errorf("brainbench: evaluate fixture %s: %w", f.FixtureID, err)
		}
		overall.add(fc)
		cc := categoryCounts[f.Category]
		if cc == nil {
			cc = &counts{}
			categoryCounts[f.Category] = cc
		}
		cc.add(fc)
		report.FixturesScored++
	}

	report.Overall = overall.metrics()
	for cat, cc := range categoryCounts {
		report.ByCategory[cat] = cc.metrics()
	}
	return report, nil
}

// evaluateFixture builds a fresh on-disk index from f's seed pages, runs
// each scoreable turn's text through search.Search, and returns the raw
// TP/FP/FN this fixture contributed.
func evaluateFixture(ctx context.Context, f Fixture, g Gold, scoreable []Turn, limit int) (counts, error) {
	dir, err := os.MkdirTemp("", "brainbench-"+f.FixtureID+"-")
	if err != nil {
		return counts{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	eng, err := index.Open(filepath.Join(dir, "index.db"))
	if err != nil {
		return counts{}, err
	}
	defer func() { _ = eng.Close() }()

	for i, page := range f.SeedPages {
		chunkRef := fmt.Sprintf("%s#%d", f.FixtureID, i)
		if err := eng.InsertChunk(ctx, chunkRef, page.Slug, page.Content, f.FixtureID, "brainbench_seed_page"); err != nil {
			return counts{}, fmt.Errorf("index seed page %s: %w", page.Slug, err)
		}
	}

	var fc counts
	for _, t := range scoreable {
		gt := g.Turns[strconv.Itoa(t.TurnID)]
		var results []search.Result
		if q := ftsQuery(t.Text); q != "" {
			var err error
			results, err = search.Search(ctx, eng, nil, q, limit, search.Options{})
			if err != nil {
				return counts{}, fmt.Errorf("search turn %d: %w", t.TurnID, err)
			}
		}

		relevant := toSet(gt.GoldSlugs)
		for _, s := range gt.AcceptableSlugs {
			relevant[s] = true
		}
		retrieved := make(map[string]bool, len(results))
		for _, r := range results {
			retrieved[r.EntitySlug] = true
		}

		for slug := range retrieved {
			if relevant[slug] {
				fc.PrecisionHits++
			} else {
				fc.FalsePositives++
			}
		}
		for _, slug := range gt.GoldSlugs {
			if retrieved[slug] {
				fc.RecallHits++
			} else {
				fc.Missed++
			}
		}
		fc.Queries++
	}
	return fc, nil
}

// ftsQuery turns a free-text conversation turn into a query safe for
// internal/index's FTS5 MATCH column.
//
// Disclosed, out-of-scope-for-this-task defect this exists to work around:
// internal/cli/search.go and internal/search.Search (T1.11) pass a caller's
// query straight into `chunks MATCH ?` with no escaping. BrainBench turns
// are full conversational sentences ("What did Alice Example say about the
// Widget Co deal?"), and FTS5's query grammar treats a bareword "?" as a
// syntax token, not literal text -- passing the turn verbatim makes
// SearchFTS return a SQL logic error on almost every question-shaped turn
// (confirmed empirically), not merely a poor ranking. The same crash hits
// any real `serenity search` invocation today whose query contains a FTS5
// syntax character (?, ", (, ), :, *, -, or a bareword AND/OR/NOT/NEAR) --
// see docs/lore.md. Fixing SearchFTS itself is out of this task's scope
// (internal/index and internal/search are T1.11's already-merged surface,
// concurrently touched by T1.15 this wave); this adapter instead builds
// its own safe query locally, same as any other caller would need to
// until that hardening lands.
//
// The transform: split on non-alphanumeric runes, double-quote each token
// (FTS5's documented technique for matching a token as literal text,
// neutralizing any special meaning), and OR them together so bm25 ranks
// chunks by how many of the turn's words they share -- an implicit AND of
// every token (including stopwords like "the"/"about") would fail to
// match almost any paraphrased page, since BrainBench's gold pages never
// echo a turn's exact wording.
func ftsQuery(text string) string {
	var tokens []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			tokens = append(tokens, cur.String())
			cur.Reset()
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	if len(tokens) == 0 {
		return ""
	}
	quoted := make([]string, len(tokens))
	for i, tok := range tokens {
		quoted[i] = `"` + strings.ReplaceAll(tok, `"`, `""`) + `"`
	}
	return strings.Join(quoted, " OR ")
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
