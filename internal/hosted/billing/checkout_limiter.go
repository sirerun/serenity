package billing

import (
	"context"
	"sync"
	"time"
)

const (
	checkoutAttemptsPerAccount = 5
	checkoutRateWindow         = time.Minute
	checkoutTrackedAccountCap  = 10_000
)

// CheckoutRateLimitError reports a local checkout throttle without exposing
// provider or account details. RetryAfter is suitable for HTTP Retry-After.
type CheckoutRateLimitError struct {
	retryAfter time.Duration
}

func (e *CheckoutRateLimitError) Error() string {
	return "billing checkout rate limit exceeded"
}

// RetryAfter returns the duration clients should wait before retrying.
func (e *CheckoutRateLimitError) RetryAfter() time.Duration {
	return e.retryAfter
}

type checkoutRateEntry struct {
	start time.Time
	count int
}

// checkoutRateLimiter is a fixed-window per-account limiter. Its map is
// bounded, and old account keys are reclaimed during periodic/pressure sweeps.
type checkoutRateLimiter struct {
	mu        sync.Mutex
	accounts  map[string]checkoutRateEntry
	lastSweep time.Time
}

func (l *checkoutRateLimiter) charge(ctx context.Context, account string, now time.Time) (bool, time.Duration, error) {
	if err := ctx.Err(); err != nil {
		return false, 0, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return false, 0, err
	}
	if l.accounts == nil {
		l.accounts = make(map[string]checkoutRateEntry)
	}
	l.sweep(now, false)

	entry, exists := l.accounts[account]
	if exists && (now.Before(entry.start) || now.Sub(entry.start) >= checkoutRateWindow) {
		delete(l.accounts, account)
		exists = false
	}
	if !exists && len(l.accounts) >= checkoutTrackedAccountCap {
		l.sweep(now, true)
		if len(l.accounts) >= checkoutTrackedAccountCap {
			return false, checkoutRateWindow, nil
		}
	}
	if !exists {
		entry = checkoutRateEntry{start: now}
	}
	if entry.count >= checkoutAttemptsPerAccount {
		retryAfter := entry.start.Add(checkoutRateWindow).Sub(now)
		if retryAfter < time.Second {
			retryAfter = time.Second
		}
		return false, retryAfter, nil
	}
	entry.count++
	l.accounts[account] = entry
	return true, 0, nil
}

func (l *checkoutRateLimiter) sweep(now time.Time, force bool) {
	if !force && now.Sub(l.lastSweep) < checkoutRateWindow {
		return
	}
	for account, entry := range l.accounts {
		if now.Before(entry.start) || now.Sub(entry.start) >= checkoutRateWindow {
			delete(l.accounts, account)
		}
	}
	l.lastSweep = now
}

func retryAfterSeconds(delay time.Duration) int64 {
	if delay <= 0 {
		return 1
	}
	seconds := int64((delay + time.Second - 1) / time.Second)
	if seconds < 1 {
		return 1
	}
	return seconds
}

var _ error = (*CheckoutRateLimitError)(nil)
