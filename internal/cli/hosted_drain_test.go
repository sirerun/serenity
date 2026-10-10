package cli

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"
)

func TestHostedHandlerFenceAndDrain(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan struct{})
	h := &hostedRequests{next: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { close(entered); <-release; close(finished) })}
	go h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	<-entered
	h.stop()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatal("new request admitted during drain")
	}
	joined := make(chan struct{})
	go func() { h.wg.Wait(); close(joined) }()
	select {
	case <-joined:
		t.Fatal("handler not joined")
	default:
	}
	close(release)
	<-finished
	select {
	case <-joined:
	case <-time.After(time.Second):
		t.Fatal("handler failed to join")
	}
}
func TestHostedServersJoinInflightRequestsBeforeReturning(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	admin, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close() }()
	entered := make(chan struct{})
	release := make(chan struct{})
	a := &hostedRequests{next: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.WriteHeader(http.StatusNoContent)
	})}
	b := &hostedRequests{next: http.NotFoundHandler()}
	public, private := &http.Server{Handler: a}, &http.Server{Handler: b}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serveHosted(ctx, func() {}, public, private, listener, admin, a, b) }()
	response := make(chan error, 1)
	go func() {
		resp, e := http.Get("http://" + listener.Addr().String())
		if e == nil {
			_, e = io.Copy(io.Discard, resp.Body)
			e = errors.Join(e, resp.Body.Close())
		}
		response <- e
	}()
	<-entered
	cancel()
	deadline := time.Now().Add(time.Second)
	for {
		a.mu.Lock()
		stopped := a.stopped
		a.mu.Unlock()
		if stopped {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not fence admission")
		}
		runtime.Gosched()
	}
	select {
	case e := <-done:
		t.Fatalf("returned before handler completed: %v", e)
	default:
	}
	close(release)
	if err := <-response; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server failed to finish drain")
	}
}
