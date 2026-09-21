package store

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestLegacySchemasUpgradeTwiceAndPreserveControlData(t *testing.T) {
	for sourceVersion := 1; sourceVersion <= 3; sourceVersion++ {
		t.Run(fmt.Sprintf("schema%d", sourceVersion), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "control.db")
			prepareLegacyFixture(t, path, sourceVersion)
			factsPath := filepath.Join(filepath.Dir(path), "canonical", "brain-a", "facts.jsonl")
			factsBefore := databaseFileHash(t, factsPath)

			for pass := 1; pass <= 2; pass++ {
				s, err := Open(path)
				if err != nil {
					t.Fatalf("open pass %d: %v", pass, err)
				}
				assertRetainedControlData(t, s.db, sourceVersion)
				if got := databaseFileHash(t, factsPath); got != factsBefore {
					t.Fatalf("canonical fact store changed during control DB migration: before %x after %x", factsBefore, got)
				}
				var version, count int
				if err := s.db.QueryRow(`SELECT max(version),count(*) FROM schema_migrations`).Scan(&version, &count); err != nil {
					t.Fatal(err)
				}
				if version != 4 || count != 4 {
					t.Fatalf("migration versions max=%d count=%d, want 4/4", version, count)
				}
				if pass == 2 {
					if err := assertOperationSchema(s.db); err != nil {
						t.Fatal(err)
					}
					assertOperationConstraints(t, s.db)
				}
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestFreshAndUpgradedOperationSchemasAreEquivalent(t *testing.T) {
	fresh, err := Open(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fresh.Close(); err != nil {
			t.Error(err)
		}
	})
	legacyPath := filepath.Join(t.TempDir(), "legacy.db")
	prepareLegacyFixture(t, legacyPath, 1)
	upgraded, err := Open(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := upgraded.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, db := range []*sql.DB{fresh.db, upgraded.db} {
		if err := assertOperationSchema(db); err != nil {
			t.Fatal(err)
		}
	}
	freshShape, err := operationSchemaShape(fresh.db)
	if err != nil {
		t.Fatal(err)
	}
	upgradedShape, err := operationSchemaShape(upgraded.db)
	if err != nil {
		t.Fatal(err)
	}
	if freshShape != upgradedShape {
		t.Fatalf("fresh and upgraded operation schema differ:\nfresh: %s\nupgraded: %s", freshShape, upgradedShape)
	}
}

func TestFutureSchemaIsRejectedWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO schema_migrations(version,applied_at) VALUES(5,'future')`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	before := databaseFileHash(t, path)
	if opened, err := Open(path); err == nil {
		_ = opened.Close()
		t.Fatal("future schema opened")
	} else if got := err.Error(); got != "migrate hosted database: unsupported hosted schema version 5" {
		t.Fatalf("future schema error = %q", got)
	}
	if after := databaseFileHash(t, path); after != before {
		t.Fatalf("future schema rejection wrote database: before %x after %x", before, after)
	}
}

func prepareLegacyFixture(t *testing.T, path string, version int) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	factsPath := filepath.Join(filepath.Dir(path), "canonical", "brain-a", "facts.jsonl")
	if err := os.MkdirAll(filepath.Dir(factsPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(factsPath, []byte("fixture canonical fact\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO accounts(id,email_hash,email,created_at,status,plan_id,plan_version) VALUES('acct','hash','fixture@example.test','created','active','free',1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO subscriptions(id,account_id,price_id,status,current_period_start,current_period_end,cancel_at_period_end) VALUES('sub','acct','price','active','start','end',0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO login_tokens(id,email,token_hash,created_at,expires_at) VALUES('token','fixture@example.test','tokenhash','created','expires')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO usage_windows(account_id,window_key,metric,committed) VALUES('acct','2026-09','writes',7)`); err != nil {
		t.Fatal(err)
	}
	if version >= 2 {
		if _, err := db.Exec(migration2); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE subscriptions SET plan_id='pilot' WHERE id='sub'`); err != nil {
			t.Fatal(err)
		}
	}
	if version >= 3 {
		if _, err := db.Exec(migration3); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE subscriptions SET grace_until='grace' WHERE id='sub'`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE schema_migrations SET applied_at='fixture'`); err != nil {
		t.Fatal(err)
	}
}

func assertRetainedControlData(t *testing.T, db *sql.DB, sourceVersion int) {
	t.Helper()
	var accountID, priceID, status, planID, periodStart, periodEnd string
	var cancelAtPeriodEnd int
	var graceUntil sql.NullString
	if err := db.QueryRow(`SELECT account_id,price_id,status,plan_id,current_period_start,current_period_end,cancel_at_period_end,grace_until FROM subscriptions WHERE id='sub'`).Scan(&accountID, &priceID, &status, &planID, &periodStart, &periodEnd, &cancelAtPeriodEnd, &graceUntil); err != nil {
		t.Fatalf("read retained subscription: %v", err)
	}
	wantPlanID := "free"
	if sourceVersion >= 2 {
		wantPlanID = "pilot"
	}
	wantGrace := sql.NullString{}
	if sourceVersion >= 3 {
		wantGrace = sql.NullString{String: "grace", Valid: true}
	}
	if accountID != "acct" || priceID != "price" || status != "active" || planID != wantPlanID || periodStart != "start" || periodEnd != "end" || cancelAtPeriodEnd != 0 || graceUntil != wantGrace {
		t.Fatalf("subscription data changed: account=%q price=%q status=%q plan=%q start=%q end=%q cancel=%d grace=%+v", accountID, priceID, status, planID, periodStart, periodEnd, cancelAtPeriodEnd, graceUntil)
	}
	var tokenID string
	if err := db.QueryRow(`SELECT id FROM login_tokens WHERE token_hash='tokenhash'`).Scan(&tokenID); err != nil || tokenID != "token" {
		t.Fatalf("login token=%q err=%v", tokenID, err)
	}
	var committed int
	if err := db.QueryRow(`SELECT committed FROM usage_windows WHERE account_id='acct' AND metric='writes'`).Scan(&committed); err != nil || committed != 7 {
		t.Fatalf("usage=%d err=%v", committed, err)
	}
}

func assertOperationSchema(db *sql.DB) error {
	var columns int
	if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info('operations')`).Scan(&columns); err != nil {
		return err
	}
	if columns != 15 {
		return fmt.Errorf("operations column count=%d, want 15", columns)
	}
	var index int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='index' AND name IN ('operations_active_client_key','operations_expired_reserved','operations_pending_review')`).Scan(&index); err != nil {
		return err
	}
	if index != 3 {
		return fmt.Errorf("operation index count=%d, want 3", index)
	}
	return nil
}

func operationSchemaShape(db *sql.DB) (string, error) {
	var shape strings.Builder
	rows, err := db.Query(`SELECT cid,name,type,"notnull",coalesce(dflt_value,''),pk FROM pragma_table_info('operations') ORDER BY cid`)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, kind, defaultValue string
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return "", err
		}
		fmt.Fprintf(&shape, "column:%d:%s:%s:%d:%s:%d\n", cid, name, kind, notNull, defaultValue, primaryKey)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return "", err
	}
	if err := rows.Close(); err != nil {
		return "", err
	}
	rows, err = db.Query(`SELECT name,coalesce(sql,'') FROM sqlite_master WHERE type='index' AND tbl_name='operations' ORDER BY name`)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var name, definition string
		if err := rows.Scan(&name, &definition); err != nil {
			return "", err
		}
		fmt.Fprintf(&shape, "index:%s:%s\n", name, definition)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return "", err
	}
	if err := rows.Close(); err != nil {
		return "", err
	}
	return shape.String(), nil
}

func assertOperationConstraints(t *testing.T, db *sql.DB) {
	t.Helper()
	const brainID = "brainfixture123456"
	if _, err := db.Exec(`INSERT INTO brains(id,account_id,state,path_key,created_at) VALUES(?,'acct','ready',?,'created')`, brainID, brainID); err != nil {
		t.Fatal(err)
	}
	const insert = `INSERT INTO operations(id,account_id,brain_id,client_key,fingerprint,deltas_json,quota_period,phase,source,lease_expires_at,created_at) VALUES(?,?,?,?,?,?,'2026-09','reserved','gateway.remember','lease','created')`
	args := func(id, key, fingerprint, deltas string) []any {
		return []any{id, "acct", brainID, key, fingerprint, deltas}
	}
	if _, err := db.Exec(insert, args("op1", "key1", "fingerprint", `[{"metric":"writes","units":1}]`)...); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(insert, args("op2", "key1", "fingerprint", `[{"metric":"writes","units":1}]`)...); err == nil {
		t.Fatal("duplicate active client key was accepted")
	}
	if _, err := db.Exec(insert, args("opbad", "keybad", "fingerprint", `not-json`)...); err == nil {
		t.Fatal("non-JSON deltas were accepted")
	}
	if _, err := db.Exec(`UPDATE operations SET phase='unknown' WHERE id='op1'`); err == nil {
		t.Fatal("unknown operation phase was accepted")
	}
	if _, err := db.Exec(`UPDATE operations SET phase='released' WHERE id='op1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(insert, args("op2", "key1", "fingerprint", `[{"metric":"writes","units":2}]`)...); err != nil {
		t.Fatalf("released key was not freed: %v", err)
	}
	if _, err := db.Exec(insert, args("op3", "", "", `[{"metric":"writes","units":1}]`)...); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(insert, args("op4", "", "", `[{"metric":"writes","units":1}]`)...); err != nil {
		t.Fatalf("unkeyed operations were not independent: %v", err)
	}
}

func databaseFileHash(t *testing.T, path string) [32]byte {
	t.Helper()
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(bytes)
}
