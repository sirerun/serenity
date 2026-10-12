package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/meter"
	brainstore "github.com/sirerun/serenity/internal/store"
)

const governedBrain = "00000000-0000-4000-8000-000000000001"

func governedTestAction(kind, id string) governedAction {
	return governedAction{Schema: "zati.serenity.action/v1", BrainID: governedBrain, CommandID: id, Kind: kind, MaximumCost: &brainstore.GovernedMoney{Currency: "USD"}, AllowedDestinations: []string{}}
}
func governedTestCall(t *testing.T, g *Gateway, token string, req governedRequest) (int, brainstore.GovernedResponse, string) {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/memory/v1/commands", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.ServeGovernedMemory(w, r)
	var response brainstore.GovernedResponse
	if w.Code == 200 {
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code, response, w.Body.String()
}
func TestGovernedMemoryRoundTripAndSharedProjection(t *testing.T) {
	ctx := context.Background()
	g, p, s, accountID, brainID, _ := newLifecycleExportFixture(t)
	g.Meter = &meter.Meter{Store: s}
	token, err := g.Issuer.Issue(ctx, brainID, accountID, []string{"memory:read", "memory:write"})
	if err != nil {
		t.Fatal(err)
	}
	action := governedTestAction("remember", "00000000-0000-4000-8000-000000000002")
	action.WriterOwner = accountID
	action.Text = "User prefers concise summaries"
	action.Sources = []brainstore.GovernedArtifact{}
	req := governedRequest{Schema: brainstore.GovernedMemorySchema, ProjectID: brainID, Action: action}
	code, response, body := governedTestCall(t, g, token, req)
	if code != 200 {
		t.Fatalf("remember: %d %s", code, body)
	}
	if len(response.Claims) != 1 || response.Claims[0].Confidence != nil || !response.Claims[0].Active || response.Claims[0].Version != 1 {
		t.Fatalf("claim: %+v", response)
	}
	claim := response.Claims[0]
	code, replayed, body := governedTestCall(t, g, token, req)
	if code != 200 || replayed.Claims[0].ID != claim.ID {
		t.Fatalf("replay %d %s", code, body)
	}
	req.Action.Text = "different"
	if code, _, _ := governedTestCall(t, g, token, req); code != 409 {
		t.Fatalf("conflict %d", code)
	}
	req.Action.Text = action.Text
	r := httptest.NewRequest("GET", "/memory/v1/commands/"+action.CommandID+"?project_id="+brainID+"&brain_id="+governedBrain, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	g.ServeGovernedMemory(w, r)
	if w.Code != 200 {
		t.Fatalf("lookup %d %s", w.Code, w.Body.String())
	}
	runtime, release, err := p.Acquire(ctx, brainID)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := brainstore.LoadMemoryProjection(runtime.Sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy.All()) != 1 || legacy.All()[0].Payload.Fact != action.Text {
		t.Fatalf("shared projection: %+v", legacy.All())
	}
	release()
	recall := governedTestAction("recall", "00000000-0000-4000-8000-000000000003")
	recall.Query = "summaries"
	recall.MaxClaims = 20
	recall.MinimumFreshness = time.Now().Add(-time.Hour).UTC()
	recall.Classification = "internal"
	req.Action = recall
	if code, res, body := governedTestCall(t, g, token, req); code != 200 || len(res.Claims) != 1 {
		t.Fatalf("recall %d %s", code, body)
	}
	retract := governedTestAction("retract", "00000000-0000-4000-8000-000000000004")
	retract.WriterOwner = accountID
	retract.Claim = &governedRef{ID: claim.ID, Version: 1}
	retract.Reason = "outdated"
	retract.Removal = "active_recall"
	req.Action = retract
	if code, res, body := governedTestCall(t, g, token, req); code != 200 || res.Claims[0].Active || res.Claims[0].Version != 2 {
		t.Fatalf("retract %d %s", code, body)
	}
	req.Action = action
	if code, res, body := governedTestCall(t, g, token, req); code != 200 || res.Claims[0].Active {
		t.Fatalf("replay resurrected %d %s", code, body)
	}
	runtime, release, err = p.Acquire(ctx, brainID)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	legacy, err = brainstore.LoadMemoryProjection(runtime.Sources)
	if err != nil || !legacy.All()[0].Expired(time.Now()) {
		t.Fatalf("legacy active after retract %v", err)
	}
}
func TestGovernedMemoryBindingAndRevocation(t *testing.T) {
	ctx := context.Background()
	g, _, s, accountID, brainID, _ := newLifecycleExportFixture(t)
	g.Meter = &meter.Meter{Store: s}
	token, err := g.Issuer.Issue(ctx, brainID, accountID, []string{"memory:read", "memory:write"})
	if err != nil {
		t.Fatal(err)
	}
	action := governedTestAction("remember", "00000000-0000-4000-8000-000000000012")
	action.WriterOwner = accountID
	action.Text = "fixture"
	req := governedRequest{Schema: brainstore.GovernedMemorySchema, ProjectID: "braintwo", Action: action}
	if code, _, _ := governedTestCall(t, g, token, req); code != 403 {
		t.Fatalf("project mismatch %d", code)
	}
	req.ProjectID = brainID
	if code, _, body := governedTestCall(t, g, token, req); code != 200 {
		t.Fatalf("write %d %s", code, body)
	}
	req.Action.BrainID = "00000000-0000-4000-8000-000000000099"
	if code, _, _ := governedTestCall(t, g, token, req); code != 403 {
		t.Fatalf("brain relabel %d", code)
	}
	binding, err := g.Issuer.Verify(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if err = g.Issuer.RevokeKey(context.Background(), binding.CredentialID); err != nil {
		t.Fatal(err)
	}
	req.Action = action
	if code, _, _ := governedTestCall(t, g, token, req); code != 401 {
		t.Fatalf("revoked %d", code)
	}
}
