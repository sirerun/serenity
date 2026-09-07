package main

import (
	"os"
	"reflect"
	"testing"

	"github.com/sirerun/serenity/internal/router"
)

// TestBuildProviderOpenAIHonorsBaseURLEnv guards the real gap found running
// T1.23's live eval against the DGX Qwen endpoint: buildProvider("openai",
// ...) previously never read OPENAI_BASE_URL, so -mode live -provider openai
// always targeted the real OpenAI API regardless of any local server
// configured via the env var -- silently ignoring a pointed-at local
// endpoint instead of erroring or using it.
func TestBuildProviderOpenAIHonorsBaseURLEnv(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "http://192.0.2.1:30000/v1")
	t.Setenv("OPENAI_API_KEY", "")

	p, _, err := buildProvider("openai", "qwen3.8-27b", "v1")
	if err != nil {
		t.Fatalf("buildProvider: %v", err)
	}
	oc, ok := p.(*router.OpenAICompatibleProvider)
	if !ok {
		t.Fatalf("buildProvider(\"openai\", ...) returned %T, want *router.OpenAICompatibleProvider", p)
	}
	if oc.BaseURL != "http://192.0.2.1:30000/v1" {
		t.Fatalf("BaseURL = %q, want the OPENAI_BASE_URL env value", oc.BaseURL)
	}
}

// TestBuildProviderOpenAIDefaultsBaseURLWhenUnset locks in the unchanged
// case: no OPENAI_BASE_URL means BaseURL stays empty, so
// OpenAICompatibleProvider's own default (the real OpenAI API) still
// applies -- this fix must not force a BaseURL where none was configured.
func TestBuildProviderOpenAIDefaultsBaseURLWhenUnset(t *testing.T) {
	if err := os.Unsetenv("OPENAI_BASE_URL"); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", "sk-test")

	p, _, err := buildProvider("openai", "gpt-x", "v1")
	if err != nil {
		t.Fatalf("buildProvider: %v", err)
	}
	oc := p.(*router.OpenAICompatibleProvider)
	if oc.BaseURL != "" {
		t.Fatalf("BaseURL = %q, want empty when OPENAI_BASE_URL is unset", oc.BaseURL)
	}
}

// TestBuildProviderOpenAIPinsTemperatureZero is T1.29's own follow-up
// finding: an unset (server-default, non-zero) temperature makes
// eval-runner's -mode live scoring non-reproducible run to run --
// individual held-out spans flip outcome between otherwise-identical
// runs. Pinning temperature=0 fixes that reproducibility gap; it did NOT,
// under a controlled same-conditions comparison, change which families
// clear the P>=0.90/R>=0.80 bar (see docs/lore.md L-0011 for the full
// finding, including a since-corrected overclaim in an earlier draft).
// "temperature" is a standard OpenAI chat-completions field, safe to send
// to any OpenAI-compatible endpoint unconditionally (unlike
// disable_thinking's server-specific chat_template_kwargs, which stays
// opt-in).
func TestBuildProviderOpenAIPinsTemperatureZero(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-test")

	p, _, err := buildProvider("openai", "qwen3.8-27b", "v1")
	if err != nil {
		t.Fatalf("buildProvider: %v", err)
	}
	oc := p.(*router.OpenAICompatibleProvider)
	want := map[string]any{"temperature": 0}
	if !reflect.DeepEqual(oc.ExtraBody, want) {
		t.Fatalf("ExtraBody = %#v, want %#v", oc.ExtraBody, want)
	}
}
