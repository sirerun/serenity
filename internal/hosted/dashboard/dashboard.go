// Package dashboard serves the hosted signup and connection journey.
package dashboard

import (
	"html/template"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/meter"
	"github.com/sirerun/serenity/internal/hosted/provision"
)

type Dashboard struct {
	Identity  *identity.Service
	Provision *provision.Provisioner
	Issuer    *credential.Issuer
	Meter     *meter.Meter
	Origin    string
	Dev       bool
}
type view struct {
	Title, Message, CSRF, BrainID, Endpoint, Token, Plan string
	SignedIn                                             bool
}

func (d *Dashboard) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", d.login)
	mux.HandleFunc("POST /login", d.requestLink)
	mux.HandleFunc("GET /login/consume", d.consume)
	mux.HandleFunc("GET /{$}", d.home)
	mux.HandleFunc("POST /credentials", d.issue)
	mux.HandleFunc("POST /credentials/rotate", d.rotate)
	mux.HandleFunc("POST /credentials/revoke", d.revoke)
	mux.HandleFunc("POST /logout", d.logout)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, 8192)
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != d.Origin) {
				http.Error(w, "Request origin rejected", 403)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func render(w http.ResponseWriter, status int, v view) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := page.Execute(w, v); err != nil {
		return
	}
}
func (d *Dashboard) login(w http.ResponseWriter, r *http.Request) {
	render(w, 200, view{Title: "Your memory, ready when you are"})
}
func (d *Dashboard) requestLink(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", 400)
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	// Forwarded headers are deliberately ignored; the reverse proxy must provide a trusted boundary.
	err = d.Identity.RequestLink(r.Context(), r.FormValue("email"), ip)
	if err == identity.ErrRateLimited {
		w.Header().Set("Retry-After", "60")
		render(w, 429, view{Title: "Please wait a moment", Message: "Too many sign-in requests. Try again in a minute."})
		return
	}
	if err != nil {
		render(w, 400, view{Title: "Unable to send your link", Message: "Check your email address and try again shortly."})
		return
	}
	render(w, 200, view{Title: "Check your inbox", Message: "Your sign-in link is on its way. It expires in 15 minutes and works once."})
}
func (d *Dashboard) consume(w http.ResponseWriter, r *http.Request) {
	raw, err := d.Identity.Consume(r.Context(), r.URL.Query().Get("token"))
	if err != nil {
		render(w, 400, view{Title: "This link is no longer available", Message: "Request a new sign-in link to continue."})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "serenity_session", Value: raw, Path: "/", HttpOnly: true, Secure: !d.Dev, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (d *Dashboard) session(w http.ResponseWriter, r *http.Request, mutation bool) (identity.Session, bool) {
	cookie, err := r.Cookie("serenity_session")
	if err != nil {
		http.Redirect(w, r, "/login", 303)
		return identity.Session{}, false
	}
	s, err := d.Identity.Session(r.Context(), cookie.Value)
	if err != nil {
		http.Redirect(w, r, "/login", 303)
		return s, false
	}
	if mutation {
		if err = r.ParseForm(); err != nil || !identity.CheckCSRF(s, r.FormValue("csrf")) {
			http.Error(w, "Invalid form token", 403)
			return s, false
		}
	}
	return s, true
}
func (d *Dashboard) home(w http.ResponseWriter, r *http.Request) {
	s, ok := d.session(w, r, false)
	if !ok {
		return
	}
	brain, err := d.Provision.Provision(r.Context(), s.AccountID)
	if err != nil {
		render(w, 503, view{Title: "Preparing your memory", Message: "Your memory is being prepared. Please reload in a moment."})
		return
	}
	ent, err := d.Meter.Entitlement(r.Context(), s.AccountID)
	if err != nil {
		http.Error(w, "Account temporarily unavailable", 503)
		return
	}
	render(w, 200, view{Title: "Your private memory", SignedIn: true, CSRF: s.CSRF, BrainID: brain.ID, Endpoint: d.Origin + "/mcp", Plan: strings.ToUpper(ent.Plan.ID[:1]) + ent.Plan.ID[1:]})
}
func (d *Dashboard) issue(w http.ResponseWriter, r *http.Request) {
	s, ok := d.session(w, r, true)
	if !ok {
		return
	}
	brain, err := d.Provision.Provision(r.Context(), s.AccountID)
	if err != nil {
		http.Error(w, "Memory unavailable", 503)
		return
	}
	raw, err := d.Issuer.Issue(r.Context(), brain.ID, s.AccountID, []string{"memory:read", "memory:write"})
	if err != nil {
		http.Error(w, "Unable to create connection", 503)
		return
	}
	render(w, 200, view{Title: "Connect your agent", SignedIn: true, CSRF: s.CSRF, BrainID: brain.ID, Endpoint: d.Origin + "/mcp", Token: raw, Message: "Copy this token now. It is shown only once. Keep it private."})
}
func (d *Dashboard) rotate(w http.ResponseWriter, r *http.Request) {
	s, ok := d.session(w, r, true)
	if !ok {
		return
	}
	raw, err := d.Issuer.Rotate(r.Context(), s.AccountID, r.FormValue("brain_id"))
	if err != nil {
		http.Error(w, "Connection not found", 404)
		return
	}
	render(w, 200, view{Title: "Connection replaced", SignedIn: true, CSRF: s.CSRF, BrainID: r.FormValue("brain_id"), Endpoint: d.Origin + "/mcp", Token: raw, Message: "The old credentials no longer work. Copy this new token now."})
}
func (d *Dashboard) revoke(w http.ResponseWriter, r *http.Request) {
	s, ok := d.session(w, r, true)
	if !ok {
		return
	}
	if err := d.Issuer.Revoke(r.Context(), s.AccountID, r.FormValue("brain_id")); err != nil {
		http.Error(w, "Connection not found", 404)
		return
	}
	http.Redirect(w, r, "/", 303)
}
func (d *Dashboard) logout(w http.ResponseWriter, r *http.Request) {
	_, ok := d.session(w, r, true)
	if !ok {
		return
	}
	cookie, err := r.Cookie("serenity_session")
	if err != nil {
		http.Error(w, "Session unavailable", 400)
		return
	}
	if err = d.Identity.Logout(r.Context(), cookie.Value); err != nil {
		http.Error(w, "Unable to sign out", 503)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "serenity_session", Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0), HttpOnly: true, Secure: !d.Dev, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/login", 303)
}

var page = template.Must(template.New("page").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Title}} · Serenity</title><style>
:root{color-scheme:light;font-family:system-ui,sans-serif;color:#223b32;background:#f4f3ed}*{box-sizing:border-box}body{margin:0}header,main,footer{max-width:760px;margin:auto;padding:24px}header{border-bottom:1px solid #d5ddd4;display:flex;justify-content:space-between;align-items:center}header a{font-family:Georgia,serif;font-size:24px;color:inherit;text-decoration:none}main{padding-top:64px;min-height:65vh}h1{font-family:Georgia,serif;font-weight:400;font-size:clamp(32px,6vw,48px);letter-spacing:-.035em;line-height:1.1}p,li{line-height:1.6}section{background:white;border:1px solid #d5ddd4;padding:24px;margin:24px 0;border-radius:12px}label{display:block;margin:18px 0 8px}input,textarea{font:inherit;width:100%;padding:12px;border:1px solid #80968a;border-radius:6px;background:#fff}textarea{resize:vertical;min-height:84px;overflow-wrap:anywhere}button{font:inherit;background:#244a3a;color:white;border:0;border-radius:6px;padding:12px 18px;cursor:pointer;margin:12px 0}button.secondary{background:#edf0e8;color:#244a3a}a{color:#244a3a}form.inline{display:inline-block;margin-right:12px}.hint,footer{color:#53655a;font-size:14px}code{overflow-wrap:anywhere}ol{padding-left:22px}:focus-visible{outline:3px solid #9b610e;outline-offset:3px}@media(max-width:450px){main{padding-top:28px}section{padding:16px}}
</style></head><body><header><a href="/">Serenity</a>{{if .SignedIn}}<form method="post" action="/logout"><input type="hidden" name="csrf" value="{{.CSRF}}"><button class="secondary">Sign out</button></form>{{end}}</header><main><h1>{{.Title}}</h1>{{if .Message}}<p role="status">{{.Message}}</p>{{end}}{{if .SignedIn}}<p>Give your agent memory that stays with you across conversations.</p><section><h2>Connect Rakazo</h2>{{if .Plan}}<p>{{.Plan}} plan · Private brain ready</p>{{end}}<label for="endpoint">MCP endpoint</label><input id="endpoint" readonly value="{{.Endpoint}}">{{if .Token}}<label for="token">Bearer token — shown once</label><textarea id="token" readonly spellcheck="false">{{.Token}}</textarea>{{else}}<form method="post" action="/credentials"><input type="hidden" name="csrf" value="{{.CSRF}}"><button>Create connection token</button></form>{{end}}<ol><li>Open Rakazo settings and choose Serenity as your memory provider.</li><li>Paste the MCP endpoint and bearer token above.</li><li>Leave Brain label empty for your default memory.</li><li>Choose “Recall and write” to save memories, then save your settings.</li></ol><p class="hint">A connection is ready to use once you have saved it in your agent. No connection activity has been inferred from this page.</p></section><section><h2>Manage access</h2><p>Replace credentials to invalidate existing connections, or revoke all access to this brain.</p><form class="inline" method="post" action="/credentials/rotate"><input type="hidden" name="csrf" value="{{.CSRF}}"><input type="hidden" name="brain_id" value="{{.BrainID}}"><button class="secondary">Replace credentials</button></form><form class="inline" method="post" action="/credentials/revoke"><input type="hidden" name="csrf" value="{{.CSRF}}"><input type="hidden" name="brain_id" value="{{.BrainID}}"><button class="secondary">Revoke access</button></form></section>{{else}}<p>Sign in with your email. No password or server setup needed.</p><form method="post" action="/login"><label for="email">Email address</label><input id="email" name="email" type="email" autocomplete="email" required maxlength="254"><button>Send sign-in link</button></form>{{end}}</main><footer>Private memory, on your terms. Forget withdraws a fact from current memory; Git history can retain earlier content. Account deletion has a separate 30-day backup retention window.</footer></body></html>`))
