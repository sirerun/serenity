package service

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/episodejson"
	hoststore "github.com/sirerun/serenity/internal/hosted/store"
)

// This control is registered only on AdminHandler, behind the existing private
// Unix peer checks. It never creates an identity, issues a key or verifies email.
func (s *Service) configureEpisodeQualification(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.RawQuery != "" || r.Header.Get("Origin") != "" {
		http.Error(w, "qualification refused", http.StatusBadRequest)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil || !episodejson.Valid(raw) {
		http.Error(w, "qualification refused", http.StatusBadRequest)
		return
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		http.Error(w, "qualification refused", http.StatusBadRequest)
		return
	}
	if value, ok := fields["brain_ids"]; !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		http.Error(w, "qualification refused", http.StatusBadRequest)
		return
	}
	for key := range fields {
		if key != "brain_ids" && key != "expires_at" {
			http.Error(w, "qualification refused", http.StatusBadRequest)
			return
		}
	}
	var req struct {
		Brains  []string  `json:"brain_ids"`
		Expires time.Time `json:"expires_at"`
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil || d.Decode(new(any)) != io.EOF || len(req.Brains) > 2 {
		http.Error(w, "qualification refused", http.StatusBadRequest)
		return
	}
	if s.cfg.WorkOSEnabled || s.cfg.BillingEnabled || s.cfg.EpisodeArchiveEnabled || len(s.cfg.EpisodeSyntheticBrains) > 0 {
		http.Error(w, "qualification unavailable", http.StatusConflict)
		return
	}
	s.Gateway.Maintenance.RLock()
	defer s.Gateway.Maintenance.RUnlock()
	// Only dedicated operator fixtures qualify. A real account, partner-created
	// account, paid account or nondefault/deleted brain is never accepted.
	err = s.Store.Transaction(r.Context(), func(tx *sql.Tx) error {
		for _, brain := range req.Brains {
			var email string
			e := tx.QueryRowContext(r.Context(), `SELECT a.email FROM brains b JOIN accounts a ON a.id=b.account_id WHERE b.id=? AND b.path_key=b.id AND b.is_default=1 AND b.state='ready' AND b.deleted_at IS NULL AND a.status='active' AND a.plan_id='free' AND a.plan_version=1 AND a.created_by_partner IS NULL AND COALESCE(a.stripe_customer_id,'')='' AND NOT EXISTS(SELECT 1 FROM subscriptions x WHERE x.account_id=a.id)`, brain).Scan(&email)
			if e != nil {
				return e
			}
			const suffix = "@serenity-episode-fixture.invalid"
			if !strings.HasSuffix(email, suffix) || len(strings.TrimSuffix(email, suffix)) < 24 {
				return sql.ErrNoRows
			}
		}
		detail, marshalErr := json.Marshal(req)
		if marshalErr != nil {
			return marshalErr
		}
		_, auditErr := tx.ExecContext(r.Context(), `INSERT INTO audit_log(actor,action,created_at,detail) VALUES('operator','episode.qualification.request',?,?)`, hoststore.Stamp(time.Now()), string(detail))
		return auditErr
	})
	if err != nil {
		http.Error(w, "qualification refused", http.StatusForbidden)
		return
	}
	if err = s.Gateway.SetEpisodeQualification(req.Brains, req.Expires); err != nil {
		http.Error(w, "qualification refused", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// revokeEpisodeQualificationCredentials cleans up explicitly named operator
// fixture keys without issuing identity or changing another client credential.
// Admission and the per-key update share one transaction, including mixed batches.
func (s *Service) revokeEpisodeQualificationCredentials(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.RawQuery != "" || r.Header.Get("Origin") != "" {
		http.Error(w, "qualification refused", http.StatusBadRequest)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	var fields map[string]json.RawMessage
	if err != nil || !episodejson.Valid(raw) || json.Unmarshal(raw, &fields) != nil || len(fields) != 1 {
		http.Error(w, "qualification refused", http.StatusBadRequest)
		return
	}
	value, ok := fields["credential_ids"]
	var ids []string
	if !ok || json.Unmarshal(value, &ids) != nil || len(ids) == 0 || len(ids) > 8 {
		http.Error(w, "qualification refused", http.StatusBadRequest)
		return
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || len(id) > 128 || seen[id] {
			http.Error(w, "qualification refused", http.StatusBadRequest)
			return
		}
		seen[id] = true
	}
	if s.cfg.WorkOSEnabled || s.cfg.BillingEnabled || s.cfg.EpisodeArchiveEnabled || len(s.cfg.EpisodeSyntheticBrains) > 0 {
		http.Error(w, "qualification unavailable", http.StatusConflict)
		return
	}
	s.Gateway.Maintenance.RLock()
	defer s.Gateway.Maintenance.RUnlock()
	err = s.Store.Transaction(r.Context(), func(tx *sql.Tx) error {
		for _, id := range ids {
			var email string
			err := tx.QueryRowContext(r.Context(), `SELECT a.email FROM client_credentials c JOIN accounts a ON a.id=c.account_id JOIN brains b ON b.id=c.brain_id AND b.account_id=a.id WHERE c.id=? AND COALESCE(c.partner_id,'')='' AND b.path_key=b.id AND b.is_default=1 AND b.state='ready' AND b.deleted_at IS NULL AND a.status='active' AND a.plan_id='free' AND a.plan_version=1 AND a.created_by_partner IS NULL AND COALESCE(a.stripe_customer_id,'')='' AND NOT EXISTS(SELECT 1 FROM subscriptions x WHERE x.account_id=a.id)`, id).Scan(&email)
			const suffix = "@serenity-episode-fixture.invalid"
			if err != nil || !strings.HasSuffix(email, suffix) || len(strings.TrimSuffix(email, suffix)) < 24 {
				return sql.ErrNoRows
			}
			if _, err = tx.ExecContext(r.Context(), `UPDATE client_credentials SET revoked_at=? WHERE id=? AND revoked_at IS NULL`, hoststore.Stamp(time.Now()), id); err != nil {
				return err
			}
		}
		detail, err := json.Marshal(ids)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO audit_log(actor,action,created_at,detail) VALUES('operator','episode.qualification.revoke',?,?)`, hoststore.Stamp(time.Now()), string(detail))
		return err
	})
	if err != nil {
		http.Error(w, "qualification refused", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
