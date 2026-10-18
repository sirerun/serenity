// Synthetic Linux UID/FD storage verifier. Never linked into the operator child.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/store"
)

const schemaSHA = "9bd9153fd18f433eb1d0416c7faa38ca6eefdc29ebcd8585f3a021b8b63ed963"

type account struct {
	Email, AccountID, BrainID, Key, Pending string
	CredentialIDs                           []string
}
type manifest struct {
	Version  int
	Accounts []account
}

func require(ok bool) {
	if !ok {
		panic("synthetic helper check failed")
	}
}
func must(err error) { require(err == nil) }

func readManifest(path string) manifest {
	info, err := os.Lstat(path)
	must(err)
	st, ok := info.Sys().(*syscall.Stat_t)
	require(ok && info.Mode().IsRegular() && info.Mode().Perm() == 0600 && st.Uid == 1800 && st.Nlink == 1)
	b, err := os.ReadFile(path)
	must(err)
	var m manifest
	must(json.Unmarshal(b, &m))
	require(m.Version == 2 && len(m.Accounts) == 2)
	return m
}

func execute() {
	require(os.Getuid() == 1800 && os.Geteuid() == 1800 && os.Getgid() == 1800 && os.Getegid() == 1800)
	groups, err := os.Getgroups()
	must(err)
	require(len(groups) == 0)
	require(len(os.Args) == 4)
	mode, root, private := os.Args[1], os.Args[2], os.Args[3]
	require(filepath.IsAbs(root) && filepath.IsAbs(private))
	dbPath := filepath.Join(root, "control.db")
	if mode == "init" {
		_, err := os.Lstat(dbPath)
		require(errors.Is(err, os.ErrNotExist))
	}
	s, err := store.Open(dbPath)
	must(err)
	defer func() { must(s.Close()) }()
	db := s.DB()
	ctx := context.Background()
	count := func(query string, args ...any) int {
		var n int
		must(db.QueryRowContext(ctx, query, args...).Scan(&n))
		return n
	}
	switch mode {
	case "init":
		require(store.SchemaVersion == 10 && count("SELECT count(*) FROM schema_migrations") == 10)
		require(count("SELECT min(version) FROM schema_migrations") == 1 && count("SELECT max(version) FROM schema_migrations") == 10)
		require(count("SELECT count(*) FROM accounts") == 0)
		rows, err := db.QueryContext(ctx, `SELECT type,name,tbl_name,coalesce(sql,'') FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY type,name`)
		must(err)
		h := sha256.New()
		for rows.Next() {
			var fields [4]string
			must(rows.Scan(&fields[0], &fields[1], &fields[2], &fields[3]))
			for _, field := range fields {
				_, err = h.Write(append([]byte(field), 0))
				must(err)
			}
		}
		must(rows.Err())
		must(rows.Close())
		require(hex.EncodeToString(h.Sum(nil)) == schemaSHA)
	case "future-schema":
		// Corrupt only this already initialized synthetic DB; never invent migrations.
		_, err = db.ExecContext(ctx, "UPDATE schema_migrations SET version=11 WHERE version=10")
		must(err)
	case "extra-schema-object":
		_, err = db.ExecContext(ctx, "CREATE TABLE supplemental_unknown_object (value TEXT)")
		must(err)
	default:
		require(mode == "active" || mode == "reissued" || mode == "revoked" || mode == "externally-revoke" || mode == "retained-revoked")
		m := readManifest(filepath.Join(private, "manifest.json"))
		original := readManifest(filepath.Join(root, "original-manifest.json"))
		require(count("SELECT count(*) FROM accounts") == 2)
		require(count("SELECT count(*) FROM brains") == 2)
		var credentialCount int
		for _, a := range m.Accounts {
			credentialCount += len(a.CredentialIDs)
		}
		require(credentialCount >= 2 && credentialCount <= 4)
		require(count("SELECT count(*) FROM client_credentials") == credentialCount)
		issuer := credential.Issuer{Store: s}
		for i, a := range m.Accounts {
			o := original.Accounts[i]
			require(a.AccountID == o.AccountID && a.BrainID == o.BrainID && a.Email == o.Email && a.Pending == "")
			require((len(a.CredentialIDs) == 1 || len(a.CredentialIDs) == 2) && len(o.CredentialIDs) == 1 && a.CredentialIDs[0] == o.CredentialIDs[0] && o.Key != "")
			if len(a.CredentialIDs) == 1 {
				require(slices.Equal(a.CredentialIDs, o.CredentialIDs))
			}
			require(count("SELECT count(*) FROM accounts WHERE id=? AND status='active' AND plan_id='free' AND plan_version=1", a.AccountID) == 1)
			require(count("SELECT count(*) FROM brains WHERE id=? AND account_id=? AND state='ready' AND is_default=1 AND deleted_at IS NULL", a.BrainID, a.AccountID) == 1)
			info, err := os.Stat(filepath.Join(root, "brains", a.BrainID, ".git"))
			must(err)
			require(info.IsDir())
			binding, err := issuer.Verify(ctx, o.Key)
			switch mode {
			case "active", "externally-revoke":
				must(err)
				require(a.Key == o.Key && binding.AccountID == a.AccountID && binding.BrainID == a.BrainID && binding.CredentialID == a.CredentialIDs[0])
				if mode == "externally-revoke" {
					// Synthetic helper only: model the established externally revoked
					// custody precondition; the operational child still cannot bypass it.
					require(len(a.CredentialIDs) == 1)
					must(issuer.RevokeKey(ctx, a.CredentialIDs[0]))
					_, revokedErr := issuer.Verify(ctx, o.Key)
					require(errors.Is(revokedErr, credential.ErrRevoked))
				}
			case "retained-revoked":
				require(a.Key == o.Key && errors.Is(err, credential.ErrRevoked))
			case "reissued":
				require(len(a.CredentialIDs) == 2 && a.Key != "" && a.Key != o.Key && errors.Is(err, credential.ErrRevoked))
				current, verifyErr := issuer.Verify(ctx, a.Key)
				must(verifyErr)
				require(current.AccountID == a.AccountID && current.BrainID == a.BrainID && current.CredentialID == a.CredentialIDs[1])
			default:
				require(a.Key == "" && errors.Is(err, credential.ErrRevoked))
			}
		}
		switch mode {
		case "active":
			require(count("SELECT count(*) FROM client_credentials WHERE revoked_at IS NULL") == 2)
		case "reissued":
			require(count("SELECT count(*) FROM client_credentials WHERE revoked_at IS NULL") == 2)
			require(count("SELECT count(*) FROM client_credentials WHERE revoked_at IS NOT NULL") == 2)
		default:
			require(count("SELECT count(*) FROM client_credentials WHERE revoked_at IS NOT NULL") == credentialCount)
		}
	}
	// Start each refusal test without WAL/SHM sidecars, so a byte-and-metadata
	// snapshot catches any writes, including unexpected sidecar creation.
	if mode == "init" || mode == "future-schema" || mode == "extra-schema-object" {
		var journal string
		must(db.QueryRowContext(ctx, "PRAGMA journal_mode=DELETE").Scan(&journal))
		require(journal == "delete")
	}
}

func main() {
	// Never format recovered errors, SQL rows, manifests, or credentials.
	defer func() {
		if recover() != nil {
			fmt.Fprintln(os.Stderr, "synthetic helper failed")
			os.Exit(1)
		}
	}()
	execute()
	fmt.Println(`{"uid":1800,"gid":1800,"supplementary_groups":[],"check":"passed"}`)
}
