package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	hoststore "github.com/sirerun/serenity/internal/hosted/store"
)

func qualificationService(t *testing.T, fixture bool) (*Service, string, string) {
	t.Helper()
	db, cfg, account, brain := episodeReplayServiceFixture(t)
	cfg.EpisodeArchiveEnabled = false
	cfg.EpisodeSyntheticBrains = nil
	if fixture {
		email := "synthetic-qualification-0123456789-a@serenity-episode-fixture.invalid"
		if _, err := db.DB().Exec("UPDATE accounts SET email=?,email_hash=? WHERE id=?", email, hoststore.Hash(email), account); err != nil {
			t.Fatal(err)
		}
	}
	svc, err := AssembleWithDependencies(context.Background(), cfg, true, db, nil, deletionEmbedding{}, testLifecycleDependencies(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := svc.Close(); err != nil {
			t.Error(err)
		}
	})
	return svc, account, brain
}
func qualificationRequest(brain string, until time.Time) []byte {
	raw, _ := json.Marshal(map[string]any{"brain_ids": []string{brain}, "expires_at": until})
	return raw
}
func TestEpisodeQualificationPrivateTransportAndAuthenticatedAPI(t *testing.T) {
	svc, account, brain := qualificationService(t, true)
	body := qualificationRequest(brain, time.Now().Add(time.Minute))
	public := httptest.NewRecorder()
	svc.Handler.ServeHTTP(public, httptest.NewRequest("POST", "/episodes/qualification", bytes.NewReader(body)))
	if public.Code == 204 {
		t.Fatal("public control exposed")
	}
	disabled := httptest.NewRecorder()
	svc.Handler.ServeHTTP(disabled, httptest.NewRequest("POST", "/episodes/v1", strings.NewReader(`{}`)))
	if disabled.Code != 404 {
		t.Fatal("public control changed enrollment")
	}
	private := httptest.NewRecorder()
	svc.AdminHandler().ServeHTTP(private, httptest.NewRequest("POST", "/episodes/qualification", bytes.NewReader(body)))
	if private.Code != 204 {
		t.Fatalf("private fixture admission %d", private.Code)
	}
	token, err := svc.Gateway.Issuer.Issue(context.Background(), brain, account, []string{"memory:read", "memory:write"})
	if err != nil {
		t.Fatal(err)
	}
	code, _, raw := episodeReplayCall(t, svc, token, episodeReplayRequest{Schema: "serenity.episodes/v1", ProjectID: brain, Operation: "capabilities", Qualification: "synthetic"})
	if code != 200 || !strings.Contains(raw, `"real_history_capture":false`) {
		t.Fatalf("synthetic API %d %s", code, raw)
	}
	closeGate := httptest.NewRecorder()
	svc.AdminHandler().ServeHTTP(closeGate, httptest.NewRequest("POST", "/episodes/qualification", strings.NewReader(`{"brain_ids":[]}`)))
	if closeGate.Code != 204 {
		t.Fatal("disable refused")
	}
	code, _, _ = episodeReplayCall(t, svc, token, episodeReplayRequest{Schema: "serenity.episodes/v1", ProjectID: brain, Operation: "capabilities", Qualification: "synthetic"})
	if code != 404 {
		t.Fatal("disabled gate still serves")
	}
	if svc.cfg.EpisodeArchiveEnabled || len(svc.cfg.EpisodeSyntheticBrains) > 0 {
		t.Fatal("permanent configuration changed")
	}
}
func TestEpisodeQualificationRejectsOrdinaryAccountAndMalformedControl(t *testing.T) {
	svc, _, brain := qualificationService(t, false)
	w := httptest.NewRecorder()
	svc.AdminHandler().ServeHTTP(w, httptest.NewRequest("POST", "/episodes/qualification", bytes.NewReader(qualificationRequest(brain, time.Now().Add(time.Minute)))))
	if w.Code != 403 {
		t.Fatalf("ordinary account accepted %d", w.Code)
	}
	for _, body := range []string{`{"brain_ids":null}`, `{"BRAIN_IDS":[]}`, `{"brain_ids":[],"extra":true}`, `{"brain_ids":[],"brain_ids":[]}`} {
		w = httptest.NewRecorder()
		svc.AdminHandler().ServeHTTP(w, httptest.NewRequest("POST", "/episodes/qualification", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatalf("malformed admitted %d", w.Code)
		}
	}
}
