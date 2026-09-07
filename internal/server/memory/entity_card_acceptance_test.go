package memory

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/writer"
)

func TestMemoryV1EntityResolutionCard(t *testing.T) {
	t.Run("recent_events_and_commitment_cap", func(t *testing.T) {
		h, _ := newTestHandlers(t)
		entityReviewPage(t, h, "person", "jules", "Jules")
		save := func(fact, kind string, when time.Time) {
			t.Helper()
			h.deps.Clock = fixedClock{when}
			got := acceptanceCall(t, h, "remember", map[string]any{"fact": fact, "kind": kind, "entity": "person/jules", "provenance": "card acceptance fixture"})
			if got["status"] != "inserted" {
				t.Fatalf("source fixture did not save: %v", got)
			}
			h.deps.Clock = fixedClock{testNow}
		}
		save("recent release happened", "event", testNow.AddDate(0, 0, -2))
		save("historical release outside window", "event", testNow.AddDate(0, 0, -91))
		save("still open commitment", "commitment", testNow.AddDate(0, 0, -200))
		response := acceptanceCall(t, h, "entity", map[string]any{"name": "Jules"})
		card := response["card"].(map[string]any)
		threads := card["open_threads"].([]any)
		if len(threads) != 2 {
			t.Fatalf("recent event and active old commitment should remain; old event should not: %v", threads)
		}
		seen := map[string]string{}
		for _, raw := range threads {
			item := raw.(map[string]any)
			seen[item["text"].(string)] = item["kind"].(string)
		}
		if seen["recent release happened"] != "recent_event" || seen["still open commitment"] != "commitment" {
			t.Fatalf("public threads missing or mislabeled: %v", threads)
		}
		touched := card["last_touched"].(map[string]any)
		if touched["last_timeline_date"] != testNow.AddDate(0, 0, -2).Format("2006-01-02") {
			t.Fatalf("last timeline date did not follow eligible event: %v", touched)
		}
		if card["active_fact_count"] != float64(3) {
			t.Fatalf("thread window incorrectly expired source facts: %v", card)
		}
		for day := 0; day < 3; day++ {
			save(fmt.Sprintf("new commitment %d", day), "commitment", testNow.AddDate(0, 0, -day))
		}
		response = acceptanceCall(t, h, "entity", map[string]any{"name": "person/jules"})
		card = response["card"].(map[string]any)
		if len(card["open_threads"].([]any)) != 3 || card["active_fact_count"] != float64(6) {
			t.Fatalf("cap should bound threads while preserving truthful active count: %v", card)
		}
	})

	t.Run("canonical_edges_out_first_and_uncapped_backlinks", func(t *testing.T) {
		h, root := newTestHandlers(t)
		entityReviewPage(t, h, "person", "hub", "Hub")
		// A fixture-local vocabulary extension models a migrated brain that has a
		// mentions relation; the protocol must omit it regardless of storage tier.
		h.deps.Config.Families["mentions"] = config.Family{Tier: domain.TierShard, HalfLifeDays: 90}
		if err := h.deps.Config.Save(filepath.Join(root, config.FileName)); err != nil {
			t.Fatal(err)
		}
		h.deps.Shard.Vocabulary = map[string]bool{}
		for family := range h.deps.Config.Families {
			h.deps.Shard.Vocabulary[family] = true
		}
		appendEdge := func(id, subject, target, predicate string) {
			t.Helper()
			c := domain.Claim{ID: id, SubjectSlug: subject, Predicate: predicate, Family: predicate, Object: target, ObjectKey: id, State: domain.StateActive, Visibility: domain.VisibilityShared}
			if _, _, err := writer.Shard(h.deps.Queue, h.deps.Shard, c); err != nil {
				t.Fatal(err)
			}
		}
		for n := 0; n < 3; n++ {
			slug := fmt.Sprintf("outgoing-%02d", n)
			entityReviewPage(t, h, "person", slug, slug)
			appendEdge("out-"+slug, "hub", slug, "has_balance")
		}
		for n := 0; n < 12; n++ {
			slug := fmt.Sprintf("incoming-%02d", n)
			entityReviewPage(t, h, "person", slug, slug)
			appendEdge("in-"+slug, slug, "hub", "has_balance")
		}
		entityReviewPage(t, h, "person", "mentioned-only", "Mentioned Only")
		appendEdge("mention-out", "hub", "mentioned-only", "mentions")
		appendEdge("mention-in", "mentioned-only", "hub", "mentions")
		response := acceptanceCall(t, h, "entity", map[string]any{"name": "hub"})
		card := response["card"].(map[string]any)
		edges := card["edges"].([]any)
		if len(edges) != 10 || card["backlink_count"] != float64(12) {
			t.Fatalf("edge cap or uncapped canonical backlinks incorrect: %v", card)
		}
		outgoing := map[string]bool{}
		for i, raw := range edges {
			edge := raw.(map[string]any)
			if edge["type"] == "mentions" || edge["slug"] == "mentioned-only" {
				t.Fatalf("mention became a typed card edge: %v", edge)
			}
			if i < 3 {
				if edge["direction"] != "out" {
					t.Fatalf("outgoing edges must precede incoming: %v", edges)
				}
				outgoing[edge["slug"].(string)] = true
			} else if edge["direction"] != "in" {
				t.Fatalf("unexpected extra outgoing edge: %v", edges)
			}
		}
		for n := 0; n < 3; n++ {
			if !outgoing[fmt.Sprintf("outgoing-%02d", n)] {
				t.Fatalf("real outgoing edge lost: %v", edges)
			}
		}
	})

	t.Run("near_miss_and_suffix_resolution", func(t *testing.T) {
		h, _ := newTestHandlers(t)
		entityReviewPage(t, h, "project", "zephyr-initiative", "Zephyr Initiative", "Wind Project")
		response := acceptanceCall(t, h, "entity", map[string]any{"name": "Zephyr Unknown"})
		if response["found"] != false {
			t.Fatalf("near miss falsely claimed an exact entity: %v", response)
		}
		suggestions := response["suggestions"].([]any)
		if len(suggestions) != 1 || suggestions[0].(map[string]any)["slug"] != "zephyr-initiative" {
			t.Fatalf("near miss lost real token-overlap suggestion: %v", response)
		}
		for _, name := range []string{"initiative", "Wind Project", "Zephyr Initiative"} {
			response = acceptanceCall(t, h, "entity", map[string]any{"name": name})
			if response["found"] != true || response["card"].(map[string]any)["entity"].(map[string]any)["slug"] != "zephyr-initiative" {
				t.Fatalf("real name/alias/suffix lookup %q failed: %v", name, response)
			}
		}
	})
}
