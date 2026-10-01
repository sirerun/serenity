package memory

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

func TestRememberIgnoresClientCanonicalOperationID(t *testing.T) {
	h, _ := newTestHandlers(t)
	request := map[string]any{
		"fact":                   "client marker must not be trusted",
		"provenance":             "test",
		"canonical_operation_id": "client-controlled-id",
	}
	args, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response, isError, err := h.remember(context.Background(), args)
	if err != nil || isError {
		t.Fatalf("remember response=%+v isError=%v err=%v", response, isError, err)
	}
	result, ok := response.(rememberResponse)
	if !ok {
		t.Fatalf("remember response type = %T", response)
	}
	data, _, err := h.deps.Sources.Read(result.ID)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := store.DecodeMemoryFact(data)
	if err != nil {
		t.Fatal(err)
	}
	if payload.CanonicalOperationID != "" {
		t.Fatalf("client controlled canonical operation ID %q", payload.CanonicalOperationID)
	}
}

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
