// Package chunk splits a source's text into an ordered, gapless,
// non-overlapping sequence of chunks — the first stage of the extraction
// pipeline (RFC 0001 §9, §7.6: chunk -> extract candidate observations ->
// embed -> reconcile). It has no dependency on any other internal package;
// T1.8 (extraction) and T1.10 (embeddings) import it, not the reverse.
package chunk

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Span is a byte-offset range into a source's text, [Start, End).
type Span struct {
	Start int
	End   int
}

// Chunk is one span of a source's text, plus its exact text: Text always
// equals text[Span.Start:Span.End] for the text Split was called with.
type Chunk struct {
	Span Span
	Text string
}

// Config bounds one Split call.
type Config struct {
	// MaxTokens caps each chunk's approximate token count. A "token" here
	// is a maximal run of non-whitespace runes (a word) — a v1
	// approximation; no real tokenizer dependency exists yet in go.mod.
	// MaxTokens <= 0 falls back to DefaultMaxTokens.
	MaxTokens int
	// OverlapTokens is a backward SEARCH WINDOW, in words: when a raw
	// max-token cut would land mid-paragraph, Split may look up to
	// OverlapTokens words earlier for a blank-line break and snap the
	// chunk boundary there instead. It never causes duplicated or
	// overlapping span content — every boundary Split picks is one of
	// the text's own word-start byte offsets, so returned spans always
	// exactly tile the source. OverlapTokens < 0 falls back to 0;
	// OverlapTokens >= the effective MaxTokens clamps to MaxTokens - 1
	// (a chunk always makes forward progress by at least one word).
	OverlapTokens int
}

// DefaultMaxTokens and DefaultOverlapTokens are the pinned chunking
// constants (T1.6 acc: "token bound and overlap are config constants
// pinned by test"). Changing either is a deliberate, tested decision — it
// reshapes every downstream chunk boundary and cache key (T1.8's
// (chunk sha, model@version, prompt version) cache key).
const (
	DefaultMaxTokens     = 512
	DefaultOverlapTokens = 64
)

// DefaultConfig is the chunking configuration used unless overridden.
var DefaultConfig = Config{MaxTokens: DefaultMaxTokens, OverlapTokens: DefaultOverlapTokens}

// Split partitions text into an ordered, gapless, non-overlapping sequence
// of chunks bounded by cfg.MaxTokens words, snapping boundaries to blank
// lines within cfg.OverlapTokens words when one is available (see Config).
// text == "" returns nil. A word (a maximal run of non-whitespace runes)
// never spans two chunks, and a chunk boundary never splits a multi-byte
// UTF-8 rune. If text has no word at all (entirely whitespace, non-empty),
// Split returns exactly one chunk spanning the whole text.
//
// The returned chunks always satisfy: chunks[0].Span.Start == 0,
// chunks[len-1].Span.End == len(text), chunks[i].Span.End ==
// chunks[i+1].Span.Start for every adjacent pair (ordered, contiguous,
// non-overlapping — their union is exactly the source text), and
// chunks[i].Text == text[chunks[i].Span.Start:chunks[i].Span.End].
func Split(text string, cfg Config) []Chunk {
	if text == "" {
		return nil
	}

	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	overlap := cfg.OverlapTokens
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= maxTokens {
		overlap = maxTokens - 1
	}

	starts, ends := wordSpans(text)
	if len(starts) == 0 {
		return []Chunk{{Span: Span{Start: 0, End: len(text)}, Text: text}}
	}

	var chunks []Chunk
	pos := 0
	wordIdx := 0
	for pos < len(text) {
		targetIdx := wordIdx + maxTokens

		end := len(text)
		nextIdx := len(starts)
		if targetIdx < len(starts) {
			end = starts[targetIdx]
			nextIdx = targetIdx
		}

		upper := targetIdx
		if upper > len(starts)-1 {
			upper = len(starts) - 1
		}
		lower := targetIdx - overlap
		if lower < wordIdx+1 {
			lower = wordIdx + 1
		}
		for j := upper; j >= lower; j-- {
			if strings.Contains(text[ends[j-1]:starts[j]], "\n\n") {
				end = starts[j]
				nextIdx = j
				break
			}
		}

		chunks = append(chunks, Chunk{Span: Span{Start: pos, End: end}, Text: text[pos:end]})
		pos = end
		wordIdx = nextIdx
	}
	return chunks
}

// wordSpans scans text UTF-8-safely and returns the byte-offset start and
// end of every maximal run of non-whitespace runes ("word"), in order.
// starts[i] < ends[i] <= starts[i+1] for every i.
func wordSpans(text string) (starts, ends []int) {
	inWord := false
	wordStart := 0
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if unicode.IsSpace(r) {
			if inWord {
				starts = append(starts, wordStart)
				ends = append(ends, i)
				inWord = false
			}
		} else if !inWord {
			inWord = true
			wordStart = i
		}
		i += size
	}
	if inWord {
		starts = append(starts, wordStart)
		ends = append(ends, len(text))
	}
	return starts, ends
}
