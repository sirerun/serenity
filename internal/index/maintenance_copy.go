package index

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ErrMaintenanceCopyUnqualified never contains database contents or paths.
var ErrMaintenanceCopyUnqualified = errors.New("index: maintenance copy unqualified or unavailable")

const maintenanceCopyMaxFamilyBytes = 32 << 20
const maintenanceCopyMaxSchemaRows = 128
const maintenanceCopyMaxSchemaBytes = 128 << 10

// MaintenanceFamilyFile identifies one file in the operation-owned copy. Paths
// and database row contents never enter qualification evidence.
type MaintenanceFamilyFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// Qualification is preparation evidence, never erasure/activation authority.
// SQLite may create, modify or remove SHM in this explicitly owned copy.
type Qualification struct {
	Schema             string                  `json:"schema"`
	SchemaSHA256       string                  `json:"schema_sha256"`
	Before             []MaintenanceFamilyFile `json:"before"`
	After              []MaintenanceFamilyFile `json:"after"`
	EmptyRuntimeTables []string                `json:"empty_runtime_tables"`
	SidecarEffects     string                  `json:"sidecar_effects"`
}

type maintenanceFilePin struct {
	file MaintenanceFamilyFile
	info os.FileInfo
}
type maintenanceFamily struct {
	files     map[string]maintenanceFilePin
	directory os.FileInfo
}
type maintenanceSchemaRow struct {
	Type  string
	Name  string
	Table string
	SQL   sql.NullString
}

// QualifyMaintenanceCopy inspects only a caller-owned, private, CLOSED complete
// SQLite family copy in a dedicated directory containing index.db[-wal/-shm].
// The caller owns source-copy custody and exclusion. This API never opens the
// original database, runs migrations on the copy, discards files or invokes a
// provider. Its read-only SQL may change owned-copy SHM; this is not an immutable
// filesystem inspection. All runtime/control tables must be empty.
func QualifyMaintenanceCopy(ctx context.Context, copyPath string) (Qualification, error) {
	return qualifyMaintenanceCopy(ctx, copyPath, nil)
}

// observe is a per-call test-only interleaving seam, not a mutable global hook.
func qualifyMaintenanceCopy(ctx context.Context, copyPath string, observe func(string)) (result Qualification, err error) {
	if ctx == nil {
		return result, ErrMaintenanceCopyUnqualified
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return result, ErrMaintenanceCopyUnqualified
	}
	absolute, err := filepath.Abs(copyPath)
	if err != nil || absolute != copyPath || filepath.Base(absolute) != "index.db" {
		return result, ErrMaintenanceCopyUnqualified
	}
	parent := filepath.Dir(absolute)
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil || resolved != parent {
		return result, ErrMaintenanceCopyUnqualified
	}
	before, err := maintenancePinFamily(ctx, parent)
	if err != nil {
		return result, ErrMaintenanceCopyUnqualified
	}
	if observe != nil {
		observe("pinned")
	}
	// A fresh in-memory reference runs only the current trusted migration code.
	// It cannot modify the operation-owned source copy or create scratch files.
	reference, err := Open(":memory:")
	if err != nil {
		return result, ErrMaintenanceCopyUnqualified
	}
	reference.db.SetMaxOpenConns(1)
	expected, schemaErr := maintenanceReadSchema(ctx, reference.db)
	referenceErr := reference.Close()
	if errors.Join(schemaErr, referenceErr) != nil {
		return result, ErrMaintenanceCopyUnqualified
	}
	u := url.URL{Scheme: "file", Path: absolute}
	query := url.Values{"mode": {"ro"}, "_pragma": {"query_only(1)", "trusted_schema(OFF)", "busy_timeout(100)"}}
	u.RawQuery = query.Encode()
	// Re-pin immediately before SQLite resolves the filename, after any caller
	// interleaving. Custody still requires private cooperative exclusion; this
	// is not a claim against arbitrary same-UID filesystem mutation.
	opening, pinErr := maintenancePinFamily(ctx, parent)
	if pinErr != nil || !maintenanceExactFamily(before, opening) {
		return result, ErrMaintenanceCopyUnqualified
	}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return result, ErrMaintenanceCopyUnqualified
	}
	db.SetMaxOpenConns(1)
	closed := false
	defer func() {
		if !closed {
			_ = db.Close()
		}
	}()
	if err = db.PingContext(ctx); err != nil {
		return result, ErrMaintenanceCopyUnqualified
	}
	if observe != nil {
		observe("opened")
	}
	actual, err := maintenanceReadSchema(ctx, db)
	if err != nil {
		return result, ErrMaintenanceCopyUnqualified
	}
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		return result, ErrMaintenanceCopyUnqualified
	}
	actualJSON, err := json.Marshal(actual)
	if err != nil || string(expectedJSON) != string(actualJSON) {
		return result, ErrMaintenanceCopyUnqualified
	}
	for _, pragma := range []string{"PRAGMA user_version", "PRAGMA application_id"} {
		var value int64
		if err = db.QueryRowContext(ctx, pragma).Scan(&value); err != nil || value != 0 {
			return result, ErrMaintenanceCopyUnqualified
		}
	}
	// Unknown schemas must not reach quick_check: SQLite evaluates CHECK
	// expressions during integrity inspection. Only the exact current schema
	// may run integrity or control-table queries.
	if observe != nil {
		observe("schema-qualified")
	}
	var integrity string
	if err = db.QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&integrity); err != nil || integrity != "ok" {
		return result, ErrMaintenanceCopyUnqualified
	}
	// Table names come from a fixed current cohort, never from caller SQL or
	// row data. A future runtime-table contract change must qualify a new cohort.
	controls := []string{"jobs", "disposition_items", "disposition_history", "spend_ledger", "caches", "events"}
	if !maintenanceSameTableSet(controls, RuntimeTables) {
		return result, ErrMaintenanceCopyUnqualified
	}
	for _, table := range controls {
		var found int
		err = db.QueryRowContext(ctx, "SELECT 1 FROM "+table+" LIMIT 1").Scan(&found)
		if !errors.Is(err, sql.ErrNoRows) {
			return result, ErrMaintenanceCopyUnqualified
		}
	}
	if observe != nil {
		observe("queried")
	}
	closeErr := db.Close()
	closed = true
	if closeErr != nil {
		return result, ErrMaintenanceCopyUnqualified
	}
	if observe != nil {
		observe("closed")
	}
	after, err := maintenancePinFamily(ctx, parent)
	if err != nil || !os.SameFile(before.directory, after.directory) {
		return result, ErrMaintenanceCopyUnqualified
	}
	for _, name := range []string{"index.db", "index.db-wal"} {
		first, was := before.files[name]
		last, is := after.files[name]
		if was != is || was && (!os.SameFile(first.info, last.info) || first.file != last.file) {
			return result, ErrMaintenanceCopyUnqualified
		}
	}
	if ctx.Err() != nil {
		return result, ErrMaintenanceCopyUnqualified
	}
	digest := sha256.Sum256(actualJSON)
	return Qualification{Schema: "serenity.index-maintenance-cohort/v1", SchemaSHA256: hex.EncodeToString(digest[:]), Before: before.publicFiles(), After: after.publicFiles(), EmptyRuntimeTables: controls, SidecarEffects: "owned_copy_shm_may_change"}, nil
}

func maintenanceExactFamily(a, b maintenanceFamily) bool {
	if !os.SameFile(a.directory, b.directory) || len(a.files) != len(b.files) {
		return false
	}
	for name, first := range a.files {
		last, ok := b.files[name]
		if !ok || first.file != last.file || !os.SameFile(first.info, last.info) || !first.info.ModTime().Equal(last.info.ModTime()) {
			return false
		}
	}
	return true
}

func maintenanceSameTableSet(a, b []string) bool {
	a = append([]string(nil), a...)
	b = append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func (f maintenanceFamily) publicFiles() []MaintenanceFamilyFile {
	names := make([]string, 0, len(f.files))
	for name := range f.files {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]MaintenanceFamilyFile, 0, len(names))
	for _, name := range names {
		out = append(out, f.files[name].file)
	}
	return out
}

func maintenancePinFamily(ctx context.Context, parent string) (family maintenanceFamily, err error) {
	info, err := os.Lstat(parent)
	if err != nil || !maintenancePrivateInfo(info, true) {
		return family, ErrMaintenanceCopyUnqualified
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		return family, ErrMaintenanceCopyUnqualified
	}
	defer func() { _ = root.Close() }()
	openedRoot, rootErr := root.Lstat(".")
	if rootErr != nil || !maintenancePrivateInfo(openedRoot, true) || !os.SameFile(info, openedRoot) {
		return family, ErrMaintenanceCopyUnqualified
	}
	directory, err := root.Open(".")
	if err != nil {
		return family, ErrMaintenanceCopyUnqualified
	}
	entries, readErr := directory.ReadDir(4)
	closeErr := directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) || closeErr != nil || len(entries) > 3 {
		return family, ErrMaintenanceCopyUnqualified
	}
	family = maintenanceFamily{files: map[string]maintenanceFilePin{}, directory: info}
	total := int64(0)
	for _, entry := range entries {
		name := entry.Name()
		if name != "index.db" && name != "index.db-wal" && name != "index.db-shm" {
			return family, ErrMaintenanceCopyUnqualified
		}
		file, openErr := maintenanceOpenPrivate(root, name)
		if openErr != nil {
			return family, ErrMaintenanceCopyUnqualified
		}
		first, statErr := file.Stat()
		if statErr != nil || !maintenancePrivateInfo(first, false) || first.Size() > maintenanceCopyMaxFamilyBytes-total {
			_ = file.Close()
			return family, ErrMaintenanceCopyUnqualified
		}
		digest := sha256.New()
		var count int64
		buffer := make([]byte, 64<<10)
		for {
			if ctx.Err() != nil {
				_ = file.Close()
				return family, ErrMaintenanceCopyUnqualified
			}
			n, readErr := file.Read(buffer)
			if n > 0 {
				_, _ = digest.Write(buffer[:n])
				count += int64(n)
			}
			if count > first.Size() {
				_ = file.Close()
				return family, ErrMaintenanceCopyUnqualified
			}
			if errors.Is(readErr, io.EOF) {
				break
			}
			if readErr != nil {
				_ = file.Close()
				return family, ErrMaintenanceCopyUnqualified
			}
		}
		last, lastErr := file.Stat()
		current, currentErr := root.Lstat(name)
		closeErr = file.Close()
		if errors.Join(lastErr, currentErr, closeErr) != nil || !maintenancePrivateInfo(current, false) || !os.SameFile(first, last) || !os.SameFile(first, current) || count != first.Size() || last.Size() != first.Size() || !last.ModTime().Equal(first.ModTime()) {
			return family, ErrMaintenanceCopyUnqualified
		}
		total += count
		family.files[name] = maintenanceFilePin{file: MaintenanceFamilyFile{Name: name, SHA256: hex.EncodeToString(digest.Sum(nil)), Bytes: count}, info: first}
	}
	if _, ok := family.files["index.db"]; !ok {
		return family, ErrMaintenanceCopyUnqualified
	}
	current, err := os.Lstat(parent)
	if err != nil || !maintenancePrivateInfo(current, true) || !os.SameFile(info, current) {
		return family, ErrMaintenanceCopyUnqualified
	}
	return family, nil
}

func maintenanceReadSchema(ctx context.Context, db *sql.DB) ([]maintenanceSchemaRow, error) {
	var count, totalBytes int64
	err := db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(length(CAST(type AS BLOB))+length(CAST(name AS BLOB))+length(CAST(tbl_name AS BLOB))+COALESCE(length(CAST(sql AS BLOB)),0)),0) FROM sqlite_schema`).Scan(&count, &totalBytes)
	if err != nil || count > maintenanceCopyMaxSchemaRows || totalBytes > maintenanceCopyMaxSchemaBytes {
		return nil, ErrMaintenanceCopyUnqualified
	}
	rows, err := db.QueryContext(ctx, "SELECT type,name,tbl_name,sql FROM sqlite_schema ORDER BY type,name,tbl_name,sql LIMIT 129")
	if err != nil {
		return nil, ErrMaintenanceCopyUnqualified
	}
	defer func() { _ = rows.Close() }()
	out := []maintenanceSchemaRow{}
	total := 0
	for rows.Next() {
		var row maintenanceSchemaRow
		if err = rows.Scan(&row.Type, &row.Name, &row.Table, &row.SQL); err != nil {
			return nil, ErrMaintenanceCopyUnqualified
		}
		total += len(row.Type) + len(row.Name) + len(row.Table) + len(row.SQL.String)
		if len(out) >= maintenanceCopyMaxSchemaRows || total > maintenanceCopyMaxSchemaBytes {
			return nil, ErrMaintenanceCopyUnqualified
		}
		out = append(out, row)
	}
	if rows.Err() != nil {
		return nil, ErrMaintenanceCopyUnqualified
	}
	return out, nil
}
