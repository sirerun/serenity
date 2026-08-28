// Package compose implements the synthesis half of RFC 0001 section 11:
// `serenity ask` composes a cited natural-language answer from the
// claims a brain repo has actually accumulated. Three behaviors are
// structural, not model-trusted:
//
//   - Every citation the answer reports resolves to a claim this call
//     actually retrieved -- extractCitations whitelist-filters the
//     model's response against the retrieved candidate set, so a
//     model-invented claim id is dropped, never surfaced (the model is
//     never asked to be honest about this; the code enforces it).
//   - A cited claim that replaced an earlier one always carries its full
//     supersession chain (RFC's "believed X until June, now Y"), built
//     deterministically from domain.Claim.Supersedes/SupersededBy --
//     independent of whether the model's prose happens to mention it.
//   - A question with no matching claim always returns a non-empty gap
//     statement naming how stale the brain's newest evidence is, instead
//     of a fabricated answer or a silent empty result.
//
// Retrieval has two layers, mirroring internal/search's own division of
// labor. internal/search.Search (T1.11) ranks raw source/entity-page TEXT
// chunks, not individual claims -- the derived index never indexes claims
// for point lookup (T1.9's write path is append-only, trust-0; semantic
// dedup and claim search are E2 work). So Ask walks the canonical claim
// stores directly (store.FenceWriter, store.ShardStore -- the same
// fence-vs-shard authority split internal/index.Rebuild applies) to build
// the citable claim set, and uses chunk search only to widen which
// subjects are relevant to the query beyond plain keyword overlap on the
// claims themselves.
package compose

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/router"
	"github.com/sirerun/serenity/internal/search"
	"github.com/sirerun/serenity/internal/store"
)

// Completer mirrors internal/extract.Completer: the subset of
// *router.Router this package calls through. Production callers always
// pass a real *router.Router; tests build one with a fake router.Provider
// (the pattern internal/router/router_test.go uses) so the router's own
// tier resolution, confidence cap, and spend-ledger recording run for
// real -- never fake this interface directly to hand-script a canned
// answer.
type Completer interface {
	Complete(ctx context.Context, tc router.TaskClass, p router.Prompt, b router.Budget) (router.Result, error)
}

// retrievalLimit caps how many candidate claims one synthesis call is
// shown -- RFC §12's brief-packing precedent (drop whole items, never
// truncate mid-item) for the same reason: a bounded, deterministic prompt
// size rather than an unbounded one that grows with the brain.
const retrievalLimit = 8

// searchPoolMultiplier widens the chunk-search request beyond
// retrievalLimit so enough candidate subjects surface before the claim
// side narrows them back down.
const searchPoolMultiplier = 4

// Citation is one claim the answer actually used to ground a fact.
type Citation struct {
	ClaimID    string
	Subject    string
	Predicate  string
	Object     string
	SourceRef  string
	Confidence float64
	ObservedAt time.Time
	// ValidTo is the claim's own validity-window end (§7.2), when known --
	// renderSupersession's "believed X until <date>" phrasing (RFC §11)
	// means the date X stopped holding, which is exactly what ValidTo
	// records; ObservedAt (when the claim was recorded) is only a
	// fallback for claims the extraction pipeline hasn't populated a
	// window for yet (T1.8/T1.9 do not set it as of this writing).
	ValidTo string
}

// Supersession is one cited fact's full lifecycle, oldest first, ending
// at the currently-live claim.
type Supersession struct {
	Subject   string
	Predicate string
	Chain     []Citation
}

// Answer is what Ask returns: RFC §11's cited answer plus an explicit gap
// statement. Text and Gap are mutually exclusive -- exactly one is
// non-empty.
type Answer struct {
	// Text is the synthesized prose, with every cited claim's
	// supersession history (if any) appended. Empty exactly when Gap is
	// set.
	Text string
	// Citations lists every claim tag the answer's own text actually
	// used. Every entry is guaranteed, structurally, to be a claim this
	// call retrieved -- see extractCitations.
	Citations []Citation
	// Supersessions holds the full history for any retrieved claim that
	// replaced an earlier one, whether or not the model's prose cited it
	// by tag -- the history is surfaced because the claim was judged
	// relevant enough to show the model, not only when the model quotes
	// it back.
	Supersessions []Supersession
	// Gap is non-empty exactly when no claim in the brain answers the
	// question -- names the newest evidence's age (RFC §11).
	Gap string
}

// Composer answers questions over a brain repo's accumulated claims.
// Construct with New; the zero value is not usable (Root/Config/Store/
// Router are required).
type Composer struct {
	Root         string
	Config       *config.Config
	Store        search.Store   // chunk-level retrieval (T1.11); *index.SQLite satisfies this
	Embedder     embed.Embedder // nil degrades to FTS-only, matching runSearch's precedent
	Router       Completer
	ModelVersion string

	now func() time.Time
}

// New builds a Composer. root is the brain repo root; st is the derived
// index (*index.SQLite satisfies search.Store); embedder may be nil.
func New(root string, cfg *config.Config, st search.Store, embedder embed.Embedder, r Completer, modelVersion string) *Composer {
	return &Composer{
		Root:         root,
		Config:       cfg,
		Store:        st,
		Embedder:     embedder,
		Router:       r,
		ModelVersion: modelVersion,
		now:          time.Now,
	}
}

// Ask composes a cited answer to query from the brain's current claims.
func (c *Composer) Ask(ctx context.Context, query string) (Answer, error) {
	bySubject, err := AllClaims(c.Root, c.Config)
	if err != nil {
		return Answer{}, fmt.Errorf("compose: %w", err)
	}

	live := resolveLive(groupBySubjectFamily(bySubject))

	candidates, err := c.relevant(ctx, query, live)
	if err != nil {
		return Answer{}, err
	}
	if len(candidates) == 0 {
		return c.gapAnswer(bySubject), nil
	}
	if len(candidates) > retrievalLimit {
		candidates = candidates[:retrievalLimit]
	}

	indexOnly, err := c.anySourceIndexOnly(candidates)
	if err != nil {
		return Answer{}, err
	}

	result, err := c.Router.Complete(ctx, router.TaskClassComposerSynthesis,
		router.Prompt{Text: buildPrompt(query, candidates), IndexOnly: indexOnly}, router.Budget{})
	if err != nil {
		return Answer{}, fmt.Errorf("compose: synthesis: %w", err)
	}

	citations := extractCitations(result.Text, candidates)
	supersessions := supersessionsFor(candidates)

	text := sanitizeText(result.Text, candidates)
	for _, s := range supersessions {
		text += "\n\n" + renderSupersession(s)
	}

	return Answer{Text: text, Citations: citations, Supersessions: supersessions}, nil
}

// AllClaims walks every entity page and shard file under root and
// returns every claim -- current and historical -- keyed by subject
// slug. This mirrors internal/index.Rebuild's fence-vs-shard authority
// split exactly (fence rows for shard-tier families are stale derived
// heads, skipped in favor of the shard's own lines) but, unlike Rebuild,
// keeps full shard history (ShardStore.Lines) rather than only resolved
// heads, because supersession-chain phrasing needs the superseded lines
// too.
func AllClaims(root string, cfg *config.Config) (map[string][]domain.Claim, error) {
	out := map[string][]domain.Claim{}

	pages, err := filepath.Glob(filepath.Join(root, "brain", "entities", "*", "*.md"))
	if err != nil {
		return nil, fmt.Errorf("compose: glob entity pages: %w", err)
	}
	sort.Strings(pages)
	fw := store.NewFenceWriter(root)
	for _, path := range pages {
		p, err := fw.ParseEntity(path)
		if err != nil {
			return nil, fmt.Errorf("compose: parse %s: %w", path, err)
		}
		for _, cl := range p.Claims {
			if cfg.TierOf(cl.Family) == domain.TierShard {
				continue // the shard below is canonical for this family
			}
			out[p.Entity.Slug] = append(out[p.Entity.Slug], cl)
		}
	}

	ss := store.NewShardStore(root)
	slugs, err := ss.Slugs()
	if err != nil {
		return nil, fmt.Errorf("compose: shard slugs: %w", err)
	}
	for _, slug := range slugs {
		families, err := ss.Families(slug)
		if err != nil {
			return nil, fmt.Errorf("compose: shard families for %s: %w", slug, err)
		}
		for _, family := range families {
			lines, err := ss.Lines(slug, family)
			if err != nil {
				return nil, fmt.Errorf("compose: shard lines %s/%s: %w", slug, family, err)
			}
			out[slug] = append(out[slug], lines...)
		}
	}
	return out, nil
}

// groupKey scopes claim resolution to one (subject, family) at a time --
// the same scope index.Rebuild resolves shard heads within, so an
// ObjectKey never collides across two unrelated predicate families for
// the same subject.
type groupKey struct{ Subject, Family string }

func groupBySubjectFamily(bySubject map[string][]domain.Claim) map[groupKey][]domain.Claim {
	out := map[groupKey][]domain.Claim{}
	for slug, claims := range bySubject {
		for _, cl := range claims {
			k := groupKey{slug, cl.Family}
			out[k] = append(out[k], cl)
		}
	}
	return out
}

// liveClaim is one currently-resolved claim plus its full ancestor chain
// (oldest first), if any.
type liveClaim struct {
	domain.Claim
	History []domain.Claim
}

// resolveLive computes, for every (subject, family) group, the live
// heads via store.ResolveHeadLines -- the exact liveness rule
// ShardStore.ResolveHeads applies -- and attaches each head's ancestor
// chain. This works uniformly over fence and shard claims because
// ResolveHeadLines is a pure function of the Supersedes/State fields
// alone, regardless of storage tier: a fence-tier ancestor's own State is
// already StateSuperseded when FenceWriter round-trips it, while a
// shard-tier ancestor's stored State typically stays "active" forever
// (shards are append-only) and is instead excluded via the newer line's
// Supersedes pointer -- ResolveHeadLines accounts for both.
func resolveLive(groups map[groupKey][]domain.Claim) []liveClaim {
	var out []liveClaim
	for _, group := range groups {
		heads := store.ResolveHeadLines(group)
		byID := make(map[string]domain.Claim, len(group))
		bySupersededBy := make(map[string]domain.Claim, len(group))
		for _, cl := range group {
			byID[cl.ID] = cl
			if cl.SupersededBy != "" {
				bySupersededBy[cl.SupersededBy] = cl
			}
		}
		for _, key := range store.HeadKeys(heads) {
			h := heads[key]
			out = append(out, liveClaim{Claim: h, History: ancestorsOf(h, byID, bySupersededBy)})
		}
	}
	// Deterministic order: prompt content -- and hence a real model's
	// response -- must never depend on Go's randomized map iteration.
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.SubjectSlug != b.SubjectSlug {
			return a.SubjectSlug < b.SubjectSlug
		}
		if a.Family != b.Family {
			return a.Family < b.Family
		}
		return a.ID < b.ID
	})
	return out
}

// ancestorsOf walks c's ancestor chain back through byID/bySupersededBy
// (the group c came from) and returns the ancestors oldest first,
// excluding c itself. The two storage tiers carry the supersession link
// in different, non-overlapping directions once round-tripped through
// disk: a shard-tier claim's own Supersedes field survives ShardStore's
// lossless JSONL round-trip, so the newest line points backward at what
// it replaced -- but a fence-tier claim's Supersedes field does NOT
// survive FenceWriter.RenderEntity/ParseEntity's table round-trip (the
// rendered table has no column for it, store/fence.go's claimsHeader);
// only the superseded row's own SupersededBy forward pointer does
// (encoded in the state cell as "superseded→<id>"). This walks whichever
// direction the group actually carries, backward-first, so either tier's
// convention resolves the same chain. A dangling pointer (the referenced
// id is not in this group) stops the walk rather than erroring -- the
// chain built so far is still correct history, just possibly incomplete.
func ancestorsOf(c domain.Claim, byID, bySupersededBy map[string]domain.Claim) []domain.Claim {
	var chain []domain.Claim
	cur := c
	for {
		var prev domain.Claim
		var ok bool
		if cur.Supersedes != "" {
			prev, ok = byID[cur.Supersedes]
		}
		if !ok {
			prev, ok = bySupersededBy[cur.ID]
		}
		if !ok {
			break
		}
		chain = append([]domain.Claim{prev}, chain...)
		cur = prev
	}
	return chain
}

// relevant ranks live claims against query: chunk-level search widens the
// set of relevant subjects (relevantSubjects), and a direct lexical
// overlap against each claim's own subject/predicate/object text catches
// what chunk search cannot see (a source chunk carries no entity slug for
// most connectors; an entity-page chunk carries only title+summary, not
// each individual claim's predicate/object). A claim matches if either
// signal fires; ranking favors claims that fire on both.
func (c *Composer) relevant(ctx context.Context, query string, live []liveClaim) ([]liveClaim, error) {
	relevantSubjects, err := c.relevantSubjects(ctx, query, live)
	if err != nil {
		return nil, err
	}

	qTokens := tokenize(query)
	type scored struct {
		lc    liveClaim
		score int
	}
	var matched []scored
	for _, lc := range live {
		score := lexicalScore(qTokens, lc.Claim)
		subjectHit := relevantSubjects[lc.SubjectSlug]
		if score == 0 && !subjectHit {
			continue
		}
		if subjectHit {
			score++
		}
		matched = append(matched, scored{lc, score})
	}
	sort.SliceStable(matched, func(i, j int) bool {
		if matched[i].score != matched[j].score {
			return matched[i].score > matched[j].score
		}
		return matched[i].lc.ID < matched[j].lc.ID
	})
	out := make([]liveClaim, len(matched))
	for i, m := range matched {
		out[i] = m.lc
	}
	return out, nil
}

// relevantSubjects runs internal/search.Search (chunk-level, T1.11) and
// maps its hits back to subject slugs: an entity-page hit names its slug
// directly, and a raw-source-chunk hit's SourceSHA256 is mapped back to
// every live claim whose provenance cites that source.
func (c *Composer) relevantSubjects(ctx context.Context, query string, live []liveClaim) (map[string]bool, error) {
	limit := retrievalLimit * searchPoolMultiplier
	hits, err := search.Search(ctx, c.Store, c.Embedder, query, limit, search.Options{})
	if err != nil {
		return nil, fmt.Errorf("compose: chunk search: %w", err)
	}

	bySource := map[string][]string{}
	for _, lc := range live {
		if sha := lc.Provenance.SourceSHA256; sha != "" {
			bySource[sha] = appendUnique(bySource[sha], lc.SubjectSlug)
		}
	}

	out := map[string]bool{}
	for _, h := range hits {
		if h.EntitySlug != "" {
			out[h.EntitySlug] = true
		}
		for _, slug := range bySource[h.SourceSHA256] {
			out[slug] = true
		}
	}
	return out, nil
}

func appendUnique(list []string, v string) []string {
	for _, existing := range list {
		if existing == v {
			return list
		}
	}
	return append(list, v)
}

var tokenSplit = regexp.MustCompile(`[^a-z0-9]+`)

// stopWords are dropped from query tokenization so common question
// scaffolding ("what is X's role") never accidentally matches an
// unrelated claim by coincidence.
var stopWords = map[string]bool{
	"a": true, "an": true, "and": true, "at": true, "do": true, "does": true,
	"for": true, "how": true, "in": true, "is": true, "of": true, "on": true,
	"the": true, "to": true, "what": true, "when": true, "where": true,
	"who": true, "why": true,
}

func tokenize(s string) map[string]bool {
	out := map[string]bool{}
	for _, tok := range tokenSplit.Split(strings.ToLower(s), -1) {
		if tok == "" || stopWords[tok] {
			continue
		}
		out[tok] = true
	}
	return out
}

// lexicalScore counts how many of a claim's own subject-slug/predicate/
// object tokens appear in the query's token set.
func lexicalScore(qTokens map[string]bool, c domain.Claim) int {
	score := 0
	for tok := range tokenize(c.SubjectSlug) {
		if qTokens[tok] {
			score++
		}
	}
	for tok := range tokenize(c.Predicate) {
		if qTokens[tok] {
			score++
		}
	}
	for tok := range tokenize(c.Object) {
		if qTokens[tok] {
			score++
		}
	}
	return score
}

// anySourceIndexOnly reports whether any candidate claim's provenance
// cites a source marked index_only (RFC §7.4, §14): such bytes must never
// leave the machine, and a claim's own predicate/object text can restate
// index_only content even though domain.Claim carries no IndexOnly flag
// of its own -- so this checks back against the source store before every
// synthesis call, the same rule router.Complete already enforces for
// extraction prompts (T1.7), applied here at the claim layer since no
// upstream stage tags claims with it yet.
func (c *Composer) anySourceIndexOnly(candidates []liveClaim) (bool, error) {
	ss := store.NewSourceStore(c.Root)
	sources, err := ss.All()
	if err != nil {
		return false, fmt.Errorf("compose: read sources: %w", err)
	}
	indexOnly := make(map[string]bool, len(sources))
	for _, src := range sources {
		if src.IndexOnly {
			indexOnly[src.SHA256] = true
		}
	}
	for _, lc := range candidates {
		if indexOnly[lc.Provenance.SourceSHA256] {
			return true, nil
		}
	}
	return false, nil
}

// buildPrompt renders the structured synthesis prompt: every candidate
// claim tagged with a citation bracket the model must reuse verbatim.
// The model is instructed never to invent a tag, but that instruction is
// advisory only -- extractCitations is the actual enforcement.
func buildPrompt(query string, candidates []liveClaim) string {
	var b strings.Builder
	b.WriteString("You are Serenity's composer (RFC 0001 section 11). Answer the question using ONLY the claims listed below -- never state a fact that is not one of them.\n")
	b.WriteString("Cite every fact with its exact bracket tag shown before the claim, for example [claim:1a2b3c4d]. Never write a tag that is not listed below.\n")
	b.WriteString("If the claims below do not answer the question, say so plainly instead of guessing.\n\n")
	fmt.Fprintf(&b, "Question: %s\n\nClaims:\n", query)
	for _, lc := range candidates {
		fmt.Fprintf(&b, "[claim:%s] %s %s %s (confidence %.2f, observed %s)\n",
			lc.ID, lc.SubjectSlug, lc.Predicate, lc.Object, lc.Confidence, formatObservedAt(lc.Provenance.ObservedAt))
	}
	b.WriteString("\nAnswer:")
	return b.String()
}

func formatObservedAt(t time.Time) string {
	if t.IsZero() {
		return "unknown date"
	}
	return t.Format("2006-01-02")
}

var citationTag = regexp.MustCompile(`\[claim:([A-Za-z0-9_-]+)\]`)

// extractCitations scans text for citation tags and keeps only the ones
// that name a claim actually present in candidates -- the structural
// guard against a hallucinated citation: a tag the model invented, or
// copied wrong, is silently dropped here rather than trusted and
// surfaced. This is the sole authority for Answer.Citations; nothing else
// populates it.
func extractCitations(text string, candidates []liveClaim) []Citation {
	byID := make(map[string]liveClaim, len(candidates))
	for _, lc := range candidates {
		byID[lc.ID] = lc
	}

	seen := map[string]bool{}
	var out []Citation
	for _, m := range citationTag.FindAllStringSubmatch(text, -1) {
		id := m[1]
		if seen[id] {
			continue
		}
		lc, ok := byID[id]
		if !ok {
			continue // never a real claim this call retrieved -- dropped, not surfaced
		}
		seen[id] = true
		out = append(out, citationOf(lc.Claim))
	}
	return out
}

// sanitizeText strips any citation tag from the model's raw response that
// does not name a claim actually present in candidates. extractCitations
// already keeps Answer.Citations honest; without this, a hallucinated tag
// would still reach the user verbatim inside Answer.Text even though it
// resolves to nothing -- the same trust-nothing posture, applied to the
// prose the user actually reads rather than only to the structured field.
func sanitizeText(text string, candidates []liveClaim) string {
	byID := make(map[string]bool, len(candidates))
	for _, lc := range candidates {
		byID[lc.ID] = true
	}
	return citationTag.ReplaceAllStringFunc(text, func(tag string) string {
		m := citationTag.FindStringSubmatch(tag)
		if len(m) == 2 && byID[m[1]] {
			return tag
		}
		return "" // never a real claim this call retrieved -- stripped, not surfaced
	})
}

func citationOf(c domain.Claim) Citation {
	return Citation{
		ClaimID:    c.ID,
		Subject:    c.SubjectSlug,
		Predicate:  c.Predicate,
		Object:     c.Object,
		SourceRef:  c.SourceRef,
		Confidence: c.Confidence,
		ObservedAt: c.Provenance.ObservedAt,
		ValidTo:    c.ValidTo,
	}
}

// supersessionsFor builds the full lifecycle for every candidate claim
// that has ancestor history -- independent of whether the model's prose
// actually cited that claim's tag: a claim judged relevant enough to
// retrieve has its history surfaced regardless, so the guarantee never
// depends on the model doing its job correctly.
func supersessionsFor(candidates []liveClaim) []Supersession {
	var out []Supersession
	for _, lc := range candidates {
		if len(lc.History) == 0 {
			continue
		}
		chain := make([]Citation, 0, len(lc.History)+1)
		for _, h := range lc.History {
			chain = append(chain, citationOf(h))
		}
		chain = append(chain, citationOf(lc.Claim))
		out = append(out, Supersession{Subject: lc.SubjectSlug, Predicate: lc.Predicate, Chain: chain})
	}
	return out
}

// renderSupersession renders RFC §11's own phrasing ("believed X until
// June, now Y") for one fact's chain, oldest to current.
func renderSupersession(s Supersession) string {
	parts := make([]string, len(s.Chain))
	for i, step := range s.Chain {
		if i == len(s.Chain)-1 {
			parts[i] = fmt.Sprintf("now %s [claim:%s]", step.Object, step.ClaimID)
		} else {
			parts[i] = fmt.Sprintf("believed %s until %s [claim:%s]", step.Object, untilLabel(step), step.ClaimID)
		}
	}
	return fmt.Sprintf("Supersession (%s %s): %s.", s.Subject, s.Predicate, strings.Join(parts, ", then "))
}

// untilLabel is the date an outdated chain step's "believed X until ..."
// names: the claim's own ValidTo when the claim carries a validity window
// (the field means exactly "valid until"), falling back to when the claim
// was observed for a claim with no window recorded.
func untilLabel(step Citation) string {
	if step.ValidTo != "" {
		return step.ValidTo
	}
	return formatObservedAt(step.ObservedAt)
}

// gapAnswer builds RFC §11's explicit gap statement: no claim answers the
// question, stated plainly, plus how stale the brain's newest evidence is
// overall -- never a fabricated answer, never a silent empty result.
func (c *Composer) gapAnswer(bySubject map[string][]domain.Claim) Answer {
	newest := newestObservedAt(bySubject)
	var gap string
	if newest.IsZero() {
		gap = "This brain has no claims yet -- there is no evidence to answer this question."
	} else {
		age := c.now().Sub(newest)
		gap = fmt.Sprintf("No claim in this brain answers this question. The newest evidence in the brain is from %s, %s old.",
			newest.Format("2006-01-02"), formatAge(age))
	}
	return Answer{Gap: gap}
}

func newestObservedAt(bySubject map[string][]domain.Claim) time.Time {
	var newest time.Time
	for _, claims := range bySubject {
		for _, cl := range claims {
			if t := cl.Provenance.ObservedAt; t.After(newest) {
				newest = t
			}
		}
	}
	return newest
}

func formatAge(d time.Duration) string {
	days := int(d.Hours() / 24)
	switch {
	case days <= 0:
		return "less than a day"
	case days == 1:
		return "1 day"
	default:
		return fmt.Sprintf("%d days", days)
	}
}
