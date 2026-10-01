package admintransport

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func privatePath(t *testing.T) string {
	t.Helper()
	parent, err := os.MkdirTemp(os.Getenv("TMPDIR"), "admin-peer-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(parent); err != nil {
			t.Error(err)
		}
	})
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(parent, "admin.sock")
}

func TestRealUnixPeerIdentityAndForgedPublicRequest(t *testing.T) {
	path := privatePath(t)
	l, err := Listen(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	info, err := os.Lstat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("socket permissions: %v %v", info, err)
	}
	var calls atomic.Int32
	s, err := l.HTTPServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, ok := AuthenticatedUID(r.Context())
		if !ok || uid != uint32(os.Geteuid()) {
			t.Error("missing verified peer identity")
		}
		calls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Serve(l) }()
	t.Cleanup(func() {
		_ = s.Close()
		if err := <-done; !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("Serve: %v", err)
		}
	})
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", path)
	}}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	res, err := client.Get("http://admin/backup")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent || calls.Load() != 1 {
		t.Fatalf("verified local request: status=%d calls=%d", res.StatusCode, calls.Load())
	}
	r := httptest.NewRequest(http.MethodPost, "/backup", nil)
	r.Header.Set("X-Operator-UID", "0")
	r.Header.Set("Authorization", "Bearer claimed-owner")
	w := httptest.NewRecorder()
	s.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden || calls.Load() != 1 {
		t.Fatal("public request or forged headers authenticated a peer")
	}
}

func TestPrivateDirectoryExistingPathsAndCanceledCreation(t *testing.T) {
	path := privatePath(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Listen(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled creation: %v", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canceled creation published socket")
	}
	if err := os.WriteFile(path, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(context.Background(), path); err == nil {
		t.Fatal("existing file accepted")
	}
	bytes, err := os.ReadFile(path)
	if err != nil || string(bytes) != "preserve" {
		t.Fatal("existing file changed")
	}
	unsafe := privatePath(t)
	if err := os.Chmod(filepath.Dir(unsafe), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(context.Background(), unsafe); err == nil {
		t.Fatal("public directory accepted")
	}
	symlink := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(filepath.Dir(path), symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(context.Background(), filepath.Join(symlink, "new.sock")); err == nil {
		t.Fatal("symlink directory accepted")
	}
}

func TestClosePreservesReplacementAndIsIdempotent(t *testing.T) {
	path := privatePath(t)
	l, err := Listen(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	owned := path + ".owned"
	if err := os.Rename(path, owned); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(path)
	if err != nil || string(bytes) != "replacement" {
		t.Fatal("Close removed a replacement path")
	}
}

func TestUnsafeHigherAncestorRefusedBeforeSocketCreation(t *testing.T) {
	base := filepath.Dir(privatePath(t))
	if err := os.Chmod(base, 0777); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(base, "p")
	child := filepath.Join(parent, "s")
	if err := os.MkdirAll(child, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(child, "admin.sock")
	if l, err := Listen(context.Background(), path); err == nil {
		t.Cleanup(func() { _ = l.Close() })
		t.Fatal("private subtree under non-sticky writable ancestor accepted")
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unsafe ancestor check published socket")
	}
}

func TestWrongOrUnavailablePeerCredentialsNeverReachHandler(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "wrong_uid", true: "credential_error"}[failed], func(t *testing.T) {
			path := privatePath(t)
			checked := make(chan struct{})
			l, err := listen(context.Background(), path, func(*net.UnixConn) (uint32, error) {
				close(checked)
				if failed {
					return 0, errors.New("credential unavailable")
				}
				return uint32(os.Geteuid()) + 1, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = l.Close() })
			done := make(chan error, 1)
			go func() { _, err := l.Accept(); done <- err }()
			conn, err := net.Dial("unix", path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = conn.Close() }()
			<-checked
			if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			var b [1]byte
			if _, err := conn.Read(b[:]); err == nil {
				t.Fatal("rejected peer remained open")
			} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
				t.Fatal("rejected peer was not closed")
			}
			_ = l.Close()
			if err := <-done; !errors.Is(err, net.ErrClosed) {
				t.Fatalf("Accept stopped with %v", err)
			}
		})
	}
}
