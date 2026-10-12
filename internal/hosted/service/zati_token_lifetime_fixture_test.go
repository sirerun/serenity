package service_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/ajent-social/go/mcpoauth"
	"github.com/sirerun/serenity/internal/hosted/store"
)

// Synthetic lifetime shaping only: actual code redemption and token creation
// run first. Shorten the exact persisted token and response together. Refresh
// issuance remains production's one hour so unattended renewal does not loop.
func shortInitialOAuthTokenFixture(next http.Handler, db *store.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/oauth/token" {
			next.ServeHTTP(w, r)
			return
		}
		requestBody, err := io.ReadAll(io.LimitReader(r.Body, 65537))
		if err != nil || len(requestBody) > 65536 {
			http.Error(w, "invalid fixture token request", 400)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(requestBody))
		form, err := url.ParseQuery(string(requestBody))
		if err != nil {
			http.Error(w, "invalid fixture token form", 400)
			return
		}
		if form.Get("grant_type") != "authorization_code" {
			next.ServeHTTP(w, r)
			return
		}
		capture := httptest.NewRecorder()
		next.ServeHTTP(capture, r)
		if capture.Code == http.StatusOK {
			var body map[string]json.RawMessage
			var raw string
			err := json.Unmarshal(capture.Body.Bytes(), &body)
			if err == nil {
				err = json.Unmarshal(body["access_token"], &raw)
			}
			if err == nil && raw == "" {
				err = fmt.Errorf("missing access token")
			}
			if err == nil {
				err = db.Transaction(r.Context(), func(tx *sql.Tx) error {
					var record []byte
					id := store.Hash(raw)
					if err := tx.QueryRowContext(r.Context(), "SELECT record FROM oauth_tokens WHERE id=?", id).Scan(&record); err != nil {
						return err
					}
					var token mcpoauth.TokenRecord
					if err := json.Unmarshal(record, &token); err != nil {
						return err
					}
					expiry := time.Now().UTC().Add(4 * time.Minute)
					if token.ID != id || token.GrantID == "" || !token.ExpiresAt.After(expiry) {
						return fmt.Errorf("unexpected issued token")
					}
					token.ExpiresAt = expiry
					encoded, err := json.Marshal(token)
					if err != nil {
						return err
					}
					result, err := tx.ExecContext(r.Context(), "UPDATE oauth_tokens SET record=?,expires_at=? WHERE id=?", encoded, store.Stamp(expiry), id)
					if err != nil {
						return err
					}
					n, err := result.RowsAffected()
					if err != nil {
						return err
					}
					if n != 1 {
						return fmt.Errorf("token update cardinality")
					}
					return nil
				})
			}
			if err != nil {
				http.Error(w, "fixture lifetime shaping failed", 500)
				return
			}
			body["expires_in"] = json.RawMessage("240")
			encoded, err := json.Marshal(body)
			if err != nil {
				http.Error(w, "fixture token encoding failed", 500)
				return
			}
			capture.Body.Reset()
			_, _ = capture.Body.Write(encoded)
			capture.Header().Del("Content-Length")
		}
		for name, values := range capture.Header() {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		w.WriteHeader(capture.Code)
		_, _ = w.Write(capture.Body.Bytes())
	})
}
