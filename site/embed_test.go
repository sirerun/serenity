package website

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicFilesOnly(t *testing.T) {
	handler := Handler()
	for _, tc := range []struct {
		path     string
		status   int
		contains string
	}{
		{"/", 200, "keep your context"},
		{"/docs/", 200, "<!doctype html>"},
		{"/assets/hosted.css", 200, ".hosted-main"},
		{"/assets/", 404, "404"},
		{"/embed.go", 404, "404"},
		{"/CNAME", 404, "404"},
		{"/missing", 404, "404"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.contains) {
				t.Fatalf("%s: status=%d, expected status=%d and content %q", tc.path, w.Code, tc.status, tc.contains)
			}
		})
	}
}
