package index

import (
	"context"
	"fmt"
)

// ClaimStateCounts groups the claims table by its state column (RFC
// section 16's weekly report card: "claims by state"). A brain with no
// claims yet returns an empty, non-nil map -- callers (internal/report)
// render that as a legitimate all-zero report rather than treating it as
// an error.
func (s *SQLite) ClaimStateCounts(ctx context.Context) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT state, COUNT(*) FROM claims GROUP BY state`)
	if err != nil {
		return nil, fmt.Errorf("index: claim state counts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]int64{}
	for rows.Next() {
		var state string
		var n int64
		if err := rows.Scan(&state, &n); err != nil {
			return nil, fmt.Errorf("index: claim state counts: %w", err)
		}
		out[state] = n
	}
	return out, rows.Err()
}
