package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/meter"
	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

func episodeHTTPFixture(t *testing.T, brain string) episodeRequest {
	t.Helper()
	raw, err := os.ReadFile("../../../testdata/conformance/episodes/append.json")
	if err != nil {
		t.Fatal(err)
	}
	var req episodeRequest
	if err = json.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	req.ProjectID = brain
	return req
}
func episodeHTTPCall(t *testing.T, g *Gateway, token string, req episodeRequest) (int, episodeResponse, string) {
	t.Helper()
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/episodes/v1", bytes.NewReader(raw))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.ServeEpisodes(w, r)
	var res episodeResponse
	if w.Code == 200 {
		if err = json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("cacheable archive response")
	}
	return w.Code, res, w.Body.String()
}
func episodeHTTPSetup(t *testing.T) (*Gateway, *pool.Pool, string, string) {
	t.Helper()
	g, p, s, account, brain, _ := newLifecycleExportFixture(t)
	g.Meter = &meter.Meter{Store: s}
	g.EpisodeArchiveEnabled = true
	g.EpisodeSyntheticBrains = []string{brain}
	token, err := g.Issuer.Issue(context.Background(), brain, account, []string{"memory:read", "memory:write"})
	if err != nil {
		t.Fatal(err)
	}
	return g, p, brain, token
}
func episodeReadRequest(brain, operation string) episodeRequest {
	return episodeRequest{Schema: store.EpisodeSchema, ProjectID: brain, Operation: operation, Qualification: "synthetic"}
}
func TestEpisodeHTTPExactRoundTripStableRetryAndRestart(t *testing.T) {
	g, p, brain, token := episodeHTTPSetup(t)
	req := episodeHTTPFixture(t, brain)
	code, res, body := episodeHTTPCall(t, g, token, req)
	if code != 200 || res.Receipt == nil {
		t.Fatalf("append %d %s", code, body)
	}
	receipt := res.Receipt
	if res.PhysicalPurge != "unsupported" || res.ManagedRestore != "unsupported" {
		t.Fatal("false erasure/restore claim")
	}
	code, retry, body := episodeHTTPCall(t, g, token, req)
	if code != 200 || !reflect.DeepEqual(receipt, retry.Receipt) {
		t.Fatalf("retry %d %s", code, body)
	}
	changed := episodeHTTPFixture(t, brain)
	changed.Mutation.Events[0].Text = "changed"
	if code, _, _ := episodeHTTPCall(t, g, token, changed); code != 409 {
		t.Fatalf("changed identity %d", code)
	}
	read := episodeReadRequest(brain, "read")
	read.EventKey = receipt.EventRefs[0].EventKey
	code, exact, body := episodeHTTPCall(t, g, token, read)
	if code != 200 || len(exact.Events) != 1 || !reflect.DeepEqual(exact.Events[0].Event, req.Mutation.Events[0]) {
		t.Fatalf("exact %d %s", code, body)
	}
	if exact.Events[0].Host != req.Mutation.Host || exact.Events[0].HostVersion != req.Mutation.HostVersion || !reflect.DeepEqual(exact.Events[0].CaptureGaps, req.Mutation.Gaps) {
		t.Fatal("host or capture gaps lost")
	}
	commandRead := episodeReadRequest(brain, "read")
	commandRead.CommandKey = receipt.CommandKey
	if code, got, body := episodeHTTPCall(t, g, token, commandRead); code != 200 || got.Capture == nil || !reflect.DeepEqual(got.Capture.Gaps, req.Mutation.Gaps) {
		t.Fatalf("capture provenance %d %s", code, body)
	}
	status := episodeReadRequest(brain, "status")
	status.CommandKey = receipt.CommandKey
	if code, got, body := episodeHTTPCall(t, g, token, status); code != 200 || !reflect.DeepEqual(got.Receipt, receipt) {
		t.Fatalf("status %d %s", code, body)
	}
	status.CommandKey = strings.Repeat("0", 64)
	if code, got, body := episodeHTTPCall(t, g, token, status); code != 200 || got.Status != "unknown" {
		t.Fatalf("absence %d %s", code, body)
	}
	// Reopen the runtime pool over the same canonical files/control ledger.
	root := p.BrainsRoot()
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := pool.New(pool.Config{MaxOpen: 1, MaxInFlight: 1, BrainsRoot: root, Embedder: journalTestEmbedder{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	g.Pool = reopened
	if code, got, body := episodeHTTPCall(t, g, token, req); code != 200 || !reflect.DeepEqual(got.Receipt, receipt) {
		t.Fatalf("restart retry %d %s", code, body)
	}
}
func TestEpisodeHTTPGatesBindingScopesAndRevocation(t *testing.T) {
	g, _, brain, token := episodeHTTPSetup(t)
	req := episodeHTTPFixture(t, brain)
	g.EpisodeArchiveEnabled = false
	if code, _, _ := episodeHTTPCall(t, g, token, req); code != 404 {
		t.Fatalf("default gate %d", code)
	}
	g.EpisodeArchiveEnabled = true
	g.EpisodeSyntheticBrains = nil
	if code, _, _ := episodeHTTPCall(t, g, token, req); code != 403 {
		t.Fatalf("synthetic scope %d", code)
	}
	g.EpisodeSyntheticBrains = []string{brain}
	req.ProjectID = "other-brain"
	if code, _, _ := episodeHTTPCall(t, g, token, req); code != 403 {
		t.Fatalf("brain mismatch %d", code)
	}
	req.ProjectID = brain
	req.Qualification = "production"
	if code, _, _ := episodeHTTPCall(t, g, token, req); code != 400 {
		t.Fatalf("real history %d", code)
	}
	req.Qualification = "synthetic"
	binding, err := g.Issuer.Verify(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	readOnly, err := g.Issuer.Issue(context.Background(), brain, binding.AccountID, []string{"memory:read"})
	if err != nil {
		t.Fatal(err)
	}
	if code, _, _ := episodeHTTPCall(t, g, readOnly, req); code != 403 {
		t.Fatalf("read credential mutation %d", code)
	}
	if _, err = g.Issuer.Store.DB().Exec("UPDATE client_credentials SET revoked_at=? WHERE id=?", time.Now().UTC().Format(time.RFC3339Nano), binding.CredentialID); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := episodeHTTPCall(t, g, token, req); code != 401 {
		t.Fatalf("revoked %d", code)
	}
}
func TestEpisodeHTTPContextCheckpointAndContentFreeFence(t *testing.T) {
	g, _, brain, token := episodeHTTPSetup(t)
	req := episodeHTTPFixture(t, brain)
	code, appendRes, body := episodeHTTPCall(t, g, token, req)
	if code != 200 {
		t.Fatalf("append %d %s", code, body)
	}
	query := episodeReadRequest(brain, "context")
	query.Query = "Exact visible"
	query.MaxBytes = 1
	if code, res, body := episodeHTTPCall(t, g, token, query); code != 200 || len(res.Events) != 0 || res.NextOffset == nil || len(res.Omissions) < 2 {
		t.Fatalf("budget %d %s", code, body)
	}
	query.MaxBytes = 64 << 10
	query.WorkspaceLabel = "another-label"
	if code, res, body := episodeHTTPCall(t, g, token, query); code != 200 || len(res.Events) != 0 {
		t.Fatalf("filter %d %s", code, body)
	}
	query.WorkspaceLabel = ""
	if code, res, body := episodeHTTPCall(t, g, token, query); code != 200 || len(res.Events) != 1 {
		t.Fatalf("context %d %s", code, body)
	}
	checkpoint := episodeHTTPFixture(t, brain)
	checkpoint.Operation = "checkpoint"
	checkpoint.Mutation.Operation = "checkpoint"
	checkpoint.Mutation.CommandID = "checkpoint-001"
	checkpoint.Mutation.ConversationID = "new-conversation"
	checkpoint.Mutation.Events = nil
	checkpoint.Mutation.Checkpoint = &store.EpisodeCheckpoint{Text: "Exact checkpoint: verify permission again.", Sources: appendRes.Receipt.EventRefs, CreatedAt: time.Date(2026, 10, 16, 10, 0, 0, 0, time.UTC), Model: "synthetic/no-model"}
	code, cp, body := episodeHTTPCall(t, g, token, checkpoint)
	if code != 200 {
		t.Fatalf("checkpoint %d %s", code, body)
	}
	read := episodeReadRequest(brain, "read")
	read.CommandKey = cp.Receipt.CommandKey
	if code, res, body := episodeHTTPCall(t, g, token, read); code != 200 || !reflect.DeepEqual(res.Checkpoint, checkpoint.Mutation.Checkpoint) {
		t.Fatalf("checkpoint read %d %s", code, body)
	}
	exclude := episodeHTTPFixture(t, brain)
	exclude.Operation = "exclude"
	exclude.Mutation = &store.EpisodeInput{Operation: "exclude", ProducerID: req.Mutation.ProducerID, ConversationID: req.Mutation.ConversationID, CommandID: "exclude-001"}
	if code, res, body := episodeHTTPCall(t, g, token, exclude); code != 200 || res.Status != "excluded" || res.Generation != 1 || res.PhysicalPurge != "unsupported" {
		t.Fatalf("exclude %d %s", code, body)
	}
	if code, _, _ := episodeHTTPCall(t, g, token, read); code != 404 {
		t.Fatalf("dependent checkpoint disclosed %d", code)
	}
	if code, _, _ := episodeHTTPCall(t, g, token, req); code != 409 {
		t.Fatalf("stale generation admitted %d", code)
	}
	current := uint64(1)
	req.Generation = &current
	if code, _, _ := episodeHTTPCall(t, g, token, req); code != 410 {
		t.Fatalf("fenced replay %d", code)
	}
	sync := episodeReadRequest(brain, "fences")
	code, fences, body := episodeHTTPCall(t, g, token, sync)
	if code != 200 || fences.Generation != 1 || len(fences.Fences) != 1 || strings.Contains(body, req.Mutation.ConversationID) || strings.Contains(body, req.Mutation.ProducerID) {
		t.Fatalf("fence sync %d %s", code, body)
	}
	if code, res, body := episodeHTTPCall(t, g, token, query); code != 200 || len(res.Events) != 0 {
		t.Fatalf("excluded context %d %s", code, body)
	}
}
func TestEpisodeHTTPPublicationFailureStatusUnknownAndRetry(t *testing.T) {
	g, p, brain, token := episodeHTTPSetup(t)
	req := episodeHTTPFixture(t, brain)
	root := filepath.Join(p.BrainsRoot(), brain)
	hook := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := episodeHTTPCall(t, g, token, req); code != 503 {
		t.Fatalf("failed flush acknowledged %d", code)
	}
	lookup := episodeReadRequest(brain, "status")
	lookup.CommandKey = store.EpisodeCommandKey(req.Mutation.ProducerID, req.Mutation.CommandID)
	if code, res, body := episodeHTTPCall(t, g, token, lookup); code != 200 || res.Status != "unknown" || res.Receipt != nil {
		t.Fatalf("unpublished status %d %s", code, body)
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	if code, res, body := episodeHTTPCall(t, g, token, req); code != 200 || res.Receipt == nil {
		t.Fatalf("publication recovery %d %s", code, body)
	}
}
func TestEpisodeHTTPStrictInputAndSizeBounds(t *testing.T) {
	g, _, brain, token := episodeHTTPSetup(t)
	req := episodeHTTPFixture(t, brain)
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, body, origin, contentType string
		want                            int
	}{
		{"unknown-field", strings.TrimSuffix(string(raw), "}") + `,"brain_id":"forged"}`, "", "application/json", 400},
		{"trailing-json", string(raw) + `{}`, "", "application/json", 400},
		{"origin", string(raw), "https://evil.example", "application/json", 403},
		{"content-type", string(raw), "", "text/plain", 415},
		{"oversized", strings.Repeat("x", store.EpisodeMaxRequestBytes+1), "", "application/json", 413},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/episodes/v1", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			g.ServeEpisodes(w, r)
			if w.Code != tc.want {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
	req.Mutation.Events = make([]store.EpisodeEvent, 65)
	if code, _, _ := episodeHTTPCall(t, g, token, req); code != 400 {
		t.Fatalf("event count %d", code)
	}
}
func TestEpisodeHTTPQuotaAndNoModelCapability(t *testing.T) {
	g, _, brain, token := episodeHTTPSetup(t)
	req := episodeHTTPFixture(t, brain)
	binding, err := g.Issuer.Verify(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	entitlement, _, err := g.Meter.Subject(context.Background(), binding.AccountID, binding.PartnerID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = g.Issuer.Store.DB().Exec("INSERT INTO usage_windows(account_id,window_key,metric,committed) VALUES(?,?,?,?)", binding.AccountID, entitlement.Window, "writes", entitlement.Plan.Writes); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := episodeHTTPCall(t, g, token, req); code != 429 {
		t.Fatalf("write quota %d", code)
	}
	capReq := episodeReadRequest(brain, "capabilities")
	if code, res, body := episodeHTTPCall(t, g, token, capReq); code != 200 || res.Capabilities["model_calls"] != false || res.Capabilities["real_history_capture"] != false {
		t.Fatalf("capabilities %d %s", code, body)
	}
	exclude := episodeReadRequest(brain, "exclude")
	gen := uint64(0)
	exclude.Generation = &gen
	exclude.Mutation = &store.EpisodeInput{Operation: "exclude", ProducerID: "p", ConversationID: "not-yet-uploaded", CommandID: "exclude-at-quota"}
	if code, res, body := episodeHTTPCall(t, g, token, exclude); code != 200 || res.Generation != 1 {
		t.Fatalf("quota must leave fence admission %d %s", code, body)
	}
}
func TestEpisodeHTTPUnsettledMeterStatusIsUnknown(t *testing.T) {
	g, p, brain, token := episodeHTTPSetup(t)
	req := episodeHTTPFixture(t, brain)
	runtime, release, err := p.AcquireCanonical(context.Background(), brain)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := writer.Episode(context.Background(), runtime.Queue, runtime.Sources, runtime.Root, brain, 0, *req.Mutation, nil)
	release()
	if err != nil {
		t.Fatal(err)
	}
	lookup := episodeReadRequest(brain, "status")
	lookup.CommandKey = receipt.CommandKey
	if code, res, body := episodeHTTPCall(t, g, token, lookup); code != 200 || res.Status != "unknown" {
		t.Fatalf("unmetered status %d %s", code, body)
	}
}

func TestEpisodeHTTPLargeAdmittedEventRemainsExactlyReadable(t *testing.T) {
	g, _, brain, token := episodeHTTPSetup(t)
	req := episodeHTTPFixture(t, brain)
	req.Mutation.Events[0].Text = strings.Repeat("\x00", store.EpisodeMaxTextBytes)
	data := make([]byte, store.EpisodeMaxAttachmentBytes)
	sum := sha256.Sum256(data)
	req.Mutation.Events[0].Attachments[0].Data = data
	req.Mutation.Events[0].Attachments[0].SHA256 = hex.EncodeToString(sum[:])
	code, res, body := episodeHTTPCall(t, g, token, req)
	if code != 200 {
		t.Fatalf("large admission %d %s", code, body)
	}
	read := episodeReadRequest(brain, "read")
	read.EventKey = res.Receipt.EventRefs[0].EventKey
	code, exact, body := episodeHTTPCall(t, g, token, read)
	if code != 200 || len(exact.Events) != 1 {
		t.Fatalf("large read %d %s", code, body)
	}
	if !reflect.DeepEqual(exact.Events[0].Event, req.Mutation.Events[0]) {
		t.Fatal("large admitted original not exact")
	}
}

func TestEpisodeHTTPStorageAdmissionUsesCanonicalEncoding(t *testing.T) {
	g, p, brain, token := episodeHTTPSetup(t)
	ctx := context.Background()
	binding, err := g.Issuer.Verify(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	limits, account, err := g.Meter.Subject(ctx, binding.AccountID, binding.PartnerID)
	if err != nil {
		t.Fatal(err)
	}
	runtime, release, err := p.AcquireCanonical(ctx, brain)
	if err != nil {
		t.Fatal(err)
	}
	// Release the fixture lease before the HTTP handler acquires its own.
	root, sources := runtime.Root, runtime.Sources
	release()
	inventory, err := g.inventory(ctx, binding.AccountID, filepath.Dir(root))
	if err != nil {
		t.Fatal(err)
	}
	filler, err := os.Create(filepath.Join(root, "storage-budget-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	if err = filler.Truncate(max(limits.Plan.StorageBytes, account.Plan.StorageBytes) - inventory.StorageBytes - 300*1024); err != nil {
		t.Fatal(err)
	}
	if err = filler.Close(); err != nil {
		t.Fatal(err)
	}
	req := episodeHTTPFixture(t, brain)
	req.Mutation.Events = req.Mutation.Events[:1]
	req.Mutation.Events[0].Text = strings.Repeat("<", 64<<10)
	req.Mutation.Events[0].Attachments = nil
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err = encoder.Encode(req); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "/episodes/v1", &encoded)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.ServeEpisodes(w, r)
	if w.Code != 429 || !strings.Contains(w.Body.String(), "limit_exceeded") {
		t.Fatalf("canonical quota bypass: %d %s", w.Code, w.Body.String())
	}
	projection, err := store.LoadEpisodes(ctx, sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(projection.Records) != 0 {
		t.Fatal("refused request mutated canonical archive")
	}
}
