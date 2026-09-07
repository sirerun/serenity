package index

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// OpenReport opens an existing index for read-only observation. Unlike Open,
// it does not create a directory, database, or run migrations. A missing index
// requires an explicit sync, not an apparently healthy empty report.
func OpenReport(path string) (*SQLite, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("report index unavailable (run serenity sync): %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("report index is not a regular file")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: absolute}
	query := url.Values{"mode": {"ro"}, "_pragma": {"query_only(1)", "busy_timeout(5000)"}}
	u.RawQuery = query.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &SQLite{db: db, clock: realClock{}}, nil
}
