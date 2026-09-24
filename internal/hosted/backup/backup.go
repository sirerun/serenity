// Package backup creates portable manifest-v2 control snapshots and canonical
// Git bundles, and restores them into a frozen, pending-reactivation state.
//
// Every backup and restore stages its full output in a private temporary
// directory beside the requested destination and only becomes visible at
// that destination through one atomic rename, after every entry has been
// built and validated. A failure at any point leaves the destination
// completely absent -- never a partial snapshot or a partial restore -- and
// the caller can retry the same call.
package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/store"
	"github.com/sirerun/serenity/internal/hosted/testhooks"
)

const (
	controlDBName = "control.db"
	manifestFile  = "manifest.json" // must match contracts.ManifestV2's reserved name
)

// ErrLegacySnapshot means the on-disk manifest is the retired version-1 shape
// (this package's original Manifest/Brain, before manifest v2). It carries no
// checksums, no bundle heads and no schema/journal binding, so none of this
// package's v2 guarantees can be retrofitted onto it. The explicit policy is
// refusal, not silent best-effort support: recreate the backup under the
// current binary before it can be restored.
var ErrLegacySnapshot = errors.New("hosted/backup: version-1 snapshot is retired and can no longer be restored; recreate the backup under manifest v2")

// ErrManifestInventoryMismatch means the manifest's brain inventory does not
// match the restored control database's ready-brain set exactly: a brain the
// database expects is missing from the manifest, the manifest names a brain
// the database does not have, or an entry was duplicated. Duplicate or
// unsorted IDs are already rejected by ManifestV2.Validate; this check adds
// the cross-reference Validate cannot make on its own.
var ErrManifestInventoryMismatch = errors.New("hosted/backup: manifest brain inventory does not match the control database")

// ErrSchemaMismatch means the manifest's recorded source schema version does
// not match the actual control-database artifact's schema version, checked
// by a read-only connection before store.Open is ever allowed to migrate it.
var ErrSchemaMismatch = errors.New("hosted/backup: control database schema does not match the manifest")

// safeID accepts exactly the identifier shape store.go's brain/account IDs
// use: 16-64 ASCII letters or digits. It is also used as a filesystem path
// element, so this doubles as the path-safety check for that element.
func safeID(id string) bool {
	if len(id) < 16 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// manifestProbe reads only the field needed to recognize a retired version-1
// manifest before attempting to unmarshal it as ManifestV2.
type manifestProbe struct {
	Version int `json:"version"`
}

// requireRealDir rejects a path that is not an actual directory or that is
// (or passes through, at its final component) a symlink: dataDir, a snapshot
// root and each brain root must be exactly what they claim to be, not a link
// that could redirect a read or a write outside the intended tree.
func requireRealDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("hosted/backup: %s must be a real directory, not a symlink", path)
	}
	return nil
}

// Create refuses to overwrite an existing destination. It builds the full
// snapshot -- control database, every ready brain's bundle, and the manifest
// that binds them -- in a private staging directory beside destination, and
// only renames it into place once every artifact is written and the manifest
// validates. Create must run only while no concurrent writer can mutate
// dataDir (an exclusive local lock for an offline run, or the live service's
// coordinated maintenance window for an online run); it does not itself
// acquire that exclusivity.
//
// buildSHA, if non-empty, is recorded as-is as the manifest's source build
// identity: the caller (service/CLI assembly) is expected to supply the
// release-injected version it already carries. If buildSHA is empty, Create
// falls back to the toolchain-embedded VCS revision this binary was built
// from (runtime/debug.ReadBuildInfo); internal/hosted/backup cannot import
// internal/cli's Version without an import cycle (internal/cli already
// imports this package). If neither source yields a real revision, Create
// fails rather than recording a meaningless placeholder.
//
// journal is required: interfaces.md decision 3 rule 6 requires the
// deletion-journal watermark to be read before any data is copied (reading
// it after could let a snapshot silently miss a deletion). Create never
// substitutes a fabricated empty watermark for a missing journal -- a zero
// watermark is only ever what journal itself reports for an actually empty
// journal. Until task48 ships a production DeletionJournal adapter, callers
// must supply an explicit implementation (see
// docs/launch/evidence/T23.49/integration-request.md); passing one that
// silently claims emptiness without checking a real journal is a caller
// error this package has no way to detect.
func Create(ctx context.Context, dataDir, destination, buildSHA string, journal contracts.DeletionJournal) (err error) {
	if journal == nil {
		return errors.New("hosted/backup: a deletion-journal reader is required to record the pre-copy watermark")
	}
	resolvedBuildSHA, err := resolveBuildSHA(buildSHA)
	if err != nil {
		return err
	}
	if err = requireRealDir(dataDir); err != nil {
		return err
	}
	if _, statErr := os.Lstat(destination); statErr == nil {
		return fmt.Errorf("hosted/backup: destination already exists: %s", destination)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}

	// Read the deletion journal's current watermark before copying any data
	// (interfaces.md decision 3 rule 6). ReadThrough(ctx, zero) walks the
	// whole journal from its start and reports the last verified position as
	// To; task48's real adapter should optimize this once it exists, but
	// correctness, not performance, is this task's concern.
	journalRead, err := journal.ReadThrough(ctx, contracts.DeletionWatermark{})
	if err != nil {
		return fmt.Errorf("hosted/backup: read deletion journal watermark: %w", err)
	}
	watermark := journalRead.To

	if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(destination), "."+filepath.Base(destination)+".staging-*")
	if err != nil {
		return fmt.Errorf("hosted/backup: create staging directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(staging)
		}
	}()

	controlPath := filepath.Join(staging, controlDBName)
	live, err := store.Open(filepath.Join(dataDir, controlDBName))
	if err != nil {
		return err
	}
	if _, err = live.DB().ExecContext(ctx, `VACUUM INTO ?`, controlPath); err != nil {
		return errors.Join(fmt.Errorf("hosted/backup: snapshot control database: %w", err), live.Close())
	}
	if err = live.Close(); err != nil {
		return err
	}

	schemaVersion, ids, err := inspectStagedControlDB(ctx, controlPath)
	if err != nil {
		return err
	}

	brains := make([]contracts.BrainArtifact, len(ids))
	for i, id := range ids {
		artifact, buildErr := buildBrainArtifact(ctx, dataDir, staging, id)
		if buildErr != nil {
			return buildErr
		}
		brains[i] = artifact
	}

	// The control database handle above is already closed (inspectStagedControlDB
	// closes it internally after checkpointing), so the bytes on disk are final
	// before they are hashed here.
	controlSHA, controlLen, err := sha256File(controlPath)
	if err != nil {
		return err
	}

	manifest := contracts.ManifestV2{
		Version: 2,
		Source: contracts.SourceRef{
			BuildSHA:      resolvedBuildSHA,
			SchemaVersion: schemaVersion,
		},
		CreatedAt: time.Now().UTC(),
		ControlDB: contracts.ArtifactRef{
			RelativePath: controlDBName,
			LengthBytes:  controlLen,
			SHA256:       controlSHA,
		},
		Brains:           brains,
		JournalWatermark: watermark,
	}
	if err = manifest.Validate(); err != nil {
		return fmt.Errorf("hosted/backup: built an invalid manifest: %w", err)
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err = writeFileSynced(filepath.Join(staging, manifestFile), append(data, '\n'), 0600); err != nil {
		return err
	}
	if err = fsyncDir(staging); err != nil {
		return err
	}

	// The manifest v2 artifact is fully staged and fsynced; nothing further
	// happens before this backup becomes the visible, atomically published
	// snapshot at destination.
	testhooks.At(testhooks.PhaseBackupManifestWritten)

	if _, statErr := os.Lstat(destination); statErr == nil {
		return fmt.Errorf("hosted/backup: destination was created concurrently: %s", destination)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if err = os.Rename(staging, destination); err != nil {
		return fmt.Errorf("hosted/backup: publish snapshot: %w", err)
	}
	published = true
	return fsyncDir(filepath.Dir(destination))
}

// inspectStagedControlDB opens the freshly vacuumed staging copy exactly once
// to read its schema version and ready-brain inventory, then checkpoints its
// WAL fully into the main file and closes the handle before returning, so no
// SQLite handle referencing controlPath survives past this call: the caller
// hashes and eventually renames the file, and a WAL/SHM sidecar or a lingering
// handle across that rename would both be a durability defect.
func inspectStagedControlDB(ctx context.Context, controlPath string) (schemaVersion int, readyBrainIDs []string, err error) {
	snapshot, err := store.Open(controlPath)
	if err != nil {
		return 0, nil, err
	}
	closeErr := func() (cerr error) {
		defer func() { cerr = errors.Join(cerr, snapshot.Close()) }()
		if err = snapshot.DB().QueryRowContext(ctx, `SELECT max(version) FROM schema_migrations`).Scan(&schemaVersion); err != nil {
			return err
		}
		rows, e := snapshot.DB().QueryContext(ctx, `SELECT id,path_key FROM brains WHERE state='ready' AND deleted_at IS NULL ORDER BY id`)
		if e != nil {
			return e
		}
		for rows.Next() {
			var id, path string
			if e = rows.Scan(&id, &path); e != nil {
				_ = rows.Close()
				return e
			}
			if id != path || !safeID(id) {
				_ = rows.Close()
				return fmt.Errorf("hosted/backup: invalid brain path in snapshot: %q", id)
			}
			readyBrainIDs = append(readyBrainIDs, id)
		}
		if e = rows.Err(); e != nil {
			_ = rows.Close()
			return e
		}
		if e = rows.Close(); e != nil {
			return e
		}
		_, e = snapshot.DB().ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
		return e
	}()
	if closeErr != nil {
		return 0, nil, closeErr
	}
	return schemaVersion, readyBrainIDs, nil
}

// buildBrainArtifact stages one ready brain's bundle and reports its manifest
// entry. A ready brain missing its canonical .git repository always fails as
// corruption: an emptied directory is not proof the brain was never used, so
// this package never infers "never touched" from directory content. Today's
// provisioning (provision.finish) can mark a brain ready before pool.Acquire
// has ever lazily created its .git repository, which makes this reachable
// for a legitimately untouched brain -- that is a cross-owner provisioning
// gap (see integration-request.md), not a reason to weaken this check.
func buildBrainArtifact(ctx context.Context, dataDir, staging, id string) (contracts.BrainArtifact, error) {
	root := filepath.Join(dataDir, "brains", id)
	if err := requireRealDir(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return contracts.BrainArtifact{}, fmt.Errorf("hosted/backup: ready brain %s has no directory (corruption)", id)
		}
		return contracts.BrainArtifact{}, err
	}
	gitInfo, err := os.Lstat(filepath.Join(root, ".git"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return contracts.BrainArtifact{}, fmt.Errorf("hosted/backup: ready brain %s has no canonical git repository (corruption)", id)
		}
		return contracts.BrainArtifact{}, err
	}
	if !gitInfo.IsDir() || gitInfo.Mode()&os.ModeSymlink != 0 {
		return contracts.BrainArtifact{}, fmt.Errorf("hosted/backup: brain %s has an unsafe .git entry", id)
	}

	if err = flushDirtyCanonicalState(ctx, root); err != nil {
		return contracts.BrainArtifact{}, fmt.Errorf("hosted/backup: brain %s: %w", id, err)
	}

	relBundle := id + ".bundle"
	bundlePath := filepath.Join(staging, relBundle)
	if output, e := exec.CommandContext(ctx, "git", "-C", root, "bundle", "create", bundlePath, "--all").CombinedOutput(); e != nil {
		return contracts.BrainArtifact{}, fmt.Errorf("hosted/backup: bundle brain %s: %w: %s", id, e, output)
	}
	heads, err := bundleHeads(ctx, bundlePath)
	if err != nil {
		return contracts.BrainArtifact{}, fmt.Errorf("hosted/backup: brain %s: %w", id, err)
	}
	if len(heads) == 0 {
		return contracts.BrainArtifact{}, fmt.Errorf("hosted/backup: brain %s bundle carries no heads", id)
	}
	sum, size, err := sha256File(bundlePath)
	if err != nil {
		return contracts.BrainArtifact{}, err
	}
	return contracts.BrainArtifact{
		ID: id,
		ArtifactRef: contracts.ArtifactRef{
			RelativePath: relBundle,
			LengthBytes:  size,
			SHA256:       sum,
		},
		Heads: heads,
	}, nil
}

// flushDirtyCanonicalState commits any uncommitted change in root's working
// tree before it is bundled, rather than letting `git bundle` -- which only
// ever carries committed, ref-reachable history -- silently exclude it. Create
// runs only under the caller's coordinated exclusivity (an offline lock or
// the live service's maintenance window), so a dirty tree found here is
// stable: it is either the working half of an operation that crashed before
// its canonical write finished (never acknowledged to a client; safe to
// include or lose either way) or a writer.Flush that has not yet run. Either
// way, folding it into one extra commit means the snapshot always reflects a
// real, self-consistent state of the brain -- never a bundle silently missing
// files still sitting dirty on disk. If the flush itself fails, Create fails
// rather than bundling an inconsistent tree.
func flushDirtyCanonicalState(ctx context.Context, root string) error {
	status, err := exec.CommandContext(ctx, "git", "-C", root, "status", "--porcelain").Output()
	if err != nil {
		return fmt.Errorf("check canonical working tree: %w", err)
	}
	if len(strings.TrimSpace(string(status))) == 0 {
		return nil
	}
	if output, e := exec.CommandContext(ctx, "git", "-C", root, "add", "-A").CombinedOutput(); e != nil {
		return fmt.Errorf("stage dirty canonical state: %w: %s", e, output)
	}
	if output, e := exec.CommandContext(ctx, "git", "-C", root, "commit", "--quiet", "-m", "serenity: pre-backup flush").CombinedOutput(); e != nil {
		return fmt.Errorf("commit dirty canonical state: %w: %s", e, output)
	}
	return nil
}

// bundleHeads verifies bundlePath is a well-formed, self-contained bundle and
// returns the refs it carries, sorted ascending by ref to match
// ManifestV2's required order.
func bundleHeads(ctx context.Context, bundlePath string) ([]contracts.BundleHead, error) {
	if output, e := exec.CommandContext(ctx, "git", "bundle", "verify", "--quiet", bundlePath).CombinedOutput(); e != nil {
		return nil, fmt.Errorf("verify bundle: %w: %s", e, output)
	}
	output, err := exec.CommandContext(ctx, "git", "bundle", "list-heads", bundlePath).Output()
	if err != nil {
		return nil, fmt.Errorf("list bundle heads: %w", err)
	}
	return parseRefLines(string(output))
}

// localHeads lists the local branch heads (refs/heads/*) a restored working
// repository carries, plus the symbolic HEAD pointer itself, in the same
// (objectID, ref) shape bundleHeads reports. `git bundle create --all` on
// this system's single-checked-out-branch repositories always records both
// the named branch ref and a duplicate "HEAD" entry (contracts.BundleHead's
// Ref field is documented to allow exactly this); a plain `git clone` does
// not reproduce a "HEAD" entry in `show-ref`'s output, so it is resolved
// separately here to compare like for like. refs/remotes/origin/* tracking
// refs an ordinary clone also creates are excluded: those mirror, rather
// than replace, the bundle's own heads.
func localHeads(ctx context.Context, dir string) ([]contracts.BundleHead, error) {
	output, err := exec.CommandContext(ctx, "git", "-C", dir, "show-ref", "--heads").Output()
	var heads []contracts.BundleHead
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 || len(exitErr.Stderr) != 0 {
			return nil, fmt.Errorf("list restored refs: %w", err)
		}
		// show-ref exits 1 with no output when there are no matching refs.
	} else if heads, err = parseRefLines(string(output)); err != nil {
		return nil, err
	}
	if headSHA, e := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--verify", "HEAD").Output(); e == nil {
		heads = append(heads, contracts.BundleHead{ObjectID: strings.TrimSpace(string(headSHA)), Ref: "HEAD"})
	}
	sort.Slice(heads, func(i, j int) bool { return heads[i].Ref < heads[j].Ref })
	return heads, nil
}

func parseRefLines(output string) ([]contracts.BundleHead, error) {
	var heads []contracts.BundleHead
	for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("unexpected ref line: %q", line)
		}
		heads = append(heads, contracts.BundleHead{ObjectID: fields[0], Ref: fields[1]})
	}
	sort.Slice(heads, func(i, j int) bool { return heads[i].Ref < heads[j].Ref })
	return heads, nil
}

func equalHeads(a, b []contracts.BundleHead) bool {
	return slices.Equal(a, b)
}

func sha256File(path string) (sum string, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func writeFileSynced(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func fsyncDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}

// resolveBuildSHA returns explicit if it names a real value, otherwise falls
// back to the toolchain-embedded VCS revision (available whenever the binary
// was built with `go build`/`go test` inside a git checkout, no ldflags
// wiring required), and fails closed -- never a placeholder like "unknown" --
// if neither source yields a real revision.
func resolveBuildSHA(explicit string) (string, error) {
	if s := strings.TrimSpace(explicit); s != "" {
		return s, nil
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", errors.New("hosted/backup: no explicit build identity supplied and runtime build info is unavailable")
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && s.Value != "" {
			return s.Value, nil
		}
	}
	return "", errors.New("hosted/backup: no explicit build identity supplied and no VCS revision is embedded in this binary")
}

// Restore disables all restored sessions, credentials and accounts. An
// operator must reconcile deletion and subscription state before
// reactivating accounts (task50's recovery activation barrier). This
// deliberately cannot resurrect an old credential or paid entitlement.
//
// Restore validates the entire snapshot -- manifest shape, every artifact's
// checksum and length, bundle integrity and head identity, control-database
// schema and integrity, and the brain inventory against the restored control
// database -- before writing anything to destination. It builds the full
// restored tree in a private staging directory and only renames it into
// place once validation is complete and every account is frozen, so a
// failure at any point leaves no destination at all, never a partially
// restored one that could be mistaken for usable.
func Restore(ctx context.Context, snapshot, destination string) (err error) {
	if err = requireRealDir(snapshot); err != nil {
		return err
	}
	if _, statErr := os.Lstat(destination); statErr == nil {
		return fmt.Errorf("hosted/backup: restore destination already exists: %s", destination)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}

	root, err := os.OpenRoot(snapshot)
	if err != nil {
		return fmt.Errorf("hosted/backup: open snapshot: %w", err)
	}
	defer func() { _ = root.Close() }()

	manifest, err := readManifest(root)
	if err != nil {
		return err
	}

	if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(destination), "."+filepath.Base(destination)+".staging-*")
	if err != nil {
		return fmt.Errorf("hosted/backup: create restore staging directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(staging)
		}
	}()
	// scratch holds verified-copy working files that never get published:
	// every artifact is copied out of the untrusted snapshot exactly once,
	// hashed while copying, and only ever read again from here afterward --
	// never re-read from the original snapshot path after verification.
	scratch, err := os.MkdirTemp("", "serenity-restore-verify-*")
	if err != nil {
		return fmt.Errorf("hosted/backup: create verification scratch directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(scratch) }()

	controlPath := filepath.Join(staging, controlDBName)
	if err = verifyAndCopy(root, manifest.ControlDB, controlPath); err != nil {
		return fmt.Errorf("hosted/backup: control database: %w", err)
	}
	// Inspect the exact bytes just verified and copied, through a read-only
	// connection that cannot migrate them, before store.Open (below) is ever
	// allowed to. This binds the manifest's claimed schema to the actual
	// artifact prior to any mutation of it.
	if err = inspectRestoredControlDB(controlPath, manifest.Source.SchemaVersion); err != nil {
		return err
	}

	db, dbErr := store.Open(controlPath)
	if dbErr != nil {
		return dbErr
	}
	closeDB := func() error {
		if db == nil {
			return nil
		}
		_, cerr := db.DB().ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
		cerr = errors.Join(cerr, db.Close())
		db = nil
		return cerr
	}
	defer func() {
		if db != nil {
			err = errors.Join(err, closeDB())
		}
	}()

	if err = verifyBrainInventory(ctx, db, manifest.Brains); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Join(staging, "brains"), 0700); err != nil {
		return err
	}
	for _, b := range manifest.Brains {
		if err = restoreBrain(ctx, root, scratch, staging, b); err != nil {
			return err
		}
	}

	if err = freezeRestoredControlDB(ctx, db); err != nil {
		return err
	}
	if err = closeDB(); err != nil {
		return err
	}

	if err = fsyncDir(staging); err != nil {
		return err
	}
	if _, statErr := os.Lstat(destination); statErr == nil {
		return fmt.Errorf("hosted/backup: restore destination was created concurrently: %s", destination)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if err = os.Rename(staging, destination); err != nil {
		return fmt.Errorf("hosted/backup: publish restore: %w", err)
	}
	published = true
	return fsyncDir(filepath.Dir(destination))
}

// readManifest recognizes and explicitly refuses a retired version-1
// manifest before ever attempting to parse it as ManifestV2, then validates
// the v2 shape structurally. It does not check artifact existence or
// content; the caller does that against the actual snapshot files. The
// manifest file itself must be a plain regular file: os.Root follows an
// in-root symlink by default, so this rejects one explicitly rather than
// silently reading whatever it points to.
func readManifest(root *os.Root) (contracts.ManifestV2, error) {
	info, err := root.Lstat(manifestFile)
	if err != nil {
		return contracts.ManifestV2{}, fmt.Errorf("hosted/backup: stat manifest: %w", err)
	}
	if !info.Mode().IsRegular() {
		return contracts.ManifestV2{}, fmt.Errorf("hosted/backup: manifest %s is not a regular file", manifestFile)
	}
	data, err := root.ReadFile(manifestFile)
	if err != nil {
		return contracts.ManifestV2{}, fmt.Errorf("hosted/backup: read manifest: %w", err)
	}
	var probe manifestProbe
	if err = json.Unmarshal(data, &probe); err != nil {
		return contracts.ManifestV2{}, fmt.Errorf("hosted/backup: parse manifest: %w", err)
	}
	if probe.Version == 1 {
		return contracts.ManifestV2{}, fmt.Errorf("%w: %w", ErrLegacySnapshot, contracts.ErrManifestVersion)
	}
	var manifest contracts.ManifestV2
	if err = json.Unmarshal(data, &manifest); err != nil {
		return contracts.ManifestV2{}, fmt.Errorf("hosted/backup: parse manifest: %w", err)
	}
	if err = manifest.Validate(); err != nil {
		return contracts.ManifestV2{}, err
	}
	return manifest, nil
}

// inspectRestoredControlDB opens the just-copied, not-yet-migrated artifact
// through a read-only connection (mode=ro; no WAL/SHM sidecar is ever
// created), runs an integrity check independent of the checksum already
// verified, and binds the manifest's claimed schema version to the actual
// on-disk value before store.Open (the caller's next step) is allowed to
// upgrade it.
func inspectRestoredControlDB(path string, expectedSchema int) error {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1&_pragma=query_only(1)")
	if err != nil {
		return fmt.Errorf("hosted/backup: open control database for inspection: %w", err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	var integrity string
	if err = db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil {
		return fmt.Errorf("hosted/backup: check control database integrity: %w", err)
	}
	if integrity != "ok" {
		return fmt.Errorf("hosted/backup: control database failed integrity check: %s", integrity)
	}
	var version int
	if err = db.QueryRow(`SELECT max(version) FROM schema_migrations`).Scan(&version); err != nil {
		return fmt.Errorf("hosted/backup: read control database schema version: %w", err)
	}
	if version != expectedSchema {
		return fmt.Errorf("%w: manifest claims schema %d, artifact is schema %d", ErrSchemaMismatch, expectedSchema, version)
	}
	return nil
}

// verifyAndCopy copies ref's named file from root into destPath in a single
// pass, hashing the exact bytes written, and only then compares the result
// against ref: verifying from root and separately re-reading root to copy
// would leave a window in which the source could change between the two
// reads. It never follows a symlink: the manifest names a single safe path
// element, so an entry of that name that is not a regular file is refused
// outright rather than opened.
func verifyAndCopy(root *os.Root, ref contracts.ArtifactRef, destPath string) error {
	info, err := root.Lstat(ref.RelativePath)
	if err != nil {
		return fmt.Errorf("missing artifact %s: %w", ref.RelativePath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("artifact %s is not a regular file", ref.RelativePath)
	}
	src, err := root.Open(ref.RelativePath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(destPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(dst, h), src)
	if err != nil {
		_ = dst.Close()
		return err
	}
	if err = dst.Sync(); err != nil {
		_ = dst.Close()
		return err
	}
	if err = dst.Close(); err != nil {
		return err
	}
	if n != ref.LengthBytes {
		return fmt.Errorf("artifact %s length mismatch: manifest %d, copied %d", ref.RelativePath, ref.LengthBytes, n)
	}
	if sum := hex.EncodeToString(h.Sum(nil)); sum != ref.SHA256 {
		return fmt.Errorf("artifact %s checksum mismatch", ref.RelativePath)
	}
	return nil
}

// verifyBrainInventory cross-checks the manifest's brain list against the
// restored control database's own ready-brain set. ManifestV2.Validate
// already proves the manifest's own list is sorted and duplicate-free; this
// proves it is also the *right* list -- neither missing a brain the database
// expects nor naming one the database does not have.
func verifyBrainInventory(ctx context.Context, db *store.Store, brains []contracts.BrainArtifact) error {
	rows, err := db.DB().QueryContext(ctx, `SELECT id FROM brains WHERE state='ready' AND deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return err
	}
	var expected []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		expected = append(expected, id)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	got := make([]string, len(brains))
	for i, b := range brains {
		got[i] = b.ID
	}
	if !slices.Equal(expected, got) {
		return fmt.Errorf("%w: database expects %v, manifest names %v", ErrManifestInventoryMismatch, expected, got)
	}
	return nil
}

// restoreBrain stages one brain into staging/brains/<id>. A non-empty brain's
// bundle is copied out of the untrusted snapshot into scratch (verified
// against the manifest during that single copy), and every later operation
// -- verify, clone -- reads only that verified private copy, never the
// original snapshot path again. After cloning, the restored repository's own
// local branch heads are read back and compared exactly to the manifest's
// recorded heads: a plain `git clone` also creates refs/remotes/origin/*
// tracking refs the bundle never had, and its default-branch handling is not
// itself proof every original ref survived, so this checks the actual result
// rather than trusting the clone's exit code alone.
func restoreBrain(ctx context.Context, root *os.Root, scratch, staging string, b contracts.BrainArtifact) error {
	dest := filepath.Join(staging, "brains", b.ID)
	if b.Empty {
		return os.Mkdir(dest, 0700)
	}
	bundlePath := filepath.Join(scratch, b.ID+".bundle")
	if err := verifyAndCopy(root, b.ArtifactRef, bundlePath); err != nil {
		return fmt.Errorf("hosted/backup: brain %s bundle: %w", b.ID, err)
	}
	heads, err := bundleHeads(ctx, bundlePath)
	if err != nil {
		return fmt.Errorf("hosted/backup: brain %s: %w", b.ID, err)
	}
	if !equalHeads(heads, b.Heads) {
		return fmt.Errorf("hosted/backup: brain %s bundle heads do not match the manifest", b.ID)
	}
	if output, e := exec.CommandContext(ctx, "git", "clone", "--quiet", "--", bundlePath, dest).CombinedOutput(); e != nil {
		return fmt.Errorf("hosted/backup: restore brain %s: %w: %s", b.ID, e, output)
	}
	restored, err := localHeads(ctx, dest)
	if err != nil {
		return fmt.Errorf("hosted/backup: brain %s: %w", b.ID, err)
	}
	if !equalHeads(restored, b.Heads) {
		return fmt.Errorf("hosted/backup: brain %s restored repository heads do not match the manifest", b.ID)
	}
	return nil
}

// freezeRestoredControlDB revokes every session, credential and pending OAuth
// grant the restored database records, and marks accounts/subscriptions
// pending operator reconciliation. oauth_tokens and oauth_refresh cascade
// from oauth_grants (ON DELETE CASCADE); deleting grants clears them too.
func freezeRestoredControlDB(ctx context.Context, db *store.Store) error {
	return db.Transaction(ctx, func(tx *sql.Tx) error {
		for _, statement := range []string{
			`DELETE FROM oauth_grants`, `DELETE FROM oauth_codes`, `DELETE FROM oauth_consents`,
			`DELETE FROM sessions`, `DELETE FROM login_tokens`,
			`UPDATE accounts SET status='restore_pending',plan_id='free'`,
			`UPDATE subscriptions SET status='restore_pending'`,
		} {
			if _, e := tx.ExecContext(ctx, statement); e != nil {
				return e
			}
		}
		_, e := tx.ExecContext(ctx, `UPDATE client_credentials SET revoked_at=?`, store.Stamp(time.Now()))
		return e
	})
}
