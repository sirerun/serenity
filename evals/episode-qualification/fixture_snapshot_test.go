//go:build linux

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFixtureSnapshotKnownEmptyAndStableWAL(t *testing.T) {
	e := fixture(t)
	first, err := readFixtureSnapshot(context.Background(), e.target.DataDir, e.privateDir, e.target.ConfigSHA256, e.target.AccountCap)
	if err != nil {
		t.Fatal("empty snapshot refused")
	}
	second, err := readFixtureSnapshot(context.Background(), e.target.DataDir, e.privateDir, e.target.ConfigSHA256, e.target.AccountCap)
	if err != nil || first != second {
		t.Fatal("empty snapshot unstable")
	}
	mustOperate(t, e, "prepare")
	prepared, err := readFixtureSnapshot(context.Background(), e.target.DataDir, e.privateDir, e.target.ConfigSHA256, e.target.AccountCap)
	if err != nil {
		t.Fatal("prepared snapshot refused")
	}
	again, err := readFixtureSnapshot(context.Background(), e.target.DataDir, e.privateDir, e.target.ConfigSHA256, e.target.AccountCap)
	if err != nil || prepared != again || prepared == first {
		t.Fatal("prepared snapshot unstable")
	}
	mustOperate(t, e, "revoke")
	revoked, err := readFixtureSnapshot(context.Background(), e.target.DataDir, e.privateDir, e.target.ConfigSHA256, e.target.AccountCap)
	if err != nil || revoked.FixtureScopeSHA256 == prepared.FixtureScopeSHA256 {
		t.Fatal("revocation not reflected")
	}
}

func TestFixtureSnapshotUnknownAndPartialRefuse(t *testing.T) {
	for _, kind := range []string{"extra", "pending", "unknown-credential", "config", "cancel", "reserved-empty"} {
		t.Run(kind, func(t *testing.T) {
			e := fixture(t)
			if kind != "reserved-empty" {
				mustOperate(t, e, "prepare")
			}
			ctx := context.Background()
			config := e.target.ConfigSHA256
			switch kind {
			case "extra":
				if os.WriteFile(filepath.Join(e.privateDir, "unknown"), []byte("synthetic"), 0600) != nil {
					t.Fatal("fixture")
				}
			case "pending":
				m, err := e.load(ctx)
				if err != nil {
					t.Fatal(err)
				}
				m.Accounts[0].Pending = "revoke"
				save(t, e, m)
			case "unknown-credential":
				if _, err := e.store.DB().Exec("UPDATE client_credentials SET scopes='foreign'"); err != nil {
					t.Fatal(err)
				}
			case "config":
				config = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "reserved-empty":
				if _, err := e.store.CreateAccount(ctx, "foreign@serenity-episode-fixture.invalid"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := readFixtureSnapshot(ctx, e.target.DataDir, e.privateDir, config, e.target.AccountCap); err == nil {
				t.Fatal("unsafe snapshot accepted")
			}
		})
	}
}

func TestFixtureSnapshotReservedDomainExactCohort(t *testing.T) {
	for _, extraEmail := range []string{"third@SERENITY-EPISODE-FIXTURE.INVALID", "not-episode-prefix@serenity-episode-fixture.invalid"} {
		t.Run(extraEmail, func(t *testing.T) {
			e := fixture(t)
			mustOperate(t, e, "prepare")
			baseline, err := readFixtureSnapshot(context.Background(), e.target.DataDir, e.privateDir, e.target.ConfigSHA256, e.target.AccountCap)
			if err != nil {
				t.Fatal("baseline refused")
			}
			if _, err = e.store.CreateAccount(context.Background(), "unrelated@example.invalid"); err != nil {
				t.Fatal(err)
			}
			unrelated, err := readFixtureSnapshot(context.Background(), e.target.DataDir, e.privateDir, e.target.ConfigSHA256, e.target.AccountCap)
			if err != nil || unrelated != baseline {
				t.Fatal("unrelated account affected scoped snapshot")
			}
			if _, err = e.store.CreateAccount(context.Background(), extraEmail); err != nil {
				t.Fatal(err)
			}
			for attempt := 0; attempt < 2; attempt++ {
				if _, err = readFixtureSnapshot(context.Background(), e.target.DataDir, e.privateDir, e.target.ConfigSHA256, e.target.AccountCap); err == nil {
					t.Fatal("extra reserved account accepted before/between observations")
				}
			}
		})
	}
}
