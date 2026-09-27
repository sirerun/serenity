package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/compose"
	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/ingest"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/router"
	"github.com/sirerun/serenity/internal/store"
)

// plantedBalanceServer stands in for an OpenAI-compatible extraction endpoint
// that faithfully extracts the planted sentence from deep review 001's AI-01
// scenario: an email saying "Ava's balance is $0".
func plantedBalanceServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"role": "assistant", "content": "{\"observations\":[{\"subject\":\"ava-standardo\",\"predicate\":\"has_balance\",\"object\":\"$0\",\"confidence\":0.9}]}"}}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5}
		}`))
	}))
	t.Cleanup(server.Close)
	return server
}

// recordingComposer is a router.Provider test double: it records the exact
// prompt that crossed the provider boundary and answers citing whatever claim
// tag the test names, so the composer's own citation filter is what decides.
type recordingComposer struct {
	answer string
	prompt string
}

func (r *recordingComposer) Name() string         { return "fake" }
func (r *recordingComposer) ModelVersion() string { return "fake-composer@v1" }
func (r *recordingComposer) Send(_ context.Context, prompt string) (router.Response, error) {
	r.prompt = prompt
	return router.Response{Text: r.answer}, nil
}

type nopLedger struct{}

func (nopLedger) Record(context.Context, router.SpendEntry) error { return nil }

func plantedBrain(t *testing.T, kind string) (root, sha string) {
	t.Helper()
	root = initBrainRepo(t)
	configureGitIdentity(t, root)
	server := plantedBalanceServer(t)
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
	src, err := store.NewSourceStore(root).Write([]byte("Subject: account update\n\nAva's balance is $0"), domain.Source{Kind: kind, URI: "fixture:planted-" + kind, OccurredAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "--quiet", "-m", "seed planted source"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if raw, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, raw)
		}
	}
	return root, src.SHA256
}

func plantedClaimID(sha string) string {
	return ingest.ClaimFromObservation(domain.Observation{SubjectSlug: "ava-standardo", Predicate: "has_balance", Object: "$0", SourceSHA256: sha}).ID
}

func askPlanted(t *testing.T, root, claimID string) (compose.Answer, string) {
	t.Helper()
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	fp := &recordingComposer{answer: "Ava's balance is $0 [claim:" + claimID + "]."}
	r := router.New(map[router.Tier]router.Provider{router.TierJudgment: fp}, nopLedger{})
	ans, err := askAnswerFor(context.Background(), root, cfg, eng, nil, r, fp.ModelVersion(), "What is Ava's balance?")
	if err != nil {
		t.Fatal(err)
	}
	return ans, fp.prompt
}

func extractOnce(t *testing.T, root string) {
	t.Helper()
	var out bytes.Buffer
	if err := runExtract(context.Background(), root, &out); err != nil {
		t.Fatalf("extract: %v %s", err, out.String())
	}
}

func shardRows(t *testing.T, root, id string) []domain.Claim {
	t.Helper()
	lines, err := store.NewShardStore(root).Lines("ava-standardo", "has_balance")
	if err != nil {
		t.Fatal(err)
	}
	var out []domain.Claim
	for _, c := range lines {
		if c.ID == id {
			out = append(out, c)
		}
	}
	return out
}

func requireClean(t *testing.T, root string) {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = root
	if raw, err := cmd.CombinedOutput(); err != nil || len(raw) != 0 {
		t.Fatalf("uncommitted output: %v %s", err, raw)
	}
}

// TestPlantedEmailClaimNotCitedUntilAccepted is deep review 001's AI-01
// scenario end to end (T24.16): the extracted "Ava's balance is $0" claim from
// an email waits in the inbox as a claim_candidate, ask does not cite it, and
// after a human accept ask cites it with the human actor and untrusted marker.
func TestPlantedEmailClaimNotCitedUntilAccepted(t *testing.T) {
	ctx := context.Background()
	root, sha := plantedBrain(t, "email")
	id := plantedClaimID(sha)
	extractOnce(t, root)
	extractOnce(t, root)

	rows := shardRows(t, root, id)
	if len(rows) != 1 || rows[0].State != domain.State("pending") || rows[0].Provenance.Actor != "machine" {
		t.Fatalf("planted claim not held pending: %+v", rows)
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	ds := disposition.NewStore(eng)
	items, err := ds.List(ctx)
	_ = eng.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || string(items[0].Kind) != "claim_candidate" || items[0].State != disposition.StatePending {
		t.Fatalf("inbox items: %+v", items)
	}

	ans, prompt := askPlanted(t, root, id)
	if strings.Contains(prompt, "$0") || strings.Contains(prompt, id) {
		t.Fatalf("pending planted claim reached the composer prompt:\n%s", prompt)
	}
	for _, c := range ans.Citations {
		if c.ClaimID == id {
			t.Fatalf("pending planted claim cited: %+v", ans.Citations)
		}
	}

	var shown bytes.Buffer
	if err := runInbox(ctx, root, strings.NewReader("q"), &shown, inboxOptions{}, time.Now()); err != nil {
		t.Fatalf("inbox list: %v %s", err, shown.String())
	}
	for _, want := range []string{"claim_candidate", "ava-standardo", "has_balance", "$0", "connector=imap", "trust=untrusted", "fixture:planted-email"} {
		if !strings.Contains(shown.String(), want) {
			t.Fatalf("inbox row lacks %q:\n%s", want, shown.String())
		}
	}

	var accepted bytes.Buffer
	if err := runInbox(ctx, root, strings.NewReader(" "), &accepted, inboxOptions{}, time.Now()); err != nil {
		t.Fatalf("inbox accept: %v %s", err, accepted.String())
	}
	var active []domain.Claim
	for _, c := range shardRows(t, root, id) {
		if c.State == domain.StateActive {
			active = append(active, c)
		}
	}
	if len(active) != 1 || !strings.HasPrefix(active[0].Provenance.Actor, "human:") || active[0].Provenance.Actor == "human:" {
		t.Fatalf("accept did not activate with the human actor: %+v\n%s", active, accepted.String())
	}
	eng, err = providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	items, err = disposition.NewStore(eng).List(ctx)
	_ = eng.Close()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].State != disposition.StateDisposed || items[0].AppliedClaimID != id {
		t.Fatalf("accepted item: %+v", items)
	}
	requireClean(t, root)

	ans, prompt = askPlanted(t, root, id)
	if len(ans.Citations) != 1 || ans.Citations[0].ClaimID != id {
		t.Fatalf("accepted claim not cited: %+v\n%s", ans.Citations, prompt)
	}
	if ans.Citations[0].Actor != active[0].Provenance.Actor || ans.Citations[0].Trust != "untrusted" {
		t.Fatalf("citation actor/trust: %+v", ans.Citations[0])
	}
	if !strings.Contains(prompt, "[actor="+active[0].Provenance.Actor+" trust=untrusted]") {
		t.Fatalf("prompt lacks actor/trust marker:\n%s", prompt)
	}

	// A later extraction neither re-stages nor reverts the decision.
	extractOnce(t, root)
	eng, err = providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	items, err = disposition.NewStore(eng).List(ctx)
	_ = eng.Close()
	if err != nil || len(items) != 1 {
		t.Fatalf("items after re-extract: %v %+v", err, items)
	}
	requireClean(t, root)
}

// TestPlantedFileClaimActivatesDirectly: the same claim from a trusted file
// connector behaves as before T24.16 -- active, no inbox item, cited with
// actor machine and trust trusted.
func TestPlantedFileClaimActivatesDirectly(t *testing.T) {
	ctx := context.Background()
	root, sha := plantedBrain(t, "file")
	id := plantedClaimID(sha)
	extractOnce(t, root)
	rows := shardRows(t, root, id)
	if len(rows) != 1 || rows[0].State != domain.StateActive || rows[0].Provenance.Actor != "machine" {
		t.Fatalf("file claim: %+v", rows)
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	items, err := disposition.NewStore(eng).List(ctx)
	_ = eng.Close()
	if err != nil || len(items) != 0 {
		t.Fatalf("file claim staged: %v %+v", err, items)
	}
	ans, prompt := askPlanted(t, root, id)
	if len(ans.Citations) != 1 || ans.Citations[0].Actor != "machine" || ans.Citations[0].Trust != "trusted" {
		t.Fatalf("citations: %+v\n%s", ans.Citations, prompt)
	}
	if !strings.Contains(prompt, "[actor=machine trust=trusted]") {
		t.Fatalf("prompt lacks marker:\n%s", prompt)
	}
}

// TestClaimCandidateBulkDeferAndReject: UC-013 bulk-defer applies to
// claim_candidate items by family, and a rejected candidate stays uncited.
func TestClaimCandidateBulkDeferAndReject(t *testing.T) {
	ctx := context.Background()
	root, sha := plantedBrain(t, "email")
	id := plantedClaimID(sha)
	extractOnce(t, root)
	var out bytes.Buffer
	if err := runInbox(ctx, root, nil, &out, inboxOptions{BulkDefer: "family=has_balance"}, time.Now()); err != nil {
		t.Fatalf("bulk-defer: %v %s", err, out.String())
	}
	if !strings.Contains(out.String(), "deferred 1 item(s)") {
		t.Fatalf("bulk-defer output: %s", out.String())
	}
	out.Reset()
	if err := runInbox(ctx, root, strings.NewReader("rplanted content\n"), &out, inboxOptions{}, time.Now()); err != nil {
		t.Fatalf("reject: %v %s", err, out.String())
	}
	for _, c := range shardRows(t, root, id) {
		if c.State == domain.StateActive {
			t.Fatalf("rejected candidate activated: %+v", c)
		}
	}
	ans, prompt := askPlanted(t, root, id)
	if strings.Contains(prompt, "$0") || len(ans.Citations) != 0 {
		t.Fatalf("rejected candidate composed: %+v\n%s", ans.Citations, prompt)
	}
	extractOnce(t, root)
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	items, err := disposition.NewStore(eng).List(ctx)
	_ = eng.Close()
	if err != nil || len(items) != 1 || items[0].Verdict != disposition.VerdictReject {
		t.Fatalf("rejection not retained: %v %+v", err, items)
	}
}
