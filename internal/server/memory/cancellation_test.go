package memory

import (
	"context"
	"testing"
)

func TestForgetOperationWireContract(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()
	cancel := func(key string) cancelOperationResponse {
		t.Helper()
		v, bad, err := h.cancelOperation(ctx, mustMarshal(t, map[string]string{"operation_key": key}))
		if err != nil || bad {
			t.Fatalf("cancel: %v %+v", err, v)
		}
		return v.(cancelOperationResponse)
	}
	missing := cancel("missing")
	if !missing.Canceled || missing.ID != "" || missing.Expired {
		t.Fatalf("missing cancellation: %+v", missing)
	}
	req := rememberRequest{OperationKey: "missing", Fact: "must not be created", Provenance: "test"}
	v, bad, err := h.remember(ctx, mustMarshal(t, req))
	if err != nil || !bad || asVerbError(t, v, bad).Error != "operation_canceled" {
		t.Fatal("missing operation resurrected", err)
	}
	req.OperationKey = "present"
	v, bad, err = h.remember(ctx, mustMarshal(t, req))
	if err != nil || bad {
		t.Fatal(err)
	}
	id := v.(rememberResponse).ID
	canceled := cancel("present")
	if !canceled.Canceled || !canceled.Expired || canceled.ID != id {
		t.Fatalf("present cancellation: %+v", canceled)
	}
	if cancel("present").Expired {
		t.Fatal("repeated cancellation reported new expiry")
	}
	v, bad, err = h.remember(ctx, mustMarshal(t, req))
	if err != nil || bad {
		t.Fatal(err)
	}
	recovered := v.(rememberResponse)
	if recovered.ID != id || !recovered.Expired || recovered.SearchState != "not_eligible" {
		t.Fatalf("recovery: %+v", recovered)
	}
}
