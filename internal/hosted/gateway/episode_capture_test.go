//go:build linux || darwin

package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/episodecapture"
	"github.com/sirerun/serenity/internal/episodeclient"
	"github.com/sirerun/serenity/internal/episodehost"
	"github.com/sirerun/serenity/internal/store"
)

func captureQueue(t *testing.T, brain string) *episodeclient.Queue {
	t.Helper()
	binding := episodeclient.Binding{Brain: brain, Policy: "policy1", Producer: "producer1"}
	q, err := episodeclient.OpenQueue(episodeclient.Options{Directory: filepath.Join(t.TempDir(), "queue"), Key: make([]byte, 32), Policy: episodeclient.Policy{Binding: binding, Mappings: []episodeclient.Mapping{{Workspace: "workspace1", Binding: binding}}}, MaxItems: 8, MaxBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = q.Close() })
	return q
}
func captureClient(t *testing.T, server *httptest.Server, brain, token string, allow func(context.Context, episodeclient.ReplayKey) bool) *episodecapture.Client {
	t.Helper()
	c, err := episodecapture.New(episodecapture.Options{Endpoint: server.URL + "/episodes/v1", HTTPClient: server.Client(), Binding: episodeclient.Binding{Brain: brain, Policy: "policy1", Producer: "producer1"}, Synthetic: true, Token: func(context.Context) (string, error) { return token, nil }, Authorize: allow})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func captureInput() episodeclient.Input {
	stamp := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	return episodeclient.Input{Workspace: "workspace1", Conversation: "conversation1", Event: "event1", Sequence: 1, Role: "user", Kind: "message", SourceTime: stamp, ObservedTime: stamp, HostMode: "normalized-visible-v1", Privacy: "public", Class: "visible", Text: "remember api_key=synthetic-secret-value then resume"}
}
func TestEpisodeCaptureTLSQueueToCanonicalAndReplayFence(t *testing.T) {
	g, _, brain, token := episodeHTTPSetup(t)
	server := httptest.NewTLSServer(http.HandlerFunc(g.ServeEpisodes))
	defer server.Close()
	q := captureQueue(t, brain)
	c := captureClient(t, server, brain, token, func(context.Context, episodeclient.ReplayKey) bool { return true })
	input := captureInput()
	if _, err := q.Capture(input); err != nil {
		t.Fatal(err)
	}
	result, err := q.Drain(context.Background(), c, c, 8)
	if err != nil || result.Published != 1 || result.Remaining != 0 {
		t.Fatalf("drain %+v %v", result, err)
	}
	read := episodeReadRequest(brain, "read")
	read.EventKey = store.EpisodeEventKey("producer1", "conversation1", "event1")
	code, body, raw := episodeHTTPCall(t, g, token, read)
	if code != 200 || len(body.Events) != 1 || strings.Contains(raw, "synthetic-secret-value") || !strings.Contains(body.Events[0].Event.Text, "[REDACTED]") {
		t.Fatalf("read/redaction %d %s", code, raw)
	}
	// Re-import after local acknowledgment loss reproduces the same command and
	// receipt; it must not allocate another archive occurrence.
	if _, err = q.Capture(input); err != nil {
		t.Fatal(err)
	}
	if _, err = q.Drain(context.Background(), c, c, 8); err != nil {
		t.Fatal(err)
	}
	search := episodeReadRequest(brain, "search")
	code, found, raw := episodeHTTPCall(t, g, token, search)
	if code != 200 || len(found.Events) != 1 {
		t.Fatalf("duplicate %d %s", code, raw)
	}
	generation := uint64(0)
	exclude := episodeReadRequest(brain, "exclude")
	exclude.Generation = &generation
	exclude.Mutation = &store.EpisodeInput{Operation: "exclude", ProducerID: "producer1", ConversationID: "conversation1", CommandID: "delete1"}
	if code, _, raw := episodeHTTPCall(t, g, token, exclude); code != 200 {
		t.Fatalf("exclude %d %s", code, raw)
	}
	if _, err = q.Capture(input); err != nil {
		t.Fatal(err)
	}
	result, err = q.Drain(context.Background(), c, c, 8)
	if !errors.Is(err, episodeclient.ErrBlocked) || result.Published != 0 {
		t.Fatalf("fenced %+v %v", result, err)
	}
	if stats, _ := q.Stats(); stats.Items != 1 {
		t.Fatal("unacknowledged fenced evidence silently lost")
	}
}
func TestEpisodeCaptureAuthorityChangeAndBadReceiptRetainQueue(t *testing.T) {
	g, _, brain, token := episodeHTTPSetup(t)
	server := httptest.NewTLSServer(http.HandlerFunc(g.ServeEpisodes))
	defer server.Close()
	q := captureQueue(t, brain)
	if _, err := q.Capture(captureInput()); err != nil {
		t.Fatal(err)
	}
	c := captureClient(t, server, brain, token, func(context.Context, episodeclient.ReplayKey) bool { return false })
	if _, err := q.Drain(context.Background(), c, c, 1); !errors.Is(err, episodeclient.ErrFence) {
		t.Fatal(err)
	}
	// A syntactically successful response without a content-bound receipt must
	// never dequeue. Fences are obtained from the real authenticated gateway.
	bad := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, "invalid", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"schema": store.EpisodeSchema, "project_id": brain, "status": "completed", "revision": strings.Repeat("a", 64), "generation": 0})
	}))
	defer bad.Close()
	good := captureClient(t, server, brain, token, func(context.Context, episodeclient.ReplayKey) bool { return true })
	wrong := captureClient(t, bad, brain, token, func(context.Context, episodeclient.ReplayKey) bool { return true })
	if _, err := q.Drain(context.Background(), good, wrong, 1); !errors.Is(err, episodeclient.ErrBlocked) {
		t.Fatal(err)
	}
	if stats, _ := q.Stats(); stats.Items != 1 {
		t.Fatal("unverified receipt removed pending event")
	}
}
func TestEpisodeCaptureCodexSyntheticStaging(t *testing.T) {
	q := captureQueue(t, "syntheticbrain1234")
	raw := `{"timestamp":"2026-10-16T00:00:00Z","type":"session_meta","payload":{"id":"session1","cli_version":"0.163.0"}}` + "\n" + `{"timestamp":"2026-10-16T00:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"token secret=synthetic-hidden-value"}]}}` + "\n"
	e := episodehost.Enrollment{SessionID: "session1", VisibleSession: true}
	if _, err := episodecapture.StageCodex(q, strings.NewReader(raw), e, episodehost.Cursor{}, "workspace1", false); err == nil {
		t.Fatal("real-history mode accepted")
	}
	page, err := episodecapture.StageCodex(q, strings.NewReader(raw), e, episodehost.Cursor{}, "workspace1", true)
	if err != nil || page.Cursor.Lines != 2 {
		t.Fatalf("stage %+v %v", page, err)
	}
	if _, err := episodecapture.StageCodex(q, strings.NewReader(raw), e, episodehost.Cursor{}, "workspace1", true); err != nil {
		t.Fatal(err)
	}
	if stats, _ := q.Stats(); stats.Items != 1 {
		t.Fatal("staging replay duplicated")
	}
}

func TestEpisodeCaptureRejectsAmbiguousWireJSON(t *testing.T) {
	g, _, brain, token := episodeHTTPSetup(t)
	for _, extra := range []string{`,"operation":"append"`, `,"OPERATION":"append"`, `,"query":"\ud800"`} {
		raw := `{"schema":"serenity.episodes/v1","project_id":"` + brain + `","operation":"fences","qualification":"synthetic"` + extra + `}`
		req := httptest.NewRequest("POST", "/episodes/v1", strings.NewReader(raw))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		g.ServeEpisodes(w, req)
		if w.Code != 400 {
			t.Fatalf("ambiguous JSON status %d", w.Code)
		}
	}
}
