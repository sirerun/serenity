package server

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/secrets"
)

func TestMain(m *testing.M) {
	secrets.MockForTesting() // never touch the real OS keychain from tests
	m.Run()
}

// TestListenRefusesNonLoopback proves the acc line "default config
// refuses a non-loopback bind with a named error at the listener": no
// AllowLAN opt-in, a non-loopback bind address, Listen must return
// ErrNonLoopbackBindRefused and leave nothing listening.
func TestListenRefusesNonLoopback(t *testing.T) {
	for _, bind := range []string{"203.0.113.1:9443", "0.0.0.0:9443", ":9443"} {
		s := New(Config{Bind: bind, TokenSource: func() (string, error) { return "t", nil }})
		err := s.Listen()
		if !errors.Is(err, ErrNonLoopbackBindRefused) {
			t.Fatalf("bind %q: Listen err = %v, want ErrNonLoopbackBindRefused", bind, err)
		}
		if s.listener != nil {
			t.Fatalf("bind %q: listener set after refused bind", bind)
		}
		if addr := s.Addr(); addr != "" {
			t.Fatalf("bind %q: Addr() = %q after refused bind, want empty", bind, addr)
		}
	}
}

// TestListenAcceptsLoopback proves loopback binds succeed with no
// LAN opt-in at all -- the secure default is usable, not just refusing.
func TestListenAcceptsLoopback(t *testing.T) {
	for _, bind := range []string{"127.0.0.1:0", "[::1]:0", "localhost:0"} {
		s := New(Config{Bind: bind, TokenSource: func() (string, error) { return "t", nil }})
		if err := s.Listen(); err != nil {
			t.Fatalf("bind %q: Listen err = %v, want nil", bind, err)
		}
		if s.Addr() == "" {
			t.Fatalf("bind %q: Addr() empty after successful Listen", bind)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("bind %q: Close err = %v", bind, err)
		}
	}
}

func TestListenConfiguresHTTPResourceLimits(t *testing.T) {
	s := New(Config{Bind: "127.0.0.1:0", TokenSource: func() (string, error) { return "t", nil }})
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = s.Close() }()
	if got := s.httpSrv.ReadHeaderTimeout; got != readHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %s, want %s", got, readHeaderTimeout)
	}
	if got := s.httpSrv.IdleTimeout; got != idleTimeout {
		t.Fatalf("IdleTimeout = %s, want %s", got, idleTimeout)
	}
	if got := s.httpSrv.MaxHeaderBytes; got != maxHeaderBytes {
		t.Fatalf("MaxHeaderBytes = %d, want %d", got, maxHeaderBytes)
	}
	if got := s.httpSrv.WriteTimeout; got != 0 {
		t.Fatalf("WriteTimeout = %s, want zero for long-running tool calls", got)
	}
}

// TestListenExplicitLANOptIn proves AllowLAN is a real opt-in, not dead
// config: the same non-loopback bind that TestListenRefusesNonLoopback
// rejects succeeds once AllowLAN is set.
func TestListenExplicitLANOptIn(t *testing.T) {
	s := New(Config{Bind: "0.0.0.0:0", AllowLAN: true, TokenSource: func() (string, error) { return "t", nil }})
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen with AllowLAN=true err = %v, want nil", err)
	}
	defer func() { _ = s.Close() }()
	if s.Addr() == "" {
		t.Fatal("Addr() empty after successful Listen")
	}
}

// startTestServer binds an ephemeral loopback port, registers a second
// route beyond /healthz, serves in the background, and returns the base
// URL plus a cleanup func.
func startTestServer(t *testing.T, tokenSource func() (string, error)) (baseURL string) {
	t.Helper()
	s := New(Config{Bind: "127.0.0.1:0", TokenSource: tokenSource})
	s.Handle("/v1/ping", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pong"))
	}))
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = s.Serve(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(8 * time.Second):
			// Server's own shutdownGrace is 5s; a 2s cleanup margin
			// (tighter than the thing it's waiting on) is an
			// intermittent flake under load or -race, verified while
			// building T4.4: reproduced ~2/3 of standalone runs with
			// -race. Give real headroom above shutdownGrace instead of
			// racing it.
			t.Fatal("server did not shut down within 8s")
		}
	})
	return "http://" + s.Addr()
}

func TestLongHandlerCallSurvivesTransportLimits(t *testing.T) {
	const token = "long-call-token"
	s := New(Config{Bind: "127.0.0.1:0", TokenSource: func() (string, error) { return token, nil }})
	s.Handle("/v1/long", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(8 * time.Second):
			t.Fatal("server did not shut down")
		}
	}()
	req, err := http.NewRequest(http.MethodGet, "http://"+s.Addr()+"/v1/long", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("long request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("long request status = %d, want 200", resp.StatusCode)
	}
}

func TestOversizedHeaderRejectedByRealListener(t *testing.T) {
	const token = "header-limit-token"
	base := startTestServer(t, func() (string, error) { return token, nil })
	req, err := http.NewRequest(http.MethodGet, base+"/v1/ping", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Overlarge", strings.Repeat("a", 2*maxHeaderBytes))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("oversized-header request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusRequestHeaderFieldsTooLarge {
		t.Fatalf("oversized-header status = %d, want %d", resp.StatusCode, http.StatusRequestHeaderFieldsTooLarge)
	}
}

func TestIncompleteHeaderTimesOutOnRealListener(t *testing.T) {
	s := newWithTransportLimits(Config{Bind: "127.0.0.1:0", TokenSource: func() (string, error) { return "t", nil }}, 50*time.Millisecond, time.Second, maxHeaderBytes)
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = s.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.Serve(ctx) }()
	conn, err := net.Dial("tcp", s.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := io.WriteString(conn, "GET /healthz HTTP/1.1\r\nHost: localhost\r\n"); err != nil {
		t.Fatal(err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(conn).ReadByte(); err == nil {
		t.Fatal("incomplete header connection remained open")
	}
}

func TestIdleKeepAliveClosesOnRealListener(t *testing.T) {
	s := newWithTransportLimits(Config{Bind: "127.0.0.1:0", TokenSource: func() (string, error) { return "t", nil }}, time.Second, 50*time.Millisecond, maxHeaderBytes)
	if err := s.Listen(); err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = s.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.Serve(ctx) }()
	conn, err := net.Dial("tcp", s.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := io.WriteString(conn, "GET /healthz HTTP/1.1\r\nHost: localhost\r\nAuthorization: Bearer t\r\nConnection: keep-alive\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatalf("read keep-alive response: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(conn).ReadByte(); err == nil {
		t.Fatal("idle keep-alive connection remained open")
	}
}

func get(t *testing.T, url, token string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
}

// TestBearerAuthEveryRoute proves the acc lines "no token, or a wrong
// token, on any route (including /healthz-style variants) returns 401"
// and "correct token returns 200" -- against a real listener, for two
// distinct routes.
func TestBearerAuthEveryRoute(t *testing.T) {
	const want = "s3cr3t-token"
	base := startTestServer(t, func() (string, error) { return want, nil })

	for _, path := range []string{"/healthz", "/v1/ping"} {
		t.Run(path+"/no-token", func(t *testing.T) {
			resp := get(t, base+path, "")
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", resp.StatusCode)
			}
		})
		t.Run(path+"/wrong-token", func(t *testing.T) {
			resp := get(t, base+path, "not-the-token")
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", resp.StatusCode)
			}
		})
		t.Run(path+"/correct-token", func(t *testing.T) {
			resp := get(t, base+path, want)
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if len(body) == 0 {
				t.Fatal("empty body on authenticated success")
			}
		})
	}
}

// TestTokenRotationInvalidatesOldToken proves the acc line
// "`serenity connect --rotate-token` invalidates the old token": through
// the real internal/secrets integration (mocked keyring, never the OS
// keychain), the old token stops working and the new one works, with no
// server restart required since TokenSource is read per request.
func TestTokenRotationInvalidatesOldToken(t *testing.T) {
	oldToken, _, err := secrets.EnsureDaemonToken()
	if err != nil {
		t.Fatalf("EnsureDaemonToken: %v", err)
	}
	base := startTestServer(t, secrets.DaemonToken)

	resp := get(t, base+"/healthz", oldToken)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pre-rotation status = %d, want 200", resp.StatusCode)
	}

	newToken, err := secrets.RotateDaemonToken()
	if err != nil {
		t.Fatalf("RotateDaemonToken: %v", err)
	}
	if newToken == oldToken {
		t.Fatal("RotateDaemonToken returned the same token")
	}

	resp = get(t, base+"/healthz", oldToken)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("post-rotation old-token status = %d, want 401", resp.StatusCode)
	}

	resp = get(t, base+"/healthz", newToken)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post-rotation new-token status = %d, want 200", resp.StatusCode)
	}
}

func TestFromBrainConfig(t *testing.T) {
	sc := config.Server{
		Bind:           "127.0.0.1:9443",
		AllowLAN:       true,
		ClientCAFile:   "/path/ca.pem",
		ServerCertFile: "/path/cert.pem",
		ServerKeyFile:  "/path/key.pem",
	}
	got := FromBrainConfig(sc)
	want := Config{
		Bind:           "127.0.0.1:9443",
		AllowLAN:       true,
		ClientCAFile:   "/path/ca.pem",
		ServerCertFile: "/path/cert.pem",
		ServerKeyFile:  "/path/key.pem",
	}
	if got.Bind != want.Bind || got.AllowLAN != want.AllowLAN || got.ClientCAFile != want.ClientCAFile ||
		got.ServerCertFile != want.ServerCertFile || got.ServerKeyFile != want.ServerKeyFile {
		t.Fatalf("FromBrainConfig = %+v, want %+v", got, want)
	}
}
