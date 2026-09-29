// Package dashboard serves the hosted signup and connection journey.
package dashboard

import (
	_ "embed"
	"encoding/json"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/hosted/billing"
	"github.com/sirerun/serenity/internal/hosted/gateway"

	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/meter"
	"github.com/sirerun/serenity/internal/hosted/partner"
	"github.com/sirerun/serenity/internal/hosted/provision"
	"github.com/sirerun/serenity/internal/hosted/store"
	website "github.com/sirerun/serenity/site"
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

func (u usageRow) Remaining() int64 {
	if u.Used >= u.Limit {
		return 0
	}
	return u.Limit - u.Used
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
	Page                                                 string
	SignedIn                                             bool
	PartnerLinks                                         []partnerLink
}

// partnerLink is a first-party app linked through the partner API (ADR 023).
type partnerLink struct{ ID, Name, Since string }

func (d *Dashboard) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /assets/app.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(`document.addEventListener("click",async(event)=>{const button=event.target.closest("[data-copy]");if(!button)return;const field=document.getElementById(button.dataset.copy);try{await navigator.clipboard.writeText(field.value);button.textContent="Copied";}catch{field.focus();field.select();button.textContent="Select and copy";}});`))
	})
	mux.HandleFunc("GET /login", d.login)
	mux.HandleFunc("POST /login", d.requestLink)
	mux.HandleFunc("GET /login/consume", d.consume)
	mux.Handle("GET /", website.Handler())
	mux.HandleFunc("GET /dashboard", d.home)
	for _, path := range []string{"connections", "memories", "usage", "settings"} {
		mux.HandleFunc("GET /dashboard/"+path, d.home)
	}
	mux.HandleFunc("GET /billing", d.home)
	mux.HandleFunc("POST /credentials", d.issue)
	mux.HandleFunc("POST /brains", d.addBrain)
	mux.HandleFunc("POST /memories", d.remember)
	mux.HandleFunc("GET /brains/{id}/export", d.export)
	mux.HandleFunc("POST /brains/{id}/delete", d.deleteBrain)
	mux.HandleFunc("POST /account/delete", d.deleteAccount)
	mux.HandleFunc("POST /credentials/rotate", d.rotate)
	mux.HandleFunc("POST /credentials/revoke", d.revoke)
	mux.HandleFunc("POST /connections/partners/disconnect", d.disconnectPartner)
	mux.HandleFunc("POST /logout", d.logout)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handler, pattern := mux.Handler(r); pattern == "GET /" {
			handler.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; img-src 'self' https://d2ol7oe51mr4n9.cloudfront.net; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
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
	if resume, e := r.Cookie("serenity_oauth_resume"); e == nil {
		http.SetCookie(w, &http.Cookie{Name: "serenity_oauth_resume", Value: "", Path: "/", HttpOnly: true, Secure: !d.Dev, SameSite: http.SameSiteLaxMode, MaxAge: -1})
		// A resume cookie is only an opaque handle, never a redirect destination.
		if len(resume.Value) > 0 && len(resume.Value) <= 256 {
			http.Redirect(w, r, "/oauth/authorize?request="+url.QueryEscape(resume.Value), http.StatusSeeOther)
			return
		}
	}
	if resume, e := r.Cookie(partner.ResumeCookie); e == nil {
		http.SetCookie(w, &http.Cookie{Name: partner.ResumeCookie, Value: "", Path: "/", HttpOnly: true, Secure: !d.Dev, SameSite: http.SameSiteLaxMode, MaxAge: -1})
		// The partner resume cookie is likewise only an opaque request handle.
		if len(resume.Value) > 0 && len(resume.Value) <= 64 {
			http.Redirect(w, r, "/partner/consent?request="+url.QueryEscape(resume.Value), http.StatusSeeOther)
			return
		}
	}
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
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
	inventory, err := d.Gateway.Inventory(r.Context(), s.AccountID, d.Provision.BrainsRoot)
	if err != nil {
		http.Error(w, "Usage temporarily unavailable", http.StatusServiceUnavailable)
		return
	}
	usage = append(usage, usageRow{Name: "Brains", Used: inventory.Brains, Limit: ent.Plan.Brains}, usageRow{Name: "Live memories", Used: inventory.Memories, Limit: ent.Plan.Memories}, usageRow{Name: "Storage (bytes, including history)", Used: inventory.StorageBytes, Limit: ent.Plan.StorageBytes})
	section := strings.TrimPrefix(r.URL.Path, "/dashboard/")
	if r.URL.Path == "/dashboard" {
		section = "overview"
	}
	if r.URL.Path == "/billing" {
		section = "usage"
	}
	titles := map[string]string{"overview": "Your memory, at a glance", "connections": "Connect your agents", "memories": "Your project memories", "usage": "Usage and plan", "settings": "Account settings"}
	message := ""
	if section == "memories" && r.URL.Query().Get("saved") == "1" {
		message = "Memory saved. Your connected agents can recall it now."
	}
	if section == "settings" && r.URL.Query().Get("revoked") == "1" {
		message = "Access revoked. This project's previous connections no longer work."
	}
	var links []partnerLink
	if section == "connections" {
		if r.URL.Query().Get("disconnected") == "1" {
			message = "App disconnected. Your account and memories are unchanged."
		}
		links, err = d.partnerLinks(r, s.AccountID)
		if err != nil {
			http.Error(w, "Connections unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	render(w, 200, view{PartnerLinks: links, Message: message, Page: section, Usage: usage, Reset: ent.ResetAt.UTC().Format("2 Jan 2006 15:04 UTC"), OperationKey: store.ID(), Connected: connected > 0, MemorySaved: saved > 0, Brains: brains, CanAddBrain: int64(len(brains)) < ent.Plan.Brains, Title: titles[section], SignedIn: true, Billing: d.Billing, CSRF: s.CSRF, BrainID: brain.ID, Endpoint: d.Origin + "/mcp", Plan: strings.ToUpper(ent.Plan.ID[:1]) + ent.Plan.ID[1:]})
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
	render(w, 200, view{Page: "connections", Title: "Your connection token", SignedIn: true, CSRF: s.CSRF, BrainID: brain.ID, Endpoint: d.Origin + "/mcp", Token: raw, Message: "Copy this token now. It is shown only once. Keep it private."})
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
	render(w, 200, view{Page: "connections", Title: "Connection replaced", SignedIn: true, CSRF: s.CSRF, BrainID: r.FormValue("brain_id"), Endpoint: d.Origin + "/mcp", Token: raw, Message: "The old credentials no longer work. Copy this new token now."})
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
	http.Redirect(w, r, "/dashboard/settings?revoked=1", http.StatusSeeOther)
}
func (d *Dashboard) partnerLinks(r *http.Request, accountID string) ([]partnerLink, error) {
	rows, err := d.Issuer.Store.DB().QueryContext(r.Context(), `SELECT l.partner_id,p.display_name,l.created_at FROM partner_links l JOIN partners p ON p.id=l.partner_id WHERE l.account_id=? AND l.status='active' ORDER BY l.created_at`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []partnerLink
	for rows.Next() {
		var link partnerLink
		var since string
		if err = rows.Scan(&link.ID, &link.Name, &since); err != nil {
			return nil, err
		}
		if t, e := time.Parse(time.RFC3339Nano, since); e == nil {
			link.Since = t.UTC().Format("2 Jan 2006")
		}
		out = append(out, link)
	}
	return out, rows.Err()
}

// disconnectPartner lets the account owner end a partner link. The account
// and its memories are untouched; only the partner's keys and link end.
func (d *Dashboard) disconnectPartner(w http.ResponseWriter, r *http.Request) {
	s, ok := d.session(w, r, true)
	if !ok {
		return
	}
	id := r.FormValue("partner_id")
	if !partner.ValidID(id) {
		http.Error(w, "Connection not found", http.StatusNotFound)
		return
	}
	err := partner.Unlink(r.Context(), d.Issuer.Store, id, s.AccountID, time.Now())
	if err != nil {
		http.Error(w, "Connection not found", http.StatusNotFound)
		return
	}
	now := store.Stamp(time.Now())
	_, _ = d.Issuer.Store.DB().ExecContext(r.Context(), `INSERT INTO audit_log(account_id,actor,action,created_at,detail) VALUES(?,'account','partner.disconnect',?,?)`, s.AccountID, now, id)
	http.Redirect(w, r, "/dashboard/connections?disconnected=1", http.StatusSeeOther)
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

//go:embed page.html
var pageHTML string

var page = template.Must(template.New("page").Funcs(template.FuncMap{
	"count": func(n int64) string {
		value := strconv.FormatInt(n, 10)
		for i := len(value) - 3; i > 0 && value[i-1] != '-'; i -= 3 {
			value = value[:i] + "," + value[i:]
		}
		return value
	},
	"project": func(id string) string {
		if len(id) > 8 {
			id = id[len(id)-8:]
		}
		return "Memory " + id
	},
}).Parse(pageHTML))

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
	http.Redirect(w, r, "/dashboard/memories", http.StatusSeeOther)
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
	http.Redirect(w, r, "/dashboard/memories?saved=1", http.StatusSeeOther)
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
	http.Redirect(w, r, "/dashboard/memories", http.StatusSeeOther)
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
