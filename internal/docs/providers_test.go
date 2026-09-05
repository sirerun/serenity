// This file is the file-first CI gate for docs/providers.md (plan T1.25):
// the document is data, and this test suite keeps it honest rather than
// trusting convention, the same approach threat_model_test.go takes for
// docs/threat-model.md.
package docs

import (
	"os"
	"strings"
	"testing"
)

// providersPath is relative to this package directory (go test's working
// directory), two levels up from internal/docs to the repo root.
const providersPath = "../../docs/providers.md"

func readProvidersDoc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(providersPath)
	if err != nil {
		t.Fatalf("docs/providers.md must exist and be readable: %v", err)
	}
	if strings.TrimSpace(string(b)) == "" {
		t.Fatal("docs/providers.md exists but is empty")
	}
	return string(b)
}

// TestProvidersDocExists asserts the doc exists and is non-empty.
func TestProvidersDocExists(t *testing.T) {
	readProvidersDoc(t)
}

// TestProvidersDocMentionsRequiredTopics asserts docs/providers.md covers
// the topics plan T1.25's acc line requires: OPENROUTER_API_KEY (the exact
// env var name), that OpenRouter is the default provider for new brains,
// how to override to anthropic/openai, and the caveat that embeddings
// never route through OpenRouter.
func TestProvidersDocMentionsRequiredTopics(t *testing.T) {
	content := readProvidersDoc(t)
	lower := strings.ToLower(content)

	if !strings.Contains(content, "OPENROUTER_API_KEY") {
		t.Error("docs/providers.md must mention the exact env var name OPENROUTER_API_KEY")
	}

	if !strings.Contains(lower, "default") || !strings.Contains(lower, "openrouter") {
		t.Error("docs/providers.md must document that OpenRouter is the default provider for new brains")
	}

	if !strings.Contains(lower, "anthropic") || !strings.Contains(lower, "openai") {
		t.Error("docs/providers.md must document overriding models.provider to anthropic or openai")
	}
	if !strings.Contains(lower, "models.provider") {
		t.Error("docs/providers.md must name the models.provider field for the override")
	}

	if !strings.Contains(lower, "no embeddings endpoint") && !strings.Contains(lower, "never route") && !strings.Contains(lower, "never routed") && !strings.Contains(lower, "never resolves") && !strings.Contains(lower, "always resolves") {
		t.Error("docs/providers.md must document that embeddings are not supported on OpenRouter and always resolve via the OpenAI-shaped adapter")
	}
	if !strings.Contains(lower, "embedding") {
		t.Error("docs/providers.md must mention embeddings at all")
	}
}
