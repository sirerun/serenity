package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// sourcePathsSchema maps a file-backed source's path_hash meta value to the
// absolute path it was read from (PRIV-03, T24.24). Committed source
// records carry only a root-relative uri plus the hash; the absolute path
// lives here, in the local derived index that is never committed or
// pushed. It is runtime-only state repopulated by the next sync, so a
// rebuilt or deleted index loses nothing canonical.
const sourcePathsSchema = `CREATE TABLE IF NOT EXISTS source_paths(
			path_hash TEXT PRIMARY KEY,
			abs_path TEXT NOT NULL)`

// RecordSourcePath stores (or refreshes) the absolute path for pathHash.
func (s *SQLite) RecordSourcePath(ctx context.Context, pathHash, absPath string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO source_paths(path_hash, abs_path) VALUES(?, ?)
		ON CONFLICT(path_hash) DO UPDATE SET abs_path = excluded.abs_path`, pathHash, absPath)
	if err != nil {
		return fmt.Errorf("index: record source path: %w", err)
	}
	return nil
}

// SourcePath returns the absolute path recorded for pathHash; ok is false
// when this machine's index has no entry (for example, a source synced on
// another machine).
func (s *SQLite) SourcePath(ctx context.Context, pathHash string) (absPath string, ok bool, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT abs_path FROM source_paths WHERE path_hash = ?`, pathHash).Scan(&absPath)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("index: source path: %w", err)
	}
	return absPath, true, nil
}
