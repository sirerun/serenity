package direction

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
)

// TestLedgerPublicationRequiresHumanActorAndCLIOrigin is AI-03's
// publication gate: a precept_draft or decompose acceptance publishes only
// when its recorded actor is human AND the caller is the CLI channel. A
// `human:` actor presented through any other origin, or an `agent:` actor
// through the CLI, is refused before anything is written.
func TestLedgerPublicationRequiresHumanActorAndCLIOrigin(t *testing.T) {
	for _, kind := range []disposition.Kind{disposition.KindPreceptDraft, disposition.KindDecompose} {
		t.Run(string(kind), func(t *testing.T) {
			s, ds, item, _ := ledgerPublicationFixture(t, kind)
			ctx := context.Background()
			if err := s.PreviewDisposition(ctx, item, "human:reviewer", OriginHTTP, decomposeFixedNow); !errors.Is(err, ErrHumanChannelRequired) {
				t.Fatalf("preview over HTTP origin err = %v, want ErrHumanChannelRequired", err)
			}
			if err := s.PreviewDisposition(ctx, item, "agent:daemon", OriginCLI, decomposeFixedNow); !errors.Is(err, ErrHumanChannelRequired) {
				t.Fatalf("preview with agent actor err = %v, want ErrHumanChannelRequired", err)
			}
			accepted := acceptLedger(t, ds, item)
			if _, err := s.ApplyAndCommitDisposition(ctx, ds, accepted, OriginHTTP, decomposeFixedNow.Add(time.Hour)); !errors.Is(err, ErrHumanChannelRequired) {
				t.Fatalf("publish over HTTP origin err = %v, want ErrHumanChannelRequired", err)
			}
			infos, err := s.List(ctx)
			if err != nil || len(infos) != 1 {
				t.Fatalf("ledger entries = %d (%v), want only the fixture parent", len(infos), err)
			}
			if _, err := s.ApplyAndCommitDisposition(ctx, ds, accepted, OriginCLI, decomposeFixedNow.Add(time.Hour)); err != nil {
				t.Fatalf("publish from the CLI with a human actor: %v", err)
			}
		})
	}
}

func TestAuthorizeLedgerDisposition(t *testing.T) {
	cases := []struct {
		kind    disposition.Kind
		verdict disposition.Verdict
		actor   string
		origin  Origin
		ok      bool
	}{
		{disposition.KindPreceptDraft, disposition.VerdictAccept, "human:reviewer", OriginCLI, true},
		{disposition.KindPreceptDraft, disposition.VerdictEditAccept, "human:reviewer", OriginCLI, true},
		{disposition.KindPreceptDraft, disposition.VerdictAccept, "human:reviewer", OriginHTTP, false},
		{disposition.KindPreceptDraft, disposition.VerdictAccept, "agent:daemon", OriginCLI, false},
		{disposition.KindPreceptDraft, disposition.VerdictAccept, "human:", OriginCLI, false},
		{disposition.KindPreceptDraft, disposition.VerdictAccept, "human:reviewer", "", false},
		{disposition.KindDecompose, disposition.VerdictAccept, "agent:daemon", OriginHTTP, false},
		{disposition.KindPreceptDraft, disposition.VerdictReject, "agent:daemon", OriginHTTP, true},
		{disposition.KindPreceptDraft, disposition.VerdictDefer, "agent:daemon", OriginHTTP, true},
		{disposition.KindDistill, disposition.VerdictAccept, "agent:daemon", OriginHTTP, true},
	}
	for _, c := range cases {
		err := AuthorizeLedgerDisposition(c.kind, c.verdict, c.actor, c.origin)
		if (err == nil) != c.ok {
			t.Errorf("AuthorizeLedgerDisposition(%s, %s, %q, %q) = %v, want ok=%v", c.kind, c.verdict, c.actor, c.origin, err, c.ok)
		}
		if err != nil && !errors.Is(err, ErrHumanChannelRequired) {
			t.Errorf("error %v does not wrap ErrHumanChannelRequired", err)
		}
	}
}
