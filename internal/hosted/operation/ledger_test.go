package operation_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/operation"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func fixture(t *testing.T) (*store.Store, string, string) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateAccount(context.Background(), "ledger@example.com")
	if err != nil {
		t.Fatal(err)
	}
	bid := store.ID()
	if _, err = s.InsertBrain(context.Background(), a.ID, bid, bid, "ready", time.Now()); err != nil {
		t.Fatal(err)
	}
	return s, a.ID, bid
}

func TestLedgerReserveFinalizeAndReplay(t *testing.T) {
	ctx := context.Background()
	s, account, brain := fixture(t)
	defer func() { _ = s.Close() }()
	l := &operation.Ledger{Store: s, Clock: func() time.Time { return time.Unix(100, 0).UTC() }}
	req := contracts.ReserveRequest{AccountID: account, BrainID: brain, ClientKey: "k1", Fingerprint: "fp", QuotaPeriod: "2026-09", Source: "gateway.remember", LeaseFor: time.Minute, Deltas: []contracts.ReserveDelta{{Metric: "writes", Units: 1, Limit: 2}, {Metric: "input_tokens", Units: 7, Limit: 10}}}
	r, err := l.Reserve(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = l.EnterCanonical(ctx, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = l.Finalize(ctx, r.ID, contracts.OperationCommitted, contracts.Evidence{Kind: contracts.EvidenceCommitted, Ref: "commit-1"}); err != nil {
		t.Fatal(err)
	}
	replay, err := l.Reserve(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if replay.ID != r.ID {
		t.Fatalf("replay allocated %q, want %q", replay.ID, r.ID)
	}
	var writes, tokens int64
	if err = s.DB().QueryRow(`SELECT committed FROM usage_windows WHERE account_id=? AND window_key=? AND metric='writes'`, account, "2026-09").Scan(&writes); err != nil {
		t.Fatal(err)
	}
	if err = s.DB().QueryRow(`SELECT committed FROM usage_windows WHERE account_id=? AND window_key=? AND metric='input_tokens'`, account, "2026-09").Scan(&tokens); err != nil {
		t.Fatal(err)
	}
	if writes != 1 || tokens != 7 {
		t.Fatalf("usage writes=%d tokens=%d", writes, tokens)
	}
}

func TestLedgerHoldsCapacityAndRejectsKeyReuse(t *testing.T) {
	ctx := context.Background()
	s, account, brain := fixture(t)
	defer func() { _ = s.Close() }()
	l := &operation.Ledger{Store: s}
	req := contracts.ReserveRequest{AccountID: account, BrainID: brain, ClientKey: "k1", Fingerprint: "fp", QuotaPeriod: "2026-09", Source: "gateway.remember", LeaseFor: time.Minute, Deltas: []contracts.ReserveDelta{{Metric: "writes", Units: 2, Limit: 2}}}
	r, err := l.Reserve(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	_, err = l.Reserve(ctx, req)
	if !errors.Is(err, contracts.ErrOperationInProgress) {
		t.Fatalf("second reservation: %v", err)
	}
	bad := req
	bad.Fingerprint = "different"
	if _, err = l.Reserve(ctx, bad); !errors.Is(err, contracts.ErrOperationKeyReuse) {
		t.Fatalf("key reuse: %v", err)
	}
	if _, err = l.Reserve(ctx, contracts.ReserveRequest{AccountID: account, BrainID: brain, QuotaPeriod: "2026-09", Source: "gateway.remember", LeaseFor: time.Minute, Deltas: []contracts.ReserveDelta{{Metric: "writes", Units: 1, Limit: 2}}}); !errors.Is(err, contracts.ErrOperationLimitExceeded) {
		t.Fatalf("held capacity: %v", err)
	}
	if _, err = l.Finalize(ctx, r.ID, contracts.OperationReleased, contracts.Evidence{Kind: contracts.EvidenceNoCanonicalAttempt}); err != nil {
		t.Fatal(err)
	}
	if _, err = l.Reserve(ctx, contracts.ReserveRequest{AccountID: account, BrainID: brain, QuotaPeriod: "2026-09", Source: "gateway.remember", LeaseFor: time.Minute, Deltas: []contracts.ReserveDelta{{Metric: "writes", Units: 2, Limit: 2}}}); err != nil {
		t.Fatalf("released capacity not reusable: %v", err)
	}
}
