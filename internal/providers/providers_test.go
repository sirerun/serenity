package providers

import (
	"context"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/router"
)

// fakeLedger is a test double implementing router.SpendLedger. Test-file
// only, per the zero-stub policy -- none of these tests ever call
// Router.Complete, so it never needs to actually record anything.
type fakeLedger struct{}

func (f *fakeLedger) Record(_ context.Context, _ router.SpendEntry) error { return nil }

// TestBuildExtractionRouterOpenRouterProvider asserts that
// models.provider: openrouter routes an extraction pin through
// router.OpenAICompatibleProvider pointed at OpenRouter's base URL (ADR
// 013), rather than the pre-existing "claude" substring inference --
// OpenRouter's vendor-prefixed ids (e.g. "anthropic/claude-3-5-sonnet")
// would otherwise silently misroute to AnthropicProvider.
func TestBuildExtractionRouterOpenRouterProvider(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")

	cfg := &config.Config{
		Models: config.Models{
			Provider:   "openrouter",
			Extraction: "anthropic/claude-3-5-sonnet@20241022",
		},
	}

	r, ok, note := BuildExtractionRouter(cfg, &fakeLedger{})
	if !ok {
		t.Fatalf("BuildExtractionRouter ok = false, want true (note: %s)", note)
	}

	p, ok := r.Provider(router.TierLocalCheap)
	if !ok {
		t.Fatal("router has no provider registered for TierLocalCheap")
	}
	oc, ok := p.(*router.OpenAICompatibleProvider)
	if !ok {
		t.Fatalf("provider type = %T, want *router.OpenAICompatibleProvider", p)
	}
	if oc.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("BaseURL = %q, want %q", oc.BaseURL, "https://openrouter.ai/api/v1")
	}
}

// TestBuildComposerRouterOpenRouterProvider mirrors
// TestBuildExtractionRouterOpenRouterProvider for BuildComposerRouter,
// which resolves to router.TierJudgment rather than TierLocalCheap.
func TestBuildComposerRouterOpenRouterProvider(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-test")

	cfg := &config.Config{
		Models: config.Models{
			Provider: "openrouter",
			Composer: "anthropic/claude-3-5-sonnet@20241022",
		},
	}

	r, ok, note := BuildComposerRouter(cfg, &fakeLedger{})
	if !ok {
		t.Fatalf("BuildComposerRouter ok = false, want true (note: %s)", note)
	}

	p, ok := r.Provider(router.TierJudgment)
	if !ok {
		t.Fatal("router has no provider registered for TierJudgment")
	}
	oc, ok := p.(*router.OpenAICompatibleProvider)
	if !ok {
		t.Fatalf("provider type = %T, want *router.OpenAICompatibleProvider", p)
	}
	if oc.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("BaseURL = %q, want %q", oc.BaseURL, "https://openrouter.ai/api/v1")
	}
}

// TestBuildExtractionRouterSubstringInferenceUnchanged pins down that the
// pre-existing "claude" substring inference is byte-identical behavior
// when models.provider is empty, and that provider "anthropic" only forces
// AnthropicProvider when the pin also names a claude model -- it does not
// override substring inference for a non-claude-named pin: rows c/d prove
// substring inference alone governs whenever provider isn't "openrouter".
func TestBuildExtractionRouterSubstringInferenceUnchanged(t *testing.T) {
	cases := []struct {
		name        string
		provider    string
		pin         string
		envKey      string
		envValue    string
		wantOpenAI  bool
		wantAnthrop bool
	}{
		{
			name:        "explicit anthropic provider with claude-named pin",
			provider:    "anthropic",
			pin:         "claude-3-5-sonnet@20241022",
			envKey:      "ANTHROPIC_API_KEY",
			envValue:    "sk-ant-test",
			wantAnthrop: true,
		},
		{
			name:        "empty provider with claude-named pin infers anthropic",
			provider:    "",
			pin:         "claude-3-5-sonnet@20241022",
			envKey:      "ANTHROPIC_API_KEY",
			envValue:    "sk-ant-test",
			wantAnthrop: true,
		},
		{
			name:       "empty provider with non-claude pin infers openai-compatible",
			provider:   "",
			pin:        "gpt-4o-mini@2024-07-18",
			envKey:     "OPENAI_API_KEY",
			envValue:   "sk-openai-test",
			wantOpenAI: true,
		},
		{
			name:       "anthropic provider with non-claude pin still substring-infers openai-compatible",
			provider:   "anthropic",
			pin:        "gpt-4o-mini@2024-07-18",
			envKey:     "OPENAI_API_KEY",
			envValue:   "sk-openai-test",
			wantOpenAI: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.envKey, tc.envValue)

			cfg := &config.Config{
				Models: config.Models{
					Provider:   tc.provider,
					Extraction: tc.pin,
				},
			}

			r, ok, note := BuildExtractionRouter(cfg, &fakeLedger{})
			if !ok {
				t.Fatalf("BuildExtractionRouter ok = false, want true (note: %s)", note)
			}
			p, ok := r.Provider(router.TierLocalCheap)
			if !ok {
				t.Fatal("router has no provider registered for TierLocalCheap")
			}

			switch {
			case tc.wantAnthrop:
				if _, ok := p.(*router.AnthropicProvider); !ok {
					t.Fatalf("provider type = %T, want *router.AnthropicProvider", p)
				}
			case tc.wantOpenAI:
				if _, ok := p.(*router.OpenAICompatibleProvider); !ok {
					t.Fatalf("provider type = %T, want *router.OpenAICompatibleProvider", p)
				}
			}
		})
	}
}

// TestBuildExtractionRouterOpenRouterMissingKey asserts the explicit-skip
// contract: models.provider: openrouter with no OPENROUTER_API_KEY set
// returns ok=false and a note naming the missing env var, never a crash
// or a silent fallback to another provider.
func TestBuildExtractionRouterOpenRouterMissingKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")

	cfg := &config.Config{
		Models: config.Models{
			Provider:   "openrouter",
			Extraction: "anthropic/claude-3-5-sonnet@20241022",
		},
	}

	_, ok, note := BuildExtractionRouter(cfg, &fakeLedger{})
	if ok {
		t.Fatal("BuildExtractionRouter ok = true, want false when OPENROUTER_API_KEY is unset")
	}
	if note == "" {
		t.Fatal("note is empty, want a human-readable reason naming OPENROUTER_API_KEY")
	}
	if !strings.Contains(note, "OPENROUTER_API_KEY") {
		t.Fatalf("note = %q, want it to mention OPENROUTER_API_KEY", note)
	}
}

// TestBuildEmbeddingRouterIgnoresProvider asserts BuildEmbeddingRouter
// never reads cfg.Models.Provider: OpenRouter has no embeddings endpoint,
// so an OPENROUTER_API_KEY must never satisfy the embeddings credential
// check, and models.provider: openrouter must not change embeddings
// behavior at all once OPENAI_API_KEY is present.
func TestBuildEmbeddingRouterIgnoresProvider(t *testing.T) {
	t.Run("openrouter key alone does not satisfy embeddings", func(t *testing.T) {
		t.Setenv("OPENAI_API_KEY", "")
		t.Setenv("OPENAI_EMBEDDINGS_BASE_URL", "")
		t.Setenv("OPENROUTER_API_KEY", "sk-or-test")

		cfg := &config.Config{
			Models: config.Models{
				Provider:  "openrouter",
				Embedding: "text-embedding-3-small@2024-01-25",
			},
		}

		_, ok, note := BuildEmbeddingRouter(cfg, &fakeLedger{})
		if ok {
			t.Fatalf("BuildEmbeddingRouter ok = true, want false -- OPENROUTER_API_KEY must never satisfy embeddings (note: %s)", note)
		}
	})

	t.Run("openai key still works with provider set to openrouter", func(t *testing.T) {
		t.Setenv("OPENAI_API_KEY", "sk-openai-test")
		t.Setenv("OPENAI_EMBEDDINGS_BASE_URL", "")
		t.Setenv("OPENROUTER_API_KEY", "sk-or-test")

		cfg := &config.Config{
			Models: config.Models{
				Provider:  "openrouter",
				Embedding: "text-embedding-3-small@2024-01-25",
			},
		}

		_, ok, note := BuildEmbeddingRouter(cfg, &fakeLedger{})
		if !ok {
			t.Fatalf("BuildEmbeddingRouter ok = false, want true (note: %s)", note)
		}
	})
}
