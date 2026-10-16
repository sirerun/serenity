package gateway

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEpisodeQualificationExpiryDisableAndSafetyBuild(t *testing.T) {
	g := &Gateway{}
	until := time.Now().Add(time.Minute)
	if err := g.SetEpisodeQualification([]string{"fixture-brain"}, until); !episodeArchiveBuildEnabled {
		if err == nil {
			t.Fatal("safety build accepted enablement")
		}
		return
	} else if err != nil {
		t.Fatal(err)
	}
	if !g.episodeBrainAllowed("fixture-brain") || g.episodeBrainAllowed("other-brain") {
		t.Fatal("incorrect brain scope")
	}
	g.mu.Lock()
	g.episodeQualification.until = time.Now().Add(-time.Second)
	g.mu.Unlock()
	w := httptest.NewRecorder()
	g.ServeEpisodes(w, httptest.NewRequest("POST", "/episodes/v1", strings.NewReader(`{}`)))
	if w.Code != 404 || !strings.Contains(w.Body.String(), "archive_disabled") {
		t.Fatal("expired enrollment survived")
	}
	if err := g.SetEpisodeQualification([]string{"fixture-brain"}, until); err != nil {
		t.Fatal(err)
	}
	if err := g.SetEpisodeQualification(nil, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if g.episodeBrainAllowed("fixture-brain") {
		t.Fatal("disable did not revoke")
	}
	for _, brains := range [][]string{{"a", "b", "c"}, {"a", "a"}, {"../unsafe"}} {
		if g.SetEpisodeQualification(brains, until) == nil {
			t.Fatal("invalid scope admitted")
		}
	}
	if g.SetEpisodeQualification([]string{"a"}, time.Now().Add(16*time.Minute)) == nil {
		t.Fatal("unbounded duration")
	}
	g.EpisodeArchiveEnabled = true
	g.EpisodeSyntheticBrains = []string{"existing"}
	if g.SetEpisodeQualification([]string{"a"}, until) == nil {
		t.Fatal("permanent configuration overridden")
	}
	if !g.episodeBrainAllowed("existing") {
		t.Fatal("permanent config changed")
	}
}
