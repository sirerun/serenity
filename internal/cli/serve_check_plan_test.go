package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/secrets"
)

// TestServeHTTPCheckPlanUsesConfiguredRouter proves `serve --http` builds
// DIRECTION's check_plan with a live router (FUN-06): a free-text plan
// posted to /direction/check_plan reaches the brain's pinned local-cheap
// chat model (a recording OpenAI-compatible endpoint here) instead of
// short-circuiting to unverified on a nil router.
func TestServeHTTPCheckPlanUsesConfiguredRouter(t *testing.T) {
	requireGit(t)

	// The fake model classifies the plan as one in-set action whose
	// evidence is copied verbatim from the posted plan text, so the
	// classifier's fail-closed gate (empty action list, uncited evidence)
	// trusts it and the verdict comes from stage 1, not a refusal.
	var calls atomic.Int32
	var sawClassifyPrompt atomic.Bool
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "ship the release on friday") {
			sawClassifyPrompt.Store(true)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"role": "assistant", "content": "{\"confidence\":0.9,\"actions\":[{\"action\":\"deploy_to_prod\",\"params\":{},\"evidence\":\"ship the release on friday\"}]}"}}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5}
		}`))
	}))
	t.Cleanup(model.Close)
	t.Setenv("OPENAI_BASE_URL", model.URL)
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")

	root := pushFixture(t)
	cfgPath := filepath.Join(root, config.FileName)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Models.Provider = ""
	cfg.Models.Extraction = "test-classify@v1"
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatal(err)
	}

	token, err := secrets.DaemonToken()
	if err != nil {
		t.Fatalf("daemon token: %v", err)
	}
	endpoint, _, cancel, done := runningHTTPServe(t, root)
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("serve --http exited with error on shutdown: %v", err)
			}
		case <-time.After(8 * time.Second):
			t.Error("serve --http did not shut down")
		}
	}()

	url := strings.TrimSuffix(endpoint, "/mcp") + "/direction/check_plan"
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(`{"plan_text":"ship the release on friday"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("check_plan status = %d, body = %s", resp.StatusCode, raw)
	}
	var verdict struct {
		Status     string   `json:"status"`
		Confidence *float64 `json:"confidence"`
	}
	if err := json.Unmarshal(raw, &verdict); err != nil {
		t.Fatalf("decode verdict %s: %v", raw, err)
	}
	if calls.Load() == 0 || !sawClassifyPrompt.Load() {
		t.Fatalf("check_plan never called the configured router's provider (calls=%d); verdict = %s", calls.Load(), raw)
	}
	if verdict.Status == "unverified" {
		t.Fatalf("check_plan verdict = unverified with a configured router; body = %s", raw)
	}
}
