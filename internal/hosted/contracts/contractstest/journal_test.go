package contractstest_test

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/contracts/contractstest"
)

func TestReferenceJournalConformance(t *testing.T) {
	contractstest.RunJournalSuite(t, contractstest.ReferenceJournal)
}

// Negative control 1: the withdrawn key scheme. Objects keyed subject-first
// with a StartAfter watermark permanently miss an entry whose subject sorts
// before an already-consumed key. This reproduces that read on the same object
// store to show the suite's incremental-read scenario is not vacuous.
func TestWithdrawnSubjectFirstKeySchemeLosesEntries(t *testing.T) {
	ctx := context.Background()
	store := contractstest.NewMemObjectStore()
	put := func(subject string, n int) {
		key := fmt.Sprintf("deletion-journal/account/%s/%d.json", subject, n)
		if _, err := store.PutIfAbsent(ctx, key, []byte(subject)); err != nil {
			t.Fatal(err)
		}
	}
	read := func(startAfter string) (keys []string) {
		keys, _, _ = store.ListAfter(ctx, "deletion-journal/", startAfter, 0)
		return keys
	}
	put("zzz", 1)
	watermark := read("")[len(read(""))-1] // the continuation marker after the first read
	put("aaa", 2)                          // appended later, sorts earlier
	if got := read(watermark); len(got) != 0 {
		t.Fatalf("expected the old scheme to miss the later entry, read %v", got)
	}
}

// Negative control 2: the withdrawn completeness authority. A reader that
// takes "how many entries exist" from the local control database, which a
// restore replaces with the snapshot's copy, declares a journal complete while
// entries appended after the snapshot are missing. The journal read from the
// snapshot's watermark finds them.
func TestLocalHighWaterCannotProveTailCompleteness(t *testing.T) {
	ctx := context.Background()
	clock := contractstest.NewClock()
	store := contractstest.NewMemObjectStore()
	writer := contractstest.NewRefJournal(store, "old-writer", 1, clock.Now)
	var snapshot contracts.DeletionEntry
	for i, s := range []string{"a1", "a2", "a3", "a4"} {
		e, err := writer.AppendDeletion(ctx, contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectAccount, SubjectID: s, Outcome: contracts.DeletionIntentRequested})
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			snapshot = e // backup taken here; its restored control DB says high-water = 2
		}
	}
	localHighWater := snapshot.Watermark.SequenceID
	keys, _, _ := store.ListAfter(ctx, contracts.JournalPrefix(1), "", 0)
	if int64(len(keys)) == localHighWater {
		t.Fatal("test setup: journal should hold more than the stale high-water")
	}
	// A control-DB-based read would verify entries 1..localHighWater and stop, calling it complete.
	res, err := contractstest.NewRefJournal(store, "recovery", 2, clock.Now).ReadThrough(ctx, snapshot.Watermark)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Entries) != 2 || res.Entries[0].SubjectID != "a3" || res.Entries[1].SubjectID != "a4" {
		t.Fatalf("journal read from the snapshot watermark = %+v; want the two entries after it", res.Entries)
	}
}

func TestJournalObjectLayoutIsSelfDescribing(t *testing.T) {
	ctx := context.Background()
	clock := contractstest.NewClock()
	store := contractstest.NewMemObjectStore()
	j := contractstest.NewRefJournal(store, "w1", 1, clock.Now)
	e, err := j.AppendDeletion(ctx, contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectBrain, SubjectID: "brain-9", Outcome: contracts.DeletionOutcomePurged})
	if err != nil {
		t.Fatal(err)
	}
	body, found, _ := store.Get(ctx, contracts.JournalKey(1, 1))
	if !found || contracts.HashObject(body) != e.Watermark.EntryHash {
		t.Fatal("the stored object is not the one the watermark hashes")
	}
	var obj contracts.JournalObject
	if err := json.Unmarshal(body, &obj); err != nil || obj.Kind != contracts.JournalKindEntry || obj.SubjectID != "brain-9" || obj.WriterID != "w1" {
		t.Fatalf("object = %+v, %v", obj, err)
	}
	keys, _, _ := store.ListAfter(ctx, "", "", 0)
	if !sort.StringsAreSorted(keys) {
		t.Fatal("keys not sorted")
	}
	_ = time.Now // the layout must carry no wall-clock ordering dependency
}
