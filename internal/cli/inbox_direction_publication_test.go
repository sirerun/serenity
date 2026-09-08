package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/direction"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
)

func commitInboxLedgerFixture(t *testing.T, root string) {
	t.Helper()
	for _, args := range [][]string{{"config", "user.name", "Inbox ledger fixture"}, {"config", "user.email", "fixture@example.invalid"}, {"add", ".dira"}, {"commit", "--quiet", "-m", "seed canonical ledger"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
	}
}

func seedInboxLedgerDecision(t *testing.T, kind disposition.Kind) (*disposition.Store, *direction.Store, disposition.Item, string) {
	t.Helper()
	ds, root := openInboxTestStore(t)
	store := newTestDirectionStore(t, root)
	ctx := context.Background()
	parent := &ledger.Entry{Kind: ledger.KindIntent, Title: "Ship the synthetic release", State: ledger.StateActive, Created: inboxFixedNow.Format(time.RFC3339)}
	if err := ledger.Add(ctx, store, parent); err != nil {
		t.Fatal(err)
	}
	commitInboxLedgerFixture(t, root)
	if kind == disposition.KindDecompose {
		return ds, store, seedDecomposeItem(t, ds, ctx, inboxFixedNow, parent.ID, "Prepare the release notes", "needed for review", ""), root
	}
	payload, err := json.Marshal(direction.PreceptDraftPayload{QuestionID: "review", Question: "How should changes ship?", Answer: "Review them first", Title: "Review changes before release", Body: "Human review is required before release.", Alternatives: []domain.RejectedAlternative{{Option: "No review", WhyNot: "Unreviewed changes can surprise the owner"}}})
	if err != nil {
		t.Fatal(err)
	}
	item, err := ds.Create(ctx, kind, payload, "", inboxFixedNow)
	if err != nil {
		t.Fatal(err)
	}
	return ds, store, item, root
}

func TestInboxLedgerCommitFailureRemainsVisibleAndRetryable(t *testing.T) {
	for _, kind := range []disposition.Kind{disposition.KindPreceptDraft, disposition.KindDecompose} {
		t.Run(string(kind), func(t *testing.T) {
			ds, store, item, root := seedInboxLedgerDecision(t, kind)
			ctx := context.Background()
			hook := filepath.Join(root, ".git", "hooks", "pre-commit")
			if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := runInbox(ctx, root, strings.NewReader(" "), &out, inboxOptions{}, inboxFixedNow); err == nil {
				t.Fatal("commit fault was swallowed")
			}
			recorded, err := ds.Get(ctx, item.ID)
			if err != nil || recorded.State != disposition.StateDisposed {
				t.Fatalf("recorded decision: %+v %v; %s", recorded, err, &out)
			}
			out.Reset()
			if err := runInbox(ctx, root, strings.NewReader(""), &out, inboxOptions{Unapplied: true}, inboxFixedNow); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), item.ID) {
				t.Fatalf("accepted ledger effect hidden after commit failure: %s", &out)
			}
			if err := os.Remove(hook); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 2; i++ {
				out.Reset()
				if err := runInbox(ctx, root, strings.NewReader(""), &out, inboxOptions{ApplyID: item.ID}, inboxFixedNow.Add(time.Hour)); err != nil {
					t.Fatal(err)
				}
			}
			infos, err := store.List(ctx)
			if err != nil || len(infos) != 2 {
				t.Fatalf("entries=%d err=%v", len(infos), err)
			}
			history, err := ds.HistoryFor(ctx, item.ID)
			if err != nil || len(history) != 1 {
				t.Fatalf("history=%d err=%v", len(history), err)
			}
			out.Reset()
			if err := runInbox(ctx, root, strings.NewReader(""), &out, inboxOptions{Unapplied: true}, inboxFixedNow); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), item.ID) {
				t.Fatal("completed effect still listed")
			}
		})
	}
}
