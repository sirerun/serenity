//go:build linux || darwin

package episodecapture

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sirerun/serenity/internal/episodeclient"
	"github.com/sirerun/serenity/internal/store"
)

var testBinding = episodeclient.Binding{Brain: "syntheticbrain1234", Policy: "policy1", Producer: "producer1"}

func options(s *httptest.Server) Options {
	return Options{Endpoint: s.URL + "/episodes/v1", HTTPClient: s.Client(), Binding: testBinding, Synthetic: true, Token: func(context.Context) (string, error) { return "synthetic-token", nil }, Authorize: func(context.Context, episodeclient.ReplayKey) bool { return true }}
}
func replayKey() episodeclient.ReplayKey {
	return episodeclient.ReplayKey{Binding: testBinding, Workspace: "workspace1", Conversation: "conversation1", Event: "event1"}
}
func TestReconcileRejectsIncompleteOrWrongBinding(t *testing.T) {
	cases := []map[string]any{
		{"schema": store.EpisodeSchema, "project_id": testBinding.Brain, "status": "completed", "revision": strings.Repeat("a", 64), "fences": map[string]int{}},
		{"schema": store.EpisodeSchema, "project_id": "otherbrain123456", "status": "completed", "generation": 0, "revision": strings.Repeat("a", 64), "fences": map[string]int{}},
		{"schema": store.EpisodeSchema, "project_id": testBinding.Brain, "status": "completed", "generation": 0, "revision": strings.Repeat("a", 64)},
		{"schema": store.EpisodeSchema, "project_id": testBinding.Brain, "status": "completed", "generation": 1, "revision": strings.Repeat("a", 64), "fences": map[string]int{strings.Repeat("b", 64): 2}},
	}
	for _, body := range cases {
		s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(body) }))
		c, err := New(options(s))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = c.Reconcile(context.Background(), replayKey()); err == nil {
			t.Fatalf("accepted incomplete response %+v", body)
		}
		s.Close()
	}
}
func TestRedirectNeverForwardsCredential(t *testing.T) {
	var calls atomic.Int64
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer target.Close()
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	c, err := New(options(source))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Reconcile(context.Background(), replayKey()); err == nil {
		t.Fatal("followed redirect")
	}
	if calls.Load() != 0 {
		t.Fatal("redirect target contacted")
	}
}
func TestProductionAndInsecureEndpointsUnavailable(t *testing.T) {
	s := httptest.NewTLSServer(http.NotFoundHandler())
	defer s.Close()
	o := options(s)
	o.Synthetic = false
	if _, err := New(o); err == nil {
		t.Fatal("production capture accepted")
	}
	o = options(s)
	o.Endpoint = "http://example.invalid/episodes/v1"
	if _, err := New(o); err == nil {
		t.Fatal("cleartext accepted")
	}
	o = options(s)
	o.Endpoint = s.URL + "/episodes/v1?token=bad"
	if _, err := New(o); err == nil {
		t.Fatal("query accepted")
	}
}
func TestFenceScopeRetainsProducerIdentity(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"schema": store.EpisodeSchema, "project_id": testBinding.Brain, "status": "completed", "generation": 1, "revision": strings.Repeat("a", 64), "fences": map[string]int{store.EpisodeConversationKey("another-producer", "conversation1"): 1}})
	}))
	defer s.Close()
	c, err := New(options(s))
	if err != nil {
		t.Fatal(err)
	}
	v, err := c.Reconcile(context.Background(), replayKey())
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Conversations) != 1 || !v.Conversations[episodeclient.ConversationFenceKey("another-producer", "conversation1")] {
		t.Fatal("incomplete fence set")
	}
	if v.Conversations[episodeclient.ConversationFenceKey(testBinding.Producer, "conversation1")] {
		t.Fatal("cross-producer fence conflation")
	}
}
