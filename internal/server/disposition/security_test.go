package disposition

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	coredisp "github.com/sirerun/serenity/internal/disposition"
)

// TestDisposeIdempotencyReplayIgnoresMutatedPayload extends T4.4's own
// TestDisposeReplayReturnsByteIdenticalResponse (proven for an honest
// retry: the exact same request body sent twice) to the adversarial
// angle T4.11's own acc line names: "replayed idempotency keys." An
// attacker who captured a legitimate idempotency_key and replays it with
// a DIFFERENT verdict/note/actor must get back the ORIGINAL, unmutated
// result -- never their own mutated payload applied.
//
// This is a proving test, not a bug fix: internal/disposition.Store.
// Dispose (disposition.go) fetches the item via s.Get at the very top of
// the call, before any per-call mutation, and its idempotency branch
// returns exactly that pre-fetched item -- the current call's verdict,
// editedPayload, note, and actor are read only far enough to pass the
// two validations that run before the idempotency check (verdict.valid(),
// and reject-requires-note), never applied. This test proves that
// behavior holds through the real HTTP wire path, not just at the
// Store's own Go API.
func TestDisposeIdempotencyReplayIgnoresMutatedPayload(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	items := seedItems(t, ctx, env.dispStore, 1, fixedNow)
	base, token := startTestServer(t, env)

	const key = "adversarial-replay-key"
	original := DisposeRequest{
		ItemID: items[0].ID, Verdict: string(coredisp.VerdictAccept),
		IdempotencyKey: key, Actor: "tester-original",
	}
	origBody := readBody(t, postJSON(t, base, token, "/disposition/dispose", original))
	var origOut DisposeResponse
	if err := json.Unmarshal(origBody, &origOut); err != nil {
		t.Fatalf("decode original: %v", err)
	}
	if len(origOut.Results) != 1 || origOut.Results[0].Replayed {
		t.Fatalf("original dispose = %+v, want one Replayed=false result", origOut.Results)
	}
	if origOut.Results[0].Item.Verdict != coredisp.VerdictAccept {
		t.Fatalf("original Item.Verdict = %q, want %q", origOut.Results[0].Item.Verdict, coredisp.VerdictAccept)
	}

	// Adversarial replay: same idempotency_key, but a mutated
	// verdict/note/actor -- crafted to still pass Dispose's own
	// pre-idempotency validation (a non-empty note, so "reject requires
	// note" does not itself short-circuit this call before the replay
	// branch is even reached).
	mutated := DisposeRequest{
		ItemID:         items[0].ID,
		Verdict:        string(coredisp.VerdictReject),
		Note:           "attacker-supplied rejection note",
		Actor:          "attacker",
		IdempotencyKey: key,
	}
	mutatedBody := readBody(t, postJSON(t, base, token, "/disposition/dispose", mutated))
	var mutatedOut DisposeResponse
	if err := json.Unmarshal(mutatedBody, &mutatedOut); err != nil {
		t.Fatalf("decode replay: %v", err)
	}
	if len(mutatedOut.Results) != 1 || !mutatedOut.Results[0].Replayed {
		t.Fatalf("mutated replay = %+v, want one Replayed=true result", mutatedOut.Results)
	}
	got := mutatedOut.Results[0].Item
	if got.Verdict != coredisp.VerdictAccept {
		t.Fatalf("replayed Item.Verdict = %q, want the ORIGINAL %q -- an attacker's mutated replay must never overwrite the recorded outcome", got.Verdict, coredisp.VerdictAccept)
	}
	if got.Note == mutated.Note {
		t.Fatalf("replayed Item.Note = %q, matches the attacker's mutated note -- the replay must return the original, unmutated item", got.Note)
	}
	if got.Actor == mutated.Actor {
		t.Fatalf("replayed Item.Actor = %q, matches the attacker's mutated actor", got.Actor)
	}

	// The store's own canonical record agrees -- not just this one
	// response payload.
	final, err := env.dispStore.Get(ctx, items[0].ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if final.Verdict != coredisp.VerdictAccept {
		t.Fatalf("stored item Verdict = %q after adversarial replay, want %q unchanged", final.Verdict, coredisp.VerdictAccept)
	}
	if final.Actor == mutated.Actor {
		t.Fatalf("stored item Actor = %q, matches the attacker's mutated actor -- the replay must never have written through", final.Actor)
	}
}

// TestDisposeRouteTraversalRejectedWith4xx is T4.11's own "path
// traversal" acc-line clause, applied to the HTTP transport (T4.3) +
// DISPOSITION (T4.4): every route this package registers is a fixed,
// literal pattern (Register's own four s.Handle calls) with no path
// parameters -- request identifiers travel in JSON bodies, never in the
// URL path -- so there is no application-level traversal vector to fix
// here. What this test verifies empirically, against a real listener,
// is that net/http.ServeMux's own path-cleaning behavior (which this
// package relies on rather than reimplementing) actually holds: a
// request whose path contains "../" segments is never dispatched to a
// DISPOSITION handler, and the client observes a 4xx once the
// redirect-then-lookup sequence resolves -- proven, not assumed, since
// this repo's own security posture for this surface depends on stdlib
// behavior it has never directly exercised before this task.
func TestDisposeRouteTraversalRejectedWith4xx(t *testing.T) {
	env := newTestEnv(t)
	base, token := startTestServer(t, env)

	paths := []string{
		"/disposition/../../../etc/passwd",
		"/disposition/dispose/../../../../etc/passwd",
		"/disposition/%2e%2e/%2e%2e/etc/passwd",
		"/../etc/passwd",
	}
	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, base+p, bytes.NewReader([]byte(`{}`)))
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request %s: %v", p, err)
			}
			body := readBody(t, resp)
			if resp.StatusCode < 400 || resp.StatusCode > 499 {
				t.Fatalf("path %q: status = %d, want a 4xx; body = %s", p, resp.StatusCode, body)
			}
		})
	}
}
