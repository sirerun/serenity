//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/privatefs"
	"github.com/sirerun/serenity/internal/hosted/provision"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func fixture(t *testing.T) *engine {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{root, filepath.Join(root, "data"), filepath.Join(root, "data", "brains"), filepath.Join(root, "private")} {
		if err = os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.Chmod(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	db, err := store.Open(filepath.Join(root, "data", "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return &engine{store: db, privateDir: filepath.Join(root, "private"), target: target{DataDir: filepath.Join(root, "data"), ConfigPath: filepath.Join(root, "config.json"), ConfigSHA256: strings.Repeat("a", 64), Source: strings.Repeat("b", 40), AccountCap: 10}}
}

func mustOperate(t *testing.T, e *engine, mode string) *manifest {
	t.Helper()
	if err := e.operate(context.Background(), mode); err != nil {
		t.Fatal(err)
	}
	m, err := e.load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return m
}
func count(t *testing.T, e *engine, q string, args ...any) int {
	t.Helper()
	var n int
	if err := e.store.DB().QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func save(t *testing.T, e *engine, m *manifest) {
	t.Helper()
	if err := e.persist(context.Background(), m); err != nil {
		t.Fatal(err)
	}
}
func refuse(t *testing.T, e *engine, mode string) {
	t.Helper()
	if err := e.operate(context.Background(), mode); !errors.Is(err, errRefused) {
		t.Fatalf("expected content-free refusal, got %v", err)
	}
}

func TestPrepareReplayReissueRevoke(t *testing.T) {
	e := fixture(t)
	m := mustOperate(t, e, "prepare")
	if count(t, e, `SELECT count(*) FROM accounts`) != 2 || count(t, e, `SELECT count(*) FROM brains WHERE is_default=1 AND state='ready'`) != 2 || count(t, e, `SELECT count(*) FROM accounts WHERE plan_id='free' AND plan_version=1`) != 2 {
		t.Fatal("wrong fixture cardinality or plan")
	}
	if count(t, e, `SELECT count(*) FROM sessions`)+count(t, e, `SELECT count(*) FROM login_tokens`)+count(t, e, `SELECT count(*) FROM subscriptions`) != 0 {
		t.Fatal("unexpected identity/billing side effects")
	}
	issuer := credential.Issuer{Store: e.store}
	for _, a := range m.Accounts {
		b, err := issuer.Verify(context.Background(), a.Key)
		if err != nil || b.AccountID != a.AccountID || b.BrainID != a.BrainID || strings.Join(b.Scopes, ",") != "memory:read,memory:write" {
			t.Fatal("wrong credential binding")
		}
	}
	replay := mustOperate(t, e, "prepare")
	for i, a := range m.Accounts {
		if replay.Accounts[i].Key != a.Key || replay.Accounts[i].AccountID != a.AccountID || replay.Accounts[i].BrainID != a.BrainID {
			t.Fatal("replay changed fixture")
		}
	}
	rotated := mustOperate(t, e, "reissue")
	for i, a := range m.Accounts {
		if _, err := issuer.Verify(context.Background(), a.Key); !errors.Is(err, credential.ErrRevoked) {
			t.Fatal("old key still valid")
		}
		if rotated.Accounts[i].Key == a.Key {
			t.Fatal("key not replaced")
		}
	}
	mustOperate(t, e, "revoke")
	mustOperate(t, e, "revoke")
	if count(t, e, `SELECT count(*) FROM client_credentials WHERE revoked_at IS NULL`) != 0 || count(t, e, `SELECT count(*) FROM accounts`) != 2 || count(t, e, `SELECT count(*) FROM brains`) != 2 {
		t.Fatal("revoke must retain inert accounts and brains")
	}
	for _, a := range rotated.Accounts {
		if _, err := os.Stat(filepath.Join(e.target.DataDir, "brains", a.BrainID, ".git")); err != nil {
			t.Fatal("brain data removed")
		}
	}
}

func TestInterruptionRecovery(t *testing.T) {
	for _, stage := range []string{"account-created", "brain-allocated", "brain-ready", "key-issued"} {
		t.Run(stage, func(t *testing.T) {
			e := fixture(t)
			interrupted := errors.New("injected interruption")
			e.checkpoint = func(at string) error {
				if at == stage {
					return interrupted
				}
				return nil
			}
			if err := e.operate(context.Background(), "prepare"); !errors.Is(err, interrupted) {
				t.Fatal("checkpoint not reached")
			}
			before, err := e.load(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			e.checkpoint = nil
			after := mustOperate(t, e, "prepare")
			if count(t, e, `SELECT count(*) FROM accounts`) != 2 || count(t, e, `SELECT count(*) FROM brains`) != 2 || count(t, e, `SELECT count(*) FROM client_credentials WHERE revoked_at IS NULL`) != 2 {
				t.Fatal("recovery duplicated state")
			}
			if before.Accounts[0].BrainID != "" && after.Accounts[0].BrainID != before.Accounts[0].BrainID {
				t.Fatal("brain allocation changed")
			}
			if stage == "key-issued" && count(t, e, `SELECT count(*) FROM client_credentials WHERE revoked_at IS NOT NULL`) != 1 {
				t.Fatal("orphan key was not revoked")
			}
		})
	}
}

func TestForeignAndHumanAdoptionRefused(t *testing.T) {
	for _, kind := range []string{"email-collision", "account-id", "foreign-credential-id", "duplicate-credential-id", "unknown-key", "session", "paid", "email-changed", "extra-brain"} {
		t.Run(kind, func(t *testing.T) {
			e := fixture(t)
			m := mustOperate(t, e, "prepare")
			a := m.Accounts[0]
			switch kind {
			case "email-collision":
				m.Accounts[0].AccountID = ""
				m.Accounts[0].BrainID = ""
				m.Accounts[0].CredentialIDs = nil
				m.Accounts[0].Key = ""
				save(t, e, m)
			case "account-id":
				m.Accounts[0].AccountID = store.ID()
				save(t, e, m)
			case "foreign-credential-id":
				m.Accounts[0].CredentialIDs = append(m.Accounts[0].CredentialIDs, m.Accounts[1].CredentialIDs[0])
				save(t, e, m)
			case "duplicate-credential-id":
				m.Accounts[0].CredentialIDs = append(m.Accounts[0].CredentialIDs, m.Accounts[0].CredentialIDs[0])
				save(t, e, m)
			case "unknown-key":
				issuer := credential.Issuer{Store: e.store}
				if _, err := issuer.Issue(context.Background(), a.BrainID, a.AccountID, []string{"memory:read", "memory:write"}); err != nil {
					t.Fatal(err)
				}
			case "session":
				if _, err := e.store.DB().Exec(`INSERT INTO sessions VALUES(?,?,?,?,?,?)`, store.ID(), a.AccountID, "synthetic-hash", "now", "later", "synthetic-csrf"); err != nil {
					t.Fatal(err)
				}
			case "paid":
				if _, err := e.store.DB().Exec(`UPDATE accounts SET plan_id='builder' WHERE id=?`, a.AccountID); err != nil {
					t.Fatal(err)
				}
			case "email-changed":
				if _, err := e.store.DB().Exec(`UPDATE accounts SET email='human@example.invalid' WHERE id=?`, a.AccountID); err != nil {
					t.Fatal(err)
				}
			case "extra-brain":
				id := store.ID()
				if _, err := e.store.DB().Exec(`INSERT INTO brains(id,account_id,state,is_default,path_key,created_at) VALUES(?,?,'allocating',0,?,'now')`, id, a.AccountID, id); err != nil {
					t.Fatal(err)
				}
			}
			live := count(t, e, `SELECT count(*) FROM client_credentials WHERE revoked_at IS NULL`)
			refuse(t, e, "revoke")
			if count(t, e, `SELECT count(*) FROM client_credentials WHERE revoked_at IS NULL`) != live {
				t.Fatal("refusal revoked a key")
			}
		})
	}
}

func TestCapRefusalAndUnrelatedRowsUnchanged(t *testing.T) {
	e := fixture(t)
	other, err := e.store.CreateAccount(context.Background(), "unrelated@example.invalid")
	if err != nil {
		t.Fatal(err)
	}
	e.target.AccountCap = 2
	refuse(t, e, "prepare")
	if count(t, e, `SELECT count(*) FROM accounts`) != 1 {
		t.Fatal("cap refusal partially provisioned")
	}
	p := provision.Provisioner{Store: e.store, BrainsRoot: filepath.Join(e.target.DataDir, "brains")}
	otherBrain, err := p.Provision(context.Background(), other.ID)
	if err != nil {
		t.Fatal(err)
	}
	issuer := credential.Issuer{Store: e.store}
	otherKey, err := issuer.Issue(context.Background(), otherBrain.ID, other.ID, []string{"memory:read"})
	if err != nil {
		t.Fatal(err)
	}
	e.target.AccountCap = 3
	mustOperate(t, e, "prepare")
	mustOperate(t, e, "reissue")
	mustOperate(t, e, "revoke")
	if count(t, e, `SELECT count(*) FROM accounts WHERE id=? AND email=? AND status='active' AND plan_id='free' AND plan_version=1`, other.ID, other.Email) != 1 || count(t, e, `SELECT count(*) FROM brains WHERE account_id=?`, other.ID) != 1 {
		t.Fatal("unrelated fixture changed")
	}
	if _, err := issuer.Verify(context.Background(), otherKey); err != nil {
		t.Fatal("unrelated credential changed")
	}
}

func TestPrivatePathsAndTargetBinding(t *testing.T) {
	for _, kind := range []string{"manifest-mode", "manifest-symlink", "directory-mode", "brain-symlink", "source", "config", "database", "schema"} {
		t.Run(kind, func(t *testing.T) {
			e := fixture(t)
			m := mustOperate(t, e, "prepare")
			path := filepath.Join(e.privateDir, "manifest.json")
			switch kind {
			case "manifest-mode":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "manifest-symlink":
				if err := os.Rename(path, path+".saved"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".saved", path); err != nil {
					t.Fatal(err)
				}
			case "directory-mode":
				if err := os.Chmod(e.privateDir, 0755); err != nil {
					t.Fatal(err)
				}
			case "brain-symlink":
				if err := os.Symlink(e.privateDir, filepath.Join(e.target.DataDir, "brains", m.Accounts[0].BrainID, "foreign")); err != nil {
					t.Fatal(err)
				}
			case "source":
				e.target.Source = strings.Repeat("c", 40)
			case "config":
				e.target.ConfigSHA256 = strings.Repeat("d", 64)
			case "database":
				old := e.target.DataDir
				e.target.DataDir = filepath.Join(filepath.Dir(old), "different")
				if err := os.MkdirAll(filepath.Join(e.target.DataDir, "brains"), 0700); err != nil {
					t.Fatal(err)
				}
			case "schema":
				if _, err := e.store.DB().Exec(`DELETE FROM schema_migrations WHERE version=10`); err != nil {
					t.Fatal(err)
				}
			}
			refuse(t, e, "prepare")
		})
	}
	e := fixture(t)
	mustOperate(t, e, "prepare")
	if _, err := privatefs.ReadFile(context.Background(), e.privateDir, "manifest.json", 65536); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(e.privateDir, "manifest.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("manifest not private")
	}
}

func TestMainRefusesMissingCustodyWithoutSecretOutput(t *testing.T) {
	// Missing custody cannot open a target or accept a maintenance boolean.
	// This is not live guard qualification.
	var out bytes.Buffer
	if run(nil, &out) != 1 || out.String() != "REFUSED: fixture custody or operation failed; retain hold for reconciliation\n" {
		t.Fatal("main did not fail closed")
	}
	if strings.Contains(out.String(), "sk_live_") {
		t.Fatal("secret-shaped output")
	}
}

func TestRevokeInterruptionAndPartialPreparation(t *testing.T) {
	e := fixture(t)
	mustOperate(t, e, "prepare")
	interrupted := errors.New("injected interruption")
	e.checkpoint = func(stage string) error {
		if stage == "keys-revoked" {
			return interrupted
		}
		return nil
	}
	if err := e.operate(context.Background(), "revoke"); !errors.Is(err, interrupted) {
		t.Fatal("revoke checkpoint not reached")
	}
	e.checkpoint = nil
	mustOperate(t, e, "revoke")
	mustOperate(t, e, "prepare")
	if count(t, e, `SELECT count(*) FROM client_credentials WHERE revoked_at IS NULL`) != 2 {
		t.Fatal("recovery did not reissue exactly two keys")
	}
	for _, stage := range []string{"account-created", "brain-allocated"} {
		t.Run(stage, func(t *testing.T) {
			e := fixture(t)
			e.checkpoint = func(at string) error {
				if at == stage {
					return interrupted
				}
				return nil
			}
			if err := e.operate(context.Background(), "prepare"); !errors.Is(err, interrupted) {
				t.Fatal("preparation checkpoint not reached")
			}
			e.checkpoint = nil
			mustOperate(t, e, "revoke")
			mustOperate(t, e, "prepare")
			if count(t, e, `SELECT count(*) FROM accounts`) != 2 || count(t, e, `SELECT count(*) FROM brains`) != 2 {
				t.Fatal("partial revoke broke replay")
			}
		})
	}
}

func TestMissingManifestNeverAdoptsReservedIdentity(t *testing.T) {
	e := fixture(t)
	if _, err := e.store.CreateAccount(context.Background(), email(store.ID(), 0)); err != nil {
		t.Fatal(err)
	}
	refuse(t, e, "prepare")
	if count(t, e, `SELECT count(*) FROM accounts`) != 1 {
		t.Fatal("foreign reserved account adopted or duplicated")
	}
}

func TestPendingBrainDirectoryCollisionRefused(t *testing.T) {
	e := fixture(t)
	m := &manifest{Version: 2, Namespace: store.ID(), Target: e.target, Accounts: make([]fixtureAccount, 2)}
	for i := range m.Accounts {
		m.Accounts[i].Email = email(m.Namespace, i)
	}
	a, err := e.store.CreateAccount(context.Background(), m.Accounts[0].Email)
	if err != nil {
		t.Fatal(err)
	}
	m.Accounts[0].AccountID = a.ID
	m.Accounts[0].BrainID = store.ID()
	m.Accounts[0].Pending = "brain"
	if err = os.Mkdir(filepath.Join(e.target.DataDir, "brains", m.Accounts[0].BrainID), 0700); err != nil {
		t.Fatal(err)
	}
	save(t, e, m)
	refuse(t, e, "prepare")
	if count(t, e, `SELECT count(*) FROM brains`) != 0 {
		t.Fatal("foreign directory adopted")
	}
}

func TestCustodyLossPreventsNextMutation(t *testing.T) {
	e := fixture(t)
	allowed := true
	e.authorize = func() error {
		if !allowed {
			return errRefused
		}
		return nil
	}
	e.checkpoint = func(stage string) error {
		if stage == "account-created" {
			allowed = false
		}
		return nil
	}
	refuse(t, e, "prepare")
	if count(t, e, `SELECT count(*) FROM accounts`) != 1 || count(t, e, `SELECT count(*) FROM brains`) != 0 || count(t, e, `SELECT count(*) FROM client_credentials`) != 0 {
		t.Fatal("continued mutation after custody loss")
	}
	// Recovery only after a new validated custody interval; intent predates row.
	allowed = true
	e.checkpoint = nil
	mustOperate(t, e, "prepare")
}

func TestInterruptedOwnershipRequiresSameMaintenanceHold(t *testing.T) {
	e := fixture(t)
	e.custodyEpoch = "synthetic-hold-one"
	interrupted := errors.New("interrupted")
	e.checkpoint = func(stage string) error {
		if stage == "key-issued" {
			return interrupted
		}
		return nil
	}
	if err := e.operate(context.Background(), "prepare"); !errors.Is(err, interrupted) {
		t.Fatal("checkpoint not reached")
	}
	e.checkpoint = nil
	e.custodyEpoch = "synthetic-hold-two"
	refuse(t, e, "prepare")
	if count(t, e, `SELECT count(*) FROM client_credentials WHERE revoked_at IS NULL`) != 1 {
		t.Fatal("foreign epoch mutated orphan")
	}
	e.custodyEpoch = "synthetic-hold-one"
	mustOperate(t, e, "prepare")
	mustOperate(t, e, "revoke")
	// Fully bound, inert identities can be reused under a new reviewed hold.
	e.custodyEpoch = "synthetic-hold-two"
	mustOperate(t, e, "prepare")
}
