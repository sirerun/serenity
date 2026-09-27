package providers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/router"
)

// TestBuildRoutersApplyConfiguredRedactPatterns proves serenity.yml's
// `redact.patterns` reaches the router chokepoint through every Build*
// function: a configured pattern and a built-in key shape are both
// masked in the request body a real adapter sends. The provider is the
// substring-inferred OpenAI-compatible adapter pointed at a local test
// server, so no network egress and no real credential is involved.
func TestBuildRoutersApplyConfiguredRedactPatterns(t *testing.T) {
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("server: read body: %v", err)
		}
		bodies = append(bodies, string(body))
		w.Header().Set("content-type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/embeddings") {
			_, _ = w.Write([]byte(`{"data":[{"embedding":[0.5,-0.25,1]}],"usage":{"prompt_tokens":1,"total_tokens":1}}`))
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer server.Close()

	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", server.URL+"/v1")
	t.Setenv("OPENAI_EMBEDDINGS_BASE_URL", server.URL+"/v1")

	cfg := &config.Config{
		Models: config.Models{
			Provider:   "openai",
			Extraction: "local-model@v1",
			Composer:   "local-model@v1",
			Embedding:  "local-embed@v1",
		},
		Redact: config.Redact{Patterns: []config.RedactPattern{{Name: "employee_id", Regex: `EMP-[0-9]{6}`}}},
	}

	fakeAWSKey := "AKIATEST" + strings.Repeat("0", 12)
	prompt := "Badge EMP-123456 with key " + fakeAWSKey

	builders := []struct {
		name  string
		build func(*config.Config, router.SpendLedger) (*router.Router, bool, string)
		class router.TaskClass
	}{
		{"extraction", BuildExtractionRouter, router.TaskClassExtractionCandidates},
		{"composer", BuildComposerRouter, router.TaskClassComposerSynthesis},
		{"embedding", BuildEmbeddingRouter, router.TaskClassEmbedding},
	}
	for _, b := range builders {
		t.Run(b.name, func(t *testing.T) {
			bodies = nil
			r, ok, note := b.build(cfg, &fakeLedger{})
			if !ok {
				t.Fatalf("%s router not built: %s", b.name, note)
			}
			if _, err := r.Complete(context.Background(), b.class, router.Prompt{Text: prompt}, router.Budget{}); err != nil {
				t.Fatalf("Complete: %v", err)
			}
			if len(bodies) != 1 {
				t.Fatalf("server recorded %d bodies, want 1", len(bodies))
			}
			body := bodies[0]
			if strings.Contains(body, "EMP-123456") || strings.Contains(body, fakeAWSKey) {
				t.Fatalf("%s request body leaked a sensitive value:\n%s", b.name, body)
			}
			if !strings.Contains(body, "[REDACTED:EMPLOYEE_ID]") || !strings.Contains(body, "[REDACTED:API_KEY]") {
				t.Fatalf("%s request body is missing a placeholder:\n%s", b.name, body)
			}
		})
	}
}

// TestBuildRouterRefusesInvalidRedactPattern: a config assembled
// in-process (bypassing Load's validation) with a broken regex must not
// yield a router that silently drops the rule; Build* returns not-ok
// with a note naming the pattern.
func TestBuildRouterRefusesInvalidRedactPattern(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	cfg := &config.Config{
		Models: config.Models{Provider: "openrouter", Composer: "vendor/model@v1"},
		Redact: config.Redact{Patterns: []config.RedactPattern{{Name: "broken_rule", Regex: "("}}},
	}
	r, ok, note := BuildComposerRouter(cfg, &fakeLedger{})
	if ok || r != nil {
		t.Fatalf("BuildComposerRouter built a router over an invalid redact pattern (note %q)", note)
	}
	if !strings.Contains(note, "broken_rule") {
		t.Fatalf("skip note does not name the invalid pattern: %q", note)
	}
}
