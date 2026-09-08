package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/store"
)

func TestExtractLowConfidenceRetainedAndExplicitlyReviewed(t *testing.T) {
	for _, tc := range []struct {
		name, keys, family string
		prior, assertion   bool
		state              disposition.State
	}{
		{"space retains evidence", " ", "works_at", false, false, disposition.StatePending},
		{"defer", "d", "works_at", false, false, disposition.StateDeferred},
		{"reject", "runsupported evidence\n", "works_at", false, false, disposition.StateDisposed},
		{"cancel human assertion", "eHuman Company\nn\n", "works_at", false, false, disposition.StatePending},
		{"empty human assertion", "e\n", "works_at", false, false, disposition.StatePending},
		{"assert new fence claim", "eHuman Company\ny\n", "works_at", false, true, disposition.StateDisposed},
		{"assert replacement", "eHuman Company\ny\n", "works_at", true, true, disposition.StateDisposed},
		{"assert new shard claim", "eHuman Company\ny\n", "has_balance", false, true, disposition.StateDisposed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			root := initBrainRepo(t)
			configureGitIdentity(t, root)
			var confidence atomic.Int64
			confidence.Store(2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				payload := fmt.Sprintf(`{"observations":[{"subject":"acme","predicate":%q,"object":"Uncertain Company","confidence":%.1f}]}`, tc.family, float64(confidence.Load())/10)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": payload}}}})
			}))
			defer server.Close()
			t.Setenv("OPENAI_BASE_URL", server.URL)
			for _, key := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "OPENROUTER_API_KEY"} {
				t.Setenv(key, "")
			}
			cfg, err := config.Load(filepath.Join(root, config.FileName))
			if err != nil {
				t.Fatal(err)
			}
			cfg.Models.Provider = ""
			cfg.Models.Extraction = "test-extract@v1"
			cfg.Models.Embedding = "none@v0"
			if err := cfg.Save(filepath.Join(root, config.FileName)); err != nil {
				t.Fatal(err)
			}
			fw := store.NewFenceWriter(root)
			if tc.prior {
				page := store.NewEntityPage(domain.Entity{Type: "person", Slug: "acme"})
				page.Claims = []domain.Claim{{ID: "prior", SubjectSlug: "acme", Predicate: tc.family, Family: tc.family, Object: "Old Company", Confidence: .9, State: domain.StateActive}}
				path, err := fw.WriteEntity(page)
				if err != nil {
					t.Fatal(err)
				}
				f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.WriteString("\nHuman explanation must remain.\n"); err != nil {
					t.Fatal(err)
				}
				if err := f.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.NewSourceStore(root).Write([]byte("Uncertain source suggests an association."), domain.Source{Kind: "file", URI: "fixture:uncertain", OccurredAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{{"add", "."}, {"commit", "--quiet", "-m", "seed uncertain source"}} {
				cmd := exec.Command("git", args...)
				cmd.Dir = root
				if raw, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("fixture: %v %s", err, raw)
				}
			}
			eng, err := providers.OpenIndex(root)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = eng.Close() }()
			ds := disposition.NewStore(eng)
			extract := func() {
				t.Helper()
				var out bytes.Buffer
				if err := runExtract(ctx, root, &out); err != nil {
					t.Fatalf("extract: %v %s", err, out.String())
				}
			}
			extract()
			extract()
			items, err := ds.List(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 1 || items[0].Kind != disposition.KindDistill {
				t.Fatalf("evidence missing or duplicated: %+v", items)
			}
			before := append([]byte(nil), items[0].Payload...)
			original, ok, err := disposition.ExtractionObservation(items[0])
			if err != nil || !ok || original.Confidence != .2 || original.SourceSHA256 == "" || original.Span == "" || original.Model != "test-extract@v1" {
				t.Fatalf("original provenance: %+v %v", original, err)
			}
			var out bytes.Buffer
			if err := runInbox(ctx, root, strings.NewReader(tc.keys), &out, inboxOptions{}, time.Now()); err != nil {
				t.Fatalf("inbox: %v %s", err, out.String())
			}
			if tc.prior && !strings.Contains(out.String(), `supersede canonical claim prior: "Old Company"`) {
				t.Fatalf("replacement not disclosed before confirmation: %s", out.String())
			}
			extract()
			confidence.Store(9)
			extract() // Higher model confidence cannot bypass retained human review.
			items, err = ds.List(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 1 || items[0].State != tc.state || !bytes.Equal(before, items[0].Payload) {
				t.Fatalf("original evidence/state replaced: %+v", items)
			}
			var claims []domain.Claim
			if tc.family == "has_balance" {
				heads, readErr := store.NewShardStore(root).ResolveHeads("acme", tc.family)
				if readErr != nil {
					t.Fatal(readErr)
				}
				for _, claim := range heads {
					claims = append(claims, claim)
				}
			} else {
				paths, err := filepath.Glob(filepath.Join(root, "brain/entities/*/acme.md"))
				if err != nil {
					t.Fatal(err)
				}
				for _, path := range paths {
					page, err := fw.ParseEntity(path)
					if err != nil {
						t.Fatal(err)
					}
					for _, c := range page.Claims {
						if c.State == domain.StateActive && c.SupersededBy == "" {
							claims = append(claims, c)
						}
					}
					if tc.prior {
						raw, err := os.ReadFile(path)
						if err != nil || !strings.Contains(string(raw), "Human explanation must remain.") {
							t.Fatalf("human prose lost: %v", err)
						}
					}
				}
			}
			if tc.assertion {
				if len(claims) != 1 || claims[0].Object != "Human Company" || claims[0].Confidence != 1 || !strings.HasPrefix(claims[0].Provenance.Actor, "human:") || claims[0].Provenance.SourceSHA256 != "" || claims[0].Provenance.Model != "" || claims[0].Provenance.Meta["distill_item_id"] != items[0].ID || items[0].AppliedClaimID != claims[0].ID {
					t.Fatalf("incorrect human effect: %+v item=%+v", claims, items[0])
				}
				if items[0].Verdict != disposition.VerdictEditAccept {
					t.Fatal("explicit assertion not recorded")
				}
			} else if len(claims) != 0 || items[0].AppliedClaimID != "" {
				t.Fatalf("low confidence silently promoted: %+v", claims)
			}
			history, err := ds.HistoryFor(ctx, items[0].ID)
			if err != nil {
				t.Fatal(err)
			}
			expectedHistory := 0
			if tc.state != disposition.StatePending {
				expectedHistory = 1
			}
			if len(history) != expectedHistory {
				t.Fatalf("history count=%d want=%d", len(history), expectedHistory)
			}
			cmd := exec.Command("git", "status", "--porcelain")
			cmd.Dir = root
			if raw, err := cmd.CombinedOutput(); err != nil || len(raw) != 0 {
				t.Fatalf("uncommitted effect: %v %s", err, raw)
			}
		})
	}
}
