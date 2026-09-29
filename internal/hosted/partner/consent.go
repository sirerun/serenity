package partner

import (
	"context"
	"database/sql"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/store"
)

const consentCSP = "default-src 'none'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; img-src 'self' https://d2ol7oe51mr4n9.cloudfront.net; form-action 'self'; frame-ancestors 'none'; base-uri 'none'"

type linkRequest struct {
	ID, PartnerID, PartnerName, AccountID, ReturnURL, Status string
	ExpiresAt                                                time.Time
}

var errRequestUnavailable = errors.New("link request unavailable")

func (s *Service) loadRequest(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (linkRequest, error) {
	var req linkRequest
	var expires, partnerStatus string
	err := q.QueryRowContext(ctx, `SELECT l.id,l.partner_id,p.display_name,l.account_id,l.return_url,l.status,l.expires_at,p.status FROM link_requests l JOIN partners p ON p.id=l.partner_id WHERE l.id=?`, id).Scan(&req.ID, &req.PartnerID, &req.PartnerName, &req.AccountID, &req.ReturnURL, &req.Status, &expires, &partnerStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return req, errRequestUnavailable
	}
	if err != nil {
		return req, err
	}
	if req.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires); err != nil {
		return req, err
	}
	if req.Status != "pending" || partnerStatus != "active" || !req.ExpiresAt.After(s.now()) {
		return req, errRequestUnavailable
	}
	return req, nil
}

// withFormAction permits the browser to follow the post-decision redirect to
// the partner's return URL; Chromium applies form-action to such redirects.
func withFormAction(w http.ResponseWriter, returnURL string) {
	csp := consentCSP
	if u, err := url.Parse(returnURL); err == nil && u.Scheme != "" && u.Host != "" && !strings.ContainsAny(u.Host, ";'\" ") {
		csp = strings.Replace(csp, "form-action 'self'", "form-action 'self' "+u.Scheme+"://"+u.Host, 1)
	}
	w.Header().Set("Content-Security-Policy", csp)
}

func (s *Service) session(r *http.Request) (identity.Session, error) {
	c, err := r.Cookie("serenity_session")
	if err != nil {
		return identity.Session{}, err
	}
	return s.Identity.Session(r.Context(), c.Value)
}

func renderConsent(w http.ResponseWriter, status int, v consentView) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "same-origin")
	if w.Header().Get("Content-Security-Policy") == "" {
		w.Header().Set("Content-Security-Policy", consentCSP)
	}
	w.WriteHeader(status)
	_ = consentTemplate.Execute(w, v)
}

type consentView struct {
	Title, Message, Partner, Request, CSRF string
	Ask, WrongAccount                      bool
}

func (s *Service) consentPage(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("request")
	if id == "" || len(id) > 64 {
		renderConsent(w, http.StatusBadRequest, consentView{Title: "This request is no longer available", Message: "Return to the app and start linking again."})
		return
	}
	req, err := s.loadRequest(r.Context(), s.Store.DB(), id)
	if err != nil {
		renderConsent(w, http.StatusBadRequest, consentView{Title: "This request is no longer available", Message: "Return to the app and start linking again."})
		return
	}
	session, err := s.session(r)
	if err != nil {
		// Resume here after the ordinary magic-link sign-in. The cookie is an
		// opaque handle and never a redirect destination.
		http.SetCookie(w, &http.Cookie{Name: ResumeCookie, Value: id, Path: "/", HttpOnly: true, Secure: !s.Dev, SameSite: http.SameSiteLaxMode, MaxAge: int(LinkRequestTTL / time.Second)})
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if session.AccountID != req.AccountID {
		http.SetCookie(w, &http.Cookie{Name: ResumeCookie, Value: id, Path: "/", HttpOnly: true, Secure: !s.Dev, SameSite: http.SameSiteLaxMode, MaxAge: int(LinkRequestTTL / time.Second)})
		renderConsent(w, http.StatusForbidden, consentView{Title: "Sign in with the right account", Message: "This request is for a different Serenity account. Sign out, then sign in with the email address you use in " + req.PartnerName + ".", CSRF: session.CSRF, WrongAccount: true})
		return
	}
	withFormAction(w, req.ReturnURL)
	renderConsent(w, http.StatusOK, consentView{Title: "Allow " + req.PartnerName + " to read and write your Serenity memory?", Partner: req.PartnerName, Request: req.ID, CSRF: session.CSRF, Ask: true})
}

func (s *Service) consentDecision(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	session, err := s.session(r)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if r.Header.Get("Origin") != s.Origin || r.Header.Get("Sec-Fetch-Site") == "cross-site" || r.ParseForm() != nil || !identity.CheckCSRF(session, r.PostForm.Get("csrf")) {
		http.Error(w, "Invalid consent form", http.StatusForbidden)
		return
	}
	decision := r.PostForm.Get("decision")
	if decision != "allow" && decision != "deny" {
		http.Error(w, "Invalid consent decision", http.StatusBadRequest)
		return
	}
	id := r.PostForm.Get("request")
	now := s.now()
	code := randomToken()
	var req linkRequest
	err = s.Store.Transaction(r.Context(), func(tx *sql.Tx) error {
		var e error
		req, e = s.loadRequest(r.Context(), tx, id)
		if e != nil {
			return e
		}
		if req.AccountID != session.AccountID {
			return errRequestUnavailable
		}
		var result sql.Result
		if decision == "allow" {
			result, e = tx.ExecContext(r.Context(), `UPDATE link_requests SET status='approved',code_hash=?,decided_at=? WHERE id=? AND status='pending'`, store.Hash(code), store.Stamp(now), id)
		} else {
			result, e = tx.ExecContext(r.Context(), `UPDATE link_requests SET status='denied',decided_at=? WHERE id=? AND status='pending'`, store.Stamp(now), id)
		}
		if e != nil {
			return e
		}
		if n, e := result.RowsAffected(); e != nil || n != 1 {
			return errors.Join(errRequestUnavailable, e)
		}
		_, e = tx.ExecContext(r.Context(), `INSERT INTO audit_log(account_id,actor,action,created_at,detail) VALUES(?,'account',?,?,?)`, session.AccountID, "partner.consent_"+decision, store.Stamp(now), req.PartnerID)
		return e
	})
	if errors.Is(err, errRequestUnavailable) {
		renderConsent(w, http.StatusBadRequest, consentView{Title: "This request is no longer available", Message: "Return to the app and start linking again."})
		return
	}
	if err != nil {
		http.Error(w, "Could not record your decision. Try again.", http.StatusServiceUnavailable)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: ResumeCookie, Value: "", Path: "/", HttpOnly: true, Secure: !s.Dev, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	target, err := url.Parse(req.ReturnURL)
	if err != nil {
		http.Error(w, "Invalid return destination", http.StatusBadRequest)
		return
	}
	query := target.Query()
	if decision == "allow" {
		query.Set("code", code)
	} else {
		query.Set("error", "denied")
	}
	query.Set("state", req.ID)
	target.RawQuery = query.Encode()
	withFormAction(w, req.ReturnURL)
	w.Header().Set("Location", target.String())
	w.WriteHeader(http.StatusSeeOther)
}

var consentTemplate = template.Must(template.New("partner-consent").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Link an app · Serenity</title><link rel="icon" href="/assets/brand.svg"><link rel="stylesheet" href="/assets/site.css?v=721000741e"><link rel="stylesheet" href="/assets/hosted.css?v=7635e0d0e2"></head><body><header class="site-nav"><a class="brand" href="/"><img src="/assets/brand.svg" alt="">Serenity</a></header><main class="hosted-main login"><p class="eyebrow">Your memory, on your terms</p><h1>{{.Title}}</h1>{{if .Message}}<p>{{.Message}}</p>{{end}}{{if .Ask}}<p><strong>{{.Partner}}</strong> will read and save memories in your default Serenity memory, the same one your other agents use. Your account and memories stay yours. You can disconnect {{.Partner}} anytime from your dashboard.</p><form method="post" action="/partner/consent"><input type="hidden" name="request" value="{{.Request}}"><input type="hidden" name="csrf" value="{{.CSRF}}"><button name="decision" value="allow">Allow</button><button class="secondary" name="decision" value="deny">Deny</button></form>{{end}}{{if .WrongAccount}}<form method="post" action="/logout"><input type="hidden" name="csrf" value="{{.CSRF}}"><button class="secondary">Sign out</button></form>{{end}}</main><footer class="site-footer">Serenity · Private project memory.</footer></body></html>`))
