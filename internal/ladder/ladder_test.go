package ladder

import (
	"context"
	"errors"
	"testing"
	"time"
)

// validConfigYAML mirrors RFC 0001 §10.3's own example block exactly.
const validConfigYAML = `
default: {min_dispositions: 50, min_accept: 0.98, sample_rate: 0.05}
per_cell:
  email/has_balance:  {min_dispositions: 50,  min_accept: 0.99, sample_rate: 0.05}
  email/works_at:     {min_dispositions: 20,  min_accept: 0.95, sample_rate: 0.10}
correlation_guards:
  min_span_days: 14
  min_distinct_sources: 5
never_automate: [precept-touching, effect-requests, tombstones, contact_new_party]
`

func TestParseConfigValidDocument(t *testing.T) {
	cfg, err := ParseConfig([]byte(validConfigYAML))
	if err != nil {
		t.Fatalf("ParseConfig: unexpected error: %v", err)
	}
	if cfg.Default.MinDispositions != 50 || cfg.Default.MinAccept != 0.98 || cfg.Default.SampleRate != 0.05 {
		t.Fatalf("Default = %+v, want RFC §10.3 priors", cfg.Default)
	}
	if got := cfg.PolicyFor("email/has_balance"); got.MinAccept != 0.99 {
		t.Fatalf("PolicyFor(email/has_balance).MinAccept = %v, want 0.99", got.MinAccept)
	}
	if got := cfg.PolicyFor("email/unknown_family"); got != cfg.Default {
		t.Fatalf("PolicyFor(unknown) = %+v, want Default fallback %+v", got, cfg.Default)
	}
	if cfg.CorrelationGuards.MinSpanDays != 14 || cfg.CorrelationGuards.MinDistinctSources != 5 {
		t.Fatalf("CorrelationGuards = %+v, want {14 5}", cfg.CorrelationGuards)
	}
	if !cfg.NeverAutomates("tombstones") {
		t.Fatalf("NeverAutomates(tombstones) = false, want true")
	}
	if cfg.NeverAutomates("email/has_balance") {
		t.Fatalf("NeverAutomates(email/has_balance) = true, want false")
	}
}

// TestParseConfigMissingCorrelationGuardFieldsFailsToParse is the acc
// line's own clause, verbatim: "config missing min_span_days or
// min_distinct_sources fails to parse".
func TestParseConfigMissingCorrelationGuardFieldsFailsToParse(t *testing.T) {
	cases := map[string]string{
		"missing min_span_days": `
default: {min_dispositions: 50, min_accept: 0.98, sample_rate: 0.05}
correlation_guards:
  min_distinct_sources: 5
`,
		"missing min_distinct_sources": `
default: {min_dispositions: 50, min_accept: 0.98, sample_rate: 0.05}
correlation_guards:
  min_span_days: 14
`,
		"missing correlation_guards entirely": `
default: {min_dispositions: 50, min_accept: 0.98, sample_rate: 0.05}
`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseConfig([]byte(doc)); err == nil {
				t.Fatalf("ParseConfig(%s): got nil error, want a parse failure", name)
			}
		})
	}
}

func TestParseConfigRejectsMalformedYAML(t *testing.T) {
	if _, err := ParseConfig([]byte("not: [valid: yaml")); err == nil {
		t.Fatalf("ParseConfig(malformed yaml): got nil error, want a parse failure")
	}
}

// synthHistory builds n dispositions, all accepted, spread evenly across
// spanDays and distinctSources distinct source ids.
func synthHistory(n, spanDays, distinctSources int) []Disposition {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	modulus := distinctSources
	if modulus < 1 {
		modulus = 1
	}
	history := make([]Disposition, n)
	for i := 0; i < n; i++ {
		dayOffset := 0
		if n > 1 && spanDays > 0 {
			dayOffset = i * spanDays / (n - 1)
		}
		history[i] = Disposition{
			Accepted:   true,
			OccurredAt: start.AddDate(0, 0, dayOffset),
			SourceID:   sourceID(i % modulus),
		}
	}
	return history
}

func sourceID(i int) string {
	return "src-" + string(rune('a'+i))
}

func testConfig() *Config {
	return &Config{
		Default:           CellPolicy{MinDispositions: 10, MinAccept: 0.9, SampleRate: 0.5},
		CorrelationGuards: CorrelationGuards{MinSpanDays: 14, MinDistinctSources: 5},
		NeverAutomate:     []string{"tombstones"},
	}
}

func TestPromoteSatisfiedHistoryPromotes(t *testing.T) {
	e := NewEngine(testConfig())
	history := synthHistory(10, 20, 5) // meets counts, span, and sources
	if !e.Promote("email/has_balance", history) {
		t.Fatalf("Promote: got false, want true for a history satisfying every threshold")
	}
	if got := e.TrustOf("email/has_balance"); got != Trust1 {
		t.Fatalf("TrustOf after promotion = %v, want Trust1", got)
	}
}

// TestPromoteMeetingCountsButShortSpanDoesNotPromote is the acc line's own
// clause, verbatim: "synthetic history meeting counts but spanning 3 days
// does not promote".
func TestPromoteMeetingCountsButShortSpanDoesNotPromote(t *testing.T) {
	e := NewEngine(testConfig())
	// 10 dispositions (meets MinDispositions), all accepted (meets
	// MinAccept), across 5 distinct sources (meets MinDistinctSources) --
	// but crammed into a 3-day span, well under the 14-day guard.
	history := synthHistory(10, 3, 5)
	if e.Promote("email/has_balance", history) {
		t.Fatalf("Promote: got true, want false — history spans only 3 days, guard requires 14")
	}
	if got := e.TrustOf("email/has_balance"); got != Trust0 {
		t.Fatalf("TrustOf after failed promotion = %v, want Trust0", got)
	}
}

func TestPromoteInsufficientDistinctSourcesDoesNotPromote(t *testing.T) {
	e := NewEngine(testConfig())
	history := synthHistory(10, 20, 2) // meets span, fails distinct-source guard (needs 5)
	if e.Promote("email/has_balance", history) {
		t.Fatalf("Promote: got true, want false — only 2 distinct sources, guard requires 5")
	}
}

func TestPromoteBelowMinDispositionsDoesNotPromote(t *testing.T) {
	e := NewEngine(testConfig())
	history := synthHistory(3, 20, 5) // below MinDispositions (10)
	if e.Promote("email/has_balance", history) {
		t.Fatalf("Promote: got true, want false — only 3 dispositions, policy requires 10")
	}
}

func TestPromoteBelowMinAcceptDoesNotPromote(t *testing.T) {
	e := NewEngine(testConfig())
	history := synthHistory(10, 20, 5)
	// Flip half the history to rejected, well below the 0.9 accept-rate
	// threshold.
	for i := range history {
		if i%2 == 0 {
			history[i].Accepted = false
		}
	}
	if e.Promote("email/has_balance", history) {
		t.Fatalf("Promote: got true, want false — accept rate is 0.5, policy requires 0.9")
	}
}

// TestDemoteAfterHumanReversalDropsToTrust0 is the acc line's own clause,
// verbatim: "one human reversal of an auto action demotes the cell to
// trust 0".
func TestDemoteAfterHumanReversalDropsToTrust0(t *testing.T) {
	e := NewEngine(testConfig())
	history := synthHistory(10, 20, 5)
	if !e.Promote("email/has_balance", history) {
		t.Fatalf("Promote: want true as setup precondition")
	}
	if got := e.TrustOf("email/has_balance"); got != Trust1 {
		t.Fatalf("TrustOf before reversal = %v, want Trust1", got)
	}

	// A human reverses one auto action.
	e.Demote("email/has_balance")

	if got := e.TrustOf("email/has_balance"); got != Trust0 {
		t.Fatalf("TrustOf after one human reversal = %v, want Trust0", got)
	}
}

// TestNeverAutomateCellRefusesPromotion is the acc line's own clause,
// verbatim: "a never_automate cell refuses promotion".
func TestNeverAutomateCellRefusesPromotion(t *testing.T) {
	e := NewEngine(testConfig())
	// A history that would satisfy every count/rate/guard threshold for
	// any ordinary cell.
	history := synthHistory(10, 20, 5)
	if e.Promote("tombstones", history) {
		t.Fatalf("Promote(tombstones): got true, want false — tombstones is in never_automate")
	}
	if got := e.TrustOf("tombstones"); got != Trust0 {
		t.Fatalf("TrustOf(tombstones) = %v, want Trust0", got)
	}
}

type fakeRecorder struct {
	actions []AutoAction
}

func (f *fakeRecorder) RecordAutoAction(_ context.Context, action AutoAction) error {
	f.actions = append(f.actions, action)
	return nil
}

var errRecordFailed = errors.New("record failed")

type failingRecorder struct{}

func (failingRecorder) RecordAutoAction(context.Context, AutoAction) error {
	return errRecordFailed
}

// TestLogAutoActionAlwaysRecordsAndSamplesAtRate is the acc line's own
// clause, verbatim: "every auto action writes an auto disposition and is
// sampled at sample_rate".
func TestLogAutoActionAlwaysRecordsAndSamplesAtRate(t *testing.T) {
	cfg := &Config{
		Default:           CellPolicy{MinDispositions: 1, MinAccept: 0, SampleRate: 0.3},
		CorrelationGuards: CorrelationGuards{MinSpanDays: 0, MinDistinctSources: 0},
	}
	e := NewEngine(cfg)

	// Deterministic sequence: 3 of 10 draws fall under the 0.3 sample
	// rate (0.10, 0.20, 0.29), the other 7 do not.
	draws := []float64{0.10, 0.99, 0.20, 0.50, 0.29, 0.31, 0.60, 0.70, 0.80, 0.90}
	i := 0
	e.RandFloat64 = func() float64 {
		v := draws[i]
		i++
		return v
	}

	rec := &fakeRecorder{}
	ctx := context.Background()
	at := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for range draws {
		if err := e.LogAutoAction(ctx, rec, "email/has_balance", at); err != nil {
			t.Fatalf("LogAutoAction: unexpected error: %v", err)
		}
	}

	if len(rec.actions) != len(draws) {
		t.Fatalf("recorded %d auto actions, want %d — every auto action must be logged regardless of sampling", len(rec.actions), len(draws))
	}

	sampledCount := 0
	for _, a := range rec.actions {
		if a.Cell != "email/has_balance" {
			t.Fatalf("action.Cell = %q, want email/has_balance", a.Cell)
		}
		if !a.Timestamp.Equal(at) {
			t.Fatalf("action.Timestamp = %v, want %v", a.Timestamp, at)
		}
		if a.Sampled {
			sampledCount++
		}
	}
	if sampledCount != 3 {
		t.Fatalf("sampled %d of %d actions at rate 0.3 with a known draw sequence, want exactly 3", sampledCount, len(draws))
	}
}

func TestLogAutoActionPropagatesRecorderError(t *testing.T) {
	e := NewEngine(&Config{Default: CellPolicy{SampleRate: 0}})
	err := e.LogAutoAction(context.Background(), failingRecorder{}, "email/has_balance", time.Now())
	if !errors.Is(err, errRecordFailed) {
		t.Fatalf("LogAutoAction error = %v, want errRecordFailed", err)
	}
}

func TestTrustString(t *testing.T) {
	if Trust0.String() != "trust0" {
		t.Fatalf("Trust0.String() = %q, want trust0", Trust0.String())
	}
	if Trust1.String() != "trust1" {
		t.Fatalf("Trust1.String() = %q, want trust1", Trust1.String())
	}
}
