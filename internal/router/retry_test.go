package router

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"
	"testing"
	"time"
)

// fakeNetError is a minimal net.Error implementation for constructing a
// deterministic transient network failure in tests, without depending on
// a real dialer or a live listener. Test-file only, per the zero-stub
// policy.
type fakeNetError struct {
	msg     string
	timeout bool
}

func (e *fakeNetError) Error() string   { return e.msg }
func (e *fakeNetError) Timeout() bool   { return e.timeout }
func (e *fakeNetError) Temporary() bool { return e.timeout } //nolint:staticcheck // net.Error still requires this method (Go 1.26).

var _ net.Error = (*fakeNetError)(nil)

func TestHTTPClientOrDefaultSetsTimeoutWhenNil(t *testing.T) {
	got := httpClientOrDefault(nil)
	if got == http.DefaultClient {
		t.Fatal("httpClientOrDefault(nil) returned http.DefaultClient, whose zero-value Timeout never bounds a call")
	}
	if got.Timeout != defaultProviderHTTPTimeout {
		t.Fatalf("httpClientOrDefault(nil).Timeout = %v, want %v", got.Timeout, defaultProviderHTTPTimeout)
	}
}

func TestHTTPClientOrDefaultPreservesCallerClient(t *testing.T) {
	caller := &http.Client{Timeout: 7 * time.Second}
	got := httpClientOrDefault(caller)
	if got != caller {
		t.Fatal("httpClientOrDefault did not return the caller's own *http.Client unchanged")
	}
}

func TestRetryBackoffDoublesAndCaps(t *testing.T) {
	base := 2 * time.Second
	max := 30 * time.Second
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 2 * time.Second},
		{1, 4 * time.Second},
		{2, 8 * time.Second},
		{3, 16 * time.Second},
		{4, 30 * time.Second}, // would be 32s uncapped
		{10, 30 * time.Second},
	}
	for _, c := range cases {
		got := retryBackoff(c.attempt, base, max)
		if got != c.want {
			t.Fatalf("retryBackoff(%d, %v, %v) = %v, want %v", c.attempt, base, max, got, c.want)
		}
	}
}

func TestIsTransientNetworkErrorClassification(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"net.Error timeout", &fakeNetError{msg: "dial tcp: i/o timeout", timeout: true}, true},
		{"net.Error non-timeout", &fakeNetError{msg: "some net error"}, true},
		{"wrapped net.Error", fmt.Errorf("router: %w", &fakeNetError{msg: "connection reset"}), true},
		{"io.EOF", io.EOF, true},
		{"io.ErrUnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"ECONNRESET", syscall.ECONNRESET, true},
		{"ECONNREFUSED", syscall.ECONNREFUSED, true},
		{"EPIPE", syscall.EPIPE, true},
		{"context.DeadlineExceeded", context.DeadlineExceeded, true},
		{"application-level status error", errors.New("openai_compatible: status 500: internal server error"), false},
		{"malformed body error", errors.New("openai_compatible: decode response: unexpected end of JSON input"), false},
	}
	for _, c := range cases {
		got := isTransientNetworkError(c.err)
		if got != c.want {
			t.Fatalf("isTransientNetworkError(%v) [%s] = %v, want %v", c.err, c.name, got, c.want)
		}
	}
}
