package oauth

import (
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"
)

type rateWindow struct {
	start time.Time
	count int
}
type ipRateLimiter struct {
	perIPLimit  int
	globalLimit int
	window      time.Duration
	now         func() time.Time

	mu     sync.Mutex
	perIP  map[string]*rateWindow
	global rateWindow
}

func newIPRateLimiter(perIPLimit, globalLimit int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		perIPLimit:  perIPLimit,
		globalLimit: globalLimit,
		window:      window,
		now:         time.Now,
		perIP:       make(map[string]*rateWindow),
	}
}

func (l *ipRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()

	if now.Sub(l.global.start) >= l.window {
		l.global = rateWindow{start: now}
		l.perIP = make(map[string]*rateWindow)
	}
	if l.global.count >= l.globalLimit {
		return false
	}

	entry := l.perIP[ip]
	if entry == nil || now.Sub(entry.start) >= l.window {
		entry = &rateWindow{start: now}
		l.perIP[ip] = entry
	}
	if entry.count >= l.perIPLimit {
		return false
	}

	entry.count++
	l.global.count++
	return true
}

func withRateLimit(l *ipRateLimiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r)) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "too many requests"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
func clientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	// The deployed reverse proxy replaces this header; only loopback peers
	// may supply it. Never trust arbitrary X-Forwarded-For values.
	if peer := net.ParseIP(ip); peer != nil && peer.IsLoopback() {
		if forwarded := net.ParseIP(r.Header.Get("X-Serenity-Client-IP")); forwarded != nil {
			return forwarded.String()
		}
	}
	return ip
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
