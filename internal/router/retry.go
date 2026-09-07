package router

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"syscall"
	"time"
)

// defaultProviderHTTPTimeout bounds every real provider HTTP call
// (AnthropicProvider, OpenAICompatibleProvider, OpenAIEmbeddingsProvider)
// when the caller leaves HTTPClient nil. http.DefaultClient's Timeout
// zero value means "no timeout at all", so a stalled connection -- a peer
// that accepts the TCP connection but never sends a byte -- hangs Send,
// and therefore Router.Complete, forever. This was a real gap: T1.23's
// live eval hit dropped connections under DGX host CPU contention with no
// bound anywhere in the call chain. 120s is long enough for a
// slow-but-live reasoning completion (the judgment tier's real workload)
// while still bounding the worst case; a caller needing a different bound
// sets HTTPClient explicitly -- this default only fills the gap left by a
// nil client.
const defaultProviderHTTPTimeout = 120 * time.Second

// httpClientOrDefault returns c unchanged when the caller supplied one,
// else a client carrying defaultProviderHTTPTimeout. Never returns
// http.DefaultClient, whose zero-value Timeout never bounds a call.
func httpClientOrDefault(c *http.Client) *http.Client {
	if c != nil {
		return c
	}
	return &http.Client{Timeout: defaultProviderHTTPTimeout}
}

// defaultRetryAttempts is the total number of provider.Send attempts
// Router.Complete makes for one call, including the first -- 3 means
// "try, then up to 2 retries," never an unbounded loop.
const defaultRetryAttempts = 3

// defaultRetryBaseDelay/defaultRetryMaxDelay bound the exponential
// backoff between retry attempts (doubling from base, capped at max --
// the same shape this repo's network-retry convention uses elsewhere for
// gh/git calls).
const (
	defaultRetryBaseDelay = 2 * time.Second
	defaultRetryMaxDelay  = 30 * time.Second
)

// retryBackoff returns the delay before the retry attempt numbered
// attempt (0-indexed: the delay before the SECOND Send call overall is
// retryBackoff(0, ...)), doubling from base and capped at max.
func retryBackoff(attempt int, base, max time.Duration) time.Duration {
	d := base
	for i := 0; i < attempt; i++ {
		if d >= max {
			return max
		}
		d *= 2
	}
	if d > max {
		return max
	}
	return d
}

// isTransientNetworkError reports whether err is a connection-level
// failure worth retrying -- a dropped connection, a dial failure, a
// transport-level timeout, or a truncated response -- as opposed to an
// application-level failure (a non-2xx status, a malformed response body)
// that retrying cannot fix. Router.Complete uses this to decide whether a
// failed provider.Send call gets retried.
//
// net/http always wraps a Client.Do transport failure in *url.Error,
// which itself satisfies net.Error (including the case where the
// underlying cause is the request's own context deadline expiring), so
// the errors.As(err, &netErr) check below is the primary path; the
// explicit io/syscall/context checks are a fallback for a Provider
// implementation that returns an unwrapped underlying error directly.
func isTransientNetworkError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return false
}
