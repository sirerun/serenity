package imap_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	imapapi "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"

	"github.com/sirerun/serenity/internal/connector"
	imapconn "github.com/sirerun/serenity/internal/connector/imap"
	"github.com/sirerun/serenity/internal/store"
)

func newTestConnector(srv *fakeServer) *imapconn.Connector {
	return &imapconn.Connector{
		Account:   testAccount,
		Mailbox:   testMailbox,
		BatchSize: 1, // force multiple FETCH round trips even for a handful of fixtures
		Dial:      srv.dial(testAccount, testPassword),
	}
}

// countSources walks root's content-addressed source layout and returns
// how many distinct sha256 directories (identified by their meta.yaml
// sidecar) exist on disk.
func countSources(t *testing.T, root string) int {
	t.Helper()
	n := 0
	dir := filepath.Join(root, "brain", "sources")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return 0
	}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "meta.yaml" {
			n++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return n
}

// writeItems runs every RawItem Poll returned through ToSource and the
// real SourceStore, exactly as the (later) sync pipeline will.
func writeItems(t *testing.T, ss *store.SourceStore, items []connector.RawItem, c *imapconn.Connector) {
	t.Helper()
	for _, item := range items {
		src, err := c.ToSource(item)
		if err != nil {
			t.Fatalf("ToSource: %v", err)
		}
		if _, err := ss.Write(item.Bytes, src); err != nil {
			t.Fatalf("SourceStore.Write: %v", err)
		}
	}
}

// TestDoubleImportProducesZeroDuplicateSources is T1.4's golden acc test:
// "golden test replays testdata/imap/*.eml through an in-process fake IMAP
// server: double poll yields zero duplicate sources."
func TestDoubleImportProducesZeroDuplicateSources(t *testing.T) {
	srv := startFakeServer(t)
	fixtures := loadFixtures(t)
	for _, f := range fixtures {
		srv.appendFixture(t, testMailbox, f)
	}

	c := newTestConnector(srv)
	root := t.TempDir()
	ss := store.NewSourceStore(root)
	ctx := context.Background()

	items1, cursor1, err := c.Poll(ctx, nil)
	if err != nil {
		t.Fatalf("first Poll: %v", err)
	}
	if len(items1) != len(fixtures) {
		t.Fatalf("first Poll returned %d items, want %d", len(items1), len(fixtures))
	}
	for _, item := range items1 {
		if item.Kind != "email" {
			t.Errorf("item kind = %q, want %q", item.Kind, "email")
		}
	}
	writeItems(t, ss, items1, c)
	if got := countSources(t, root); got != len(fixtures) {
		t.Fatalf("after first poll: %d sources on disk, want %d", got, len(fixtures))
	}

	// Every fixture's exact bytes must have made it through untouched.
	want := make(map[string]bool, len(fixtures))
	for _, f := range fixtures {
		want[string(readFile(t, f))] = true
	}
	for _, item := range items1 {
		if !want[string(item.Bytes)] {
			t.Errorf("fetched message does not match any fixture (first 40 bytes: %q)", truncate(item.Bytes, 40))
		}
	}

	items2, _, err := c.Poll(ctx, cursor1)
	if err != nil {
		t.Fatalf("second Poll: %v", err)
	}
	if len(items2) != 0 {
		t.Fatalf("second Poll (no new mail) returned %d items, want 0", len(items2))
	}
	writeItems(t, ss, items2, c)
	if got := countSources(t, root); got != len(fixtures) {
		t.Fatalf("after double poll: %d sources on disk, want %d (zero duplicates)", got, len(fixtures))
	}
}

// TestPollIsIncremental proves a second Poll only returns mail that
// arrived after the first: the cursor names a resume point, not a replay.
func TestPollIsIncremental(t *testing.T) {
	srv := startFakeServer(t)
	fixtures := loadFixtures(t)
	for _, f := range fixtures {
		srv.appendFixture(t, testMailbox, f)
	}

	c := newTestConnector(srv)
	ctx := context.Background()

	items1, cursor1, err := c.Poll(ctx, nil)
	if err != nil {
		t.Fatalf("first Poll: %v", err)
	}
	if len(items1) != len(fixtures) {
		t.Fatalf("first Poll returned %d items, want %d", len(items1), len(fixtures))
	}

	newUID := srv.appendFixture(t, testMailbox, fixtures[0]) // same bytes, a genuinely new message/UID
	_ = newUID

	items2, cursor2, err := c.Poll(ctx, cursor1)
	if err != nil {
		t.Fatalf("second Poll: %v", err)
	}
	if len(items2) != 1 {
		t.Fatalf("second Poll returned %d items, want exactly the 1 new message", len(items2))
	}
	if string(items2[0].Bytes) != string(readFile(t, fixtures[0])) {
		t.Error("second Poll's item does not match the newly appended message")
	}

	items3, _, err := c.Poll(ctx, cursor2)
	if err != nil {
		t.Fatalf("third Poll: %v", err)
	}
	if len(items3) != 0 {
		t.Fatalf("third Poll (no new mail since cursor2) returned %d items, want 0", len(items3))
	}
}

// TestPollResetsCursorOnUIDValidityChange proves the pitfall fix: when the
// mailbox's UIDVALIDITY changes (simulated here by deleting and
// recreating INBOX, which is what a real IMAP server does on e.g. a
// full mailbox rebuild), Poll discards the stale cursor instead of
// treating the new server's UIDs as continuations of the old ones.
func TestPollResetsCursorOnUIDValidityChange(t *testing.T) {
	srv := startFakeServer(t)
	fixtures := loadFixtures(t)
	srv.appendFixture(t, testMailbox, fixtures[0])

	c := newTestConnector(srv)
	ctx := context.Background()

	_, cursor1, err := c.Poll(ctx, nil)
	if err != nil {
		t.Fatalf("first Poll: %v", err)
	}
	var state1 struct {
		UIDValidity uint32 `json:"uid_validity"`
		LastUID     uint32 `json:"last_uid"`
	}
	if err := json.Unmarshal(cursor1, &state1); err != nil {
		t.Fatalf("decode cursor1: %v", err)
	}
	if state1.LastUID == 0 {
		t.Fatal("cursor1.LastUID = 0 after a successful poll, want it to have advanced")
	}

	srv.recreateMailbox(t, testMailbox) // bumps UIDVALIDITY; new mailbox starts empty at UID 1
	srv.appendFixture(t, testMailbox, fixtures[1])
	srv.appendFixture(t, testMailbox, fixtures[2])

	items, cursor2, err := c.Poll(ctx, cursor1)
	if err != nil {
		t.Fatalf("Poll after UIDVALIDITY change: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("Poll after UIDVALIDITY change returned %d items, want 2 (the new mailbox's messages)", len(items))
	}

	var state2 struct {
		UIDValidity uint32 `json:"uid_validity"`
		LastUID     uint32 `json:"last_uid"`
	}
	if err := json.Unmarshal(cursor2, &state2); err != nil {
		t.Fatalf("decode cursor2: %v", err)
	}
	if state2.UIDValidity == state1.UIDValidity {
		t.Fatal("cursor2.UIDValidity == cursor1.UIDValidity, want the recreate to have bumped it")
	}
}

// TestPollSkipsMessageExpungedBetweenSearchAndFetch is the "handle
// EXPUNGE mid-fetch" pitfall test: a second connection deletes and
// expunges a message deterministically after the connector's UID SEARCH
// has already computed its result set but before its subsequent FETCH
// runs. Poll must not error, must return every still-present message, and
// must still advance the cursor past the expunged UID (it is gone, not
// pending retry).
func TestPollSkipsMessageExpungedBetweenSearchAndFetch(t *testing.T) {
	fixtures := loadFixtures(t)
	if len(fixtures) < 3 {
		t.Fatalf("need at least 3 fixtures, have %d", len(fixtures))
	}

	searchStarted := make(chan struct{})
	proceed := make(chan struct{})

	srv := startFakeServerWithSessionWrap(t, func(s imapserver.Session) imapserver.Session {
		return &searchHookSession{
			SessionIMAP4rev2: s.(imapserver.SessionIMAP4rev2),
			afterSearch: func() {
				close(searchStarted)
				<-proceed
			},
		}
	})

	uid1 := srv.appendFixture(t, testMailbox, fixtures[0])
	uid2 := srv.appendFixture(t, testMailbox, fixtures[1])
	_ = uid1
	srv.appendFixture(t, testMailbox, fixtures[2])

	go func() {
		<-searchStarted
		rc := srv.rawClient(t)
		if _, err := rc.Select(testMailbox, nil).Wait(); err != nil {
			t.Errorf("raw client select: %v", err)
		}
		var uids imapapi.UIDSet
		uids.AddNum(uid2)
		if _, err := rc.Store(uids, &imapapi.StoreFlags{
			Op:    imapapi.StoreFlagsAdd,
			Flags: []imapapi.Flag{imapapi.FlagDeleted},
		}, nil).Collect(); err != nil {
			t.Errorf("raw client mark deleted: %v", err)
		}
		if _, err := rc.Expunge().Collect(); err != nil {
			t.Errorf("raw client expunge: %v", err)
		}
		close(proceed)
	}()

	c := newTestConnector(srv) // BatchSize 1: SEARCH once, then one FETCH per UID
	items, cursor, err := c.Poll(context.Background(), nil)
	if err != nil {
		t.Fatalf("Poll with a concurrent mid-fetch expunge: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2 (the message expunged between SEARCH and FETCH must be skipped, not errored)", len(items))
	}

	var state struct {
		UIDValidity uint32 `json:"uid_validity"`
		LastUID     uint32 `json:"last_uid"`
	}
	if err := json.Unmarshal(cursor, &state); err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if state.LastUID < 3 {
		t.Fatalf("cursor.LastUID = %d, want it to have advanced past the expunged UID (>= 3)", state.LastUID)
	}

	// A follow-up poll must see no further new mail -- the expunged
	// message must never be retried.
	items2, _, err := c.Poll(context.Background(), cursor)
	if err != nil {
		t.Fatalf("follow-up Poll: %v", err)
	}
	if len(items2) != 0 {
		t.Fatalf("follow-up Poll returned %d items, want 0", len(items2))
	}
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
