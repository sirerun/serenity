package operation_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/operation"
)

func TestLedgerLookupReturnsCompleteRecordWithoutChangingIt(t *testing.T) {
	ctx := context.Background()
	s, account, brain := fixture(t)
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	clock := time.Date(2026, 10, 1, 12, 34, 56, 789123000, time.UTC)
	l := &operation.Ledger{Store: s, Clock: func() time.Time { return clock }}
	reserved, err := l.Reserve(ctx, contracts.ReserveRequest{
		AccountID: account, BrainID: brain, ClientKey: "opaque-client-key", Fingerprint: "normalized-request-fingerprint",
		QuotaPeriod: "2026-10", Source: "gateway.remember", LeaseFor: 5 * time.Minute,
		Deltas: []contracts.ReserveDelta{{Metric: "writes", Units: 1, Limit: 4}, {Metric: "input_tokens", Units: 9, Limit: 40}},
	})
	if err != nil {
		t.Fatal(err)
	}
	entered, err := l.EnterCanonical(ctx, reserved.ID)
	if err != nil {
		t.Fatal(err)
	}
	want, err := l.Finalize(ctx, reserved.ID, contracts.OperationCommitted, contracts.Evidence{Kind: contracts.EvidenceCommitted, Ref: "commit-evidence"})
	if err != nil {
		t.Fatal(err)
	}

	got, err := l.Lookup(ctx, reserved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.AccountID != want.AccountID || got.BrainID != want.BrainID ||
		got.ClientKey != want.ClientKey || got.Fingerprint != want.Fingerprint ||
		got.QuotaPeriod != want.QuotaPeriod || got.Phase != want.Phase || got.Source != want.Source || got.Evidence != want.Evidence ||
		!reflect.DeepEqual(got.Deltas, want.Deltas) || !got.CanonicalEnteredAt.Equal(entered.CanonicalEnteredAt) ||
		!got.LeaseExpiresAt.Equal(want.LeaseExpiresAt) || !got.CreatedAt.Equal(want.CreatedAt) || !got.FinalizedAt.Equal(want.FinalizedAt) {
		t.Fatalf("Lookup returned incomplete or changed record:\n got: %+v\nwant: %+v", got, want)
	}

	var writes, tokens int64
	if err = s.DB().QueryRowContext(ctx, `SELECT committed FROM usage_windows WHERE account_id=? AND window_key=? AND metric='writes'`, account, "2026-10").Scan(&writes); err != nil {
		t.Fatal(err)
	}
	if err = s.DB().QueryRowContext(ctx, `SELECT committed FROM usage_windows WHERE account_id=? AND window_key=? AND metric='input_tokens'`, account, "2026-10").Scan(&tokens); err != nil {
		t.Fatal(err)
	}
	if writes != 1 || tokens != 9 {
		t.Fatalf("Lookup changed committed usage: writes=%d input_tokens=%d", writes, tokens)
	}
	again, err := l.Lookup(ctx, reserved.ID)
	if err != nil || !reflect.DeepEqual(again, got) {
		t.Fatalf("repeated Lookup changed returned scalar record: got=%+v err=%v", again, err)
	}
}

func TestLedgerLookupCanceledContext(t *testing.T) {
	s, account, brain := fixture(t)
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	l := &operation.Ledger{Store: s}
	rec, err := l.Reserve(context.Background(), contracts.ReserveRequest{
		AccountID: account, BrainID: brain, Fingerprint: "fp", QuotaPeriod: "2026-10",
		Source: "gateway.remember", LeaseFor: time.Minute,
		Deltas: []contracts.ReserveDelta{{Metric: "writes", Units: 1, Limit: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = l.Lookup(ctx, rec.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("Lookup with canceled context = %v, want context.Canceled", err)
	}
}

func TestLedgerLookupMissingAndEmptyID(t *testing.T) {
	s, _, _ := fixture(t)
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	l := &operation.Ledger{Store: s}
	for _, id := range []string{"missing-operation-id", ""} {
		if rec, err := l.Lookup(context.Background(), id); !errors.Is(err, contracts.ErrOperationNotFound) || rec.ID != "" {
			t.Errorf("Lookup(%q) = %+v, %v; want zero record and ErrOperationNotFound", id, rec, err)
		}
	}
}
