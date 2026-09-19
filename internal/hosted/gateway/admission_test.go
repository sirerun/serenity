package gateway

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestAdmissionAndUntrustedForwarding(t *testing.T) {
	var l limiter
	now := time.Now()
	for range 120 {
		if !l.allow("account:a", 120, now) {
			t.Fatal("early limit")
		}
	}
	if l.allow("account:a", 120, now) {
		t.Fatal("account exceeded allowance")
	}
	if !l.allow("account:b", 120, now) {
		t.Fatal("another account blocked")
	}
	if !l.allow("account:a", 120, now.Add(time.Minute)) {
		t.Fatal("window did not reset")
	}
	r := httptest.NewRequest("POST", "http://example.test/mcp", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("X-Serenity-Client-IP", "192.0.2.2")
	if clientIP(r) != "192.0.2.1" {
		t.Fatal("trusted remote forwarding")
	}
	r.RemoteAddr = "127.0.0.1:1234"
	if clientIP(r) != "192.0.2.2" {
		t.Fatal("local proxy forwarding lost")
	}
}

func TestHTTPRateLimitHasRetryAfter(t *testing.T) {
	var g Gateway
	for i := 0; i < 601; i++ {
		r := httptest.NewRequest("POST", "http://example.test/mcp", nil)
		r.RemoteAddr = "192.0.2.1:1234"
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		want := 401
		if i == 600 {
			want = 429
			if w.Header().Get("Retry-After") == "" {
				t.Fatal("missing retry guidance")
			}
		}
		if w.Code != want {
			t.Fatalf("request %d: status %d want %d", i, w.Code, want)
		}
	}
}
