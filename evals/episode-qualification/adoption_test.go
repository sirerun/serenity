//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func adoptionFixture(t *testing.T) (*engine, *manifest, sourceAdoption) {
	t.Helper()
	e := fixture(t)
	e.custodyEpoch = strings.Repeat("1", 64)
	m := mustOperate(t, e, "prepare")
	externallyRevokeFixture(t, e, m)
	binding := sourceAdoption{OldSource: e.target.Source, OldManifestSHA256: manifestHash(t, e)}
	e.target.Source = strings.Repeat("c", 40)
	e.custodyEpoch = strings.Repeat("2", 64)
	return e, m, binding
}

func manifestBytes(t *testing.T, e *engine) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(e.privateDir, "manifest.json"))
	if err != nil {
		t.Fatal("read synthetic manifest")
	}
	return data
}

func manifestHash(t *testing.T, e *engine) string {
	t.Helper()
	return store.Hash(string(manifestBytes(t, e)))
}

// Hash every synthetic control-table row, not merely credential cardinalities.
// Never print rows, verifiers, retained secrets or their hashes on failure.
func controlSnapshot(t *testing.T, e *engine) [32]byte {
	t.Helper()
	rows, err := e.store.DB().Query(`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	for _, name := range names {
		rows, err := e.store.DB().Query(`SELECT * FROM "` + strings.ReplaceAll(name, `"`, `""`) + `" ORDER BY rowid`)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fmt.Fprintln(h, name)
		for rows.Next() {
			values, args := make([]any, len(columns)), make([]any, len(columns))
			for i := range values {
				args[i] = &values[i]
			}
			if err := rows.Scan(args...); err != nil {
				t.Fatal(err)
			}
			if err := json.NewEncoder(h).Encode(values); err != nil {
				t.Fatal(err)
			}
		}
		if err := errors.Join(rows.Err(), rows.Close()); err != nil {
			t.Fatal(err)
		}
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

func assertManifestUnwritten(t *testing.T, e *engine, before []byte, info os.FileInfo) {
	t.Helper()
	after, err := os.Stat(filepath.Join(e.privateDir, "manifest.json"))
	if err != nil || !os.SameFile(info, after) || !info.ModTime().Equal(after.ModTime()) || !bytes.Equal(before, manifestBytes(t, e)) {
		t.Fatal("refusal rewrote private manifest")
	}
}

func TestSourceAdoptionChangesOnlySourceAndCustodyEpoch(t *testing.T) {
	e, original, binding := adoptionFixture(t)
	// Multiple historical credentials must all survive, inert, in exact order.
	e.target.Source = binding.OldSource
	rotated := mustOperate(t, e, "reissue")
	externallyRevokeFixture(t, e, rotated)
	original = rotated
	e.target.Source = strings.Repeat("c", 40)
	e.custodyEpoch = strings.Repeat("3", 64)
	binding.OldManifestSHA256 = manifestHash(t, e)
	if _, err := e.store.CreateAccount(context.Background(), "unrelated@example.invalid"); err != nil {
		t.Fatal(err)
	}
	// Exercise the engine on a write-disabled connection as well as checking
	// all rows/total_changes. Production uses the stricter offline constructor.
	if _, err := e.store.DB().Exec(`PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	before := controlSnapshot(t, e)
	changes := count(t, e, `SELECT total_changes()`)
	if err := e.adoptSource(context.Background(), binding); err != nil {
		t.Fatal(err)
	}
	after, err := e.load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	original.Target.Source = e.target.Source
	original.CustodyEpoch = e.custodyEpoch
	if !reflect.DeepEqual(original, after) {
		t.Fatal("adoption changed fields beyond source and custody epoch")
	}
	if before != controlSnapshot(t, e) || changes != count(t, e, `SELECT total_changes()`) {
		t.Fatal("adoption changed control database")
	}
	issuer := credential.Issuer{Store: e.store}
	for _, a := range after.Accounts {
		if _, err := issuer.Verify(context.Background(), a.Key); !errors.Is(err, credential.ErrRevoked) {
			t.Fatal("adoption made retained key authenticatable")
		}
	}
	path := filepath.Join(e.privateDir, "manifest.json")
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("manifest is not private")
	}
	committed := manifestBytes(t, e)
	// A fresh root packet with the same stale old hash is not idempotence.
	if err := e.adoptSource(context.Background(), binding); !errors.Is(err, errAdoptionManifestChanged) {
		t.Fatal("stale old hash did not receive precise refusal")
	}
	assertManifestUnwritten(t, e, committed, info)
	if before != controlSnapshot(t, e) || changes != count(t, e, `SELECT total_changes()`) {
		t.Fatal("retry changed database")
	}
}

func TestSourceAdoptionRefusalsDoNotWrite(t *testing.T) {
	for _, damage := range []string{
		"wrong-hash", "wrong-old-source", "already-current", "wrong-config-hash", "wrong-config-path", "wrong-cap", "wrong-data-dir",
		"live-key", "historical-live-key", "wrong-secret", "swapped-key", "unrecorded-key", "duplicate-id", "foreign-id",
		"pending", "missing-account", "missing-brain", "missing-key", "missing-inventory", "wrong-email", "extra-reserved", "paid", "billed", "billing-failure", "partner-account", "partner-key", "human-session", "ambiguous-brain", "invalid-revocation", "changed-custody", "schema-incomplete",
	} {
		t.Run(damage, func(t *testing.T) {
			e, m, binding := adoptionFixture(t)
			exec := func(q string, args ...any) {
				t.Helper()
				if _, err := e.store.DB().Exec(q, args...); err != nil {
					t.Fatal(err)
				}
			}
			switch damage {
			case "wrong-old-source":
				binding.OldSource = strings.Repeat("d", 40)
			case "already-current":
				m.Target.Source = e.target.Source
			case "wrong-config-hash":
				m.Target.ConfigSHA256 = strings.Repeat("f", 64)
			case "wrong-config-path":
				m.Target.ConfigPath = "/other/config.json"
			case "wrong-cap":
				m.Target.AccountCap++
			case "wrong-data-dir":
				m.Target.DataDir = "/other/data"
			case "live-key":
				exec(`UPDATE client_credentials SET revoked_at=NULL WHERE id=?`, m.Accounts[1].CredentialIDs[0])
			case "historical-live-key", "unrecorded-key":
				issuer := credential.Issuer{Store: e.store}
				key, err := issuer.Issue(context.Background(), m.Accounts[0].BrainID, m.Accounts[0].AccountID, []string{"memory:read", "memory:write"})
				if err != nil {
					t.Fatal(err)
				}
				b, err := issuer.Verify(context.Background(), key)
				if err != nil {
					t.Fatal(err)
				}
				if damage == "historical-live-key" {
					m.Accounts[0].CredentialIDs = append(m.Accounts[0].CredentialIDs, b.CredentialID)
				} else if err := issuer.RevokeKey(context.Background(), b.CredentialID); err != nil {
					t.Fatal(err)
				}
			case "wrong-secret":
				key := m.Accounts[0].Key
				replacement := "A"
				if key[17] == 'A' {
					replacement = "B"
				}
				m.Accounts[0].Key = key[:17] + replacement + key[18:]
			case "swapped-key":
				m.Accounts[0].Key = m.Accounts[1].Key
			case "duplicate-id":
				m.Accounts[0].CredentialIDs = append(m.Accounts[0].CredentialIDs, m.Accounts[0].CredentialIDs[0])
			case "foreign-id":
				m.Accounts[0].CredentialIDs = append(m.Accounts[0].CredentialIDs, m.Accounts[1].CredentialIDs[0])
			case "pending":
				m.Accounts[1].Pending = "key"
				m.Accounts[1].Key = ""
			case "missing-account":
				m.Accounts[1].AccountID = ""
			case "missing-brain":
				m.Accounts[1].BrainID = ""
			case "missing-key":
				m.Accounts[1].Key = ""
			case "missing-inventory":
				m.Accounts[1].CredentialIDs = nil
			case "wrong-email":
				m.Accounts[1].Email = "foreign@example.invalid"
			case "extra-reserved":
				if _, err := e.store.CreateAccount(context.Background(), email(store.ID(), 0)); err != nil {
					t.Fatal(err)
				}
			case "billing-failure":
				exec(`INSERT INTO billing_failures VALUES(?,?,?,?,?)`, m.Accounts[1].AccountID, "synthetic-subscription", "synthetic-invoice", "synthetic-event", "now")
			case "partner-account":
				exec(`UPDATE accounts SET created_by_partner='synthetic-partner' WHERE id=?`, m.Accounts[1].AccountID)
			case "partner-key":
				exec(`UPDATE client_credentials SET partner_id='synthetic-partner' WHERE id=?`, m.Accounts[1].CredentialIDs[0])
			case "paid":
				exec(`UPDATE accounts SET plan_id='builder' WHERE id=?`, m.Accounts[1].AccountID)
			case "billed":
				exec(`UPDATE accounts SET stripe_customer_id='synthetic-customer' WHERE id=?`, m.Accounts[1].AccountID)
			case "human-session":
				exec(`INSERT INTO sessions VALUES(?,?,?,?,?,?)`, store.ID(), m.Accounts[1].AccountID, "synthetic-hash", "now", "later", "synthetic-csrf")
			case "ambiguous-brain":
				id := store.ID()
				exec(`INSERT INTO brains(id,account_id,state,is_default,path_key,created_at) VALUES(?,?,'allocating',0,?,'now')`, id, m.Accounts[1].AccountID, id)
			case "invalid-revocation":
				exec(`UPDATE client_credentials SET revoked_at='uncertain' WHERE id=?`, m.Accounts[1].CredentialIDs[0])
			case "schema-incomplete":
				exec(`DELETE FROM schema_migrations WHERE version=10`)
			}
			save(t, e, m)
			binding.OldManifestSHA256 = manifestHash(t, e)
			if damage == "wrong-hash" {
				binding.OldManifestSHA256 = strings.Repeat("0", 64)
			}
			if damage == "changed-custody" {
				e.authorize = func() error { return errRefused }
			}
			before := controlSnapshot(t, e)
			changes := count(t, e, `SELECT total_changes()`)
			data := manifestBytes(t, e)
			info, err := os.Stat(filepath.Join(e.privateDir, "manifest.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := e.adoptSource(context.Background(), binding); err == nil {
				t.Fatal("unsafe adoption accepted")
			}
			assertManifestUnwritten(t, e, data, info)
			if before != controlSnapshot(t, e) || changes != count(t, e, `SELECT total_changes()`) {
				t.Fatal("refused adoption changed database")
			}
		})
	}
}

func TestOrdinaryOperationsNeverAdoptSource(t *testing.T) {
	for _, mode := range []string{"prepare", "reissue", "revoke", "adopt-source"} {
		t.Run(mode, func(t *testing.T) {
			e, _, _ := adoptionFixture(t)
			before, data := controlSnapshot(t, e), manifestBytes(t, e)
			refuse(t, e, mode)
			if before != controlSnapshot(t, e) || !bytes.Equal(data, manifestBytes(t, e)) {
				t.Fatal("ordinary operation migrated or mutated retained fixture")
			}
		})
	}
}

func TestSourceAdoptionInterruptionAndRetry(t *testing.T) {
	for _, stage := range []string{"adoption-before-publication", "adoption-after-publication", "adoption-after-durable-publication"} {
		t.Run(stage, func(t *testing.T) {
			e, _, binding := adoptionFixture(t)
			before, data := controlSnapshot(t, e), manifestBytes(t, e)
			changes := count(t, e, `SELECT total_changes()`)
			e.checkpoint = func(at string) error {
				if at == stage {
					return errors.New("synthetic interruption")
				}
				return nil
			}
			want := errAdoptionNotPublished
			if stage != "adoption-before-publication" {
				want = errAdoptionPublicationUncertain
			}
			if err := e.adoptSource(context.Background(), binding); !errors.Is(err, want) {
				t.Fatal("interruption reported incorrect publication state")
			}
			e.checkpoint = nil
			if stage == "adoption-before-publication" {
				if !bytes.Equal(data, manifestBytes(t, e)) {
					t.Fatal("pre-publication interruption changed manifest")
				}
				if err := e.adoptSource(context.Background(), binding); err != nil {
					t.Fatal("fresh-custody retry failed before prior publication")
				}
			} else {
				committed := manifestBytes(t, e)
				info, err := os.Stat(filepath.Join(e.privateDir, "manifest.json"))
				if err != nil {
					t.Fatal(err)
				}
				if err := e.adoptSource(context.Background(), binding); !errors.Is(err, errAdoptionManifestChanged) {
					t.Fatal("post-publication retry was not precisely refused")
				}
				assertManifestUnwritten(t, e, committed, info)
			}
			if before != controlSnapshot(t, e) || changes != count(t, e, `SELECT total_changes()`) {
				t.Fatal("interruption/retry changed database")
			}
			entries, err := os.ReadDir(e.privateDir)
			if err != nil || len(entries) != 2 {
				t.Fatal("unexpected private output after handled interruption")
			}
		})
	}
}

func TestAdoptionJSONRejectsAmbiguity(t *testing.T) {
	for _, data := range []string{`{"Source":1,"Source":2}`, `{"Source":1,"source":2}`, `{"a":[{"Key":1,"Key":2}]}`, `{} {}`, `{"a":`} {
		if uniqueManifestJSON([]byte(data)) == nil {
			t.Fatal("ambiguous JSON accepted")
		}
	}
	if uniqueManifestJSON([]byte(`{"Target":{"Source":"old"},"Accounts":[{"Key":"synthetic"}]}`)) != nil {
		t.Fatal("unique JSON refused")
	}
}

func TestAdoptionPrivateCustodyAndLockRefusals(t *testing.T) {
	for _, damage := range []string{"missing-lock", "locked", "public-manifest", "hardlink", "symlink", "oversized", "duplicate-source", "changed-before-publication", "lost-custody-before-publication"} {
		t.Run(damage, func(t *testing.T) {
			e, _, binding := adoptionFixture(t)
			path := filepath.Join(e.privateDir, "manifest.json")
			lockPath := filepath.Join(e.privateDir, "fixture.lock")
			var err error
			switch damage {
			case "missing-lock":
				err = os.Remove(lockPath)
			case "locked":
				lock, openErr := os.OpenFile(lockPath, os.O_RDWR, 0)
				if openErr != nil {
					t.Fatal(openErr)
				}
				t.Cleanup(func() {
					if err := lock.Close(); err != nil {
						t.Error(err)
					}
				})
				err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
			case "public-manifest":
				err = os.Chmod(path, 0644)
			case "hardlink":
				err = os.Link(path, path+".alias")
			case "symlink":
				if err = os.Rename(path, path+".saved"); err != nil {
					t.Fatal(err)
				}
				err = os.Symlink(path+".saved", path)
			case "oversized":
				err = os.WriteFile(path, append(manifestBytes(t, e), bytes.Repeat([]byte(" "), 65536)...), 0600)
				binding.OldManifestSHA256 = manifestHash(t, e)
			case "duplicate-source":
				data := bytes.Replace(manifestBytes(t, e), []byte(`"Source":`), []byte(`"Source":"`+binding.OldSource+`","Source":`), 1)
				err = os.WriteFile(path, data, 0600)
				binding.OldManifestSHA256 = manifestHash(t, e)
			}
			if err != nil {
				t.Fatal(err)
			}
			before, data := controlSnapshot(t, e), manifestBytes(t, e)
			if damage == "changed-before-publication" || damage == "lost-custody-before-publication" {
				e.checkpoint = func(stage string) error {
					if stage == "adoption-before-publication" {
						if damage == "changed-before-publication" {
							// Simulate a changed file despite the lock; the engine
							// must not overwrite it using stale root authority.
							data = append(data, '\n')
							if err := os.WriteFile(path, data, 0600); err != nil {
								t.Fatal(err)
							}
						} else {
							e.authorize = func() error { return errRefused }
						}
					}
					return nil
				}
			}
			if err := e.adoptSource(context.Background(), binding); err == nil {
				t.Fatal("unsafe private custody accepted")
			}
			if before != controlSnapshot(t, e) || !bytes.Equal(data, manifestBytes(t, e)) {
				t.Fatal("unsafe custody refusal changed state")
			}
			if damage == "missing-lock" {
				if _, err := os.Lstat(lockPath); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("adoption created missing lock")
				}
			}
		})
	}
}

func TestSourceAdoptionOutcomeContract(t *testing.T) {
	for code, outcome := range map[int]error{2: errAdoptionRefused, 3: errAdoptionManifestChanged, 5: errAdoptionNotPublished, 6: errAdoptionPublicationUncertain} {
		actual, message := adoptionOutcome(errors.Join(outcome, errors.New("synthetic cleanup")))
		if actual != code || message != outcome.Error() {
			t.Fatal("adoption result lost precise publication status")
		}
	}
}
