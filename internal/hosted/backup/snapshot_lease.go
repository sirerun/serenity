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
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/privatefs"
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
	leaseRecordName       = "lease.json"
	releaseRecordName     = "release.json"
	maxReleaseRecordBytes = 4096
	leaseMetadataVersion  = 1
	maxLeaseMetadataBytes = 1 << 20
	maxLeaseBytes         = int64(1 << 40)
	maxCandidateTextBytes = 256
	maxCandidateRows      = 64
	stageOrphanGrace      = 24 * time.Hour
)

type SnapshotLeaseStoreOptions struct {
	LeaseRoot                string
	MaxArtifactBytesPerLease int64
	MaxMetadataBytesPerLease int64
	MaxRestoreScratchBytes   int64
	MaxRetainedArtifactBytes int64
	MaxRetainedMetadataBytes int64
	MaxLeases                int
}

type SnapshotLeaseStore struct {
	root       string
	rootDevice uint64
	rootInode  uint64
	options    SnapshotLeaseStoreOptions
	authority  SnapshotPinLifecycleAuthority
	mu         sync.Mutex
	closed     bool
}

type SnapshotPinLifecycleAuthority interface {
	BeginPinAttempt(ctx context.Context, planRef string, reservationVersion uint64, leaseID, manifestSHA256 string) (PinAttemptRef, error)
	CommitPinAttempt(ctx context.Context, attempt PinAttemptRef, pin SnapshotPinRef) error
	CancelPinAttempt(ctx context.Context, attempt PinAttemptRef) error
	FindPinAttempt(ctx context.Context, planRef, manifestSHA256 string) (PinAttemptRef, error)
	ListPinAttempts(ctx context.Context) ([]PinAttemptRef, error)
	ReconcilePin(ctx context.Context, pin SnapshotPinRef) (PinReconcileDecision, error)
	CompletePinRelease(ctx context.Context, authorization PinReleaseAuthorization) error
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

type releaseTombstone struct {
	Version        int                   `json:"version"`
	Checksum       string                `json:"checksum"`
	ID             string                `json:"id"`
	PinID          string                `json:"pin_id"`
	PlanRef        string                `json:"plan_ref"`
	ManifestSHA256 string                `json:"manifest_sha256"`
	Disposition    PinReleaseDisposition `json:"disposition"`
	RecordVersion  uint64                `json:"record_version"`
	State          string                `json:"state"`
	MetadataBytes  int64                 `json:"metadata_bytes"`
}

func (t releaseTombstone) authorization() PinReleaseAuthorization {
	return PinReleaseAuthorization{PinID: t.PinID, PlanRef: t.PlanRef, ManifestSHA256: t.ManifestSHA256, Disposition: t.Disposition, RecordVersion: t.RecordVersion}
}

func validReleaseTombstone(t releaseTombstone) bool {
	return t.Version == 1 && isCanonicalID(t.ID) && isCanonicalID(t.PinID) && validPlanRef(t.PlanRef) && isSHA256(t.ManifestSHA256) && t.RecordVersion > 0 && t.State == "RELEASED" && (t.Disposition == PinAbandonedBeforeEffects || t.Disposition == PinCommittedRestoreComplete) && t.MetadataBytes > 0 && t.MetadataBytes <= maxReleaseRecordBytes
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
	t := releaseTombstone{Version: 1, ID: r.ID, PinID: r.PinID, PlanRef: r.PlanRef, ManifestSHA256: r.ManifestSHA256, Disposition: r.Disposition, RecordVersion: r.RecordVersion, State: "RELEASED"}
	var raw []byte
	stable := false
	for i := 0; i < 8; i++ {
		t.Checksum = ""
		base, err := json.Marshal(t)
		if err != nil {
			return err
		}
		t.Checksum = digestBytes(base)
		raw, err = json.Marshal(t)
		if err != nil {
			return err
		}
		if t.MetadataBytes == int64(len(raw)) {
			stable = true
			break
		}
		t.MetadataBytes = int64(len(raw))
	}
	if !stable || len(raw) > maxReleaseRecordBytes {
		return ErrSnapshotLeaseLimit
	}
	if !validReleaseTombstone(t) {
		return ErrSnapshotLeaseInvalid
	}
	if existing, err := readReleaseTombstone(path); err == nil {
		if existing.ID != t.ID || existing.PinID != t.PinID || existing.PlanRef != t.PlanRef || existing.ManifestSHA256 != t.ManifestSHA256 || existing.Disposition != t.Disposition || existing.RecordVersion != t.RecordVersion {
			return ErrSnapshotLeaseConflict
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
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
	rootInfo, err := root.Stat(".")
	if err != nil || !os.SameFile(before, rootInfo) {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	current, err := os.Lstat(path)
	if err != nil || !os.SameFile(current, rootInfo) {
		return errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	tempID, err := randomID()
	if err != nil {
		return err
	}
	temp := ".release.tmp-" + tempID
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(raw)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		return errors.Join(writeErr, closeErr, root.Remove(temp))
	}
	current, err = os.Lstat(path)
	if err != nil || !os.SameFile(current, rootInfo) {
		return errors.Join(ErrSnapshotLeaseInvalid, err, root.Remove(temp))
	}
	if err = root.Rename(temp, releaseRecordName); err != nil {
		return errors.Join(err, root.Remove(temp))
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	err = dir.Sync()
	return errors.Join(err, dir.Close())
}

type VerifiedSnapshotLease struct {
	store         *SnapshotLeaseStore
	record        leaseDiskRecord
	path          string
	mu            sync.Mutex
	cond          *sync.Cond
	borrowers     int
	closing       bool
	closed        bool
	restoreActive bool
	borrowLocks   []*os.File
}

func NewSnapshotLeaseStore(ctx context.Context, options SnapshotLeaseStoreOptions, lifecycle SnapshotPinLifecycleAuthority) (store *SnapshotLeaseStore, retErr error) {
	if isNilInterface(ctx) {
		return nil, ErrNilContext
	}
	if isNilInterface(lifecycle) {
		return nil, fmt.Errorf("%w: lifecycle authority is required", ErrSnapshotLeaseInvalid)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if options.LeaseRoot == "" || options.MaxLeases < 1 || options.MaxLeases > maxInspectionCount || options.MaxArtifactBytesPerLease < 1 || options.MaxArtifactBytesPerLease > maxLeaseBytes || options.MaxMetadataBytesPerLease < 1 || options.MaxMetadataBytesPerLease > maxLeaseMetadataBytes || options.MaxRestoreScratchBytes < 1 || options.MaxRestoreScratchBytes > maxLeaseBytes || options.MaxRetainedArtifactBytes < options.MaxArtifactBytesPerLease || options.MaxRetainedArtifactBytes > maxLeaseBytes || options.MaxRetainedMetadataBytes < options.MaxMetadataBytesPerLease || options.MaxRetainedMetadataBytes > maxLeaseBytes {
		return nil, fmt.Errorf("%w: invalid store limits", ErrSnapshotLeaseLimit)
	}
	if err := privatefs.ValidateDirectory(ctx, options.LeaseRoot); err != nil {
		return nil, fmt.Errorf("hosted/backup: validate lease root: %w", err)
	}
	dev, ino, err := pathIdentity(options.LeaseRoot)
	if err != nil {
		return nil, err
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
	if err != nil || rootDev != dev || rootIno != ino {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	if err = root.Mkdir(".releases", 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	releases := filepath.Join(rootPath, ".releases")
	if err = privatefs.ValidateDirectory(ctx, releases); err != nil {
		return nil, err
	}
	current, err := os.Lstat(rootPath)
	if err != nil || !os.SameFile(current, rootInfo) {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err)
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
	return &SnapshotLeaseStore{root: rootPath, rootDevice: dev, rootInode: ino, options: options, authority: lifecycle}, nil
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
	if usage.leases >= s.options.MaxLeases {
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
	id, err := randomID()
	if err != nil {
		return nil, err
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
	if err = os.Rename(verified.scratch, leasePath); err != nil {
		return nil, err
	}
	keep = true // The renamed STAGED record is durable inventory even if the parent sync fails.
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

func (l *VerifiedSnapshotLease) Candidate(ctx context.Context, accountID string) (candidate VerifiedAccountCandidate, retErr error) {
	if !safeID(accountID) {
		return nil, ErrSnapshotLeaseInvalid
	}
	if err := l.beginBorrow(ctx, false); err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, l.endBorrow()) }()
	if err := verifyRecordFiles(ctx, l.path, l.record); err != nil {
		return nil, err
	}
	db, err := openSnapshotControlDB(ctx, filepath.Join(l.path, controlDBName), l.record.Manifest.Source.SchemaVersion)
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
	if err = writeLeaseRecord(filepath.Join(s.root, record.ID), &record, s.options.MaxMetadataBytesPerLease); err != nil {
		return SnapshotPinRef{}, fmt.Errorf("write PIN_PENDING lease record: %w", err)
	}
	if err = fsyncDir(filepath.Join(s.root, record.ID)); err != nil {
		return SnapshotPinRef{}, fmt.Errorf("sync PIN_PENDING lease directory: %w", err)
	}
	if err = fsyncDir(s.root); err != nil {
		return SnapshotPinRef{}, fmt.Errorf("sync PIN_PENDING store root: %w", err)
	}
	pin := pinRef(record)
	if err = s.authority.CommitPinAttempt(ctx, attempt, pin); err != nil {
		return SnapshotPinRef{}, err
	}
	if err = ctx.Err(); err != nil {
		return SnapshotPinRef{}, err
	}
	record.State = "PINNED"
	if err = writeLeaseRecord(filepath.Join(s.root, record.ID), &record, s.options.MaxMetadataBytesPerLease); err != nil {
		return SnapshotPinRef{}, err
	}
	if err = fsyncDir(filepath.Join(s.root, record.ID)); err != nil {
		return SnapshotPinRef{}, err
	}
	if err = fsyncDir(s.root); err != nil {
		return SnapshotPinRef{}, err
	}
	return pin, nil
}

func (s *SnapshotLeaseStore) CancelPin(ctx context.Context, attempt PinAttemptRef) (retErr error) {
	if !validPlanRef(attempt.PlanRef) || !isCanonicalID(attempt.LeaseID) || !isSHA256(attempt.ManifestSHA256) || attempt.ReservationVersion == 0 || attempt.AttemptVersion == 0 {
		return ErrSnapshotLeaseInvalid
	}
	unlock, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, unlock()) }()
	path := filepath.Join(s.root, attempt.LeaseID)
	if _, err = os.Lstat(path); err == nil {
		leaseLock, lockErr := lockLeaseContext(ctx, path, false)
		if lockErr != nil {
			return lockErr
		}
		defer func() { retErr = errors.Join(retErr, unlockFile(leaseLock), leaseLock.Close()) }()
		r, readErr := readLeaseRecord(path, s.options.MaxMetadataBytesPerLease)
		if readErr != nil {
			return readErr
		}
		if r.ManifestSHA256 != attempt.ManifestSHA256 {
			return ErrSnapshotLeaseConflict
		}
		if r.State == "PIN_PENDING" || r.State == "PINNED" {
			if r.PlanRef == attempt.PlanRef && r.ReservationVersion == attempt.ReservationVersion && r.AttemptVersion == attempt.AttemptVersion {
				return ErrSnapshotLeaseConflict
			}
		} else if r.State != "STAGED" {
			return ErrSnapshotLeaseConflict
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// The owner implements exact-attempt CAS and durable canceled tombstones.
	// It must not consult or mutate a later attempt when this is a retry of N.
	if err = s.authority.CancelPinAttempt(ctx, attempt); err != nil {
		return err
	}
	return ctx.Err()
}

func (s *SnapshotLeaseStore) Reconcile(ctx context.Context) (retErr error) {
	unlock, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, unlock()) }()
	if err = s.finishReleaseJournal(ctx); err != nil {
		return err
	}
	attempts, err := s.authority.ListPinAttempts(ctx)
	if err != nil {
		return err
	}
	protected := map[string]bool{}
	for _, a := range attempts {
		if a.State == PinAttemptPending {
			protected[a.LeaseID] = true
		}
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	var errs []error
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
		}
		if r.State == "RELEASING" {
			authorization := PinReleaseAuthorization{PinID: r.PinID, PlanRef: r.PlanRef, ManifestSHA256: r.ManifestSHA256, Disposition: r.Disposition, RecordVersion: r.RecordVersion}
			leaseLock, e3 := lockLeaseContext(ctx, path, false)
			if e3 != nil {
				errs = append(errs, e3)
				continue
			}
			e2 = s.releaseLease(ctx, path, r, authorization)
			e2 = errors.Join(e2, unlockFile(leaseLock), leaseLock.Close())
			if e2 != nil {
				errs = append(errs, e2)
			}
			continue
		}
		if r.State == "PINNED" {
			decision, e2 := s.authority.ReconcilePin(ctx, pinRef(r))
			if e2 != nil {
				errs = append(errs, e2)
				continue
			}
			switch decision.Action {
			case PinKeep:
				continue
			case PinRelease:
				if !validRelease(decision.Authorization, r) {
					errs = append(errs, ErrSnapshotLeaseConflict)
					continue
				}
				leaseLock, e3 := lockLeaseContext(ctx, path, false)
				if e3 != nil {
					errs = append(errs, e3)
					continue
				}
				if e2 = s.validateRoot(); e2 == nil {
					e2 = s.releaseLease(ctx, path, r, decision.Authorization)
				}
				unlockErr := errors.Join(unlockFile(leaseLock), leaseLock.Close())
				if e2 = errors.Join(e2, unlockErr); e2 != nil {
					errs = append(errs, e2)
					continue
				}
				if e2 = s.authority.CompletePinRelease(ctx, decision.Authorization); e2 != nil {
					errs = append(errs, e2)
				}
			default:
				errs = append(errs, ErrSnapshotLeaseConflict)
			}
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
				e2 = os.RemoveAll(path)
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
	for _, attempt := range attempts {
		if attempt.State == PinAttemptPending && attempt.LeaseID == current.ID {
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
	if err = os.RemoveAll(l.path); err != nil {
		return err
	}
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
	return fsyncDir(l.store.root)
}

func RestoreVerified(ctx context.Context, lease *VerifiedSnapshotLease, destination string) (retErr error) {
	if lease == nil {
		return ErrSnapshotLeaseInvalid
	}
	if err := lease.beginBorrow(ctx, true); err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, lease.endBorrow()) }()
	snapshot, cleanup, err := lease.prepareRestoreSnapshot(ctx)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, cleanup()) }()
	return restoreCore(ctx, snapshot, destination, lease.store.root)
}

func (l *VerifiedSnapshotLease) prepareRestoreSnapshot(ctx context.Context) (string, func() error, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	if l.record.ArtifactBytes > l.store.options.MaxRestoreScratchBytes {
		return "", nil, ErrSnapshotLeaseLimit
	}
	rootDev, rootIno, err := pathIdentity(l.path)
	if err != nil || rootDev != l.record.RootDevice || rootIno != l.record.RootInode {
		return "", nil, errors.Join(ErrSnapshotLeaseInvalid, err)
	}
	sourceRoot, err := os.OpenRoot(l.path)
	if err != nil {
		return "", nil, err
	}
	sourceInfo, err := sourceRoot.Stat(".")
	if err != nil {
		return "", nil, errors.Join(err, sourceRoot.Close())
	}
	d, i, err := fileIdentity(sourceInfo)
	if err != nil || d != l.record.RootDevice || i != l.record.RootInode {
		return "", nil, errors.Join(ErrSnapshotLeaseInvalid, err, sourceRoot.Close())
	}
	parent, err := os.OpenRoot(l.store.root)
	if err != nil {
		return "", nil, errors.Join(err, sourceRoot.Close())
	}
	name, err := createInspectionScratch(parent)
	if err != nil {
		return "", nil, errors.Join(err, parent.Close(), sourceRoot.Close())
	}
	created, err := parent.Stat(name)
	if err != nil {
		return "", nil, errors.Join(err, parent.Close(), sourceRoot.Close())
	}
	cleanup := func() error {
		var ce error
		if re := removeInspectionScratch(parent, name, created); re != nil {
			ce = errors.Join(ce, re)
		}
		return errors.Join(ce, parent.Close(), sourceRoot.Close())
	}
	for _, fr := range l.record.Files {
		if err = ctx.Err(); err != nil {
			return "", nil, errors.Join(err, cleanup())
		}
		if filepath.Base(fr.Name) != fr.Name {
			return "", nil, errors.Join(ErrSnapshotLeaseInvalid, cleanup())
		}
		if e := ctx.Err(); e != nil {
			return "", nil, errors.Join(e, cleanup())
		}
		in, e := openRegularFile(filepath.Join(l.path, fr.Name))
		if e != nil {
			return "", nil, errors.Join(e, cleanup())
		}
		info, e := in.Stat()
		if e != nil {
			return "", nil, errors.Join(e, in.Close(), cleanup())
		}
		dev, ino, e := fileIdentity(info)
		if e != nil || !info.Mode().IsRegular() || info.Size() != fr.Size || dev != fr.Device || ino != fr.Inode {
			return "", nil, errors.Join(ErrSnapshotLeaseInvalid, e, in.Close(), cleanup())
		}
		out, e := parent.OpenFile(filepath.Join(name, fr.Name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return "", nil, errors.Join(e, in.Close(), cleanup())
		}
		h := sha256.New()
		n, e := copyContext(ctx, io.MultiWriter(out, h), in, fr.Size)
		inClose := in.Close()
		if e == nil {
			e = out.Sync()
		}
		outClose := out.Close()
		if e != nil || inClose != nil || outClose != nil {
			return "", nil, errors.Join(e, inClose, outClose, cleanup())
		}
		if n != fr.Size || hex.EncodeToString(h.Sum(nil)) != fr.SHA256 {
			return "", nil, errors.Join(ErrSnapshotLeaseInvalid, cleanup())
		}
	}
	if err = syncTree(filepath.Join(l.store.root, name)); err != nil {
		return "", nil, errors.Join(err, cleanup())
	}
	if err = fsyncDir(l.store.root); err != nil {
		return "", nil, errors.Join(err, cleanup())
	}
	return filepath.Join(l.store.root, name), cleanup, nil
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
	artifacts, metadata int64
}

func (s *SnapshotLeaseStore) usageLocked() (leaseUsage, error) {
	var u leaseUsage
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
		}
	}
	journal := filepath.Join(s.root, ".releases")
	entries, err := os.ReadDir(journal)
	if err != nil {
		return u, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		marker := filepath.Join(journal, entry.Name())
		tombstone, e := readReleaseTombstone(marker)
		var metadataBytes int64
		if e == nil {
			metadataBytes = tombstone.MetadataBytes
		} else if errors.Is(e, os.ErrNotExist) {
			r, readErr := readLeaseRecord(marker, maxLeaseMetadataBytes)
			if readErr != nil {
				return u, readErr
			}
			if r.State == "RELEASING" {
				if _, statErr := os.Lstat(filepath.Join(s.root, r.ID)); errors.Is(statErr, os.ErrNotExist) {
					metadataBytes = r.MetadataBytes
				} else if statErr != nil {
					return u, statErr
				}
			}
		} else {
			return u, e
		}
		if metadataBytes > s.options.MaxMetadataBytesPerLease || u.metadata > math.MaxInt64-metadataBytes {
			return u, ErrSnapshotLeaseLimit
		}
		u.metadata += metadataBytes
	}
	return u, nil
}

func (s *SnapshotLeaseStore) lock(ctx context.Context) (func() error, error) {
	if isNilInterface(ctx) {
		return nil, ErrNilContext
	}
	if err := s.validateRoot(); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(s.root, ".store.lock")
	var f *os.File
	var err error
	for tries := 0; tries < 2; tries++ {
		before, statErr := os.Lstat(lockPath)
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			return nil, statErr
		}
		if statErr == nil && (!before.Mode().IsRegular() || before.Mode().Perm()&0077 != 0) {
			return nil, ErrSnapshotLeaseInvalid
		}
		f, err = openLockFile(lockPath, errors.Is(statErr, os.ErrNotExist))
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		break
	}
	if f == nil {
		return nil, ErrSnapshotLeaseConflict
	}
	opened, err := f.Stat()
	if err != nil {
		return nil, errors.Join(err, f.Close())
	}
	current, err := os.Lstat(lockPath)
	if err != nil || !os.SameFile(current, opened) || !current.Mode().IsRegular() || current.Mode().Perm()&0077 != 0 {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err, f.Close())
	}
	if err = lockFileContext(ctx, f, false); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	if err = s.validateRoot(); err != nil {
		return nil, errors.Join(err, unlockFile(f), f.Close())
	}
	current, err = os.Lstat(lockPath)
	if err != nil || !os.SameFile(current, opened) {
		return nil, errors.Join(ErrSnapshotLeaseInvalid, err, unlockFile(f), f.Close())
	}
	return func() error { return errors.Join(unlockFile(f), f.Close()) }, nil
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
	_, we := f.Write(raw)
	if we == nil {
		we = f.Sync()
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
	dir, e := root.Open(".")
	if e != nil {
		return e
	}
	e = dir.Sync()
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
	previousMetadata := r.MetadataBytes
	r.State = "RELEASING"
	r.Disposition = a.Disposition
	r.RecordVersion = a.RecordVersion
	candidate := r
	if _, err := encodeLeaseRecord(&candidate, s.options.MaxMetadataBytesPerLease); err != nil {
		return err
	}
	usage, err := s.usageLocked()
	if err != nil {
		return err
	}
	growth := candidate.MetadataBytes - previousMetadata
	if growth < 0 || usage.metadata > s.options.MaxRetainedMetadataBytes || growth > s.options.MaxRetainedMetadataBytes-usage.metadata {
		return ErrSnapshotLeaseLimit
	}
	if err := s.validateRoot(); err != nil {
		return err
	}
	if err := writeLeaseRecord(path, &r, s.options.MaxMetadataBytesPerLease); err != nil {
		return err
	}
	journal := filepath.Join(s.root, ".releases")
	marker := filepath.Join(journal, r.ID)
	if err := os.Mkdir(marker, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if existing, err := readLeaseRecord(marker, maxLeaseMetadataBytes); err == nil {
		if existing.ID != r.ID || existing.PinID != r.PinID || existing.PlanRef != r.PlanRef || existing.ManifestSHA256 != r.ManifestSHA256 || existing.Disposition != r.Disposition || existing.RecordVersion != r.RecordVersion || (existing.State != "RELEASING" && existing.State != "RELEASED") {
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
	if err := s.validateRoot(); err != nil {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	if err := fsyncDir(s.root); err != nil {
		return err
	}
	r.State = "RELEASED"
	if err := writeReleaseTombstone(marker, r); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(marker, leaseRecordName)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := fsyncDir(marker); err != nil {
		return err
	}
	return fsyncDir(journal)
}

func (s *SnapshotLeaseStore) finishReleaseJournal(ctx context.Context) error {
	journal := filepath.Join(s.root, ".releases")
	if err := privatefs.ValidateDirectory(ctx, journal); err != nil {
		return err
	}
	entries, err := os.ReadDir(journal)
	if err != nil {
		return err
	}
	var errs []error
	for _, entry := range entries {
		if err = ctx.Err(); err != nil {
			return errors.Join(errors.Join(errs...), err)
		}
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			errs = append(errs, ErrSnapshotLeaseInvalid)
			continue
		}
		marker := filepath.Join(journal, entry.Name())
		tombstone, tombstoneErr := readReleaseTombstone(marker)
		if tombstoneErr == nil {
			a := tombstone.authorization()
			if err = s.authority.CompletePinRelease(ctx, a); err != nil {
				errs = append(errs, err)
				continue
			}
			if err = os.RemoveAll(marker); err != nil {
				errs = append(errs, err)
				continue
			}
			if err = fsyncDir(journal); err != nil {
				errs = append(errs, err)
			}
			continue
		}
		if !errors.Is(tombstoneErr, os.ErrNotExist) {
			errs = append(errs, tombstoneErr)
			continue
		}
		r, e := readLeaseRecord(marker, maxLeaseMetadataBytes)
		if errors.Is(e, os.ErrNotExist) {
			live := filepath.Join(s.root, entry.Name())
			pending, readErr := readLeaseRecord(live, s.options.MaxMetadataBytesPerLease)
			if readErr == nil && pending.ID == entry.Name() && pending.State == "RELEASING" {
				lk, lockErr := lockLeaseContext(ctx, live, false)
				if lockErr == nil {
					auth := PinReleaseAuthorization{PinID: pending.PinID, PlanRef: pending.PlanRef, ManifestSHA256: pending.ManifestSHA256, Disposition: pending.Disposition, RecordVersion: pending.RecordVersion}
					lockErr = s.releaseLease(ctx, live, pending, auth)
					lockErr = errors.Join(lockErr, unlockFile(lk), lk.Close())
				}
				if lockErr != nil {
					errs = append(errs, lockErr)
				}
				continue
			}
			errs = append(errs, errors.Join(ErrSnapshotLeaseInvalid, e, readErr))
			continue
		}
		if e != nil {
			errs = append(errs, e)
			continue
		}
		if r.PinID == "" || !validPlanRef(r.PlanRef) || !isSHA256(r.ManifestSHA256) || r.Disposition == 0 || r.RecordVersion == 0 {
			errs = append(errs, ErrSnapshotLeaseInvalid)
			continue
		}
		a := PinReleaseAuthorization{PinID: r.PinID, PlanRef: r.PlanRef, ManifestSHA256: r.ManifestSHA256, Disposition: r.Disposition, RecordVersion: r.RecordVersion}
		if r.State == "RELEASING" {
			live := filepath.Join(s.root, r.ID)
			if _, e = os.Lstat(live); e == nil {
				lk, le := lockLeaseContext(ctx, live, false)
				if le != nil {
					errs = append(errs, le)
					continue
				}
				le = verifyRecordFiles(ctx, live, r)
				if le == nil {
					le = s.validateRoot()
				}
				if le == nil {
					le = os.RemoveAll(live)
				}
				le = errors.Join(le, unlockFile(lk), lk.Close())
				if le != nil {
					errs = append(errs, le)
					continue
				}
				if le = fsyncDir(s.root); le != nil {
					errs = append(errs, le)
					continue
				}
			} else if !errors.Is(e, os.ErrNotExist) {
				errs = append(errs, e)
				continue
			}
			r.State = "RELEASED"
			if e = writeReleaseTombstone(marker, r); e != nil {
				errs = append(errs, e)
				continue
			}
			if e = os.Remove(filepath.Join(marker, leaseRecordName)); e != nil && !errors.Is(e, os.ErrNotExist) {
				errs = append(errs, e)
				continue
			}
			if e = fsyncDir(marker); e != nil {
				errs = append(errs, e)
				continue
			}
			if e = fsyncDir(journal); e != nil {
				errs = append(errs, e)
				continue
			}
		} else if r.State != "RELEASED" {
			errs = append(errs, ErrSnapshotLeaseInvalid)
			continue
		}
		if e = s.authority.CompletePinRelease(ctx, a); e != nil {
			errs = append(errs, e)
			continue
		}
		if e = os.RemoveAll(marker); e != nil {
			errs = append(errs, e)
			continue
		}
		if e = fsyncDir(journal); e != nil {
			errs = append(errs, e)
		}
	}
	return errors.Join(errs...)
}

func syncLeaseFiles(path string, r leaseDiskRecord) error {
	for _, f := range r.Files {
		h, e := os.OpenFile(filepath.Join(path, f.Name), os.O_RDONLY, 0)
		if e != nil {
			return e
		}
		e = h.Sync()
		ce := h.Close()
		if e != nil || ce != nil {
			return errors.Join(e, ce)
		}
	}
	return nil
}
