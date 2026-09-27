package memory

import (
	"context"
	"strings"
	"testing"
)

// TestRecallCapsRejectBeforeEmbedder pins SEC-L06: an oversize query or
// limit is rejected as invalid_params, naming the cap, before the paid
// embedder sees a single call. The at-cap controls prove the recording
// embedder is really on the search path, so zero calls on the rejects is
// the validation's doing, not an unwired fake.
func TestRecallCapsRejectBeforeEmbedder(t *testing.T) {
	cases := []struct {
		name      string
		args      map[string]any
		reject    bool
		wantInMsg string
	}{
		{name: "query over byte cap", args: map[string]any{"query": strings.Repeat("a", 4097)}, reject: true, wantInMsg: "4096 bytes"},
		{name: "multibyte query over byte cap", args: map[string]any{"query": strings.Repeat("é", 2049)}, reject: true, wantInMsg: "4096 bytes"},
		{name: "limit over cap", args: map[string]any{"query": "attributed fact", "limit": 101}, reject: true, wantInMsg: "100"},
		{name: "query at byte cap", args: map[string]any{"query": strings.Repeat("a", 4096)}},
		{name: "limit at cap", args: map[string]any{"query": "attributed fact", "limit": 100}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := newTestHandlers(t)
			e := &recordingMemoryEmbedder{}
			h.deps.Embedder = e
			v, bad, err := h.recall(context.Background(), mustMarshal(t, tc.args))
			if err != nil {
				t.Fatalf("recall error: %v", err)
			}
			if !tc.reject {
				if bad || e.count() != 1 {
					t.Fatalf("at-cap recall should reach the embedder once: bad=%v calls=%d resp=%+v", bad, e.count(), v)
				}
				return
			}
			ve, ok := v.(VerbError)
			if !bad || !ok || ve.Error != ErrCodeInvalidParams {
				t.Fatalf("want invalid_params with no embedder call, got bad=%v calls=%d resp=%+v", bad, e.count(), v)
			}
			if !strings.Contains(ve.Message, tc.wantInMsg) {
				t.Fatalf("error message %q does not name the cap %q", ve.Message, tc.wantInMsg)
			}
			if e.count() != 0 {
				t.Fatalf("embedder called %d times for a rejected recall", e.count())
			}
		})
	}
}
