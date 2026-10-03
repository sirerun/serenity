package backup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sirerun/serenity/internal/gitrun"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/privatefs"
)

const maxInspectionCount = 10_000

type InspectionOptions struct {
	ScratchRoot            string
	ExpectedManifestSHA256 string
	MaxDeclaredBytes       int64
	MaxAccounts            int
	MaxBrains              int
	maxMetadataBytes       int64
	stageIntent            func(string, os.FileInfo, int64, int64) error
	stageAfterIntent       func()
}

type SnapshotAccount struct {
	ID     string
	Status string
}

type SnapshotInspection struct {
	ManifestSHA256        string
	Source                contracts.SourceRef
	JournalWatermark      contracts.DeletionWatermark
	Accounts              []SnapshotAccount
	Brains                []contracts.BrainArtifact
	VerifiedArtifactCount int
	DeclaredArtifactBytes int64
}

type verifiedArtifacts struct {
	inspection  SnapshotInspection
	manifest    contracts.ManifestV2
	manifestRaw []byte
	scratch     string
	scratchName string
	scratchInfo os.FileInfo
}

// InspectSnapshot verifies a private snapshot without migrating or publishing
// any of its contents. It does not authenticate who created the snapshot or
// say whether any account is eligible for recovery.
func InspectSnapshot(ctx context.Context, snapshot string, options InspectionOptions) (SnapshotInspection, error) {
	verified, err := inspectVerified(ctx, snapshot, options, false)
	return verified.inspection, err
}

func inspectVerified(ctx context.Context, snapshot string, options InspectionOptions, retain bool) (verified verifiedArtifacts, err error) {
	if isNilInterface(ctx) {
		return verifiedArtifacts{}, ErrNilContext
	}
	if err = validateInspectionOptions(options); err != nil {
		return verifiedArtifacts{}, err
	}
	if err = privatefs.ValidateDirectory(ctx, snapshot); err != nil {
		return verifiedArtifacts{}, fmt.Errorf("hosted/backup: validate snapshot directory: %w", err)
	}
	if err = privatefs.ValidateDirectory(ctx, options.ScratchRoot); err != nil {
		return verifiedArtifacts{}, fmt.Errorf("hosted/backup: validate inspection scratch root: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return verifiedArtifacts{}, err
	}
	root, err := os.OpenRoot(snapshot)
	if err != nil {
		return verifiedArtifacts{}, fmt.Errorf("hosted/backup: open snapshot root: %w", err)
	}
	defer func() {
		if closeErr := root.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("hosted/backup: close snapshot root: %w", closeErr))
		}
		if err != nil {
			verified = verifiedArtifacts{}
		}
	}()

	manifestBytes, err := readManifestBytes(ctx, root)
	if err != nil {
		return verifiedArtifacts{}, err
	}
	manifestDigest := sha256.Sum256(manifestBytes)
	manifestSHA := hex.EncodeToString(manifestDigest[:])
	if manifestSHA != options.ExpectedManifestSHA256 {
		return verifiedArtifacts{}, errors.New("hosted/backup: manifest does not match the approved digest")
	}
	if err = ctx.Err(); err != nil {
		return verifiedArtifacts{}, err
	}
	manifest, err := parseManifest(manifestBytes)
	if err != nil {
		return verifiedArtifacts{}, fmt.Errorf("hosted/backup: parse manifest: %w", err)
	}
	if options.maxMetadataBytes > 0 && int64(len(manifestBytes)) >= options.maxMetadataBytes {
		return verifiedArtifacts{}, ErrSnapshotLeaseLimit
	}
	if len(manifest.Brains) > options.MaxBrains {
		return verifiedArtifacts{}, errors.New("hosted/backup: brain inventory exceeds inspection limit")
	}
	declared, artifactCount, err := declaredArtifactSize(manifest, options.MaxDeclaredBytes)
	if err != nil {
		return verifiedArtifacts{}, err
	}

	scratchParent, err := os.OpenRoot(options.ScratchRoot)
	if err != nil {
		return verifiedArtifacts{}, fmt.Errorf("hosted/backup: open inspection scratch root: %w", err)
	}
	scratchName, err := createInspectionScratch(scratchParent)
	if err != nil {
		return verifiedArtifacts{}, errors.Join(fmt.Errorf("hosted/backup: create inspection scratch: %w", err), scratchParent.Close())
	}
	scratch := filepath.Join(options.ScratchRoot, scratchName)
	scratchIdentity, err := scratchParent.Stat(scratchName)
	if err != nil {
		return verifiedArtifacts{}, errors.Join(fmt.Errorf("hosted/backup: stat inspection scratch: %w", err), scratchParent.Close())
	}
	defer func() {
		if !retain || err != nil {
			if cleanupErr := removeInspectionScratch(scratchParent, scratchName, scratchIdentity); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("hosted/backup: remove inspection scratch: %w", cleanupErr))
			}
		}
		if closeErr := scratchParent.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("hosted/backup: close inspection scratch root: %w", closeErr))
			if retain {
				err = errors.Join(err, removeRetainedScratch(options.ScratchRoot, scratchName, scratchIdentity))
			}
		}
		if err != nil {
			verified = verifiedArtifacts{}
		}
	}()
	if err = privatefs.ValidateDirectory(ctx, scratch); err != nil {
		return verifiedArtifacts{}, fmt.Errorf("hosted/backup: validate private inspection scratch: %w", err)
	}
	if options.stageIntent != nil {
		if err = options.stageIntent(scratchName, scratchIdentity, declared, int64(len(manifestBytes))); err != nil {
			return verifiedArtifacts{}, fmt.Errorf("hosted/backup: persist stage intent: %w", err)
		}
		if options.stageAfterIntent != nil {
			options.stageAfterIntent()
		}
	}

	controlPath := filepath.Join(scratch, controlDBName)
	if _, err = verifyAndCopyContext(ctx, root, manifest.ControlDB, controlPath); err != nil {
		return verifiedArtifacts{}, fmt.Errorf("hosted/backup: control database: %w", err)
	}
	db, err := openSnapshotControlDB(ctx, controlPath, manifest.Source.SchemaVersion)
	if err != nil {
		return verifiedArtifacts{}, err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("hosted/backup: close read-only snapshot database: %w", closeErr))
		}
		if err != nil {
			verified = verifiedArtifacts{}
		}
	}()
	if err = verifyBrainInventory(ctx, db, manifest.Brains, options.MaxBrains); err != nil {
		return verifiedArtifacts{}, err
	}
	accounts, err := readSnapshotAccounts(ctx, db, options.MaxAccounts)
	if err != nil {
		return verifiedArtifacts{}, err
	}
	for _, brain := range manifest.Brains {
		if err = ctx.Err(); err != nil {
			return verifiedArtifacts{}, err
		}
		bundlePath := filepath.Join(scratch, brain.ID+".bundle")
		if _, err = verifyAndCopyContext(ctx, root, brain.ArtifactRef, bundlePath); err != nil {
			return verifiedArtifacts{}, fmt.Errorf("hosted/backup: brain %s bundle: %w", brain.ID, err)
		}
		if err = verifyStagedBrain(ctx, scratch, brain); err != nil {
			return verifiedArtifacts{}, fmt.Errorf("hosted/backup: brain %s: %w", brain.ID, err)
		}
	}
	if err = ctx.Err(); err != nil {
		return verifiedArtifacts{}, err
	}
	verified = verifiedArtifacts{
		inspection: SnapshotInspection{
			ManifestSHA256:        manifestSHA,
			Source:                manifest.Source,
			JournalWatermark:      manifest.JournalWatermark,
			Accounts:              accounts,
			Brains:                cloneBrainArtifacts(manifest.Brains),
			VerifiedArtifactCount: artifactCount,
			DeclaredArtifactBytes: declared,
		},
		manifest:    manifest,
		manifestRaw: slices.Clone(manifestBytes),
		scratch:     scratch,
		scratchName: scratchName,
		scratchInfo: scratchIdentity,
	}
	return verified, nil
}

func createInspectionScratch(parent *os.Root) (string, error) {
	for attempt := 0; attempt < 8; attempt++ {
		var suffix [16]byte
		if _, err := rand.Read(suffix[:]); err != nil {
			return "", err
		}
		name := ".serenity-snapshot-inspect-" + hex.EncodeToString(suffix[:])
		if err := parent.Mkdir(name, 0700); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return "", err
		}
		return name, nil
	}
	return "", errors.New("could not allocate a unique inspection scratch directory")
}

// removeInspectionScratch refuses to recursively remove a path that has been
// replaced since this invocation created it. The parent Root pins resolution
// to the validated scratch root; same-UID concurrent filesystem tampering is
// outside the private scratch directory's trust boundary.
func removeInspectionScratch(parent *os.Root, name string, created os.FileInfo) error {
	current, err := parent.Lstat(name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if created == nil || !os.SameFile(created, current) || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 {
		return errors.New("inspection scratch path was replaced; refusing cleanup")
	}
	return parent.RemoveAll(name)
}

func validateInspectionOptions(options InspectionOptions) error {
	if !isSHA256(options.ExpectedManifestSHA256) {
		return errors.New("hosted/backup: expected manifest digest must be 64 lowercase hex characters")
	}
	if options.MaxDeclaredBytes <= 0 || options.MaxDeclaredBytes > 1<<40 {
		return errors.New("hosted/backup: declared byte limit must be between 1 byte and 1 TiB")
	}
	if options.MaxAccounts <= 0 || options.MaxAccounts > maxInspectionCount || options.MaxBrains <= 0 || options.MaxBrains > maxInspectionCount {
		return fmt.Errorf("hosted/backup: account and brain limits must be between 1 and %d", maxInspectionCount)
	}
	return nil
}

func isSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func declaredArtifactSize(manifest contracts.ManifestV2, max int64) (int64, int, error) {
	declared := int64(0)
	count := 1 // control database
	add := func(size int64) error {
		if size <= 0 || size > math.MaxInt64-declared || declared+size > max {
			return errors.New("hosted/backup: declared artifact bytes exceed inspection limit")
		}
		declared += size
		return nil
	}
	if err := add(manifest.ControlDB.LengthBytes); err != nil {
		return 0, 0, err
	}
	for _, brain := range manifest.Brains {
		if brain.Empty {
			continue
		}
		if err := add(brain.LengthBytes); err != nil {
			return 0, 0, err
		}
		count++
	}
	return declared, count, nil
}

func cloneBrainArtifacts(brains []contracts.BrainArtifact) []contracts.BrainArtifact {
	result := make([]contracts.BrainArtifact, len(brains))
	for i, brain := range brains {
		result[i] = brain
		result[i].Heads = slices.Clone(brain.Heads)
	}
	return result
}

func readSnapshotAccounts(ctx context.Context, db *sql.DB, limit int) ([]SnapshotAccount, error) {
	rows, err := db.QueryContext(ctx, `SELECT id,status FROM accounts ORDER BY id LIMIT ?`, limit+1)
	if err != nil {
		return nil, err
	}
	accounts := make([]SnapshotAccount, 0, min(limit, 128))
	for rows.Next() {
		var account SnapshotAccount
		if err = rows.Scan(&account.ID, &account.Status); err != nil {
			_ = rows.Close()
			return nil, err
		}
		if !safeID(account.ID) || !validSnapshotAccountStatus(account.Status) {
			_ = rows.Close()
			return nil, errors.New("hosted/backup: malformed account inventory in snapshot")
		}
		accounts = append(accounts, account)
		if len(accounts) > limit {
			_ = rows.Close()
			return nil, errors.New("hosted/backup: account inventory exceeds inspection limit")
		}
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	return accounts, nil
}

func validSnapshotAccountStatus(status string) bool {
	switch status {
	case "active", "deleted", "deleting", "restore_pending":
		return true
	default:
		return false
	}
}

func verifyStagedBrain(ctx context.Context, scratch string, brain contracts.BrainArtifact) error {
	bundlePath := filepath.Join(scratch, brain.ID+".bundle")
	heads, err := bundleHeads(ctx, bundlePath)
	if err != nil {
		return err
	}
	if !equalHeads(heads, brain.Heads) {
		return errors.New("bundle heads do not match manifest")
	}
	repoPath := filepath.Join(scratch, brain.ID+".repo")
	if output, err := gitrun.CloneBundle(ctx, bundlePath, repoPath); err != nil {
		return fmt.Errorf("clone bundle: %w: %s", err, output)
	}
	if err := restoreAllBranches(ctx, repoPath); err != nil {
		return err
	}
	restored, err := localHeads(ctx, repoPath)
	if err != nil {
		return err
	}
	if !equalHeads(restored, brain.Heads) {
		return errors.New("restored repository heads do not match manifest")
	}
	return ctx.Err()
}

// Use url.URL so scratch names containing URI metacharacters cannot redirect
// SQLite to a different file or alter read-only connection options.
func sqliteReadOnlyURI(path string) string {
	u := url.URL{Scheme: "file", Path: path}
	query := url.Values{}
	query.Set("mode", "ro")
	query.Set("immutable", "1")
	query.Add("_pragma", "query_only(1)")
	u.RawQuery = query.Encode()
	return u.String()
}

// Keep an explicit source check near the URI builder so accidental callers do
// not hand arbitrary relative paths to the SQLite URI parser.
func validateScratchDBPath(path string) error {
	if !filepath.IsAbs(path) || strings.ContainsRune(path, '\x00') {
		return errors.New("hosted/backup: absolute SQLite path required")
	}
	return nil
}
