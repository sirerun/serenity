//go:build linux

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func exampleProof() custodyProof {
	d := strings.Repeat("a", 64)
	return custodyProof{Version: 1, Purpose: "serenity-episode-fixture-offline-v1", Operation: "prepare", Nonce: d, IssuedAt: 1000, ExpiresAt: 1120, BootID: "00000000-0000-0000-0000-000000000001", UID: 1200, GID: 1200, ToolSource: strings.Repeat("b", 40), ToolPath: "/opt/tool", ToolSHA256: d, WrapperPath: "/opt/wrapper.py", WrapperSHA256: d, RuntimePath: "/var/lib/serenity-release/fixture/activation-runtime.py", RuntimeSHA256: d, ServiceBinaryPath: "/opt/serenity", ServiceBinarySHA256: d, ServiceSource: strings.Repeat("c", 40), ConfigPath: "/etc/serenity/hosted.json", ConfigSHA256: d, DataDir: "/data", PrivateDir: "/fixture", AccountCap: 10, HoldSHA256: d, StateSHA256: d, ReadySHA256: d, AuthorityReceiptPath: "/root/proof.json", AuthorityReceiptSHA256: d, AuthorityGeneratorPath: "/root/generator.py", AuthorityGeneratorSHA256: d, LockDevice: 1, LockInode: 2, SchemaSHA256: d, AuthorityScope: "synthetic-scope", AuthoritySessionID: "synthetic-session", ConcurrencyTarget: "synthetic-function", Hold: maintenanceHold{ManifestSHA256: d, ExecutableSHA256: d, Source: strings.Repeat("d", 40), PreviousStateSHA256: d, ExpectedStateSHA256: d, Supervisor: "serenity-release-fixture.service", Stage: "/var/lib/serenity-release/fixture", BackupTimerPriorState: "active", Phase: "maintenance-ready", Timestamp: 999}}
}

func TestProofBindingAndFreshness(t *testing.T) {
	base := exampleProof()
	if !base.valid(1001, base.ToolSource, 1200, 1200) {
		t.Fatal("valid synthetic proof rejected")
	}
	for name, mutate := range map[string]func(*custodyProof){
		"expired":        func(p *custodyProof) { p.ExpiresAt = 1001 },
		"future":         func(p *custodyProof) { p.IssuedAt = 1002 },
		"unbounded":      func(p *custodyProof) { p.ExpiresAt = 1400 },
		"wrong-source":   func(p *custodyProof) { p.ToolSource = strings.Repeat("e", 40) },
		"wrong-user":     func(p *custodyProof) { p.UID = 1201 },
		"wrong-group":    func(p *custodyProof) { p.GID = 1201 },
		"no-cap":         func(p *custodyProof) { p.AccountCap = 0 },
		"wrong-state":    func(p *custodyProof) { p.StateSHA256 = strings.Repeat("e", 64) },
		"wrong-phase":    func(p *custodyProof) { p.Hold.Phase = "failed" },
		"wrong-config":   func(p *custodyProof) { p.ConfigPath = "/other.json" },
		"wrong-runtime":  func(p *custodyProof) { p.RuntimePath = "/other.py" },
		"missing-schema": func(p *custodyProof) { p.SchemaSHA256 = "" },
		"wrong-path":     func(p *custodyProof) { p.DataDir = "/data/../other" },
	} {
		t.Run(name, func(t *testing.T) {
			p := base
			mutate(&p)
			if p.valid(1001, base.ToolSource, 1200, 1200) {
				t.Fatal("bad synthetic proof accepted")
			}
		})
	}
	if base.valid(1001, base.ToolSource, 0, 0) {
		t.Fatal("data-writing root process accepted")
	}
}

func TestCustodyArgumentsFailClosed(t *testing.T) {
	for _, args := range [][]string{nil, {"--maintenance=true"}, {"--custody-fds", "0,1,2,3,4,5"}, {"--custody-fds", "3,3,3,3,3,3"}, {"--custody-fds", "-1,4,5,6,7,8"}, {"--custody-fds", "x,4,5,6,7,8"}} {
		if _, err := newCustody(args); err == nil {
			t.Fatal("invalid custody accepted")
		}
	}
}

func TestInheritedDescriptorMustBePrivateRegularRootFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "proof")
	if err := os.WriteFile(path, []byte("synthetic"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.Close(); err != nil {
			t.Errorf("close synthetic descriptor: %v", err)
		}
	})
	if _, _, err := pinFD(int(f.Fd()), true); err == nil {
		t.Fatal("nonprivate descriptor accepted")
	}
	if err = os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	write, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := write.Close(); err != nil {
			t.Errorf("close synthetic descriptor: %v", err)
		}
	})
	if _, _, err = pinFD(int(write.Fd()), true); err == nil {
		t.Fatal("writable descriptor accepted")
	}
	if err = os.Link(path, filepath.Join(dir, "alias")); err != nil {
		t.Fatal(err)
	}
	if _, _, err = pinFD(int(f.Fd()), true); err == nil {
		t.Fatal("hardlinked descriptor accepted")
	}
}

func TestReadOnlySchemaPreflightDoesNotMigrate(t *testing.T) {
	e := fixture(t)
	ctx := context.Background()
	sha, err := schemaDigest(ctx, e.store.DB())
	if err != nil {
		t.Fatal(err)
	}
	p := exampleProof()
	p.DataDir = e.target.DataDir
	p.PrivateDir = e.privateDir
	p.SchemaSHA256 = sha
	// Real fresh interfaces create only synthetic test storage. Private modes are
	// set by the test owner; the production preflight never repairs permissions.
	for _, suffix := range []string{"", "-wal", "-shm"} {
		path := filepath.Join(p.DataDir, "control.db"+suffix)
		if _, err := os.Stat(path); err == nil {
			if err = os.Chmod(path, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = preflightDB(ctx, p); err != nil {
		t.Fatal("schema10 preflight rejected")
	}
	if _, err = e.store.DB().Exec(`DELETE FROM schema_migrations WHERE version=10`); err != nil {
		t.Fatal(err)
	}
	if preflightDB(ctx, p) == nil {
		t.Fatal("old schema accepted")
	}
	if count(t, e, `SELECT max(version) FROM schema_migrations`) != 9 {
		t.Fatal("preflight migrated database")
	}
}

func TestSchemaShapeAndDigestRefuseUnknownState(t *testing.T) {
	e := fixture(t)
	ctx := context.Background()
	first, err := schemaDigest(ctx, e.store.DB())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.store.CreateAccount(ctx, "synthetic@example.invalid"); err != nil {
		t.Fatal(err)
	}
	after, err := schemaDigest(ctx, e.store.DB())
	if err != nil || first != after {
		t.Fatal("schema digest depends on customer rows")
	}
	if _, err = e.store.DB().Exec(`DROP INDEX brains_account_state`); err != nil {
		t.Fatal(err)
	}
	if _, err = schemaDigest(ctx, e.store.DB()); err == nil {
		t.Fatal("missing schema object accepted")
	}
}

func TestSchemaRequiresExistingImmutableCheckoutTrigger(t *testing.T) {
	e := fixture(t)
	if _, err := schemaDigest(context.Background(), e.store.DB()); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.DB().Exec(`DROP TRIGGER checkout_attempt_request_immutable`); err != nil {
		t.Fatal(err)
	}
	if _, err := schemaDigest(context.Background(), e.store.DB()); err == nil {
		t.Fatal("missing immutability trigger admitted")
	}
}
