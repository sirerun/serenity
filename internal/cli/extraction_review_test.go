package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/reconcile"
	"github.com/sirerun/serenity/internal/store"
)

// Exercise the real provider HTTP adapter, extraction command, persisted queue,
// interactive inbox, canonical publication and repeated extraction together.
func TestExtractConflictInboxDecisionSurvivesRepeat(t *testing.T) {
	for _, tc := range []struct{ name, keys, want string }{
		{"accept", " ", "Acme Corp"},
		{"edit", "eHuman Company\n", "Human Company"},
		{"reject", "rnot supported\n", "Old Company"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			root := initBrainRepo(t)
			configureGitIdentity(t, root)
			server := fakeExtractionServer(t)
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
			page := store.NewEntityPage(domain.Entity{Type: "person", Slug: "acme"})
			page.Claims = []domain.Claim{{ID: "prior-claim", SubjectSlug: "acme", Predicate: "works_at", Family: "works_at", Object: "Old Company", State: domain.StateActive, Confidence: .9}, {ID: "unrelated", SubjectSlug: "acme", Predicate: "prefers", Family: "prefers", Object: "Gardening", State: domain.StateActive, Confidence: .9}}
			path, err := fw.WriteEntity(page)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.NewSourceStore(root).Write([]byte("Acme works at Acme Corp."), domain.Source{Kind: "file", URI: "fixture:conflict", OccurredAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
			for _, args := range [][]string{{"add", "."}, {"commit", "--quiet", "-m", "seed canonical conflict"}} {
				cmd := exec.Command("git", args...)
				cmd.Dir = root
				if raw, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git: %v %s", err, raw)
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
			if len(items) != 1 || items[0].Kind != disposition.KindReconcile || items[0].State != disposition.StatePending {
				t.Fatalf("reviews: %+v", items)
			}
			var proposal reconcile.ReconcilePayload
			if err := json.Unmarshal(items[0].Payload, &proposal); err != nil {
				t.Fatal(err)
			}
			if proposal.A.Provenance.SourceSHA256 == "" || proposal.A.Object != "Acme Corp" || proposal.B.ID != "prior-claim" {
				t.Fatalf("proposal: %+v", proposal)
			}
			page, err = fw.ParseEntity(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Claims) != 2 {
				t.Fatalf("proposal became canonical before approval: %+v", page.Claims)
			}
			var inboxOut bytes.Buffer
			if err := runInbox(ctx, root, strings.NewReader(tc.keys), &inboxOut, inboxOptions{}, time.Now()); err != nil {
				t.Fatalf("inbox: %v %s", err, inboxOut.String())
			}
			extract()
			extract()
			page, err = fw.ParseEntity(path)
			if err != nil {
				t.Fatal(err)
			}
			var current []string
			unrelated := false
			for _, c := range page.Claims {
				if c.ID == "unrelated" && c.Object == "Gardening" {
					unrelated = true
				}
				if c.Family == "works_at" && c.State == domain.StateActive && c.SupersededBy == "" {
					current = append(current, c.Object)
				}
			}
			if len(current) != 1 || current[0] != tc.want || !unrelated {
				t.Fatalf("canonical state after repeated extraction: current=%v unrelated=%v claims=%+v", current, unrelated, page.Claims)
			}
			items, err = ds.List(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 1 || items[0].State != disposition.StateDisposed {
				t.Fatalf("human decision replaced or duplicated: %+v", items)
			}
			history, err := ds.HistoryFor(ctx, items[0].ID)
			if err != nil || len(history) != 1 {
				t.Fatalf("decision history: %v %+v", err, history)
			}
			if tc.name != "reject" && items[0].AppliedClaimID == "" {
				t.Fatal("accepted effect not marked applied")
			}
			cmd := exec.Command("git", "status", "--porcelain")
			cmd.Dir = root
			if raw, err := cmd.CombinedOutput(); err != nil || len(raw) != 0 {
				t.Fatalf("uncommitted output: %v %s", err, raw)
			}
		})
	}
}
