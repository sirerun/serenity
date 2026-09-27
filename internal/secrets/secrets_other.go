//go:build !darwin

package secrets

import "github.com/zalando/go-keyring"

// platformSet stores value under account. Keychain access control lists
// are macOS-only; elsewhere go-keyring's backend is used as is.
func platformSet(account, value string) error {
	return keyring.Set(Service, account, value)
}
