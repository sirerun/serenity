//go:build linux

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sirerun/serenity/internal/hosted/privatefs"
)

type fixtureSnapshot struct {
	FixtureScopeSHA256     string `json:"fixture_scope_sha256"`
	PrivateInventorySHA256 string `json:"private_inventory_sha256"`
	SchemaSHA256           string `json:"schema_sha256"`
	Version                int    `json:"version"`
}

// snapshotCLI is separate from every custody mutation. Digests are observations,
// never permission, adoption or proof that a live maintenance fence exists.
func snapshotCLI(args []string, out io.Writer) int {
	groups, err := os.Getgroups()
	if len(args) != 5 || os.Geteuid() == 0 || err != nil || len(groups) != 0 {
		return 1
	}
	cap, err := strconv.Atoi(args[4])
	if err != nil || cap <= 0 || cap > 100000 {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := readFixtureSnapshot(ctx, args[1], args[2], args[3], cap)
	if err != nil {
		return 1
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return 1
	}
	if _, err = out.Write(raw); err != nil {
		return 1
	}
	return 0
}

func snapshotPrivate(ctx context.Context, path string) ([]byte, []byte, error) {
	if privatefs.ValidateDirectory(ctx, path) != nil {
		return nil, nil, errRefused
	}
	d, err := os.Open(path)
	if err != nil {
		return nil, nil, errRefused
	}
	entries, readErr := d.ReadDir(3)
	closeErr := d.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) || closeErr != nil || len(entries) > 2 {
		return nil, nil, errRefused
	}
	names := []string{}
	var manifestRaw []byte
	for _, entry := range entries {
		name := entry.Name()
		if name != "manifest.json" && name != "fixture.lock" {
			return nil, nil, errRefused
		}
		st, err := os.Lstat(filepath.Join(path, name))
		if err != nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 {
			return nil, nil, errRefused
		}
		sys, ok := st.Sys().(*syscall.Stat_t)
		if !ok || sys.Nlink != 1 || int(sys.Uid) != os.Geteuid() {
			return nil, nil, errRefused
		}
		raw, err := privatefs.ReadFile(ctx, path, name, 65536)
		if err != nil {
			return nil, nil, errRefused
		}
		if name == "manifest.json" {
			manifestRaw = raw
		} else if len(raw) != 0 {
			return nil, nil, errRefused
		}
		names = append(names, name+":"+hashBytes(raw)+":"+strconv.FormatUint(sys.Ino, 10)+":"+strconv.FormatUint(uint64(sys.Dev), 10))
	}
	if len(entries) != 0 && (len(entries) != 2 || manifestRaw == nil) {
		return nil, nil, errRefused
	}
	slices.Sort(names)
	raw, _ := json.Marshal(names)
	return raw, manifestRaw, nil
}

func readFixtureSnapshot(ctx context.Context, dataDir, privateDir, configHash string, cap int) (result fixtureSnapshot, resultErr error) {
	if !hexDigest.MatchString(configHash) || cap <= 0 || cap > 100000 || !filepath.IsAbs(dataDir) || filepath.Clean(dataDir) != dataDir || !filepath.IsAbs(privateDir) || filepath.Clean(privateDir) != privateDir || privateDir == dataDir || strings.HasPrefix(privateDir, dataDir+string(os.PathSeparator)) {
		return result, errRefused
	}
	if privatefs.ValidateDirectory(ctx, dataDir) != nil || privatefs.ValidateDirectory(ctx, filepath.Join(dataDir, "brains")) != nil {
		return result, errRefused
	}
	before, raw, err := snapshotPrivate(ctx, privateDir)
	if err != nil {
		return result, errRefused
	}
	if raw != nil {
		lock, lockErr := os.OpenFile(filepath.Join(privateDir, "fixture.lock"), os.O_RDONLY|syscall.O_NOFOLLOW, 0)
		if lockErr != nil {
			return result, errRefused
		}
		defer func() { refuseCleanup(&resultErr, lock.Close()) }()
		if syscall.Flock(int(lock.Fd()), syscall.LOCK_SH|syscall.LOCK_NB) != nil {
			return result, errRefused
		}
		defer func() { refuseCleanup(&resultErr, syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)) }()
		locked, lockedRaw, lockErr := snapshotPrivate(ctx, privateDir)
		if lockErr != nil || !bytes.Equal(before, locked) || !bytes.Equal(raw, lockedRaw) {
			return result, errRefused
		}
	}
	dbpath := filepath.Join(dataDir, "control.db")
	for _, name := range []string{"control.db", "control.db-wal", "control.db-shm"} {
		info, statErr := os.Lstat(filepath.Join(dataDir, name))
		if errors.Is(statErr, os.ErrNotExist) && name != "control.db" {
			continue
		}
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return result, errRefused
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Nlink != 1 || int(st.Uid) != os.Geteuid() {
			return result, errRefused
		}
	}
	dbBefore, err := os.Lstat(dbpath)
	if err != nil {
		return result, errRefused
	}
	u := url.URL{Scheme: "file", Path: dbpath}
	db, err := sql.Open("sqlite", u.String()+"?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(1000)")
	if err != nil {
		return result, errRefused
	}
	defer func() { refuseCleanup(&resultErr, db.Close()) }()
	db.SetMaxOpenConns(1)
	if _, err = db.ExecContext(ctx, "PRAGMA query_only=ON"); err != nil {
		return result, errRefused
	}
	if _, err = db.ExecContext(ctx, "BEGIN"); err != nil {
		return result, errRefused
	}
	defer func() { _, e := db.ExecContext(context.Background(), "ROLLBACK"); refuseCleanup(&resultErr, e) }()
	var n, min, max int
	if db.QueryRowContext(ctx, "SELECT count(*),min(version),max(version) FROM schema_migrations").Scan(&n, &min, &max) != nil || n != 10 || min != 1 || max != 10 {
		return result, errRefused
	}
	schema, err := schemaDigest(ctx, db)
	if err != nil || schema != "9bd9153fd18f433eb1d0416c7faa38ca6eefdc29ebcd8585f3a021b8b63ed963" {
		return result, errRefused
	}
	fk, err := db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return result, errRefused
	}
	bad := fk.Next()
	err = errors.Join(fk.Err(), fk.Close())
	if bad || err != nil {
		return result, errRefused
	}
	scope := []any{dataDir, privateDir, configHash, cap}
	if raw == nil {
		if db.QueryRowContext(ctx, "SELECT count(*) FROM accounts WHERE lower(email) LIKE '%@serenity-episode-fixture.invalid'").Scan(&n) != nil || n != 0 {
			return result, errRefused
		}
		scope = append(scope, "known-empty-reserved-fixture-domain")
	} else {
		var m manifest
		if uniqueManifestJSON(raw) != nil || strictDecode(raw, &m) != nil || m.Version != 2 || !generatedID.MatchString(m.Namespace) || !sourceID.MatchString(m.Target.Source) || m.Target.DataDir != dataDir || m.Target.ConfigSHA256 != configHash || m.Target.AccountCap != cap || len(m.Accounts) != 2 {
			return result, errRefused
		}
		if m.Accounts[0].AccountID == m.Accounts[1].AccountID || m.Accounts[0].BrainID == m.Accounts[1].BrainID {
			return result, errRefused
		}
		scope = append(scope, hashBytes(raw))
		for i, a := range m.Accounts {
			if a.Email != email(m.Namespace, i) || !generatedID.MatchString(a.AccountID) || !generatedID.MatchString(a.BrainID) || a.Pending != "" || len(a.CredentialIDs) == 0 || len(a.CredentialIDs) > 128 {
				return result, errRefused
			}
			var status, plan, emailValue string
			var version int
			var foreign bool
			if db.QueryRowContext(ctx, "SELECT email,status,plan_id,plan_version,(stripe_customer_id IS NOT NULL OR created_by_partner IS NOT NULL) FROM accounts WHERE id=? AND email_hash=?", a.AccountID, hashBytes([]byte(a.Email))).Scan(&emailValue, &status, &plan, &version, &foreign) != nil || emailValue != a.Email || status != "active" || plan != "free" || version != 1 || foreign {
				return result, errRefused
			}
			var state, path string
			var def int
			var deleted sql.NullString
			if db.QueryRowContext(ctx, "SELECT state,path_key,is_default,deleted_at FROM brains WHERE id=? AND account_id=?", a.BrainID, a.AccountID).Scan(&state, &path, &def, &deleted) != nil || state != "ready" || path != a.BrainID || def != 1 || deleted.Valid {
				return result, errRefused
			}
			if db.QueryRowContext(ctx, "SELECT count(*) FROM brains WHERE account_id=?", a.AccountID).Scan(&n) != nil || n != 1 {
				return result, errRefused
			}
			for _, table := range []string{"sessions", "workos_identities", "workos_logins", "subscriptions", "checkout_attempts", "partner_links", "partner_entitlements", "link_requests"} {
				if db.QueryRowContext(ctx, "SELECT count(*) FROM "+table+" WHERE account_id=?", a.AccountID).Scan(&n) != nil || n != 0 {
					return result, errRefused
				}
			}
			for _, table := range []string{"oauth_grants", "oauth_codes", "oauth_consents"} {
				if db.QueryRowContext(ctx, "SELECT count(*) FROM "+table+" WHERE json_extract(record,'$.subject')=?", a.AccountID).Scan(&n) != nil || n != 0 {
					return result, errRefused
				}
			}
			if db.QueryRowContext(ctx, "SELECT count(*) FROM login_tokens WHERE email=?", a.Email).Scan(&n) != nil || n != 0 {
				return result, errRefused
			}
			for _, q := range []string{"SELECT * FROM accounts WHERE id=? ORDER BY id", "SELECT * FROM brains WHERE account_id=? ORDER BY id", "SELECT * FROM client_credentials WHERE account_id=? ORDER BY id"} {
				rows, err := snapshotRows(ctx, db, q, a.AccountID)
				if err != nil {
					return result, errRefused
				}
				scope = append(scope, rows)
			}
			rows, err := db.QueryContext(ctx, "SELECT id,brain_id,scopes,generation,partner_id FROM client_credentials WHERE account_id=? ORDER BY id", a.AccountID)
			if err != nil {
				return result, errRefused
			}
			seen := map[string]bool{}
			for rows.Next() {
				var id, bid, scopes string
				var generation int
				var partner sql.NullString
				if rows.Scan(&id, &bid, &scopes, &generation, &partner) != nil || !generatedID.MatchString(id) || seen[id] || !slices.Contains(a.CredentialIDs, id) || bid != a.BrainID || scopes != "memory:read,memory:write" || generation != 1 || partner.Valid {
					_ = rows.Close()
					return result, errRefused
				}
				seen[id] = true
				if len(seen) > 128 {
					_ = rows.Close()
					return result, errRefused
				}
			}
			if errors.Join(rows.Err(), rows.Close()) != nil || len(seen) != len(a.CredentialIDs) {
				return result, errRefused
			}
		}
	}
	after, afterRaw, err := snapshotPrivate(ctx, privateDir)
	if err != nil || !bytes.Equal(before, after) || !bytes.Equal(raw, afterRaw) {
		return result, errRefused
	}
	dbAfter, statErr := os.Lstat(dbpath)
	if statErr != nil || !os.SameFile(dbBefore, dbAfter) {
		return result, errRefused
	}
	scopeRaw, err := json.Marshal(scope)
	if err != nil {
		return result, errRefused
	}
	return fixtureSnapshot{FixtureScopeSHA256: hashBytes(scopeRaw), PrivateInventorySHA256: hashBytes(before), SchemaSHA256: schema, Version: 1}, nil
}

func snapshotRows(ctx context.Context, db *sql.DB, q string, id string) (result []any, resultErr error) {
	rows, err := db.QueryContext(ctx, q, id)
	if err != nil {
		return nil, errRefused
	}
	defer func() { refuseCleanup(&resultErr, rows.Close()) }()
	columns, err := rows.Columns()
	if err != nil || len(columns) > 64 {
		return nil, errRefused
	}
	result = append(result, columns)
	for rows.Next() {
		if len(result) > 128 {
			return nil, errRefused
		}
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if rows.Scan(pointers...) != nil {
			return nil, errRefused
		}
		for _, v := range values {
			switch x := v.(type) {
			case string:
				if len(x) > 65536 {
					return nil, errRefused
				}
			case []byte:
				if len(x) > 65536 {
					return nil, errRefused
				}
			}
		}
		result = append(result, values)
	}
	if rows.Err() != nil {
		return nil, errRefused
	}
	return result, nil
}
