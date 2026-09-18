// Package dashboard serves the hosted signup and connection journey.
package dashboard

import (
	"encoding/json"
	"html/template"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/hosted/billing"
	"github.com/sirerun/serenity/internal/hosted/gateway"

	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/meter"
	"github.com/sirerun/serenity/internal/hosted/provision"
	"github.com/sirerun/serenity/internal/hosted/store"
)

type Dashboard struct {
	BillingService *billing.Service
	Gateway        *gateway.Gateway
	Identity       *identity.Service
	Provision      *provision.Provisioner
	Issuer         *credential.Issuer
	Meter          *meter.Meter
	Origin         string
	Dev            bool
	Billing        bool
}
type usageRow struct {
	Name        string
	Used, Limit int64
}
type view struct {
	Usage                                                []usageRow
	Reset                                                string
	OperationKey                                         string
	Connected, MemorySaved                               bool
	Brains                                               []store.Brain
	CanAddBrain                                          bool
	Billing                                              bool
	Title, Message, CSRF, BrainID, Endpoint, Token, Plan string
	SignedIn                                             bool
}

func (d *Dashboard) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /assets/app.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(`document.addEventListener("click",async(event)=>{const button=event.target.closest("[data-copy]");if(!button)return;const field=document.getElementById(button.dataset.copy);try{await navigator.clipboard.writeText(field.value);button.textContent="Copied";}catch{field.focus();field.select();button.textContent="Select and copy";}});`))
	})
	mux.HandleFunc("GET /login", d.login)
	mux.HandleFunc("POST /login", d.requestLink)
	mux.HandleFunc("GET /login/consume", d.consume)
	mux.HandleFunc("GET /{$}", d.home)
	mux.HandleFunc("GET /billing", d.home)
	mux.HandleFunc("POST /credentials", d.issue)
	mux.HandleFunc("POST /brains", d.addBrain)
	mux.HandleFunc("POST /memories", d.remember)
	mux.HandleFunc("GET /brains/{id}/export", d.export)
	mux.HandleFunc("POST /brains/{id}/delete", d.deleteBrain)
	mux.HandleFunc("POST /account/delete", d.deleteAccount)
	mux.HandleFunc("POST /credentials/rotate", d.rotate)
	mux.HandleFunc("POST /credentials/revoke", d.revoke)
	mux.HandleFunc("POST /logout", d.logout)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, 8192)
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != d.Origin) {
				http.Error(w, "Request origin rejected", http.StatusForbidden)
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
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	// Caddy overwrites this header; only a loopback peer may supply it.
	if peer := net.ParseIP(ip); peer != nil && peer.IsLoopback() {
		if forwarded := net.ParseIP(r.Header.Get("X-Serenity-Client-IP")); forwarded != nil {
			ip = forwarded.String()
		}
	}
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
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return identity.Session{}, false
	}
	s, err := d.Identity.Session(r.Context(), cookie.Value)
	if err != nil && mutation && r.Method == http.MethodPost && r.URL.Path == "/account/delete" {
		s, err = d.Identity.DeletionSession(r.Context(), cookie.Value)
	}
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return s, false
	}
	if mutation {
		if err = r.ParseForm(); err != nil || !identity.CheckCSRF(s, r.FormValue("csrf")) {
			http.Error(w, "Invalid form token", http.StatusForbidden)
			return s, false
		}
	}
	http.SetCookie(w, &http.Cookie{Name: "serenity_session", Value: cookie.Value, Path: "/", HttpOnly: true, Secure: !d.Dev, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600})
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
		http.Error(w, "Account temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	brains, err := d.Issuer.Store.Brains(r.Context(), s.AccountID)
	if err != nil {
		http.Error(w, "Memory unavailable", http.StatusServiceUnavailable)
		return
	}
	var connected, saved int
	if err = d.Issuer.Store.DB().QueryRowContext(r.Context(), `SELECT count(*) FROM audit_log WHERE account_id=? AND action='connected'`, s.AccountID).Scan(&connected); err != nil {
		http.Error(w, "Status unavailable", http.StatusServiceUnavailable)
		return
	}
	if err = d.Issuer.Store.DB().QueryRowContext(r.Context(), `SELECT count(*) FROM audit_log WHERE account_id=? AND action='memory_saved'`, s.AccountID).Scan(&saved); err != nil {
		http.Error(w, "Status unavailable", http.StatusServiceUnavailable)
		return
	}
	usage := []usageRow{{Name: "Writes", Limit: ent.Plan.Writes}, {Name: "Recalls", Limit: ent.Plan.Recalls}, {Name: "Input tokens", Limit: ent.Plan.InputTokens}}
	for i, metric := range []string{"writes", "recalls", "input_tokens"} {
		if err = d.Issuer.Store.DB().QueryRowContext(r.Context(), `SELECT COALESCE((SELECT committed FROM usage_windows WHERE account_id=? AND window_key=? AND metric=?),0)`, s.AccountID, ent.Window, metric).Scan(&usage[i].Used); err != nil {
			http.Error(w, "Usage temporarily unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	render(w, 200, view{Usage: usage, Reset: ent.ResetAt.UTC().Format("2 Jan 2006 15:04 UTC"), OperationKey: store.ID(), Connected: connected > 0, MemorySaved: saved > 0, Brains: brains, CanAddBrain: int64(len(brains)) < ent.Plan.Brains, Title: "Your private memory", SignedIn: true, Billing: d.Billing, CSRF: s.CSRF, BrainID: brain.ID, Endpoint: d.Origin + "/mcp", Plan: strings.ToUpper(ent.Plan.ID[:1]) + ent.Plan.ID[1:]})
}
func (d *Dashboard) issue(w http.ResponseWriter, r *http.Request) {
	s, ok := d.session(w, r, true)
	if !ok {
		return
	}
	brain, err := d.Provision.Provision(r.Context(), s.AccountID)
	if err != nil {
		http.Error(w, "Memory unavailable", http.StatusServiceUnavailable)
		return
	}
	if selected := r.FormValue("brain_id"); selected != "" {
		brain, err = d.Issuer.Store.BrainByID(r.Context(), s.AccountID, selected)
		if err != nil {
			http.Error(w, "Memory not found", http.StatusNotFound)
			return
		}
	}
	raw, err := d.Issuer.Issue(r.Context(), brain.ID, s.AccountID, []string{"memory:read", "memory:write"})
	if err != nil {
		http.Error(w, "Unable to create connection", http.StatusServiceUnavailable)
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
		http.Error(w, "Connection not found", http.StatusNotFound)
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
		http.Error(w, "Connection not found", http.StatusNotFound)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
func (d *Dashboard) logout(w http.ResponseWriter, r *http.Request) {
	_, ok := d.session(w, r, true)
	if !ok {
		return
	}
	cookie, err := r.Cookie("serenity_session")
	if err != nil {
		http.Error(w, "Session unavailable", http.StatusBadRequest)
		return
	}
	if err = d.Identity.Logout(r.Context(), cookie.Value); err != nil {
		http.Error(w, "Unable to sign out", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "serenity_session", Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0), HttpOnly: true, Secure: !d.Dev, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

var page = template.Must(template.New("page").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{{.Title}} · Serenity</title><style>
:root{color-scheme:light;font-family:system-ui,sans-serif;color:#223b32;background:#f4f3ed}*{box-sizing:border-box}body{margin:0}header,main,footer{max-width:760px;margin:auto;padding:24px}header{border-bottom:1px solid #d5ddd4;display:flex;justify-content:space-between;align-items:center}header a{font-family:Georgia,serif;font-size:24px;color:inherit;text-decoration:none}main{padding-top:64px;min-height:65vh}h1{font-family:Georgia,serif;font-weight:400;font-size:clamp(32px,6vw,48px);letter-spacing:-.035em;line-height:1.1}p,li{line-height:1.6}section{background:white;border:1px solid #d5ddd4;padding:24px;margin:24px 0;border-radius:12px}label{display:block;margin:18px 0 8px}input,textarea{font:inherit;width:100%;padding:12px;border:1px solid #80968a;border-radius:6px;background:#fff}textarea{resize:vertical;min-height:84px;overflow-wrap:anywhere}button{font:inherit;background:#244a3a;color:white;border:0;border-radius:6px;padding:12px 18px;cursor:pointer;margin:12px 0}button.secondary{background:#edf0e8;color:#244a3a}a{color:#244a3a}form.inline{display:inline-block;margin-right:12px}.hint,footer{color:#53655a;font-size:14px}code{overflow-wrap:anywhere}ol{padding-left:22px}:focus-visible{outline:3px solid #9b610e;outline-offset:3px}@media(max-width:450px){main{padding-top:28px}section{padding:16px}}
</style><script src="/assets/app.js" defer></script></head><body><header><a href="/">Serenity</a>{{if .SignedIn}}<form method="post" action="/logout"><input type="hidden" name="csrf" value="{{.CSRF}}"><button class="secondary">Sign out</button></form>{{end}}</header><main><h1>{{.Title}}</h1>{{if .Message}}<p role="status">{{.Message}}</p>{{end}}{{if .SignedIn}}<p>Give your agent memory that stays with you across conversations.</p><section><h2>Connect Rakazo</h2>{{if .Plan}}<p>{{.Plan}} plan · Private brain ready</p>{{end}}<label for="endpoint">MCP endpoint</label><input id="endpoint" readonly value="{{.Endpoint}}"><button type="button" class="secondary" data-copy="endpoint">Copy endpoint</button>{{if .Token}}<label for="token">Bearer token — shown once</label><textarea id="token" readonly spellcheck="false">{{.Token}}</textarea><button type="button" class="secondary" data-copy="token">Copy token</button>{{else}}<form method="post" action="/credentials"><input type="hidden" name="csrf" value="{{.CSRF}}"><button>Create connection token</button></form>{{end}}<ol><li>Open Rakazo settings and choose Serenity as your memory provider.</li><li>Paste the MCP endpoint and bearer token above.</li><li>Leave Brain label empty for your default memory.</li><li>Choose “Recall and write” to save memories, then save your settings.</li></ol><p class="hint">{{if .Connected}}Connected: your agent has discovered its memory tools.{{else}}Waiting for your agent to connect.{{end}} {{if .MemorySaved}}First memory saved.{{end}}</p></section>{{if .OperationKey}}<section><h2>Save your first memory</h2><form method="post" action="/memories"><input type="hidden" name="csrf" value="{{.CSRF}}"><input type="hidden" name="brain_id" value="{{.BrainID}}"><input type="hidden" name="operation_key" value="{{.OperationKey}}"><label for="fact">What should your agent remember?</label><textarea id="fact" name="fact" required maxlength="4096" placeholder="I prefer concise answers with source links."></textarea><button>Save memory</button></form><p class="hint">Your text is sent to the configured embedding provider for semantic retrieval. It is not shared with other accounts.</p></section>{{end}}{{if .Usage}}<section><h2>Usage this period</h2><table><thead><tr><th>Allowance</th><th>Used</th><th>Limit</th></tr></thead><tbody>{{range .Usage}}<tr><td>{{.Name}}</td><td>{{.Used}}</td><td>{{.Limit}}</td></tr>{{end}}</tbody></table><p>Resets {{.Reset}}. Limits are shared across your brains.</p></section>{{end}}{{if .Billing}}<section><h2>Your plan</h2><p>Free: 1 brain, 1,000 memories, 500 new writes and 10,000 recalls per month. No card required.</p><form class="inline" method="post" action="/billing/checkout"><input type="hidden" name="csrf" value="{{.CSRF}}"><input type="hidden" name="plan" value="builder"><button>Builder · $19/month</button></form><form class="inline" method="post" action="/billing/checkout"><input type="hidden" name="csrf" value="{{.CSRF}}"><input type="hidden" name="plan" value="scale"><button>Scale · $49/month</button></form><form method="post" action="/billing/portal"><input type="hidden" name="csrf" value="{{.CSRF}}"><button class="secondary">Manage billing</button></form></section>{{end}}{{if .Brains}}<section><h2>Your brains</h2>{{range .Brains}}<p><code>{{.ID}}</code> · {{.State}}</p><form method="post" action="/credentials"><input type="hidden" name="csrf" value="{{$.CSRF}}"><input type="hidden" name="brain_id" value="{{.ID}}"><button class="secondary">Connect this brain</button></form><a href="/brains/{{.ID}}/export">Export memory</a><form method="post" action="/brains/{{.ID}}/delete"><input type="hidden" name="csrf" value="{{$.CSRF}}"><label><input type="checkbox" name="confirm" value="delete" required> Delete this brain and revoke its connections</label><button class="secondary">Delete brain</button></form>{{end}}{{if .CanAddBrain}}<form method="post" action="/brains"><input type="hidden" name="csrf" value="{{.CSRF}}"><button>Add private brain</button></form>{{end}}</section>{{end}}<section><h2>Manage access</h2><p>Replace credentials to invalidate existing connections, or revoke all access to this brain.</p><form class="inline" method="post" action="/credentials/rotate"><input type="hidden" name="csrf" value="{{.CSRF}}"><input type="hidden" name="brain_id" value="{{.BrainID}}"><button class="secondary">Replace credentials</button></form><form class="inline" method="post" action="/credentials/revoke"><input type="hidden" name="csrf" value="{{.CSRF}}"><input type="hidden" name="brain_id" value="{{.BrainID}}"><button class="secondary">Revoke access</button></form></section>{{else}}<p>Sign in with your email. No password or server setup needed.</p><form method="post" action="/login"><label for="email">Email address</label><input id="email" name="email" type="email" autocomplete="email" required maxlength="254"><button>Send sign-in link</button></form>{{end}}{{if .SignedIn}}<section><h2>Delete account</h2><p>This cancels your subscription, revokes connections and deletes your current memory. Backups have a separate retention window.</p><form method="post" action="/account/delete"><input type="hidden" name="csrf" value="{{.CSRF}}"><label for="delete-confirm">Type DELETE to confirm</label><input id="delete-confirm" name="confirm" required pattern="DELETE" autocomplete="off"><button class="secondary">Delete my account</button></form></section>{{end}}</main><footer>Private memory, on your terms. Forget withdraws a fact from current memory; Git history can retain earlier content. Account deletion has a separate backup retention window.</footer></body></html>`))

func (d *Dashboard) addBrain(w http.ResponseWriter, r *http.Request) {
	s, ok := d.session(w, r, true)
	if !ok {
		return
	}
	ent, err := d.Meter.Entitlement(r.Context(), s.AccountID)
	if err != nil {
		http.Error(w, "Account unavailable", http.StatusServiceUnavailable)
		return
	}
	if _, err = d.Provision.Additional(r.Context(), s.AccountID, ent.Plan.Brains); err != nil {
		http.Error(w, "Unable to add a brain. Check your plan limit.", http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (d *Dashboard) remember(w http.ResponseWriter, r *http.Request) {
	s, ok := d.session(w, r, true)
	if !ok {
		return
	}
	args, err := json.Marshal(map[string]string{"fact": r.FormValue("fact"), "provenance": "Saved by account owner in Serenity dashboard", "operation_key": r.FormValue("operation_key")})
	if err != nil {
		http.Error(w, "Invalid memory", http.StatusBadRequest)
		return
	}
	result, err := d.Gateway.CallForAccount(r.Context(), s.AccountID, r.FormValue("brain_id"), "remember", args)
	if err != nil || result.IsError {
		render(w, 409, view{Title: "Memory was not saved", Message: "Check your memory size and plan limits, then try again."})
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (d *Dashboard) export(w http.ResponseWriter, r *http.Request) {
	s, ok := d.session(w, r, false)
	if !ok {
		return
	}
	if _, err := d.Issuer.Store.BrainByID(r.Context(), s.AccountID, r.PathValue("id")); err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="serenity-memory.zip"`)
	if err := d.Gateway.Export(r.Context(), s.AccountID, r.PathValue("id"), w); err != nil {
		return
	}
}
func (d *Dashboard) deleteBrain(w http.ResponseWriter, r *http.Request) {
	s, ok := d.session(w, r, true)
	if !ok {
		return
	}
	if r.FormValue("confirm") != "delete" {
		http.Error(w, "Confirm deletion", http.StatusBadRequest)
		return
	}
	if err := d.Gateway.DeleteBrain(r.Context(), s.AccountID, r.PathValue("id"), d.Provision.BrainsRoot); err != nil {
		http.Error(w, "Unable to delete brain", http.StatusConflict)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (d *Dashboard) deleteAccount(w http.ResponseWriter, r *http.Request) {
	s, ok := d.session(w, r, true)
	if !ok {
		return
	}
	if r.FormValue("confirm") != "DELETE" {
		http.Error(w, "Confirm account deletion", http.StatusBadRequest)
		return
	}
	if d.BillingService != nil {
		if err := d.BillingService.CancelAccount(r.Context(), s.AccountID); err != nil {
			http.Error(w, "Subscription cancellation is pending; please retry.", http.StatusServiceUnavailable)
			return
		}
	}
	if err := d.Gateway.DeleteAccount(r.Context(), s.AccountID, d.Provision.BrainsRoot); err != nil {
		http.Error(w, "Deletion could not finish; please retry.", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "serenity_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: !d.Dev, SameSite: http.SameSiteLaxMode})
	render(w, http.StatusOK, view{Title: "Your account has been deleted", Message: "Your connections are revoked and current memory is removed. Backup retention is separate."})
}
