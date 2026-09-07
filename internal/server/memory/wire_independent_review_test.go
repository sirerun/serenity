package memory

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/router"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

func TestMemoryWireReviewTypedEntityIsolation(t *testing.T) {
	h, _ := newTestHandlers(t)
	for _, typ := range []string{"person", "org"} {
		page := store.NewEntityPage(domain.Entity{Type: typ, Slug: "alex"})
		if _, _, err := writer.Fence(h.deps.Queue, h.deps.Fence, page); err != nil {
			t.Fatal(err)
		}
	}
	first := acceptanceCall(t, h, "remember", map[string]any{"fact": "same text distinct subject", "provenance": "independent namespace review", "entity": "person/alex"})
	second := acceptanceCall(t, h, "remember", map[string]any{"fact": "same text distinct subject", "provenance": "independent namespace review", "entity": "org/alex"})
	if first["id"] == second["id"] || second["status"] != "inserted" {
		t.Errorf("typed entities collapsed into one duplicate identity: %v / %v", first, second)
	}
	third := acceptanceCall(t, h, "remember", map[string]any{"fact": "person-only commitment", "provenance": "independent namespace review", "entity": "person/alex", "kind": "commitment"})
	if third["status"] != "inserted" {
		t.Fatalf("positive fixture not saved: %v", third)
	}
	for _, tc := range []struct {
		typ   string
		count int
		id    any
	}{{"person", 2, first["id"]}, {"org", 1, second["id"]}} {
		t.Run(tc.typ, func(t *testing.T) {
			recall := acceptanceCall(t, h, "recall", map[string]any{"entity": tc.typ + "/alex"})
			facts := recall["facts"].([]any)
			if len(facts) != tc.count {
				t.Errorf("typed recall mixed entities: %v", recall)
			}
			found := false
			for _, fact := range facts {
				if fact.(map[string]any)["fact_id"] == tc.id {
					found = true
				}
			}
			if !found {
				t.Errorf("typed recall lost own identity: %v", recall)
			}
			entity := acceptanceCall(t, h, "entity", map[string]any{"name": tc.typ + "/alex"})
			card := entity["card"].(map[string]any)
			if card["active_fact_count"] != float64(tc.count) {
				t.Errorf("typed card mixed fact counts: %v", entity)
			}
			if tc.typ == "org" && len(card["open_threads"].([]any)) != 0 {
				t.Errorf("person commitment leaked into org card: %v", entity)
			}
		})
	}
}

type wireReviewCompleter struct {
	text, prompt string
	usage        router.Usage
}

func (c *wireReviewCompleter) Complete(_ context.Context, _ router.TaskClass, p router.Prompt, _ router.Budget) (router.Result, error) {
	c.prompt = p.Text
	return router.Result{Text: c.text, ModelVersion: "actual-provider@v2", Usage: c.usage}, nil
}

func TestMemoryV1SynthesisEvidenceCost(t *testing.T) {
	for _, measured := range []bool{false, true} {
		name := "unknown_usage"
		if measured {
			name = "measured_usage"
		}
		t.Run(name, func(t *testing.T) {
			h, root := newTestHandlers(t)
			public := acceptanceCall(t, h, "remember", map[string]any{"fact": "costreview PUBLICCURRENT", "provenance": "current public attribution"})
			acceptanceCall(t, h, "remember", map[string]any{"fact": "costreview PRIVATECURRENT", "provenance": "private attribution", "visibility": "private"})
			h.deps.Clock = fixedClock{testNow.Add(-365 * 24 * time.Hour)}
			acceptanceCall(t, h, "remember", map[string]any{"fact": "costreview PUBLICOLD", "provenance": "historical attribution"})
			h.deps.Clock = fixedClock{testNow}
			if err := index.Rebuild(t.Context(), root, h.deps.Config, h.deps.Index); err != nil {
				t.Fatal(err)
			}
			id := public["id"].(string)
			provider := &wireReviewCompleter{text: "Current report [source:" + id + "]."}
			if measured {
				provider.usage = router.Usage{InputTokens: 27, OutputTokens: 9, CostUSD: 0.0042}
			}
			h.deps.Composer = provider
			h.deps.ComposerModelVersion = "configured-alias@v1"
			got := acceptanceCall(t, h, "synthesize", map[string]any{"question": "costreview", "since": testNow.Add(-time.Hour).Format(time.RFC3339), "until": testNow.Add(time.Hour).Format(time.RFC3339)})
			if got["error"] != nil {
				t.Fatalf("synthesis failed: %v", got)
			}
			if !strings.Contains(provider.prompt, "PUBLICCURRENT") || strings.Contains(provider.prompt, "PRIVATECURRENT") || strings.Contains(provider.prompt, "PUBLICOLD") {
				t.Fatalf("provider received wrong privacy/date evidence: %s", provider.prompt)
			}
			sources := got["sources"].([]any)
			if len(sources) != 1 || sources[0] != "source-"+id {
				t.Errorf("entityless source identity lost: %v", sources)
			}
			cost := got["cost"].(map[string]any)
			if cost["model"] != "actual-provider@v2" {
				t.Errorf("cost mislabeled actual provider: %v", cost)
			}
			if measured {
				if cost["input_tokens"] != float64(27) || cost["output_tokens"] != float64(9) || cost["usd_estimate"] != 0.0042 {
					t.Errorf("measured usage changed: %v", cost)
				}
			} else if cost["input_tokens"] != nil || cost["output_tokens"] != nil || cost["usd_estimate"] != nil {
				t.Errorf("unknown usage fabricated: %v", cost)
			}
		})
	}
}
