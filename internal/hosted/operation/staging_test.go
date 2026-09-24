package operation_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/operation"
)

func TestStagingGateBoundsConcurrentReservationsAndQuota(t *testing.T) {
	physical := map[string]int64{"a": 0}
	g, err := operation.NewStagingGate(operation.StagingConfig{Budget: 100, MaxStage: 60, Headroom: 10, Quota: map[string]int64{"a": 100}, Physical: func(a string) int64 { return physical[a] }, Free: func() int64 { return 200 }})
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := g.ReserveStage(context.Background(), contracts.StageRequest{AccountID: "a", BrainID: "b", OperationID: "op"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = g.ReserveStage(context.Background(), contracts.StageRequest{AccountID: "a", BrainID: "b", OperationID: "op2"}); !errors.Is(err, contracts.ErrStagingBusy) {
		t.Fatalf("second stage: %v", err)
	}
	if err = g.AdmitMeasured(context.Background(), ticket, 61); !errors.Is(err, contracts.ErrStageCeilingExceeded) {
		t.Fatalf("ceiling: %v", err)
	}
	if err = g.AdmitMeasured(context.Background(), ticket, 60); err != nil {
		t.Fatal(err)
	}
	if err = g.Release(context.Background(), ticket, contracts.StagePublished); err != nil {
		t.Fatal(err)
	}
	if err = g.Release(context.Background(), ticket, contracts.StagePublished); err != nil {
		t.Fatalf("idempotent release: %v", err)
	}
	if g.Outstanding() != 0 {
		t.Fatalf("outstanding=%d", g.Outstanding())
	}
	physical["a"] = 50
	t2, err := g.ReserveStage(context.Background(), contracts.StageRequest{AccountID: "a", BrainID: "b", OperationID: "op3"})
	if err != nil {
		t.Fatal(err)
	}
	if err = g.AdmitMeasured(context.Background(), t2, 60); !errors.Is(err, contracts.ErrStorageQuotaExceeded) {
		t.Fatalf("quota: %v", err)
	}
}
