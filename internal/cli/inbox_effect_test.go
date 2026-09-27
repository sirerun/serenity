package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/spend"
)

// stageOverCeilingEffect stages one KindEffect item the real way: a
// spend.Checker refuses a judgment-tier row that would cross its ceiling
// and parks it for approval instead of recording it.
func stageOverCeilingEffect(t *testing.T, root string) (itemID string, row index.SpendRow) {
	t.Helper()
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	defer func() { _ = eng.Close() }()
	checker := spend.New(eng, disposition.NewStore(eng), spend.Config{MonthlyCeilingUSD: 1})
	row = index.SpendRow{
		ID:           "spend-effect-1",
		TaskClass:    "composer_synthesis",
		Tier:         "judgment",
		Provider:     "test",
		ModelVersion: "test-judge@v1",
		CostUSD:      5,
		OccurredAt:   inboxFixedNow,
	}
	decision, err := checker.CheckAndRecord(context.Background(), row, inboxFixedNow)
	if err != nil {
		t.Fatalf("CheckAndRecord: %v", err)
	}
	if decision.Recorded || decision.ItemID == "" {
		t.Fatalf("over-ceiling row was not staged for approval: %+v", decision)
	}
	return decision.ItemID, row
}

func spendRowCount(t *testing.T, root, id string) int {
	t.Helper()
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	defer func() { _ = eng.Close() }()
	rows, err := eng.SpendRows(context.Background())
	if err != nil {
		t.Fatalf("SpendRows: %v", err)
	}
	n := 0
	for _, r := range rows {
		if r.ID == id {
			n++
		}
	}
	return n
}

// TestInboxAcceptEffectAppliesThroughSpendChecker proves an accepted
// `effect` inbox item is applied (FUN-06): the held-back spend row is
// recorded through spend.Checker.ApplyDisposedEffect, and an --apply
// retry of the same accepted item neither fails nor records it twice.
func TestInboxAcceptEffectAppliesThroughSpendChecker(t *testing.T) {
	root := initBrainRepo(t)
	itemID, row := stageOverCeilingEffect(t, root)
	if n := spendRowCount(t, root, row.ID); n != 0 {
		t.Fatalf("staged effect already recorded %d row(s)", n)
	}

	var out bytes.Buffer
	if err := runInbox(context.Background(), root, strings.NewReader(" "), &out, inboxOptions{}, inboxFixedNow); err != nil {
		t.Fatalf("runInbox accept: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "applied "+itemID) {
		t.Fatalf("inbox did not report applying %s:\n%s", itemID, out.String())
	}
	if n := spendRowCount(t, root, row.ID); n != 1 {
		t.Fatalf("accepted effect recorded %d spend row(s), want 1\n%s", n, out.String())
	}

	out.Reset()
	if err := runInbox(context.Background(), root, strings.NewReader(""), &out, inboxOptions{ApplyID: itemID}, inboxFixedNow); err != nil {
		t.Fatalf("inbox --apply retry: %v\n%s", err, out.String())
	}
	if n := spendRowCount(t, root, row.ID); n != 1 {
		t.Fatalf("--apply retry left %d spend row(s), want 1", n)
	}
}

// TestInboxRejectEffectRecordsNothing proves the checker's gate: only an
// accept verdict applies an effect, so a rejected one records no spend,
// and --apply on it is refused rather than bypassing the verdict.
func TestInboxRejectEffectRecordsNothing(t *testing.T) {
	root := initBrainRepo(t)
	itemID, row := stageOverCeilingEffect(t, root)

	var out bytes.Buffer
	if err := runInbox(context.Background(), root, strings.NewReader("rtoo expensive\n"), &out, inboxOptions{}, inboxFixedNow); err != nil {
		t.Fatalf("runInbox reject: %v\n%s", err, out.String())
	}
	if n := spendRowCount(t, root, row.ID); n != 0 {
		t.Fatalf("rejected effect recorded %d spend row(s)", n)
	}
	out.Reset()
	err := runInbox(context.Background(), root, strings.NewReader(""), &out, inboxOptions{ApplyID: itemID}, inboxFixedNow)
	if err == nil || !strings.Contains(err.Error(), "want disposed with accept") {
		t.Fatalf("--apply on a rejected effect: err = %v, want the spend checker's accept gate", err)
	}
	if n := spendRowCount(t, root, row.ID); n != 0 {
		t.Fatalf("--apply on a rejected effect recorded %d spend row(s)", n)
	}
}
