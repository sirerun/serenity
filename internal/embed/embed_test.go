package embed

import (
	"context"
	"errors"
	"testing"

	"github.com/sirerun/serenity/internal/router"
)

// fakeProvider is a test double implementing router.Provider (zero-stub
// policy: test doubles live in _test.go files only). It always answers
// TaskClassEmbedding-shaped calls with a fixed JSON-array vector so
// RouterEmbedder tests never touch a network.
type fakeProvider struct {
	modelVersion string
	text         string
	err          error
	calls        int
}

func (f *fakeProvider) Name() string         { return "fake" }
func (f *fakeProvider) ModelVersion() string { return f.modelVersion }
func (f *fakeProvider) Send(_ context.Context, _ string) (router.Response, error) {
	f.calls++
	if f.err != nil {
		return router.Response{}, f.err
	}
	return router.Response{Text: f.text}, nil
}

// fakeLedger is a test double implementing router.SpendLedger.
type fakeLedger struct{ n int }

func (f *fakeLedger) Record(_ context.Context, _ router.SpendEntry) error {
	f.n++
	return nil
}

func newTestRouter(fp *fakeProvider) *router.Router {
	return router.New(map[router.Tier]router.Provider{router.TierLocalCheap: fp}, &fakeLedger{})
}

func TestRouterEmbedderDecodesVectorAndReturnsPin(t *testing.T) {
	fp := &fakeProvider{modelVersion: "fake-embed@v1", text: "[0.5,-0.25,0]"}
	e := &RouterEmbedder{Router: newTestRouter(fp), Pin: "fake-embed@v1"}

	vec, err := e.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatal(err)
	}
	want := []float32{0.5, -0.25, 0}
	if len(vec) != len(want) {
		t.Fatalf("Embed vector = %v, want %v", vec, want)
	}
	for i := range want {
		if vec[i] != want[i] {
			t.Fatalf("Embed vector = %v, want %v", vec, want)
		}
	}
	if e.ModelVersion() != "fake-embed@v1" {
		t.Fatalf("ModelVersion() = %q, want %q", e.ModelVersion(), "fake-embed@v1")
	}
	if fp.calls != 1 {
		t.Fatalf("provider called %d times, want 1", fp.calls)
	}
}

func TestRouterEmbedderErrorsOnPinMismatch(t *testing.T) {
	// The provider actually wired into the router reports a different
	// model@version than the pin this Embedder was configured with --
	// e.g. serenity.yml still says "old-model@v1" but the tier's
	// provider was repointed without updating the pin. This must error,
	// never silently store a vector under the wrong key.
	fp := &fakeProvider{modelVersion: "different-model@v2", text: "[1,2,3]"}
	e := &RouterEmbedder{Router: newTestRouter(fp), Pin: "fake-embed@v1"}

	_, err := e.Embed(context.Background(), "hello")
	if !errors.Is(err, ErrPinMismatch) {
		t.Fatalf("Embed error = %v, want ErrPinMismatch", err)
	}
}

func TestRouterEmbedderPropagatesProviderError(t *testing.T) {
	fp := &fakeProvider{modelVersion: "fake-embed@v1", err: errors.New("network down")}
	e := &RouterEmbedder{Router: newTestRouter(fp), Pin: "fake-embed@v1"}

	if _, err := e.Embed(context.Background(), "hello"); err == nil {
		t.Fatal("expected an error when the provider call fails")
	}
}

func TestRouterEmbedderErrorsOnUndecodableText(t *testing.T) {
	fp := &fakeProvider{modelVersion: "fake-embed@v1", text: "not a json array"}
	e := &RouterEmbedder{Router: newTestRouter(fp), Pin: "fake-embed@v1"}

	if _, err := e.Embed(context.Background(), "hello"); err == nil {
		t.Fatal("expected an error decoding a non-JSON-array response")
	}
}

func TestRouterEmbedderErrorsOnEmptyVector(t *testing.T) {
	fp := &fakeProvider{modelVersion: "fake-embed@v1", text: "[]"}
	e := &RouterEmbedder{Router: newTestRouter(fp), Pin: "fake-embed@v1"}

	if _, err := e.Embed(context.Background(), "hello"); err == nil {
		t.Fatal("expected an error decoding an empty vector")
	}
}
