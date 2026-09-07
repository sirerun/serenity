package disposition

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	coredisp "github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/events"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/secrets"
	"github.com/sirerun/serenity/internal/server"
)

func TestMain(m *testing.M) {
	secrets.MockForTesting() // never touch the real OS keychain from tests
	m.Run()
}

var fixedNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

type fakeClock struct{ now time.Time }

func (f fakeClock) Now() time.Time { return f.now }

// testEnv bundles a real disposition.Store + events.Store over one
// index.Open engine (the same pattern internal/disposition and
// internal/events tests already use), plus Handlers over both.
type testEnv struct {
	dispStore *coredisp.Store
	evStore   *events.Store
	handlers  *Handlers
}

func newTestEnv(t *testing.T, opts ...Option) *testEnv {
	t.Helper()
	eng, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })

	dispStore := coredisp.NewStore(eng)
	evStore := events.NewStore(eng)
	allOpts := append([]Option{WithClock(fakeClock{fixedNow}), WithPollInterval(20 * time.Millisecond)}, opts...)
	h := New(dispStore, evStore, allOpts...)
	return &testEnv{dispStore: dispStore, evStore: evStore, handlers: h}
}

// startTestServer registers env's handlers onto a real *server.Server
// bound to loopback, serves it in the background, and returns the base
// URL plus the bearer token -- the same real-HTTP-listener convention
// T4.3's own server_test.go establishes ("hitting a real listener over
// real HTTP, not internal calls").
func startTestServer(t *testing.T, env *testEnv) (baseURL, token string) {
	t.Helper()
	const want = "s3cr3t-token"
	s := server.New(server.Config{Bind: "127.0.0.1:0", TokenSource: func() (string, error) { return want, nil }})
	env.handlers.Register(s)
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = s.Serve(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(8 * time.Second):
			// internal/server.Server's own shutdownGrace is 5s; give
			// real headroom above that rather than racing it -- a
			// test binary spinning up many servers in one run can
			// legitimately make Shutdown take longer than a tight
			// margin would tolerate.
			t.Fatal("server did not shut down within 8s")
		}
	})
	return "http://" + s.Addr(), want
}

func postJSON(t *testing.T, base, token, path string, body any) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, base+path, bytes.NewReader(data))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) []byte {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	return buf.Bytes()
}

func seedItems(t *testing.T, ctx context.Context, s *coredisp.Store, n int, now time.Time) []coredisp.Item {
	t.Helper()
	out := make([]coredisp.Item, 0, n)
	for i := 0; i < n; i++ {
		it, err := s.Create(ctx, coredisp.KindDistill, json.RawMessage(fmt.Sprintf(`{"n":%d}`, i)), "", now.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		out = append(out, it)
	}
	return out
}

// TestListPendingCursorWalkTerminates proves the acc line "cursor walk of
// 120 items in pages of 50 terminates": 120 pending items, walked in
// pages of 50 by following next_cursor, must return exactly 120 items
// across exactly 3 calls and end with an empty next_cursor.
func TestListPendingCursorWalkTerminates(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	seedItems(t, ctx, env.dispStore, 120, fixedNow)

	base, token := startTestServer(t, env)

	var seen []string
	cursor := ""
	calls := 0
	for {
		calls++
		if calls > 10 {
			t.Fatalf("cursor walk did not terminate within 10 calls, saw %d items", len(seen))
		}
		resp := postJSON(t, base, token, "/disposition/list_pending", listPendingRequest{Limit: 50, Cursor: cursor})
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
		}
		var out listPendingResponse
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		for _, row := range out.Items {
			seen = append(seen, row.Item.ID)
		}
		if out.NextCursor == "" {
			break
		}
		cursor = out.NextCursor
	}

	if calls != 3 {
		t.Fatalf("calls = %d, want 3 (50+50+20)", calls)
	}
	if len(seen) != 120 {
		t.Fatalf("total items seen = %d, want 120", len(seen))
	}
	dedup := make(map[string]bool, len(seen))
	for _, id := range seen {
		if dedup[id] {
			t.Fatalf("item %s seen twice across the cursor walk", id)
		}
		dedup[id] = true
	}
}

// TestDisposeReplayReturnsByteIdenticalResponse proves the acc line
// "replayed dispose returns a byte-identical response": once an item is
// disposed, every subsequent dispose call carrying the same
// idempotency_key is itself a replay (Replayed=true) and must produce
// byte-identical response bodies to every other such replay -- compared
// here across a second and third call, since the first call is the
// original disposition, not itself a replay of anything.
func TestDisposeReplayReturnsByteIdenticalResponse(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	items := seedItems(t, ctx, env.dispStore, 1, fixedNow)
	base, token := startTestServer(t, env)

	req := disposeRequest{ItemID: items[0].ID, Verdict: string(coredisp.VerdictAccept), IdempotencyKey: "replay-key-1", Actor: "tester"}

	original := readBody(t, postJSON(t, base, token, "/disposition/dispose", req))
	replay1 := readBody(t, postJSON(t, base, token, "/disposition/dispose", req))
	replay2 := readBody(t, postJSON(t, base, token, "/disposition/dispose", req))

	var out disposeResponse
	if err := json.Unmarshal(original, &out); err != nil {
		t.Fatalf("decode original response: %v", err)
	}
	if len(out.Results) != 1 || out.Results[0].Replayed {
		t.Fatalf("original dispose Results = %+v, want one Replayed=false result", out.Results)
	}

	if !bytes.Equal(replay1, replay2) {
		t.Fatalf("two replayed-dispose responses not byte-identical:\nreplay1: %s\nreplay2: %s", replay1, replay2)
	}
	if err := json.Unmarshal(replay2, &out); err != nil {
		t.Fatalf("decode replayed response: %v", err)
	}
	if len(out.Results) != 1 || !out.Results[0].Replayed {
		t.Fatalf("replayed dispose Results = %+v, want one Replayed=true result", out.Results)
	}
}

// TestDisposeRejectWithoutNoteReturnsProtocolError proves the acc line
// "reject without note -> protocol error": a reject verdict with an
// empty note must fail with a stable protocol error, never a silent
// reject.
func TestDisposeRejectWithoutNoteReturnsProtocolError(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	items := seedItems(t, ctx, env.dispStore, 1, fixedNow)
	base, token := startTestServer(t, env)

	req := disposeRequest{ItemID: items[0].ID, Verdict: string(coredisp.VerdictReject), IdempotencyKey: "reject-key-1"}
	resp := postJSON(t, base, token, "/disposition/dispose", req)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", resp.StatusCode, body)
	}
	var perr protoError
	if err := json.Unmarshal(body, &perr); err != nil {
		t.Fatalf("decode protocol error: %v", err)
	}
	if perr.Code != "reject_requires_note" {
		t.Fatalf("error code = %q, want %q", perr.Code, "reject_requires_note")
	}

	// The item must still be pending -- the rejected-without-note verdict
	// must never have been applied.
	got, err := env.dispStore.Get(ctx, items[0].ID)
	if err != nil {
		t.Fatalf("Get after failed dispose: %v", err)
	}
	if got.State != coredisp.StatePending {
		t.Fatalf("item state = %q after rejected-without-note dispose, want pending (unchanged)", got.State)
	}
}

// TestListPendingParkedFilter proves the acc line "parked items appear
// only with the parked filter": one item is parked (three expiry-sweep
// cycles), one stays pending; the default view must show only the
// pending item, and Parked=true must show only the parked one.
func TestListPendingParkedFilter(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	parked, err := env.dispStore.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", fixedNow)
	if err != nil {
		t.Fatalf("Create parked-to-be: %v", err)
	}
	parkedID := parked.ID

	// Sweep is store-wide, so it must run BEFORE the pending item exists,
	// or the pending item would age and park right alongside it. Three
	// sweeps, each past the 14-day default threshold, bring parkedID's
	// DeferCount to disposition.MaxDeferCycles (3) and park it.
	sweepNow := fixedNow
	for i := 0; i < 3; i++ {
		sweepNow = sweepNow.Add(15 * 24 * time.Hour)
		if _, err := coredisp.Sweep(ctx, env.dispStore, nil, sweepNow); err != nil {
			t.Fatalf("Sweep %d: %v", i, err)
		}
	}

	// Created fresh at sweepNow, after every Sweep call above -- stays
	// pending.
	pending, err := env.dispStore.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", sweepNow)
	if err != nil {
		t.Fatalf("Create pending: %v", err)
	}
	pendingID := pending.ID

	base, token := startTestServer(t, env)

	// Default (non-parked) view.
	resp := postJSON(t, base, token, "/disposition/list_pending", listPendingRequest{})
	var out listPendingResponse
	if err := json.Unmarshal(readBody(t, resp), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Items) != 1 || out.Items[0].Item.ID != pendingID {
		t.Fatalf("default view items = %+v, want exactly [%s]", out.Items, pendingID)
	}

	// Parked view.
	resp = postJSON(t, base, token, "/disposition/list_pending", listPendingRequest{Parked: true})
	if err := json.Unmarshal(readBody(t, resp), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Items) != 1 || out.Items[0].Item.ID != parkedID {
		t.Fatalf("parked view items = %+v, want exactly [%s]", out.Items, parkedID)
	}
}

// TestSubscribeSSEDropAndResumeReplaysExactlyMissedEvents proves the acc
// line "dropping the SSE connection mid-stream and resuming replays
// exactly the missed events": open an SSE stream, receive two events,
// drop the connection, publish two more events while disconnected,
// reconnect with Last-Event-ID set to the last-seen cursor, and confirm
// exactly the two missed events arrive -- no duplicates, nothing lost.
func TestSubscribeSSEDropAndResumeReplaysExactlyMissedEvents(t *testing.T) {
	env := newTestEnv(t)
	base, token := startTestServer(t, env)

	publish := func(n int) {
		for i := 0; i < n; i++ {
			resp := postJSON(t, base, token, "/disposition/capture", captureRequest{Text: fmt.Sprintf("note %d", i)})
			body := readBody(t, resp)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("capture status = %d, body = %s", resp.StatusCode, body)
			}
		}
	}

	// First connection: no cursor, expect events for cursor 1 and 2.
	publish(2)
	sr1, cancel1 := openSSE(t, base, token, "")
	got1 := readSSEEvents(t, sr1, 2)
	cancel1() // drop the connection mid-stream
	if got1[0].Cursor != 1 || got1[1].Cursor != 2 {
		t.Fatalf("first connection cursors = %v, want [1 2]", cursorsOf(got1))
	}

	// While disconnected, two more events land (cursor 3, 4).
	publish(2)

	// Reconnect with Last-Event-ID = 2 (the last one seen).
	sr2, cancel2 := openSSEWithLastEventID(t, base, token, "2")
	defer cancel2()
	got2 := readSSEEvents(t, sr2, 2)
	if got2[0].Cursor != 3 || got2[1].Cursor != 4 {
		t.Fatalf("resumed connection cursors = %v, want [3 4] (exactly the missed events)", cursorsOf(got2))
	}
}

func cursorsOf(evs []events.Event) []int64 {
	out := make([]int64, len(evs))
	for i, e := range evs {
		out[i] = e.Cursor
	}
	return out
}

// openSSE opens a GET /disposition/subscribe request with
// Accept: text/event-stream and an optional ?cursor= query param,
// returning a buffered reader over the response body and a cancel func
// that both aborts the request context and closes the body.
func openSSE(t *testing.T, base, token, cursor string) (*bufio.Reader, func()) {
	t.Helper()
	return openSSEWith(t, base, token, "", cursor)
}

func openSSEWithLastEventID(t *testing.T, base, token, lastEventID string) (*bufio.Reader, func()) {
	t.Helper()
	return openSSEWith(t, base, token, lastEventID, "")
}

func openSSEWith(t *testing.T, base, token, lastEventID, cursor string) (*bufio.Reader, func()) {
	t.Helper()
	url := base + "/disposition/subscribe"
	if cursor != "" {
		url += "?cursor=" + cursor
	}
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		cancel()
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "text/event-stream")
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("GET subscribe: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body := readBody(t, resp)
		cancel()
		t.Fatalf("subscribe status = %d, body = %s", resp.StatusCode, body)
	}
	closeAll := func() {
		cancel()
		_ = resp.Body.Close()
	}
	return bufio.NewReader(resp.Body), closeAll
}

// readSSEEvents reads exactly n "id: N\ndata: {...}\n\n" blocks, each
// bounded by its own deadline so a stalled/broken stream fails the test
// clearly instead of hanging.
func readSSEEvents(t *testing.T, r *bufio.Reader, n int) []events.Event {
	t.Helper()
	out := make([]events.Event, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, readOneSSEEvent(t, r))
	}
	return out
}

func readOneSSEEvent(t *testing.T, r *bufio.Reader) events.Event {
	t.Helper()
	type result struct {
		ev  events.Event
		err error
	}
	ch := make(chan result, 1)
	go func() {
		idLine, err := r.ReadString('\n')
		if err != nil {
			ch <- result{err: fmt.Errorf("read id line: %w", err)}
			return
		}
		dataLine, err := r.ReadString('\n')
		if err != nil {
			ch <- result{err: fmt.Errorf("read data line: %w", err)}
			return
		}
		if _, err := r.ReadString('\n'); err != nil { // trailing blank line
			ch <- result{err: fmt.Errorf("read blank line: %w", err)}
			return
		}
		if !strings.HasPrefix(idLine, "id: ") {
			ch <- result{err: fmt.Errorf("unexpected id line %q", idLine)}
			return
		}
		if !strings.HasPrefix(dataLine, "data: ") {
			ch <- result{err: fmt.Errorf("unexpected data line %q", dataLine)}
			return
		}
		var ev events.Event
		data := strings.TrimPrefix(strings.TrimRight(dataLine, "\n"), "data: ")
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			ch <- result{err: fmt.Errorf("decode event data %q: %w", data, err)}
			return
		}
		ch <- result{ev: ev}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatalf("readOneSSEEvent: %v", res.err)
		}
		return res.ev
	case <-time.After(5 * time.Second):
		t.Fatal("readOneSSEEvent: timed out waiting for an SSE event")
		return events.Event{}
	}
}

// TestCaptureStagesDistillItemAndPublishesEvent covers capture's happy
// path end to end: the returned item_id round-trips through Get as a
// pending KindDistill item, and exactly one event is appended.
func TestCaptureStagesDistillItemAndPublishesEvent(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	base, token := startTestServer(t, env)

	resp := postJSON(t, base, token, "/disposition/capture", captureRequest{Text: "buy milk"})
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var out captureResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	item, err := env.dispStore.Get(ctx, out.ItemID)
	if err != nil {
		t.Fatalf("Get %s: %v", out.ItemID, err)
	}
	if item.Kind != coredisp.KindDistill || item.State != coredisp.StatePending {
		t.Fatalf("item = %+v, want KindDistill/StatePending", item)
	}

	evs, err := env.evStore.Replay(ctx, 0)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(evs) != 1 || evs[0].Kind != "disposition.item_created" {
		t.Fatalf("events = %+v, want exactly one disposition.item_created", evs)
	}
}

// TestCaptureEmptyReturnsProtocolError proves capture with neither text
// nor audio_ref fails with a stable protocol error rather than staging
// an empty item.
func TestCaptureEmptyReturnsProtocolError(t *testing.T) {
	env := newTestEnv(t)
	base, token := startTestServer(t, env)

	resp := postJSON(t, base, token, "/disposition/capture", captureRequest{})
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", resp.StatusCode, body)
	}
	var perr protoError
	if err := json.Unmarshal(body, &perr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if perr.Code != "capture_empty" {
		t.Fatalf("error code = %q, want capture_empty", perr.Code)
	}
}

// TestDisposeGroupDisposesEveryMemberIndividually proves a group_id
// dispose applies the verdict to every member sharing that GroupID, each
// as its own Dispose call (RFC 0001 §8.2: "each recorded individually
// for the ladder") -- not one bulk write.
func TestDisposeGroupDisposesEveryMemberIndividually(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	const group = "group-1"
	var members []coredisp.Item
	for i := 0; i < 3; i++ {
		it, err := env.dispStore.Create(ctx, coredisp.KindDecompose, json.RawMessage(`{}`), group, fixedNow)
		if err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
		members = append(members, it)
	}
	base, token := startTestServer(t, env)

	req := disposeRequest{GroupID: group, Verdict: string(coredisp.VerdictAccept), IdempotencyKey: "group-key-1"}
	resp := postJSON(t, base, token, "/disposition/dispose", req)
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	var out disposeResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Results) != 3 {
		t.Fatalf("Results count = %d, want 3", len(out.Results))
	}
	for _, m := range members {
		got, err := env.dispStore.Get(ctx, m.ID)
		if err != nil {
			t.Fatalf("Get %s: %v", m.ID, err)
		}
		if got.State != coredisp.StateDisposed || got.Verdict != coredisp.VerdictAccept {
			t.Fatalf("member %s = %+v, want disposed/accept", m.ID, got)
		}
	}

	// Every genuinely new dispose in the group published its own event.
	evs, err := env.evStore.Replay(ctx, 0)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(evs) != 3 {
		t.Fatalf("events after group dispose = %d, want 3", len(evs))
	}
}

// TestDisposeGroupIDWithNoMembersReturnsNotFound proves a group_id that
// matches no item returns a protocol error rather than a silent no-op
// success.
func TestDisposeGroupIDWithNoMembersReturnsNotFound(t *testing.T) {
	env := newTestEnv(t)
	base, token := startTestServer(t, env)

	req := disposeRequest{GroupID: "does-not-exist", Verdict: string(coredisp.VerdictAccept), IdempotencyKey: "k"}
	resp := postJSON(t, base, token, "/disposition/dispose", req)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

// TestDisposeMissingIdempotencyKeyReturnsProtocolError proves the wire
// enforces idempotency_key as required (RFC 0001 §8.2 gives it no `?`,
// unlike edited_payload/note).
func TestDisposeMissingIdempotencyKeyReturnsProtocolError(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	items := seedItems(t, ctx, env.dispStore, 1, fixedNow)
	base, token := startTestServer(t, env)

	req := disposeRequest{ItemID: items[0].ID, Verdict: string(coredisp.VerdictAccept)}
	resp := postJSON(t, base, token, "/disposition/dispose", req)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// TestListPendingKindFilter proves the kinds filter narrows the result
// set to exactly the requested kinds.
func TestListPendingKindFilter(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	if _, err := env.dispStore.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", fixedNow); err != nil {
		t.Fatalf("Create distill: %v", err)
	}
	reconcileItem, err := env.dispStore.Create(ctx, coredisp.KindReconcile, json.RawMessage(`{}`), "", fixedNow.Add(time.Second))
	if err != nil {
		t.Fatalf("Create reconcile: %v", err)
	}
	base, token := startTestServer(t, env)

	resp := postJSON(t, base, token, "/disposition/list_pending", listPendingRequest{Kinds: []string{string(coredisp.KindReconcile)}})
	var out listPendingResponse
	if err := json.Unmarshal(readBody(t, resp), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Items) != 1 || out.Items[0].Item.ID != reconcileItem.ID {
		t.Fatalf("filtered items = %+v, want exactly [%s]", out.Items, reconcileItem.ID)
	}
}

// TestSubscribeLongPollReturnsImmediatelyWhenEventsAlreadyExist proves
// the long-poll fallback (no Accept: text/event-stream) returns as soon
// as events already exist past the given cursor, without waiting out
// the full long-poll timeout.
func TestSubscribeLongPollReturnsImmediatelyWhenEventsAlreadyExist(t *testing.T) {
	env := newTestEnv(t, WithLongPollWait(2*time.Second))
	ctx := context.Background()
	if _, err := env.evStore.AppendEvent(ctx, "disposition.item_created", json.RawMessage(`{}`)); err != nil {
		t.Fatalf("Append: %v", err)
	}
	base, token := startTestServer(t, env)

	// A dedicated, non-keep-alive client for this one request: reusing
	// http.DefaultClient's pooled/keep-alive connection here was
	// observed to make the *server.Server's later graceful Shutdown (in
	// this test's own t.Cleanup) run out its full internal grace period
	// rather than returning promptly -- disabling keep-alives for this
	// request sidesteps that without weakening what's actually asserted
	// (the long-poll response content and its latency).
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}

	req, err := http.NewRequest(http.MethodGet, base+"/disposition/subscribe?cursor=0", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET subscribe: %v", err)
	}
	body := readBody(t, resp)
	elapsed := time.Since(start)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if elapsed > 1*time.Second {
		t.Fatalf("long-poll took %s, want well under the 2s timeout since an event already existed", elapsed)
	}
	var out longPollResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Events) != 1 {
		t.Fatalf("Events = %+v, want exactly 1", out.Events)
	}
}

// TestSubscribeRequiresAuth proves subscribe is authenticated like every
// other route (RFC 0001 §14: "every protocol endpoint authenticates") --
// covered as its own small test, with keep-alives off, rather than
// folded into the long-poll timing test above.
func TestSubscribeRequiresAuth(t *testing.T) {
	env := newTestEnv(t)
	base, _ := startTestServer(t, env)

	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Get(base + "/disposition/subscribe?cursor=0")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated subscribe status = %d, want 401", resp.StatusCode)
	}
}

// TestListPendingGroupCollapsesSharedGroupIDIntoOneRow proves the group
// wire flag collapses items sharing a non-empty GroupID into one row
// carrying every member, while an ungrouped item still appears on its
// own.
func TestListPendingGroupCollapsesSharedGroupIDIntoOneRow(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	const group = "g1"
	if _, err := env.dispStore.Create(ctx, coredisp.KindDecompose, json.RawMessage(`{}`), group, fixedNow); err != nil {
		t.Fatalf("Create member 1: %v", err)
	}
	if _, err := env.dispStore.Create(ctx, coredisp.KindDecompose, json.RawMessage(`{}`), group, fixedNow.Add(time.Second)); err != nil {
		t.Fatalf("Create member 2: %v", err)
	}
	if _, err := env.dispStore.Create(ctx, coredisp.KindDistill, json.RawMessage(`{}`), "", fixedNow.Add(2*time.Second)); err != nil {
		t.Fatalf("Create ungrouped: %v", err)
	}
	base, token := startTestServer(t, env)

	resp := postJSON(t, base, token, "/disposition/list_pending", listPendingRequest{Group: true})
	var out listPendingResponse
	if err := json.Unmarshal(readBody(t, resp), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Items) != 2 {
		t.Fatalf("rows = %d, want 2 (one grouped row + one ungrouped row)", len(out.Items))
	}
	grouped := out.Items[0]
	if len(grouped.Members) != 2 {
		t.Fatalf("grouped row Members = %+v, want 2 members", grouped.Members)
	}
	ungrouped := out.Items[1]
	if len(ungrouped.Members) != 0 {
		t.Fatalf("ungrouped row Members = %+v, want none", ungrouped.Members)
	}
}
