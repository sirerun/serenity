package authority

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/deletion"
)

type countTransport struct {
	next  Transport
	calls atomic.Int64
	gets  atomic.Int64
}

func (c *countTransport) Invoke(ctx context.Context, q Request) (Response, error) {
	c.calls.Add(1)
	if q.Action == "get" {
		c.gets.Add(1)
	}
	return c.next.Invoke(ctx, q)
}

type transportFunc func(context.Context, Request) (Response, error)

func (f transportFunc) Invoke(c context.Context, r Request) (Response, error) { return f(c, r) }

func TestTenThousandObjectStartupUsesBoundedPages(t *testing.T) {
	s, c, lease, now, _, _ := activeFixture(t)
	ctx := context.Background()
	previous := ""
	// Construct a complete immutable/witnessed corpus in bounded test batches.
	// Production append remains one validated canonical object per operation.
	for first := 1; first <= 10000; first += 80 {
		rec, e := s.kernel.Current(ctx)
		if e != nil {
			t.Fatal(e)
		}
		var state protocolState
		if e = strictDecode(rec.State, &state); e != nil {
			t.Fatal(e)
		}
		var items []ImmutableItem
		end := first + 79
		if end > 10000 {
			end = 10000
		}
		for n := first; n <= end; n++ {
			object := contracts.JournalObject{Kind: contracts.JournalKindEntry, Generation: 1, SequenceID: int64(n), PrevHash: previous, WriterID: lease.WriterID, SubjectType: contracts.DeletionSubjectAccount, SubjectID: fmt.Sprintf("synthetic-%05d", n), Outcome: contracts.DeletionIntentRequested, RecordedAt: *now}
			body, e := object.Encode()
			if e != nil {
				t.Fatal(e)
			}
			previous = contracts.HashObject(body)
			items = append(items, ImmutableItem{Key: "OBJ#" + contracts.JournalKey(1, int64(n)), Body: body})
			state.Position.LastObjectInGeneration = contracts.DeletionWatermark{Generation: 1, SequenceID: int64(n), EntryHash: previous}
		}
		encoded, e := json.Marshal(state)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = s.kernel.Apply(ctx, rec, encoded, items, Receipt{OperationID: opID(t), RequestSHA256: digest(encoded), Principal: "runtime"}, false); e != nil {
			t.Fatal(e)
		}
	}
	counter := &countTransport{next: c.transport}
	client, e := NewClient(counter)
	if e != nil {
		t.Fatal(e)
	}
	started := time.Now()
	challenge, e := client.Challenge(ctx)
	if e != nil {
		t.Fatal(e)
	}
	prepared, e := client.Prepare(ctx, challenge)
	if e != nil {
		t.Fatal(e)
	}
	reader, e := deletion.NewJournalReader(client.Reader(prepared), prepared.Position)
	if e != nil {
		t.Fatal(e)
	}
	observation, e := reader.Observe(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if observation.Read.To.SequenceID != 10000 {
		t.Fatal("incomplete fixed head")
	}
	active, e := client.Activate(ctx, prepared, observation.HistorySHA256, opID(t))
	if e != nil {
		t.Fatal(e)
	}
	// Service independently re-observes the fixed admission history after
	// activation, using a fresh page cache rather than a prebuilt replay array.
	reader, e = deletion.NewJournalReader(client.Reader(prepared), prepared.Position)
	if e != nil {
		t.Fatal(e)
	}
	again, e := reader.Observe(ctx)
	if e != nil || again.HistorySHA256 != observation.HistorySHA256 {
		t.Fatalf("service replay observation differs: %v", e)
	}
	if _, e = client.Current(ctx, active); e != nil {
		t.Fatal(e)
	}
	if counter.gets.Load() != 0 || counter.calls.Load() > 350 {
		t.Fatalf("unbounded network reads: calls=%d gets=%d", counter.calls.Load(), counter.gets.Load())
	}
	t.Logf("10000-object synthetic prepare/observe/activate/reobserve: %s; gateway calls=%d; individual object GETs=%d", time.Since(started), counter.calls.Load(), counter.gets.Load())
}
func TestPageByteBudgetNeverDropsContinuation(t *testing.T) {
	m := newMemoryAtomic()
	for n := int64(1); n <= 3; n++ {
		m.items["OBJ#"+contracts.JournalKey(1, n)] = bytes.Repeat([]byte("a"), MaxObjectBytes)
	}
	reader := boundedReader{store: m, position: contracts.JournalPosition{ActiveGeneration: 1, LastObjectInGeneration: contracts.DeletionWatermark{Generation: 1, SequenceID: 3}}, strict: true}
	after := ""
	for n := int64(1); n <= 3; n++ {
		page, e := reader.listPage(context.Background(), "deletion-journal/", after, 256)
		if e != nil {
			t.Fatal(e)
		}
		encoded, e := json.Marshal(page)
		if e != nil || len(encoded) > MaxPageBytes || len(page.Keys) != 1 || len(page.Objects) != 1 || page.Keys[0] != contracts.JournalKey(1, n) || page.More != (n < 3) {
			t.Fatalf("invalid bounded continuation: n=%d len=%d page=%+v", n, len(encoded), page.Keys)
		}
		after = page.Keys[0]
	}
}
func TestClientRejectsMissingOrMismatchedPageBodies(t *testing.T) {
	key := contracts.JournalKey(1, 1)
	for _, response := range []Response{{Keys: []string{key}}, {Keys: []string{key}, Objects: []ImmutableItem{{Key: contracts.JournalKey(1, 2), Body: []byte("x")}}}, {More: true}, {Keys: []string{key}, Objects: []ImmutableItem{{Key: key, Body: []byte(strings.Repeat("x", MaxObjectBytes+1))}}}} {
		client, _ := NewClient(transportFunc(func(context.Context, Request) (Response, error) { return response, nil }))
		if _, _, e := client.Reader(Prepared{ReservationID: strings.Repeat("a", 64)}).ListAfter(context.Background(), "deletion-journal/", "", 256); e == nil {
			t.Fatal("malformed page accepted")
		}
	}
}
