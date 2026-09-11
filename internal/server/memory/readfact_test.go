package memory

import (
	"context"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sirerun/serenity/internal/router"
)

func mustRemember(t *testing.T, h *Handlers, ctx context.Context, req rememberRequest) rememberResponse {
	t.Helper()
	v, bad, err := h.remember(ctx, mustMarshal(t, req))
	if err != nil || bad {
		t.Fatalf("remember: %v %+v", err, v)
	}
	return v.(rememberResponse)
}

func readMemoryFactByID(t *testing.T, h *Handlers, ctx context.Context, id string) (readMemoryFactResponse, VerbError, bool) {
	t.Helper()
	v, bad, err := h.readMemoryFact(ctx, mustMarshal(t, map[string]string{"id": id}))
	if err != nil {
		t.Fatalf("read_memory_fact: unexpected internal error: %v", err)
	}
	if bad {
		return readMemoryFactResponse{}, v.(VerbError), true
	}
	return v.(readMemoryFactResponse), VerbError{}, false
}

// TestReadMemoryFactExactAndLegacyID is this task's "exact vs fuzzy ids"
// acc line, positive half: both exact-id forms remember/recall document
// (the opaque fact_id and the legacy decimal id) resolve the SAME live
// fact, with no search/entity fallback involved.
func TestReadMemoryFactExactAndLegacyID(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	rem := mustRemember(t, h, ctx, rememberRequest{
		Fact: "exact read target", Provenance: "test", Entity: "people/exact-read-target",
	})

	resp, verbErr, bad := readMemoryFactByID(t, h, ctx, rem.ID)
	if bad {
		t.Fatalf("read by opaque id failed: %+v", verbErr)
	}
	if resp.ID != rem.ID || resp.Fact != "exact read target" || resp.Provenance != "test" ||
		resp.Kind != "fact" || resp.Visibility != "world" || !resp.ContentUntrusted {
		t.Fatalf("unexpected response for opaque id: %+v", resp)
	}
	if resp.EntitySlug == nil || *resp.EntitySlug != "exact-read-target" {
		t.Fatalf("entity_slug not echoed: %+v", resp)
	}
	if resp.ValidUntil != nil {
		t.Fatalf("valid_until should be nil for a never-expiring fact: %+v", resp)
	}

	recallResp, bad2, err := h.recall(ctx, mustMarshal(t, recallRequest{Entity: "people/exact-read-target"}))
	if err != nil || bad2 {
		t.Fatalf("recall: %v %+v", err, recallResp)
	}
	facts := recallResp.(recallResponse).Facts
	if len(facts) != 1 || facts[0].FactID != rem.ID {
		t.Fatalf("recall did not return the remembered fact: %+v", facts)
	}
	legacyID := strconv.FormatInt(facts[0].ID, 10)

	byLegacy, verbErr2, bad3 := readMemoryFactByID(t, h, ctx, legacyID)
	if bad3 {
		t.Fatalf("read by legacy id failed: %+v", verbErr2)
	}
	if byLegacy.ID != rem.ID || byLegacy.Fact != "exact read target" {
		t.Fatalf("legacy id resolved to a different fact: %+v", byLegacy)
	}
}

// TestReadMemoryFactRejectsInvalidShapes is this task's "invalid ids" acc
// line: anything that is not the opaque fact_id shape or the legacy
// decimal shape is rejected with invalid_params before any lookup runs --
// a page slug, an entity reference, a search-shaped string, a truncated or
// wrong-case hex id, and a legacy id with a leading zero all count.
func TestReadMemoryFactRejectsInvalidShapes(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()
	rem := mustRemember(t, h, ctx, rememberRequest{Fact: "shape probe", Provenance: "test"})

	cases := map[string]string{
		"empty":                  "",
		"page_slug":              "people/alice",
		"search_query":           "onboarding decision",
		"truncated_sha":          rem.ID[:32],
		"uppercase_sha":          strings.ToUpper(rem.ID),
		"non_hex_chars":          "zz" + rem.ID[2:],
		"legacy_leading_zero":    "01",
		"legacy_too_many_digits": "99999999999999999999",
		"path_traversal":         "../../etc/passwd",
	}
	for name, id := range cases {
		t.Run(name, func(t *testing.T) {
			_, verbErr, bad := readMemoryFactByID(t, h, ctx, id)
			if !bad {
				t.Fatalf("id %q: want rejected, got a live response", id)
			}
			if verbErr.Error != ErrCodeInvalidParams {
				t.Fatalf("id %q: error = %q, want %q", id, verbErr.Error, ErrCodeInvalidParams)
			}
		})
	}
}

// TestReadMemoryFactUniformUnavailable is this task's own core acceptance
// line: a well-formed id that is missing, private, TTL-independent-but-
// forgotten, or operation-key-canceled must ALL return the identical
// unavailable envelope -- never not_found or scope_denied -- so a caller
// cannot learn from the failure shape whether a private/withdrawn fact
// exists under a given id (RFC-EXACT-READ-01's own stated constraint).
func TestReadMemoryFactUniformUnavailable(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	// A syntactically valid but never-written sha.
	missingID := strings.Repeat("0", 64)

	private := mustRemember(t, h, ctx, rememberRequest{Fact: "private probe", Provenance: "test", Visibility: "private"})

	forgotten := mustRemember(t, h, ctx, rememberRequest{Fact: "forget probe", Provenance: "test"})
	if _, bad, err := h.forget(ctx, mustMarshal(t, map[string]string{"id": forgotten.ID})); err != nil || bad {
		t.Fatalf("forget: %v", err)
	}

	keyed := mustRemember(t, h, ctx, rememberRequest{OperationKey: "exact-read-cancel-1", Fact: "cancel probe", Provenance: "test"})
	// Read BEFORE cancellation must succeed -- proves the writer's fresh
	// state is visible, not just its absence.
	if _, verbErr, bad := readMemoryFactByID(t, h, ctx, keyed.ID); bad {
		t.Fatalf("read before cancellation unexpectedly failed: %+v", verbErr)
	}
	if _, bad, err := h.cancelOperation(ctx, mustMarshal(t, map[string]string{"operation_key": "exact-read-cancel-1"})); err != nil || bad {
		t.Fatalf("cancel_memory_operation: %v", err)
	}

	cases := map[string]string{
		"missing":   missingID,
		"private":   private.ID,
		"forgotten": forgotten.ID,
		"canceled":  keyed.ID, // read AFTER cancellation must now fail.
	}

	var first VerbError
	for name, id := range cases {
		t.Run(name, func(t *testing.T) {
			_, verbErr, bad := readMemoryFactByID(t, h, ctx, id)
			if !bad {
				t.Fatalf("id %q (%s): want unavailable, got a live response", id, name)
			}
			if verbErr.Error != ErrCodeUnavailable {
				t.Fatalf("%s: error = %q, want %q", name, verbErr.Error, ErrCodeUnavailable)
			}
			if first.Error == "" {
				first = verbErr
				return
			}
			if verbErr.Message != first.Message || verbErr.Suggestion != first.Suggestion {
				t.Fatalf("%s: envelope diverges from the first case -- missing/private/expired/canceled must be indistinguishable\ngot:  %+v\nwant: %+v", name, verbErr, first)
			}
		})
	}
}

// TestReadMemoryFactNoProviderCall proves read_memory_fact never reaches
// the embedder or composer -- a poisoned double fails the test the moment
// either is invoked, so a silent regression toward a search/synthesis path
// is caught, not just documented.
func TestReadMemoryFactNoProviderCall(t *testing.T) {
	root := gitRepoFixture(t)
	deps, closeFn := testDeps(t, root)
	t.Cleanup(closeFn)

	var embedCalls, composeCalls atomic.Int32
	deps.Embedder = poisonEmbedder{calls: &embedCalls}
	deps.Composer = poisonComposer{calls: &composeCalls}
	h := New(deps)
	ctx := context.Background()

	rem := mustRemember(t, h, ctx, rememberRequest{Fact: "no provider probe", Provenance: "test"})
	if _, verbErr, bad := readMemoryFactByID(t, h, ctx, rem.ID); bad {
		t.Fatalf("read: %+v", verbErr)
	}
	if n := embedCalls.Load(); n != 0 {
		t.Fatalf("embedder called %d times, want 0", n)
	}
	if n := composeCalls.Load(); n != 0 {
		t.Fatalf("composer called %d times, want 0", n)
	}
}

type poisonEmbedder struct{ calls *atomic.Int32 }

func (poisonEmbedder) ModelVersion() string { return "poison@v1" }
func (p poisonEmbedder) Embed(context.Context, string) ([]float32, error) {
	p.calls.Add(1)
	return nil, nil
}

type poisonComposer struct{ calls *atomic.Int32 }

func (p poisonComposer) Complete(context.Context, router.TaskClass, router.Prompt, router.Budget) (router.Result, error) {
	p.calls.Add(1)
	return router.Result{}, nil
}
