package oauth

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ajent-social/go/mcpoauth"
	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/provision"
	"github.com/sirerun/serenity/internal/hosted/store"
)

type Hosted struct {
	Server    *mcpoauth.Server
	Store     *SQLStore
	Identity  *identity.Service
	Provision *provision.Provisioner
	Origin    string
	Dev       bool
}

func New(db *store.Store, id *identity.Service, p *provision.Provisioner, origin string, dev bool) (*Hosted, error) {
	st := &SQLStore{DB: db}
	server, err := mcpoauth.New(mcpoauth.Config{Issuer: origin, Resource: origin + "/mcp", Scopes: []string{"memory:read", "memory:write"}, AllowLoopbackRedirects: true, AllowLocalhostRedirects: true, ConsentTTL: 20 * time.Minute}, st)
	if err != nil {
		return nil, err
	}
	return &Hosted{Server: server, Store: st, Identity: id, Provision: p, Origin: origin, Dev: dev}, nil
}
func (h *Hosted) Verify(ctx context.Context, raw string) (credential.Binding, error) {
	ident, err := h.Server.Verify(ctx, raw)
	if err != nil {
		return credential.Binding{}, credential.ErrInvalidCredential
	}
	brain, generation, ok := strings.Cut(ident.Binding, ".")
	if !ok {
		return credential.Binding{}, credential.ErrInvalidCredential
	}
	var status, state string
	var epoch int
	err = h.Store.DB.DB().QueryRowContext(ctx, `SELECT a.status,b.state,COALESCE(e.generation,0) FROM brains b JOIN accounts a ON a.id=b.account_id LEFT JOIN oauth_epochs e ON e.brain_id=b.id WHERE a.id=? AND b.id=?`, ident.Subject, brain).Scan(&status, &state, &epoch)
	if err != nil || status != "active" || state != "ready" || generation != strconv.Itoa(epoch) {
		return credential.Binding{}, credential.ErrRevoked
	}
	return credential.Binding{AccountID: ident.Subject, BrainID: brain, CredentialID: "oauth:" + ident.GrantID, Generation: epoch + 1, Scopes: ident.Scopes}, nil
}
func (h *Hosted) Handler() http.Handler {
	mux := http.NewServeMux()
	authorize := h.Server.AuthorizeHandler(h.consent)
	mux.HandleFunc("GET /oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		if handle := r.URL.Query().Get("request"); handle != "" {
			req, err := h.Server.Consent(r.Context(), handle)
			if err != nil {
				http.Error(w, "This connection request expired. Start again in your agent.", 400)
				return
			}
			h.consent(w, r, handle, req)
			return
		}
		authorize.ServeHTTP(w, r)
	})
	mux.HandleFunc("POST /oauth/consent", h.approve)
	mux.Handle("/oauth/register", withRateLimit(newIPRateLimiter(10, 100, time.Minute), h.Server.RegisterHandler()))
	mux.Handle("/oauth/token", withRateLimit(newIPRateLimiter(30, 1000, time.Minute), h.Server.TokenHandler()))
	mux.Handle("/oauth/revoke", withRateLimit(newIPRateLimiter(30, 1000, time.Minute), h.Server.RevokeHandler()))
	mux.HandleFunc("GET /oauth/connections", h.connections)
	mux.HandleFunc("POST /oauth/disconnect", h.disconnect)
	return withRateLimit(newIPRateLimiter(120, 2000, time.Minute), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		// Preserve same-origin form Origin for CSRF checks; never disclose
		// the consent handle to a cross-origin OAuth callback.
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; img-src 'self' https://d2ol7oe51mr4n9.cloudfront.net; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		mux.ServeHTTP(w, r)
	}))
}
func (h *Hosted) session(r *http.Request) (identity.Session, error) {
	c, err := r.Cookie("serenity_session")
	if err != nil {
		return identity.Session{}, err
	}
	return h.Identity.Session(r.Context(), c.Value)
}
func (h *Hosted) consent(w http.ResponseWriter, r *http.Request, handle string, request mcpoauth.ConsentRequest) {
	session, err := h.session(r)
	if err != nil {
		http.SetCookie(w, &http.Cookie{Name: "serenity_oauth_resume", Value: handle, Path: "/", HttpOnly: true, Secure: !h.Dev, SameSite: http.SameSiteLaxMode, MaxAge: 1200})
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if _, err = h.Provision.Provision(r.Context(), session.AccountID); err != nil {
		http.Error(w, "Your memory is being prepared. Try again shortly.", http.StatusServiceUnavailable)
		return
	}
	brains, err := h.Store.DB.Brains(r.Context(), session.AccountID)
	if err != nil {
		http.Error(w, "Memory unavailable", http.StatusServiceUnavailable)
		return
	}
	// Chromium applies form-action to redirects after form submission too.
	// Permit only the callback origin of this immutable authorization request.
	record, err := h.Store.Consent(r.Context(), store.Hash(handle))
	if err != nil {
		http.Error(w, "Connection request expired", 400)
		return
	}
	callback, err := url.Parse(record.RedirectURI)
	if err != nil || strings.ContainsAny(callback.Host, ";'\"") {
		http.Error(w, "Invalid callback destination", 400)
		return
	}
	w.Header().Set("Content-Security-Policy", strings.Replace(w.Header().Get("Content-Security-Policy"), "form-action 'self'", "form-action 'self' "+callback.Scheme+"://"+callback.Host, 1))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = consentPage.Execute(w, struct {
		Handle, CSRF, Client, Destinations string
		Brains                             []store.Brain
		Read, Write                        bool
	}{handle, session.CSRF, request.ClientName, callback.Host, brains, slices.Contains(request.Scopes, "memory:read"), slices.Contains(request.Scopes, "memory:write")})
}
func (h *Hosted) mutation(w http.ResponseWriter, r *http.Request) (identity.Session, bool) {
	s, err := h.session(r)
	if err != nil {
		http.Error(w, "Sign in again before connecting.", http.StatusUnauthorized)
		return s, false
	}
	if r.Header.Get("Origin") != h.Origin || r.Header.Get("Sec-Fetch-Site") == "cross-site" || r.ParseForm() != nil || !identity.CheckCSRF(s, r.PostForm.Get("csrf")) {
		http.Error(w, "Invalid consent form", http.StatusForbidden)
		return s, false
	}
	for _, values := range r.PostForm {
		if len(values) != 1 {
			http.Error(w, "Ambiguous consent form", 400)
			return s, false
		}
	}
	return s, true
}
func (h *Hosted) approve(w http.ResponseWriter, r *http.Request) {
	s, ok := h.mutation(w, r)
	if !ok {
		return
	}
	handle := r.PostForm.Get("handle")
	if r.PostForm.Get("decision") == "deny" {
		target, err := h.Server.Deny(r.Context(), handle)
		if err != nil {
			http.Error(w, "Connection request expired", 400)
			return
		}
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	if r.PostForm.Get("decision") != "approve" {
		http.Error(w, "Choose whether to connect", 400)
		return
	}
	brain := r.PostForm.Get("brain")
	if _, err := h.Store.DB.BrainByID(r.Context(), s.AccountID, brain); err != nil {
		http.Error(w, "Project unavailable", http.StatusForbidden)
		return
	}
	var epoch int
	if err := h.Store.DB.DB().QueryRowContext(r.Context(), "SELECT COALESCE((SELECT generation FROM oauth_epochs WHERE brain_id=?),0)", brain).Scan(&epoch); err != nil {
		http.Error(w, "Connection unavailable", http.StatusServiceUnavailable)
		return
	}
	scopes := strings.Fields(r.PostForm.Get("access"))
	if len(scopes) == 0 {
		http.Error(w, "Choose access", 400)
		return
	}
	for _, scope := range scopes {
		if scope != "memory:read" && scope != "memory:write" {
			http.Error(w, "Invalid access", 400)
			return
		}
	}
	target, err := h.Server.Approve(r.Context(), handle, mcpoauth.Approval{Subject: s.AccountID, Binding: brain + "." + strconv.Itoa(epoch), Scopes: scopes})
	if err != nil {
		http.Error(w, "Connection request expired or access was not requested. Start again in your agent.", 400)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

type challengeWriter struct {
	http.ResponseWriter
	value string
}

func (w challengeWriter) WriteHeader(code int) {
	if code == 401 {
		w.Header().Set("WWW-Authenticate", w.value)
	}
	w.ResponseWriter.WriteHeader(code)
}
func (h *Hosted) Challenge(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(challengeWriter{w, fmt.Sprintf(`Bearer resource_metadata=%q, scope="memory:read memory:write"`, h.Origin+"/.well-known/oauth-protected-resource/mcp")}, r)
	})
}

var consentPage = template.Must(template.New("consent").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Connect your agent · Serenity</title><link rel="icon" href="/assets/brand.svg"><link rel="stylesheet" href="/assets/site.css"><link rel="stylesheet" href="/assets/hosted.css"></head><body><header class="site-nav"><a class="brand" href="/"><img src="/assets/brand.svg" alt="">Serenity</a></header><main class="hosted-main login"><p class="eyebrow">Your memory, on your terms</p><h1>Connect your agent.</h1><p><strong>{{.Client}}</strong> is requesting access. This name is supplied by the connecting app; it is not a verified identity.</p><p>Callback destination: <strong>{{.Destinations}}</strong></p><form method="post" action="/oauth/consent"><input type="hidden" name="handle" value="{{.Handle}}"><input type="hidden" name="csrf" value="{{.CSRF}}"><label for="brain">Choose one project memory</label><select id="brain" name="brain">{{range .Brains}}<option value="{{.ID}}">{{.ID}}</option>{{end}}</select><fieldset><legend>Choose access</legend>{{if .Read}}<label><input type="radio" name="access" value="memory:read" checked> Read memories only</label>{{end}}{{if and .Read .Write}}<label><input type="radio" name="access" value="memory:read memory:write"> Read, save and forget memories</label>{{else}}{{if .Write}}<label><input type="radio" name="access" value="memory:write" checked> Save and forget memories</label>{{end}}{{end}}</fieldset><p>Access is limited to this project. You can disconnect the agent from your dashboard.</p><button name="decision" value="approve">Connect agent</button><button class="secondary" name="decision" value="deny">Cancel</button></form></main><footer class="site-footer">Serenity · Private project memory.</footer></body></html>`))

// Active is for evicting closed MCP sessions, never for bearer authentication.
func (h *Hosted) Active(ctx context.Context, id string) (bool, error) {
	g, err := h.Store.Grant(ctx, id)
	if errors.Is(err, mcpoauth.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !g.RevokedAt.IsZero() || !time.Now().Before(g.ExpiresAt) {
		return false, nil
	}
	brain, generation, ok := strings.Cut(g.Binding, ".")
	if !ok {
		return false, nil
	}
	var count int
	err = h.Store.DB.DB().QueryRowContext(ctx, `SELECT count(*) FROM brains b JOIN accounts a ON a.id=b.account_id LEFT JOIN oauth_epochs e ON e.brain_id=b.id WHERE a.id=? AND b.id=? AND a.status='active' AND b.state='ready' AND CAST(COALESCE(e.generation,0) AS TEXT)=?`, g.Subject, brain, generation).Scan(&count)
	return count == 1, err
}
