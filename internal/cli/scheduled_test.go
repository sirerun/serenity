package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
)

func TestScheduledReviewPresentationPreservesMeaning(t *testing.T) {
	raw, err := json.Marshal(disposition.DecayPayload{Origin: disposition.DecayOrigin, Claim: domain.Claim{SubjectSlug: "ava", Family: "has_role", Predicate: "has_role", Object: "Engineer", Confidence: .9}, DecayedConfidence: .1125})
	if err != nil {
		t.Fatal(err)
	}
	item := disposition.Item{Kind: disposition.KindDistill, Payload: raw}
	summary := itemSummary(item)
	for _, want := range []string{"ava", "Engineer", "stored confidence=0.90", "aged confidence=0.11", "canonical claim unchanged"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("missing %q: %s", want, summary)
		}
	}
	if family, ok := itemFamily(item); !ok || family != "has_role" {
		t.Fatalf("family=%s ok=%v", family, ok)
	}
	raw, err = json.Marshal(disposition.LexicalAliasPayload{Origin: disposition.LexicalAliasOrigin, A: "ava", B: "ava-lee", Reason: "human review only"})
	if err != nil {
		t.Fatal(err)
	}
	summary = itemSummary(disposition.Item{Kind: disposition.KindEntityMerge, Payload: raw})
	if !strings.Contains(summary, "ava / ava-lee") || !strings.Contains(summary, "human review only") {
		t.Fatal(summary)
	}
}

func TestCronDecayReportsActualCounts(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	if err := runInit(root, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runCron(context.Background(), root, "decay", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "claims=0 distill_created=0 alias_created=0 existing=0") {
		t.Fatal(out.String())
	}
}
