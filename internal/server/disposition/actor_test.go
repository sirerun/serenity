package disposition

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	coredisp "github.com/sirerun/serenity/internal/disposition"
)

// TestDisposeRecordsBearerPrincipalNotWireActor is AI-03's first half: a
// bearer caller that claims `human:david` on the wire is recorded as the
// transport principal, `agent:<credential id>`, never as the claimed
// human. The wire actor field is advisory and never reaches the store.
func TestDisposeRecordsBearerPrincipalNotWireActor(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	items := seedItems(t, ctx, env.dispStore, 1, fixedNow)
	base, token := startTestServer(t, env)

	resp := postJSON(t, base, token, "/disposition/dispose", DisposeRequest{
		ItemID: items[0].ID, Verdict: string(coredisp.VerdictAccept),
		IdempotencyKey: "actor-forgery", Actor: "human:david",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	var out DisposeResponse
	if err := json.Unmarshal(readBody(t, resp), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(out.Results) != 1 || out.Results[0].Item.Actor != "agent:daemon" {
		t.Fatalf("response results = %+v, want one item with actor agent:daemon", out.Results)
	}
	stored, err := env.dispStore.Get(ctx, items[0].ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Actor != "agent:daemon" {
		t.Fatalf("stored actor = %q, want agent:daemon -- the wire actor human:david must never be recorded", stored.Actor)
	}
}

// TestDisposeLedgerAcceptOverHTTPIsForbidden is AI-03's second half: an
// accept (or edit_accept) of a precept_draft or decompose item over the
// bearer transport is refused with 403 before anything is recorded, so a
// ledger-bound approval can only ever come from the CLI channel. The item
// stays pending for a human to review in `serenity inbox`.
func TestDisposeLedgerAcceptOverHTTPIsForbidden(t *testing.T) {
	for _, kind := range []coredisp.Kind{coredisp.KindPreceptDraft, coredisp.KindDecompose} {
		for _, verdict := range []coredisp.Verdict{coredisp.VerdictAccept, coredisp.VerdictEditAccept} {
			t.Run(string(kind)+"/"+string(verdict), func(t *testing.T) {
				env := newTestEnv(t)
				ctx := context.Background()
				item, err := env.dispStore.Create(ctx, kind, json.RawMessage(`{"title":"Agents may deploy on Fridays"}`), "", fixedNow)
				if err != nil {
					t.Fatalf("Create: %v", err)
				}
				base, token := startTestServer(t, env)
				resp := postJSON(t, base, token, "/disposition/dispose", DisposeRequest{
					ItemID: item.ID, Verdict: string(verdict), EditedPayload: json.RawMessage(`{"title":"edited"}`),
					IdempotencyKey: "self-accept", Actor: "human:david",
				})
				body := readBody(t, resp)
				if resp.StatusCode != http.StatusForbidden {
					t.Fatalf("status = %d, want 403; body %s", resp.StatusCode, body)
				}
				var pe ProtoError
				if err := json.Unmarshal(body, &pe); err != nil || pe.Code != "forbidden" {
					t.Fatalf("error envelope = %s (%v), want code forbidden", body, err)
				}
				stored, err := env.dispStore.Get(ctx, item.ID)
				if err != nil {
					t.Fatalf("Get: %v", err)
				}
				if stored.State != coredisp.StatePending || stored.Actor != "" {
					t.Fatalf("stored item = state %q actor %q, want untouched pending", stored.State, stored.Actor)
				}
			})
		}
	}
}

// TestDisposeLedgerGroupWithPreceptIsForbiddenBeforeAnyWrite proves a
// group dispose is checked in full before the first member is written: a
// group mixing an ordinary item with a precept_draft is refused and
// neither member is disposed.
func TestDisposeLedgerGroupWithPreceptIsForbiddenBeforeAnyWrite(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	plain, err := env.dispStore.Create(ctx, coredisp.KindDistill, json.RawMessage(`{"n":1}`), "grp", fixedNow)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	precept, err := env.dispStore.Create(ctx, coredisp.KindPreceptDraft, json.RawMessage(`{"title":"x"}`), "grp", fixedNow.Add(1))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	base, token := startTestServer(t, env)
	resp := postJSON(t, base, token, "/disposition/dispose", DisposeRequest{
		GroupID: "grp", Verdict: string(coredisp.VerdictAccept), IdempotencyKey: "group-self-accept",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body %s", resp.StatusCode, readBody(t, resp))
	}
	for _, id := range []string{plain.ID, precept.ID} {
		stored, err := env.dispStore.Get(ctx, id)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if stored.State != coredisp.StatePending {
			t.Fatalf("item %s state = %q, want pending", id, stored.State)
		}
	}
}

// TestDisposeLedgerRejectOverHTTPRecordsAgentActor proves the refusal is
// scoped to acceptance: an agent may still reject a precept_draft (with a
// note), and the rejection is attributed to the agent principal.
func TestDisposeLedgerRejectOverHTTPRecordsAgentActor(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()
	item, err := env.dispStore.Create(ctx, coredisp.KindPreceptDraft, json.RawMessage(`{"title":"x"}`), "", fixedNow)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	base, token := startTestServer(t, env)
	resp := postJSON(t, base, token, "/disposition/dispose", DisposeRequest{
		ItemID: item.ID, Verdict: string(coredisp.VerdictReject), Note: "duplicate",
		IdempotencyKey: "reject", Actor: "human:david",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	stored, err := env.dispStore.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Verdict != coredisp.VerdictReject || stored.Actor != "agent:daemon" {
		t.Fatalf("stored = verdict %q actor %q, want reject by agent:daemon", stored.Verdict, stored.Actor)
	}
}

// TestDisposeRecordsProfileCredentialAsActor proves the recorded actor
// follows the credential the transport authenticated: a server serving a
// --credential-profile credential records agent:<that credential id>.
func TestDisposeRecordsProfileCredentialAsActor(t *testing.T) {
	env := newTestEnv(t, WithCredentialID("profile:ci-agent"))
	ctx := context.Background()
	items := seedItems(t, ctx, env.dispStore, 1, fixedNow)
	base, token := startTestServer(t, env)
	resp := postJSON(t, base, token, "/disposition/dispose", DisposeRequest{
		ItemID: items[0].ID, Verdict: string(coredisp.VerdictDefer),
		IdempotencyKey: "profile-actor", Actor: "human:david",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", resp.StatusCode, readBody(t, resp))
	}
	stored, err := env.dispStore.Get(ctx, items[0].ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Actor != "agent:profile:ci-agent" {
		t.Fatalf("stored actor = %q, want agent:profile:ci-agent", stored.Actor)
	}
}
