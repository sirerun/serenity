//go:build darwin || linux

package store

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"syscall"

	"github.com/sirerun/serenity/internal/hosted/privatefs"
)

// OpenOfflineReadOnly opens an existing, fully checkpointed control database
// without initialization, migration, WAL creation or writes to that database. The
// caller MUST hold an exclusive offline maintenance fence throughout its use.
// This function does not acquire that authority and must never read a live DB:
// SQLite immutable mode deliberately does not observe concurrent changes.
func OpenOfflineReadOnly(ctx context.Context, path string) (*Store, error) {
	refused := errors.New("hosted store: offline read-only database refused")
	if ctx == nil || ctx.Err() != nil || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, refused
	}
	inspect := func() (os.FileInfo, error) {
		if privatefs.ValidateDirectory(ctx, filepath.Dir(path)) != nil {
			return nil, refused
		}
		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			if _, err := os.Lstat(path + suffix); !errors.Is(err, os.ErrNotExist) {
				return nil, refused
			}
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			return nil, refused
		}
		st, ok := info.Sys().(*syscall.Stat_t)
		if !ok || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 {
			return nil, refused
		}
		return info, nil
	}
	before, err := inspect()
	if err != nil {
		return nil, refused
	}
	u := url.URL{Scheme: "file", Path: path}
	db, err := sql.Open("sqlite", u.String()+"?mode=ro&immutable=1&_pragma=query_only(1)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, refused
	}
	db.SetMaxOpenConns(1)
	accepted := false
	defer func() {
		if !accepted {
			_ = db.Close()
		}
	}()
	var count, min, max int
	if db.QueryRowContext(ctx, `SELECT count(*),min(version),max(version) FROM schema_migrations`).Scan(&count, &min, &max) != nil || count != SchemaVersion || min != 1 || max != SchemaVersion {
		return nil, refused
	}
	after, err := inspect()
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) || ctx.Err() != nil {
		return nil, refused
	}
	accepted = true
	return &Store{db: db}, nil
}
