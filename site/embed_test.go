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
		{"/pricing/", 200, "Not available for purchase"},
		{"/docs/", 200, "<!doctype html>"},
		{"/docs/connections/", 200, "Connect your agent"},
		{"/docs/connections/claude-code/", 200, "Claude Code"},
		{"/docs/connections/codex/", 200, "Codex"},
		{"/docs/connections/other-mcp/", 200, "Other MCP clients"},
		{"/docs/connections/rakazo/", 200, "Rakazo"},
		{"/docs/connections/claude-web/", 200, "Claude on the web"},
		{"/docs/connections/chatgpt/", 200, "Not ready to connect yet"},
		{"/assets/hosted.css", 200, ".hosted-main"},
		{"/assets/", 404, "404"},
		{"/embed.go", 404, "404"},
		{"/CNAME", 404, "404"},
		{"/missing", 404, "404"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest("GET", tc.path, nil))
			policy := w.Header().Get("Content-Security-Policy")
			if !strings.Contains(policy, "frame-ancestors 'none'") || !strings.Contains(policy, "script-src 'self' 'sha256-") {
				t.Fatal("missing website script and framing policy")
			}
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.contains) {
				t.Fatalf("%s: status=%d, expected status=%d and content %q", tc.path, w.Code, tc.status, tc.contains)
			}
		})
	}
}
