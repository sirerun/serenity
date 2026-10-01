package memory

import (
	"context"
	"testing"

	"github.com/sirerun/serenity/internal/writer"
)

func TestRememberValidationRunsBeforeCanonicalOperationCallback(t *testing.T) {
	h, _ := newTestHandlers(t)
	called := false
	ctx := writer.WithCanonicalOperation(context.Background(), writer.CanonicalOperation{
		ID: "fedcba9876543210",
		BeforeCommit: func(context.Context, string) error {
			called = true
			return nil
		},
	})
	response, bad, err := h.remember(ctx, mustMarshal(t, rememberRequest{Fact: "  ", Provenance: "test"}))
	if err != nil || !bad || asVerbError(t, response, bad).Error != ErrCodeInvalidParams {
		t.Fatalf("malformed remember response=%+v err=%v", response, err)
	}
	if called {
		t.Fatal("canonical operation entered for a deterministically rejected request")
	}
}

func TestRememberOperationWireContract(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()
	req := rememberRequest{OperationKey: "post:1", Fact: "retain attribution", Provenance: "test", TTL: "30d"}
	response, bad, err := h.remember(ctx, mustMarshal(t, req))
	if err != nil || !bad || asVerbError(t, response, bad).Error != ErrCodeInvalidParams {
		t.Fatal("relative keyed TTL accepted")
	}
	req.TTL = "2027-01-01T00:00:00Z"
	response, bad, err = h.remember(ctx, mustMarshal(t, req))
	if err != nil || bad {
		t.Fatalf("insert: %v %+v", err, response)
	}
	first := response.(rememberResponse)
	_, bad, err = h.forget(ctx, mustMarshal(t, map[string]string{"id": first.ID}))
	if err != nil || bad {
		t.Fatal("forget failed")
	}
	// ADR 019: forget erased the fact and fenced its key, so a retry is
	// refused as canceled instead of echoing the erased record.
	response, bad, err = h.remember(ctx, mustMarshal(t, req))
	if err != nil || !bad || asVerbError(t, response, bad).Error != ErrCodeOperationCanceled {
		t.Fatalf("withdrawal lost: %+v", response)
	}
	req.Provenance = "changed attribution"
	response, bad, err = h.remember(ctx, mustMarshal(t, req))
	if err != nil || !bad || asVerbError(t, response, bad).Error != ErrCodeOperationCanceled {
		t.Fatal("erased key accepted a changed payload")
	}
	req.OperationKey = "post:2"
	response, bad, err = h.remember(ctx, mustMarshal(t, req))
	if err != nil || bad {
		t.Fatalf("fresh key: %v %+v", err, response)
	}
	req.Provenance = "changed again"
	response, bad, err = h.remember(ctx, mustMarshal(t, req))
	if err != nil || !bad || asVerbError(t, response, bad).Error != "operation_conflict" {
		t.Fatal("key conflict not exposed")
	}
}
