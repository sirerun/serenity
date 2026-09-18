package gateway

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type bucket struct {
	start time.Time
	count int
}
type limiter struct {
	mu      sync.Mutex
	buckets map[string]bucket
	swept   time.Time
}

func (l *limiter) allow(key string, limit int, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.buckets == nil {
		l.buckets = make(map[string]bucket)
	}
	if now.Sub(l.swept) >= time.Minute {
		for k, b := range l.buckets {
			if now.Sub(b.start) >= time.Minute {
				delete(l.buckets, k)
			}
		}
		l.swept = now
	}
	b, exists := l.buckets[key]
	if !exists && len(l.buckets) >= 10000 {
		return false
	}
	if now.Sub(b.start) >= time.Minute {
		b = bucket{start: now}
	}
	if b.count >= limit {
		return false
	}
	b.count++
	l.buckets[key] = b
	return true
}
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "unknown"
	}
	if peer := net.ParseIP(host); peer != nil && peer.IsLoopback() {
		if forwarded := net.ParseIP(r.Header.Get("X-Serenity-Client-IP")); forwarded != nil {
			return forwarded.String()
		}
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return "unknown"
}
