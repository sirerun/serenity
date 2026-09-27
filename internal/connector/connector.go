// Package connector defines the ingest boundary every provider adapter
// implements (RFC 0001 §10.1). Poll fetches new RawItems since a
// resume-cursor; ToSource converts one fetched item into the immutable,
// content-addressed domain.Source the store dedupes on. v1 connectors
// (file watcher T1.3, IMAP T1.4, git-repo crawler T1.5, voice note) all
// implement Connector; no provider-specific code lives here.
package connector

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/sirerun/serenity/internal/domain"
)

// Cursor is a connector's opaque resume position -- an IMAP
// UIDVALIDITY+UID pair, a filesystem watermark, a git commit sha, whatever
// the provider needs. Connectors own their cursor's shape; nothing outside
// the connector that produced it interprets the bytes.
type Cursor json.RawMessage

// RawItem is one unit Poll fetched, before ToSource turns it into a
// domain.Source. Bytes plus enough metadata to compute the source's
// sha256, kind, uri, and occurred_at -- kept provider-agnostic so the
// pipeline downstream of Poll never branches on connector type.
type RawItem struct {
	URI        string
	Kind       string
	Bytes      []byte
	OccurredAt time.Time
	Meta       map[string]string
	// LocalPath is the item's absolute filesystem path, for connectors that
	// read local files. It is local-only (PRIV-03): ToSource never copies it
	// into the committed Source, and the sync pipeline records it only in
	// the derived index, keyed by Meta[MetaPathHash].
	LocalPath string
}

// MetaPathHash is the source meta key holding PathHash of a local file
// source's absolute path. It lets a local index map a committed source
// back to its file without the committed record naming the path.
const MetaPathHash = "path_hash"

// PathHash returns the lowercase hex sha256 of abs in cleaned,
// forward-slash form, the stable de-duplication identifier a file-backed
// source carries in place of its absolute path.
func PathHash(abs string) string {
	sum := sha256.Sum256([]byte(filepath.ToSlash(filepath.Clean(abs))))
	return hex.EncodeToString(sum[:])
}

// Connector is the pluggable ingest boundary (RFC §10.1). Poll must be
// idempotent: replaying the same cursor returns the same items, and
// advancing the cursor never skips or duplicates a source. ToSource
// converts one fetched item into the Source the store's content-address
// dedup runs on.
type Connector interface {
	Name() string
	Poll(ctx context.Context, cursor Cursor) (items []RawItem, next Cursor, err error)
	ToSource(item RawItem) (domain.Source, error)
}
