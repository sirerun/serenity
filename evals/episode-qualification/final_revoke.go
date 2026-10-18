//go:build darwin || linux

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// verifyFinalRevoke runs only inside the service-UID child after the exact v3
// revoke. It returns no private inventory, credentials or approval assertion.
func (e *engine) verifyFinalRevoke(ctx context.Context) (result error) {
	if e.allowed(ctx) != nil {
		return errRefused
	}
	unlock, err := e.lockFixture(false)
	if err != nil {
		return errRefused
	}
	defer func() { refuseCleanup(&result, unlock()) }()
	directory, err := os.Open(e.privateDir)
	if err != nil {
		return errRefused
	}
	entries, readErr := directory.ReadDir(3)
	closeErr := directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) || closeErr != nil || len(entries) != 2 {
		return errRefused
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.Name() != "manifest.json" && entry.Name() != "fixture.lock" {
			return errRefused
		}
		info, err := os.Lstat(filepath.Join(e.privateDir, entry.Name()))
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			return errRefused
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Nlink != 1 || int(st.Uid) != os.Geteuid() {
			return errRefused
		}
		seen[entry.Name()] = true
	}
	if !seen["manifest.json"] || !seen["fixture.lock"] {
		return errRefused
	}
	m, err := e.load(ctx)
	if err != nil || len(m.Accounts) != 2 {
		return errRefused
	}
	for i := range m.Accounts {
		a := &m.Accounts[i]
		if !generatedID.MatchString(a.AccountID) || !generatedID.MatchString(a.BrainID) || len(a.CredentialIDs) == 0 || a.Key != "" || a.Pending != "" || e.inspect(ctx, a, true) != nil {
			return errRefused
		}
		for _, id := range a.CredentialIDs {
			var count int
			if e.store.DB().QueryRowContext(ctx, "SELECT count(*) FROM client_credentials WHERE id=? AND account_id=? AND brain_id=? AND revoked_at IS NOT NULL", id, a.AccountID, a.BrainID).Scan(&count) != nil || count != 1 {
				return errRefused
			}
		}
	}
	return e.allowed(ctx)
}
