package memory

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

type recordingMemoryEmbedder struct {
	mu      sync.Mutex
	calls   []string
	fail    bool
	entered chan struct{}
	release chan struct{}
}

func (*recordingMemoryEmbedder) ModelVersion() string { return "memory-test@v1" }
func (e *recordingMemoryEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	e.mu.Lock()
	e.calls = append(e.calls, text)
	fail := e.fail
	e.mu.Unlock()
	if e.entered != nil {
		close(e.entered)
		select {
		case <-e.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if fail {
		return nil, errors.New("provider unavailable")
	}
	return []float32{1, 0}, nil
}
func (e *recordingMemoryEmbedder) count() int { e.mu.Lock(); defer e.mu.Unlock(); return len(e.calls) }

func TestRememberSemanticReadinessAndRetry(t *testing.T) {
	h, _ := newTestHandlers(t)
	e := &recordingMemoryEmbedder{}
	h.deps.Embedder = e
	request := rememberRequest{OperationKey: "ready", Fact: "fresh attributed fact", Provenance: "test"}
	remember := func() rememberResponse {
		t.Helper()
		v, bad, err := h.remember(context.Background(), mustMarshal(t, request))
		if err != nil || bad {
			t.Fatalf("remember failed: %v %+v", err, v)
		}
		return v.(rememberResponse)
	}
	first := remember()
	if first.SearchState != "semantic" || e.count() != 1 {
		t.Fatalf("not semantically indexed: %+v calls=%d", first, e.count())
	}
	has, err := h.deps.Index.HasVector(context.Background(), "fact:"+first.ID, e.ModelVersion())
	if err != nil || !has {
		t.Fatal("semantic readiness without vector", err)
	}
	again := remember()
	if again.ID != first.ID || again.SearchState != "semantic" || e.count() != 1 {
		t.Fatal("exact retry re-embedded unchanged fact")
	}
	if _, bad, err := h.forget(context.Background(), mustMarshal(t, map[string]string{"id": first.ID})); err != nil || bad {
		t.Fatal("forget failed")
	}
	withdrawn := remember()
	if withdrawn.SearchState != "not_eligible" || !withdrawn.Expired || e.count() != 1 {
		t.Fatal("withdrawn retry re-entered embedding")
	}
}

func TestRememberProviderFailurePreservesCanonicalWrite(t *testing.T) {
	h, _ := newTestHandlers(t)
	e := &recordingMemoryEmbedder{fail: true}
	h.deps.Embedder = e
	request := rememberRequest{OperationKey: "recover-provider", Fact: "durable despite provider failure", Provenance: "test"}
	v, bad, err := h.remember(context.Background(), mustMarshal(t, request))
	if err != nil || bad {
		t.Fatal("provider failure became failed canonical write")
	}
	first := v.(rememberResponse)
	if first.SearchState != "lexical" {
		t.Fatalf("state=%s", first.SearchState)
	}
	projection, err := store.LoadMemoryProjection(h.deps.Sources)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := projection.Get(first.ID); !ok {
		t.Fatal("canonical fact missing")
	}
	e.mu.Lock()
	e.fail = false
	e.mu.Unlock()
	v, bad, err = h.remember(context.Background(), mustMarshal(t, request))
	if err != nil || bad {
		t.Fatal("recovery failed")
	}
	recovered := v.(rememberResponse)
	if recovered.ID != first.ID || recovered.SearchState != "semantic" {
		t.Fatal("recovery changed fact or did not index")
	}
}

func TestRememberPrivateNeverEntersEmbedding(t *testing.T) {
	h, _ := newTestHandlers(t)
	e := &recordingMemoryEmbedder{}
	h.deps.Embedder = e
	v, bad, err := h.remember(context.Background(), mustMarshal(t, rememberRequest{Fact: "private marker", Provenance: "test", Visibility: "private"}))
	if err != nil || bad {
		t.Fatal("private write failed")
	}
	if v.(rememberResponse).SearchState != "not_eligible" || e.count() != 0 {
		t.Fatal("private source entered provider")
	}
	// Positive control proves the same configured provider is usable for eligible data.
	_, bad, err = h.remember(context.Background(), mustMarshal(t, rememberRequest{Fact: "shared marker", Provenance: "test"}))
	if err != nil || bad || e.count() != 1 {
		t.Fatal("eligible control did not embed")
	}
}

func TestRememberEmbeddingSerializesWithdrawal(t *testing.T) {
	h, _ := newTestHandlers(t)
	e := &recordingMemoryEmbedder{entered: make(chan struct{}), release: make(chan struct{})}
	h.deps.Embedder = e
	remembered := make(chan error, 1)
	go func() {
		_, bad, err := h.remember(context.Background(), []byte(`{"fact":"ordered egress fixture","provenance":"test"}`))
		if bad && err == nil {
			err = errors.New("remember rejected")
		}
		remembered <- err
	}()
	select {
	case <-e.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("embedding did not start")
	}
	projection, err := store.LoadMemoryProjection(h.deps.Sources)
	if err != nil {
		t.Fatal(err)
	}
	records := projection.All()
	if len(records) != 1 {
		t.Fatal("missing canonical positive control")
	}
	forgotten := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		_, bad, err := h.forget(context.Background(), mustMarshal(t, map[string]string{"id": records[0].SHA256}))
		if bad && err == nil {
			err = errors.New("forget rejected")
		}
		forgotten <- err
	}()
	<-started
	select {
	case err := <-forgotten:
		t.Fatalf("withdrawal passed an active provider decision: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(e.release)
	if err := <-remembered; err != nil {
		t.Fatal(err)
	}
	if err := <-forgotten; err != nil {
		t.Fatal(err)
	}
	projection, err = store.LoadMemoryProjection(h.deps.Sources)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := projection.Get(records[0].SHA256)
	if !ok || !rec.Expired(testNow) {
		t.Fatal("withdrawal was not durable")
	}
}
