package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEpisodeArchiveBuildGate(t *testing.T) {
	g := &Gateway{EpisodeArchiveEnabled: true, EpisodeSyntheticBrains: []string{"synthetic"}}
	w := httptest.NewRecorder()
	g.ServeEpisodes(w, httptest.NewRequest(http.MethodPost, "/episodes/v1", strings.NewReader(`{}`)))
	if episodeArchiveBuildEnabled {
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("normal build did not enforce authentication: %d", w.Code)
		}
	} else if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), `"error":"archive_disabled"`) {
		t.Fatalf("safety build opened archive: %d %s", w.Code, w.Body.String())
	}
}
