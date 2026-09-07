// Package briefing renders the daily briefing (RFC 0001 §7 preamble, plan
// T2.17): "regenerate entity summary fences...; refresh shard heads...;
// re-embed changed chunks; render the daily briefing -- fixed sections
// Blocked / Needs you / Moved forward / Watched / Drift, hard cap 800
// words, whole-section drop-not-truncate with stated omissions."
//
// RFC 0001 §12 describes a second, related packing job: DIRECTION v1's
// own `brief` wire object (T4.6, deps: [T2.17, ...]), with its own
// per-section caps (12/8/8/5) against a caller-supplied *token* budget
// rather than this package's fixed 800-*word* one. Pack (this file) is
// written pure and Estimator-parameterized specifically so T4.6 can call
// it unchanged against its own sections and a real token Estimator --
// "the packer is a pure function reused unchanged by E4's brief" (plan
// T2.17's own acc line) -- rather than reimplementing the
// drop-whole-section-not-item rule a second time. T4.6's own per-section
// item caps (12/8/8/5) are that caller's concern, applied while building
// its Sections, before calling Pack; Pack itself knows nothing about
// per-section caps, only the one shared budget.
package briefing

import (
	"fmt"
	"strings"
)

// SectionName is one of the five fixed section names RFC 0001 §7 names
// verbatim.
type SectionName string

const (
	SectionBlocked      SectionName = "Blocked"
	SectionNeedsYou     SectionName = "Needs you"
	SectionMovedForward SectionName = "Moved forward"
	SectionWatched      SectionName = "Watched"
	SectionDrift        SectionName = "Drift"
)

// Sections lists the five fixed sections in RFC 0001 §7's own priority
// order (the order the RFC prose itself lists them in). Compose (this
// package's only production caller so far) always builds exactly this
// set, in this order, for every render -- "five fixed sections" is the
// RFC's own wording, not a caller-configurable list.
var Sections = []SectionName{SectionBlocked, SectionNeedsYou, SectionMovedForward, SectionWatched, SectionDrift}

// Item is one line-item within a section. Text is pre-rendered by the
// caller (Compose's own section builders, or a future T4.6 DIRECTION v1
// section builder) -- Pack never formats domain data itself, only
// measures and arranges already-rendered text. That is what keeps Pack a
// pure, reusable function: it has no dependency on disposition/queue/
// spend/dira, only on []Section and an Estimator.
type Item struct {
	Text string
}

// Section is one named section's candidate items, already in the
// priority order within the section that Pack should preserve. Pack
// never reorders or truncates Items -- it either keeps a section's Items
// slice whole or drops it whole.
type Section struct {
	Name  SectionName
	Items []Item
}

// Estimator measures one item's "cost" against Pack's budget. Compose's
// own daily-briefing caller passes WordEstimator against DefaultWordBudget
// (RFC 0001 §7: "hard cap 800 words"); a future T4.6 DIRECTION v1 brief is
// expected to pass a real token-count Estimator against its own
// caller-supplied token budget (RFC 0001 §12: "Server-side packing
// against the caller's token budget") -- Pack's own logic is unchanged
// either way, only the Estimator and budget its caller supplies differ.
type Estimator func(text string) int

// WordEstimator counts whitespace-separated words -- RFC 0001 §7's own
// unit ("hard cap 800 words"). Compose's default Estimator.
func WordEstimator(text string) int {
	return len(strings.Fields(text))
}

// PackedSection is one section after Pack has decided whether it fits.
// A section that was dropped whole has Items == nil and Omitted equal to
// the section's original item count -- "a section over budget is dropped
// whole with omitted: N" (plan T2.17's own acc line). A section that fit
// has its original Items slice untouched and Omitted == 0. Pack never
// produces a partially-included section.
type PackedSection struct {
	Name    SectionName
	Items   []Item
	Omitted int
}

// Briefing is one Pack call's full result: the five sections, each
// either included whole or dropped whole, always in the same order Pack
// was given (RFC 0001 §7's fixed priority order) regardless of which
// sections survived.
type Briefing struct {
	Sections []PackedSection
}

// Pack is the pure packing function both RFC 0001 §7 (this package's own
// daily briefing, whole-section drop) and §12 (DIRECTION v1's brief, T4.6)
// need. Sections are evaluated strictly in the priority order given: a
// section is included whole when the total estimate of its own Items
// fits within whatever budget still remains after every earlier section
// that was included; otherwise it is dropped whole. Dropping one section
// does not consume the budget it would have used, so a later,
// lower-priority section can still fit and be included even after an
// earlier one was dropped -- this is deliberate: RFC 0001 §7's own
// "whole-section drop-not-truncate" rule says nothing about abandoning
// the remaining sections once one has been dropped, and §12's
// "server-side packing against the caller's token budget" for T4.6 needs
// exactly this same greedy-by-priority behavior over its own four
// sections.
//
// Pack does no I/O, calls no other package, and mutates nothing it is
// given -- every input is already-rendered text plus plain data. That is
// what lets a future T4.6 caller reuse it unchanged against its own
// DIRECTION v1 sections and a real token Estimator.
func Pack(sections []Section, budget int, estimate Estimator) Briefing {
	remaining := budget
	out := make([]PackedSection, 0, len(sections))
	for _, sec := range sections {
		cost := 0
		for _, it := range sec.Items {
			cost += estimate(it.Text)
		}
		if cost <= remaining {
			out = append(out, PackedSection{Name: sec.Name, Items: sec.Items})
			remaining -= cost
			continue
		}
		out = append(out, PackedSection{Name: sec.Name, Omitted: len(sec.Items)})
	}
	return Briefing{Sections: out}
}

// Render formats a Briefing as the plain-text daily-briefing output: one
// "## <name>" heading per section, in Pack's own (RFC-fixed) order,
// followed by one "- " line per surviving item, "(nothing)" for a
// section that was included but had no items, or "(omitted: N)" for a
// section Pack dropped whole.
func Render(b Briefing) string {
	var sb strings.Builder
	for i, sec := range b.Sections {
		if i > 0 {
			sb.WriteByte('\n')
		}
		fmt.Fprintf(&sb, "## %s\n", sec.Name)
		switch {
		case sec.Omitted > 0:
			fmt.Fprintf(&sb, "(omitted: %d)\n", sec.Omitted)
		case len(sec.Items) == 0:
			sb.WriteString("(nothing)\n")
		default:
			for _, it := range sec.Items {
				fmt.Fprintf(&sb, "- %s\n", it.Text)
			}
		}
	}
	return sb.String()
}
