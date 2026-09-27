package cli

import (
	"strings"
	"testing"
)

// TestServeDocsStateWhatServeStarts guards FUN-06's doc half: the operator
// docs say what `serve` starts and does not start, and the stale claims
// that it runs the cron daemon or leaves its protocol routes unwired stay
// gone from the docs and package comments.
func TestServeDocsStateWhatServeStarts(t *testing.T) {
	mcpDoc := readFileT(t, "../../docs/operator/mcp.md")
	for _, want := range []string{
		"### What `serve` starts and does not start",
		"`serve` does not start:",
		"`serenity cron <job>`",
		"`models.extraction`",
		"`effect` item",
	} {
		if !strings.Contains(mcpDoc, want) {
			t.Errorf("docs/operator/mcp.md missing %q", want)
		}
	}

	stale := map[string][]string{
		"../../docs/operator/mcp.md":                       {"serenityd"},
		"../../docs/operator/scheduling.md":                {"will embed these same job functions"},
		"../../internal/cron/cron.go":                      {"`serenityd` embeds these same"},
		"../../internal/server/direction/direction.go":     {"Not wired into a live `serenity serve`", "no\n// provider-from-config wiring exists yet"},
		"../../internal/server/disposition/disposition.go": {"Not wired into a live `serenity serve`"},
		"../../README.md":                                  {"Free-text plan checks currently return `unverified`"},
	}
	for path, claims := range stale {
		body := readFileT(t, path)
		for _, claim := range claims {
			if strings.Contains(body, claim) {
				t.Errorf("%s still claims %q", path, claim)
			}
		}
	}
}
