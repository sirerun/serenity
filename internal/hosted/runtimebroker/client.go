package runtimebroker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/sirerun/serenity/internal/hosted/authority"
)

const maxWireBytes = authority.MaxBodyBytes + 1024

type localTransport struct {
	mu     sync.Mutex
	conn   net.Conn
	reader *bufio.Reader
	closed bool
}

func newLocalTransport(ctx context.Context, path string) (*localTransport, error) {
	conn, err := dialBroker(ctx, path)
	if err != nil {
		return nil, err
	}
	return &localTransport{conn: conn, reader: bufio.NewReaderSize(conn, maxWireBytes+2)}, nil
}
func (t *localTransport) Invoke(ctx context.Context, q authority.Request) (authority.Response, error) {
	var out authority.Response
	if ctx == nil {
		return out, authority.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return out, authority.ErrUnavailable
	}
	b, err := json.Marshal(q)
	if err != nil || len(b) > authority.MaxBodyBytes {
		return out, authority.ErrInvalid
	}
	deadline := time.Now().Add(30 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err = t.conn.SetDeadline(deadline); err != nil {
		return out, err
	}
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = t.conn.SetDeadline(time.Now()); close(done) })
	defer func() {
		if !stop() {
			<-done
		}
	}()
	if _, err = t.conn.Write(append(b, '\n')); err != nil {
		t.closed = true
		return out, errors.Join(authority.ErrUnavailable, err, t.conn.Close())
	}
	frame, err := t.reader.ReadSlice('\n')
	if err != nil || len(frame) > maxWireBytes+1 {
		t.closed = true
		return out, errors.Join(authority.ErrUnavailable, err, t.conn.Close())
	}
	var response wireResponse
	if err = strictJSON(frame, &response); err != nil {
		t.closed = true
		return out, errors.Join(authority.ErrUnavailable, err, t.conn.Close())
	}
	return response.Result, decodeError(response.Error)
}
func (t *localTransport) Close() error {
	// Closing the descriptor interrupts an in-flight bounded request before
	// taking its serialization lock; expiry cannot wait behind that request.
	err := t.conn.Close()
	t.mu.Lock()
	t.closed = true
	t.mu.Unlock()
	if errors.Is(err, net.ErrClosed) {
		return nil
	}
	return err
}

type wireResponse struct {
	Result authority.Response `json:"result"`
	Error  string             `json:"error,omitempty"`
}

func code(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, authority.ErrForbidden):
		return "forbidden"
	case errors.Is(err, authority.ErrInvalid):
		return "invalid"
	case errors.Is(err, authority.ErrConflict):
		return "conflict"
	case errors.Is(err, authority.ErrExpired):
		return "expired"
	default:
		return "unavailable"
	}
}
func decodeError(code string) error {
	switch code {
	case "":
		return nil
	case "forbidden":
		return authority.ErrForbidden
	case "invalid":
		return authority.ErrInvalid
	case "conflict":
		return authority.ErrConflict
	case "expired":
		return authority.ErrExpired
	default:
		return authority.ErrUnavailable
	}
}
