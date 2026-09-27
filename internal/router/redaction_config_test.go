package router_test

import (
	"context"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/redact"
	"github.com/sirerun/serenity/internal/router"
)

// TestSetRedactionExtendsBuiltinsAtTheChokepoint proves operator
// patterns handed to the router (serenity.yml `redact.patterns`, ADR 021)
// are applied at the same chokepoint as the built-in table, and that
// installing them cannot switch a built-in off: the configured pattern
// and a built-in key shape are both masked in the one prompt.
func TestSetRedactionExtendsBuiltinsAtTheChokepoint(t *testing.T) {
	employeeID, err := redact.NewPattern("employee_id", `EMP-[0-9]{6}`)
	if err != nil {
		t.Fatal(err)
	}

	p := &recordingProvider{name: "recording", modelVersion: "rec@v1"}
	r := router.New(map[router.Tier]router.Provider{router.TierJudgment: p}, discardLedger{})
	r.SetRedaction(redact.Options{Patterns: []redact.Pattern{employeeID}})

	in := "Badge EMP-123456 uses key " + fakeAWSKey + "."
	if _, err := r.Complete(context.Background(), router.TaskClassComposerSynthesis, router.Prompt{Text: in}, router.Budget{}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(p.sent) != 1 {
		t.Fatalf("provider received %d prompts, want 1", len(p.sent))
	}
	got := p.sent[0]
	want := "Badge [REDACTED:EMPLOYEE_ID] uses key [REDACTED:API_KEY]."
	if got != want {
		t.Fatalf("prompt reaching the provider\n got: %q\nwant: %q", got, want)
	}
	if strings.Contains(got, "EMP-123456") || strings.Contains(got, fakeAWSKey) {
		t.Fatalf("prompt reaching the provider leaked a sensitive value: %q", got)
	}
}
