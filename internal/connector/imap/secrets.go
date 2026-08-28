package imap

import (
	"github.com/zalando/go-keyring"

	"github.com/sirerun/serenity/internal/secrets"
)

// passwordKey namespaces IMAP app-password entries by account inside the
// shared "serenity" keychain service (internal/secrets.Service), so
// multiple mailboxes never collide on one keyring entry.
func passwordKey(account string) string { return "imap-app-password:" + account }

// StoredPassword returns the app password stored for account. It reports
// secrets.ErrNotFound (via keyring.ErrNotFound, which secrets.ErrNotFound
// aliases) when `serenity connectors auth imap` has not run for account.
func StoredPassword(account string) (string, error) {
	return keyring.Get(secrets.Service, passwordKey(account))
}

// StorePassword stores password as account's IMAP app password in the OS
// keychain only -- never on disk (ADR 001) -- overwriting any previously
// stored value. This is the re-auth path: RFC 0001 §10.1 requires re-auth
// to be one command.
func StorePassword(account, password string) error {
	return keyring.Set(secrets.Service, passwordKey(account), password)
}

// DeletePassword removes the stored app password for account.
func DeletePassword(account string) error {
	return keyring.Delete(secrets.Service, passwordKey(account))
}
