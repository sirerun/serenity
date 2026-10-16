//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/privatefs"
	"github.com/sirerun/serenity/internal/hosted/provision"
	"github.com/sirerun/serenity/internal/hosted/store"
)

// A caller MUST own an exclusive offline
// maintenance fence for its entire lifetime, including between retries. Tests
// supply fresh synthetic stores. A private manifest is custody, not authority.
// In particular, no boolean, environment variable or command line bypass exists.
type target struct {
	DataDir      string
	ConfigPath   string
	ConfigSHA256 string
	Source       string
	AccountCap   int
}

type fixtureAccount struct {
	Email         string
	AccountID     string
	BrainID       string
	CredentialIDs []string
	Key           string `json:",omitempty"`
	Pending       string `json:",omitempty"`
}

type manifest struct {
	Version      int
	Namespace    string
	CustodyEpoch string
	Target       target
	Accounts     []fixtureAccount
}

type engine struct {
	store      *store.Store
	target     target
	privateDir string
	// Test-only interruption injection; never logs or receives any secret.
	checkpoint   func(string) error
	authorize    func() error
	custodyEpoch string
}

var errRefused = errors.New("fixture refused: unsafe, foreign, ambiguous or unsupported state")
var hexDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)
var sourceID = regexp.MustCompile(`^[0-9a-f]{40}$`)
var generatedID = regexp.MustCompile(`^[A-Z2-7]{26}$`)

func (e *engine) mark(stage string) error {
	if e.checkpoint != nil {
		return e.checkpoint(stage)
	}
	return nil
}

func (e *engine) allowed(ctx context.Context) error {
	if ctx.Err() != nil {
		return errRefused
	}
	if e.authorize != nil {
		return e.authorize()
	}
	return nil // nil is only used by isolated in-package tests
}

func (e *engine) validate(ctx context.Context) error {
	if e.allowed(ctx) != nil {
		return errRefused
	}
	if e.store == nil || e.target.AccountCap <= 0 || !hexDigest.MatchString(e.target.ConfigSHA256) || !sourceID.MatchString(e.target.Source) || !filepath.IsAbs(e.target.ConfigPath) || filepath.Clean(e.target.ConfigPath) != e.target.ConfigPath {
		return errRefused
	}
	for _, dir := range []string{e.privateDir, e.target.DataDir, filepath.Join(e.target.DataDir, "brains")} {
		if err := privatefs.ValidateDirectory(ctx, dir); err != nil {
			return errRefused
		}
	}
	if e.privateDir == e.target.DataDir || e.privateDir == filepath.Join(e.target.DataDir, "brains") {
		return errRefused
	}
	var n, min, max int
	if err := e.store.DB().QueryRowContext(ctx, `SELECT count(*),min(version),max(version) FROM schema_migrations`).Scan(&n, &min, &max); err != nil || n != 10 || min != 1 || max != 10 {
		return errRefused
	}
	// Bind the actual connection to the exact target without reading customer rows.
	var seq int
	var name, path string
	if err := e.store.DB().QueryRowContext(ctx, `PRAGMA database_list`).Scan(&seq, &name, &path); err != nil || name != "main" || path != filepath.Join(e.target.DataDir, "control.db") {
		return errRefused
	}
	return nil
}

// refuseCleanup keeps cleanup failures content-free and prevents a successful
// result when a file, database, or lock could not be closed or released.
func refuseCleanup(result *error, err error) {
	if err != nil {
		*result = errRefused
	}
}

// persist uses private same-directory atomic replacement plus file/directory
// fsync. No key is returned, printed, or included in an error. The private file
// is the sole plaintext key output. Leftover temp files are never adopted.
func (e *engine) persist(ctx context.Context, m *manifest) (result error) {
	if e.allowed(ctx) != nil {
		return errRefused
	}
	if err := privatefs.ValidateDirectory(ctx, e.privateDir); err != nil {
		return errRefused
	}
	path := filepath.Join(e.privateDir, "manifest.json")
	if _, err := os.Lstat(path); err == nil {
		if _, err := privatefs.ReadFile(ctx, e.privateDir, "manifest.json", 65536); err != nil {
			return errRefused
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errRefused
	}
	data, err := json.Marshal(m)
	if err != nil {
		return errRefused
	}
	f, err := os.CreateTemp(e.privateDir, ".manifest-")
	if err != nil {
		return errRefused
	}
	temp := f.Name()
	defer func() {
		if err := os.Remove(temp); !errors.Is(err, os.ErrNotExist) {
			refuseCleanup(&result, err)
		}
	}()
	if _, err = f.Write(data); err != nil {
		refuseCleanup(&result, f.Close())
		return errRefused
	}
	if err = f.Sync(); err != nil {
		refuseCleanup(&result, f.Close())
		return errRefused
	}
	if err = f.Close(); err != nil {
		return errRefused
	}
	if err = os.Rename(temp, path); err != nil {
		return errRefused
	}
	dir, err := os.Open(e.privateDir)
	if err != nil {
		return errRefused
	}
	err = errors.Join(dir.Sync(), dir.Close())
	if err != nil {
		return errRefused
	}
	return nil
}

func (e *engine) load(ctx context.Context) (*manifest, error) {
	info, err := os.Lstat(filepath.Join(e.privateDir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Nlink != 1 {
		return nil, errRefused
	}
	data, err := privatefs.ReadFile(ctx, e.privateDir, "manifest.json", 65536)
	if err != nil {
		return nil, err
	}
	var m manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&m) != nil {
		return nil, errRefused
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, errRefused
	}
	if len(m.Accounts) != 2 || m.Version != 2 || !generatedID.MatchString(m.Namespace) || m.Target != e.target {
		return nil, errRefused
	}
	for i, a := range m.Accounts {
		if (a.Key != "" && a.Pending != "") || (a.BrainID == "" && (a.Key != "" || len(a.CredentialIDs) != 0)) || a.Email != email(m.Namespace, i) || (a.AccountID != "" && !generatedID.MatchString(a.AccountID)) || (a.BrainID != "" && !generatedID.MatchString(a.BrainID)) || (a.Pending != "" && a.Pending != "account" && a.Pending != "brain" && a.Pending != "key" && a.Pending != "revoke") {
			return nil, errRefused
		}
		for _, id := range a.CredentialIDs {
			if !generatedID.MatchString(id) {
				return nil, errRefused
			}
		}
	}
	for _, a := range m.Accounts {
		if a.Pending != "" && m.CustodyEpoch != e.custodyEpoch {
			return nil, errRefused
		}
	}
	m.CustodyEpoch = e.custodyEpoch
	if m.Accounts[0].AccountID != "" && m.Accounts[0].AccountID == m.Accounts[1].AccountID {
		return nil, errRefused
	}
	return &m, nil
}

func email(namespace string, i int) string {
	return fmt.Sprintf("episode-%s-%d@serenity-episode-fixture.invalid", strings.ToLower(namespace), i+1)
}

// operate is called only after inherited offline custody is verified. Modes are
// prepare (replay preserves keys), reissue, and revoke (retains rows/data).
func (e *engine) operate(ctx context.Context, mode string) (result error) {
	if mode != "prepare" && mode != "reissue" && mode != "revoke" {
		return errRefused
	}
	if err := e.validate(ctx); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(e.privateDir, "fixture.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return errRefused
	}
	defer func() { refuseCleanup(&result, lock.Close()) }()
	info, err := lock.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return errRefused
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 {
		return errRefused
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errRefused
	}
	defer func() { refuseCleanup(&result, syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)) }()
	m, err := e.load(ctx)
	if errors.Is(err, os.ErrNotExist) && mode == "prepare" {
		entries, readErr := os.ReadDir(e.privateDir)
		if readErr != nil {
			return errRefused
		}
		for _, entry := range entries {
			if entry.Name() != "fixture.lock" {
				return errRefused
			}
		}
		var reserved int
		if err = e.store.DB().QueryRowContext(ctx, `SELECT count(*) FROM accounts WHERE email LIKE 'episode-%@serenity-episode-fixture.invalid'`).Scan(&reserved); err != nil || reserved != 0 {
			return errRefused
		}
		m = &manifest{Version: 2, Namespace: store.ID(), Target: e.target, CustodyEpoch: e.custodyEpoch, Accounts: make([]fixtureAccount, 2)}
		for i := range m.Accounts {
			m.Accounts[i].Email = email(m.Namespace, i)
		}
		// Refuse existing reserved identities before recording any adoption intent.
		for _, a := range m.Accounts {
			var n int
			if err = e.store.DB().QueryRowContext(ctx, `SELECT count(*) FROM accounts WHERE email_hash=?`, store.Hash(a.Email)).Scan(&n); err != nil || n != 0 {
				return errRefused
			}
		}
		if err = e.capacity(ctx, 2); err != nil {
			return err
		}
		if err = e.persist(ctx, m); err != nil {
			return err
		}
	} else if err != nil {
		return errRefused
	}
	// Validate both identities before mutating either one.
	for i := range m.Accounts {
		if err = e.inspect(ctx, &m.Accounts[i]); err != nil {
			return err
		}
	}
	missing := 0
	for _, a := range m.Accounts {
		if a.AccountID == "" {
			missing++
		}
	}
	if mode != "revoke" {
		if err = e.capacity(ctx, missing); err != nil {
			return err
		}
	}
	for i := range m.Accounts {
		a := &m.Accounts[i]
		if mode == "revoke" {
			if a.BrainID == "" || a.Pending == "brain" {
				if len(a.CredentialIDs) != 0 {
					return errRefused
				}
				if err = e.persist(ctx, m); err != nil {
					return err
				}
				continue
			}
			a.Key = ""
			a.Pending = "revoke"
			if err = e.persist(ctx, m); err != nil {
				return err
			}
			if err = e.revoke(ctx, a); err != nil {
				return err
			}
			a.Key = ""
			a.Pending = ""
			if err = e.mark("keys-revoked"); err != nil {
				return err
			}
			if err = e.persist(ctx, m); err != nil {
				return err
			}
			continue
		}
		if a.AccountID == "" {
			a.Pending = "account"
			if err = e.persist(ctx, m); err != nil {
				return err
			}
			if e.allowed(ctx) != nil {
				return errRefused
			}
			account, createErr := e.store.CreateAccount(ctx, a.Email)
			if createErr != nil {
				return errRefused
			}
			if err = e.mark("account-created"); err != nil {
				return err
			}
			a.AccountID = account.ID
			a.Pending = ""
			if err = e.persist(ctx, m); err != nil {
				return err
			}
		}
		if a.BrainID == "" {
			// Pre-bind the brain ID before inserting; Provision then finishes this exact
			// allocating row, under the normal writer lock. No customer SQL writes.
			a.BrainID = store.ID()
			a.Pending = "brain"
			if err = e.persist(ctx, m); err != nil {
				return err
			}
		}
		if a.Pending == "brain" {
			var n int
			if err = e.store.DB().QueryRowContext(ctx, `SELECT count(*) FROM brains WHERE id=? AND account_id=?`, a.BrainID, a.AccountID).Scan(&n); err != nil {
				return errRefused
			}
			if n == 0 {
				if e.allowed(ctx) != nil {
					return errRefused
				}
				if _, err = e.store.InsertBrain(ctx, a.AccountID, a.BrainID, a.BrainID, "allocating", time.Now()); err != nil {
					return errRefused
				}
			}
			if err = e.mark("brain-allocated"); err != nil {
				return err
			}
			p := provision.Provisioner{Store: e.store, BrainsRoot: filepath.Join(e.target.DataDir, "brains")}
			if e.allowed(ctx) != nil {
				return errRefused
			}
			b, err := p.Provision(ctx, a.AccountID)
			if err != nil || b.ID != a.BrainID {
				return errRefused
			}
			if err = e.mark("brain-ready"); err != nil {
				return err
			}
			a.Pending = ""
			if err = e.persist(ctx, m); err != nil {
				return err
			}
		}
		if mode == "reissue" || a.Pending == "key" || a.Key == "" {
			a.Key = ""
			a.Pending = "key"
			if err = e.persist(ctx, m); err != nil {
				return err
			}
			if err = e.revoke(ctx, a); err != nil {
				return err
			}
			issuer := credential.Issuer{Store: e.store}
			if e.allowed(ctx) != nil {
				return errRefused
			}
			key, err := issuer.Issue(ctx, a.BrainID, a.AccountID, []string{"memory:read", "memory:write"})
			if err != nil {
				return errRefused
			}
			if err = e.mark("key-issued"); err != nil {
				return err
			}
			binding, err := issuer.Verify(ctx, key)
			if err != nil {
				return errRefused
			}
			a.CredentialIDs = append(a.CredentialIDs, binding.CredentialID)
			a.Key = key
			a.Pending = ""
			if err = e.persist(ctx, m); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *engine) capacity(ctx context.Context, additional int) error {
	var n int
	// Conservative: also count partner accounts. Never raise the configured cap.
	if err := e.store.DB().QueryRowContext(ctx, `SELECT count(*) FROM accounts WHERE status='active'`).Scan(&n); err != nil || n+additional > e.target.AccountCap {
		return errRefused
	}
	return nil
}

func (e *engine) inspect(ctx context.Context, a *fixtureAccount) error {
	var id, email, status, plan string
	var version int
	var foreign bool
	err := e.store.DB().QueryRowContext(ctx, `SELECT id,email,status,plan_id,plan_version,(stripe_customer_id IS NOT NULL OR created_by_partner IS NOT NULL) FROM accounts WHERE email_hash=?`, store.Hash(a.Email)).Scan(&id, &email, &status, &plan, &version, &foreign)
	if errors.Is(err, sql.ErrNoRows) {
		if a.AccountID != "" || a.BrainID != "" || len(a.CredentialIDs) > 0 || a.Key != "" {
			return errRefused
		}
		return nil
	}
	if err != nil || email != a.Email || status != "active" || plan != "free" || version != 1 || foreign {
		return errRefused
	}
	if a.AccountID == "" {
		// A durable account intent predates CreateAccount. This recovery assumes the
		// same exclusive offline fence; it is NOT sufficient live authority alone.
		if a.Pending != "account" {
			return errRefused
		}
		a.AccountID = id
	} else if a.AccountID != id {
		return errRefused
	}
	for _, q := range []string{
		`SELECT count(*) FROM sessions WHERE account_id=?`,
		`SELECT count(*) FROM workos_identities WHERE account_id=?`,
		`SELECT count(*) FROM workos_logins WHERE account_id=?`,
		`SELECT count(*) FROM subscriptions WHERE account_id=?`,
		`SELECT count(*) FROM checkout_attempts WHERE account_id=?`,
		`SELECT count(*) FROM partner_links WHERE account_id=?`,
		`SELECT count(*) FROM partner_entitlements WHERE account_id=?`,
		`SELECT count(*) FROM link_requests WHERE account_id=?`,
		`SELECT count(*) FROM oauth_grants WHERE json_extract(record,'$.subject')=?`,
		`SELECT count(*) FROM oauth_codes WHERE json_extract(record,'$.subject')=?`,
		`SELECT count(*) FROM oauth_consents WHERE json_extract(record,'$.subject')=?`,
	} {
		var n int
		if err = e.store.DB().QueryRowContext(ctx, q, id).Scan(&n); err != nil || n != 0 {
			return errRefused
		}
	}
	var logins int
	if err = e.store.DB().QueryRowContext(ctx, `SELECT count(*) FROM login_tokens WHERE email=?`, a.Email).Scan(&logins); err != nil || logins != 0 {
		return errRefused
	}
	var count int
	if err = e.store.DB().QueryRowContext(ctx, `SELECT count(*) FROM brains WHERE account_id=?`, id).Scan(&count); err != nil {
		return errRefused
	}
	if count == 0 {
		if a.BrainID != "" && a.Pending != "brain" {
			return errRefused
		}
	} else {
		if count != 1 || a.BrainID == "" {
			return errRefused
		}
		var path, state string
		var def int
		var deleted sql.NullString
		if err = e.store.DB().QueryRowContext(ctx, `SELECT path_key,state,is_default,deleted_at FROM brains WHERE id=? AND account_id=?`, a.BrainID, id).Scan(&path, &state, &def, &deleted); err != nil || path != a.BrainID || def != 1 || deleted.Valid || (state != "ready" && (state != "allocating" || a.Pending != "brain")) {
			return errRefused
		}
	}
	if a.BrainID != "" {
		path := filepath.Join(e.target.DataDir, "brains", a.BrainID)
		if _, err = os.Lstat(path); err == nil {
			if count == 0 {
				return errRefused
			}
			if err = privatefs.ValidateDirectory(ctx, path); err != nil {
				return errRefused
			}
			if err = checkTree(path); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) || a.Pending != "brain" {
			return errRefused
		}
	}
	rows, err := e.store.DB().QueryContext(ctx, `SELECT id,brain_id,scopes,generation,partner_id FROM client_credentials WHERE account_id=?`, id)
	if err != nil {
		return errRefused
	}
	var unknown []string
	seen := map[string]bool{}
	for _, cid := range a.CredentialIDs {
		if seen[cid] {
			if rows.Close() != nil {
				return errRefused
			}
			return errRefused
		}
		seen[cid] = true
	}
	known := 0
	for rows.Next() {
		var cid, bid, scopes string
		var generation int
		var partner sql.NullString
		if rows.Scan(&cid, &bid, &scopes, &generation, &partner) != nil || bid != a.BrainID || scopes != "memory:read,memory:write" || generation != 1 || partner.Valid {
			if rows.Close() != nil {
				return errRefused
			}
			return errRefused
		}
		if !slices.Contains(a.CredentialIDs, cid) {
			unknown = append(unknown, cid)
		} else {
			known++
		}
	}
	err = errors.Join(rows.Err(), rows.Close())
	if err != nil {
		return errRefused
	}
	if known != len(a.CredentialIDs) {
		return errRefused
	}
	if len(unknown) > 0 {
		if a.Pending != "key" || len(unknown) != 1 {
			return errRefused
		}
		// Exactly one interrupted Issue under the prior persisted intent. Revoke
		// before any replacement. This never broad-revokes a brain or OAuth epoch.
		a.CredentialIDs = append(a.CredentialIDs, unknown[0])
	}
	if a.Key != "" {
		issuer := credential.Issuer{Store: e.store}
		b, err := issuer.Verify(ctx, a.Key)
		if err != nil || b.AccountID != id || b.BrainID != a.BrainID || !slices.Contains(a.CredentialIDs, b.CredentialID) {
			return errRefused
		}
	}
	return nil
}

func (e *engine) revoke(ctx context.Context, a *fixtureAccount) error {
	issuer := credential.Issuer{Store: e.store}
	for _, id := range a.CredentialIDs {
		if e.allowed(ctx) != nil {
			return errRefused
		}
		if err := issuer.RevokeKey(ctx, id); err != nil {
			return errRefused
		}
	}
	return nil
}

// Reject symlinks, special files, foreign ownership and writable shared state
// before Provisioner/Git or the writer lock can follow anything in a brain.
func checkTree(path string) error {
	return filepath.WalkDir(path, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return errRefused
		}
		info, err := d.Info()
		if err != nil {
			return errRefused
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != uint32(os.Geteuid()) || info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) || info.Mode().Perm()&0022 != 0 || (!info.IsDir() && st.Nlink != 1) {
			return errRefused
		}
		return nil
	})
}
