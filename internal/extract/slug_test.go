package extract

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/extract/chunk"
	"github.com/sirerun/serenity/internal/router"
)

// TestExtractDropsUnsafeSubjectAndControlObjectKeepsBatch is the model-
// boundary half of the SEC-H03 fix: a subject that is not a canonical slug
// (the review's newline-plus-YAML-key payload, its alias-hijack variant, a
// colon, a benign slash, a glob bracket, an uppercase letter) or an object
// carrying a control character is dropped by filterCandidates and counted,
// while every other candidate in the same response still becomes an
// observation (FUN-03: reject per candidate, never the batch).
func TestExtractDropsUnsafeSubjectAndControlObjectKeepsBatch(t *testing.T) {
	const modelVersion = "fake-extractor@v1"
	raw := []map[string]any{
		{"subject": "acme-corp", "predicate": "works_at", "object": "Acme Corp", "confidence": 0.8},
		{"subject": "acme\ntype: person\naliases: [\"alice-tan\"]", "predicate": "works_at", "object": "Evil Corp", "confidence": 0.9},
		{"subject": "acme\naliases: [\"alice-tan\"]", "predicate": "works_at", "object": "Evil Corp", "confidence": 0.9},
		{"subject": "acme:corp", "predicate": "works_at", "object": "Acme Corp", "confidence": 0.9},
		{"subject": "acme/inc", "predicate": "works_at", "object": "Acme Corp", "confidence": 0.9},
		{"subject": "x[1]", "predicate": "works_at", "object": "Acme Corp", "confidence": 0.9},
		{"subject": "Acme", "predicate": "works_at", "object": "Acme Corp", "confidence": 0.9},
		{"subject": "acme-corp", "predicate": "said", "object": "hello\x1b[31mworld", "confidence": 0.9},
		{"subject": "acme-corp", "predicate": "said", "object": "tab\tseparated", "confidence": 0.9},
		{"subject": "lily-chen", "predicate": "relates_to", "object": "acme-corp", "confidence": 0.7},
	}
	body, err := json.Marshal(map[string]any{"observations": raw})
	if err != nil {
		t.Fatal(err)
	}
	fp := &fakeProvider{name: "fake", modelVersion: modelVersion, resp: router.Response{Text: string(body)}}
	ex := New(newTestRouter(fp), modelVersion, nil, nil)

	chunkText := "Jane works at Acme Corp. Lily Chen knows Acme Corp."
	result, err := ex.ExtractChunk(context.Background(), "src-sec-h03", false, chunk.Chunk{Span: chunk.Span{Start: 0, End: len(chunkText)}, Text: chunkText}, router.Budget{})
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}
	all := append(append([]domain.Observation{}, result.Ready...), result.Distill...)
	if len(all) != 2 {
		t.Fatalf("total observations = %d, want 2 (only the canonical-slug, control-free candidates survive); got %+v", len(all), all)
	}
	for _, o := range all {
		if !domain.ValidSlug(o.SubjectSlug) {
			t.Fatalf("observation with non-canonical subject %q reached the result", o.SubjectSlug)
		}
		if strings.IndexFunc(o.Object, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
			t.Fatalf("observation with control character in object %q reached the result", o.Object)
		}
	}
	if result.Rejected != 8 {
		t.Fatalf("Rejected = %d, want 8 (one per unsafe candidate, batch continues)", result.Rejected)
	}
}
