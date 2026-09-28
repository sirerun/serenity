package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/disposition"
)

// TestInboxApplyPrintsLedgerPayloadBeforePublishing is AI-03's inbox half:
// `inbox --apply` shows the kind, the target and the full payload of a
// ledger-bound decision before it publishes anything, so the human running
// it sees exactly what enters the directive ledger.
func TestInboxApplyPrintsLedgerPayloadBeforePublishing(t *testing.T) {
	ds, store, item, root := seedInboxLedgerDecision(t, disposition.KindPreceptDraft)
	ctx := context.Background()
	if _, err := ds.Dispose(ctx, item.ID, disposition.VerdictAccept, nil, "", currentActor(), "", inboxFixedNow); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runInbox(ctx, root, strings.NewReader(""), &out, inboxOptions{ApplyID: item.ID}, inboxFixedNow); err != nil {
		t.Fatalf("apply: %v; output %s", err, &out)
	}
	text := out.String()
	applied := strings.Index(text, "applied "+item.ID)
	if applied < 0 {
		t.Fatalf("no applied line: %s", text)
	}
	for _, want := range []string{
		"kind: precept_draft",
		"target: new ledger decision",
		"Review changes before release",
		"Human review is required before release.",
		"Unreviewed changes can surprise the owner",
		"actor: " + currentActor(),
	} {
		i := strings.Index(text, want)
		if i < 0 || i > applied {
			t.Fatalf("output lacks %q before publishing:\n%s", want, text)
		}
	}
	infos, err := store.List(ctx)
	if err != nil || len(infos) != 2 {
		t.Fatalf("entries=%d err=%v", len(infos), err)
	}
}

// TestInboxApplyRefusesLedgerAcceptFromAnotherActor proves `inbox --apply`
// publishes a ledger-bound acceptance only when the CLI's own actor
// recorded it. A `human:` actor that is not this CLI user (for example
// one forged through the HTTP transport before the actor was derived from
// the principal) is refused and nothing enters the ledger.
func TestInboxApplyRefusesLedgerAcceptFromAnotherActor(t *testing.T) {
	for _, kind := range []disposition.Kind{disposition.KindPreceptDraft, disposition.KindDecompose} {
		t.Run(string(kind), func(t *testing.T) {
			ds, store, item, root := seedInboxLedgerDecision(t, kind)
			ctx := context.Background()
			forged := "human:forged-" + strings.TrimPrefix(currentActor(), "human:")
			if _, err := ds.Dispose(ctx, item.ID, disposition.VerdictAccept, nil, "", forged, "", inboxFixedNow); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			err := runInbox(ctx, root, strings.NewReader(""), &out, inboxOptions{ApplyID: item.ID}, inboxFixedNow)
			if err == nil {
				t.Fatalf("apply published an acceptance recorded by %s; output %s", forged, &out)
			}
			if !strings.Contains(err.Error(), forged) {
				t.Fatalf("refusal %q does not name the recorded actor %s", err, forged)
			}
			infos, err := store.List(ctx)
			if err != nil || len(infos) != 1 {
				t.Fatalf("entries=%d err=%v, want only the fixture parent", len(infos), err)
			}
		})
	}
}
