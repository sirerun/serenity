// Package server implements Serenity's HTTP transport (RFC 0001 §14):
// bound to loopback by default, with a bearer token required on every
// route — loopback is authenticated too, because any local process is not
// automatically trusted. LAN/Tailscale exposure is explicit, separately
// named config, with optional mTLS layered on top.
package server

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/secrets"
)

// ErrNonLoopbackBindRefused reports an attempt to bind to a non-loopback
// address without the explicit LAN/Tailscale opt-in RFC §14 requires. It
// is returned by Listen, not net.Listen's own generic error, so callers
// and tests can distinguish "refused by policy" from "address in use" or
// similar.
var ErrNonLoopbackBindRefused = errors.New("server: non-loopback bind refused (set AllowLAN to expose beyond loopback)")

// DefaultBind is the address used when Config.Bind is empty: loopback,
// OS-assigned port.
const DefaultBind = "127.0.0.1:0"

// shutdownGrace bounds how long Serve waits for in-flight requests to
// finish once its context is canceled.
const shutdownGrace = 5 * time.Second

// Config configures the HTTP transport.
type Config struct {
	// Bind is the listen address ("host:port"). Empty selects
	// DefaultBind.
	Bind string
	// AllowLAN must be true for Bind to resolve to anything other than a
	// loopback address (RFC §14: "LAN/Tailscale exposure is explicit
	// config").
	AllowLAN bool
	// ClientCAFile, together with ServerCertFile/ServerKeyFile, turns on
	// mTLS: connections must present a certificate signed by this CA.
	// Only consulted when AllowLAN is true.
	ClientCAFile string
	// ServerCertFile and ServerKeyFile are the daemon's own TLS identity,
	// required alongside ClientCAFile for mTLS.
	ServerCertFile string
	ServerKeyFile  string
	// TokenSource returns the current daemon bearer token. Defaults to
	// secrets.DaemonToken when nil; tests inject a fake to avoid the OS
	// keychain.
	TokenSource func() (string, error)
}

// FromBrainConfig builds a transport Config from serenity.yml's `server:`
// section (internal/config.Server), the config-loading pattern the rest
// of the CLI already uses rather than a parallel mechanism.
func FromBrainConfig(sc config.Server) Config {
	return Config{
		Bind:           sc.Bind,
		AllowLAN:       sc.AllowLAN,
		ClientCAFile:   sc.ClientCAFile,
		ServerCertFile: sc.ServerCertFile,
		ServerKeyFile:  sc.ServerKeyFile,
	}
}

// Server is Serenity's HTTP transport: a loopback-by-default listener
// with bearer-token auth wrapping every registered route.
type Server struct {
	cfg      Config
	mux      *http.ServeMux
	listener net.Listener
	httpSrv  *http.Server
}

// New builds a Server. It registers /healthz (authenticated, like every
// other route — RFC §14 has no anonymous endpoints). It does not bind a
// socket; call Listen for that.
func New(cfg Config) *Server {
	if cfg.TokenSource == nil {
		cfg.TokenSource = secrets.DaemonToken
	}
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.Handle("/healthz", http.HandlerFunc(handleHealthz))
	return s
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// Handle registers an authenticated route. Every route added this way
// requires the bearer token (RFC §14: "every protocol endpoint
// authenticates"); there is no way to register an anonymous one.
func (s *Server) Handle(pattern string, h http.Handler) {
	s.mux.Handle(pattern, s.authenticate(h))
}

// authenticate wraps h to require `Authorization: Bearer <token>` matching
// the daemon token. The comparison is constant-time to avoid a timing
// oracle on the token value.
func (s *Server) authenticate(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want, err := s.cfg.TokenSource()
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		got, ok := bearerToken(r)
		if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	token := strings.TrimPrefix(h, prefix)
	if token == "" {
		return "", false
	}
	return token, true
}

// Listen binds the configured address, refusing a non-loopback bind
// without explicit opt-in (RFC §14). Call Serve to start accepting
// connections.
func (s *Server) Listen() error {
	bind := s.cfg.Bind
	if bind == "" {
		bind = DefaultBind
	}
	if !s.cfg.AllowLAN {
		if err := requireLoopback(bind); err != nil {
			return err
		}
	}

	tlsConfig, err := s.tlsConfig()
	if err != nil {
		return fmt.Errorf("server: tls config: %w", err)
	}

	var ln net.Listener
	if tlsConfig != nil {
		ln, err = tls.Listen("tcp", bind, tlsConfig)
	} else {
		ln, err = net.Listen("tcp", bind)
	}
	if err != nil {
		return fmt.Errorf("server: listen %s: %w", bind, err)
	}

	s.listener = ln
	s.httpSrv = &http.Server{Handler: s.mux}
	return nil
}

// requireLoopback rejects any bind address that isn't 127.0.0.1, ::1, or
// localhost. An empty host (e.g. ":8443") binds every interface and is
// refused just as any other non-loopback address is.
func requireLoopback(bind string) error {
	host, _, err := net.SplitHostPort(bind)
	if err != nil {
		return fmt.Errorf("server: parse bind address %q: %w", bind, err)
	}
	if isLoopbackHost(host) {
		return nil
	}
	return fmt.Errorf("%w: %q", ErrNonLoopbackBindRefused, bind)
}

func isLoopbackHost(host string) bool {
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// tlsConfig builds the mTLS server config when LAN exposure and a client
// CA are both configured. It returns (nil, nil) for the plain-HTTP
// loopback default.
func (s *Server) tlsConfig() (*tls.Config, error) {
	if !s.cfg.AllowLAN || s.cfg.ClientCAFile == "" {
		return nil, nil
	}
	if s.cfg.ServerCertFile == "" || s.cfg.ServerKeyFile == "" {
		return nil, fmt.Errorf("mTLS requires ServerCertFile and ServerKeyFile alongside ClientCAFile")
	}
	cert, err := tls.LoadX509KeyPair(s.cfg.ServerCertFile, s.cfg.ServerKeyFile)
	if err != nil {
		return nil, fmt.Errorf("load server certificate: %w", err)
	}
	caPEM, err := os.ReadFile(s.cfg.ClientCAFile)
	if err != nil {
		return nil, fmt.Errorf("read client CA file %s: %w", s.cfg.ClientCAFile, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("no certificates parsed from %s", s.cfg.ClientCAFile)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
	}, nil
}

// Addr returns the bound address. Valid only after a successful Listen.
func (s *Server) Addr() string {
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Serve accepts connections until ctx is done or the listener returns an
// unrecoverable error. Listen must be called first.
func (s *Server) Serve(ctx context.Context) error {
	if s.listener == nil {
		return errors.New("server: Serve called before Listen")
	}
	errCh := make(chan error, 1)
	go func() { errCh <- s.httpSrv.Serve(s.listener) }()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		_ = s.httpSrv.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// Close shuts the server down immediately without waiting for in-flight
// requests. Test/cleanup helper; Serve's ctx cancellation is the graceful
// path.
func (s *Server) Close() error {
	if s.httpSrv == nil {
		return nil
	}
	return s.httpSrv.Close()
}
