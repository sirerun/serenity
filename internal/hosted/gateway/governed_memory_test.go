package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/sirerun/serenity/internal/gitrun"
	"github.com/sirerun/serenity/internal/hosted/pool"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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

type governedCountingEmbedder struct{ calls atomic.Int64 }

func (e *governedCountingEmbedder) ModelVersion() string { return journalTestEmbedder{}.ModelVersion() }
func (e *governedCountingEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	e.calls.Add(1)
	return journalTestEmbedder{}.Embed(ctx, text)
}

func TestGovernedColdStartAndReadOnlyRestartStatus(t *testing.T) {
	ctx := context.Background()
	g, oldPool, s, accountID, brainID, brainRoot := newLifecycleExportFixture(t)
	if err := oldPool.Close(); err != nil {
		t.Fatal(err)
	}
	spy := &governedCountingEmbedder{}
	newPool := func() *pool.Pool {
		p, e := pool.New(pool.Config{MaxOpen: 1, MaxInFlight: 2, BrainsRoot: oldPool.BrainsRoot(), Embedder: spy})
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	p := newPool()
	g.Pool = p
	g.Meter = &meter.Meter{Store: s}
	token, err := g.Issuer.Issue(ctx, brainID, accountID, []string{"memory:read", "memory:write"})
	if err != nil {
		t.Fatal(err)
	}
	action := governedTestAction("remember", "00000000-0000-4000-8000-000000000021")
	action.WriterOwner = accountID
	action.Text = "Shared durable memory"
	req := governedRequest{Schema: brainstore.GovernedMemorySchema, ProjectID: brainID, Action: action}
	code, response, body := governedTestCall(t, g, token, req)
	if code != 200 {
		t.Fatalf("write %d %s", code, body)
	}
	if spy.calls.Load() != 0 {
		t.Fatal("governed cold write called an embedder")
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	p = newPool()
	g.Pool = p
	defer p.Close()
	before, err := brainstore.NewSourceStore(brainRoot).All()
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/memory/v1/commands/"+action.CommandID+"?project_id="+brainID+"&brain_id="+governedBrain, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	g.ServeGovernedMemory(w, r)
	if w.Code != 200 {
		t.Fatalf("restart status %d %s", w.Code, w.Body.String())
	}
	var recovered brainstore.GovernedResponse
	if err := json.Unmarshal(w.Body.Bytes(), &recovered); err != nil {
		t.Fatal(err)
	}
	if recovered.Claims[0].ID != response.Claims[0].ID || recovered.Status != "completed" {
		t.Fatal("lost original durable identity")
	}
	if _, release, err := p.AcquireExisting(ctx, brainID); err == nil {
		release()
		t.Fatal("status opened a runtime")
	}
	after, err := brainstore.NewSourceStore(brainRoot).All()
	if err != nil || len(after) != len(before) {
		t.Fatalf("status mutated canonical sources %v", err)
	}
	action = governedTestAction("recall", "00000000-0000-4000-8000-000000000022")
	action.Query = "durable"
	action.MaxClaims = 10
	action.MinimumFreshness = time.Now().Add(-time.Hour).UTC()
	action.Classification = "internal"
	req.Action = action
	if code, res, body := governedTestCall(t, g, token, req); code != 200 || len(res.Claims) != 1 {
		t.Fatalf("cold recall %d %s", code, body)
	}
	if spy.calls.Load() != 0 {
		t.Fatal("cold canonical recall recovered the provider-backed index")
	}
}

func TestGovernedMCPInteroperabilityAndAccountGate(t *testing.T) {
	ctx := context.Background()
	g, _, s, accountID, brainID, _ := newLifecycleExportFixture(t)
	g.Meter = &meter.Meter{Store: s}
	token, err := g.Issuer.Issue(ctx, brainID, accountID, []string{"memory:read", "memory:write"})
	if err != nil {
		t.Fatal(err)
	}
	action := governedTestAction("remember", "00000000-0000-4000-8000-000000000031")
	action.WriterOwner = accountID
	action.Text = "Remember across clients"
	req := governedRequest{Schema: brainstore.GovernedMemorySchema, ProjectID: brainID, Action: action}
	code, response, body := governedTestCall(t, g, token, req)
	if code != 200 {
		t.Fatalf("write %d %s", code, body)
	}
	result, err := g.CallForAccount(ctx, accountID, brainID, "recall", json.RawMessage(`{}`))
	if err != nil || result.IsError {
		t.Fatalf("MCP recall %v %+v", err, result)
	}
	var legacy struct {
		Facts []struct {
			FactID     string `json:"fact_id"`
			Fact       string `json:"fact"`
			Provenance string `json:"provenance"`
		} `json:"facts"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].Text), &legacy); err != nil || len(legacy.Facts) != 1 || legacy.Facts[0].Fact != action.Text {
		t.Fatalf("MCP continuity %v %+v", err, legacy)
	}
	args, _ := json.Marshal(map[string]string{"id": legacy.Facts[0].FactID})
	result, err = g.CallForAccount(ctx, accountID, brainID, "forget", args)
	if err != nil || !result.IsError {
		t.Fatalf("legacy forget must explain governed retraction %v %+v", err, result)
	}
	args, _ = json.Marshal(map[string]any{"adapter_command_id": "00000000-0000-4000-8000-000000000032", "claim_id": response.Claims[0].ID, "version": 1, "reason": "no longer relevant"})
	result, err = g.CallForAccount(ctx, accountID, brainID, "retract_governed_memory", args)
	if err != nil || result.IsError {
		t.Fatalf("MCP retract %v %+v", err, result)
	}
	result, err = g.CallForAccount(ctx, accountID, brainID, "recall", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	legacy.Facts = nil
	if err = json.Unmarshal([]byte(result.Content[0].Text), &legacy); err != nil || len(legacy.Facts) != 0 {
		t.Fatalf("MCP recalled retracted memory %v %+v", err, legacy)
	}
	if _, err = s.DB().ExecContext(ctx, "UPDATE accounts SET status='restore_pending' WHERE id=?", accountID); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := governedTestCall(t, g, token, req); code != 401 && code != 403 {
		t.Fatalf("nonactive account replay allowed: %d", code)
	}
}

func TestGovernedReadReceiptsCannotBypassStorageQuota(t *testing.T) {
	ctx := context.Background()
	g, _, s, accountID, brainID, root := newLifecycleExportFixture(t)
	g.Meter = &meter.Meter{Store: s}
	token, err := g.Issuer.Issue(ctx, brainID, accountID, []string{"memory:read"})
	if err != nil {
		t.Fatal(err)
	}
	limits, _, err := g.Meter.Subject(ctx, accountID, "")
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(root, "storage-budget-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	if err = file.Truncate(limits.Plan.StorageBytes); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	action := governedTestAction("recall", "00000000-0000-4000-8000-000000000041")
	action.Query = "fixture"
	action.MaxClaims = 10
	action.MinimumFreshness = time.Now().Add(-time.Hour).UTC()
	action.Classification = "internal"
	req := governedRequest{Schema: brainstore.GovernedMemorySchema, ProjectID: brainID, Action: action}
	if code, _, body := governedTestCall(t, g, token, req); code != 429 {
		t.Fatalf("storage quota bypassed: %d %s", code, body)
	}
	projection, err := brainstore.LoadGovernedMemory(brainstore.NewSourceStore(root))
	if err != nil || len(projection.Commands) != 0 {
		t.Fatalf("refused read persisted a receipt: %v", err)
	}
}

func TestGovernedStatusSerializesWithMCPRetraction(t *testing.T) {
	ctx := context.Background()
	g, _, s, accountID, brainID, _ := newLifecycleExportFixture(t)
	g.Meter = &meter.Meter{Store: s}
	token, err := g.Issuer.Issue(ctx, brainID, accountID, []string{"memory:read", "memory:write"})
	if err != nil {
		t.Fatal(err)
	}
	a := governedTestAction("remember", "00000000-0000-4000-8000-000000000051")
	a.WriterOwner = accountID
	a.Text = "concurrent fixture"
	req := governedRequest{Schema: brainstore.GovernedMemorySchema, ProjectID: brainID, Action: a}
	code, res, body := governedTestCall(t, g, token, req)
	if code != 200 {
		t.Fatalf("write %d %s", code, body)
	}
	start := make(chan struct{})
	errs := make(chan string, 12)
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r := httptest.NewRequest("GET", "/memory/v1/commands/"+a.CommandID+"?project_id="+brainID+"&brain_id="+governedBrain, nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			g.ServeGovernedMemory(w, r)
			var response brainstore.GovernedResponse
			if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &response) != nil || len(response.Claims) != 1 {
				errs <- "status failed"
				return
			}
			c := response.Claims[0]
			if !(c.Version == 1 && c.Active || c.Version == 2 && !c.Active) {
				errs <- "inconsistent snapshot"
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		args, _ := json.Marshal(map[string]any{"adapter_command_id": "00000000-0000-4000-8000-000000000052", "claim_id": res.Claims[0].ID, "version": 1, "reason": "concurrent removal"})
		result, err := g.CallForAccount(ctx, accountID, brainID, "retract_governed_memory", args)
		if err != nil || result.IsError {
			errs <- "MCP retract failed"
		}
	}()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestGovernedUnpublishedReceiptRemainsUnknownUntilExplicitReplay(t *testing.T) {
	ctx := context.Background()
	g, _, s, accountID, brainID, root := newLifecycleExportFixture(t)
	g.Meter = &meter.Meter{Store: s}
	token, err := g.Issuer.Issue(ctx, brainID, accountID, []string{"memory:read", "memory:write"})
	if err != nil {
		t.Fatal(err)
	}
	a := governedTestAction("remember", "00000000-0000-4000-8000-000000000061")
	a.WriterOwner = accountID
	a.Text = "publication failure fixture"
	req := governedRequest{Schema: brainstore.GovernedMemorySchema, ProjectID: brainID, Action: a}
	code, res, body := governedTestCall(t, g, token, req)
	if code != 200 {
		t.Fatalf("write %d %s", code, body)
	}
	// In this owned temporary brain, move HEAD back while preserving its files:
	// this models a durable source whose Git publication did not commit.
	previous, err := gitrun.Brain(root).Output(ctx, "rev-parse", "HEAD^")
	if err != nil {
		t.Fatal(err)
	}
	if err = gitrun.Brain(root).Run(ctx, "update-ref", "HEAD", strings.TrimSpace(string(previous))); err != nil {
		t.Fatal(err)
	}
	status := func() brainstore.GovernedResponse {
		t.Helper()
		r := httptest.NewRequest("GET", "/memory/v1/commands/"+a.CommandID+"?project_id="+brainID+"&brain_id="+governedBrain, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		g.ServeGovernedMemory(w, r)
		if w.Code != 200 {
			t.Fatalf("status %d %s", w.Code, w.Body.String())
		}
		var result brainstore.GovernedResponse
		if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if got := status(); got.Status != "unknown" || got.LookupAuthoritative || len(got.Claims) != 0 {
		t.Fatalf("unpublished receipt acknowledged: %+v", got)
	}
	code, recovered, body := governedTestCall(t, g, token, req)
	if code != 200 || recovered.Claims[0].ID != res.Claims[0].ID {
		t.Fatalf("explicit replay %d %s", code, body)
	}
	if got := status(); got.Status != "completed" || !got.LookupAuthoritative {
		t.Fatalf("publication not recovered: %+v", got)
	}
}
