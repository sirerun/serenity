// Package billing reconciles hosted entitlements from Stripe, never return URLs.
package billing

import (
	"errors"
	"github.com/sirerun/serenity/internal/hosted/identity"
	"io"
	"net/http"
	"strconv"
	"time"
)

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == "/billing/webhook" {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "Invalid webhook", http.StatusBadRequest)
			return
		}
		if !VerifySignature(body, r.Header.Get("Stripe-Signature"), s.Config.WebhookSecret, time.Now()) {
			http.Error(w, "Invalid signature", http.StatusBadRequest)
			return
		}
		if err = s.Webhook(r.Context(), body, r.Header.Get("Stripe-Signature")); err != nil {
			http.Error(w, "Webhook processing unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(200)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cookie, err := r.Cookie("serenity_session")
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	session, err := s.Identity.Session(r.Context(), cookie.Value)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if err = r.ParseForm(); err != nil || !identity.CheckCSRF(session, r.FormValue("csrf")) || r.Header.Get("Sec-Fetch-Site") == "cross-site" || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != s.Config.Origin) {
		http.Error(w, "Invalid form", http.StatusForbidden)
		return
	}
	var target string
	switch r.URL.Path {
	case "/billing/checkout":
		target, err = s.Checkout(r.Context(), session.AccountID, r.FormValue("plan"))
	case "/billing/portal":
		target, err = s.Portal(r.Context(), session.AccountID)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		var limitErr *CheckoutRateLimitError
		if errors.As(err, &limitErr) {
			w.Header().Set("Retry-After", strconv.FormatInt(retryAfterSeconds(limitErr.RetryAfter()), 10))
			http.Error(w, "Checkout rate limit exceeded. Please try again shortly.", http.StatusTooManyRequests)
			return
		}
		http.Error(w, "Billing is temporarily unavailable. Your existing memory remains accessible.", http.StatusServiceUnavailable)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
