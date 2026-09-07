package compose

// Independent coordinator regressions. All three tests failed semantically
// against the pre-repair T4.20 snapshot (private heads/history, gap metadata,
// and date filtering after search caps). Existing test-only fakeProvider and
// newTestRouter exercise the real Router->Provider prompt boundary.
import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/router"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

func reviewComposeRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{{"init", "--quiet"}, {"config", "user.name", "Composer review"}, {"config", "user.email", "review@example.invalid"}, {"config", "core.hooksPath", filepath.Join(root, "no-hooks")}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture git %v: %v: %s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".serenity/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", ".gitignore"}, {"-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "Seed review fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("fixture git %v: %v: %s", args, err, out)
		}
	}
	return root
}

func reviewShardClaim(t *testing.T, root, id, object, key string, visibility domain.Visibility, observed time.Time, supersedes string) {
	t.Helper()
	// Real append-only shard codec persists visibility and provenance, unlike
	// legacy fence cells. Never put private visibility into a lossy fence fixture.
	if err := store.NewShardStore(root).Append(domain.Claim{
		ID: id, SubjectSlug: "reviewer", Predicate: "has_balance", Family: "has_balance",
		Object: object, ObjectKey: key, Confidence: 0.9, State: domain.StateActive,
		Visibility: visibility, Supersedes: supersedes,
		Provenance: domain.Provenance{ObservedAt: observed, Actor: "machine"},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestComposeReviewPrivateHeadsAndAncestorsNeverEgress(t *testing.T) {
	root := reviewComposeRoot(t)
	old := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	reviewShardClaim(t, root, "review-private-ancestor", "reviewbudget PRIVATEANCESTORSENTINEL", "account-one", domain.VisibilityPrivate, old, "")
	reviewShardClaim(t, root, "review-public-head", "reviewbudget public balance", "account-one", domain.VisibilityShared, now, "review-private-ancestor")
	reviewShardClaim(t, root, "review-private-head", "reviewbudget PRIVATEHEADSENTINEL", "account-two", domain.VisibilityPrivate, now, "")
	var sent string
	fp := &fakeProvider{modelVersion: "review@v1", resp: router.Response{Text: "The public balance is [claim:review-public-head]."}, sentPrompt: &sent}
	c := New(root, config.Default(), fakeSearchStore{}, nil, newTestRouter(fp), "review@v1")
	c.now = fixedNow(now)
	answer, err := c.AskWithOptions(context.Background(), "reviewbudget", AskOptions{Since: now.Add(-time.Hour), Until: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sent, "review-public-head") {
		t.Fatal("positive public evidence never reached actual provider")
	}
	encoded, err := json.Marshal(answer)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"PRIVATEHEADSENTINEL", "PRIVATEANCESTORSENTINEL", "review-private-head", "review-private-ancestor", "2025-01-02"} {
		if strings.Contains(sent, forbidden) || strings.Contains(string(encoded), forbidden) {
			t.Errorf("private/out-of-window evidence leaked through prompt, answer, citation, history or gap: %s", forbidden)
		}
	}
}

func TestComposeReviewGapIgnoresIneligibleEvidence(t *testing.T) {
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	opts := AskOptions{Since: now.Add(-time.Hour), Until: now.Add(time.Hour)}
	gap := func(root string) string {
		t.Helper()
		var sent string
		fp := &fakeProvider{modelVersion: "review@v1", sentPrompt: &sent}
		c := New(root, config.Default(), fakeSearchStore{}, nil, newTestRouter(fp), "review@v1")
		c.now = fixedNow(now)
		a, err := c.AskWithOptions(context.Background(), "unrelatedquestion", opts)
		if err != nil {
			t.Fatal(err)
		}
		if sent != "" || a.Text != "" || len(a.Citations)+len(a.SourceCitations)+len(a.Supersessions) != 0 || a.Gap == "" {
			t.Fatalf("no eligible evidence must produce only a gap without calling provider: %+v, prompt=%q", a, sent)
		}
		return a.Gap
	}
	emptyGap := gap(reviewComposeRoot(t))
	for _, tc := range []struct {
		name       string
		visibility domain.Visibility
		observed   time.Time
	}{
		{"private_recent", domain.VisibilityPrivate, now},
		{"public_outside_window", domain.VisibilityShared, now.Add(-365 * 24 * time.Hour)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := reviewComposeRoot(t)
			reviewShardClaim(t, root, "review-ineligible", "GAPPRIVACYSENTINEL", "account", tc.visibility, tc.observed, "")
			if got := gap(root); got != emptyGap {
				t.Errorf("ineligible evidence changes observable gap metadata: got %q, empty eligible set %q", got, emptyGap)
			}
		})
	}
}

// Stable retrieval order is a test double only; the stored facts and lifecycle
// projection are real. It exposes cap-before-date filtering independently of
// SQLite ranking choices and returns the requested prefix on widening.
type reviewRankedSources struct{ hits []index.Hit }

func (s reviewRankedSources) SearchFTS(_ context.Context, _ string, limit int) ([]index.Hit, error) {
	if limit > len(s.hits) {
		limit = len(s.hits)
	}
	return append([]index.Hit(nil), s.hits[:limit]...), nil
}
func (reviewRankedSources) SearchVectors(context.Context, string, []float32, int) ([]index.Hit, error) {
	return nil, nil
}
func (reviewRankedSources) VectorFor(context.Context, string, string) ([]float32, bool, error) {
	return nil, false, nil
}

func TestComposeReviewSourceDatesFilterBeforeSearchCaps(t *testing.T) {
	root := reviewComposeRoot(t)
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	q := writer.NewQueue(nil)
	t.Cleanup(q.Close)
	w := writer.MemoryFact{Queue: q, Sources: store.NewSourceStore(root)}
	var ranked reviewRankedSources
	var eligibleID string
	for i := 0; i < 40; i++ {
		created := now.Add(-365 * 24 * time.Hour)
		fact := fmt.Sprintf("reviewbudget OUTSIDEWINDOW%d", i)
		if i == 39 {
			created, fact = now, "reviewbudget ELIGIBLESOURCESENTINEL"
		}
		r, err := w.Remember(writer.RememberInput{Fact: fact, Provenance: "independent review", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld}, created)
		if err != nil {
			t.Fatal(err)
		}
		if i == 39 {
			eligibleID = r.Record.SHA256
		}
		ranked.hits = append(ranked.hits, index.Hit{ChunkRef: "src:" + r.Record.SHA256 + ":0", Text: fact, SourceSHA256: r.Record.SHA256, Kind: store.SourceKindMemoryFact})
	}
	var sent string
	fp := &fakeProvider{modelVersion: "review@v1", resp: router.Response{Text: "The attributed report says [source:" + eligibleID + "]."}, sentPrompt: &sent}
	c := New(root, config.Default(), ranked, nil, newTestRouter(fp), "review@v1")
	c.now = fixedNow(now)
	a, err := c.AskWithOptions(context.Background(), "reviewbudget", AskOptions{Since: now.Add(-time.Hour), Until: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sent, "ELIGIBLESOURCESENTINEL") {
		t.Fatal("eligible lower-ranked source starved behind out-of-window hits before cap")
	}
	if strings.Contains(sent, "OUTSIDEWINDOW") {
		t.Fatal("out-of-window source reached provider")
	}
	if len(a.SourceCitations) != 1 {
		t.Fatalf("eligible source not cited: %+v", a)
	}
}
