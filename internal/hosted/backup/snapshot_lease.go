package backup

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/privatefs"
	"github.com/sirerun/serenity/internal/hosted/testhooks"
)

var (
	ErrSnapshotLeaseInvalid   = errors.New("hosted/backup: invalid verified snapshot lease")
	ErrSnapshotLeaseNotFound  = errors.New("hosted/backup: verified snapshot lease not found")
	ErrSnapshotLeaseConflict  = errors.New("hosted/backup: verified snapshot lease conflict")
	ErrSnapshotLeaseClosed    = errors.New("hosted/backup: verified snapshot lease is closed")
	ErrSnapshotLeaseLimit     = errors.New("hosted/backup: verified snapshot lease limit exceeded")
	ErrSnapshotLeaseAbandoned = errors.New("hosted/backup: verified snapshot lease was abandoned")
)

const (
	leaseRecordName          = "lease.json"
	releaseRecordName        = "release.json"
	stageIntentName          = "stage-intent.json"
	maxReleaseRecordBytes    = 4096
	leaseMetadataVersion     = 1
	maxLeaseMetadataBytes    = 1 << 20
	maxLeaseBytes            = int64(1 << 40)
	maxCandidateTextBytes    = 256
	maxCandidateRows         = 64
	maxIntentMetadataBytes   = int64(4096)
	maxSnapshotPinAttempts   = 4096
	maxReleaseJournalEntries = 4096
	stageOrphanGrace         = 24 * time.Hour
)

type SnapshotLeaseStoreOptions struct {
	LeaseRoot                string
	MaxArtifactBytesPerLease int64
	MaxMetadataBytesPerLease int64
	// MaxRestoreScratchBytes bounds each lease-backed raw artifact verifier copy
	// (candidate projection or RestoreVerified scratch). It does not bound
	// expanded restore output, destination database/WAL growth, or physical quota.
	MaxRestoreScratchBytes   int64
	MaxRetainedArtifactBytes int64
	MaxRetainedMetadataBytes int64
	MaxLeases                int
}

type SnapshotLeaseStore struct {
	root            string
	rootDevice      uint64
	rootInode       uint64
	lockDevice      uint64
	lockInode       uint64
	identity        SnapshotStoreIdentity
	releaseDevice   uint64
	releaseInode    uint64
	beforeStoreLock func()
	options         SnapshotLeaseStoreOptions
	authority       SnapshotPinLifecycleAuthority
	mu              sync.Mutex
	closed          bool
}

type SnapshotPinLifecycleAuthority interface {
	BeginPinAttempt(ctx context.Context, planRef string, reservationVersion uint64, leaseID, manifestSHA256 string) (PinAttemptRef, error)
	CommitPinAttempt(ctx context.Context, attempt PinAttemptRef, pin SnapshotPinRef) error
	CancelPinAttempt(ctx context.Context, attempt PinAttemptRef, proof VerifiedPinAbsence) error
	FindPinAttempt(ctx context.Context, planRef, manifestSHA256 string) (PinAttemptRef, error)
	ListPinAttempts(ctx context.Context) ([]PinAttemptRef, error)
	ReconcilePin(ctx context.Context, pin SnapshotPinRef) (PinReconcileDecision, error)
	CompletePinRelease(ctx context.Context, authorization PinReleaseAuthorization) error
}

type snapshotStoreIdentityState struct {
	rootPath              string
	rootDevice, rootInode uint64
	lockDevice, lockInode uint64
}

// SnapshotStoreIdentity is an opaque capability tying the owner and backup
// lifecycle to one verified private root and stable lock file.
type SnapshotStoreIdentity struct{ state *snapshotStoreIdentityState }

// VerifiedPinAbsence is an opaque, callback-scoped proof that the exact staged
// lease has no durable pin while the producer's store and lease locks are held.
type VerifiedPinAbsence struct{ state *pinAbsenceState }

type pinAbsenceState struct {
	mu                              sync.Mutex
	active, consumed                bool
	attempt                         PinAttemptRef
	identity                        *snapshotStoreIdentityState
	root                            *os.Root
	storeLock, leaseLock            *os.File
	leasePath                       string
	leaseDevice, leaseInode         uint64
	leaseLockDevice, leaseLockInode uint64
}

func (p VerifiedPinAbsence) Consume(exact PinAttemptRef, expected SnapshotStoreIdentity) error {
	state := p.state
	if state == nil || expected.state == nil || state.root == nil || state.storeLock == nil || state.leaseLock == nil {
		return ErrSnapshotLeaseInvalid
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.active || state.consumed || state.identity != expected.state || !sameAttempt(state.attempt, exact) {
		return ErrSnapshotLeaseConflict
	}
	rootInfo, err := state.root.Stat(".")
	if err != nil {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	rootDev, rootIno, err := fileIdentity(rootInfo)
	if err != nil || rootDev != state.identity.rootDevice || rootIno != state.identity.rootInode {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	currentRoot, err := os.Lstat(state.identity.rootPath)
	if err != nil || !os.SameFile(rootInfo, currentRoot) {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	storeInfo, err := state.storeLock.Stat()
	if err != nil {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	lockDev, lockIno, err := fileIdentity(storeInfo)
	if err != nil || lockDev != state.identity.lockDevice || lockIno != state.identity.lockInode {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	currentLock, err := os.Lstat(filepath.Join(state.identity.rootPath, ".store.lock"))
	if err != nil || !os.SameFile(storeInfo, currentLock) {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	leaseInfo, err := os.Lstat(state.leasePath)
	if err != nil || !leaseInfo.IsDir() {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	leaseDev, leaseIno, err := fileIdentity(leaseInfo)
	if err != nil || leaseDev != state.leaseDevice || leaseIno != state.leaseInode {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	leaseLockInfo, err := state.leaseLock.Stat()
	if err != nil {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	leaseLockDev, leaseLockIno, err := fileIdentity(leaseLockInfo)
	if err != nil || leaseLockDev != state.leaseLockDevice || leaseLockIno != state.leaseLockInode {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	currentLeaseLock, err := os.Lstat(filepath.Join(state.leasePath, ".lease.lock"))
	if err != nil || !os.SameFile(leaseLockInfo, currentLeaseLock) {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	state.consumed = true
	return nil
}

func (p VerifiedPinAbsence) expire() {
	if p.state == nil {
		return
	}
	p.state.mu.Lock()
	p.state.active = false
	p.state.mu.Unlock()
}

func (p VerifiedPinAbsence) wasConsumed() bool {
	if p.state == nil {
		return false
	}
	p.state.mu.Lock()
	defer p.state.mu.Unlock()
	return p.state.consumed
}

type PinAttemptRef struct {
	PlanRef            string
	LeaseID            string
	ManifestSHA256     string
	ReservationVersion uint64
	AttemptVersion     uint64
	State              PinAttemptState
}
type PinAttemptState uint8

const (
	PinAttemptPending PinAttemptState = iota + 1
	PinAttemptCommitted
)

type PinReconcileAction uint8

const (
	PinKeep PinReconcileAction = iota + 1
	PinRelease
)

type PinReconcileDecision struct {
	Action        PinReconcileAction
	Authorization PinReleaseAuthorization
}
type PinReleaseDisposition uint8

const (
	PinAbandonedBeforeEffects PinReleaseDisposition = iota + 1
	PinCommittedRestoreComplete
)

type PinReleaseAuthorization struct {
	PinID          string
	PlanRef        string
	ManifestSHA256 string
	Disposition    PinReleaseDisposition
	RecordVersion  uint64
}

type SnapshotPinRef struct {
	id, digest, planRef string
	reservationVersion  uint64
}

func (p SnapshotPinRef) ID() string                 { return p.id }
func (p SnapshotPinRef) ManifestSHA256() string     { return p.digest }
func (p SnapshotPinRef) PlanRef() string            { return p.planRef }
func (p SnapshotPinRef) ReservationVersion() uint64 { return p.reservationVersion }

type PinnedSnapshotRef struct{ pinID, digest, planRef string }

type SnapshotCheckoutAttempt struct {
	HasSessionRef bool
	SessionRef    string
}
type VerifiedAccountCandidate interface {
	AccountID() string
	SnapshotStatus() string
	CustomerBindingCandidate() string
	CheckoutAttempts() []SnapshotCheckoutAttempt
	ManifestSHA256() string
	verifiedAccountCandidate()
}
type accountCandidate struct {
	account, status, customer, digest string
	attempts                          []SnapshotCheckoutAttempt
}

func (c *accountCandidate) AccountID() string                { return c.account }
func (c *accountCandidate) SnapshotStatus() string           { return c.status }
func (c *accountCandidate) CustomerBindingCandidate() string { return c.customer }
func (c *accountCandidate) CheckoutAttempts() []SnapshotCheckoutAttempt {
	return slices.Clone(c.attempts)
}
func (c *accountCandidate) ManifestSHA256() string  { return c.digest }
func (*accountCandidate) verifiedAccountCandidate() {}

type leaseFileRecord struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
}
type leaseDiskRecord struct {
	Version            int                   `json:"version"`
	Checksum           string                `json:"checksum"`
	ID                 string                `json:"id"`
	ManifestSHA256     string                `json:"manifest_sha256"`
	State              string                `json:"state"`
	PlanRef            string                `json:"plan_ref,omitempty"`
	PinID              string                `json:"pin_id,omitempty"`
	ReservationVersion uint64                `json:"reservation_version,omitempty"`
	AttemptVersion     uint64                `json:"attempt_version,omitempty"`
	Disposition        PinReleaseDisposition `json:"disposition,omitempty"`
	RecordVersion      uint64                `json:"record_version,omitempty"`
	ArtifactBytes      int64                 `json:"artifact_bytes"`
	MetadataBytes      int64                 `json:"metadata_bytes"`
	CreatedUnix        int64                 `json:"created_unix"`
	RootDevice         uint64                `json:"root_device"`
	RootInode          uint64                `json:"root_inode"`
	Inspection         SnapshotInspection    `json:"inspection"`
	Manifest           contracts.ManifestV2  `json:"manifest"`
	Files              []leaseFileRecord     `json:"files"`
}

type stageIntentRecord struct {
	Version       int    `json:"version"`
	Checksum      string `json:"checksum"`
	Kind          string `json:"kind"`
	ID            string `json:"id"`
	OwnerLeaseID  string `json:"owner_lease_id,omitempty"`
	ScratchName   string `json:"scratch_name"`
	RootDevice    uint64 `json:"root_device"`
	RootInode     uint64 `json:"root_inode"`
	ScratchDevice uint64 `json:"scratch_device"`
	ScratchInode  uint64 `json:"scratch_inode"`
	ArtifactBytes int64  `json:"artifact_bytes"`
	MetadataBytes int64  `json:"metadata_bytes"`
	CreatedUnix   int64  `json:"created_unix"`
}

func writeStageIntent(storeRoot, name string, intent stageIntentRecord) (retErr error) {
	if !strings.HasPrefix(name, ".serenity-snapshot-inspect-") || !isCanonicalID(intent.ID) || (intent.Kind != "stage" && intent.Kind != "candidate" && intent.Kind != "restore") || (intent.Kind != "stage" && !isCanonicalID(intent.OwnerLeaseID)) {
		return ErrSnapshotLeaseInvalid
	}
	var raw []byte
	stable := false
	for i := 0; i < 8; i++ {
		intent.Checksum = ""
		base, err := json.Marshal(intent)
		if err != nil {
			return err
		}
		intent.Checksum = digestBytes(base)
		raw, err = json.Marshal(intent)
		if err != nil {
			return err
		}
		if intent.Kind == "stage" || int64(len(raw)) == intent.MetadataBytes {
			stable = true
			break
		}
		intent.MetadataBytes = int64(len(raw))
	}
	if !stable {
		return ErrSnapshotLeaseLimit
	}
	store, err := os.OpenRoot(storeRoot)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, store.Close()) }()
	rootInfo, err := store.Stat(".")
	if err != nil {
		return err
	}
	rootDev, rootIno, err := fileIdentity(rootInfo)
	if err != nil || rootDev != intent.RootDevice || rootIno != intent.RootInode {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	root, err := store.OpenRoot(name)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	scratchInfo, err := root.Stat(".")
	if err != nil {
		return err
	}
	scratchDev, scratchIno, err := fileIdentity(scratchInfo)
	if err != nil || scratchDev != intent.ScratchDevice || scratchIno != intent.ScratchInode {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	f, err := root.OpenFile(stageIntentName, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, writeErr := f.Write(raw)
	if writeErr == nil && n != len(raw) {
		writeErr = io.ErrShortWrite
	}
	writeErr = errors.Join(writeErr, f.Sync(), f.Close())
	if writeErr != nil {
		return writeErr
	}
	if err = syncOSRoot(root); err != nil {
		return err
	}
	return syncOSRoot(store)
}

func readStageIntent(path string) (intent stageIntentRecord, retErr error) {
	f, err := openRegularFile(filepath.Join(path, stageIntentName))
	if err != nil {
		return intent, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, 4097))
	err = errors.Join(err, f.Close())
	if err != nil {
		return intent, err
	}
	if len(raw) > 4096 || rejectDuplicateJSONKeys(raw) != nil {
		return intent, ErrSnapshotLeaseInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&intent); err != nil {
		return intent, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	if d.Decode(new(any)) != io.EOF {
		return intent, ErrSnapshotLeaseInvalid
	}
	sum := intent.Checksum
	intent.Checksum = ""
	base, err := json.Marshal(intent)
	if err != nil || digestBytes(base) != sum {
		return intent, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	intent.Checksum = sum
	canonical, err := json.Marshal(intent)
	if err != nil || !bytes.Equal(canonical, raw) {
		return intent, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	if intent.Version != 1 || !isCanonicalID(intent.ID) || intent.ArtifactBytes < 0 || intent.MetadataBytes < 0 || intent.CreatedUnix <= 0 || !strings.HasPrefix(intent.ScratchName, ".serenity-snapshot-inspect-") || (intent.Kind != "stage" && intent.Kind != "candidate" && intent.Kind != "restore") || (intent.Kind != "stage" && (!isCanonicalID(intent.OwnerLeaseID) || intent.MetadataBytes <= 0 || intent.MetadataBytes > maxIntentMetadataBytes)) || (intent.Kind == "stage" && (intent.OwnerLeaseID != "" || intent.MetadataBytes <= 0)) {
		return intent, ErrSnapshotLeaseInvalid
	}
	return intent, nil
}

func removeStageIntent(storeRoot, name string, created os.FileInfo, expectedRootDevice, expectedRootInode uint64) (retErr error) {
	root, err := os.OpenRoot(storeRoot)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	rootInfo, err := root.Stat(".")
	if err != nil {
		return err
	}
	rootDevice, rootInode, err := fileIdentity(rootInfo)
	if err != nil || rootDevice != expectedRootDevice || rootInode != expectedRootInode {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if created == nil || !os.SameFile(info, created) || !info.IsDir() {
		return ErrSnapshotLeaseInvalid
	}
	scratch, err := root.OpenRoot(name)
	if err != nil {
		return err
	}
	removeErr := scratch.Remove(stageIntentName)
	removeErr = errors.Join(removeErr, syncOSRoot(scratch), scratch.Close())
	if removeErr != nil {
		return removeErr
	}
	return fsyncDir(storeRoot)
}

func syncOSRoot(root *os.Root) error {
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

type releaseTombstone struct {
	Version            int                   `json:"version"`
	Checksum           string                `json:"checksum"`
	ID                 string                `json:"id"`
	PinID              string                `json:"pin_id"`
	PlanRef            string                `json:"plan_ref"`
	ManifestSHA256     string                `json:"manifest_sha256"`
	ReservationVersion uint64                `json:"reservation_version"`
	AttemptVersion     uint64                `json:"attempt_version"`
	Disposition        PinReleaseDisposition `json:"disposition"`
	RecordVersion      uint64                `json:"record_version"`
	State              string                `json:"state"`
	MetadataBytes      int64                 `json:"metadata_bytes"`
}

func (t releaseTombstone) authorization() PinReleaseAuthorization {
	return PinReleaseAuthorization{PinID: t.PinID, PlanRef: t.PlanRef, ManifestSHA256: t.ManifestSHA256, Disposition: t.Disposition, RecordVersion: t.RecordVersion}
}

func validReleaseTombstone(t releaseTombstone) bool {
	return t.Version == 1 && isCanonicalID(t.ID) && isCanonicalID(t.PinID) && validPlanRef(t.PlanRef) && isSHA256(t.ManifestSHA256) && t.ReservationVersion > 0 && t.AttemptVersion > 0 && t.RecordVersion > 0 && t.State == "RELEASED" && (t.Disposition == PinAbandonedBeforeEffects || t.Disposition == PinCommittedRestoreComplete) && t.MetadataBytes > 0 && t.MetadataBytes <= maxReleaseRecordBytes
}

func readReleaseTombstone(path string) (t releaseTombstone, retErr error) {
	f, err := openRegularFile(filepath.Join(path, releaseRecordName))
	if err != nil {
		return t, err
	}
	defer func() { retErr = errors.Join(retErr, f.Close()) }()
	raw, err := io.ReadAll(io.LimitReader(f, maxReleaseRecordBytes+1))
	if err != nil {
		return t, err
	}
	if len(raw) > maxReleaseRecordBytes {
		return t, ErrSnapshotLeaseLimit
	}
	if err = rejectDuplicateJSONKeys(raw); err != nil {
		return t, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&t); err != nil {
		return t, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return t, ErrSnapshotLeaseInvalid
	}
	checksum := t.Checksum
	t.Checksum = ""
	base, err := json.Marshal(t)
	t.Checksum = checksum
	if err != nil || digestBytes(base) != checksum {
		return t, fmt.Errorf("%w: release tombstone checksum", ErrSnapshotLeaseInvalid)
	}
	canonical, err := json.Marshal(t)
	if err != nil || !bytes.Equal(canonical, raw) || t.MetadataBytes != int64(len(raw)) || filepath.Base(path) != t.ID || !validReleaseTombstone(t) {
		return t, fmt.Errorf("%w: release tombstone fields", ErrSnapshotLeaseInvalid)
	}
	return t, nil
}

func writeReleaseTombstone(path string, r leaseDiskRecord) (retErr error) {
	t, raw, err := encodeReleaseTombstone(r)
	if err != nil {
		return err
	}
	if t.MetadataBytes != int64(len(raw)) {
		return ErrSnapshotLeaseInvalid
	}
	before, err := os.Lstat(path)
	if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(before, opened) {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(current, opened) {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	tmpID, err := randomID()
	if err != nil {
		return err
	}
	temp := ".release.tmp-" + tmpID
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	snapshotLeaseCaptureTemp(path, temp, r, raw, root, f)
	testhooks.At(testhooks.PhaseSnapshotReleaseTombstoneTempCreated)
	n, writeErr := f.Write(raw)
	if writeErr == nil && n != len(raw) {
		writeErr = io.ErrShortWrite
	}
	if writeErr == nil {
		testhooks.At(testhooks.PhaseSnapshotReleaseTombstoneTempWritten)
		writeErr = f.Sync()
	}
	if writeErr == nil {
		testhooks.At(testhooks.PhaseSnapshotReleaseTombstoneTempSynced)
	}
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		return errors.Join(writeErr, closeErr, root.Remove(temp))
	}
	current, err = os.Lstat(path)
	if err != nil || !os.SameFile(current, opened) {
		return errors.Join(ErrSnapshotLeaseInvalid, err, root.Remove(temp))
	}
	if err = root.Rename(temp, releaseRecordName); err != nil {
		return errors.Join(err, root.Remove(temp))
	}
	testhooks.At(testhooks.PhaseSnapshotReleaseTombstonePublished)
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	err = dir.Sync()
	closeErr = dir.Close()
	if err == nil && closeErr == nil {
		testhooks.At(testhooks.PhaseSnapshotReleaseTombstoneDirectorySynced)
	}
	return errors.Join(err, closeErr)
}

type VerifiedSnapshotLease struct {
	store                *SnapshotLeaseStore
	record               leaseDiskRecord
	path                 string
	mu                   sync.Mutex
	cond                 *sync.Cond
	borrowers            int
	closing              bool
	closed               bool
	restoreActive        bool
	borrowLocks          []*os.File
	beforeCandidateOpen  func(*os.File) error
	candidateAfterIntent func()
	restoreAfterIntent   func()
}

func validateSnapshotLeaseOptions(options SnapshotLeaseStoreOptions) error {
	if !filepath.IsAbs(options.LeaseRoot) || options.LeaseRoot == "" || filepath.Clean(options.LeaseRoot) != options.LeaseRoot || options.MaxLeases < 1 || options.MaxLeases > maxInspectionCount || options.MaxArtifactBytesPerLease < 1 || options.MaxArtifactBytesPerLease > maxLeaseBytes || options.MaxMetadataBytesPerLease < 1 || options.MaxMetadataBytesPerLease > maxLeaseMetadataBytes || options.MaxRestoreScratchBytes < 1 || options.MaxRestoreScratchBytes > maxLeaseBytes || options.MaxRetainedArtifactBytes < options.MaxArtifactBytesPerLease || options.MaxRetainedArtifactBytes > maxLeaseBytes || options.MaxRetainedMetadataBytes < options.MaxMetadataBytesPerLease || options.MaxRetainedMetadataBytes > maxLeaseBytes {
		return fmt.Errorf("%w: invalid store limits or lease root", ErrSnapshotLeaseLimit)
	}
	return nil
}

// PreflightSnapshotStoreIdentity validates the configured private root and
// establishes its stable lock inode before it is bound to an owner factory.
func PreflightSnapshotStoreIdentity(ctx context.Context, options SnapshotLeaseStoreOptions) (identity SnapshotStoreIdentity, retErr error) {
	if isNilInterface(ctx) {
		return identity, ErrNilContext
	}
	if err := validateSnapshotLeaseOptions(options); err != nil {
		return identity, err
	}
	if err := ctx.Err(); err != nil {
		return identity, err
	}
	rootPath := options.LeaseRoot
	if _, err := os.Lstat(rootPath); errors.Is(err, os.ErrNotExist) {
		parentPath, base := filepath.Dir(rootPath), filepath.Base(rootPath)
		if err = privatefs.ValidateDirectory(ctx, parentPath); err != nil {
			return identity, fmt.Errorf("hosted/backup: validate lease root parent: %w", err)
		}
		parent, openErr := os.OpenRoot(parentPath)
		if openErr != nil {
			return identity, openErr
		}
		mkdirErr := parent.Mkdir(base, 0700)
		if errors.Is(mkdirErr, os.ErrExist) {
			mkdirErr = nil
		}
		if mkdirErr == nil {
			mkdirErr = syncOSRoot(parent)
		}
		retErr = errors.Join(mkdirErr, parent.Close())
		if retErr != nil {
			return identity, retErr
		}
	} else if err != nil {
		return identity, err
	}
	if err := privatefs.ValidateDirectory(ctx, rootPath); err != nil {
		return identity, fmt.Errorf("hosted/backup: validate lease root: %w", err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return identity, err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	rootInfo, err := root.Stat(".")
	if err != nil {
		return identity, err
	}
	rootDev, rootIno, err := fileIdentity(rootInfo)
	if err != nil || rootDev == 0 || rootIno == 0 {
		return identity, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	lockPath := filepath.Join(rootPath, ".store.lock")
	var lock *os.File
	for i := 0; i < 2; i++ {
		before, statErr := root.Lstat(".store.lock")
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return identity, statErr
		}
		if statErr == nil && (!before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0) {
			return identity, ErrSnapshotLeaseInvalid
		}
		lock, err = openLockFile(lockPath, errors.Is(statErr, os.ErrNotExist))
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return identity, err
		}
		break
	}
	if lock == nil {
		return identity, ErrSnapshotLeaseConflict
	}
	lockInfo, statErr := lock.Stat()
	pathInfo, pathErr := root.Lstat(".store.lock")
	if statErr != nil || pathErr != nil {
		return identity, errors.Join(ErrSnapshotLeaseInvalid, statErr, pathErr, lock.Close())
	}
	lockDev, lockIno, identityErr := fileIdentity(lockInfo)
	if statErr != nil || pathErr != nil || identityErr != nil || !os.SameFile(lockInfo, pathInfo) || !lockInfo.Mode().IsRegular() || lockInfo.Mode().Perm()&0077 != 0 {
		return identity, errors.Join(ErrSnapshotLeaseInvalid, statErr, pathErr, identityErr, lock.Close())
	}
	if err = lock.Sync(); err == nil {
		err = syncOSRoot(root)
	}
	lockCloseErr := lock.Close()
	if err != nil || lockCloseErr != nil {
		return identity, errors.Join(err, lockCloseErr)
	}
	currentRoot, rootErr := os.Lstat(rootPath)
	currentLock, lockErr := os.Lstat(lockPath)
	if rootErr != nil || lockErr != nil || !os.SameFile(rootInfo, currentRoot) || !os.SameFile(lockInfo, currentLock) {
		return identity, errors.Join(ErrSnapshotLeaseInvalid, rootErr, lockErr)
	}
	identity = SnapshotStoreIdentity{state: &snapshotStoreIdentityState{rootPath: rootPath, rootDevice: rootDev, rootInode: rootIno, lockDevice: lockDev, lockInode: lockIno}}
	return identity, nil
}

func NewSnapshotLeaseStore(ctx context.Context, options SnapshotLeaseStoreOptions, lifecycle SnapshotPinLifecycleAuthority, expected SnapshotStoreIdentity) (store *SnapshotLeaseStore, retErr error) {
	if isNilInterface(ctx) {
		return nil, ErrNilContext
	}
	if isNilInterface(lifecycle) {
		return nil, fmt.Errorf("%w: lifecycle authority is required", ErrSnapshotLeaseInvalid)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateSnapshotLeaseOptions(options); err != nil {
		return nil, err
	}
	if expected.state == nil || expected.state.rootPath != options.LeaseRoot {
		return nil, ErrSnapshotLeaseInvalid
	}
	if err := privatefs.ValidateDirectory(ctx, options.LeaseRoot); err != nil {
		return nil, fmt.Errorf("hosted/backup: validate lease root: %w", err)
	}
	rootPath := filepath.Clean(options.LeaseRoot)
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	rootInfo, err := root.Stat(".")
	if err != nil {
		return nil, err
	}
	rootDev, rootIno, err := fileIdentity(rootInfo)
	if err != nil || rootDev != expected.state.rootDevice || rootIno != expected.state.rootInode {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	lockInfo, err := root.Lstat(".store.lock")
	if err != nil || !lockInfo.Mode().IsRegular() || lockInfo.Mode().Perm()&0077 != 0 {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	lockDev, lockIno, err := fileIdentity(lockInfo)
	if err != nil || lockDev != expected.state.lockDevice || lockIno != expected.state.lockInode {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	if err = root.Mkdir(".releases", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	releases := filepath.Join(rootPath, ".releases")
	if err = privatefs.ValidateDirectory(ctx, releases); err != nil {
		return nil, err
	}
	releaseInfo, err := root.Stat(".releases")
	if err != nil || !releaseInfo.IsDir() {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	releaseDevice, releaseInode, err := fileIdentity(releaseInfo)
	if err != nil || releaseDevice == 0 || releaseInode == 0 {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	current, err := os.Lstat(rootPath)
	if err != nil || !os.SameFile(current, rootInfo) {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	finalLock, err := root.Lstat(".store.lock")
	if err != nil {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	finalLockDev, finalLockIno, finalLockIdentityErr := fileIdentity(finalLock)
	if finalLockIdentityErr != nil || finalLockDev != expected.state.lockDevice || finalLockIno != expected.state.lockInode {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, finalLockIdentityErr)
	}
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	err = dir.Sync()
	err = errors.Join(err, dir.Close())
	if err != nil {
		return nil, err
	}
	return &SnapshotLeaseStore{root: rootPath, rootDevice: rootDev, rootInode: rootIno, lockDevice: lockDev, lockInode: lockIno, identity: expected, releaseDevice: releaseDevice, releaseInode: releaseInode, options: options, authority: lifecycle}, nil
}

func (s *SnapshotLeaseStore) Stage(ctx context.Context, sourcePath string, options InspectionOptions) (lease *VerifiedSnapshotLease, retErr error) {
	if err := s.checkContext(ctx); err != nil {
		return nil, err
	}
	if !isSHA256(options.ExpectedManifestSHA256) {
		return nil, fmt.Errorf("%w: malformed expected digest", ErrSnapshotLeaseInvalid)
	}
	if options.MaxDeclaredBytes > s.options.MaxArtifactBytesPerLease {
		options.MaxDeclaredBytes = s.options.MaxArtifactBytesPerLease
	}
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	options.stageIntent = func(name string, info os.FileInfo, artifactBytes, manifestBytes int64) error {
		if err := s.validateRoot(); err != nil {
			return err
		}
		dev, ino, identityErr := fileIdentity(info)
		if identityErr != nil {
			return identityErr
		}
		currentDev, currentIno, identityErr := pathIdentity(filepath.Join(s.root, name))
		if identityErr != nil || currentDev != dev || currentIno != ino {
			return errors.Join(ErrSnapshotLeaseInvalid, identityErr)
		}
		intent := stageIntentRecord{Version: 1, Kind: "stage", ID: id, ScratchName: name, RootDevice: s.rootDevice, RootInode: s.rootInode, ScratchDevice: dev, ScratchInode: ino, ArtifactBytes: artifactBytes, MetadataBytes: s.options.MaxMetadataBytesPerLease, CreatedUnix: time.Now().Unix()}
		if manifestBytes >= intent.MetadataBytes || artifactBytes < 0 || artifactBytes > s.options.MaxArtifactBytesPerLease {
			return ErrSnapshotLeaseLimit
		}
		return writeStageIntent(s.root, name, intent)
	}
	options.ScratchRoot = s.root
	options.maxMetadataBytes = s.options.MaxMetadataBytesPerLease
	unlock, err := s.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, unlock()) }()
	usage, err := s.usageLocked()
	if err != nil {
		return nil, err
	}
	if usage.leases >= s.options.MaxLeases || usage.releaseEntries >= maxReleaseJournalEntries {
		return nil, ErrSnapshotLeaseLimit
	}
	if s.options.MaxMetadataBytesPerLease > s.options.MaxRetainedMetadataBytes-usage.metadata {
		return nil, ErrSnapshotLeaseLimit
	}
	remainingArtifacts := s.options.MaxRetainedArtifactBytes - usage.artifacts
	if remainingArtifacts < 1 {
		return nil, ErrSnapshotLeaseLimit
	}
	if options.MaxDeclaredBytes > remainingArtifacts {
		options.MaxDeclaredBytes = remainingArtifacts
	}
	if options.MaxDeclaredBytes > s.options.MaxArtifactBytesPerLease {
		options.MaxDeclaredBytes = s.options.MaxArtifactBytesPerLease
	}
	verified, err := inspectVerified(ctx, sourcePath, options, true)
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			retErr = errors.Join(retErr, removeRetainedScratch(s.root, verified.scratchName, verified.scratchInfo))
		}
	}()
	if err = s.validateRoot(); err != nil {
		return nil, err
	}
	if verified.inspection.DeclaredArtifactBytes > s.options.MaxArtifactBytesPerLease || verified.inspection.DeclaredArtifactBytes > s.options.MaxRetainedArtifactBytes-usage.artifacts {
		return nil, ErrSnapshotLeaseLimit
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(verified.scratch, manifestFile), verified.manifestRaw, 0600); err != nil {
		return nil, err
	}
	record := leaseDiskRecord{Version: leaseMetadataVersion, ID: id, ManifestSHA256: verified.inspection.ManifestSHA256, State: "STAGED", ArtifactBytes: verified.inspection.DeclaredArtifactBytes, CreatedUnix: time.Now().Unix(), Inspection: cloneInspection(verified.inspection), Manifest: verified.manifest}
	for _, name := range append([]string{controlDBName}, bundleNames(verified.manifest)...) {
		frec, e := inspectOwnedFile(filepath.Join(verified.scratch, name), name)
		if e != nil {
			return nil, e
		}
		record.Files = append(record.Files, frec)
	}
	frec, err := inspectOwnedFile(filepath.Join(verified.scratch, manifestFile), manifestFile)
	if err != nil {
		return nil, err
	}
	record.Files = append(record.Files, frec)
	if record.RootDevice, record.RootInode, err = pathIdentity(verified.scratch); err != nil {
		return nil, err
	}
	if err = writeLeaseRecord(verified.scratch, &record, s.options.MaxMetadataBytesPerLease); err != nil {
		return nil, err
	}
	if record.MetadataBytes > s.options.MaxRetainedMetadataBytes-usage.metadata {
		return nil, ErrSnapshotLeaseLimit
	}
	if err = syncTree(verified.scratch); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	leasePath := filepath.Join(s.root, id)
	if err = s.validateRoot(); err != nil {
		return nil, err
	}
	if err = os.Rename(verified.scratch, leasePath); err != nil {
		return nil, err
	}
	keep = true // The renamed STAGED record is durable inventory even if the parent sync fails.
	if err = removeStageIntent(s.root, id, verified.scratchInfo, s.rootDevice, s.rootInode); err != nil {
		return nil, err
	}
	if err = s.validateRoot(); err != nil {
		return nil, err
	}
	if err = fsyncDir(s.root); err != nil {
		return nil, err
	}
	return newLease(s, record, leasePath), nil
}

func (s *SnapshotLeaseStore) FindPinned(ctx context.Context, planRef, expectedManifestSHA256 string) (pin SnapshotPinRef, retErr error) {
	if err := validatePlanDigest(planRef, expectedManifestSHA256); err != nil {
		return SnapshotPinRef{}, err
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return SnapshotPinRef{}, err
	}
	defer func() { retErr = errors.Join(retErr, unlock()) }()
	var match *leaseDiskRecord
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return SnapshotPinRef{}, err
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		r, e2 := readLeaseRecord(filepath.Join(s.root, e.Name()), s.options.MaxMetadataBytesPerLease)
		if e2 != nil {
			return SnapshotPinRef{}, e2
		}
		if r.State == "PINNED" && r.PlanRef == planRef {
			if match != nil || r.ManifestSHA256 != expectedManifestSHA256 {
				return SnapshotPinRef{}, ErrSnapshotLeaseConflict
			}
			match = &r
		}
	}
	if match == nil {
		return SnapshotPinRef{}, ErrSnapshotLeaseNotFound
	}
	attempt, attemptErr := s.authority.FindPinAttempt(ctx, planRef, expectedManifestSHA256)
	if attemptErr != nil {
		return SnapshotPinRef{}, attemptErr
	}
	if attempt.State != PinAttemptCommitted || attempt.PlanRef != match.PlanRef || attempt.LeaseID != match.ID || attempt.ManifestSHA256 != match.ManifestSHA256 || attempt.ReservationVersion != match.ReservationVersion || attempt.AttemptVersion != match.AttemptVersion {
		return SnapshotPinRef{}, ErrSnapshotLeaseConflict
	}
	return pinRef(*match), nil
}

func (s *SnapshotLeaseStore) ResumePin(ctx context.Context, planRef string, reservationVersion uint64, expectedManifestSHA256 string) (pin SnapshotPinRef, retErr error) {
	if err := validatePlanDigest(planRef, expectedManifestSHA256); err != nil || reservationVersion == 0 {
		return SnapshotPinRef{}, ErrSnapshotLeaseInvalid
	}
	if pin, err := s.FindPinned(ctx, planRef, expectedManifestSHA256); err == nil {
		if pin.ReservationVersion() != reservationVersion {
			return SnapshotPinRef{}, ErrSnapshotLeaseConflict
		}
		return pin, nil
	} else if !errors.Is(err, ErrSnapshotLeaseNotFound) {
		return SnapshotPinRef{}, err
	}
	attempt, err := s.authority.FindPinAttempt(ctx, planRef, expectedManifestSHA256)
	if err != nil {
		return SnapshotPinRef{}, err
	}
	if !attemptMatches(attempt, planRef, attempt.LeaseID, expectedManifestSHA256, reservationVersion) || (attempt.State != PinAttemptPending && attempt.State != PinAttemptCommitted) {
		return SnapshotPinRef{}, ErrSnapshotLeaseConflict
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return SnapshotPinRef{}, err
	}
	defer func() { retErr = errors.Join(retErr, unlock()) }()
	record, err := readLeaseRecord(filepath.Join(s.root, attempt.LeaseID), s.options.MaxMetadataBytesPerLease)
	if err != nil {
		return SnapshotPinRef{}, err
	}
	if record.ManifestSHA256 != expectedManifestSHA256 {
		return SnapshotPinRef{}, ErrSnapshotLeaseConflict
	}
	if record.State == "PIN_PENDING" {
		if record.PlanRef != planRef || record.ReservationVersion != reservationVersion || record.AttemptVersion != attempt.AttemptVersion || record.PinID == "" {
			return SnapshotPinRef{}, ErrSnapshotLeaseConflict
		}
		pin := pinRef(record)
		if attempt.State == PinAttemptPending {
			if err = s.authority.CommitPinAttempt(ctx, attempt, pin); err != nil {
				return SnapshotPinRef{}, err
			}
		} else if attempt.State != PinAttemptCommitted {
			return SnapshotPinRef{}, ErrSnapshotLeaseConflict
		}
		record.State = "PINNED"
		if err = s.validateRoot(); err != nil {
			return SnapshotPinRef{}, err
		}
		if err = writeLeaseRecord(filepath.Join(s.root, record.ID), &record, s.options.MaxMetadataBytesPerLease); err != nil {
			return SnapshotPinRef{}, err
		}
		return pin, nil
	}
	if record.State != "STAGED" || attempt.State != PinAttemptPending {
		return SnapshotPinRef{}, ErrSnapshotLeaseConflict
	}
	return s.finishPinLocked(ctx, record, attempt)
}

func (s *SnapshotLeaseStore) ResolvePinned(ctx context.Context, pinID, expectedManifestSHA256, planRef string) (ref PinnedSnapshotRef, retErr error) {
	if !isCanonicalID(pinID) || validatePlanDigest(planRef, expectedManifestSHA256) != nil {
		return PinnedSnapshotRef{}, ErrSnapshotLeaseInvalid
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return PinnedSnapshotRef{}, err
	}
	defer func() { retErr = errors.Join(retErr, unlock()) }()
	r, err := s.findPinRecordLocked(pinID)
	if err != nil {
		return PinnedSnapshotRef{}, err
	}
	if r.State != "PINNED" || r.PinID != pinID || r.PlanRef != planRef || r.ManifestSHA256 != expectedManifestSHA256 {
		return PinnedSnapshotRef{}, ErrSnapshotLeaseConflict
	}
	decision, err := s.authority.ReconcilePin(ctx, pinRef(r))
	if err != nil {
		return PinnedSnapshotRef{}, err
	}
	if decision.Action == PinRelease {
		return PinnedSnapshotRef{}, ErrSnapshotLeaseAbandoned
	}
	if decision.Action != PinKeep {
		return PinnedSnapshotRef{}, ErrSnapshotLeaseConflict
	}
	return PinnedSnapshotRef{pinID: pinID, digest: expectedManifestSHA256, planRef: planRef}, nil
}

func (s *SnapshotLeaseStore) ReopenPinned(ctx context.Context, ref PinnedSnapshotRef) (lease *VerifiedSnapshotLease, retErr error) {
	if !isCanonicalID(ref.pinID) || validatePlanDigest(ref.planRef, ref.digest) != nil {
		return nil, ErrSnapshotLeaseInvalid
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, unlock()) }()
	r, err := s.findPinRecordLocked(ref.pinID)
	if err != nil {
		return nil, err
	}
	if r.State != "PINNED" || r.PinID != ref.pinID || r.PlanRef != ref.planRef || r.ManifestSHA256 != ref.digest {
		return nil, ErrSnapshotLeaseConflict
	}
	decision, err := s.authority.ReconcilePin(ctx, pinRef(r))
	if err != nil {
		return nil, err
	}
	if decision.Action != PinKeep {
		return nil, ErrSnapshotLeaseAbandoned
	}
	if err = verifyRecordFiles(ctx, filepath.Join(s.root, r.ID), r); err != nil {
		return nil, err
	}
	return newLease(s, r, filepath.Join(s.root, r.ID)), nil
}

func (s *SnapshotLeaseStore) findPinRecordLocked(pinID string) (leaseDiskRecord, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return leaseDiskRecord{}, err
	}
	var match *leaseDiskRecord
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		r, e := readLeaseRecord(filepath.Join(s.root, e.Name()), s.options.MaxMetadataBytesPerLease)
		if e != nil {
			return leaseDiskRecord{}, e
		}
		if r.State == "PINNED" && r.PinID == pinID {
			if match != nil {
				return leaseDiskRecord{}, ErrSnapshotLeaseConflict
			}
			match = &r
		}
	}
	if match == nil {
		return leaseDiskRecord{}, ErrSnapshotLeaseNotFound
	}
	return *match, nil
}

func (l *VerifiedSnapshotLease) LeaseID() string { return l.record.ID }
func (l *VerifiedSnapshotLease) Inspection() SnapshotInspection {
	return cloneInspection(l.record.Inspection)
}

func (l *VerifiedSnapshotLease) prepareCandidateControlDB(ctx context.Context) (path string, projection *os.File, digest string, size int64, cleanup func() error, retErr error) {
	var sourceRecord *leaseFileRecord
	for i := range l.record.Files {
		if l.record.Files[i].Name == controlDBName {
			sourceRecord = &l.record.Files[i]
			break
		}
	}
	if sourceRecord == nil || sourceRecord.Size > l.store.options.MaxRestoreScratchBytes {
		return "", nil, "", 0, nil, ErrSnapshotLeaseLimit
	}
	dev, ino, err := pathIdentity(l.path)
	if err != nil || dev != l.record.RootDevice || ino != l.record.RootInode {
		return "", nil, "", 0, nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	source, err := openRegularFile(filepath.Join(l.path, controlDBName))
	if err != nil {
		return "", nil, "", 0, nil, err
	}
	sourceInfo, err := source.Stat()
	if err != nil {
		return "", nil, "", 0, nil, errors.Join(err, source.Close())
	}
	sourceDev, sourceIno, err := fileIdentity(sourceInfo)
	if err != nil || sourceDev != sourceRecord.Device || sourceIno != sourceRecord.Inode || sourceInfo.Size() != sourceRecord.Size || !sourceInfo.Mode().IsRegular() {
		return "", nil, "", 0, nil, errors.Join(ErrSnapshotLeaseInvalid, err, source.Close())
	}
	parent, err := os.OpenRoot(l.store.root)
	if err != nil {
		return "", nil, "", 0, nil, errors.Join(err, source.Close())
	}
	parentInfo, err := parent.Stat(".")
	if err != nil {
		return "", nil, "", 0, nil, errors.Join(err, parent.Close(), source.Close())
	}
	parentDev, parentIno, identityErr := fileIdentity(parentInfo)
	if identityErr != nil || parentDev != l.store.rootDevice || parentIno != l.store.rootInode {
		return "", nil, "", 0, nil, errors.Join(ErrSnapshotLeaseInvalid, identityErr, parent.Close(), source.Close())
	}
	name, err := createInspectionScratch(parent)
	if err != nil {
		return "", nil, "", 0, nil, errors.Join(err, parent.Close(), source.Close())
	}
	created, err := parent.Stat(name)
	if err != nil {
		return "", nil, "", 0, nil, errors.Join(err, parent.Close(), source.Close())
	}
	cleanup = func() error { return removeRetainedScratch(l.store.root, name, created) }
	intentID, err := randomID()
	if err != nil {
		return "", nil, "", 0, cleanup, errors.Join(err, cleanup())
	}
	dev, ino, err = fileIdentity(created)
	if err != nil {
		return "", nil, "", 0, cleanup, errors.Join(err, cleanup())
	}
	intent := stageIntentRecord{Version: 1, Kind: "candidate", ID: intentID, OwnerLeaseID: l.record.ID, ScratchName: name, RootDevice: l.store.rootDevice, RootInode: l.store.rootInode, ScratchDevice: dev, ScratchInode: ino, ArtifactBytes: sourceRecord.Size, CreatedUnix: time.Now().Unix()}
	if err = l.store.validateRoot(); err != nil {
		return "", nil, "", 0, cleanup, errors.Join(err, cleanup())
	}
	if err = writeStageIntent(l.store.root, name, intent); err != nil {
		return "", nil, "", 0, cleanup, errors.Join(err, cleanup())
	}
	if l.candidateAfterIntent != nil {
		l.candidateAfterIntent()
	}
	usage, err := l.store.usageLocked()
	if err != nil || usage.artifacts > l.store.options.MaxRetainedArtifactBytes {
		return "", nil, "", 0, cleanup, errors.Join(err, ErrSnapshotLeaseLimit, cleanup())
	}
	if err = l.store.validateRoot(); err != nil {
		return "", nil, "", 0, cleanup, errors.Join(err, cleanup())
	}
	output, err := parent.OpenFile(filepath.Join(name, controlDBName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", nil, "", 0, cleanup, errors.Join(err, parent.Close(), source.Close(), cleanup())
	}
	h := sha256.New()
	copied, copyErr := copyContext(ctx, io.MultiWriter(output, h), source, sourceRecord.Size)
	copyErr = errors.Join(copyErr, source.Close())
	if copyErr == nil && copied == sourceRecord.Size && hex.EncodeToString(h.Sum(nil)) == sourceRecord.SHA256 {
		copyErr = output.Sync()
	} else if copyErr == nil {
		copyErr = ErrSnapshotLeaseInvalid
	}
	if copyErr == nil {
		copyErr = output.Chmod(0400)
	}
	copyErr = errors.Join(copyErr, output.Close())
	parentCloseErr := parent.Close()
	if copyErr != nil || parentCloseErr != nil {
		return "", nil, "", 0, cleanup, errors.Join(copyErr, parentCloseErr, cleanup())
	}
	path = filepath.Join(l.store.root, name, controlDBName)
	projectedInfo, err := os.Lstat(path)
	if err != nil || !projectedInfo.Mode().IsRegular() || projectedInfo.Mode().Perm()&0222 != 0 {
		return "", nil, "", 0, cleanup, errors.Join(ErrSnapshotLeaseInvalid, err, cleanup())
	}
	projection, err = openRegularFile(path)
	if err != nil {
		return "", nil, "", 0, cleanup, errors.Join(err, cleanup())
	}
	openedInfo, err := projection.Stat()
	if err != nil || !os.SameFile(projectedInfo, openedInfo) || openedInfo.Size() != sourceRecord.Size {
		return "", nil, "", 0, cleanup, errors.Join(ErrSnapshotLeaseInvalid, err, projection.Close(), cleanup())
	}
	if err = verifyCandidateProjection(ctx, projection, sourceRecord.SHA256, sourceRecord.Size); err != nil {
		return "", nil, "", 0, cleanup, errors.Join(err, projection.Close(), cleanup())
	}
	return path, projection, sourceRecord.SHA256, sourceRecord.Size, cleanup, nil
}

func verifyCandidateProjection(ctx context.Context, file *os.File, digest string, size int64) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	h := sha256.New()
	n, err := copyContext(ctx, h, file, size)
	if err != nil {
		return err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if n != size || hex.EncodeToString(h.Sum(nil)) != digest {
		return ErrSnapshotLeaseInvalid
	}
	return nil
}

func (l *VerifiedSnapshotLease) Candidate(ctx context.Context, accountID string) (candidate VerifiedAccountCandidate, retErr error) {
	if !safeID(accountID) {
		return nil, ErrSnapshotLeaseInvalid
	}
	unlockStore, err := l.store.lock(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, unlockStore()) }()
	if err := l.beginBorrow(ctx, false); err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, l.endBorrow()) }()
	if err := verifyRecordFiles(ctx, l.path, l.record); err != nil {
		return nil, err
	}
	_, projectionFile, projectionDigest, projectionSize, cleanupProjection, err := l.prepareCandidateControlDB(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, cleanupProjection()) }()
	defer func() { retErr = errors.Join(retErr, projectionFile.Close()) }()
	fdPath := filepath.Join("/dev/fd", strconv.FormatUint(uint64(projectionFile.Fd()), 10))
	if l.beforeCandidateOpen != nil {
		if err = l.beforeCandidateOpen(projectionFile); err != nil {
			return nil, err
		}
	}
	db, err := openSnapshotControlDB(ctx, fdPath, l.record.Manifest.Source.SchemaVersion)
	if err != nil {
		return nil, err
	}
	dbClosed := false
	defer func() {
		if !dbClosed {
			retErr = errors.Join(retErr, db.Close())
		}
	}()
	var idLen, statusLen, customerLen, checkoutCount, maxSessionLen int
	err = db.QueryRowContext(ctx, `SELECT length(CAST(a.id AS BLOB)),length(CAST(a.status AS BLOB)),length(CAST(COALESCE(a.stripe_customer_id,'') AS BLOB)),(SELECT count(*) FROM checkout_attempts c WHERE c.account_id=a.id),(SELECT COALESCE(max(length(CAST(COALESCE(c.session_id,'') AS BLOB))),0) FROM checkout_attempts c WHERE c.account_id=a.id) FROM accounts a WHERE a.id=?`, accountID).Scan(&idLen, &statusLen, &customerLen, &checkoutCount, &maxSessionLen)
	if err != nil {
		return nil, err
	}
	if idLen < 1 || idLen > 128 || statusLen < 1 || statusLen > 32 || customerLen > maxCandidateTextBytes || checkoutCount > maxCandidateRows || maxSessionLen > maxCandidateTextBytes {
		return nil, ErrSnapshotLeaseLimit
	}
	var account, status, customer string
	err = db.QueryRowContext(ctx, `SELECT id,status,COALESCE(stripe_customer_id,'') FROM accounts WHERE id=?`, accountID).Scan(&account, &status, &customer)
	if err != nil {
		return nil, err
	}
	if !safeID(account) || !validSnapshotAccountStatus(status) || !validCandidateText(customer) {
		return nil, ErrSnapshotLeaseInvalid
	}
	rows, err := db.QueryContext(ctx, `SELECT session_id FROM checkout_attempts WHERE account_id=? ORDER BY COALESCE(session_id,'')`, accountID)
	if err != nil {
		return nil, err
	}
	rowsClosed := false
	defer func() {
		if !rowsClosed {
			retErr = errors.Join(retErr, rows.Close())
		}
	}()
	attempts := make([]SnapshotCheckoutAttempt, 0, checkoutCount)
	seen := map[string]bool{}
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		var session sql.NullString
		if err = rows.Scan(&session); err != nil {
			return nil, err
		}
		item := SnapshotCheckoutAttempt{}
		if session.Valid {
			item.HasSessionRef = true
			item.SessionRef = session.String
			if !validCandidateText(item.SessionRef) || seen[item.SessionRef] {
				return nil, ErrSnapshotLeaseInvalid
			}
			seen[item.SessionRef] = true
		}
		attempts = append(attempts, item)
		if len(attempts) > maxCandidateRows {
			return nil, ErrSnapshotLeaseLimit
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = rows.Close(); err != nil {
		rowsClosed = true
		return nil, err
	}
	rowsClosed = true
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = verifyCandidateProjection(ctx, projectionFile, projectionDigest, projectionSize); err != nil {
		return nil, err
	}
	if err = db.Close(); err != nil {
		dbClosed = true
		return nil, err
	}
	dbClosed = true
	return &accountCandidate{account: account, status: status, customer: customer, digest: l.record.ManifestSHA256, attempts: attempts}, nil
}

func (l *VerifiedSnapshotLease) Pin(ctx context.Context, planRef string, reservationVersion uint64, expectedManifestSHA256 string) (pin SnapshotPinRef, retErr error) {
	if validatePlanDigest(planRef, expectedManifestSHA256) != nil || reservationVersion == 0 || expectedManifestSHA256 != l.record.ManifestSHA256 {
		return SnapshotPinRef{}, ErrSnapshotLeaseInvalid
	}
	unlock, err := l.store.lock(ctx)
	if err != nil {
		return SnapshotPinRef{}, err
	}
	defer func() { retErr = errors.Join(retErr, unlock()) }()
	if err := l.beginBorrow(ctx, false); err != nil {
		return SnapshotPinRef{}, fmt.Errorf("lock pin borrower: %w", err)
	}
	defer func() { retErr = errors.Join(retErr, l.endBorrow()) }()
	current, err := readLeaseRecord(l.path, l.store.options.MaxMetadataBytesPerLease)
	if err != nil {
		return SnapshotPinRef{}, fmt.Errorf("read pin lease record: %w", err)
	}
	if current.State == "PINNED" {
		if current.PlanRef == planRef && current.ReservationVersion == reservationVersion && current.ManifestSHA256 == expectedManifestSHA256 {
			decision, e := l.store.authority.ReconcilePin(ctx, pinRef(current))
			if e != nil {
				return SnapshotPinRef{}, e
			}
			if decision.Action != PinKeep {
				return SnapshotPinRef{}, ErrSnapshotLeaseAbandoned
			}
			return pinRef(current), nil
		}
		return SnapshotPinRef{}, ErrSnapshotLeaseConflict
	}
	if current.State != "STAGED" {
		return SnapshotPinRef{}, ErrSnapshotLeaseClosed
	}
	if err = verifyRecordFiles(ctx, l.path, current); err != nil {
		return SnapshotPinRef{}, fmt.Errorf("verify pin lease files: %w", err)
	}
	attempt, err := l.store.authority.BeginPinAttempt(ctx, planRef, reservationVersion, current.ID, current.ManifestSHA256)
	if err != nil {
		return SnapshotPinRef{}, err
	}
	if !attemptMatches(attempt, planRef, current.ID, current.ManifestSHA256, reservationVersion) || attempt.State != PinAttemptPending {
		return SnapshotPinRef{}, ErrSnapshotLeaseConflict
	}
	pin, err = l.store.finishPinLocked(ctx, current, attempt)
	if err != nil {
		return SnapshotPinRef{}, fmt.Errorf("finish pin transaction: %w", err)
	}
	return pin, nil
}

func (s *SnapshotLeaseStore) finishPinLocked(ctx context.Context, record leaseDiskRecord, attempt PinAttemptRef) (SnapshotPinRef, error) {
	if !attemptMatches(attempt, attempt.PlanRef, record.ID, record.ManifestSHA256, attempt.ReservationVersion) || attempt.State != PinAttemptPending {
		return SnapshotPinRef{}, ErrSnapshotLeaseConflict
	}
	id, err := randomID()
	if err != nil {
		return SnapshotPinRef{}, err
	}
	record.State = "PIN_PENDING"
	record.PlanRef = attempt.PlanRef
	record.PinID = id
	record.ReservationVersion = attempt.ReservationVersion
	record.AttemptVersion = attempt.AttemptVersion
	previousMetadata := record.MetadataBytes
	candidate := record
	if _, err = encodeLeaseRecord(&candidate, s.options.MaxMetadataBytesPerLease); err != nil {
		return SnapshotPinRef{}, err
	}
	usage, err := s.usageLocked()
	if err != nil {
		return SnapshotPinRef{}, err
	}
	growth := candidate.MetadataBytes - previousMetadata
	if growth < 0 || usage.metadata > s.options.MaxRetainedMetadataBytes || growth > s.options.MaxRetainedMetadataBytes-usage.metadata {
		return SnapshotPinRef{}, ErrSnapshotLeaseLimit
	}
	if err = s.validateRoot(); err != nil {
		return SnapshotPinRef{}, err
	}
	if err = writeLeaseRecord(filepath.Join(s.root, record.ID), &record, s.options.MaxMetadataBytesPerLease); err != nil {
		return SnapshotPinRef{}, fmt.Errorf("write PIN_PENDING lease record: %w", err)
	}
	if err = fsyncDir(filepath.Join(s.root, record.ID)); err != nil {
		return SnapshotPinRef{}, fmt.Errorf("sync PIN_PENDING lease directory: %w", err)
	}
	if err = fsyncDir(s.root); err != nil {
		return SnapshotPinRef{}, fmt.Errorf("sync PIN_PENDING store root: %w", err)
	}
	testhooks.At(testhooks.PhaseSnapshotPinPendingRootSynced)
	testhooks.At(testhooks.PhaseSnapshotPinPendingDurable)
	pin := pinRef(record)
	if err = s.authority.CommitPinAttempt(ctx, attempt, pin); err != nil {
		return SnapshotPinRef{}, err
	}
	testhooks.At(testhooks.PhaseSnapshotPinOwnerCommitted)
	if err = ctx.Err(); err != nil {
		return SnapshotPinRef{}, err
	}
	record.State = "PINNED"
	if err = s.validateRoot(); err != nil {
		return SnapshotPinRef{}, err
	}
	if err = writeLeaseRecord(filepath.Join(s.root, record.ID), &record, s.options.MaxMetadataBytesPerLease); err != nil {
		return SnapshotPinRef{}, err
	}
	if err = fsyncDir(filepath.Join(s.root, record.ID)); err != nil {
		return SnapshotPinRef{}, err
	}
	if err = fsyncDir(s.root); err != nil {
		return SnapshotPinRef{}, err
	}
	testhooks.At(testhooks.PhaseSnapshotPinPinnedRootSynced)
	testhooks.At(testhooks.PhaseSnapshotPinPromoted)
	return pin, nil
}

func (s *SnapshotLeaseStore) CancelPin(ctx context.Context, attempt PinAttemptRef) (retErr error) {
	if !validPlanRef(attempt.PlanRef) || !isCanonicalID(attempt.LeaseID) || !isSHA256(attempt.ManifestSHA256) || attempt.ReservationVersion == 0 || attempt.AttemptVersion == 0 {
		return ErrSnapshotLeaseInvalid
	}
	storeLock, unlock, err := s.lockHandle(ctx)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, unlock()) }()
	path := filepath.Join(s.root, attempt.LeaseID)
	leaseInfo, err := os.Lstat(path)
	if err != nil {
		return errors.Join(ErrSnapshotLeaseConflict, err)
	}
	if !leaseInfo.IsDir() {
		return ErrSnapshotLeaseInvalid
	}
	leaseLock, err := lockLeaseContext(ctx, path, false)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, unlockFile(leaseLock), leaseLock.Close()) }()
	r, err := readLeaseRecord(path, s.options.MaxMetadataBytesPerLease)
	if err != nil {
		return err
	}
	if r.ID != attempt.LeaseID || r.ManifestSHA256 != attempt.ManifestSHA256 || r.State != "STAGED" || r.PinID != "" || r.PlanRef != "" || r.ReservationVersion != 0 || r.AttemptVersion != 0 {
		return ErrSnapshotLeaseConflict
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return err
	}
	rootInfo, err := root.Stat(".")
	if err != nil {
		return errors.Join(err, root.Close())
	}
	leaseLockInfo, err := leaseLock.Stat()
	if err != nil {
		return errors.Join(err, root.Close())
	}
	rootDev, rootIno, e1 := fileIdentity(rootInfo)
	leaseDev, leaseIno, e2 := fileIdentity(leaseInfo)
	leaseLockDev, leaseLockIno, e3 := fileIdentity(leaseLockInfo)
	currentLease, currentErr := os.Lstat(path)
	if e1 != nil || e2 != nil || e3 != nil || currentErr != nil || !os.SameFile(leaseInfo, currentLease) || s.identity.state == nil || rootDev != s.identity.state.rootDevice || rootIno != s.identity.state.rootInode {
		return errors.Join(ErrSnapshotLeaseInvalid, e1, e2, e3, currentErr, root.Close())
	}
	proof := VerifiedPinAbsence{state: &pinAbsenceState{active: true, attempt: attempt, identity: s.identity.state, root: root, storeLock: storeLock, leaseLock: leaseLock, leasePath: path, leaseDevice: leaseDev, leaseInode: leaseIno, leaseLockDevice: leaseLockDev, leaseLockInode: leaseLockIno}}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	defer proof.expire()
	// The owner implements exact-attempt CAS and durable canceled tombstones.
	// It must not consult or mutate a later attempt when this is a retry of N.
	if err = s.authority.CancelPinAttempt(ctx, attempt, proof); err != nil {
		return err
	}
	if !proof.wasConsumed() {
		return ErrSnapshotLeaseConflict
	}
	return ctx.Err()
}

func (s *SnapshotLeaseStore) Reconcile(ctx context.Context) (retErr error) {
	unlock, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, unlock()) }()
	attempts, err := s.authority.ListPinAttempts(ctx)
	if err != nil {
		return err
	}
	if err = validatePinAttemptList(attempts); err != nil {
		return err
	}
	if err = validateLocalPinAttemptPairs(s.root, s.options.MaxMetadataBytesPerLease, attempts); err != nil {
		return err
	}
	if err = s.finishReleaseJournal(ctx, attempts); err != nil {
		return err
	}
	protected := map[string]bool{}
	for _, a := range attempts {
		protected[a.LeaseID] = true
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	var errs []error
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ".serenity-snapshot-inspect-") {
			continue
		}
		path := filepath.Join(s.root, entry.Name())
		intent, intentErr := readStageIntent(path)
		if errors.Is(intentErr, os.ErrNotExist) {
			continue
		}
		if intentErr != nil {
			errs = append(errs, intentErr)
			continue
		}
		rootDev, rootIno, identityErr := pathIdentity(s.root)
		dev, ino, scratchErr := pathIdentity(path)
		if identityErr != nil || scratchErr != nil || rootDev != intent.RootDevice || rootIno != intent.RootInode || dev != intent.ScratchDevice || ino != intent.ScratchInode || intent.ScratchName != entry.Name() {
			errs = append(errs, errors.Join(ErrSnapshotLeaseInvalid, identityErr, scratchErr))
			continue
		}
		created, statErr := os.Lstat(path)
		identityErr = errors.Join(identityErr, statErr)
		if intent.Kind == "candidate" || intent.Kind == "restore" {
			ownerPath := filepath.Join(s.root, intent.OwnerLeaseID)
			owner, ownerErr := readLeaseRecord(ownerPath, s.options.MaxMetadataBytesPerLease)
			if ownerErr != nil || owner.ID != intent.OwnerLeaseID {
				errs = append(errs, errors.Join(ErrSnapshotLeaseInvalid, ownerErr))
				continue
			}
			leaseLock, lockErr := lockLeaseContext(ctx, ownerPath, false)
			if lockErr != nil {
				errs = append(errs, lockErr)
				continue
			}
			if identityErr == nil {
				identityErr = s.validateRoot()
			}
			if identityErr == nil {
				identityErr = removeRetainedScratch(s.root, entry.Name(), created)
			}
			identityErr = errors.Join(identityErr, unlockFile(leaseLock), leaseLock.Close())
			if identityErr == nil {
				identityErr = fsyncDir(s.root)
			}
			if identityErr != nil {
				errs = append(errs, identityErr)
			}
			continue
		}
		if identityErr == nil {
			identityErr = s.validateRoot()
		}
		if identityErr == nil {
			identityErr = removeRetainedScratch(s.root, entry.Name(), created)
		}
		if identityErr == nil {
			identityErr = fsyncDir(s.root)
		}
		if identityErr != nil {
			errs = append(errs, identityErr)
		}
	}
	for _, e := range entries {
		if err = ctx.Err(); err != nil {
			return errors.Join(errors.Join(errs...), err)
		}
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(s.root, e.Name())
		r, e2 := readLeaseRecord(path, s.options.MaxMetadataBytesPerLease)
		if e2 != nil {
			errs = append(errs, e2)
			continue
		}
		if intent, intentErr := readStageIntent(path); intentErr == nil {
			dev, ino, identityErr := pathIdentity(path)
			created, statErr := os.Lstat(path)
			if intent.ID != r.ID || !strings.HasPrefix(intent.ScratchName, ".serenity-snapshot-inspect-") || intent.RootDevice != s.rootDevice || intent.RootInode != s.rootInode || intent.ScratchDevice != dev || intent.ScratchInode != ino || identityErr != nil || statErr != nil {
				errs = append(errs, errors.Join(ErrSnapshotLeaseInvalid, identityErr, statErr))
				continue
			}
			if e2 = removeStageIntent(s.root, r.ID, created, s.rootDevice, s.rootInode); e2 != nil {
				errs = append(errs, e2)
				continue
			}
		} else if !errors.Is(intentErr, os.ErrNotExist) {
			errs = append(errs, intentErr)
			continue
		}
		if r.State == "PIN_PENDING" {
			attempt, attemptErr := s.authority.FindPinAttempt(ctx, r.PlanRef, r.ManifestSHA256)
			if attemptErr != nil {
				errs = append(errs, attemptErr)
				continue
			}
			if !attemptMatches(attempt, r.PlanRef, r.ID, r.ManifestSHA256, r.ReservationVersion) || attempt.AttemptVersion != r.AttemptVersion {
				errs = append(errs, ErrSnapshotLeaseConflict)
				continue
			}
			if attempt.State == PinAttemptPending {
				continue
			}
			if attempt.State != PinAttemptCommitted {
				errs = append(errs, ErrSnapshotLeaseConflict)
				continue
			}
			r.State = "PINNED"
			if e2 = s.validateRoot(); e2 == nil {
				e2 = writeLeaseRecord(path, &r, s.options.MaxMetadataBytesPerLease)
			}
			if e2 != nil {
				errs = append(errs, e2)
				continue
			}
			if e2 = fsyncDir(s.root); e2 != nil {
				errs = append(errs, e2)
				continue
			}
			testhooks.At(testhooks.PhaseSnapshotPinPromoted)
		}
		if r.State == "RELEASING" {
			marker := filepath.Join(s.root, ".releases", r.ID)
			if _, markerErr := os.Lstat(marker); markerErr == nil {
				continue
			} else if !errors.Is(markerErr, os.ErrNotExist) {
				errs = append(errs, markerErr)
				continue
			}
			rootInfo, authErr := os.Lstat(s.root)
			var rootDev, rootIno uint64
			if authErr == nil {
				rootDev, rootIno, authErr = fileIdentity(rootInfo)
			}
			if authErr == nil && (!rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 || rootDev != s.rootDevice || rootIno != s.rootInode) {
				authErr = ErrSnapshotLeaseInvalid
			}
			leaseInfo, leaseErr := os.Lstat(path)
			if authErr == nil && (leaseErr != nil || !leaseInfo.IsDir() || leaseInfo.Mode()&os.ModeSymlink != 0) {
				authErr = errors.Join(ErrSnapshotLeaseInvalid, leaseErr)
			}
			if authErr == nil {
				authErr = s.validateLiveLeaseIdentity(path, r)
			}
			var leaseLock *os.File
			if authErr == nil {
				leaseLock, authErr = lockExistingLease(ctx, path)
			}
			if authErr == nil {
				authErr = validateHeldLeaseLock(path, leaseLock)
			}
			leaseRecordInfo, recordIdentityErr := captureLeaseRecordIdentity(path, r, s.options.MaxMetadataBytesPerLease)
			if authErr == nil {
				authErr = recordIdentityErr
			}
			if authErr == nil {
				authErr = s.ensureReleasePeakBeforeOwner(ctx, r)
			}
			var authorization PinReleaseAuthorization
			if authErr == nil {
				authorization, authErr = s.authorizeReleaseResume(ctx, r, attempts)
			}
			if authErr == nil {
				authErr = verifyRootIdentity(s, rootDev, rootIno)
			}
			if authErr == nil {
				authErr = verifyLeaseDirectoryIdentity(path, leaseInfo)
			}
			if authErr == nil {
				authErr = validateHeldLeaseLock(path, leaseLock)
			}
			if authErr == nil {
				authErr = verifyLeaseRecordUnchanged(path, leaseRecordInfo, r, s.options.MaxMetadataBytesPerLease)
			}
			if authErr == nil {
				authErr = verifyRecordFiles(ctx, path, r)
			}
			if authErr == nil {
				authErr = s.releaseLease(ctx, path, r, authorization)
			}
			if leaseLock != nil {
				authErr = errors.Join(authErr, unlockFile(leaseLock), leaseLock.Close())
			}
			var releaseMarkerInfo os.FileInfo
			var releaseMarkerDev, releaseMarkerIno uint64
			if authErr == nil {
				releaseMarkerInfo, authErr = os.Lstat(filepath.Join(s.root, ".releases", r.ID))
			}
			if authErr == nil {
				releaseMarkerDev, releaseMarkerIno, authErr = fileIdentity(releaseMarkerInfo)
			}
			if authErr == nil {
				testhooks.At(testhooks.PhaseSnapshotReleaseOwnerCompletionStarting)
				authErr = s.authority.CompletePinRelease(ctx, authorization)
				if authErr == nil {
					authErr = verifyMarkerIdentity(filepath.Join(s.root, ".releases", r.ID), releaseMarkerDev, releaseMarkerIno)
				}
				if authErr == nil {
					authErr = verifyRootIdentity(s, rootDev, rootIno)
				}
				if authErr == nil {
					authErr = validateJournalIdentity(s, filepath.Join(s.root, ".releases"))
				}
				if authErr == nil {
					authErr = verifyLeaseAbsent(path)
				}
				if authErr == nil {
					testhooks.At(testhooks.PhaseSnapshotReleaseOwnerCompleted)
				}
			}
			if authErr != nil {
				errs = append(errs, authErr)
			}
			continue
		}
		if r.State == "PINNED" {
			rootInfo, identityErr := os.Lstat(s.root)
			var rootDev, rootIno uint64
			if identityErr == nil {
				rootDev, rootIno, identityErr = fileIdentity(rootInfo)
			}
			if identityErr == nil && (!rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 || rootDev != s.rootDevice || rootIno != s.rootInode) {
				identityErr = ErrSnapshotLeaseInvalid
			}
			leaseInfo, leaseErr := os.Lstat(path)
			if identityErr == nil && (leaseErr != nil || !leaseInfo.IsDir() || leaseInfo.Mode()&os.ModeSymlink != 0) {
				identityErr = errors.Join(ErrSnapshotLeaseInvalid, leaseErr)
			}
			var leaseLock *os.File
			if identityErr == nil {
				leaseLock, identityErr = lockExistingLease(ctx, path)
			}
			if identityErr == nil {
				identityErr = validateHeldLeaseLock(path, leaseLock)
			}
			marker := filepath.Join(s.root, ".releases", r.ID)
			if identityErr == nil {
				if _, e := os.Lstat(marker); e == nil {
					identityErr = ErrSnapshotLeaseConflict
				} else if !errors.Is(e, os.ErrNotExist) {
					identityErr = e
				}
			}
			if identityErr != nil {
				if leaseLock != nil {
					identityErr = errors.Join(identityErr, unlockFile(leaseLock), leaseLock.Close())
				}
				errs = append(errs, identityErr)
				continue
			}
			leaseRecordInfo, recordIdentityErr := captureLeaseRecordIdentity(path, r, s.options.MaxMetadataBytesPerLease)
			if recordIdentityErr != nil {
				errs = append(errs, errors.Join(recordIdentityErr, unlockFile(leaseLock), leaseLock.Close()))
				continue
			}
			if e := s.ensureReleasePeakBeforeOwner(ctx, r); e != nil {
				errs = append(errs, errors.Join(e, unlockFile(leaseLock), leaseLock.Close()))
				continue
			}
			decision, reconcileErr := s.authority.ReconcilePin(ctx, pinRef(r))
			if reconcileErr == nil {
				reconcileErr = verifyRootIdentity(s, rootDev, rootIno)
			}
			if reconcileErr == nil {
				reconcileErr = verifyLeaseDirectoryIdentity(path, leaseInfo)
			}
			if reconcileErr == nil {
				reconcileErr = validateHeldLeaseLock(path, leaseLock)
			}
			if reconcileErr == nil {
				reconcileErr = verifyLeaseRecordUnchanged(path, leaseRecordInfo, r, s.options.MaxMetadataBytesPerLease)
			}
			if reconcileErr == nil {
				if _, e := os.Lstat(marker); e == nil {
					reconcileErr = ErrSnapshotLeaseConflict
				} else if !errors.Is(e, os.ErrNotExist) {
					reconcileErr = e
				}
			}
			if reconcileErr != nil {
				errs = append(errs, errors.Join(reconcileErr, unlockFile(leaseLock), leaseLock.Close()))
				continue
			}
			switch decision.Action {
			case PinKeep:
				if e := errors.Join(unlockFile(leaseLock), leaseLock.Close()); e != nil {
					errs = append(errs, e)
				}
				continue
			case PinRelease:
				if !validRelease(decision.Authorization, r) {
					errs = append(errs, errors.Join(ErrSnapshotLeaseConflict, unlockFile(leaseLock), leaseLock.Close()))
					continue
				}
				testhooks.At(testhooks.PhaseSnapshotReleaseOwnerAuthorized)
				e2 := verifyRecordFiles(ctx, path, r)
				if e2 == nil {
					e2 = s.releaseLease(ctx, path, r, decision.Authorization)
				}
				markerInfo, markerErr := os.Lstat(marker)
				if e2 == nil && markerErr != nil {
					e2 = errors.Join(ErrSnapshotLeaseInvalid, markerErr)
				}
				var markerDev, markerIno uint64
				if e2 == nil {
					markerDev, markerIno, e2 = fileIdentity(markerInfo)
				}
				if e2 == nil {
					e2 = verifyMarkerIdentity(marker, markerDev, markerIno)
				}
				unlockErr := errors.Join(unlockFile(leaseLock), leaseLock.Close())
				if e2 = errors.Join(e2, unlockErr); e2 != nil {
					errs = append(errs, e2)
					continue
				}
				md, mi := markerDev, markerIno
				testhooks.At(testhooks.PhaseSnapshotReleaseOwnerCompletionStarting)
				e2 = s.authority.CompletePinRelease(ctx, decision.Authorization)
				if e2 == nil {
					e2 = verifyMarkerIdentity(marker, md, mi)
				}
				if e2 == nil {
					e2 = verifyRootIdentity(s, rootDev, rootIno)
				}
				if e2 == nil {
					e2 = validateJournalIdentity(s, filepath.Join(s.root, ".releases"))
				}
				if e2 == nil {
					e2 = verifyLeaseAbsent(path)
				}
				if e2 != nil {
					errs = append(errs, e2)
				} else {
					testhooks.At(testhooks.PhaseSnapshotReleaseOwnerCompleted)
				}
			default:
				errs = append(errs, errors.Join(ErrSnapshotLeaseConflict, unlockFile(leaseLock), leaseLock.Close()))
			}
			continue
		} else if r.State == "STAGED" && !protected[r.ID] && time.Since(time.Unix(r.CreatedUnix, 0)) >= stageOrphanGrace {
			leaseLock, e3 := lockLeaseContext(ctx, path, false)
			if e3 != nil {
				errs = append(errs, e3)
				continue
			}
			if e2 = verifyRecordFiles(ctx, path, r); e2 != nil {
				e2 = errors.Join(e2, unlockFile(leaseLock), leaseLock.Close())
				errs = append(errs, e2)
				continue
			}
			if e2 = s.validateRoot(); e2 == nil {
				e2 = s.removeLeaseDirectory(r.ID, r.RootDevice, r.RootInode)
			}
			unlockErr := errors.Join(unlockFile(leaseLock), leaseLock.Close())
			if e2 = errors.Join(e2, unlockErr); e2 != nil {
				errs = append(errs, e2)
			} else if e2 = fsyncDir(s.root); e2 != nil {
				errs = append(errs, e2)
			}
		}
	}
	return errors.Join(errs...)
}

func validatePinAttemptList(attempts []PinAttemptRef) error {
	if uint64(len(attempts)) > uint64(maxSnapshotPinAttempts) {
		return ErrSnapshotLeaseLimit
	}
	seenAttempts := make(map[string]struct{}, len(attempts))
	seenReservations := make(map[string]string, len(attempts))
	seenLeases := make(map[string]string, len(attempts))
	for _, a := range attempts {
		if (a.State != PinAttemptPending && a.State != PinAttemptCommitted) || !validPlanRef(a.PlanRef) || !isCanonicalID(a.LeaseID) || !isSHA256(a.ManifestSHA256) || a.ReservationVersion == 0 || a.AttemptVersion == 0 {
			return ErrSnapshotLeaseInvalid
		}
		attemptKey := fmt.Sprintf("%s:%d:%s:%d", a.PlanRef, a.ReservationVersion, a.LeaseID, a.AttemptVersion)
		if _, ok := seenAttempts[attemptKey]; ok {
			return ErrSnapshotLeaseConflict
		}
		seenAttempts[attemptKey] = struct{}{}
		reservationKey := fmt.Sprintf("%s:%d", a.PlanRef, a.ReservationVersion)
		if old, ok := seenReservations[reservationKey]; ok && old != attemptKey {
			return ErrSnapshotLeaseConflict
		}
		seenReservations[reservationKey] = attemptKey
		if old, ok := seenLeases[a.LeaseID]; ok && old != attemptKey {
			return ErrSnapshotLeaseConflict
		}
		seenLeases[a.LeaseID] = attemptKey
	}
	return nil
}

func validateLocalPinAttemptPairs(root string, maxMetadata int64, attempts []PinAttemptRef) error {
	for _, a := range attempts {
		r, err := readLeaseRecord(filepath.Join(root, a.LeaseID), maxMetadata)
		if errors.Is(err, os.ErrNotExist) && a.State == PinAttemptCommitted {
			marker := filepath.Join(root, ".releases", a.LeaseID)
			release, markerErr := readLeaseRecord(marker, maxLeaseMetadataBytes)
			if markerErr == nil && release.ID == a.LeaseID && release.ManifestSHA256 == a.ManifestSHA256 && release.PlanRef == a.PlanRef && release.ReservationVersion == a.ReservationVersion && release.AttemptVersion == a.AttemptVersion && release.State == "RELEASING" && release.PinID != "" {
				continue
			}
			tombstone, tombstoneErr := readReleaseTombstone(marker)
			if tombstoneErr == nil && tombstone.ID == a.LeaseID && tombstone.ManifestSHA256 == a.ManifestSHA256 && tombstone.PlanRef == a.PlanRef && tombstone.ReservationVersion == a.ReservationVersion && tombstone.AttemptVersion == a.AttemptVersion && tombstone.State == "RELEASED" && tombstone.PinID != "" {
				continue
			}
			return errors.Join(ErrSnapshotLeaseConflict, err, markerErr, tombstoneErr)
		}
		if err != nil {
			return errors.Join(ErrSnapshotLeaseConflict, err)
		}
		if r.ID != a.LeaseID || r.ManifestSHA256 != a.ManifestSHA256 {
			return ErrSnapshotLeaseConflict
		}
		fieldsMatch := r.PlanRef == a.PlanRef && r.ReservationVersion == a.ReservationVersion && r.AttemptVersion == a.AttemptVersion
		switch a.State {
		case PinAttemptPending:
			if r.State == "STAGED" && r.PlanRef == "" && r.PinID == "" && r.ReservationVersion == 0 && r.AttemptVersion == 0 {
				continue
			}
			if r.State == "PIN_PENDING" && fieldsMatch && r.PinID != "" {
				continue
			}
			return ErrSnapshotLeaseConflict
		case PinAttemptCommitted:
			if (r.State == "PIN_PENDING" || r.State == "PINNED" || r.State == "RELEASING") && fieldsMatch && r.PinID != "" {
				continue
			}
			return ErrSnapshotLeaseConflict
		default:
			return ErrSnapshotLeaseInvalid
		}
	}
	return nil
}

func (l *VerifiedSnapshotLease) Close(ctx context.Context) (retErr error) {
	if isNilInterface(ctx) {
		return ErrNilContext
	}
	l.mu.Lock()
	l.closing = true
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			l.mu.Lock()
			l.cond.Broadcast()
			l.mu.Unlock()
		case <-done:
		}
	}()
	for l.borrowers > 0 && ctx.Err() == nil {
		l.cond.Wait()
	}
	close(done)
	if err := ctx.Err(); err != nil {
		l.mu.Unlock()
		return err
	}
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.mu.Unlock()
	unlock, err := l.store.lock(ctx)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, unlock()) }()
	leaseUnlock, err := lockLeaseContext(ctx, l.path, false)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, unlockFile(leaseUnlock), leaseUnlock.Close()) }()
	current, err := readLeaseRecord(l.path, l.store.options.MaxMetadataBytesPerLease)
	if err != nil {
		return err
	}
	l.record = current
	if current.State == "PIN_PENDING" || current.State == "PINNED" {
		l.mu.Lock()
		l.closed = true
		l.mu.Unlock()
		return nil
	}
	if current.State != "STAGED" {
		return ErrSnapshotLeaseConflict
	}
	attempts, err := l.store.authority.ListPinAttempts(ctx)
	if err != nil {
		return err
	}
	if err = validatePinAttemptList(attempts); err != nil {
		return err
	}
	if err = validateLocalPinAttemptPairs(l.store.root, l.store.options.MaxMetadataBytesPerLease, attempts); err != nil {
		return err
	}
	for _, attempt := range attempts {
		if attempt.LeaseID == current.ID {
			l.mu.Lock()
			l.closed = true
			l.mu.Unlock()
			return nil
		}
	}
	if err = verifyRecordFiles(ctx, l.path, current); err != nil {
		return err
	}
	if err = l.store.validateRoot(); err != nil {
		return err
	}
	if err = l.store.removeLeaseDirectory(current.ID, current.RootDevice, current.RootInode); err != nil {
		return err
	}
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
	return nil
}

func RestoreVerified(ctx context.Context, lease *VerifiedSnapshotLease, destination string) (retErr error) {
	if lease == nil {
		return ErrSnapshotLeaseInvalid
	}
	unlockStore, err := lease.store.lock(ctx)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, unlockStore()) }()
	if err := lease.beginBorrow(ctx, true); err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, lease.endBorrow()) }()
	if lease.record.ArtifactBytes > lease.store.options.MaxRestoreScratchBytes {
		return ErrSnapshotLeaseLimit
	}
	if err = verifyRecordFiles(ctx, lease.path, lease.record); err != nil {
		return err
	}
	return restoreCoreWithScratch(ctx, lease.path, destination, lease.store.root, func(name string, created os.FileInfo) error {
		dev, ino, e := fileIdentity(created)
		if e != nil {
			return e
		}
		id, e := randomID()
		if e != nil {
			return e
		}
		intent := stageIntentRecord{Version: 1, Kind: "restore", ID: id, OwnerLeaseID: lease.record.ID, ScratchName: name, RootDevice: lease.store.rootDevice, RootInode: lease.store.rootInode, ScratchDevice: dev, ScratchInode: ino, ArtifactBytes: lease.record.ArtifactBytes, CreatedUnix: time.Now().Unix()}
		if e = lease.store.validateRoot(); e != nil {
			return e
		}
		if e = writeStageIntent(lease.store.root, name, intent); e != nil {
			return e
		}
		usage, e := lease.store.usageLocked()
		if e != nil {
			return e
		}
		if usage.artifacts > lease.store.options.MaxRetainedArtifactBytes || usage.metadata > lease.store.options.MaxRetainedMetadataBytes {
			return ErrSnapshotLeaseLimit
		}
		if lease.restoreAfterIntent != nil {
			lease.restoreAfterIntent()
		}
		return lease.store.validateRoot()
	})
}

func (l *VerifiedSnapshotLease) beginBorrow(ctx context.Context, restore bool) error {
	if isNilInterface(ctx) {
		return ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed || l.closing {
		return ErrSnapshotLeaseClosed
	}
	if restore && l.restoreActive {
		return ErrSnapshotLeaseConflict
	}
	if restore {
		l.restoreActive = true
	}
	lock, err := lockLeaseContext(ctx, l.path, true)
	if err != nil {
		if restore {
			l.restoreActive = false
		}
		return err
	}
	l.borrowLocks = append(l.borrowLocks, lock)
	l.borrowers++
	return nil
}
func (l *VerifiedSnapshotLease) endBorrow() error {
	l.mu.Lock()
	if len(l.borrowLocks) == 0 {
		l.mu.Unlock()
		return ErrSnapshotLeaseInvalid
	}
	lock := l.borrowLocks[len(l.borrowLocks)-1]
	l.borrowLocks = l.borrowLocks[:len(l.borrowLocks)-1]
	l.borrowers--
	l.restoreActive = false
	l.cond.Broadcast()
	l.mu.Unlock()
	return errors.Join(unlockFile(lock), lock.Close())
}
func newLease(s *SnapshotLeaseStore, r leaseDiskRecord, path string) *VerifiedSnapshotLease {
	l := &VerifiedSnapshotLease{store: s, record: r, path: path}
	l.cond = sync.NewCond(&l.mu)
	return l
}
func (s *SnapshotLeaseStore) checkContext(ctx context.Context) error {
	if isNilInterface(ctx) {
		return ErrNilContext
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return ErrSnapshotLeaseClosed
	}
	return ctx.Err()
}

type leaseUsage struct {
	leases              int
	releaseEntries      int
	artifacts, metadata int64
}

func (s *SnapshotLeaseStore) readReleaseJournalEntries() (entries []os.DirEntry, retErr error) {
	if err := s.validateReleaseRoot(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	rootInfo, err := root.Stat(".")
	if err != nil {
		return nil, err
	}
	rootDev, rootIno, err := fileIdentity(rootInfo)
	if err != nil || rootDev != s.rootDevice || rootIno != s.rootInode {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	pathRoot, err := os.Lstat(s.root)
	if err != nil || !os.SameFile(rootInfo, pathRoot) {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	journal, err := root.OpenRoot(".releases")
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, journal.Close()) }()
	journalInfo, err := journal.Stat(".")
	if err != nil {
		return nil, err
	}
	journalDev, journalIno, err := fileIdentity(journalInfo)
	if err != nil || journalDev != s.releaseDevice || journalIno != s.releaseInode {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	pathJournal, err := os.Lstat(filepath.Join(s.root, ".releases"))
	if err != nil || !os.SameFile(journalInfo, pathJournal) || !pathJournal.IsDir() || pathJournal.Mode()&os.ModeSymlink != 0 {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	dir, err := journal.Open(".")
	if err != nil {
		return nil, err
	}
	entries, readErr := dir.ReadDir(maxReleaseJournalEntries + 1)
	closeErr := dir.Close()
	if closeErr != nil {
		return nil, errors.Join(readErr, closeErr)
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, readErr
	}
	if len(entries) > maxReleaseJournalEntries {
		return nil, ErrSnapshotLeaseLimit
	}
	// Snapshot and validate every direct entry before callers process any of
	// them. Hidden, noncanonical, symlinked, and non-directory entries count
	// toward the cap and fail closed rather than escaping the scan.
	for _, entry := range entries {
		if !isCanonicalID(entry.Name()) {
			return nil, ErrSnapshotLeaseInvalid
		}
		info, statErr := journal.Lstat(entry.Name())
		if statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
			return nil, errors.Join(ErrSnapshotLeaseInvalid, statErr)
		}
	}
	if err = s.validateReleaseRoot(); err != nil {
		return nil, err
	}
	return entries, nil
}

func readReleaseMarkerEntries(path string) (entries []os.DirEntry, retErr error) {
	before, err := os.Lstat(path)
	if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 || before.Mode().Perm()&0077 != 0 {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(before, opened) {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	entries, readErr := dir.ReadDir(maxReleaseJournalEntries + 1)
	closeErr := dir.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, errors.Join(readErr, closeErr)
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(entries) > maxReleaseJournalEntries {
		return nil, ErrSnapshotLeaseLimit
	}
	for _, entry := range entries {
		name := entry.Name()
		allowed := name == leaseRecordName || name == releaseRecordName
		if strings.HasPrefix(name, ".lease.tmp-") {
			allowed = isCanonicalID(strings.TrimPrefix(name, ".lease.tmp-"))
		} else if strings.HasPrefix(name, ".release.tmp-") {
			allowed = isCanonicalID(strings.TrimPrefix(name, ".release.tmp-"))
		}
		if !allowed {
			return nil, ErrSnapshotLeaseInvalid
		}
		info, statErr := root.Lstat(name)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
			return nil, errors.Join(ErrSnapshotLeaseInvalid, statErr)
		}
		if strings.HasPrefix(name, ".release.tmp-") || strings.HasPrefix(name, ".lease.tmp-") {
			if info.Size() < 0 || info.Size() > maxReleaseRecordBytes {
				return nil, ErrSnapshotLeaseLimit
			}
		}
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(opened, current) {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	return entries, nil
}

func tombstoneMatchesReleaseRecord(t releaseTombstone, r leaseDiskRecord) bool {
	return r.State == "RELEASING" && t.ID == r.ID && t.PinID == r.PinID && t.PlanRef == r.PlanRef && t.ManifestSHA256 == r.ManifestSHA256 && t.ReservationVersion == r.ReservationVersion && t.AttemptVersion == r.AttemptVersion && t.Disposition == r.Disposition && t.RecordVersion == r.RecordVersion && t.State == "RELEASED"
}

func releaseAttemptMatchesRecord(attempts []PinAttemptRef, r leaseDiskRecord) error {
	count := 0
	for _, attempt := range attempts {
		if attempt.LeaseID != r.ID {
			continue
		}
		count++
		if attempt.State != PinAttemptCommitted || attempt.PlanRef != r.PlanRef || attempt.ManifestSHA256 != r.ManifestSHA256 || attempt.ReservationVersion != r.ReservationVersion || attempt.AttemptVersion != r.AttemptVersion {
			return ErrSnapshotLeaseConflict
		}
	}
	if count != 1 || r.PinID == "" || r.ReservationVersion == 0 || r.AttemptVersion == 0 || r.Disposition == 0 || r.RecordVersion == 0 {
		return ErrSnapshotLeaseConflict
	}
	return nil
}

func exactReleaseAuthorization(a PinReleaseAuthorization, r leaseDiskRecord) bool {
	return validRelease(a, r) && a.Disposition == r.Disposition && a.RecordVersion == r.RecordVersion
}

func (s *SnapshotLeaseStore) authorizeReleaseResume(ctx context.Context, r leaseDiskRecord, attempts []PinAttemptRef) (PinReleaseAuthorization, error) {
	if err := ctx.Err(); err != nil {
		return PinReleaseAuthorization{}, err
	}
	if r.State != "RELEASING" || !validPlanRef(r.PlanRef) || !isCanonicalID(r.PinID) || !isSHA256(r.ManifestSHA256) || r.ReservationVersion == 0 || r.AttemptVersion == 0 || r.Disposition == 0 || r.RecordVersion == 0 {
		return PinReleaseAuthorization{}, ErrSnapshotLeaseInvalid
	}
	if err := releaseAttemptMatchesRecord(attempts, r); err != nil {
		return PinReleaseAuthorization{}, err
	}
	decision, err := s.authority.ReconcilePin(ctx, pinRef(r))
	if err != nil {
		return PinReleaseAuthorization{}, err
	}
	if err = ctx.Err(); err != nil {
		return PinReleaseAuthorization{}, err
	}
	if decision.Action != PinRelease || !exactReleaseAuthorization(decision.Authorization, r) {
		return PinReleaseAuthorization{}, ErrSnapshotLeaseConflict
	}
	testhooks.At(testhooks.PhaseSnapshotReleaseOwnerAuthorized)
	return decision.Authorization, nil
}

func liveLeaseTempBytes(path string, expected os.FileInfo, record leaseDiskRecord) (int64, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = root.Close() }()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(expected, opened) {
		return 0, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	dir, err := root.Open(".")
	if err != nil {
		return 0, err
	}
	entries, readErr := dir.ReadDir(maxReleaseJournalEntries + 1)
	closeErr := dir.Close()
	if closeErr != nil {
		return 0, errors.Join(readErr, closeErr)
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return 0, readErr
	}
	if len(entries) > maxReleaseJournalEntries {
		return 0, ErrSnapshotLeaseLimit
	}
	allowed := map[string]bool{".lease.lock": true, leaseRecordName: true}
	for _, file := range record.Files {
		allowed[file.Name] = true
	}
	var total int64
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".lease.tmp-") {
			if !isCanonicalID(strings.TrimPrefix(name, ".lease.tmp-")) {
				return 0, ErrSnapshotLeaseInvalid
			}
			info, e := root.Lstat(name)
			if e != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || info.Size() < 0 || info.Size() > maxReleaseRecordBytes || total > math.MaxInt64-info.Size() {
				return 0, errors.Join(ErrSnapshotLeaseInvalid, e)
			}
			total += info.Size()
			continue
		}
		if !allowed[name] {
			return 0, ErrSnapshotLeaseInvalid
		}
		info, e := root.Lstat(name)
		if e != nil {
			if record.State == "RELEASING" && name != ".lease.lock" && name != leaseRecordName && errors.Is(e, os.ErrNotExist) {
				continue
			}
			return 0, errors.Join(ErrSnapshotLeaseInvalid, e)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
			return 0, ErrSnapshotLeaseInvalid
		}
		if name != ".lease.lock" && name != leaseRecordName {
			for _, file := range record.Files {
				if file.Name == name {
					d, i, idErr := fileIdentity(info)
					if idErr != nil || d != file.Device || i != file.Inode || info.Size() != file.Size {
						return 0, errors.Join(ErrSnapshotLeaseInvalid, idErr)
					}
					break
				}
			}
		}
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(expected, current) {
		return 0, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	return total, nil
}

func (s *SnapshotLeaseStore) usageLocked() (leaseUsage, error) {
	var u leaseUsage
	counted := map[string]bool{}
	es, err := os.ReadDir(s.root)
	if err != nil {
		return u, err
	}
	for _, e := range es {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		r, e2 := readLeaseRecord(filepath.Join(s.root, e.Name()), s.options.MaxMetadataBytesPerLease)
		if e2 != nil {
			return u, e2
		}
		if r.State == "STAGED" || r.State == "PIN_PENDING" || r.State == "PINNED" || r.State == "RELEASING" {
			if r.ArtifactBytes > s.options.MaxArtifactBytesPerLease || r.MetadataBytes > s.options.MaxMetadataBytesPerLease || u.artifacts > math.MaxInt64-r.ArtifactBytes || u.metadata > math.MaxInt64-r.MetadataBytes {
				return u, ErrSnapshotLeaseLimit
			}
			u.leases++
			u.artifacts += r.ArtifactBytes
			u.metadata += r.MetadataBytes
			info, statErr := os.Lstat(filepath.Join(s.root, e.Name()))
			if statErr != nil {
				return u, statErr
			}
			tempBytes, tempErr := liveLeaseTempBytes(filepath.Join(s.root, e.Name()), info, r)
			if tempErr != nil || u.metadata > math.MaxInt64-tempBytes {
				return u, errors.Join(ErrSnapshotLeaseInvalid, tempErr)
			}
			u.metadata += tempBytes
			counted[r.ID] = true
		}
	}
	rootDev, rootIno, rootErr := pathIdentity(s.root)
	if rootErr != nil || rootDev != s.rootDevice || rootIno != s.rootInode {
		return u, errors.Join(ErrSnapshotLeaseInvalid, rootErr)
	}
	for _, entry := range es {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), ".serenity-snapshot-inspect-") {
			continue
		}
		p := filepath.Join(s.root, entry.Name())
		intent, intentErr := readStageIntent(p)
		if errors.Is(intentErr, os.ErrNotExist) {
			contents, readErr := os.ReadDir(p)
			if readErr != nil || len(contents) != 0 {
				return u, errors.Join(ErrSnapshotLeaseInvalid, readErr)
			}
			continue
		}
		if intentErr != nil {
			return u, intentErr
		}
		dev, ino, identityErr := pathIdentity(p)
		if identityErr != nil || intent.RootDevice != rootDev || intent.RootInode != rootIno || intent.ScratchName != entry.Name() || intent.ScratchDevice != dev || intent.ScratchInode != ino {
			return u, errors.Join(ErrSnapshotLeaseInvalid, identityErr)
		}
		if intent.Kind == "candidate" || intent.Kind == "restore" {
			if intent.ArtifactBytes > s.options.MaxRestoreScratchBytes || intent.MetadataBytes > s.options.MaxRetainedMetadataBytes-u.metadata || intent.ArtifactBytes > s.options.MaxRetainedArtifactBytes-u.artifacts {
				return u, ErrSnapshotLeaseLimit
			}
			u.artifacts += intent.ArtifactBytes
			u.metadata += intent.MetadataBytes
			continue
		}
		if counted[intent.ID] {
			continue
		}
		if intent.ArtifactBytes > s.options.MaxArtifactBytesPerLease || intent.MetadataBytes != s.options.MaxMetadataBytesPerLease || u.leases >= s.options.MaxLeases || intent.ArtifactBytes > s.options.MaxRetainedArtifactBytes-u.artifacts || intent.MetadataBytes > s.options.MaxRetainedMetadataBytes-u.metadata {
			return u, ErrSnapshotLeaseLimit
		}
		u.leases++
		u.artifacts += intent.ArtifactBytes
		u.metadata += intent.MetadataBytes
	}
	entries, err := s.readReleaseJournalEntries()
	if err != nil {
		return u, err
	}
	u.releaseEntries = len(entries)
	for _, entry := range entries {
		marker := filepath.Join(s.root, ".releases", entry.Name())
		markerEntries, markerErr := readReleaseMarkerEntries(marker)
		if markerErr != nil {
			return u, markerErr
		}
		tombstone, e := readReleaseTombstone(marker)
		var metadataBytes int64
		if e == nil {
			metadataBytes = tombstone.MetadataBytes
			if _, statErr := os.Lstat(filepath.Join(marker, leaseRecordName)); statErr == nil {
				record, readErr := readLeaseRecord(marker, maxLeaseMetadataBytes)
				if readErr != nil || !tombstoneMatchesReleaseRecord(tombstone, record) {
					return u, errors.Join(ErrSnapshotLeaseInvalid, readErr)
				}
				if metadataBytes > math.MaxInt64-record.MetadataBytes {
					return u, ErrSnapshotLeaseLimit
				}
				metadataBytes += record.MetadataBytes
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return u, statErr
			}
		} else if errors.Is(e, os.ErrNotExist) {
			r, readErr := readLeaseRecord(marker, maxLeaseMetadataBytes)
			if readErr != nil || r.State != "RELEASING" || r.ID != entry.Name() {
				return u, errors.Join(ErrSnapshotLeaseInvalid, readErr)
			}
			metadataBytes = r.MetadataBytes
			if _, statErr := os.Lstat(filepath.Join(s.root, r.ID)); statErr == nil {
				live, liveErr := readLeaseRecord(filepath.Join(s.root, r.ID), s.options.MaxMetadataBytesPerLease)
				liveRaw, liveEncodeErr := encodeLeaseRecord(&live, s.options.MaxMetadataBytesPerLease)
				markerRaw, markerEncodeErr := encodeLeaseRecord(&r, maxLeaseMetadataBytes)
				if liveErr != nil || liveEncodeErr != nil || markerEncodeErr != nil || !bytes.Equal(liveRaw, markerRaw) {
					return u, errors.Join(ErrSnapshotLeaseConflict, liveErr, liveEncodeErr, markerEncodeErr)
				}
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return u, statErr
			}
		} else {
			return u, e
		}
		for _, markerEntry := range markerEntries {
			if !strings.HasPrefix(markerEntry.Name(), ".release.tmp-") && !strings.HasPrefix(markerEntry.Name(), ".lease.tmp-") {
				continue
			}
			info, statErr := os.Lstat(filepath.Join(marker, markerEntry.Name()))
			if statErr != nil || info.Size() < 0 || metadataBytes > math.MaxInt64-info.Size() {
				return u, errors.Join(ErrSnapshotLeaseInvalid, statErr)
			}
			metadataBytes += info.Size()
		}
		if u.metadata > math.MaxInt64-metadataBytes {
			return u, ErrSnapshotLeaseLimit
		}
		u.metadata += metadataBytes
	}
	if u.metadata > s.options.MaxRetainedMetadataBytes {
		return u, ErrSnapshotLeaseLimit
	}
	return u, nil
}

func (s *SnapshotLeaseStore) lock(ctx context.Context) (func() error, error) {
	_, unlock, err := s.lockHandle(ctx)
	return unlock, err
}

func (s *SnapshotLeaseStore) lockHandle(ctx context.Context) (*os.File, func() error, error) {
	if isNilInterface(ctx) {
		return nil, nil, ErrNilContext
	}
	if err := s.validateRoot(); err != nil {
		return nil, nil, err
	}
	lockPath := filepath.Join(s.root, ".store.lock")
	var f *os.File
	var err error
	for tries := 0; tries < 2; tries++ {
		before, statErr := os.Lstat(lockPath)
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return nil, nil, statErr
		}
		if errors.Is(statErr, os.ErrNotExist) {
			return nil, nil, ErrSnapshotLeaseInvalid
		}
		if statErr == nil && (!before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0) {
			return nil, nil, ErrSnapshotLeaseInvalid
		}
		f, err = openLockFile(lockPath, false)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		break
	}
	if f == nil {
		return nil, nil, ErrSnapshotLeaseConflict
	}
	opened, err := f.Stat()
	if err != nil {
		return nil, nil, errors.Join(err, f.Close())
	}
	current, err := os.Lstat(lockPath)
	lockDev, lockIno, identityErr := fileIdentity(opened)
	if err != nil || identityErr != nil || !os.SameFile(current, opened) || !current.Mode().IsRegular() || current.Mode().Perm()&0077 != 0 || lockDev != s.lockDevice || lockIno != s.lockInode {
		return nil, nil, errors.Join(ErrSnapshotLeaseInvalid, err, identityErr, f.Close())
	}
	if s.beforeStoreLock != nil {
		s.beforeStoreLock()
	}
	if err = lockFileContext(ctx, f, false); err != nil {
		return nil, nil, errors.Join(err, f.Close())
	}
	if err = s.validateRoot(); err != nil {
		return nil, nil, errors.Join(err, unlockFile(f), f.Close())
	}
	current, err = os.Lstat(lockPath)
	if err != nil || !os.SameFile(current, opened) || lockDev != s.lockDevice || lockIno != s.lockInode {
		return nil, nil, errors.Join(ErrSnapshotLeaseInvalid, err, unlockFile(f), f.Close())
	}
	return f, func() error { return errors.Join(unlockFile(f), f.Close()) }, nil
}

func (s *SnapshotLeaseStore) validateReleaseRoot() error {
	if err := s.validateRoot(); err != nil {
		return err
	}
	journal := filepath.Join(s.root, ".releases")
	info, err := os.Lstat(journal)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	device, inode, err := fileIdentity(info)
	if err != nil || device != s.releaseDevice || inode != s.releaseInode {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	return nil
}

func (s *SnapshotLeaseStore) validateRoot() error {
	dev, ino, err := pathIdentity(s.root)
	if err != nil {
		return err
	}
	if dev != s.rootDevice || ino != s.rootInode {
		return ErrSnapshotLeaseInvalid
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return err
	}
	info, err := root.Stat(".")
	if err != nil {
		return errors.Join(err, root.Close())
	}
	d, i, e := fileIdentity(info)
	if e != nil || d != s.rootDevice || i != s.rootInode {
		return errors.Join(ErrSnapshotLeaseInvalid, e, root.Close())
	}
	lockInfo, lockErr := root.Stat(".store.lock")
	if lockErr != nil || !lockInfo.Mode().IsRegular() || lockInfo.Mode().Perm()&0077 != 0 {
		return errors.Join(ErrSnapshotLeaseInvalid, lockErr, root.Close())
	}
	lockDev, lockIno, identityErr := fileIdentity(lockInfo)
	if identityErr != nil || lockDev != s.lockDevice || lockIno != s.lockInode {
		return errors.Join(ErrSnapshotLeaseInvalid, identityErr, root.Close())
	}
	currentLock, pathErr := os.Lstat(filepath.Join(s.root, ".store.lock"))
	if pathErr != nil || !os.SameFile(lockInfo, currentLock) {
		return errors.Join(ErrSnapshotLeaseInvalid, pathErr, root.Close())
	}
	return root.Close()
}

func lockLeaseContext(ctx context.Context, dir string, shared bool) (*os.File, error) {
	dirInfo, err := os.Lstat(dir)
	if err != nil || !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	path := filepath.Join(dir, ".lease.lock")
	before, statErr := os.Lstat(path)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	if statErr == nil && (!before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0) {
		return nil, ErrSnapshotLeaseInvalid
	}
	f, err := openLockFile(path, errors.Is(statErr, os.ErrNotExist))
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil {
		return nil, errors.Join(err, f.Close())
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(current, opened) || !current.Mode().IsRegular() || current.Mode().Perm()&0077 != 0 {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err, f.Close())
	}
	if err = lockFileContext(ctx, f, shared); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	current, err = os.Lstat(path)
	currentDir, dirErr := os.Lstat(dir)
	if err != nil || dirErr != nil || !os.SameFile(current, opened) || !os.SameFile(currentDir, dirInfo) || !currentDir.IsDir() {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err, dirErr, unlockFile(f), f.Close())
	}
	return f, nil
}

func (s *SnapshotLeaseStore) validateLiveLeaseIdentity(path string, record leaseDiskRecord) error {
	if err := s.validateRoot(); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	dev, ino, err := fileIdentity(info)
	if err != nil || dev != record.RootDevice || ino != record.RootInode {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	return nil
}

func lockExistingLease(ctx context.Context, path string) (*os.File, error) {
	info, err := os.Lstat(filepath.Join(path, ".lease.lock"))
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	return lockLeaseContext(ctx, path, false)
}

func validateHeldLeaseLock(path string, lock *os.File) error {
	if lock == nil {
		return ErrSnapshotLeaseInvalid
	}
	opened, err := lock.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Mode().Perm()&0077 != 0 {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	current, err := os.Lstat(filepath.Join(path, ".lease.lock"))
	if err != nil || !current.Mode().IsRegular() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, current) {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	return nil
}

func sameLeaseRecord(a, b leaseDiskRecord) bool {
	aRaw, aErr := encodeLeaseRecord(&a, maxLeaseMetadataBytes)
	bRaw, bErr := encodeLeaseRecord(&b, maxLeaseMetadataBytes)
	return aErr == nil && bErr == nil && bytes.Equal(aRaw, bRaw)
}

func verifyReleasingLease(ctx context.Context, path string, record leaseDiskRecord, lock *os.File, allowMissingArtifacts bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateLeaseRecord(record); err != nil {
		return err
	}
	if record.State != "RELEASING" {
		return ErrSnapshotLeaseConflict
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	dev, ino, err := fileIdentity(info)
	if err != nil || dev != record.RootDevice || ino != record.RootInode {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	if err = validateHeldLeaseLock(path, lock); err != nil {
		return err
	}
	leaseLockInfo, err := root.Lstat(".lease.lock")
	if err != nil || !leaseLockInfo.Mode().IsRegular() || leaseLockInfo.Mode().Perm()&0077 != 0 {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	live, err := readLeaseRecord(path, maxLeaseMetadataBytes)
	if err != nil || !sameLeaseRecord(live, record) {
		return errors.Join(ErrSnapshotLeaseConflict, err)
	}
	entriesFile, err := root.Open(".")
	if err != nil {
		return err
	}
	entries, readErr := entriesFile.ReadDir(maxReleaseJournalEntries + 1)
	closeErr := entriesFile.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return errors.Join(readErr, closeErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if len(entries) > maxReleaseJournalEntries {
		return ErrSnapshotLeaseLimit
	}
	expected := make(map[string]struct{}, len(record.Files)+2)
	expected[".lease.lock"] = struct{}{}
	expected[leaseRecordName] = struct{}{}
	for _, file := range record.Files {
		expected[file.Name] = struct{}{}
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if _, ok := expected[name]; !ok && !strings.HasPrefix(name, ".lease.tmp-") {
			return ErrSnapshotLeaseInvalid
		}
		if strings.HasPrefix(name, ".lease.tmp-") && !isCanonicalID(strings.TrimPrefix(name, ".lease.tmp-")) {
			return ErrSnapshotLeaseInvalid
		}
		if _, ok := seen[name]; ok {
			return ErrSnapshotLeaseInvalid
		}
		seen[name] = struct{}{}
		child, statErr := root.Lstat(name)
		if statErr != nil || !child.Mode().IsRegular() || child.Mode()&os.ModeSymlink != 0 {
			return errors.Join(ErrSnapshotLeaseInvalid, statErr)
		}
		if strings.HasPrefix(name, ".lease.tmp-") && (child.Mode().Perm()&0077 != 0 || child.Size() < 0 || child.Size() > maxLeaseMetadataBytes) {
			return ErrSnapshotLeaseInvalid
		}
	}
	for name := range expected {
		if _, ok := seen[name]; ok {
			continue
		}
		if name != ".lease.lock" && name != leaseRecordName && allowMissingArtifacts {
			continue
		}
		return ErrSnapshotLeaseInvalid
	}
	for _, file := range record.Files {
		f, openErr := root.Open(file.Name)
		if errors.Is(openErr, os.ErrNotExist) && allowMissingArtifacts {
			continue
		}
		if openErr != nil {
			return openErr
		}
		fileInfo, statErr := f.Stat()
		if statErr != nil {
			_ = f.Close()
			return statErr
		}
		fileDev, fileIno, idErr := fileIdentity(fileInfo)
		if idErr != nil || !fileInfo.Mode().IsRegular() || fileInfo.Size() != file.Size || fileDev != file.Device || fileIno != file.Inode {
			_ = f.Close()
			return errors.Join(ErrSnapshotLeaseInvalid, idErr)
		}
		hash := sha256.New()
		count, readErr := copyContext(ctx, hash, f, file.Size)
		closeErr := f.Close()
		if readErr != nil || closeErr != nil || count != file.Size || hex.EncodeToString(hash.Sum(nil)) != file.SHA256 {
			return errors.Join(ErrSnapshotLeaseInvalid, readErr, closeErr)
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(opened, current) {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	return nil
}

func validatePlanDigest(planRef, digest string) error {
	if !validPlanRef(planRef) || !isSHA256(digest) {
		return ErrSnapshotLeaseInvalid
	}
	return nil
}
func validPlanRef(v string) bool {
	if len(v) != 64 {
		return false
	}
	for _, c := range v {
		if c < '0' || c > '9' {
			if c < 'a' || c > 'f' {
				return false
			}
		}
	}
	return true
}
func isCanonicalID(v string) bool { return validPlanRef(v) }
func randomID() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func bundleNames(m contracts.ManifestV2) []string {
	r := make([]string, 0, len(m.Brains))
	for _, b := range m.Brains {
		if !b.Empty {
			r = append(r, b.ID+".bundle")
		}
	}
	return r
}
func cloneInspection(i SnapshotInspection) SnapshotInspection {
	i.Accounts = slices.Clone(i.Accounts)
	i.Brains = cloneBrainArtifacts(i.Brains)
	return i
}
func attemptMatches(a PinAttemptRef, ref, lease, digest string, reservation uint64) bool {
	return a.PlanRef == ref && a.LeaseID == lease && a.ManifestSHA256 == digest && a.ReservationVersion == reservation && a.AttemptVersion > 0
}
func sameAttempt(a, b PinAttemptRef) bool {
	return a.PlanRef == b.PlanRef && a.LeaseID == b.LeaseID && a.ManifestSHA256 == b.ManifestSHA256 && a.ReservationVersion == b.ReservationVersion && a.AttemptVersion == b.AttemptVersion
}
func pinRef(r leaseDiskRecord) SnapshotPinRef {
	return SnapshotPinRef{id: r.PinID, digest: r.ManifestSHA256, planRef: r.PlanRef, reservationVersion: r.ReservationVersion}
}
func validRelease(a PinReleaseAuthorization, r leaseDiskRecord) bool {
	return a.PinID == r.PinID && a.PlanRef == r.PlanRef && a.ManifestSHA256 == r.ManifestSHA256 && a.RecordVersion > 0 && (a.Disposition == PinAbandonedBeforeEffects || a.Disposition == PinCommittedRestoreComplete)
}
func digestBytes(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

func validCandidateText(s string) bool {
	if !utf8.ValidString(s) || len(s) > maxCandidateTextBytes {
		return false
	}
	for _, r := range s {
		if r == 0 || r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return false
		}
	}
	return true
}

func inspectOwnedFile(path, name string) (record leaseFileRecord, retErr error) {
	f, err := openRegularFile(path)
	if err != nil {
		return leaseFileRecord{}, err
	}
	defer func() { retErr = errors.Join(retErr, f.Close()) }()
	info, err := f.Stat()
	if err != nil {
		return leaseFileRecord{}, err
	}
	if !info.Mode().IsRegular() {
		return leaseFileRecord{}, ErrSnapshotLeaseInvalid
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, maxLeaseBytes+1))
	if err != nil {
		return leaseFileRecord{}, err
	}
	if n != info.Size() || n > maxLeaseBytes {
		return leaseFileRecord{}, ErrSnapshotLeaseInvalid
	}
	dev, ino, err := fileIdentity(info)
	if err != nil {
		return leaseFileRecord{}, err
	}
	return leaseFileRecord{Name: name, SHA256: hex.EncodeToString(h.Sum(nil)), Size: n, Device: dev, Inode: ino}, nil
}
func pathIdentity(path string) (uint64, uint64, error) {
	i, e := os.Stat(path)
	if e != nil {
		return 0, 0, e
	}
	return fileIdentity(i)
}
func (s *SnapshotLeaseStore) removeLeaseDirectory(id string, device, inode uint64) (retErr error) {
	if !isCanonicalID(id) {
		return ErrSnapshotLeaseInvalid
	}
	if err := s.validateRoot(); err != nil {
		return err
	}
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	rootInfo, err := root.Stat(".")
	if err != nil {
		return err
	}
	d, i, err := fileIdentity(rootInfo)
	if err != nil || d != s.rootDevice || i != s.rootInode {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	entry, err := root.Lstat(id)
	if err != nil {
		return err
	}
	if !entry.IsDir() || entry.Mode()&os.ModeSymlink != 0 {
		return ErrSnapshotLeaseInvalid
	}
	d, i, err = fileIdentity(entry)
	if err != nil || d != device || i != inode {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	current, err := os.Lstat(filepath.Join(s.root, id))
	if err != nil || !os.SameFile(current, entry) {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	if err = root.RemoveAll(id); err != nil {
		return err
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	err = dir.Sync()
	return errors.Join(err, dir.Close())
}

func verifyRecordFiles(ctx context.Context, path string, r leaseDiskRecord) (retErr error) {
	dev, ino, e := pathIdentity(path)
	if e != nil {
		return e
	}
	if dev != r.RootDevice || ino != r.RootInode {
		return ErrSnapshotLeaseInvalid
	}
	root, e := os.OpenRoot(path)
	if e != nil {
		return e
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	rootInfo, e := root.Stat(".")
	if e != nil {
		return e
	}
	rootD, rootI, e := fileIdentity(rootInfo)
	if e != nil || rootD != r.RootDevice || rootI != r.RootInode {
		return errors.Join(ErrSnapshotLeaseInvalid, e)
	}
	for _, fr := range r.Files {
		if e := ctx.Err(); e != nil {
			return e
		}
		if filepath.Base(fr.Name) != fr.Name || fr.Name == "." || fr.Name == ".." {
			return ErrSnapshotLeaseInvalid
		}
		f, e := openRegularFile(filepath.Join(path, fr.Name))
		if e != nil {
			return e
		}
		i, e := f.Stat()
		if e != nil {
			_ = f.Close()
			return e
		}
		d, n, e := fileIdentity(i)
		if e != nil {
			_ = f.Close()
			return e
		}
		if !i.Mode().IsRegular() || i.Size() != fr.Size || d != fr.Device || n != fr.Inode {
			_ = f.Close()
			return ErrSnapshotLeaseInvalid
		}
		h := sha256.New()
		count, e := copyContext(ctx, h, f, fr.Size)
		ce := f.Close()
		if e != nil || ce != nil {
			return errors.Join(e, ce)
		}
		if count != fr.Size || hex.EncodeToString(h.Sum(nil)) != fr.SHA256 {
			return ErrSnapshotLeaseInvalid
		}
	}
	return nil
}
func copyContext(ctx context.Context, dst io.Writer, src io.Reader, limit int64) (int64, error) {
	buf := make([]byte, 64<<10)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, e := src.Read(buf)
		if n > 0 {
			if int64(n) > limit-total {
				return total, ErrSnapshotLeaseLimit
			}
			w, we := dst.Write(buf[:n])
			total += int64(w)
			if we != nil {
				return total, we
			}
			if w != n {
				return total, io.ErrShortWrite
			}
		}
		if e == io.EOF {
			return total, nil
		}
		if e != nil {
			return total, e
		}
	}
}
func removeRetainedScratch(root, name string, created os.FileInfo) error {
	p, e := os.OpenRoot(root)
	if e != nil {
		return e
	}
	removeErr := removeInspectionScratch(p, name, created)
	return errors.Join(removeErr, p.Close())
}
func encodeLeaseRecord(r *leaseDiskRecord, limit int64) ([]byte, error) {
	if limit > maxLeaseMetadataBytes {
		return nil, ErrSnapshotLeaseLimit
	}
	var manifestBytes int64
	for _, f := range r.Files {
		if f.Name == manifestFile {
			manifestBytes = f.Size
			break
		}
	}
	if manifestBytes < 0 || manifestBytes >= limit {
		return nil, ErrSnapshotLeaseLimit
	}
	r.MetadataBytes = manifestBytes
	r.Checksum = ""
	var raw []byte
	stable := false
	for i := 0; i < 8; i++ {
		r.Checksum = ""
		base, e := json.Marshal(r)
		if e != nil {
			return nil, e
		}
		r.Checksum = digestBytes(base)
		raw, e = json.Marshal(r)
		if e != nil {
			return nil, e
		}
		total := manifestBytes + int64(len(raw))
		if total == r.MetadataBytes {
			stable = true
			break
		}
		r.MetadataBytes = total
	}
	if !stable || r.MetadataBytes > limit {
		return nil, ErrSnapshotLeaseLimit
	}
	return raw, nil
}

func writeLeaseRecord(path string, r *leaseDiskRecord, limit int64) (retErr error) {
	raw, e := encodeLeaseRecord(r, limit)
	if e != nil {
		return e
	}
	tmpID, e := randomID()
	if e != nil {
		return e
	}
	leaf := filepath.Base(path)
	parent := filepath.Base(filepath.Dir(path))
	lstat, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !lstat.IsDir() || lstat.Mode()&os.ModeSymlink != 0 {
		return ErrSnapshotLeaseInvalid
	}
	root, e := os.OpenRoot(path)
	if e != nil {
		return e
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	rootInfo, e := root.Stat(".")
	if e != nil || !os.SameFile(lstat, rootInfo) {
		return errors.Join(ErrSnapshotLeaseInvalid, e)
	}
	if parent != ".releases" && leaf == r.ID {
		dev, ino, e := fileIdentity(rootInfo)
		if e != nil || dev != r.RootDevice || ino != r.RootInode {
			return errors.Join(ErrSnapshotLeaseInvalid, e)
		}
	}
	current, e := os.Lstat(path)
	if e != nil || !os.SameFile(current, rootInfo) {
		return errors.Join(ErrSnapshotLeaseInvalid, e)
	}
	tmp := ".lease.tmp-" + tmpID
	f, e := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	releasePhase := ""
	pinPhase := ""
	if r.State == "PIN_PENDING" {
		pinPhase = "pending"
	}
	if r.State == "PINNED" {
		pinPhase = "pinned"
	}
	if r.State == "RELEASING" {
		if parent == ".releases" {
			releasePhase = "marker"
		} else {
			releasePhase = "live"
		}
	}
	snapshotLeaseCaptureTemp(path, tmp, *r, raw, root, f)
	if pinPhase == "pending" {
		testhooks.At(testhooks.PhaseSnapshotPinPendingTempCreated)
	}
	if pinPhase == "pinned" {
		testhooks.At(testhooks.PhaseSnapshotPinPinnedTempCreated)
	}
	if releasePhase == "live" {
		testhooks.At(testhooks.PhaseSnapshotReleaseLiveRecordTempCreated)
	} else if releasePhase == "marker" {
		testhooks.At(testhooks.PhaseSnapshotReleaseMarkerRecordTempCreated)
	}
	n, we := f.Write(raw)
	if we == nil && n != len(raw) {
		we = io.ErrShortWrite
	}
	if we == nil && pinPhase == "pending" {
		testhooks.At(testhooks.PhaseSnapshotPinPendingTempWritten)
	}
	if we == nil && pinPhase == "pinned" {
		testhooks.At(testhooks.PhaseSnapshotPinPinnedTempWritten)
	}
	if we == nil {
		if releasePhase == "live" {
			testhooks.At(testhooks.PhaseSnapshotReleaseLiveRecordTempWritten)
		} else if releasePhase == "marker" {
			testhooks.At(testhooks.PhaseSnapshotReleaseMarkerRecordTempWritten)
		}
	}
	if we == nil {
		we = f.Sync()
	}
	if we == nil {
		if pinPhase == "pending" {
			testhooks.At(testhooks.PhaseSnapshotPinPendingTempSynced)
		}
		if pinPhase == "pinned" {
			testhooks.At(testhooks.PhaseSnapshotPinPinnedTempSynced)
		}
		if releasePhase == "live" {
			testhooks.At(testhooks.PhaseSnapshotReleaseLiveRecordTempSynced)
		} else if releasePhase == "marker" {
			testhooks.At(testhooks.PhaseSnapshotReleaseMarkerRecordTempSynced)
		}
	}
	ce := f.Close()
	if we != nil || ce != nil {
		return errors.Join(we, ce, root.Remove(tmp))
	}
	current, e = os.Lstat(path)
	if e != nil || !os.SameFile(current, rootInfo) {
		return errors.Join(ErrSnapshotLeaseInvalid, e, root.Remove(tmp))
	}
	if e = root.Rename(tmp, leaseRecordName); e != nil {
		return errors.Join(e, root.Remove(tmp))
	}
	if pinPhase == "pending" {
		testhooks.At(testhooks.PhaseSnapshotPinPendingPublished)
	}
	if pinPhase == "pinned" {
		testhooks.At(testhooks.PhaseSnapshotPinPinnedPublished)
	}
	if releasePhase == "live" {
		testhooks.At(testhooks.PhaseSnapshotReleaseLiveRecordPublished)
	} else if releasePhase == "marker" {
		testhooks.At(testhooks.PhaseSnapshotReleaseMarkerRecordPublished)
	}
	dir, e := root.Open(".")
	if e != nil {
		return e
	}
	e = dir.Sync()
	if e == nil {
		if pinPhase == "pending" {
			testhooks.At(testhooks.PhaseSnapshotPinPendingDirectorySynced)
		}
		if pinPhase == "pinned" {
			testhooks.At(testhooks.PhaseSnapshotPinPinnedDirectorySynced)
		}
		if releasePhase == "live" {
			testhooks.At(testhooks.PhaseSnapshotReleaseLiveRecordDirectorySynced)
		} else if releasePhase == "marker" {
			testhooks.At(testhooks.PhaseSnapshotReleaseMarkerRecordDirectorySynced)
		}
	}
	e = errors.Join(e, dir.Close())
	current, statErr := os.Lstat(path)
	if statErr != nil || !os.SameFile(current, rootInfo) {
		e = errors.Join(e, ErrSnapshotLeaseInvalid, statErr)
	}
	return e
}

func readLeaseRecord(path string, limit int64) (r leaseDiskRecord, retErr error) {
	f, e := openRegularFile(filepath.Join(path, leaseRecordName))
	if e != nil {
		return r, e
	}
	defer func() { retErr = errors.Join(retErr, f.Close()) }()
	raw, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil {
		return r, e
	}
	if int64(len(raw)) > limit {
		return r, ErrSnapshotLeaseLimit
	}
	if e = rejectDuplicateJSONKeys(raw); e != nil {
		return r, errors.Join(ErrSnapshotLeaseInvalid, e)
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e = d.Decode(&r); e != nil {
		return r, errors.Join(ErrSnapshotLeaseInvalid, e)
	}
	if e = d.Decode(new(any)); e != io.EOF {
		return r, ErrSnapshotLeaseInvalid
	}
	checksum := r.Checksum
	r.Checksum = ""
	base, e := json.Marshal(r)
	r.Checksum = checksum
	if e != nil || digestBytes(base) != checksum {
		return r, fmt.Errorf("%w: lease checksum", ErrSnapshotLeaseInvalid)
	}
	canonical, e := json.Marshal(r)
	if e != nil || !bytes.Equal(canonical, raw) {
		return r, fmt.Errorf("%w: lease encoding is not canonical", ErrSnapshotLeaseInvalid)
	}
	if r.Version != leaseMetadataVersion || !isCanonicalID(r.ID) || !isSHA256(r.ManifestSHA256) || r.ID != filepath.Base(path) {
		return r, fmt.Errorf("%w: lease identity fields", ErrSnapshotLeaseInvalid)
	}
	manifestSize := int64(0)
	for _, f := range r.Files {
		if f.Name == manifestFile {
			manifestSize = f.Size
		}
	}
	if r.MetadataBytes != manifestSize+int64(len(raw)) {
		return r, fmt.Errorf("%w: lease metadata accounting got=%d want=%d", ErrSnapshotLeaseInvalid, r.MetadataBytes, manifestSize+int64(len(raw)))
	}
	if e = validateLeaseRecord(r); e != nil {
		return r, fmt.Errorf("invalid lease invariants: %w", e)
	}
	return r, nil
}

func validateLeaseRecord(r leaseDiskRecord) error {
	if r.ArtifactBytes <= 0 || r.ArtifactBytes > maxLeaseBytes || r.MetadataBytes <= 0 || r.MetadataBytes > maxLeaseMetadataBytes || r.CreatedUnix <= 0 || r.RootDevice == 0 || r.RootInode == 0 || r.Inspection.ManifestSHA256 != r.ManifestSHA256 {
		return ErrSnapshotLeaseInvalid
	}
	declared, _, err := declaredArtifactSize(r.Manifest, maxLeaseBytes)
	if err != nil || declared != r.ArtifactBytes || r.Inspection.DeclaredArtifactBytes != declared {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	if len(r.Files) == 0 {
		return ErrSnapshotLeaseInvalid
	}
	expected := map[string]leaseFileRecord{controlDBName: {Name: controlDBName, SHA256: r.Manifest.ControlDB.SHA256, Size: r.Manifest.ControlDB.LengthBytes}, manifestFile: {Name: manifestFile, SHA256: r.ManifestSHA256}}
	for _, brain := range r.Manifest.Brains {
		if !brain.Empty {
			expected[brain.ID+".bundle"] = leaseFileRecord{Name: brain.ID + ".bundle", SHA256: brain.SHA256, Size: brain.LengthBytes}
		}
	}
	if len(expected) != len(r.Files) {
		return ErrSnapshotLeaseInvalid
	}
	seen := map[string]bool{}
	var manifestSize int64
	for _, f := range r.Files {
		want, ok := expected[f.Name]
		if !ok || seen[f.Name] || !isSHA256(f.SHA256) || f.Size <= 0 || f.Size > maxLeaseBytes || f.Device == 0 || f.Inode == 0 {
			return ErrSnapshotLeaseInvalid
		}
		if want.SHA256 != "" && f.SHA256 != want.SHA256 {
			return ErrSnapshotLeaseInvalid
		}
		if f.Name != manifestFile && f.Size != want.Size {
			return ErrSnapshotLeaseInvalid
		}
		if f.Name == manifestFile {
			manifestSize = f.Size
			if f.SHA256 != r.ManifestSHA256 {
				return ErrSnapshotLeaseInvalid
			}
		}
		seen[f.Name] = true
	}
	switch r.State {
	case "STAGED":
		if r.PlanRef != "" || r.PinID != "" || r.ReservationVersion != 0 || r.AttemptVersion != 0 || r.Disposition != 0 || r.RecordVersion != 0 {
			return ErrSnapshotLeaseInvalid
		}
	case "PIN_PENDING", "PINNED":
		if !validPlanRef(r.PlanRef) || !isCanonicalID(r.PinID) || r.ReservationVersion == 0 || r.AttemptVersion == 0 || r.Disposition != 0 || r.RecordVersion != 0 {
			return ErrSnapshotLeaseInvalid
		}
	case "RELEASING", "RELEASED":
		if !validPlanRef(r.PlanRef) || !isCanonicalID(r.PinID) || r.ReservationVersion == 0 || r.AttemptVersion == 0 || r.RecordVersion == 0 || (r.Disposition != PinAbandonedBeforeEffects && r.Disposition != PinCommittedRestoreComplete) {
			return ErrSnapshotLeaseInvalid
		}
	default:
		return ErrSnapshotLeaseInvalid
	}
	if manifestSize <= 0 {
		return ErrSnapshotLeaseInvalid
	}
	return nil
}
func (s *SnapshotLeaseStore) ensureInitialReleasePeak(current, candidate leaseDiskRecord) error {
	usage, err := s.usageLocked()
	if err != nil {
		return err
	}
	if usage.releaseEntries >= maxReleaseJournalEntries {
		return ErrSnapshotLeaseLimit
	}
	_, candidateRaw, err := encodeLeaseRecord(&candidate, s.options.MaxMetadataBytesPerLease)
	if err != nil {
		return err
	}
	_, tombstoneRaw, err := encodeReleaseTombstone(candidate)
	if err != nil {
		return err
	}
	base := usage.metadata - current.MetadataBytes
	if base < 0 {
		return ErrSnapshotLeaseInvalid
	}
	rawBytes := int64(len(candidateRaw))
	m := candidate.MetadataBytes
	old := current.MetadataBytes
	tomb := int64(len(tombstoneRaw))
	if base > math.MaxInt64-old || old > math.MaxInt64-rawBytes || base+old > math.MaxInt64-rawBytes || m > math.MaxInt64-rawBytes || base > math.MaxInt64-m || base+m > math.MaxInt64-m || base+2*m > math.MaxInt64-tomb {
		return ErrSnapshotLeaseLimit
	}
	peak := base + old + rawBytes
	if markerTempPeak := base + m + rawBytes; markerTempPeak > peak {
		peak = markerTempPeak
	}
	if twoRecordsPeak := base + 2*m; twoRecordsPeak > peak {
		peak = twoRecordsPeak
	}
	if deletionPeak := base + 2*m + tomb; deletionPeak > peak {
		peak = deletionPeak
	}
	if peak > s.options.MaxRetainedMetadataBytes {
		return ErrSnapshotLeaseLimit
	}
	return nil
}

func (s *SnapshotLeaseStore) ensureReleasePeakBeforeOwner(ctx context.Context, current leaseDiskRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	candidate := current
	candidate.State = "RELEASING"
	// Both currently valid dispositions have one-digit wire encodings. Use the
	// larger valid value and the largest possible record version to reserve a
	// conservative encoded peak without treating this value as authority.
	candidate.Disposition = PinCommittedRestoreComplete
	candidate.RecordVersion = math.MaxUint64
	return s.ensureInitialReleasePeak(current, candidate)
}

func (s *SnapshotLeaseStore) releaseLease(ctx context.Context, path string, r leaseDiskRecord, a PinReleaseAuthorization) error {
	if !validRelease(a, r) {
		return ErrSnapshotLeaseConflict
	}
	if err := s.validateRoot(); err != nil {
		return err
	}
	if err := verifyRecordFiles(ctx, path, r); err != nil {
		return err
	}
	r.State = "RELEASING"
	r.Disposition = a.Disposition
	r.RecordVersion = a.RecordVersion
	candidate := r
	if _, err := encodeLeaseRecord(&candidate, s.options.MaxMetadataBytesPerLease); err != nil {
		return err
	}
	if err := s.ensureInitialReleasePeak(r, candidate); err != nil {
		return err
	}
	if err := s.validateRoot(); err != nil {
		return err
	}
	if err := writeLeaseRecord(path, &r, s.options.MaxMetadataBytesPerLease); err != nil {
		return err
	}
	if err := s.validateReleaseRoot(); err != nil {
		return err
	}
	journal := filepath.Join(s.root, ".releases")
	marker := filepath.Join(journal, r.ID)
	if err := os.Mkdir(marker, 0700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrSnapshotLeaseConflict
		}
		return err
	} else {
		testhooks.At(testhooks.PhaseSnapshotReleaseMarkerCreated)
	}
	if existing, err := readLeaseRecord(marker, maxLeaseMetadataBytes); err == nil {
		if existing.ID != r.ID || existing.PinID != r.PinID || existing.PlanRef != r.PlanRef || existing.ManifestSHA256 != r.ManifestSHA256 || existing.ReservationVersion != r.ReservationVersion || existing.AttemptVersion != r.AttemptVersion || existing.Disposition != r.Disposition || existing.RecordVersion != r.RecordVersion || (existing.State != "RELEASING" && existing.State != "RELEASED") {
			return ErrSnapshotLeaseConflict
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := writeLeaseRecord(marker, &r, maxLeaseMetadataBytes); err != nil {
		return err
	}
	if err := syncTree(marker); err != nil {
		return err
	}
	if err := fsyncDir(journal); err != nil {
		return err
	}
	testhooks.At(testhooks.PhaseSnapshotReleaseMarkerJournalSynced)
	if err := s.validateReleaseRoot(); err != nil {
		return err
	}
	if err := s.ensureReleasePeakCapacity(r); err != nil {
		return err
	}
	if err := s.removeLeaseDirectory(r.ID, r.RootDevice, r.RootInode); err != nil {
		return err
	}
	testhooks.At(testhooks.PhaseSnapshotReleaseLiveTreeRemoved)
	if err := fsyncDir(s.root); err != nil {
		return err
	}
	testhooks.At(testhooks.PhaseSnapshotReleaseLiveTreeRootSynced)
	r.State = "RELEASED"
	if err := s.writeReleaseTombstoneWithCapacity(marker, r); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(marker, leaseRecordName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	testhooks.At(testhooks.PhaseSnapshotReleaseMarkerRecordRemoved)
	if err := fsyncDir(marker); err != nil {
		return err
	}
	testhooks.At(testhooks.PhaseSnapshotReleaseMarkerDirectorySynced)
	if err := fsyncDir(journal); err != nil {
		return err
	}
	testhooks.At(testhooks.PhaseSnapshotReleaseTombstoneJournalSynced)
	return nil
}

func validateJournalIdentity(s *SnapshotLeaseStore, journal string) error {
	info, err := os.Lstat(journal)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	dev, ino, err := fileIdentity(info)
	if err != nil || dev != s.releaseDevice || ino != s.releaseInode {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	return nil
}
func verifyRootIdentity(s *SnapshotLeaseStore, dev, ino uint64) error {
	info, err := os.Lstat(s.root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	d, i, err := fileIdentity(info)
	if err != nil || d != dev || i != ino || d != s.rootDevice || i != s.rootInode {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	return nil
}
func verifyMarkerIdentity(path string, dev, ino uint64) error {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	d, i, err := fileIdentity(info)
	if err != nil || d != dev || i != ino {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	return nil
}
func captureLeaseRecordIdentity(path string, record leaseDiskRecord, limit int64) (os.FileInfo, error) {
	info, err := os.Lstat(filepath.Join(path, leaseRecordName))
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	current, err := readLeaseRecord(path, limit)
	if err != nil || !sameLeaseRecord(current, record) {
		return nil, errors.Join(ErrSnapshotLeaseConflict, err)
	}
	return info, nil
}
func verifyLeaseRecordUnchanged(path string, expected os.FileInfo, record leaseDiskRecord, limit int64) error {
	currentInfo, err := os.Lstat(filepath.Join(path, leaseRecordName))
	if err != nil || !os.SameFile(expected, currentInfo) || !currentInfo.Mode().IsRegular() || currentInfo.Mode()&os.ModeSymlink != 0 {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	current, err := readLeaseRecord(path, limit)
	if err != nil || !sameLeaseRecord(current, record) {
		return errors.Join(ErrSnapshotLeaseConflict, err)
	}
	return nil
}
func verifyReleaseReceiptUnchanged(path string, expected os.FileInfo, expectedTombstone releaseTombstone) error {
	if err := verifyRetainedReleaseReceipt(path, expected); err != nil {
		return err
	}
	current, err := readReleaseTombstone(path)
	if err != nil || current != expectedTombstone {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	return nil
}

func verifyRetainedReleaseReceipt(path string, expected os.FileInfo) error {
	current, err := os.Lstat(filepath.Join(path, releaseRecordName))
	if err != nil || !current.Mode().IsRegular() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(expected, current) {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	return nil
}
func verifyLeaseAbsent(path string) error {
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return ErrSnapshotLeaseConflict
}

func verifyLeaseDirectoryIdentity(path string, expected os.FileInfo) error {
	current, err := os.Lstat(path)
	if err != nil || !current.IsDir() || current.Mode()&os.ModeSymlink != 0 || !os.SameFile(expected, current) {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	return nil
}

func encodeReleaseTombstone(r leaseDiskRecord) (releaseTombstone, []byte, error) {
	t := releaseTombstone{Version: 1, ID: r.ID, PinID: r.PinID, PlanRef: r.PlanRef, ManifestSHA256: r.ManifestSHA256, ReservationVersion: r.ReservationVersion, AttemptVersion: r.AttemptVersion, Disposition: r.Disposition, RecordVersion: r.RecordVersion, State: "RELEASED"}
	for i := 0; i < 8; i++ {
		t.Checksum = ""
		base, err := json.Marshal(t)
		if err != nil {
			return t, nil, err
		}
		t.Checksum = digestBytes(base)
		raw, err := json.Marshal(t)
		if err != nil {
			return t, nil, err
		}
		if t.MetadataBytes == int64(len(raw)) {
			if !validReleaseTombstone(t) {
				return t, nil, ErrSnapshotLeaseInvalid
			}
			return t, raw, nil
		}
		t.MetadataBytes = int64(len(raw))
	}
	return t, nil, ErrSnapshotLeaseLimit
}
func (s *SnapshotLeaseStore) ensureReleasePeakCapacity(r leaseDiskRecord) error {
	u, err := s.usageLocked()
	if err != nil {
		return err
	}
	_, raw, err := encodeReleaseTombstone(r)
	if err != nil {
		return err
	}
	if u.metadata < 0 || int64(len(raw)) > math.MaxInt64-u.metadata || u.metadata+int64(len(raw)) > s.options.MaxRetainedMetadataBytes {
		return ErrSnapshotLeaseLimit
	}
	return nil
}
func (s *SnapshotLeaseStore) writeReleaseTombstoneWithCapacity(path string, r leaseDiskRecord) error {
	if err := s.ensureReleasePeakCapacity(r); err != nil {
		return err
	}
	return writeReleaseTombstone(path, r)
}

func (s *SnapshotLeaseStore) finishReleaseJournal(ctx context.Context, attempts []PinAttemptRef) error {
	entries, err := s.readReleaseJournalEntries()
	if err != nil {
		return err
	}
	journal := filepath.Join(s.root, ".releases")
	var errs []error
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return errors.Join(errors.Join(errs...), err)
		}
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || !isCanonicalID(entry.Name()) {
			errs = append(errs, ErrSnapshotLeaseInvalid)
			continue
		}
		marker := filepath.Join(journal, entry.Name())
		markerInfo, e := os.Lstat(marker)
		if e != nil || !markerInfo.IsDir() || markerInfo.Mode()&os.ModeSymlink != 0 {
			errs = append(errs, errors.Join(ErrSnapshotLeaseInvalid, e))
			continue
		}
		markerDev, markerIno, e := fileIdentity(markerInfo)
		if e != nil {
			errs = append(errs, e)
			continue
		}
		rootInfo, e := os.Lstat(s.root)
		if e != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
			errs = append(errs, errors.Join(ErrSnapshotLeaseInvalid, e))
			continue
		}
		rootDev, rootIno, e := fileIdentity(rootInfo)
		if e != nil || rootDev != s.rootDevice || rootIno != s.rootInode {
			errs = append(errs, errors.Join(ErrSnapshotLeaseInvalid, e))
			continue
		}
		if e = validateJournalIdentity(s, journal); e != nil {
			errs = append(errs, e)
			continue
		}
		tombstone, tombstoneErr := readReleaseTombstone(marker)
		if tombstoneErr == nil {
			r := leaseDiskRecord{ID: tombstone.ID, PinID: tombstone.PinID, PlanRef: tombstone.PlanRef, ManifestSHA256: tombstone.ManifestSHA256, ReservationVersion: tombstone.ReservationVersion, AttemptVersion: tombstone.AttemptVersion, Disposition: tombstone.Disposition, RecordVersion: tombstone.RecordVersion, State: tombstone.State, MetadataBytes: tombstone.MetadataBytes}
			if r.ID != entry.Name() || r.State != "RELEASED" || releaseAttemptMatchesRecord(attempts, r) != nil {
				errs = append(errs, ErrSnapshotLeaseConflict)
				continue
			}
			if _, statErr := os.Lstat(filepath.Join(s.root, r.ID)); !errors.Is(statErr, os.ErrNotExist) {
				errs = append(errs, errors.Join(ErrSnapshotLeaseConflict, statErr))
				continue
			}
			receiptInfo, receiptErr := os.Lstat(filepath.Join(marker, releaseRecordName))
			if receiptErr != nil || !receiptInfo.Mode().IsRegular() || receiptInfo.Mode()&os.ModeSymlink != 0 {
				errs = append(errs, errors.Join(ErrSnapshotLeaseInvalid, receiptErr))
				continue
			}
			if e = verifyMarkerIdentity(marker, markerDev, markerIno); e != nil {
				errs = append(errs, e)
				continue
			}
			if e = s.authority.CompletePinRelease(ctx, tombstone.authorization()); e != nil {
				errs = append(errs, e)
				continue
			}
			if e = ctx.Err(); e != nil {
				errs = append(errs, e)
				continue
			}
			if e = verifyMarkerIdentity(marker, markerDev, markerIno); e != nil {
				errs = append(errs, e)
				continue
			}
			if e = verifyRootIdentity(s, rootDev, rootIno); e == nil {
				e = validateJournalIdentity(s, journal)
			}
			if e == nil {
				e = verifyReleaseReceiptUnchanged(marker, receiptInfo, tombstone)
			}
			if e == nil {
				e = verifyLeaseAbsent(filepath.Join(s.root, r.ID))
			}
			if e != nil {
				errs = append(errs, e)
				continue
			}
			// Keep the exact receipt directory: the owner retains the committed attempt.
			continue
		}
		if !errors.Is(tombstoneErr, os.ErrNotExist) {
			errs = append(errs, tombstoneErr)
			continue
		}
		r, e := readLeaseRecord(marker, maxLeaseMetadataBytes)
		if e != nil || r.ID != entry.Name() || r.State != "RELEASING" {
			errs = append(errs, errors.Join(ErrSnapshotLeaseInvalid, e))
			continue
		}
		if e = releaseAttemptMatchesRecord(attempts, r); e != nil {
			errs = append(errs, e)
			continue
		}
		markerRecordInfo, e := captureLeaseRecordIdentity(marker, r, maxLeaseMetadataBytes)
		if e != nil {
			errs = append(errs, e)
			continue
		}
		live := filepath.Join(s.root, r.ID)
		liveInfo, liveErr := os.Lstat(live)
		var lock *os.File
		if liveErr == nil {
			if !liveInfo.IsDir() || liveInfo.Mode()&os.ModeSymlink != 0 {
				errs = append(errs, ErrSnapshotLeaseInvalid)
				continue
			}
			lock, e = lockLeaseContext(ctx, live, false)
			if e != nil {
				errs = append(errs, e)
				continue
			}
		} else if !errors.Is(liveErr, os.ErrNotExist) {
			errs = append(errs, liveErr)
			continue
		}
		var liveRecordInfo os.FileInfo
		if lock != nil {
			liveRecordInfo, e = captureLeaseRecordIdentity(live, r, maxLeaseMetadataBytes)
		}
		a, authErr := s.authorizeReleaseResume(ctx, r, attempts)
		if e == nil {
			e = authErr
		}
		if e == nil {
			e = verifyLeaseRecordUnchanged(marker, markerRecordInfo, r, maxLeaseMetadataBytes)
		}
		if e == nil {
			e = verifyMarkerIdentity(marker, markerDev, markerIno)
		}
		if e == nil {
			e = verifyRootIdentity(s, rootDev, rootIno)
		}
		if e == nil && lock != nil {
			e = verifyLeaseDirectoryIdentity(live, liveInfo)
		}
		if e == nil && lock != nil {
			e = verifyLeaseRecordUnchanged(live, liveRecordInfo, r, maxLeaseMetadataBytes)
		}
		if e != nil {
			if lock != nil {
				_ = unlockFile(lock)
				_ = lock.Close()
			}
			errs = append(errs, e)
			continue
		}
		if lock != nil {
			e = verifyReleasingLease(ctx, live, r, lock, true)
			if e == nil {
				e = s.ensureReleasePeakCapacity(r)
			}
			if e == nil {
				e = s.removeLeaseDirectory(r.ID, r.RootDevice, r.RootInode)
				testhooks.At(testhooks.PhaseSnapshotReleaseLiveTreeRemoved)
			}
			if e == nil {
				e = fsyncDir(s.root)
				if e == nil {
					testhooks.At(testhooks.PhaseSnapshotReleaseRootSynced)
				}
			}
			e = errors.Join(e, unlockFile(lock), lock.Close())
		}
		if e == nil {
			e = s.writeReleaseTombstoneWithCapacity(marker, r)
		}
		if e == nil {
			e = verifyMarkerIdentity(marker, markerDev, markerIno)
		}
		if e == nil {
			e = os.Remove(filepath.Join(marker, leaseRecordName))
			if e == nil {
				testhooks.At(testhooks.PhaseSnapshotReleaseMarkerRecordRemoved)
			}
		}
		if e == nil {
			e = fsyncDir(marker)
			if e == nil {
				testhooks.At(testhooks.PhaseSnapshotReleaseMarkerDirectorySynced)
			}
		}
		if e == nil {
			e = fsyncDir(journal)
			if e == nil {
				testhooks.At(testhooks.PhaseSnapshotReleaseTombstoneJournalSynced)
			}
		}
		var receiptInfo os.FileInfo
		if e == nil {
			receiptInfo, e = os.Lstat(filepath.Join(marker, releaseRecordName))
		}
		var expectedTombstone releaseTombstone
		if e == nil && (!receiptInfo.Mode().IsRegular() || receiptInfo.Mode()&os.ModeSymlink != 0) {
			e = ErrSnapshotLeaseInvalid
		}
		if e == nil {
			expectedTombstone, e = readReleaseTombstone(marker)
		}
		if e == nil {
			testhooks.At(testhooks.PhaseSnapshotReleaseOwnerCompletionStarting)
			e = s.authority.CompletePinRelease(ctx, a)
			if e == nil {
				testhooks.At(testhooks.PhaseSnapshotReleaseOwnerCompleted)
			}
		}
		if e == nil {
			e = verifyReleaseReceiptUnchanged(marker, receiptInfo, expectedTombstone)
		}
		if e == nil {
			e = verifyMarkerIdentity(marker, markerDev, markerIno)
		}
		if e == nil {
			e = verifyRootIdentity(s, rootDev, rootIno)
		}
		if e == nil {
			e = validateJournalIdentity(s, journal)
		}
		if e == nil {
			e = verifyLeaseAbsent(live)
		}
		if e != nil {
			errs = append(errs, e)
		}
	}
	return errors.Join(errs...)
}
