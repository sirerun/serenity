package main

import (
	"os"
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
