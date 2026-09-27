package secrets

import (
	"os"
	"strings"
	"testing"
)

// TestThreatModelDescribesProviderKeysAsEnvironment keeps
// docs/threat-model.md honest about where key material lives (SEC-L08):
// model provider keys are read from the process environment, not the OS
// keychain, and the keychain item's access control list is documented.
func TestThreatModelDescribesProviderKeysAsEnvironment(t *testing.T) {
	b, err := os.ReadFile("../../docs/threat-model.md")
	if err != nil {
		t.Fatalf("read threat model: %v", err)
	}
	doc := string(b)
	for _, want := range []string{
		"OPENROUTER_API_KEY",
		"ANTHROPIC_API_KEY",
		"OPENAI_API_KEY",
		"environment variable",
		"access control list",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("docs/threat-model.md does not mention %q", want)
		}
	}
	for _, stale := range []string{
		"OS keychain: model API keys",
		"Model API keys and connector OAuth tokens live in the OS keychain",
		"cloud model provider keys, connector OAuth",
	} {
		if strings.Contains(doc, stale) {
			t.Errorf("docs/threat-model.md still claims provider keys live in the keychain: %q", stale)
		}
	}
}
