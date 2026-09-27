package oauth

import (
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"
)

// rateLimits are the per-minute allowances the OAuth handler enforces
// (SEC-H02). Every /oauth path is judged by its source prefix first. Only the
// unauthenticated state-creating paths (register, authorize) share a global
// ceiling, and that ceiling is at least ten times the largest per-key
// allowance so it needs dozens of distinct prefixes to reach. Token and revoke
// requests, including refresh_token grants, are never subject to it.
type rateLimits struct {
	PerPrefix     int // every /oauth request, keyed by prefixKey
	Register      int // POST /oauth/register, keyed by prefixKey
	Token         int // /oauth/token and /oauth/revoke, keyed by prefixKey
	StateCreating int // global ceiling over register and authorize together
}

func defaultRateLimits() rateLimits {
	return rateLimits{PerPrefix: 120, Register: 10, Token: 30, StateCreating: 5000}
}

// maxTrackedKeys bounds limiter memory under a flood that spreads across more
// prefixes than one window can plausibly see. Past it every key starts a new
// window; the global ceiling on state-creating paths still holds.
const maxTrackedKeys = 1 << 17

type rateWindow struct {
	start time.Time
	count int
}

// limiter admits at most perKeyLimit requests per key per window and, when
// globalLimit is positive, at most globalLimit requests overall per window.
// A zero perKeyLimit disables the per-key window; a zero globalLimit disables
// the global ceiling.
type limiter struct {
	perKeyLimit int
	globalLimit int
	window      time.Duration
	now         func() time.Time

	mu        sync.Mutex
	keys      map[string]*rateWindow
	global    rateWindow
	lastSweep time.Time
}

func newLimiter(perKeyLimit, globalLimit int, window time.Duration) *limiter {
	return &limiter{
		perKeyLimit: perKeyLimit,
		globalLimit: globalLimit,
		window:      window,
		now:         time.Now,
		keys:        make(map[string]*rateWindow),
	}
}

// allow admits one request for key. The per-key window is evaluated before
// the global ceiling: a key over its allowance is refused without charging
// the global window, and a saturated global window refuses without charging
// the key, so one key's consumption is never attributed to another.
func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	var entry *rateWindow
	if l.perKeyLimit > 0 {
		entry = l.keys[key]
		if entry == nil || now.Sub(entry.start) >= l.window {
			entry = &rateWindow{start: now}
			l.keys[key] = entry
		}
		if entry.count >= l.perKeyLimit {
			return false
		}
	}
	if l.globalLimit > 0 {
		if now.Sub(l.global.start) >= l.window {
			l.global = rateWindow{start: now}
		}
		if l.global.count >= l.globalLimit {
			return false
		}
		l.global.count++
	}
	if entry != nil {
		entry.count++
	}
	return true
}

// sweep drops keys whose window has elapsed, once per window, and resets the
// table outright if a flood has spread over more keys than we track.
func (l *limiter) sweep(now time.Time) {
	if len(l.keys) > maxTrackedKeys {
		l.keys = make(map[string]*rateWindow)
		l.lastSweep = now
		return
	}
	if now.Sub(l.lastSweep) < l.window {
		return
	}
	l.lastSweep = now
	for key, entry := range l.keys {
		if now.Sub(entry.start) >= l.window {
			delete(l.keys, key)
		}
	}
}

func withRateLimit(l *limiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(prefixKey(clientIP(r))) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "too many requests"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// prefixKey collapses a source address to the prefix an attacker rotates
// within cheaply: /24 for IPv4 and /56 for IPv6, so a whole /64 (or 256 of
// them) counts as one key. An unparsable address is its own key.
func prefixKey(address string) string {
	ip := net.ParseIP(address)
	if ip == nil {
		return address
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.Mask(net.CIDRMask(24, 32)).String()
	}
	return ip.Mask(net.CIDRMask(56, 128)).String()
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
