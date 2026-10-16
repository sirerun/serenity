//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"time"
)

// sourceAdoption is authority only when carried by the inherited root v2
// proof. It is never populated from a CLI flag, environment or manifest.
type sourceAdoption struct {
	OldSource         string `json:"old_source"`
	OldManifestSHA256 string `json:"old_manifest_sha256"`
}

func (a sourceAdoption) valid(current string) bool {
	return sourceID.MatchString(a.OldSource) && sourceID.MatchString(current) &&
		a.OldSource != current && hexDigest.MatchString(a.OldManifestSHA256)
}

// uniqueManifestJSON rejects duplicate/case-aliased fields, including inside
// nested targets/accounts. DisallowUnknownFields alone accepts duplicates.
func uniqueManifestJSON(data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	var value func() error
	value = func() error {
		token, err := d.Token()
		if err != nil {
			return errRefused
		}
		switch token {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				name, ok := key.(string)
				name = strings.ToLower(name)
				if err != nil || !ok || seen[name] {
					return errRefused
				}
				seen[name] = true
				if value() != nil {
					return errRefused
				}
			}
			if end, err := d.Token(); err != nil || end != json.Delim('}') {
				return errRefused
			}
		case json.Delim('['):
			for d.More() {
				if value() != nil {
					return errRefused
				}
			}
			if end, err := d.Token(); err != nil || end != json.Delim(']') {
				return errRefused
			}
		}
		return nil
	}
	if value() != nil {
		return errRefused
	}
	if _, err := d.Token(); err != io.EOF {
		return errRefused
	}
	return nil
}

// adoptSource is deliberately separate from operate: prepare/reissue/revoke
// never substitute an expected source or accept adoption authority. Production
// dispatch supplies the separately fenced, non-migrating read-only store.
func (e *engine) adoptSource(ctx context.Context, binding sourceAdoption) (result error) {
	if !binding.valid(e.target.Source) || e.validate(ctx) != nil {
		return errRefused
	}
	unlock, err := e.lockFixture(false)
	if err != nil {
		return err
	}
	published := false
	defer func() {
		if err := unlock(); err != nil {
			if published {
				result = errAdoptionPublicationUncertain
			} else {
				result = errRefused
			}
		}
	}()
	oldTarget := e.target
	oldTarget.Source = binding.OldSource
	m, err := e.loadBound(ctx, oldTarget, binding.OldManifestSHA256)
	if err != nil {
		return err
	}
	var reserved int
	if e.store.DB().QueryRowContext(ctx, `SELECT count(*) FROM accounts WHERE email LIKE 'episode-%@serenity-episode-fixture.invalid'`).Scan(&reserved) != nil || reserved != 2 {
		return errRefused
	}
	// Do not recover partial intents or adopt an empty/revoked-without-custody
	// inventory. Every retained secret must pass the existing secret-checked
	// revoked-key recovery in inspect, and every recorded credential must be
	// revoked, including older credentials without a retained key string.
	for i := range m.Accounts {
		a := &m.Accounts[i]
		if a.Pending != "" || a.AccountID == "" || a.BrainID == "" || a.Key == "" || len(a.CredentialIDs) == 0 {
			return errRefused
		}
		var billing int
		if e.store.DB().QueryRowContext(ctx, `SELECT count(*) FROM billing_failures WHERE account_id=?`, a.AccountID).Scan(&billing) != nil || billing != 0 {
			return errRefused
		}
		var foreignKeys int
		if e.store.DB().QueryRowContext(ctx, `SELECT count(*) FROM client_credentials WHERE brain_id=? AND account_id<>?`, a.BrainID, a.AccountID).Scan(&foreignKeys) != nil || foreignKeys != 0 {
			return errRefused
		}
		if e.inspect(ctx, a, true) != nil {
			return errRefused
		}
		for _, id := range a.CredentialIDs {
			var stamp string
			if e.store.DB().QueryRowContext(ctx, `SELECT revoked_at FROM client_credentials WHERE id=? AND account_id=? AND brain_id=?`, id, a.AccountID, a.BrainID).Scan(&stamp) != nil {
				return errRefused
			}
			if at, err := time.Parse(time.RFC3339Nano, stamp); err != nil || at.IsZero() {
				return errRefused
			}
		}
	}
	if e.capacity(ctx, 0) != nil || e.allowed(ctx) != nil {
		return errRefused
	}
	m.Target.Source = e.target.Source
	err = e.persistChecked(ctx, m, func() error {
		if e.mark("adoption-before-publication") != nil {
			return errAdoptionNotPublished
		}
		if e.allowed(ctx) != nil {
			return errRefused
		}
		// Recheck the exact original private file immediately before replacement.
		if _, err := e.loadBound(ctx, oldTarget, binding.OldManifestSHA256); err != nil {
			return err
		}
		return e.allowed(ctx)
	}, func() error {
		published = true
		return e.mark("adoption-after-publication")
	})
	if published && (err != nil || e.mark("adoption-after-durable-publication") != nil || e.allowed(ctx) != nil) {
		return errAdoptionPublicationUncertain
	}
	return err
}
