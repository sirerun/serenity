package gate

import (
	"os"
	"strings"
	"testing"
)

// TestAdversarialReleaseWorkflow is the CI-side companion to the release
// gate. Keep the release workflow's required adversarial job coupled to the
// corpus tests and to the seeded-vulnerability negative check: a future
// workflow edit that drops either step must fail review before a tag can ship.
func TestAdversarialReleaseWorkflow(t *testing.T) {
	b, err := os.ReadFile("../../.github/workflows/release.yml")
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(b)
	for _, want := range []string{
		"adversarial-gate:",
		"go test -race -count=1 ./internal/gate",
		"SERENITY_ADVERSARIAL_VULNERABLE=1 go test -count=1 ./internal/gate",
		"if [ $status -eq 0 ]",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("release.yml is missing adversarial gate assertion %q", want)
		}
	}
}
