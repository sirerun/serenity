package chunk

import (
	"math/rand"
	"strings"
	"testing"
)

// trial is one randomized (text, Config) input for TestSplitProperty.
type trial struct {
	text string
	cfg  Config
}

// genTrials deterministically generates the fixed set of property-test
// trials (T1.6 acc: >= 200 trials, varied text and Config, including edge
// Config values: 0, negative, 1, and OverlapTokens >= MaxTokens). The fixed
// seed mirrors internal/store/normalizer_test.go's style.
func genTrials() []trial {
	rng := rand.New(rand.NewSource(42))
	alphabet := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 \t\n日é🙂")

	trials := make([]trial, 0, 220)
	for i := 0; i < 220; i++ {
		n := rng.Intn(400)
		var b strings.Builder
		for j := 0; j < n; j++ {
			b.WriteRune(alphabet[rng.Intn(len(alphabet))])
			if rng.Intn(37) == 0 {
				b.WriteString("\n\n") // occasional blank-line paragraph break
			}
		}

		var cfg Config
		switch rng.Intn(6) {
		case 0:
			cfg.MaxTokens = 0
		case 1:
			cfg.MaxTokens = -rng.Intn(10)
		case 2:
			cfg.MaxTokens = 1
		default:
			cfg.MaxTokens = 1 + rng.Intn(40)
		}
		eff := cfg.MaxTokens
		if eff <= 0 {
			eff = DefaultMaxTokens
		}
		switch rng.Intn(4) {
		case 0:
			cfg.OverlapTokens = -1 - rng.Intn(5)
		case 1:
			cfg.OverlapTokens = 0
		case 2:
			cfg.OverlapTokens = eff + rng.Intn(10)
		default:
			cfg.OverlapTokens = rng.Intn(20)
		}

		trials = append(trials, trial{text: b.String(), cfg: cfg})
	}
	return trials
}

// TestSplitProperty proves the T1.6 acceptance invariants across randomized
// text and Config values: chunk union == source text, spans ordered and
// non-overlapping, text[span] reconstructs each chunk, and the token bound
// is actually respected.
func TestSplitProperty(t *testing.T) {
	trials := genTrials()

	t.Run("union", func(t *testing.T) {
		for i, tr := range trials {
			var got strings.Builder
			for _, c := range Split(tr.text, tr.cfg) {
				got.WriteString(c.Text)
			}
			if got.String() != tr.text {
				t.Fatalf("trial %d (cfg=%+v): union mismatch: got %d bytes, want %d bytes",
					i, tr.cfg, got.Len(), len(tr.text))
			}
		}
	})

	t.Run("ordered_nonoverlap", func(t *testing.T) {
		for i, tr := range trials {
			chunks := Split(tr.text, tr.cfg)
			if tr.text == "" {
				if len(chunks) != 0 {
					t.Fatalf("trial %d: expected no chunks for empty text, got %d", i, len(chunks))
				}
				continue
			}
			if len(chunks) == 0 {
				t.Fatalf("trial %d: expected at least one chunk for non-empty text", i)
			}
			if chunks[0].Span.Start != 0 {
				t.Fatalf("trial %d: first chunk must start at 0, got %d", i, chunks[0].Span.Start)
			}
			last := chunks[len(chunks)-1]
			if last.Span.End != len(tr.text) {
				t.Fatalf("trial %d: last chunk must end at len(text)=%d, got %d", i, len(tr.text), last.Span.End)
			}
			for j := 0; j < len(chunks); j++ {
				if chunks[j].Span.Start >= chunks[j].Span.End {
					t.Fatalf("trial %d: chunk %d is empty/degenerate: %+v", i, j, chunks[j].Span)
				}
				if j > 0 && chunks[j-1].Span.End != chunks[j].Span.Start {
					t.Fatalf("trial %d: chunk %d end %d != chunk %d start %d (gap or overlap)",
						i, j-1, chunks[j-1].Span.End, j, chunks[j].Span.Start)
				}
			}
		}
	})

	t.Run("reconstruct", func(t *testing.T) {
		for i, tr := range trials {
			for j, c := range Split(tr.text, tr.cfg) {
				if tr.text[c.Span.Start:c.Span.End] != c.Text {
					t.Fatalf("trial %d chunk %d: text[span] != chunk.Text", i, j)
				}
			}
		}
	})

	t.Run("token_bound", func(t *testing.T) {
		for i, tr := range trials {
			eff := tr.cfg.MaxTokens
			if eff <= 0 {
				eff = DefaultMaxTokens
			}
			for j, c := range Split(tr.text, tr.cfg) {
				if words := len(strings.Fields(c.Text)); words > eff {
					t.Fatalf("trial %d chunk %d: %d words exceeds MaxTokens bound %d", i, j, words, eff)
				}
			}
		}
	})
}

// TestDefaultConstants pins the chunking constants (T1.6 acc: "token bound
// and overlap are config constants pinned by test") — a future change to
// either is a deliberate, tested decision.
func TestDefaultConstants(t *testing.T) {
	if DefaultMaxTokens != 512 {
		t.Fatalf("DefaultMaxTokens = %d, want 512", DefaultMaxTokens)
	}
	if DefaultOverlapTokens != 64 {
		t.Fatalf("DefaultOverlapTokens = %d, want 64", DefaultOverlapTokens)
	}
	want := Config{MaxTokens: DefaultMaxTokens, OverlapTokens: DefaultOverlapTokens}
	if DefaultConfig != want {
		t.Fatalf("DefaultConfig = %+v, want %+v", DefaultConfig, want)
	}
}

// TestSplitBlankLineSnap is a worked example (not randomized): a naive
// max-token cut lands mid-paragraph, but a wide enough OverlapTokens search
// window snaps the boundary to the blank line between paragraphs instead.
func TestSplitBlankLineSnap(t *testing.T) {
	para1 := "one two three four five"
	para2 := "six seven eight nine ten"
	text := para1 + "\n\n" + para2

	naive := Split(text, Config{MaxTokens: 7, OverlapTokens: 0})
	if len(naive) < 2 || !strings.HasPrefix(naive[1].Text, "eight") {
		t.Fatalf(`expected the raw (unsnapped) cut to land mid-paragraph2 (chunk 1 starting with "eight"), got %+v`, naive)
	}

	snapped := Split(text, Config{MaxTokens: 7, OverlapTokens: 3})
	if len(snapped) != 2 {
		t.Fatalf("expected exactly 2 chunks after snapping to the blank line, got %d: %+v", len(snapped), snapped)
	}
	if snapped[0].Text != para1+"\n\n" {
		t.Fatalf("chunk 0 = %q, want %q", snapped[0].Text, para1+"\n\n")
	}
	if snapped[1].Text != para2 {
		t.Fatalf("chunk 1 = %q, want %q", snapped[1].Text, para2)
	}
}
