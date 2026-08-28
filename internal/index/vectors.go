package index

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
)

// encodeVector serializes a vector to a fixed little-endian float32 BLOB.
// The encoding is a pure function of the input floats: the same vector
// always produces the same bytes, which is what lets the wipe-and-rebuild
// invariant (§7.5) extend to the vectors table -- see
// TestVectorsParticipateInRebuildIdentity.
func encodeVector(vec []float32) []byte {
	buf := make([]byte, 4*len(vec))
	for i, f := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// decodeVector is encodeVector's inverse.
func decodeVector(buf []byte) ([]float32, error) {
	if len(buf)%4 != 0 {
		return nil, fmt.Errorf("index: vector blob length %d is not a multiple of 4", len(buf))
	}
	vec := make([]float32, len(buf)/4)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return vec, nil
}

// UpsertVector stores chunkRef's embedding under model. Writing a second
// vector for the same (chunkRef, model) pair replaces it; writing under a
// different model leaves the existing pin's row untouched (composite
// primary key, see migrate's vectors table).
func (s *SQLite) UpsertVector(ctx context.Context, chunkRef, model string, vec []float32) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO vectors(chunk_ref, model, vec) VALUES(?, ?, ?)
		ON CONFLICT(chunk_ref, model) DO UPDATE SET vec = excluded.vec`,
		chunkRef, model, encodeVector(vec))
	return err
}

// HasVector reports whether chunkRef has a stored vector under model
// specifically -- a chunk with only a different pin's vector reports
// false, which is exactly what lets a caller route it to FTS instead of
// silently comparing it against the wrong pin (§10.1).
func (s *SQLite) HasVector(ctx context.Context, chunkRef, model string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM vectors WHERE chunk_ref = ? AND model = ?`, chunkRef, model).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("index: has vector: %w", err)
	}
	return n > 0, nil
}

// SearchVectors performs an exact cosine scan (§7.5: "exact cosine scan
// over memory-mapped embeddings ... single-digit milliseconds" at
// gbrain-production scale) restricted to model's own rows. It never reads,
// let alone scores, a row stored under a different pin.
//
// Hits are hydrated from the chunks table so the return shape matches
// SearchFTS: a chunk_ref with no matching chunks row (should not happen in
// normal operation -- InsertChunk and UpsertVector are always called
// together for the same chunk) is silently omitted rather than erroring,
// since a missing text projection is a data-completeness gap, not a
// pin-mixing one.
func (s *SQLite) SearchVectors(ctx context.Context, model string, query []float32, limit int) ([]Hit, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT chunk_ref, vec FROM vectors WHERE model = ?`, model)
	if err != nil {
		return nil, fmt.Errorf("index: search vectors: %w", err)
	}
	type scored struct {
		chunkRef string
		score    float64
	}
	var candidates []scored
	for rows.Next() {
		var chunkRef string
		var blob []byte
		if err := rows.Scan(&chunkRef, &blob); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("index: search vectors: %w", err)
		}
		vec, err := decodeVector(blob)
		if err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("index: search vectors: chunk %s: %w", chunkRef, err)
		}
		score, err := cosine(query, vec)
		if err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("index: search vectors: chunk %s: %w", chunkRef, err)
		}
		candidates = append(candidates, scored{chunkRef, score})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("index: search vectors: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("index: search vectors: %w", err)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].chunkRef < candidates[j].chunkRef // deterministic tiebreak
	})
	if limit >= 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}

	hits := make([]Hit, 0, len(candidates))
	for _, c := range candidates {
		var entitySlug, text string
		err := s.db.QueryRowContext(ctx,
			`SELECT entity_slug, text FROM chunks WHERE chunk_ref = ?`, c.chunkRef).Scan(&entitySlug, &text)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("index: search vectors: hydrate chunk %s: %w", c.chunkRef, err)
		}
		hits = append(hits, Hit{ChunkRef: c.chunkRef, EntitySlug: entitySlug, Text: text, Score: c.score})
	}
	return hits, nil
}

// cosine computes cosine similarity in [-1, 1]. Vectors under the same pin
// are always the same dimensionality by construction (one embedding model,
// one output width); a mismatch means two different pins' vectors are
// being compared, which must never happen -- callers only reach cosine via
// SearchVectors, which already scopes its rows to a single model, so this
// is a defensive error, not an expected path.
func cosine(a, b []float32) (float64, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("vector dimension mismatch: %d vs %d", len(a), len(b))
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0, nil
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB)), nil
}
