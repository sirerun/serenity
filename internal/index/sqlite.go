// Package index implements the derived, rebuildable database (RFC §7.5).
// It is NEVER canonical: there are no backups, and the wipe-and-rebuild
// invariant test proves it reconstructs identically from repo bytes within
// the pinned model set. Runtime-only state (queues, caches) that may live
// DB-only is enumerated in an allowlist as it arrives in later milestones.
package index

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/sirerun/serenity/internal/domain"
)

// SQLite is the default engine: embedded, pure Go, FTS5 for lexical
// search. Postgres+pgvector is the documented scale profile, not the
// default; the Engine interface keeps it pluggable.
type SQLite struct {
	db *sql.DB
}

// Engine is the pluggable derived-index boundary (§7.5 BrainIndex).
type Engine interface {
	UpsertEntity(ctx context.Context, e domain.Entity) error
	UpsertClaim(ctx context.Context, c domain.Claim) error
	InsertChunk(ctx context.Context, chunkRef, entitySlug, text string) error
	SearchFTS(ctx context.Context, query string, limit int) ([]Hit, error)
	ResetAll(ctx context.Context) error
	Stats(ctx context.Context) (map[string]int64, error)
	Dump(ctx context.Context, w io.Writer) error
	Close() error
}

var _ Engine = (*SQLite)(nil)

// RuntimeTables lists tables that hold runtime-only state — queues,
// caches, and ledgers whose only source of truth is this database, not
// the canonical repo (RFC §7 preamble: "Runtime-only state ... is DB-only
// by design, enumerated in an allowlist exactly as gbrain's
// system-of-record doc does"). Rebuild and ResetAll must never wipe a
// table on this list. Each entry gets its real schema when the milestone
// that owns it lands (T1.1 jobs, M2 disposition_items/disposition_history,
// T1.7 spend_ledger); until then they're schema shells so the allowlist
// can be proven before any consumer exists.
var RuntimeTables = []string{
	"jobs",
	"disposition_items",
	"disposition_history",
	"spend_ledger",
	"caches",
}

// Hit is one search result.
type Hit struct {
	ChunkRef   string
	EntitySlug string
	Text       string
	Score      float64
}

func Open(path string) (*SQLite, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	s := &SQLite{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("index migrate: %w", err)
	}
	return s, nil
}

func (s *SQLite) Close() error { return s.db.Close() }

func (s *SQLite) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS entities(
			slug TEXT PRIMARY KEY,
			type TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS claims(
			slug TEXT NOT NULL,
			id TEXT NOT NULL,
			family TEXT NOT NULL,
			predicate TEXT NOT NULL,
			object TEXT NOT NULL,
			object_key TEXT NOT NULL,
			confidence REAL NOT NULL,
			valid_from TEXT NOT NULL DEFAULT '',
			valid_to TEXT NOT NULL DEFAULT '',
			src TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL,
			superseded_by TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (slug, id))`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS chunks USING fts5(
			chunk_ref UNINDEXED, entity_slug UNINDEXED, text)`,
		// Vectors are keyed by model@version so embedding-model pins never
		// mix in search (§7.5). Empty until an embedding model is pinned.
		`CREATE TABLE IF NOT EXISTS vectors(
			chunk_ref TEXT PRIMARY KEY,
			model TEXT NOT NULL,
			vec BLOB NOT NULL)`,
	}
	// Schema shells for runtime-only state (see RuntimeTables): generic
	// enough to hold a row today, replaced with a real schema by whichever
	// milestone task owns the table.
	for _, t := range RuntimeTables {
		stmts = append(stmts, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s(
			id TEXT PRIMARY KEY,
			payload TEXT NOT NULL DEFAULT '')`, t))
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLite) UpsertEntity(ctx context.Context, e domain.Entity) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO entities(slug, type) VALUES(?, ?)
		ON CONFLICT(slug) DO UPDATE SET type = excluded.type`, e.Slug, e.Type)
	return err
}

func (s *SQLite) UpsertClaim(ctx context.Context, c domain.Claim) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO claims
		(slug, id, family, predicate, object, object_key, confidence,
		 valid_from, valid_to, src, state, superseded_by)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(slug, id) DO UPDATE SET
			family = excluded.family, predicate = excluded.predicate,
			object = excluded.object, object_key = excluded.object_key,
			confidence = excluded.confidence, valid_from = excluded.valid_from,
			valid_to = excluded.valid_to, src = excluded.src,
			state = excluded.state, superseded_by = excluded.superseded_by`,
		c.SubjectSlug, c.ID, c.Family, c.Predicate, c.Object, c.ObjectKey,
		c.Confidence, c.ValidFrom, c.ValidTo, c.SourceRef, string(c.State), c.SupersededBy)
	return err
}

func (s *SQLite) InsertChunk(ctx context.Context, chunkRef, entitySlug, text string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO chunks(chunk_ref, entity_slug, text) VALUES(?,?,?)`,
		chunkRef, entitySlug, text)
	return err
}

func (s *SQLite) SearchFTS(ctx context.Context, query string, limit int) ([]Hit, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT chunk_ref, entity_slug, text, bm25(chunks)
		FROM chunks WHERE chunks MATCH ? ORDER BY bm25(chunks) LIMIT ?`, query, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var hits []Hit
	for rows.Next() {
		var h Hit
		var bm float64
		if err := rows.Scan(&h.ChunkRef, &h.EntitySlug, &h.Text, &bm); err != nil {
			return nil, err
		}
		h.Score = -bm // bm25() returns lower-is-better; flip for callers
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

func (s *SQLite) ResetAll(ctx context.Context) error {
	derived := []string{"entities", "claims", "chunks", "vectors"}
	for _, t := range derived {
		if slices.Contains(RuntimeTables, t) {
			// A table can't be both rebuilt-from-repo and runtime-only;
			// catch the allowlist bug loudly instead of silently wiping
			// runtime state on the next rebuild.
			return fmt.Errorf("index: %s is listed as both derived and RuntimeTables", t)
		}
	}
	for _, t := range derived {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM `+t); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLite) Stats(ctx context.Context) (map[string]int64, error) {
	out := map[string]int64{}
	for _, t := range []string{"entities", "claims", "chunks", "vectors"} {
		var n int64
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+t).Scan(&n); err != nil {
			return nil, err
		}
		out[t] = n
	}
	return out, nil
}

// Dump writes every table in deterministic order. The wipe-and-rebuild
// invariant test compares dumps byte-for-byte: same repo bytes, same
// pinned model set, same dump — always.
func (s *SQLite) Dump(ctx context.Context, w io.Writer) error {
	queries := []struct {
		label string
		query string
		cols  int
	}{
		{"entities", `SELECT slug, type FROM entities ORDER BY slug`, 2},
		{"claims", `SELECT slug, id, family, predicate, object, object_key,
			confidence, valid_from, valid_to, src, state, superseded_by
			FROM claims ORDER BY slug, id`, 12},
		{"chunks", `SELECT chunk_ref, entity_slug, text FROM chunks ORDER BY chunk_ref`, 3},
		{"vectors", `SELECT chunk_ref, model FROM vectors ORDER BY chunk_ref`, 2},
	}
	for _, q := range queries {
		rows, err := s.db.QueryContext(ctx, q.query)
		if err != nil {
			return err
		}
		vals := make([]any, q.cols)
		ptrs := make([]any, q.cols)
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				_ = rows.Close()
				return err
			}
			cells := make([]string, q.cols)
			for i, v := range vals {
				cells[i] = formatValue(v)
			}
			if _, err := fmt.Fprintf(w, "%s\t%s\n", q.label, strings.Join(cells, "\t")); err != nil {
				_ = rows.Close()
				return err
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
	}
	return nil
}

func formatValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	case []byte:
		return string(x)
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}
