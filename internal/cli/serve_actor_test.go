package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/sirerun/serenity/internal/secrets"
)

// TestServeHTTPDispositionActorIsTheServingCredential drives the real
// `serve --http --credential-profile` command: an agent proposes a
// precept_draft and an effect over DIRECTION, then claims `human:david`
// while disposing them over DISPOSITION. Accepting the precept is refused
// with 403, and the deferred effect is recorded under the serving
// credential, agent:profile:<name>, not under the claimed human.
func TestServeHTTPDispositionActorIsTheServingCredential(t *testing.T) {
	requireGit(t)
	root := pushFixture(t)
	if _, _, err := secrets.EnsureProfileDaemonToken("actor-agent"); err != nil {
		t.Fatal(err)
	}
	token, err := secrets.ProfileDaemonToken("actor-agent")
	if err != nil {
		t.Fatal(err)
	}
	addr, _, cancel, done := runningHTTPServeArgs(t, root, "--credential-profile", "actor-agent")
	defer func() {
		cancel()
		<-done
	}()

	post := func(path string, body any) (int, map[string]json.RawMessage) {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, addr+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		var decoded map[string]json.RawMessage
		_ = json.NewDecoder(resp.Body).Decode(&decoded)
		return resp.StatusCode, decoded
	}
	itemID := func(path string, body any) string {
		t.Helper()
		code, out := post(path, body)
		var id string
		if code != http.StatusOK || json.Unmarshal(out["item_id"], &id) != nil || id == "" {
			t.Fatalf("%s: status %d body %v", path, code, out)
		}
		return id
	}

	precept := itemID("/direction/propose", map[string]any{"kind": "precept_draft", "payload": map[string]any{
		"title": "Agents may deploy on Fridays", "alternatives": []map[string]string{{"option": "do not adopt this", "why_not": "floor"}},
	}})
	effect := itemID("/direction/propose", map[string]any{"kind": "effect", "payload": map[string]any{}})

	code, out := post("/disposition/dispose", map[string]any{"item_id": precept, "verdict": "accept", "idempotency_key": "k1", "actor": "human:david"})
	if code != http.StatusForbidden {
		t.Fatalf("self-accept of a precept over HTTP: status %d body %v, want 403", code, out)
	}
	code, out = post("/disposition/dispose", map[string]any{"item_id": effect, "verdict": "defer", "idempotency_key": "k2", "actor": "human:david"})
	if code != http.StatusOK {
		t.Fatalf("defer effect: status %d body %v", code, out)
	}
	var disposed struct {
		Results []struct {
			Item struct {
				Actor string `json:"actor"`
			} `json:"item"`
		} `json:"results"`
	}
	raw, _ := json.Marshal(out)
	if err := json.Unmarshal(raw, &disposed); err != nil || len(disposed.Results) != 1 {
		t.Fatalf("decode dispose response %s: %v", raw, err)
	}
	if got := disposed.Results[0].Item.Actor; got != "agent:profile:actor-agent" {
		t.Fatalf("recorded actor = %q, want agent:profile:actor-agent", got)
	}
}
