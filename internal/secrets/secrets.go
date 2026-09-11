// Package secrets stores key material in the OS keychain — never in
// files, the brain repo, or the index (RFC §14). The daemon bearer token
// is on by default: loopback is authenticated too, because any local
// process is not automatically trusted.
package secrets

import (
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/zalando/go-keyring"
)

// Service is the keychain service name for all Serenity secrets.
const Service = "serenity"

const daemonTokenKey = "daemon-auth-token"

// ErrNotFound reports an absent secret.
var ErrNotFound = keyring.ErrNotFound

// DaemonToken returns the stored daemon bearer token.
func DaemonToken() (string, error) {
	return keyring.Get(Service, daemonTokenKey)
}

// EnsureDaemonToken returns the daemon token, generating and storing a
// new one when absent. created reports whether a new token was minted.
func EnsureDaemonToken() (token string, created bool, err error) {
	t, err := keyring.Get(Service, daemonTokenKey)
	if err == nil {
		return t, false, nil
	}
	if !errors.Is(err, keyring.ErrNotFound) {
		return "", false, err
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", false, err
	}
	t = hex.EncodeToString(buf)
	if err := keyring.Set(Service, daemonTokenKey, t); err != nil {
		return "", false, err
	}
	return t, true, nil
}

// RotateDaemonToken mints a fresh daemon bearer token and overwrites the
// stored one, invalidating whatever token was previously issued
// (`serenity connect --rotate-token`, ADR 010: "a leaked token is revoked
// by one command").
func RotateDaemonToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	t := hex.EncodeToString(buf)
	if err := keyring.Set(Service, daemonTokenKey, t); err != nil {
		return "", err
	}
	return t, nil
}

// MockForTesting swaps the OS keychain for an in-memory store. Test-only.
func MockForTesting() { keyring.MockInit() }

// profileAccountKey namespaces an explicit, operator-selected credential
// profile's keychain account distinctly from both the legacy shared
// daemonTokenKey and every other profile name (RFC-BRAIN-AUTH-02). This is
// the single derivation `connect --credential-profile` (provisioning,
// status, rotation) and `serve --http --credential-profile` all use, so
// they are guaranteed to resolve to the identical keychain entry for a
// given name.
func profileAccountKey(name string) string { return daemonTokenKey + ":profile:" + name }

// ProfileDaemonToken returns the stored bearer token for an explicit
// credential profile. An unprovisioned profile reports ErrNotFound exactly
// like DaemonToken does for the legacy slot, so callers fail closed
// identically; it never falls back to the legacy shared token.
func ProfileDaemonToken(name string) (string, error) {
	return keyring.Get(Service, profileAccountKey(name))
}

// EnsureProfileDaemonToken returns the profile's token, minting and storing
// a fresh, independent random one when absent. It never copies the legacy
// shared token's value: a newly provisioned profile always starts from its
// own independent secret.
func EnsureProfileDaemonToken(name string) (token string, created bool, err error) {
	t, err := keyring.Get(Service, profileAccountKey(name))
	if err == nil {
		return t, false, nil
	}
	if !errors.Is(err, keyring.ErrNotFound) {
		return "", false, err
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", false, err
	}
	t = hex.EncodeToString(buf)
	if err := keyring.Set(Service, profileAccountKey(name), t); err != nil {
		return "", false, err
	}
	return t, true, nil
}

// RotateProfileDaemonToken mints a fresh token for exactly this profile,
// invalidating whatever it previously held. Every other profile, and the
// legacy shared token, are untouched -- distinct keychain accounts, so
// rotation never affects a slot beyond the one explicitly named.
func RotateProfileDaemonToken(name string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	t := hex.EncodeToString(buf)
	if err := keyring.Set(Service, profileAccountKey(name), t); err != nil {
		return "", err
	}
	return t, nil
}
