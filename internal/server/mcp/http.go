package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sync"
	"time"
)

// Streamable HTTP transport (MCP 2025-11-25, "Streamable HTTP"). Scope
// (T4.21): a single POST/DELETE endpoint returning JSON only -- no
// unsolicited server-to-client SSE stream, no stream resumability. GET
// returns 405. This is a spec-legal reduced surface, not a partial
// implementation of the full transport: the spec's SSE response mode and
// Last-Event-ID resumability are both explicitly optional server
// capabilities.
const (
	// SessionIDHeader carries this transport's own session identity,
	// assigned by the server on a successful "initialize" and echoed by
	// the client on every subsequent request against that session. This is
	// a DIFFERENT header from the daemon's own Authorization bearer token
	// (internal/server's auth, unconditionally required on every route
	// including this one) -- a valid session id is not a credential and
	// grants nothing on its own without the token also being present.
	SessionIDHeader = "Mcp-Session-Id"

	// ProtocolVersionHeader carries MCP's own negotiated protocol version
	// on every HTTP request except the bootstrapping "initialize" call.
	// This is a THIRD, distinct notion of "version" that this task's own
	// acc line requires keeping apart from the other two: (a) the JSON-RPC
	// envelope's fixed "jsonrpc":"2.0" field (response.JSONRPC, never
	// negotiated, never this header's value) and (b) MEMORY_VERBS v1's own
	// domain protocol_version integer embedded in verb payloads
	// (internal/server/memory.ProtocolVersion, 1) -- a field inside a
	// tool's JSON *content*, not a transport header, and never read or
	// written by this file.
	ProtocolVersionHeader = "MCP-Protocol-Version"

	// MaxHTTPSessions bounds the number of concurrent Streamable HTTP
	// sessions one HTTPHandler tracks -- every route already requires the
	// daemon bearer token (RFC 0001 §14), but an authenticated client
	// still should not be able to grow unbounded server-side state by
	// opening sessions and never closing them.
	MaxHTTPSessions = 64

	// SessionIdleTimeout evicts a session that has not been used (any
	// POST/DELETE naming its id) for this long, bounding session resources
	// held by a client that disconnected without sending DELETE.
	SessionIdleTimeout = 30 * time.Minute
)

// errTooManySessions is returned by HTTPHandler's internal session
// bookkeeping when MaxHTTPSessions is already held; handlePost maps it to
// 503.
var errTooManySessions = errors.New("mcp: too many concurrent HTTP sessions")

type httpSession struct {
	session  *Session
	lastSeen time.Time
}

// HTTPHandler implements the Streamable HTTP transport over one Server's
// tool registry. Register it on an authenticated internal/server.Server
// route (e.g. s.Handle("/mcp", h)) -- this handler performs no auth or
// listener binding of its own, by design: RFC 0001 §14's bearer-token/mTLS
// check already wraps every registered route on that Server, and
// duplicating it here would be a second, divergent auth path; likewise
// loopback-vs-LAN binding stays entirely internal/server's own concern.
//
// Every accepted tools/call runs under h's own ctx (process/handler
// lifetime), never an individual HTTP request's r.Context(): request-body
// EOF or the client disconnecting only makes the waiting POST handler give
// up on delivering the eventual response -- it does not cancel the call
// already running (T4.21 acc line). Only an explicit notifications/
// cancelled, routed through the named session, cancels a call early.
// Close cancels h's ctx and joins every such call before returning, so a
// process shutdown drains in-flight tool work before the caller closes
// whatever the tool handlers still depend on (the shared writer queue, the
// derived index) -- the same order stdio's own Serve already guarantees
// for one connection, extended here across every concurrent HTTP session.
type HTTPHandler struct {
	srv    *Server
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	sessions map[string]*httpSession

	workers sync.WaitGroup
}

// NewHTTPHandler builds the Streamable HTTP MCP handler serving srv's tool
// registry.
func NewHTTPHandler(srv *Server) *HTTPHandler {
	ctx, cancel := context.WithCancel(context.Background())
	return &HTTPHandler{srv: srv, ctx: ctx, cancel: cancel, sessions: make(map[string]*httpSession)}
}

// Close cancels every in-flight tool call's context and blocks until each
// one's goroutine returns, then drops every tracked session. Call it once,
// after the listener has stopped accepting new connections (internal/
// server.Server.Serve returning) and before closing anything a tool
// handler depends on.
func (h *HTTPHandler) Close() {
	h.cancel()
	h.workers.Wait()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessions = make(map[string]*httpSession)
}

// ServeHTTP implements http.Handler. POST carries one JSON-RPC message;
// DELETE ends a session early. Every other method, GET included, is 405 --
// this transport never upgrades a response to text/event-stream (scope
// note: "JSON-only responses and GET 405 are sufficient").
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.handlePost(w, r)
	case http.MethodDelete:
		h.handleDelete(w, r)
	default:
		w.Header().Set("Allow", "POST, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// rejectedOrigin reports whether r must be refused before any tool
// invocation or session lookup happens. MCP's own Streamable HTTP transport
// spec requires validating Origin to defend against a hostile web page
// driving this loopback endpoint via a victim's browser (DNS rebinding /
// CSRF-shaped attacks) -- no legitimate MCP client (this repo's CLI, an
// external agent host, gbrain's own StreamableHTTPClientTransport) is a
// browser page and none sets Origin, so the correct default with no
// browser-based client to accommodate is to refuse any request that
// carries one at all, rather than inventing an allowlist with nothing yet
// to allow.
func rejectedOrigin(r *http.Request) bool { return r.Header.Get("Origin") != "" }

func hasJSONContentType(r *http.Request) bool {
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mt == "application/json"
}

func (h *HTTPHandler) handlePost(w http.ResponseWriter, r *http.Request) {
	if rejectedOrigin(r) {
		http.Error(w, "Origin header not allowed", http.StatusForbidden)
		return
	}
	if !hasJSONContentType(r) {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxFrameBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "request body exceeds the size limit or could not be read", http.StatusRequestEntityTooLarge)
		return
	}

	sessionID := r.Header.Get(SessionIDHeader)
	if sessionID == "" {
		h.bootstrap(w, body)
		return
	}

	if r.Header.Get(ProtocolVersionHeader) != ProtocolVersion {
		http.Error(w, "missing or unsupported "+ProtocolVersionHeader, http.StatusBadRequest)
		return
	}
	sess, ok := h.lookupSession(sessionID)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	req, bad := parse(body)
	if bad != nil {
		writeJSON(w, *bad)
		return
	}
	if len(req.id) == 0 {
		// A notification (or a batch of responses this server never
		// sends): per spec, no body, 202 Accepted.
		sess.HandleNotification(req)
		w.WriteHeader(http.StatusAccepted)
		return
	}

	reply, pending, fatal := sess.HandleRequest(h.ctx, req)
	if fatal != nil {
		// HTTP has no persistent connection to end the way stdio ends
		// Serve on a fatal protocol violation; report it as this one
		// request's own JSON-RPC error instead of dropping the session.
		writeJSON(w, failure(req.id, -32600, fatal.Error()))
		return
	}
	if pending == nil {
		writeJSON(w, *reply)
		return
	}

	done := make(chan response, 1)
	h.workers.Go(func() {
		reply, ok := pending.Run()
		if ok {
			done <- reply
		}
		close(done)
	})
	select {
	case reply, ok := <-done:
		if !ok {
			// Cancelled: MCP sends no response, success or failure, for a
			// cancelled call.
			return
		}
		writeJSON(w, reply)
	case <-r.Context().Done():
		// The client disconnected, or the request body hit EOF. The tool
		// call keeps running under h.ctx (not r.Context()) -- it is
		// tracked by h.workers and joined by Close, but its result is
		// simply undeliverable on this now-dead response writer.
		return
	}
}

// bootstrap handles a POST with no Mcp-Session-Id header: per the
// Streamable HTTP spec, the only valid first message on a fresh HTTP
// connection to a session-issuing server is "initialize". A parse failure
// gets the ordinary JSON-RPC parse-error envelope (no session needed to
// report that); anything else that isn't "initialize" is a transport-level
// 400, since there is no session for its reply to belong to.
func (h *HTTPHandler) bootstrap(w http.ResponseWriter, body []byte) {
	req, bad := parse(body)
	if bad != nil {
		writeJSON(w, *bad)
		return
	}
	if req.method != "initialize" {
		http.Error(w, "initialize must be the first request on a new HTTP connection", http.StatusBadRequest)
		return
	}
	id, sess, err := h.newSession()
	if err != nil {
		if errors.Is(err, errTooManySessions) {
			http.Error(w, "too many concurrent MCP sessions", http.StatusServiceUnavailable)
		} else {
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	reply, _, fatal := sess.HandleRequest(h.ctx, req)
	if fatal != nil {
		// Unreachable in practice: a brand-new session's call set is
		// empty, so HandleRequest's own duplicate-id check can never fire
		// on the very first request. Handled anyway rather than assumed.
		h.dropSession(id)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if reply.Error != nil {
		// Bad initialize params: don't hold a session open that can never
		// leave state 0. The client retries the same way, no session id.
		h.dropSession(id)
		writeJSON(w, *reply)
		return
	}
	w.Header().Set(SessionIDHeader, id)
	w.Header().Set(ProtocolVersionHeader, ProtocolVersion)
	writeJSON(w, *reply)
}

func (h *HTTPHandler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if rejectedOrigin(r) {
		http.Error(w, "Origin header not allowed", http.StatusForbidden)
		return
	}
	sessionID := r.Header.Get(SessionIDHeader)
	if sessionID == "" {
		http.Error(w, SessionIDHeader+" header is required", http.StatusBadRequest)
		return
	}
	if _, ok := h.lookupSession(sessionID); !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	// DELETE ends the session for future requests; it deliberately does
	// not cancel any call still running under it -- the same "a client-
	// facing event never implicitly cancels tool work" posture as an
	// individual request disconnecting (see handlePost). Only an explicit
	// notifications/cancelled cancels a call early.
	h.dropSession(sessionID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) newSession() (id string, sess *Session, err error) {
	id, err = newSessionID()
	if err != nil {
		return "", nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.evictIdleLocked()
	if len(h.sessions) >= MaxHTTPSessions {
		return "", nil, errTooManySessions
	}
	sess = h.srv.NewSession()
	h.sessions[id] = &httpSession{session: sess, lastSeen: time.Now()}
	return id, sess, nil
}

func (h *HTTPHandler) lookupSession(id string) (*Session, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.evictIdleLocked()
	hs, ok := h.sessions[id]
	if !ok {
		return nil, false
	}
	hs.lastSeen = time.Now()
	return hs.session, true
}

func (h *HTTPHandler) dropSession(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.sessions, id)
}

// evictIdleLocked must be called with h.mu held.
func (h *HTTPHandler) evictIdleLocked() {
	cutoff := time.Now().Add(-SessionIdleTimeout)
	for id, hs := range h.sessions {
		if hs.lastSeen.Before(cutoff) {
			delete(h.sessions, id)
		}
	}
}

func newSessionID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate MCP session id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func writeJSON(w http.ResponseWriter, v response) {
	data, err := json.Marshal(v)
	if err != nil {
		// v is always built from this package's own success/failure
		// helpers or a tool's own already-marshaled Result (invoke already
		// proved it encodes, upstream) -- Marshal only fails here on a
		// genuinely unexpected internal defect.
		http.Error(w, "encode response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}
