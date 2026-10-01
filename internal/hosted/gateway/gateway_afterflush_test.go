package gateway

import (
	"testing"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

func TestRememberFinalizeUsesDurableAfterFlushOutcome(t *testing.T) {
	phase, evidence := rememberFinalizeOutcome(true, "fact-sha")
	if phase != contracts.OperationCommitted || evidence.Kind != contracts.EvidenceCommitted || evidence.Ref != "fact:fact-sha" {
		t.Fatalf("after-flush outcome = (%q,%+v), want committed fact identity", phase, evidence)
	}
}

func TestRememberFinalizeKeepsAmbiguousAndNoEntryDistinct(t *testing.T) {
	phase, evidence := rememberFinalizeOutcome(true, "")
	if phase != contracts.OperationPendingReview || evidence.Kind != contracts.EvidenceUnknown {
		t.Fatalf("entered without durable flush = (%q,%+v), want pending review", phase, evidence)
	}
	phase, evidence = rememberFinalizeOutcome(false, "")
	if phase != contracts.OperationReleased || evidence.Kind != contracts.EvidenceNoCanonicalAttempt {
		t.Fatalf("no canonical entry = (%q,%+v), want released", phase, evidence)
	}
	phase, evidence = rememberFinalizeOutcome(false, "fact-sha")
	if phase != contracts.OperationReleased || evidence.Kind != contracts.EvidenceNoCanonicalAttempt {
		t.Fatalf("durable identity without trusted entry = (%q,%+v), want released", phase, evidence)
	}
}
