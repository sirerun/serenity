// Package partner implements the server-to-server surface that first-party
// apps (such as Blink) use to create, link, and meter Serenity accounts
// (ADR 023). Every call authenticates with a partner secret, is rate limited
// per partner, and writes an audit_log row.
package partner

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/gateway"
	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/meter"
	"github.com/sirerun/serenity/internal/hosted/provision"
	"github.com/sirerun/serenity/internal/hosted/store"
)

const (
	// RateLimit is the per-partner request allowance per minute.
	RateLimit = 600
	// LinkRequestTTL bounds both consent and code redemption.
	LinkRequestTTL = 15 * time.Minute
	// DefaultAccountCap is the default PartnerAccountCap.
	DefaultAccountCap = 100000
	// ResumeCookie carries a pending consent request across magic-link login.
	ResumeCookie = "serenity_partner_resume"
)

// Service serves /partner/v1/* and the browser consent page.
type Service struct {
	Store     *store.Store
	Identity  *identity.Service
	Provision *provision.Provisioner
	Issuer    *credential.Issuer
	Meter     *meter.Meter
	Gateway   *gateway.Gateway
	Origin    string
	Dev       bool
	// AccountCap is PartnerAccountCap: the ceiling on active accounts created
	// by partners, separate from the public registration cap.
	AccountCap int
	Clock      func() time.Time
	limiter    limiter
}

// Partner is an authenticated partner record.
type Partner struct {
	ID, DisplayName, RedirectPrefix string
}

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

// Handler routes the partner API and the consent page.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /partner/v1/accounts:ensure", s.api("accounts.ensure", s.ensure))
	mux.HandleFunc("POST /partner/v1/link-requests", s.api("link_requests.create", s.createLinkRequest))
	mux.HandleFunc("POST /partner/v1/link-requests/{action}", s.api("link_requests.redeem", s.redeem))
	mux.HandleFunc("POST /partner/v1/credentials", s.api("credentials.issue", s.issue))
	mux.HandleFunc("DELETE /partner/v1/credentials/{id}", s.api("credentials.revoke", s.revokeKey))
	mux.HandleFunc("DELETE /partner/v1/links/{account}", s.api("links.delete", s.unlink))
	mux.HandleFunc("PUT /partner/v1/entitlements/{account}", s.api("entitlements.put", s.entitlement))
	mux.HandleFunc("GET /partner/v1/accounts/{account}/usage", s.api("usage.get", s.usage))
	mux.HandleFunc("GET /partner/consent", s.consentPage)
	mux.HandleFunc("POST /partner/consent", s.consentDecision)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if _, pattern := mux.Handler(r); pattern == "" {
			writeError(w, http.StatusNotFound, "not_found", "Unknown partner endpoint.")
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// call is the per-request audit context a handler fills in.
type call struct {
	partner Partner
	account string
	detail  map[string]any
}

type statusWriter struct {
	http.ResponseWriter
	status int
	code   string
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}

func (s *Service) api(action string, handle func(http.ResponseWriter, *http.Request, *call)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p, ok := s.authenticate(r.Context(), r.Header.Get("Authorization"))
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "Partner authentication failed.")
			return
		}
		if !s.limiter.allow(p.ID, RateLimit, s.now()) {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "rate_limited", "Partner request rate exceeded.")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 16384)
		c := &call{partner: p, detail: map[string]any{}}
		recorder := &statusWriter{ResponseWriter: w}
		handle(recorder, r, c)
		if recorder.status == 0 {
			recorder.status = http.StatusOK
		}
		c.detail["status"] = recorder.status
		if recorder.code != "" {
			c.detail["error"] = recorder.code
		}
		s.audit(context.WithoutCancel(r.Context()), c, action)
	}
}

func (s *Service) audit(ctx context.Context, c *call, action string) {
	detail, err := json.Marshal(c.detail)
	if err != nil {
		return
	}
	var account any
	if c.account != "" {
		account = c.account
	}
	// Audit is best effort after the response: the mutation already committed
	// and a failed audit write must not report a false failure to the partner.
	if err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `INSERT INTO audit_log(account_id,actor,action,created_at,detail) VALUES(?,?,?,?,?)`, account, "partner:"+c.partner.ID, "partner."+action, store.Stamp(s.now()), string(detail))
		return e
	}); err != nil {
		// Keep the partner response independent of audit storage, but make
		// the lost audit record observable without logging request data or
		// database details that may contain sensitive identifiers.
		slog.Error("partner audit persistence failed")
	}
}

// dummySecretHash equalizes timing when the partner ID is unknown.
var dummySecretHash = store.Hash("serenity-unknown-partner")

func (s *Service) authenticate(ctx context.Context, header string) (Partner, bool) {
	value, ok := strings.CutPrefix(header, "Partner ")
	if !ok {
		return Partner{}, false
	}
	id, secret, ok := strings.Cut(value, ":")
	if !ok || !ValidID(id) || secret == "" || len(secret) > 512 {
		return Partner{}, false
	}
	var p Partner
	var hash, status string
	err := s.Store.DB().QueryRowContext(ctx, `SELECT id,display_name,redirect_prefix,secret_hash,status FROM partners WHERE id=?`, id).Scan(&p.ID, &p.DisplayName, &p.RedirectPrefix, &hash, &status)
	if err != nil {
		hash = dummySecretHash
	}
	match := subtle.ConstantTimeCompare([]byte(store.Hash(secret)), []byte(hash)) == 1
	if err != nil || !match || status != "active" {
		return Partner{}, false
	}
	return p, true
}

// ValidID reports whether id is an acceptable partner identifier.
func ValidID(id string) bool {
	if len(id) < 1 || len(id) > 32 {
		return false
	}
	for _, c := range id {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return false
		}
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	body, _ := json.Marshal(map[string]string{"error": code, "message": message})
	w.Header().Set("Content-Type", "application/json")
	// The audit wrapper records the error code with the call.
	if sw, ok := w.(*statusWriter); ok {
		sw.code = code
	}
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}

func decode(r *http.Request, v any) error {
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(v); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("request must contain one JSON object")
	}
	return nil
}

func randomToken() string {
	v := make([]byte, 32)
	_, _ = rand.Read(v)
	return base64.RawURLEncoding.EncodeToString(v)
}

// linked reports whether the partner holds an active link to the account.
func (s *Service) linked(ctx context.Context, partnerID, accountID string) (bool, error) {
	var n int
	err := s.Store.DB().QueryRowContext(ctx, `SELECT count(*) FROM partner_links WHERE partner_id=? AND account_id=? AND status='active'`, partnerID, accountID).Scan(&n)
	return n == 1, err
}

func (s *Service) accountActive(ctx context.Context, accountID string) (bool, error) {
	var status string
	err := s.Store.DB().QueryRowContext(ctx, `SELECT status FROM accounts WHERE id=?`, accountID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return status == "active", err
}

func upsertLink(ctx context.Context, tx *sql.Tx, partnerID, accountID, via string, now time.Time) error {
	stamp := store.Stamp(now)
	_, err := tx.ExecContext(ctx, `INSERT INTO partner_links(partner_id,account_id,status,linked_via,created_at,updated_at) VALUES(?,?,'active',?,?,?) ON CONFLICT(partner_id,account_id) DO UPDATE SET status='active',linked_via=excluded.linked_via,updated_at=excluded.updated_at,revoked_at=NULL`, partnerID, accountID, via, stamp, stamp)
	return err
}

func (s *Service) ensure(w http.ResponseWriter, r *http.Request, c *call) {
	var req struct {
		Email         string `json:"email"`
		EmailVerified *bool  `json:"email_verified"`
	}
	if err := decode(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Body must be {email, email_verified}.")
		return
	}
	if req.EmailVerified == nil || !*req.EmailVerified {
		writeError(w, http.StatusBadRequest, "email_unverified", "The partner must verify the email address first.")
		return
	}
	now := s.now()
	account, err := s.Identity.EnsurePartnerAccount(r.Context(), req.Email, c.partner.ID, s.AccountCap, func(tx *sql.Tx, id string) error {
		return upsertLink(r.Context(), tx, c.partner.ID, id, "created", now)
	})
	switch {
	case errors.Is(err, identity.ErrInvalidEmail):
		writeError(w, http.StatusBadRequest, "invalid_email", "The email address is not valid.")
		return
	case errors.Is(err, identity.ErrCapacity):
		writeError(w, http.StatusServiceUnavailable, "capacity_reached", "Partner account capacity is reached.")
		return
	case errors.Is(err, identity.ErrAccountUnavailable):
		writeError(w, http.StatusConflict, "account_unavailable", "The account is not available.")
		return
	case err != nil:
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Try again shortly.")
		return
	}
	c.account = account.ID
	c.detail["created"] = account.Created
	linked, err := s.linked(r.Context(), c.partner.ID, account.ID)
	if err == nil && !linked && account.CreatedByPartner == c.partner.ID {
		// Only reachable when the account predates its link row: a link that
		// was revoked is kept as a row, so a revoked link is never restored
		// here and always needs the owner's consent.
		var rows int
		if err = s.Store.DB().QueryRowContext(r.Context(), `SELECT count(*) FROM partner_links WHERE partner_id=? AND account_id=?`, c.partner.ID, account.ID).Scan(&rows); err == nil && rows == 0 {
			err = s.Store.Transaction(r.Context(), func(tx *sql.Tx) error {
				return upsertLink(r.Context(), tx, c.partner.ID, account.ID, "created", now)
			})
			linked = err == nil
		}
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Try again shortly.")
		return
	}
	if linked {
		// Provisioning is idempotent; a retry completes an interrupted one.
		if _, err = s.Provision.Provision(r.Context(), account.ID); err != nil {
			writeError(w, http.StatusServiceUnavailable, "provisioning", "The memory is being prepared. Retry shortly.")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"account_id": account.ID, "created": account.Created, "linked": linked})
}

// ValidateRedirectPrefix accepts an https URL or a custom app scheme with a
// host, with no credentials, query, or fragment.
func ValidateRedirectPrefix(prefix string) error {
	u, err := url.Parse(prefix)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return errors.New("redirect prefix must be scheme://host[/path] without query or fragment")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "javascript", "data", "file", "blob", "about", "vbscript":
		return errors.New("redirect prefix must use https or an app scheme")
	}
	if strings.ContainsAny(prefix, " \t\r\n;'\"<>\\") {
		return errors.New("redirect prefix contains invalid characters")
	}
	return nil
}

// returnURLAllowed requires return to equal prefix or extend it at a path or
// query boundary, so a lookalike host such as prefix+".evil" never matches.
func returnURLAllowed(prefix, raw string) bool {
	if raw == "" || len(raw) > 2048 || strings.ContainsAny(raw, " \t\r\n#\\") {
		return false
	}
	if _, err := url.Parse(raw); err != nil {
		return false
	}
	return raw == prefix || strings.HasPrefix(raw, prefix+"/") || strings.HasPrefix(raw, prefix+"?")
}

func (s *Service) createLinkRequest(w http.ResponseWriter, r *http.Request, c *call) {
	var req struct {
		AccountID string `json:"account_id"`
		ReturnURL string `json:"return_url"`
	}
	if err := decode(r, &req); err != nil || req.AccountID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "Body must be {account_id, return_url}.")
		return
	}
	c.account = req.AccountID
	if !returnURLAllowed(c.partner.RedirectPrefix, req.ReturnURL) {
		writeError(w, http.StatusBadRequest, "invalid_return_url", "return_url must match the partner's registered redirect prefix.")
		return
	}
	active, err := s.accountActive(r.Context(), req.AccountID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Try again shortly.")
		return
	}
	if !active {
		writeError(w, http.StatusNotFound, "not_found", "Account not found.")
		return
	}
	linked, err := s.linked(r.Context(), c.partner.ID, req.AccountID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Try again shortly.")
		return
	}
	if linked {
		writeError(w, http.StatusConflict, "already_linked", "The account is already linked.")
		return
	}
	now := s.now()
	id := store.ID()
	expires := now.Add(LinkRequestTTL)
	err = s.Store.Transaction(r.Context(), func(tx *sql.Tx) error {
		_, e := tx.ExecContext(r.Context(), `INSERT INTO link_requests(id,partner_id,account_id,return_url,status,created_at,expires_at) VALUES(?,?,?,?,'pending',?,?)`, id, c.partner.ID, req.AccountID, req.ReturnURL, store.Stamp(now), store.Stamp(expires))
		return e
	})
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Try again shortly.")
		return
	}
	c.detail["link_request_id"] = id
	writeJSON(w, http.StatusCreated, map[string]any{"link_request_id": id, "consent_url": s.Origin + "/partner/consent?request=" + url.QueryEscape(id), "expires_at": expires.Format(time.RFC3339)})
}

var errRedeemState = errors.New("link request is not redeemable")
var errRedeemExpired = errors.New("link request expired")
var errRedeemCode = errors.New("invalid code")

func (s *Service) redeem(w http.ResponseWriter, r *http.Request, c *call) {
	id, ok := strings.CutSuffix(r.PathValue("action"), ":redeem")
	if !ok || id == "" {
		writeError(w, http.StatusNotFound, "not_found", "Unknown partner endpoint.")
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := decode(r, &req); err != nil || req.Code == "" || len(req.Code) > 128 {
		writeError(w, http.StatusBadRequest, "invalid_request", "Body must be {code}.")
		return
	}
	c.detail["link_request_id"] = id
	now := s.now()
	var account string
	err := s.Store.Transaction(r.Context(), func(tx *sql.Tx) error {
		var status, expires string
		var codeHash sql.NullString
		e := tx.QueryRowContext(r.Context(), `SELECT account_id,status,code_hash,expires_at FROM link_requests WHERE id=? AND partner_id=?`, id, c.partner.ID).Scan(&account, &status, &codeHash, &expires)
		if errors.Is(e, sql.ErrNoRows) {
			return store.ErrNotFound
		}
		if e != nil {
			return e
		}
		if status != "approved" || !codeHash.Valid {
			return errRedeemState
		}
		expiry, e := time.Parse(time.RFC3339Nano, expires)
		if e != nil {
			return e
		}
		if !expiry.After(now) {
			return errRedeemExpired
		}
		if subtle.ConstantTimeCompare([]byte(store.Hash(req.Code)), []byte(codeHash.String)) != 1 {
			return errRedeemCode
		}
		var accountStatus string
		if e = tx.QueryRowContext(r.Context(), `SELECT status FROM accounts WHERE id=?`, account).Scan(&accountStatus); e != nil {
			return e
		}
		if accountStatus != "active" {
			return errRedeemState
		}
		result, e := tx.ExecContext(r.Context(), `UPDATE link_requests SET status='redeemed',redeemed_at=? WHERE id=? AND status='approved'`, store.Stamp(now), id)
		if e != nil {
			return e
		}
		if n, e := result.RowsAffected(); e != nil || n != 1 {
			return errors.Join(errRedeemState, e)
		}
		return upsertLink(r.Context(), tx, c.partner.ID, account, "consent", now)
	})
	c.account = account
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "Link request not found.")
	case errors.Is(err, errRedeemState):
		writeError(w, http.StatusConflict, "not_redeemable", "The link request is not approved or was already redeemed.")
	case errors.Is(err, errRedeemExpired):
		writeError(w, http.StatusGone, "expired", "The link request expired.")
	case errors.Is(err, errRedeemCode):
		writeError(w, http.StatusBadRequest, "invalid_code", "The code does not match.")
	case err != nil:
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Try again shortly.")
	default:
		if _, e := s.Provision.Provision(r.Context(), account); e != nil {
			writeError(w, http.StatusServiceUnavailable, "provisioning", "Linked; the memory is being prepared. Retry credentials shortly.")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"account_id": account, "linked": true})
	}
}

// requireLink writes the error response and returns false unless the partner
// holds an active link to an active account.
func (s *Service) requireLink(w http.ResponseWriter, r *http.Request, c *call, accountID string) bool {
	c.account = accountID
	active, err := s.accountActive(r.Context(), accountID)
	if err == nil && active {
		var linked bool
		linked, err = s.linked(r.Context(), c.partner.ID, accountID)
		if err == nil && linked {
			return true
		}
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Try again shortly.")
		return false
	}
	writeError(w, http.StatusNotFound, "not_linked", "The account is not linked to this partner.")
	return false
}

func (s *Service) issue(w http.ResponseWriter, r *http.Request, c *call) {
	var req struct {
		AccountID string `json:"account_id"`
	}
	if err := decode(r, &req); err != nil || req.AccountID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "Body must be {account_id}.")
		return
	}
	if !s.requireLink(w, r, c, req.AccountID) {
		return
	}
	brain, err := s.Provision.Provision(r.Context(), req.AccountID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "provisioning", "The memory is being prepared. Retry shortly.")
		return
	}
	id, raw, err := s.Issuer.IssuePartner(r.Context(), req.AccountID, brain.ID, c.partner.ID)
	if errors.Is(err, credential.ErrLimit) {
		writeError(w, http.StatusConflict, "credential_limit", "The account has too many active connections.")
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Try again shortly.")
		return
	}
	c.detail["credential_id"] = id
	writeJSON(w, http.StatusCreated, map[string]any{"credential_id": id, "key": raw})
}

func (s *Service) revokeKey(w http.ResponseWriter, r *http.Request, c *call) {
	id := r.PathValue("id")
	c.detail["credential_id"] = id
	var account string
	err := s.Store.DB().QueryRowContext(r.Context(), `SELECT account_id FROM client_credentials WHERE id=? AND partner_id=?`, id, c.partner.ID).Scan(&account)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "not_found", "Credential not found.")
		return
	}
	if err == nil {
		c.account = account
		err = s.Issuer.RevokeKey(r.Context(), id)
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Try again shortly.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Unlink ends a partner link: it revokes every key the partner holds for the
// account and invalidates consent still in flight. It never deletes the
// account or its brains, and never touches other clients. It returns
// store.ErrNotFound when no active link exists.
func Unlink(ctx context.Context, db *store.Store, partnerID, accountID string, now time.Time) error {
	return db.Transaction(ctx, func(tx *sql.Tx) error {
		stamp := store.Stamp(now)
		result, err := tx.ExecContext(ctx, `UPDATE partner_links SET status='revoked',revoked_at=?,updated_at=? WHERE partner_id=? AND account_id=? AND status='active'`, stamp, stamp, partnerID, accountID)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return store.ErrNotFound
		}
		if _, err = tx.ExecContext(ctx, `UPDATE client_credentials SET revoked_at=? WHERE account_id=? AND partner_id=? AND revoked_at IS NULL`, stamp, accountID, partnerID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE link_requests SET status='denied',decided_at=? WHERE partner_id=? AND account_id=? AND status IN ('pending','approved')`, stamp, partnerID, accountID)
		return err
	})
}

func (s *Service) unlink(w http.ResponseWriter, r *http.Request, c *call) {
	c.account = r.PathValue("account")
	err := Unlink(r.Context(), s.Store, c.partner.ID, c.account, s.now())
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "No active link for this account.")
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Try again shortly.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) entitlement(w http.ResponseWriter, r *http.Request, c *call) {
	var req struct {
		Tier      string  `json:"tier"`
		ExpiresAt *string `json:"expires_at"`
	}
	if err := decode(r, &req); err != nil || (req.Tier != "free" && req.Tier != "pro") {
		writeError(w, http.StatusBadRequest, "invalid_request", `Body must be {tier:"free"|"pro", expires_at}.`)
		return
	}
	var expires any
	var expiresOut any
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "expires_at must be RFC 3339.")
			return
		}
		expires = store.Stamp(t)
		expiresOut = t.UTC().Format(time.RFC3339)
	}
	if !s.requireLink(w, r, c, r.PathValue("account")) {
		return
	}
	c.detail["tier"] = req.Tier
	err := s.Store.Transaction(r.Context(), func(tx *sql.Tx) error {
		_, e := tx.ExecContext(r.Context(), `INSERT INTO partner_entitlements(account_id,partner_id,tier,expires_at,updated_at) VALUES(?,?,?,?,?) ON CONFLICT(account_id,partner_id) DO UPDATE SET tier=excluded.tier,expires_at=excluded.expires_at,updated_at=excluded.updated_at`, c.account, c.partner.ID, req.Tier, expires, store.Stamp(s.now()))
		return e
	})
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Try again shortly.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"account_id": c.account, "tier": req.Tier, "expires_at": expiresOut})
}

func (s *Service) committed(ctx context.Context, accountID, window string) (map[string]int64, error) {
	out := map[string]int64{"writes": 0, "recalls": 0, "input_tokens": 0}
	for metric := range out {
		var used int64
		if err := s.Store.DB().QueryRowContext(ctx, `SELECT COALESCE((SELECT committed FROM usage_windows WHERE account_id=? AND window_key=? AND metric=?),0)`, accountID, window, metric).Scan(&used); err != nil {
			return nil, err
		}
		out[metric] = used
	}
	return out, nil
}

// usage reports the account plan and, when live, the partner bucket. The
// shape is the one Blink's client codes against; "partner" is null when the
// partner has no live pro entitlement for the account.
func (s *Service) usage(w http.ResponseWriter, r *http.Request, c *call) {
	if !s.requireLink(w, r, c, r.PathValue("account")) {
		return
	}
	ctx := r.Context()
	account, err := s.Meter.Entitlement(ctx, c.account)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Try again shortly.")
		return
	}
	partnerEnt, live, err := s.Meter.PartnerEntitlement(ctx, c.account, c.partner.ID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Try again shortly.")
		return
	}
	inventory, err := s.Gateway.Inventory(ctx, c.account, s.Provision.BrainsRoot)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Try again shortly.")
		return
	}
	accountUsed, err := s.committed(ctx, c.account, account.Window)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Try again shortly.")
		return
	}
	body := map[string]any{
		"account_id": c.account,
		"account": map[string]any{
			"plan":          account.Plan.ID,
			"writes":        accountUsed["writes"],
			"recalls":       accountUsed["recalls"],
			"input_tokens":  accountUsed["input_tokens"],
			"memories":      inventory.Memories,
			"storage_bytes": inventory.StorageBytes,
			"limits": map[string]int64{
				"writes":        account.Plan.Writes,
				"recalls":       account.Plan.Recalls,
				"input_tokens":  account.Plan.InputTokens,
				"memories":      account.Plan.Memories,
				"storage_bytes": account.Plan.StorageBytes,
			},
			"reset_at": account.ResetAt.UTC().Format(time.RFC3339),
		},
		"partner": nil,
	}
	if live {
		partnerUsed, err := s.committed(ctx, c.account, partnerEnt.Window)
		if err != nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable", "Try again shortly.")
			return
		}
		body["partner"] = map[string]any{
			"tier":          "pro",
			"plan":          partnerEnt.Plan.ID,
			"writes":        partnerUsed["writes"],
			"recalls":       partnerUsed["recalls"],
			"input_tokens":  partnerUsed["input_tokens"],
			"memories":      inventory.Memories,
			"storage_bytes": inventory.StorageBytes,
			"limits": map[string]int64{
				"writes":       partnerEnt.Plan.Writes,
				"recalls":      partnerEnt.Plan.Recalls,
				"input_tokens": partnerEnt.Plan.InputTokens,
				// Inventory is account-wide; partner-pro writes may fill it to
				// the larger of the two plans (ADR 023).
				"memories":      max(partnerEnt.Plan.Memories, account.Plan.Memories),
				"storage_bytes": max(partnerEnt.Plan.StorageBytes, account.Plan.StorageBytes),
			},
			"reset_at": partnerEnt.ResetAt.UTC().Format(time.RFC3339),
		}
	}
	writeJSON(w, http.StatusOK, body)
}

// Seed creates or rotates a partner. Only the SHA-256 digest of secret is
// stored. Rotation replaces the digest in place; existing links and keys
// are unaffected.
func Seed(ctx context.Context, db *store.Store, id, displayName, secret, redirectPrefix, status string, now time.Time) error {
	if !ValidID(id) {
		return errors.New("partner id must be 1-32 characters of a-z, 0-9, - or _")
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" || len(displayName) > 64 {
		return errors.New("display name must be 1-64 characters")
	}
	if len(secret) < 32 {
		return errors.New("partner secret must be at least 32 characters")
	}
	if err := ValidateRedirectPrefix(redirectPrefix); err != nil {
		return err
	}
	if status == "" {
		status = "active"
	}
	if status != "active" && status != "disabled" {
		return errors.New("status must be active or disabled")
	}
	sum := sha256.Sum256([]byte(secret))
	stamp := store.Stamp(now)
	return db.Transaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO partners(id,display_name,secret_hash,redirect_prefix,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET display_name=excluded.display_name,secret_hash=excluded.secret_hash,redirect_prefix=excluded.redirect_prefix,status=excluded.status,updated_at=excluded.updated_at`, id, displayName, hex.EncodeToString(sum[:]), redirectPrefix, status, stamp, stamp)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO audit_log(actor,action,created_at,detail) VALUES('operator','partner.seed',?,?)`, stamp, id)
		return err
	})
}

type bucket struct {
	start time.Time
	count int
}

// limiter is a fixed one-minute window per partner.
type limiter struct {
	mu      sync.Mutex
	buckets map[string]bucket
}

func (l *limiter) allow(key string, limit int, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.buckets == nil {
		l.buckets = map[string]bucket{}
	}
	b := l.buckets[key]
	if now.Sub(b.start) >= time.Minute || now.Before(b.start) {
		b = bucket{start: now}
	}
	if b.count >= limit {
		return false
	}
	b.count++
	l.buckets[key] = b
	return true
}
