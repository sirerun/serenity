package compose

import (
	"context"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/router"
)

// TestAskRedactsSensitiveSpansFromOutboundPrompt is T4.11's own acc line:
// "the cloud prompt snapshot for synthesize contains redaction
// placeholders." It captures the exact string that crosses the
// router.Provider boundary -- the real cloud-egress point, not just
// buildPrompt's own return value -- and proves a card-number-shaped claim
// object never reaches it unredacted.
func TestAskRedactsSensitiveSpansFromOutboundPrompt(t *testing.T) {
	root := t.TempDir()
	const cardNumber = "4111 1111 1111 1111" // standard Luhn-valid test Visa number

	// writeAvaEntity files every claim under avaSlug's own entity page
	// regardless of the claim's SubjectSlug field (the fence format
	// attributes claims to the entity they're stored under, at parse
	// time) -- so this claim uses avaSlug like every other test in this
	// package, per that existing convention.
	claim := domain.Claim{
		ID:          "card-on-file",
		SubjectSlug: avaSlug,
		Predicate:   "owns_account",
		Object:      "Card on file: " + cardNumber,
		Confidence:  0.9,
		State:       domain.StateActive,
		Family:      "owns_account",
		SourceRef:   "src#1",
		Provenance:  domain.Provenance{SourceSHA256: "sha-card"},
	}
	writeAvaEntity(t, root, []domain.Claim{claim})

	var sent string
	fp := &fakeProvider{
		modelVersion: "fake-composer@v1",
		resp:         router.Response{Text: "The card on file is [claim:card-on-file]."},
		sentPrompt:   &sent,
	}
	c := New(root, config.Default(), fakeSearchStore{}, nil, newTestRouter(fp), "fake-composer@v1")

	if _, err := c.Ask(context.Background(), "What card is on file?"); err != nil {
		t.Fatalf("Ask: %v", err)
	}

	if sent == "" {
		t.Fatal("provider never received a prompt -- Ask did not reach Complete")
	}
	if strings.Contains(sent, cardNumber) {
		t.Fatalf("outbound prompt still carries the raw card number:\n%s", sent)
	}
	if !strings.Contains(sent, "[REDACTED:CARD_NUMBER]") {
		t.Fatalf("outbound prompt has no redaction placeholder where the card number was:\n%s", sent)
	}
}
