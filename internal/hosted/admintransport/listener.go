// Package admintransport supplies an owner-only local admin transport. It
// authenticates an OS peer, not a human approval or a recovery authorization.
package admintransport

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"
)

type peerKey struct{}
type peerIdentity struct{ uid uint32 }
type peerConnection struct {
	net.Conn
	identity peerIdentity
}

// AuthenticatedUID reports only credentials verified by this listener. HTTP
// headers and caller-selected identity strings cannot create this value.
func AuthenticatedUID(ctx context.Context) (uint32, bool) {
	if ctx == nil {
		return 0, false
	}
	p, ok := ctx.Value(peerKey{}).(peerIdentity)
	return p.uid, ok
}

// Listener accepts only peers with the same effective UID as its creator.
// Close removes only its own socket inode; an existing path is never replaced.
type Listener struct {
	listener *net.UnixListener
	owner    uint32
	path     string
	created  os.FileInfo
	peerUID  func(*net.UnixConn) (uint32, error)
	once     sync.Once
	closeErr error
}

// Listen binds a fresh socket in an existing, owner-only directory without
// symlink aliases. The context governs creation; the caller owns Close and
// HTTP server shutdown after this function returns.
func Listen(ctx context.Context, path string) (*Listener, error) {
	return listen(ctx, path, osPeerUID)
}

func listen(ctx context.Context, path string, credentials func(*net.UnixConn) (uint32, error)) (*Listener, error) {
	if ctx == nil {
		return nil, errors.New("admin transport: context required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("admin transport: absolute clean socket path required")
	}
	parent := filepath.Dir(path)
	info, err := os.Lstat(parent)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil || resolved != parent || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("admin transport: private directory required")
	}
	owner := uint32(os.Geteuid())
	if uid, ok := fileUID(info); !ok || uid != owner {
		return nil, errors.New("admin transport: directory owner mismatch")
	}
	// A private final directory is insufficient if another user can rename
	// an ancestor. Trust only root/the creating UID and sticky-protected
	// shared ancestors; check every component through the filesystem root.
	for ancestor := parent; ; ancestor = filepath.Dir(ancestor) {
		if err := ownershipEnforced(ancestor); err != nil {
			return nil, err
		}
		entry, statErr := os.Lstat(ancestor)
		if statErr != nil {
			return nil, statErr
		}
		uid, ok := fileUID(entry)
		if !entry.IsDir() || !ok || (uid != owner && uid != 0) ||
			(entry.Mode().Perm()&0022 != 0 && entry.Mode()&os.ModeSticky == 0) {
			return nil, errors.New("admin transport: unsafe ancestor directory")
		}
		if ancestor == filepath.Dir(ancestor) {
			break
		}
	}
	if _, err = os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			return nil, errors.New("admin transport: socket path exists")
		}
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	u, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	u.SetUnlinkOnClose(false)
	l := &Listener{listener: u, owner: owner, path: path, peerUID: credentials}
	l.created, err = os.Lstat(path)
	if err == nil {
		err = os.Chmod(path, 0600)
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return nil, errors.Join(err, l.Close())
	}
	return l, nil
}

func (l *Listener) Accept() (net.Conn, error) {
	for {
		conn, err := l.listener.AcceptUnix()
		if err != nil {
			return nil, err
		}
		uid, err := l.peerUID(conn)
		if err != nil || uid != l.owner {
			_ = conn.Close()
			continue
		}
		return &peerConnection{Conn: conn, identity: peerIdentity{uid: uid}}, nil
	}
}

func (l *Listener) Addr() net.Addr { return l.listener.Addr() }

func (l *Listener) Close() error {
	l.once.Do(func() {
		l.closeErr = l.listener.Close()
		current, err := os.Lstat(l.path)
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		if err != nil {
			l.closeErr = errors.Join(l.closeErr, err)
			return
		}
		if l.created != nil && os.SameFile(l.created, current) {
			l.closeErr = errors.Join(l.closeErr, os.Remove(l.path))
		}
	})
	return l.closeErr
}

// HTTPServer binds request identity to accepted peer credentials. Serving this
// server over another listener denies requests, even with forged headers.
// The caller must Serve on this Listener and explicitly shut the server down.
func (l *Listener) HTTPServer(handler http.Handler) (*http.Server, error) {
	if handler == nil {
		return nil, errors.New("admin transport: handler required")
	}
	v := reflect.ValueOf(handler)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if v.IsNil() {
			return nil, errors.New("admin transport: handler required")
		}
	}
	return &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 << 10,
		ConnContext: func(ctx context.Context, conn net.Conn) context.Context {
			if peer, ok := conn.(*peerConnection); ok && peer.identity.uid == l.owner {
				return context.WithValue(ctx, peerKey{}, peer.identity)
			}
			return ctx
		},
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if uid, ok := AuthenticatedUID(r.Context()); !ok || uid != l.owner {
				http.Error(w, "Admin transport authorization required", http.StatusForbidden)
				return
			}
			handler.ServeHTTP(w, r)
		}),
	}, nil
}
