package recovery

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/sirerun/serenity/internal/hosted/backup"
	"github.com/sirerun/serenity/internal/hosted/privatefs"
)

var (
	ErrPinOwnerInvalid        = errors.New("hosted/recovery: invalid snapshot pin owner input")
	ErrPinOwnerNotFound       = errors.New("hosted/recovery: snapshot pin owner record not found")
	ErrPinOwnerConflict       = errors.New("hosted/recovery: snapshot pin owner conflict")
	ErrPinOwnerLimit          = errors.New("hosted/recovery: snapshot pin owner bound exceeded")
	ErrPinOwnerCorrupt        = errors.New("hosted/recovery: corrupt snapshot pin owner history")
	ErrPinOwnerUnavailable    = errors.New("hosted/recovery: snapshot pin owner unavailable")
	ErrPinOwnerOutcomeUnknown = errors.New("hosted/recovery: snapshot pin owner outcome unknown")
)

const (
	pinOwnerWireVersion       = uint64(1)
	pinOwnerMaxRecordBytes    = 4096
	pinOwnerMaxSuperBytes     = 4096
	pinOwnerMaxRecords        = 256
	pinOwnerMaxAttempts       = 32
	pinOwnerMaxLiveAttempts   = 4096
	pinOwnerRecordReserve     = int64(pinOwnerMaxRecords * pinOwnerMaxRecordBytes)
	pinOwnerSuperName         = "superblock.json"
	pinOwnerLockName          = ".store.lock"
	pinOwnerReservationsName  = "reservations"
	pinOwnerHashDomain        = "serenity.recovery.snapshot-pin-owner.v1\x00"
	pinOwnerSuperHashDomain   = "serenity.recovery.snapshot-pin-owner-superblock.v1\x00"
	pinOwnerZeroDigest        = "0000000000000000000000000000000000000000000000000000000000000000"
	pinOwnerAttemptPending    = "PENDING"
	pinOwnerAttemptCommitted  = "COMMITTED"
	pinOwnerAttemptCanceled   = "CANCELED"
	pinOwnerEventReserve      = "RESERVE"
	pinOwnerEventBegin        = "PIN_BEGIN"
	pinOwnerEventCommit       = "PIN_COMMIT"
	pinOwnerEventCancel       = "PIN_CANCEL"
	pinOwnerEventAbandon      = "ABANDON"
	pinOwnerEventReleaseBegin = "RELEASE_BEGIN"
	pinOwnerEventReleaseDone  = "RELEASE_COMPLETE"
)

var pinOwnerOperationIDPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

type SnapshotPinOwnerOptions struct {
	OwnerRoot             string
	BackupLeaseRoot       string
	MaxPlans              int
	MaxOwnerMetadataBytes int64
}

type PinOwnerState string

const (
	PinOwnerReserved               PinOwnerState = "RESERVED"
	PinOwnerPinPending             PinOwnerState = "PIN_PENDING"
	PinOwnerPinned                 PinOwnerState = "PINNED"
	PinOwnerAbandonedBeforeEffects PinOwnerState = "ABANDONED_BEFORE_EFFECTS"
	PinOwnerReleasing              PinOwnerState = "RELEASING"
	PinOwnerReleased               PinOwnerState = "RELEASED"
)

type PinOwnerReservation struct {
	planRef            string
	operationID        string
	reservationVersion uint64
	recordVersion      uint64
	state              PinOwnerState
}

func (r PinOwnerReservation) PlanRef() string            { return r.planRef }
func (r PinOwnerReservation) OperationID() string        { return r.operationID }
func (r PinOwnerReservation) ReservationVersion() uint64 { return r.reservationVersion }
func (r PinOwnerReservation) RecordVersion() uint64      { return r.recordVersion }
func (r PinOwnerReservation) State() PinOwnerState       { return r.state }
func (r PinOwnerReservation) Valid() bool {
	return pinOwnerIsHex(r.planRef, 64) && pinOwnerOperationIDPattern.MatchString(r.operationID) && r.reservationVersion > 0 && r.recordVersion > 0 && pinOwnerValidState(r.state)
}

type PinAbandonReason string

const (
	PinAbandonOperatorRequested PinAbandonReason = "OPERATOR_ABORT_BEFORE_READY"
	PinAbandonSetupFailed       PinAbandonReason = "SETUP_FAILURE_BEFORE_READY"
)

type SnapshotPinOwner struct {
	options       SnapshotPinOwnerOptions
	expected      backup.SnapshotStoreIdentity
	mu            sync.Mutex
	ownerDevice   uint64
	ownerInode    uint64
	ownerLockDev  uint64
	ownerLockIno  uint64
	backupDevice  uint64
	backupInode   uint64
	backupLockDev uint64
	backupLockIno uint64
	storeID       string
	activated     bool
	closed        bool
}

// OpenSnapshotPinOwner creates or validates the owner root and its durable
// identity binding. Lifecycle methods remain disabled until the private
// factory gate runs after NewSnapshotLeaseStore has returned successfully.
func OpenSnapshotPinOwner(ctx context.Context, options SnapshotPinOwnerOptions, expected backup.SnapshotStoreIdentity) (owner *SnapshotPinOwner, retErr error) {
	if ctx == nil || expected == (backup.SnapshotStoreIdentity{}) || !pinOwnerValidOptions(options) {
		return nil, errors.Join(ErrPinOwnerInvalid, backup.ErrSnapshotLeaseInvalid)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := pinOwnerEnsureOwnerRoot(ctx, options.OwnerRoot); err != nil {
		return nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	if err := privatefs.ValidateDirectory(ctx, options.BackupLeaseRoot); err != nil {
		return nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	o := &SnapshotPinOwner{options: options, expected: expected}
	if err := pinOwnerCheckCapacity(o, 0); err != nil {
		return nil, err
	}
	ownerRoot, err := pinOwnerOpenDirectory(options.OwnerRoot)
	if err != nil {
		return nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	defer func() {
		retErr = errors.Join(retErr, ownerRoot.Close())
		if retErr != nil {
			owner = nil
		}
	}()
	lockPresent, err := pinOwnerValidateBootstrapPrefix(ctx, ownerRoot)
	if err != nil {
		return nil, err
	}
	ownerDev, ownerIno, err := pinOwnerFileIdentity(ownerRoot)
	if err != nil {
		return nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	ownerLock, err := pinOwnerOpenLockAt(ownerRoot, pinOwnerLockName, !lockPresent)
	if err != nil {
		return nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	defer func() {
		retErr = errors.Join(retErr, ownerLock.Close())
		if retErr != nil {
			owner = nil
		}
	}()
	if err = pinOwnerLockFile(ctx, ownerLock); err != nil {
		return nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	if err = ownerLock.Sync(); err != nil {
		return nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	if err = ownerRoot.Sync(); err != nil {
		return nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	defer func() {
		retErr = errors.Join(retErr, pinOwnerUnlockFile(ownerLock))
		if retErr != nil {
			owner = nil
		}
	}()
	lockDev, lockIno, err := pinOwnerFileIdentity(ownerLock)
	if err != nil {
		return nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	backupRoot, err := pinOwnerOpenDirectory(options.BackupLeaseRoot)
	if err != nil {
		return nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	defer func() {
		retErr = errors.Join(retErr, backupRoot.Close())
		if retErr != nil {
			owner = nil
		}
	}()
	backupDev, backupIno, err := pinOwnerFileIdentity(backupRoot)
	if err != nil {
		return nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	backupLock, err := pinOwnerOpenLockAt(backupRoot, pinOwnerLockName, false)
	if err != nil {
		return nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	defer func() {
		retErr = errors.Join(retErr, backupLock.Close())
		if retErr != nil {
			owner = nil
		}
	}()
	backupLockDev, backupLockIno, err := pinOwnerFileIdentity(backupLock)
	if err != nil {
		return nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	o.ownerDevice, o.ownerInode = ownerDev, ownerIno
	o.ownerLockDev, o.ownerLockIno = lockDev, lockIno
	o.backupDevice, o.backupInode = backupDev, backupIno
	o.backupLockDev, o.backupLockIno = backupLockDev, backupLockIno
	if err = pinOwnerValidateNamedIdentity(ctx, o); err != nil {
		return nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	if _, err = pinOwnerValidateBootstrapPrefix(ctx, ownerRoot); err != nil {
		return nil, err
	}
	if err = pinOwnerEnsureDirectoryAt(ctx, ownerRoot, pinOwnerReservationsName); err != nil {
		return nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	if err = pinOwnerValidateNamedIdentity(ctx, o); err != nil {
		return nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	block, err := pinOwnerReadSuperblock(ctx, ownerRoot)
	if errors.Is(err, os.ErrNotExist) {
		if _, err = pinOwnerValidateBootstrapPrefix(ctx, ownerRoot); err != nil {
			return nil, err
		}
		storeID, idErr := pinOwnerRandomHex(32)
		if idErr != nil {
			return nil, idErr
		}
		block = pinOwnerSuperblock{Version: pinOwnerWireVersion, StoreID: storeID, OwnerRootDevice: ownerDev, OwnerRootInode: ownerIno, OwnerLockDevice: lockDev, OwnerLockInode: lockIno, BackupRootDevice: backupDev, BackupRootInode: backupIno, BackupLockDevice: backupLockDev, BackupLockInode: backupLockIno}
		if err = pinOwnerValidateNamedIdentity(ctx, o); err != nil {
			return nil, errors.Join(ErrPinOwnerUnavailable, err)
		}
		if err = pinOwnerWriteSuperblock(ctx, ownerRoot, block); err != nil {
			return nil, errors.Join(ErrPinOwnerOutcomeUnknown, err)
		}
		if err = pinOwnerValidateNamedIdentity(ctx, o); err != nil {
			return nil, errors.Join(ErrPinOwnerOutcomeUnknown, err)
		}
	} else if err != nil {
		return nil, errors.Join(ErrPinOwnerCorrupt, err)
	} else if block.OwnerRootDevice != ownerDev || block.OwnerRootInode != ownerIno || block.OwnerLockDevice != lockDev || block.OwnerLockInode != lockIno || block.BackupRootDevice != backupDev || block.BackupRootInode != backupIno || block.BackupLockDevice != backupLockDev || block.BackupLockInode != backupLockIno {
		return nil, ErrPinOwnerUnavailable
	}
	o.storeID = block.StoreID
	if err = pinOwnerValidateRootPair(ctx, o, ownerRoot, ownerLock, backupRoot, backupLock); err != nil {
		return nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	if _, err = pinOwnerLoadAll(ctx, ownerRoot, o); err != nil {
		return nil, err
	}
	return o, nil
}

// pinOwnerOpenVerifiedPair is the only activation path. It creates the exact
// opaque identity from the supplied complete backup options, opens the owner,
// invokes the real four-argument backup constructor, and only then enables the
// lifecycle authority after a final root/lock identity recheck.
func pinOwnerOpenVerifiedPair(ctx context.Context, ownerOptions SnapshotPinOwnerOptions, backupOptions backup.SnapshotLeaseStoreOptions) (*SnapshotPinOwner, *backup.SnapshotLeaseStore, error) {
	if ownerOptions.BackupLeaseRoot != backupOptions.LeaseRoot {
		return nil, nil, ErrPinOwnerInvalid
	}
	expected, err := backup.PreflightSnapshotStoreIdentity(ctx, backupOptions)
	if err != nil {
		return nil, nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	o, err := OpenSnapshotPinOwner(ctx, ownerOptions, expected)
	if err != nil {
		return nil, nil, err
	}
	store, err := backup.NewSnapshotLeaseStore(ctx, backupOptions, o, expected)
	if err != nil {
		return nil, nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	if err = o.pinOwnerFinalizeFactoryGate(ctx, expected); err != nil {
		return nil, nil, err
	}
	return o, store, nil
}

func (o *SnapshotPinOwner) pinOwnerFinalizeFactoryGate(ctx context.Context, expected backup.SnapshotStoreIdentity) (retErr error) {
	if o == nil || expected == (backup.SnapshotStoreIdentity{}) || expected != o.expected {
		return ErrPinOwnerUnavailable
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return ErrPinOwnerUnavailable
	}
	ownerRoot, ownerLock, backupRoot, backupLock, release, err := pinOwnerAcquire(ctx, o)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, release()) }()
	if err = pinOwnerValidateRootPair(ctx, o, ownerRoot, ownerLock, backupRoot, backupLock); err != nil {
		return errors.Join(ErrPinOwnerUnavailable, err)
	}
	o.activated = true
	return nil
}

func (o *SnapshotPinOwner) ReservePinPlan(ctx context.Context, operationID string) (PinOwnerReservation, error) {
	if !pinOwnerOperationIDPattern.MatchString(operationID) {
		return PinOwnerReservation{}, ErrPinOwnerInvalid
	}
	return pinOwnerWithLock(o, ctx, func(root *os.File) (out PinOwnerReservation, retErr error) {
		plans, err := pinOwnerLoadAll(ctx, root, o)
		if err != nil {
			return PinOwnerReservation{}, err
		}
		for _, plan := range plans {
			if plan.head.OperationID == operationID {
				return pinOwnerReservationFromRecord(plan.head), nil
			}
		}
		if len(plans) >= o.options.MaxPlans {
			return PinOwnerReservation{}, errors.Join(ErrPinOwnerLimit, backup.ErrSnapshotLeaseLimit)
		}
		if err = pinOwnerCheckCapacity(o, len(plans)+1); err != nil {
			return PinOwnerReservation{}, err
		}
		planRef, err := pinOwnerRandomHex(64)
		if err != nil {
			return PinOwnerReservation{}, err
		}
		for _, plan := range plans {
			if plan.head.PlanRef == planRef {
				return PinOwnerReservation{}, ErrPinOwnerOutcomeUnknown
			}
		}
		reservations, err := pinOwnerOpenDirAt(root, pinOwnerReservationsName)
		if err != nil {
			return PinOwnerReservation{}, errors.Join(ErrPinOwnerUnavailable, err)
		}
		defer func() {
			retErr = errors.Join(retErr, reservations.Close())
			if retErr != nil {
				out = PinOwnerReservation{}
			}
		}()
		if err = pinOwnerValidateNamedIdentity(ctx, o); err != nil {
			return PinOwnerReservation{}, errors.Join(ErrPinOwnerUnavailable, err)
		}
		if err = pinOwnerMkdirAt(reservations, planRef, 0700); err != nil {
			return PinOwnerReservation{}, errors.Join(ErrPinOwnerOutcomeUnknown, err)
		}
		if err = reservations.Sync(); err != nil {
			return PinOwnerReservation{}, errors.Join(ErrPinOwnerOutcomeUnknown, err)
		}
		if err = pinOwnerValidateNamedIdentity(ctx, o); err != nil {
			return PinOwnerReservation{}, errors.Join(ErrPinOwnerOutcomeUnknown, err)
		}
		planDir, err := pinOwnerOpenDirAt(reservations, planRef)
		if err != nil {
			return PinOwnerReservation{}, errors.Join(ErrPinOwnerOutcomeUnknown, err)
		}
		defer func() {
			retErr = errors.Join(retErr, planDir.Close())
			if retErr != nil {
				out = PinOwnerReservation{}
			}
		}()
		r := pinOwnerRecord{Version: pinOwnerWireVersion, StoreID: o.storeID, PlanRef: planRef, ReservationVersion: 1, RecordVersion: 1, PreviousSHA256: pinOwnerZeroDigest, OperationID: operationID, Event: pinOwnerEventReserve, State: string(PinOwnerReserved)}
		if err = pinOwnerAppend(ctx, root, o, planDir, r); err != nil {
			return PinOwnerReservation{}, err
		}
		return pinOwnerReservationFromRecord(r), nil
	})
}

func (o *SnapshotPinOwner) BeginPinAttempt(ctx context.Context, planRef string, reservationVersion uint64, leaseID, manifestSHA256 string) (backup.PinAttemptRef, error) {
	if !pinOwnerIsHex(planRef, 64) || reservationVersion == 0 || !pinOwnerIsHex(leaseID, 64) || !pinOwnerIsHex(manifestSHA256, 64) {
		return backup.PinAttemptRef{}, errors.Join(ErrPinOwnerInvalid, backup.ErrSnapshotLeaseInvalid)
	}
	return pinOwnerWithLock(o, ctx, func(root *os.File) (out backup.PinAttemptRef, retErr error) {
		plan, dir, err := pinOwnerFindPlan(ctx, root, o, planRef)
		if err != nil {
			return backup.PinAttemptRef{}, err
		}
		defer func() {
			retErr = errors.Join(retErr, dir.Close())
			if retErr != nil {
				out = backup.PinAttemptRef{}
			}
		}()
		head := plan.head
		if head.ReservationVersion != reservationVersion {
			return backup.PinAttemptRef{}, pinOwnerConflict()
		}
		if head.State == string(PinOwnerPinPending) && head.AttemptState == pinOwnerAttemptPending && pinOwnerAttemptMatches(head, planRef, reservationVersion, leaseID, manifestSHA256, head.AttemptVersion) {
			return pinOwnerAttemptRef(head), nil
		}
		if head.State != string(PinOwnerReserved) || (head.AttemptState != "" && head.AttemptState != pinOwnerAttemptCanceled) {
			return backup.PinAttemptRef{}, pinOwnerConflict()
		}
		if head.AttemptHighWater >= pinOwnerMaxAttempts {
			return backup.PinAttemptRef{}, errors.Join(ErrPinOwnerLimit, backup.ErrSnapshotLeaseLimit)
		}
		r := head
		r.RecordVersion++
		r.PreviousSHA256 = plan.hash
		r.Event = pinOwnerEventBegin
		r.State = string(PinOwnerPinPending)
		r.AttemptHighWater++
		r.AttemptVersion = r.AttemptHighWater
		r.LeaseID = leaseID
		r.ManifestSHA256 = manifestSHA256
		r.AttemptState = pinOwnerAttemptPending
		r.PinID, r.ReleaseDisposition, r.ReasonCode = "", "", ""
		r.ReleaseRecordVersion = 0
		if err = pinOwnerAppend(ctx, root, o, dir, r); err != nil {
			return backup.PinAttemptRef{}, err
		}
		return pinOwnerAttemptRef(r), nil
	})
}

func (o *SnapshotPinOwner) CommitPinAttempt(ctx context.Context, attempt backup.PinAttemptRef, pin backup.SnapshotPinRef) error {
	if err := pinOwnerValidateAttemptInput(attempt, backup.PinAttemptPending); err != nil {
		return errors.Join(err, backup.ErrSnapshotLeaseInvalid)
	}
	return pinOwnerWithLockError(o, ctx, func(root *os.File) (retErr error) {
		plan, dir, err := pinOwnerFindPlan(ctx, root, o, attempt.PlanRef)
		if err != nil {
			return err
		}
		defer func() { retErr = errors.Join(retErr, dir.Close()) }()
		head := plan.head
		if head.State == string(PinOwnerPinned) && head.AttemptState == pinOwnerAttemptCommitted && pinOwnerAttemptMatches(head, attempt.PlanRef, attempt.ReservationVersion, attempt.LeaseID, attempt.ManifestSHA256, attempt.AttemptVersion) {
			return pinOwnerMatchPin(head, pin)
		}
		if head.State != string(PinOwnerPinPending) || head.AttemptState != pinOwnerAttemptPending || !pinOwnerAttemptMatches(head, attempt.PlanRef, attempt.ReservationVersion, attempt.LeaseID, attempt.ManifestSHA256, attempt.AttemptVersion) {
			return pinOwnerConflict()
		}
		if err = pinOwnerValidatePin(head, pin); err != nil {
			return err
		}
		r := head
		r.RecordVersion++
		r.PreviousSHA256 = plan.hash
		r.Event = pinOwnerEventCommit
		r.State = string(PinOwnerPinned)
		r.AttemptState = pinOwnerAttemptCommitted
		r.PinID = pin.ID()
		if err = pinOwnerAppend(ctx, root, o, dir, r); err != nil {
			return err
		}
		return nil
	})
}

func (o *SnapshotPinOwner) CancelPinAttempt(ctx context.Context, attempt backup.PinAttemptRef, proof backup.VerifiedPinAbsence) error {
	if err := pinOwnerValidateAttemptInput(attempt, backup.PinAttemptPending); err != nil {
		return errors.Join(err, backup.ErrSnapshotLeaseInvalid)
	}
	return pinOwnerWithLockError(o, ctx, func(root *os.File) (retErr error) {
		plan, dir, err := pinOwnerFindPlan(ctx, root, o, attempt.PlanRef)
		if err != nil {
			return err
		}
		defer func() { retErr = errors.Join(retErr, dir.Close()) }()
		foundCanceled := false
		for _, prior := range plan.records {
			if prior.Event == pinOwnerEventCancel && pinOwnerAttemptMatches(prior, attempt.PlanRef, attempt.ReservationVersion, attempt.LeaseID, attempt.ManifestSHA256, attempt.AttemptVersion) {
				foundCanceled = true
			}
		}
		if foundCanceled {
			if err = proof.Consume(attempt, o.expected); err != nil {
				return errors.Join(ErrPinOwnerConflict, err)
			}
			return nil
		}
		head := plan.head
		if head.State != string(PinOwnerPinPending) || head.AttemptState != pinOwnerAttemptPending || !pinOwnerAttemptMatches(head, attempt.PlanRef, attempt.ReservationVersion, attempt.LeaseID, attempt.ManifestSHA256, attempt.AttemptVersion) {
			return pinOwnerConflict()
		}
		if err = proof.Consume(attempt, o.expected); err != nil {
			return errors.Join(ErrPinOwnerConflict, err)
		}
		pinOwnerCrashInject("after-proof-consume")
		r := head
		r.RecordVersion++
		r.PreviousSHA256 = plan.hash
		r.Event = pinOwnerEventCancel
		r.State = string(PinOwnerReserved)
		r.AttemptState = pinOwnerAttemptCanceled
		r.PinID = ""
		if err = pinOwnerAppend(ctx, root, o, dir, r); err != nil {
			return err
		}
		return nil
	})
}

func (o *SnapshotPinOwner) FindPinAttempt(ctx context.Context, planRef, manifestSHA256 string) (backup.PinAttemptRef, error) {
	if !pinOwnerIsHex(planRef, 64) || !pinOwnerIsHex(manifestSHA256, 64) {
		return backup.PinAttemptRef{}, errors.Join(ErrPinOwnerInvalid, backup.ErrSnapshotLeaseInvalid)
	}
	return pinOwnerWithLock(o, ctx, func(root *os.File) (out backup.PinAttemptRef, retErr error) {
		plan, dir, err := pinOwnerFindPlan(ctx, root, o, planRef)
		if err != nil {
			return backup.PinAttemptRef{}, err
		}
		defer func() {
			retErr = errors.Join(retErr, dir.Close())
			if retErr != nil {
				out = backup.PinAttemptRef{}
			}
		}()
		if plan.head.ManifestSHA256 != manifestSHA256 || plan.head.AttemptVersion == 0 {
			return backup.PinAttemptRef{}, pinOwnerNotFound()
		}
		if plan.head.AttemptState == pinOwnerAttemptCanceled {
			return backup.PinAttemptRef{}, pinOwnerConflict()
		}
		return pinOwnerAttemptRef(plan.head), nil
	})
}

func (o *SnapshotPinOwner) ListPinAttempts(ctx context.Context) ([]backup.PinAttemptRef, error) {
	return pinOwnerWithLock(o, ctx, func(root *os.File) ([]backup.PinAttemptRef, error) {
		plans, err := pinOwnerLoadAll(ctx, root, o)
		if err != nil {
			return nil, err
		}
		out := make([]backup.PinAttemptRef, 0, len(plans))
		for _, plan := range plans {
			r := plan.head
			if r.AttemptState == pinOwnerAttemptPending || r.AttemptState == pinOwnerAttemptCommitted {
				out = append(out, pinOwnerAttemptRef(r))
			}
		}
		if len(out) > pinOwnerMaxLiveAttempts {
			return nil, errors.Join(ErrPinOwnerLimit, backup.ErrSnapshotLeaseLimit)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].PlanRef != out[j].PlanRef {
				return out[i].PlanRef < out[j].PlanRef
			}
			if out[i].ReservationVersion != out[j].ReservationVersion {
				return out[i].ReservationVersion < out[j].ReservationVersion
			}
			if out[i].AttemptVersion != out[j].AttemptVersion {
				return out[i].AttemptVersion < out[j].AttemptVersion
			}
			return out[i].LeaseID < out[j].LeaseID
		})
		return out, nil
	})
}

func (o *SnapshotPinOwner) ReconcilePin(ctx context.Context, pin backup.SnapshotPinRef) (backup.PinReconcileDecision, error) {
	if !pinOwnerIsHex(pin.ID(), 64) || !pinOwnerIsHex(pin.PlanRef(), 64) || !pinOwnerIsHex(pin.ManifestSHA256(), 64) || pin.ReservationVersion() == 0 {
		return backup.PinReconcileDecision{}, errors.Join(ErrPinOwnerInvalid, backup.ErrSnapshotLeaseInvalid)
	}
	return pinOwnerWithLock(o, ctx, func(root *os.File) (out backup.PinReconcileDecision, retErr error) {
		plan, dir, err := pinOwnerFindPlan(ctx, root, o, pin.PlanRef())
		if err != nil {
			return backup.PinReconcileDecision{}, err
		}
		defer func() {
			retErr = errors.Join(retErr, dir.Close())
			if retErr != nil {
				out = backup.PinReconcileDecision{}
			}
		}()
		head := plan.head
		if head.ReservationVersion != pin.ReservationVersion() {
			return backup.PinReconcileDecision{}, pinOwnerConflict()
		}
		switch PinOwnerState(head.State) {
		case PinOwnerReserved:
			// No pin is bound to a RESERVED row. A caller-supplied pin ref is
			// stale or unbound, so it cannot be acknowledged as an exact pin.
			return backup.PinReconcileDecision{}, pinOwnerConflict()
		case PinOwnerPinPending:
			// PIN_PENDING has an exact attempt but no owner-recorded PinID yet.
			// Do not bless an unverifiable producer PinID as an exact tuple.
			return backup.PinReconcileDecision{}, pinOwnerConflict()
		case PinOwnerPinned:
			if head.ManifestSHA256 != pin.ManifestSHA256() || head.PinID != pin.ID() || head.AttemptState != pinOwnerAttemptCommitted {
				return backup.PinReconcileDecision{}, pinOwnerConflict()
			}
			return backup.PinReconcileDecision{Action: backup.PinKeep}, nil
		case PinOwnerAbandonedBeforeEffects:
			if head.ManifestSHA256 != pin.ManifestSHA256() || head.PinID != pin.ID() || head.AttemptState != pinOwnerAttemptCommitted {
				return backup.PinReconcileDecision{}, pinOwnerConflict()
			}
			r := head
			r.RecordVersion++
			r.PreviousSHA256 = plan.hash
			r.Event = pinOwnerEventReleaseBegin
			r.State = string(PinOwnerReleasing)
			r.ReleaseDisposition = "ABANDONED_BEFORE_EFFECTS"
			r.ReleaseRecordVersion = r.RecordVersion
			r.ReasonCode = ""
			if err = pinOwnerAppend(ctx, root, o, dir, r); err != nil {
				return backup.PinReconcileDecision{}, err
			}
			return backup.PinReconcileDecision{Action: backup.PinRelease, Authorization: pinOwnerAuthorization(r)}, nil
		case PinOwnerReleasing:
			if head.ManifestSHA256 != pin.ManifestSHA256() || head.PinID != pin.ID() || head.AttemptState != pinOwnerAttemptCommitted {
				return backup.PinReconcileDecision{}, pinOwnerConflict()
			}
			return backup.PinReconcileDecision{Action: backup.PinRelease, Authorization: pinOwnerAuthorization(head)}, nil
		case PinOwnerReleased:
			if head.ManifestSHA256 != pin.ManifestSHA256() || head.PinID != pin.ID() || head.AttemptState != pinOwnerAttemptCommitted {
				return backup.PinReconcileDecision{}, pinOwnerConflict()
			}
			return backup.PinReconcileDecision{Action: backup.PinKeep}, nil
		default:
			return backup.PinReconcileDecision{Action: backup.PinKeep}, nil
		}
	})
}

func (o *SnapshotPinOwner) CompletePinRelease(ctx context.Context, authorization backup.PinReleaseAuthorization) error {
	if !pinOwnerIsHex(authorization.PinID, 64) || !pinOwnerIsHex(authorization.PlanRef, 64) || !pinOwnerIsHex(authorization.ManifestSHA256, 64) || authorization.Disposition != backup.PinAbandonedBeforeEffects || authorization.RecordVersion == 0 {
		return errors.Join(ErrPinOwnerInvalid, backup.ErrSnapshotLeaseInvalid)
	}
	return pinOwnerWithLockError(o, ctx, func(root *os.File) (retErr error) {
		plan, dir, err := pinOwnerFindPlan(ctx, root, o, authorization.PlanRef)
		if err != nil {
			return err
		}
		defer func() { retErr = errors.Join(retErr, dir.Close()) }()
		head := plan.head
		if (head.State == string(PinOwnerReleased) || head.State == string(PinOwnerReleasing)) && pinOwnerAuthorizationMatches(head, authorization) {
			if head.State == string(PinOwnerReleased) {
				return nil
			}
			if err = pinOwnerVerifyBackupRelease(ctx, o, head); err != nil {
				return err
			}
			r := head
			r.RecordVersion++
			r.PreviousSHA256 = plan.hash
			r.Event = pinOwnerEventReleaseDone
			r.State = string(PinOwnerReleased)
			if err = pinOwnerAppend(ctx, root, o, dir, r); err != nil {
				return err
			}
			return nil
		}
		return pinOwnerConflict()
	})
}

func (o *SnapshotPinOwner) MarkAbandonedBeforeEffects(ctx context.Context, exact PinOwnerReservation, reason PinAbandonReason) (PinOwnerReservation, error) {
	if !exact.Valid() || (reason != PinAbandonOperatorRequested && reason != PinAbandonSetupFailed) {
		return PinOwnerReservation{}, ErrPinOwnerInvalid
	}
	return pinOwnerWithLock(o, ctx, func(root *os.File) (out PinOwnerReservation, retErr error) {
		plan, dir, err := pinOwnerFindPlan(ctx, root, o, exact.planRef)
		if err != nil {
			return PinOwnerReservation{}, err
		}
		defer func() {
			retErr = errors.Join(retErr, dir.Close())
			if retErr != nil {
				out = PinOwnerReservation{}
			}
		}()
		head := plan.head
		if !pinOwnerExactReservation(head, exact) {
			if head.State == string(PinOwnerAbandonedBeforeEffects) && head.ReasonCode == string(reason) && len(plan.records) >= 2 {
				previous := plan.records[len(plan.records)-2]
				if pinOwnerExactReservation(previous, exact) {
					return pinOwnerReservationFromRecord(head), nil
				}
			}
			return PinOwnerReservation{}, pinOwnerConflict()
		}
		if head.State == string(PinOwnerAbandonedBeforeEffects) && head.ReasonCode == string(reason) {
			return pinOwnerReservationFromRecord(head), nil
		}
		if (head.State != string(PinOwnerReserved) && head.State != string(PinOwnerPinned)) || head.AttemptState == pinOwnerAttemptPending || head.State == string(PinOwnerPinned) && head.AttemptState != pinOwnerAttemptCommitted {
			return PinOwnerReservation{}, pinOwnerConflict()
		}
		r := head
		r.RecordVersion++
		r.PreviousSHA256 = plan.hash
		r.Event = pinOwnerEventAbandon
		r.State = string(PinOwnerAbandonedBeforeEffects)
		r.ReasonCode = string(reason)
		if err = pinOwnerAppend(ctx, root, o, dir, r); err != nil {
			return PinOwnerReservation{}, err
		}
		return pinOwnerReservationFromRecord(r), nil
	})
}

func pinOwnerWithLock[T any](o *SnapshotPinOwner, ctx context.Context, fn func(*os.File) (T, error)) (result T, retErr error) {
	var zero T
	if o == nil || ctx == nil {
		return zero, ErrPinOwnerInvalid
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed || !o.activated {
		return zero, ErrPinOwnerUnavailable
	}
	root, _, _, _, release, err := pinOwnerAcquire(ctx, o)
	if err != nil {
		return zero, err
	}
	defer func() {
		retErr = errors.Join(retErr, release())
		if retErr != nil {
			var failed T
			result = failed
		}
	}()
	if retErr = pinOwnerValidateSuperblock(ctx, root, o); retErr != nil {
		return zero, retErr
	}
	result, retErr = fn(root)
	return result, retErr
}

func pinOwnerWithLockError(o *SnapshotPinOwner, ctx context.Context, fn func(*os.File) error) error {
	_, err := pinOwnerWithLock(o, ctx, func(root *os.File) (struct{}, error) { return struct{}{}, fn(root) })
	return err
}

func pinOwnerAcquire(ctx context.Context, o *SnapshotPinOwner) (ownerRoot, ownerLock, backupRoot, backupLock *os.File, release func() error, retErr error) {
	locked := false
	succeeded := false
	defer func() {
		if succeeded {
			return
		}
		if locked {
			retErr = errors.Join(retErr, pinOwnerUnlockFile(ownerLock))
		}
		retErr = errors.Join(retErr, backupLockClose(backupLock), backupRootClose(backupRoot))
		if ownerLock != nil {
			retErr = errors.Join(retErr, ownerLock.Close())
		}
		if ownerRoot != nil {
			retErr = errors.Join(retErr, ownerRoot.Close())
		}
		ownerRoot, ownerLock, backupRoot, backupLock, release = nil, nil, nil, nil, nil
	}()
	if ctx == nil {
		return nil, nil, nil, nil, nil, ErrPinOwnerInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, nil, nil, err
	}
	var err error
	ownerRoot, err = pinOwnerOpenDirectory(o.options.OwnerRoot)
	if err != nil {
		return nil, nil, nil, nil, nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	dev, ino, err := pinOwnerFileIdentity(ownerRoot)
	if err != nil || dev != o.ownerDevice || ino != o.ownerInode {
		return nil, nil, nil, nil, nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	ownerLock, err = pinOwnerOpenLockAt(ownerRoot, pinOwnerLockName, false)
	if err != nil {
		return nil, nil, nil, nil, nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	ld, li, err := pinOwnerFileIdentity(ownerLock)
	if err != nil || ld != o.ownerLockDev || li != o.ownerLockIno {
		return nil, nil, nil, nil, nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	if err = pinOwnerLockFile(ctx, ownerLock); err != nil {
		return nil, nil, nil, nil, nil, err
	}
	locked = true
	if err = pinOwnerValidateNamedIdentity(ctx, o); err != nil {
		return nil, nil, nil, nil, nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	backupRoot, err = pinOwnerOpenDirectory(o.options.BackupLeaseRoot)
	if err != nil {
		return nil, nil, nil, nil, nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	backupLock, err = pinOwnerOpenLockAt(backupRoot, pinOwnerLockName, false)
	if err != nil {
		return nil, nil, nil, nil, nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	if err = pinOwnerValidateRootPair(ctx, o, ownerRoot, ownerLock, backupRoot, backupLock); err != nil {
		return nil, nil, nil, nil, nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	release = func() error {
		locked = false
		return errors.Join(pinOwnerUnlockFile(ownerLock), ownerLock.Close(), ownerRoot.Close(), backupRootClose(backupRoot), backupLockClose(backupLock))
	}
	succeeded = true
	return ownerRoot, ownerLock, backupRoot, backupLock, release, nil
}

func backupRootClose(f *os.File) error {
	if f == nil {
		return nil
	}
	return f.Close()
}
func backupLockClose(f *os.File) error {
	if f == nil {
		return nil
	}
	return f.Close()
}

func pinOwnerValidateRootPair(ctx context.Context, o *SnapshotPinOwner, ownerRoot, ownerLock, backupRoot, backupLock *os.File) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	pairs := []struct {
		file     *os.File
		dev, ino uint64
	}{{ownerRoot, o.ownerDevice, o.ownerInode}, {ownerLock, o.ownerLockDev, o.ownerLockIno}, {backupRoot, o.backupDevice, o.backupInode}, {backupLock, o.backupLockDev, o.backupLockIno}}
	for _, p := range pairs {
		d, i, e := pinOwnerFileIdentity(p.file)
		if e != nil || d != p.dev || i != p.ino {
			return errors.Join(ErrPinOwnerUnavailable, e)
		}
	}
	return pinOwnerValidateNamedIdentity(ctx, o)
}

func pinOwnerValidateNamedIdentity(ctx context.Context, o *SnapshotPinOwner) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, v := range []struct {
		path                       string
		dev, ino, lockDev, lockIno uint64
	}{
		{o.options.OwnerRoot, o.ownerDevice, o.ownerInode, o.ownerLockDev, o.ownerLockIno},
		{o.options.BackupLeaseRoot, o.backupDevice, o.backupInode, o.backupLockDev, o.backupLockIno},
	} {
		if err := privatefs.ValidateDirectory(ctx, v.path); err != nil {
			return err
		}
		root, err := pinOwnerOpenDirectory(v.path)
		if err != nil {
			return err
		}
		d, i, err := pinOwnerFileIdentity(root)
		if err != nil || d != v.dev || i != v.ino {
			closeErr := root.Close()
			return errors.Join(ErrPinOwnerUnavailable, err, closeErr)
		}
		lock, err := pinOwnerOpenLockAt(root, pinOwnerLockName, false)
		if err != nil {
			closeErr := root.Close()
			return errors.Join(err, closeErr)
		}
		ld, li, err := pinOwnerFileIdentity(lock)
		if err == nil && (ld != v.lockDev || li != v.lockIno) {
			err = ErrPinOwnerUnavailable
		}
		closeErr := errors.Join(lock.Close(), root.Close())
		if err != nil || closeErr != nil {
			return errors.Join(ErrPinOwnerUnavailable, err, closeErr)
		}
	}
	return ctx.Err()
}

func pinOwnerValidOptions(o SnapshotPinOwnerOptions) bool {
	return filepath.IsAbs(o.OwnerRoot) && filepath.Clean(o.OwnerRoot) == o.OwnerRoot && filepath.IsAbs(o.BackupLeaseRoot) && filepath.Clean(o.BackupLeaseRoot) == o.BackupLeaseRoot && o.OwnerRoot != o.BackupLeaseRoot && o.MaxPlans >= 1 && o.MaxPlans <= 256 && o.MaxOwnerMetadataBytes >= 1 && o.MaxOwnerMetadataBytes <= 256<<20
}

func pinOwnerEnsureOwnerRoot(ctx context.Context, path string) error {
	if err := privatefs.ValidateDirectory(ctx, filepath.Dir(path)); err != nil {
		return err
	}
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		if err = os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		parent, e := pinOwnerOpenDirectory(filepath.Dir(path))
		if e != nil {
			return e
		}
		e = errors.Join(parent.Sync(), parent.Close())
		if e != nil {
			return e
		}
	} else if err != nil {
		return err
	}
	return privatefs.ValidateDirectory(ctx, path)
}

func pinOwnerCheckCapacity(o *SnapshotPinOwner, plans int) error {
	if plans < 0 || int64(plans) > (1<<63-1)/pinOwnerRecordReserve {
		return ErrPinOwnerLimit
	}
	need := int64(plans)*pinOwnerRecordReserve + pinOwnerMaxSuperBytes
	if need > o.options.MaxOwnerMetadataBytes {
		return errors.Join(ErrPinOwnerLimit, backup.ErrSnapshotLeaseLimit)
	}
	return nil
}

func pinOwnerValidateAttemptInput(a backup.PinAttemptRef, state backup.PinAttemptState) error {
	if !pinOwnerIsHex(a.PlanRef, 64) || !pinOwnerIsHex(a.LeaseID, 64) || !pinOwnerIsHex(a.ManifestSHA256, 64) || a.ReservationVersion == 0 || a.AttemptVersion == 0 || a.State != state {
		return ErrPinOwnerInvalid
	}
	return nil
}
func pinOwnerIsHex(v string, n int) bool {
	if len(v) != n {
		return false
	}
	for _, c := range v {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
func pinOwnerValidState(s PinOwnerState) bool {
	switch s {
	case PinOwnerReserved, PinOwnerPinPending, PinOwnerPinned, PinOwnerAbandonedBeforeEffects, PinOwnerReleasing, PinOwnerReleased:
		return true
	}
	return false
}

var pinOwnerTestCrashHook func(string)

func pinOwnerCrashInject(point string) {
	if pinOwnerTestCrashHook != nil {
		pinOwnerTestCrashHook(point)
	}
}

func pinOwnerRandomHex(n int) (string, error) {
	b := make([]byte, n/2)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func pinOwnerConflict() error {
	return errors.Join(ErrPinOwnerConflict, backup.ErrSnapshotLeaseConflict)
}
func pinOwnerNotFound() error {
	return errors.Join(ErrPinOwnerNotFound, backup.ErrSnapshotLeaseNotFound)
}
func pinOwnerReservationFromRecord(r pinOwnerRecord) PinOwnerReservation {
	return PinOwnerReservation{planRef: r.PlanRef, operationID: r.OperationID, reservationVersion: r.ReservationVersion, recordVersion: r.RecordVersion, state: PinOwnerState(r.State)}
}
func pinOwnerAttemptMatches(r pinOwnerRecord, plan string, res uint64, lease, digest string, attempt uint64) bool {
	return r.PlanRef == plan && r.ReservationVersion == res && r.LeaseID == lease && r.ManifestSHA256 == digest && r.AttemptVersion == attempt
}
func pinOwnerAttemptRef(r pinOwnerRecord) backup.PinAttemptRef {
	state := backup.PinAttemptPending
	if r.AttemptState == pinOwnerAttemptCommitted {
		state = backup.PinAttemptCommitted
	}
	return backup.PinAttemptRef{PlanRef: r.PlanRef, LeaseID: r.LeaseID, ManifestSHA256: r.ManifestSHA256, ReservationVersion: r.ReservationVersion, AttemptVersion: r.AttemptVersion, State: state}
}
func pinOwnerExactReservation(r pinOwnerRecord, x PinOwnerReservation) bool {
	return r.PlanRef == x.planRef && r.OperationID == x.operationID && r.ReservationVersion == x.reservationVersion && r.RecordVersion == x.recordVersion && r.State == string(x.state)
}
func pinOwnerValidatePin(r pinOwnerRecord, p backup.SnapshotPinRef) error {
	if !pinOwnerIsHex(p.ID(), 64) || p.ID() == "" || p.PlanRef() != r.PlanRef || p.ManifestSHA256() != r.ManifestSHA256 || p.ReservationVersion() != r.ReservationVersion {
		return pinOwnerConflict()
	}
	return nil
}
func pinOwnerMatchPin(r pinOwnerRecord, p backup.SnapshotPinRef) error {
	if e := pinOwnerValidatePin(r, p); e != nil {
		return e
	}
	if r.PinID != p.ID() {
		return pinOwnerConflict()
	}
	return nil
}
func pinOwnerAuthorization(r pinOwnerRecord) backup.PinReleaseAuthorization {
	return backup.PinReleaseAuthorization{PinID: r.PinID, PlanRef: r.PlanRef, ManifestSHA256: r.ManifestSHA256, Disposition: backup.PinAbandonedBeforeEffects, RecordVersion: r.ReleaseRecordVersion}
}
func pinOwnerAuthorizationMatches(r pinOwnerRecord, a backup.PinReleaseAuthorization) bool {
	return r.PinID == a.PinID && r.PlanRef == a.PlanRef && r.ManifestSHA256 == a.ManifestSHA256 && r.ReleaseDisposition == "ABANDONED_BEFORE_EFFECTS" && r.ReleaseRecordVersion == a.RecordVersion && a.Disposition == backup.PinAbandonedBeforeEffects
}

type pinOwnerBackupRelease struct {
	Version            int                          `json:"version"`
	Checksum           string                       `json:"checksum"`
	ID                 string                       `json:"id"`
	PinID              string                       `json:"pin_id"`
	PlanRef            string                       `json:"plan_ref"`
	ManifestSHA256     string                       `json:"manifest_sha256"`
	ReservationVersion uint64                       `json:"reservation_version"`
	AttemptVersion     uint64                       `json:"attempt_version"`
	Disposition        backup.PinReleaseDisposition `json:"disposition"`
	RecordVersion      uint64                       `json:"record_version"`
	State              string                       `json:"state"`
	MetadataBytes      int64                        `json:"metadata_bytes"`
}

func pinOwnerVerifyBackupRelease(ctx context.Context, o *SnapshotPinOwner, r pinOwnerRecord) (retErr error) {
	if err := pinOwnerValidateNamedIdentity(ctx, o); err != nil {
		return errors.Join(ErrPinOwnerUnavailable, err)
	}
	root, err := pinOwnerOpenDirectory(o.options.BackupLeaseRoot)
	if err != nil {
		return errors.Join(ErrPinOwnerUnavailable, err)
	}
	defer func() { retErr = errors.Join(retErr, root.Close()) }()
	releases, err := pinOwnerOpenDirAt(root, ".releases")
	if err != nil {
		return errors.Join(ErrPinOwnerUnavailable, err)
	}
	defer func() { retErr = errors.Join(retErr, releases.Close()) }()
	marker, err := pinOwnerOpenDirAt(releases, r.LeaseID)
	if err != nil {
		return errors.Join(ErrPinOwnerUnavailable, err)
	}
	defer func() { retErr = errors.Join(retErr, marker.Close()) }()
	raw, err := pinOwnerReadBoundedAt(ctx, marker, "release.json", pinOwnerMaxRecordBytes)
	if err != nil {
		return errors.Join(ErrPinOwnerUnavailable, err)
	}
	var t pinOwnerBackupRelease
	if err = pinOwnerStrictDecode(raw, &t); err != nil {
		return errors.Join(ErrPinOwnerCorrupt, err)
	}
	checksum := t.Checksum
	t.Checksum = ""
	base, err := json.Marshal(t)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(base)
	t.Checksum = checksum
	if checksum != hex.EncodeToString(sum[:]) || t.Version != 1 || t.ID != r.LeaseID || t.PinID != r.PinID || t.PlanRef != r.PlanRef || t.ManifestSHA256 != r.ManifestSHA256 || t.ReservationVersion != r.ReservationVersion || t.AttemptVersion != r.AttemptVersion || t.Disposition != backup.PinAbandonedBeforeEffects || t.RecordVersion != r.ReleaseRecordVersion || t.State != "RELEASED" || t.MetadataBytes != int64(len(raw)) {
		return ErrPinOwnerCorrupt
	}
	if err = pinOwnerValidateNamedIdentity(ctx, o); err != nil {
		return errors.Join(ErrPinOwnerUnavailable, err)
	}
	return nil
}

// Canonical v1 superblock payload and record wire structs. Field order is part of the format.
type pinOwnerSuperblockPayload struct {
	Version          uint64 `json:"version"`
	StoreID          string `json:"store_id"`
	OwnerRootDevice  uint64 `json:"owner_root_device"`
	OwnerRootInode   uint64 `json:"owner_root_inode"`
	OwnerLockDevice  uint64 `json:"owner_lock_device"`
	OwnerLockInode   uint64 `json:"owner_lock_inode"`
	BackupRootDevice uint64 `json:"backup_root_device"`
	BackupRootInode  uint64 `json:"backup_root_inode"`
	BackupLockDevice uint64 `json:"backup_lock_device"`
	BackupLockInode  uint64 `json:"backup_lock_inode"`
}
type pinOwnerSuperblock struct {
	Version          uint64 `json:"version"`
	StoreID          string `json:"store_id"`
	OwnerRootDevice  uint64 `json:"owner_root_device"`
	OwnerRootInode   uint64 `json:"owner_root_inode"`
	OwnerLockDevice  uint64 `json:"owner_lock_device"`
	OwnerLockInode   uint64 `json:"owner_lock_inode"`
	BackupRootDevice uint64 `json:"backup_root_device"`
	BackupRootInode  uint64 `json:"backup_root_inode"`
	BackupLockDevice uint64 `json:"backup_lock_device"`
	BackupLockInode  uint64 `json:"backup_lock_inode"`
	ChecksumSHA256   string `json:"checksum_sha256"`
}
type pinOwnerRecordPayload struct {
	Version              uint64 `json:"version"`
	StoreID              string `json:"store_id"`
	PlanRef              string `json:"plan_ref"`
	ReservationVersion   uint64 `json:"reservation_version"`
	RecordVersion        uint64 `json:"record_version"`
	PreviousSHA256       string `json:"previous_sha256"`
	OperationID          string `json:"operation_id"`
	Event                string `json:"event"`
	State                string `json:"state"`
	AttemptHighWater     uint64 `json:"attempt_high_water"`
	AttemptVersion       uint64 `json:"attempt_version"`
	LeaseID              string `json:"lease_id"`
	ManifestSHA256       string `json:"manifest_sha256"`
	AttemptState         string `json:"attempt_state"`
	PinID                string `json:"pin_id"`
	ReleaseDisposition   string `json:"release_disposition"`
	ReleaseRecordVersion uint64 `json:"release_record_version"`
	ReasonCode           string `json:"reason_code"`
}
type pinOwnerRecord struct {
	Version              uint64 `json:"version"`
	StoreID              string `json:"store_id"`
	PlanRef              string `json:"plan_ref"`
	ReservationVersion   uint64 `json:"reservation_version"`
	RecordVersion        uint64 `json:"record_version"`
	PreviousSHA256       string `json:"previous_sha256"`
	OperationID          string `json:"operation_id"`
	Event                string `json:"event"`
	State                string `json:"state"`
	AttemptHighWater     uint64 `json:"attempt_high_water"`
	AttemptVersion       uint64 `json:"attempt_version"`
	LeaseID              string `json:"lease_id"`
	ManifestSHA256       string `json:"manifest_sha256"`
	AttemptState         string `json:"attempt_state"`
	PinID                string `json:"pin_id"`
	ReleaseDisposition   string `json:"release_disposition"`
	ReleaseRecordVersion uint64 `json:"release_record_version"`
	ReasonCode           string `json:"reason_code"`
	ChecksumSHA256       string `json:"checksum_sha256"`
}
type pinOwnerPlan struct {
	records []pinOwnerRecord
	head    pinOwnerRecord
	hash    string
}

func pinOwnerSuperPayload(b pinOwnerSuperblock) pinOwnerSuperblockPayload {
	return pinOwnerSuperblockPayload{b.Version, b.StoreID, b.OwnerRootDevice, b.OwnerRootInode, b.OwnerLockDevice, b.OwnerLockInode, b.BackupRootDevice, b.BackupRootInode, b.BackupLockDevice, b.BackupLockInode}
}
func pinOwnerRecordPayloadOf(r pinOwnerRecord) pinOwnerRecordPayload {
	return pinOwnerRecordPayload{r.Version, r.StoreID, r.PlanRef, r.ReservationVersion, r.RecordVersion, r.PreviousSHA256, r.OperationID, r.Event, r.State, r.AttemptHighWater, r.AttemptVersion, r.LeaseID, r.ManifestSHA256, r.AttemptState, r.PinID, r.ReleaseDisposition, r.ReleaseRecordVersion, r.ReasonCode}
}
func pinOwnerChecksum(v any) (string, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(append([]byte(pinOwnerHashDomain), b...))
	return hex.EncodeToString(h[:]), nil
}

func pinOwnerSuperChecksum(v any) (string, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(append([]byte(pinOwnerSuperHashDomain), b...))
	return hex.EncodeToString(h[:]), nil
}
func pinOwnerEncodeSuper(b pinOwnerSuperblock) ([]byte, error) {
	c, e := pinOwnerSuperChecksum(pinOwnerSuperPayload(b))
	if e != nil {
		return nil, e
	}
	b.ChecksumSHA256 = c
	raw, e := json.Marshal(b)
	if e == nil && len(raw) > pinOwnerMaxSuperBytes {
		return nil, ErrPinOwnerLimit
	}
	return raw, e
}
func pinOwnerEncodeRecord(r pinOwnerRecord) ([]byte, string, error) {
	c, e := pinOwnerChecksum(pinOwnerRecordPayloadOf(r))
	if e != nil {
		return nil, "", e
	}
	r.ChecksumSHA256 = c
	raw, e := json.Marshal(r)
	if e != nil {
		return nil, "", e
	}
	if len(raw) > pinOwnerMaxRecordBytes {
		return nil, "", ErrPinOwnerLimit
	}
	h := sha256.Sum256(raw)
	return raw, hex.EncodeToString(h[:]), nil
}
func pinOwnerStrictDecode(raw []byte, v any) error {
	if len(raw) == 0 || len(raw) > pinOwnerMaxRecordBytes {
		return ErrPinOwnerCorrupt
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return errors.Join(ErrPinOwnerCorrupt, e)
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return errors.Join(ErrPinOwnerCorrupt, e)
	}
	canonical, e := json.Marshal(v)
	if e != nil || !bytes.Equal(canonical, raw) {
		return errors.Join(ErrPinOwnerCorrupt, e)
	}
	return nil
}
func pinOwnerDecodeSuper(raw []byte) (pinOwnerSuperblock, error) {
	var b pinOwnerSuperblock
	version, found, err := pinOwnerWireVersionHint(raw, pinOwnerMaxSuperBytes)
	if err != nil {
		return b, err
	}
	if found && version != pinOwnerWireVersion {
		return b, ErrPinOwnerUnavailable
	}
	if err = pinOwnerStrictDecode(raw, &b); err != nil {
		return b, err
	}
	sum, e := pinOwnerSuperChecksum(pinOwnerSuperPayload(b))
	if e != nil || sum != b.ChecksumSHA256 || !pinOwnerIsHex(b.StoreID, 32) {
		return b, ErrPinOwnerCorrupt
	}
	return b, nil
}
func pinOwnerRecordVersionHint(raw []byte) (uint64, bool, error) {
	return pinOwnerWireVersionHint(raw, pinOwnerMaxRecordBytes)
}

func pinOwnerWireVersionHint(raw []byte, maxBytes int) (uint64, bool, error) {
	if len(raw) == 0 || len(raw) > maxBytes {
		return 0, false, ErrPinOwnerCorrupt
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	tok, err := d.Token()
	if err != nil {
		return 0, false, errors.Join(ErrPinOwnerCorrupt, err)
	}
	open, ok := tok.(json.Delim)
	if !ok || open != '{' {
		return 0, false, nil // The strict v1 decoder classifies non-object records.
	}
	seen := make(map[string]struct{})
	var rawVersion json.RawMessage
	foundVersion := false
	for d.More() {
		keyToken, tokenErr := d.Token()
		if tokenErr != nil {
			return 0, false, errors.Join(ErrPinOwnerCorrupt, tokenErr)
		}
		key, ok := keyToken.(string)
		if !ok {
			return 0, false, ErrPinOwnerCorrupt
		}
		if _, duplicate := seen[key]; duplicate {
			return 0, false, ErrPinOwnerCorrupt
		}
		seen[key] = struct{}{}
		var value json.RawMessage
		if tokenErr = d.Decode(&value); tokenErr != nil {
			return 0, false, errors.Join(ErrPinOwnerCorrupt, tokenErr)
		}
		if key == "version" {
			rawVersion = value
			foundVersion = true
		}
	}
	closeToken, err := d.Token()
	if err != nil || closeToken != json.Delim('}') {
		return 0, false, errors.Join(ErrPinOwnerCorrupt, err)
	}
	var trailing any
	if err = d.Decode(&trailing); err != io.EOF {
		return 0, false, errors.Join(ErrPinOwnerCorrupt, err)
	}
	if !foundVersion {
		return 0, false, ErrPinOwnerCorrupt
	}
	var version uint64
	if len(rawVersion) == 0 {
		return 0, false, ErrPinOwnerCorrupt
	}
	for _, b := range rawVersion {
		if b < '0' || b > '9' {
			return 0, false, ErrPinOwnerCorrupt
		}
	}
	if len(rawVersion) > 1 && rawVersion[0] == '0' {
		return 0, false, ErrPinOwnerCorrupt
	}
	if err = json.Unmarshal(rawVersion, &version); err != nil || version == 0 {
		return 0, false, ErrPinOwnerCorrupt
	}
	return version, true, nil
}

func pinOwnerDecodeRecord(raw []byte) (pinOwnerRecord, string, error) {
	var r pinOwnerRecord
	version, found, err := pinOwnerRecordVersionHint(raw)
	if err != nil {
		return r, "", err
	}
	if found && version != pinOwnerWireVersion {
		return r, "", ErrPinOwnerUnavailable
	}
	if err = pinOwnerStrictDecode(raw, &r); err != nil {
		return r, "", err
	}
	sum, e := pinOwnerChecksum(pinOwnerRecordPayloadOf(r))
	if e != nil || sum != r.ChecksumSHA256 {
		return r, "", ErrPinOwnerCorrupt
	}
	h := sha256.Sum256(raw)
	return r, hex.EncodeToString(h[:]), nil
}

func pinOwnerReadDirBounded(ctx context.Context, dir *os.File, max int) (entries []os.DirEntry, retErr error) {
	if ctx == nil || max < 0 {
		return nil, ErrPinOwnerInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := pinOwnerReopenDirectory(dir)
	if err != nil {
		return nil, err
	}
	defer func() {
		retErr = errors.Join(retErr, f.Close())
		if retErr != nil {
			entries = nil
		}
	}()
	entries, err = f.ReadDir(max + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > max {
		return nil, ErrPinOwnerLimit
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

// pinOwnerValidateBootstrapPrefix permits only the durable prefixes that can
// result from initializing a new owner root. It is called before creating the
// lock and again while holding the lock before adding reservations or a
// superblock, so existing history or unknown entries cannot be rebound.
func pinOwnerValidateBootstrapPrefix(ctx context.Context, root *os.File) (lockPresent bool, retErr error) {
	entries, err := pinOwnerReadDirBounded(ctx, root, 3)
	if err != nil {
		return false, errors.Join(ErrPinOwnerUnavailable, err)
	}
	hasSuper, hasReservations := false, false
	for _, entry := range entries {
		switch entry.Name() {
		case pinOwnerLockName:
			lockPresent = true
		case pinOwnerSuperName:
			hasSuper = true
		case pinOwnerReservationsName:
			hasReservations = true
		default:
			return false, ErrPinOwnerUnavailable
		}
	}
	if !lockPresent {
		if len(entries) != 0 {
			return false, ErrPinOwnerUnavailable
		}
		return false, nil
	}
	lock, err := pinOwnerOpenLockAt(root, pinOwnerLockName, false)
	if err != nil {
		return false, errors.Join(ErrPinOwnerUnavailable, err)
	}
	if err = lock.Close(); err != nil {
		return false, errors.Join(ErrPinOwnerUnavailable, err)
	}
	if hasSuper && !hasReservations {
		return false, ErrPinOwnerUnavailable
	}
	if !hasSuper && hasReservations {
		reservations, openErr := pinOwnerOpenDirAt(root, pinOwnerReservationsName)
		if openErr != nil {
			return false, errors.Join(ErrPinOwnerUnavailable, openErr)
		}
		children, readErr := pinOwnerReadDirBounded(ctx, reservations, 0)
		closeErr := reservations.Close()
		if readErr != nil || closeErr != nil || len(children) != 0 {
			return false, errors.Join(ErrPinOwnerUnavailable, readErr, closeErr)
		}
	}
	return true, nil
}

func pinOwnerReadBoundedAt(ctx context.Context, dir *os.File, name string, limit int64) (data []byte, retErr error) {
	if ctx == nil {
		return nil, ErrPinOwnerInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := pinOwnerOpenRegularAt(dir, name, false)
	if err != nil {
		return nil, err
	}
	defer func() {
		retErr = errors.Join(retErr, f.Close())
		if retErr != nil {
			data = nil
		}
	}()
	i, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !i.Mode().IsRegular() || i.Size() < 0 || i.Size() > limit {
		return nil, ErrPinOwnerCorrupt
	}
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) > limit {
		return nil, ErrPinOwnerCorrupt
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return b, nil
}
func pinOwnerReadSuperblock(ctx context.Context, root *os.File) (pinOwnerSuperblock, error) {
	b, e := pinOwnerReadBoundedAt(ctx, root, pinOwnerSuperName, pinOwnerMaxSuperBytes)
	if e != nil {
		return pinOwnerSuperblock{}, e
	}
	return pinOwnerDecodeSuper(b)
}

func pinOwnerValidateSuperblock(ctx context.Context, root *os.File, o *SnapshotPinOwner) error {
	if ctx == nil || root == nil || o == nil {
		return ErrPinOwnerInvalid
	}
	block, err := pinOwnerReadSuperblock(ctx, root)
	if err != nil {
		return err
	}
	if block.StoreID != o.storeID || block.OwnerRootDevice != o.ownerDevice || block.OwnerRootInode != o.ownerInode || block.OwnerLockDevice != o.ownerLockDev || block.OwnerLockInode != o.ownerLockIno || block.BackupRootDevice != o.backupDevice || block.BackupRootInode != o.backupInode || block.BackupLockDevice != o.backupLockDev || block.BackupLockInode != o.backupLockIno {
		return ErrPinOwnerUnavailable
	}
	return nil
}

func pinOwnerWriteSuperblock(ctx context.Context, root *os.File, b pinOwnerSuperblock) error {
	raw, e := pinOwnerEncodeSuper(b)
	if e != nil {
		return e
	}
	if e = pinOwnerAtomicWriteAt(ctx, root, pinOwnerSuperName, raw); e != nil {
		return e
	}
	readback, e := pinOwnerReadBoundedAt(ctx, root, pinOwnerSuperName, pinOwnerMaxSuperBytes)
	if e != nil {
		return errors.Join(ErrPinOwnerOutcomeUnknown, e)
	}
	if !bytes.Equal(raw, readback) {
		return ErrPinOwnerOutcomeUnknown
	}
	decoded, e := pinOwnerDecodeSuper(readback)
	if e != nil || decoded != b && decoded.StoreID != b.StoreID {
		return errors.Join(ErrPinOwnerOutcomeUnknown, e)
	}
	if e = ctx.Err(); e != nil {
		return errors.Join(ErrPinOwnerOutcomeUnknown, e)
	}
	return nil
}
func pinOwnerRecordName(v uint64) string { return fmt.Sprintf("%020d.json", v) }
func pinOwnerParseRecordName(s string) (uint64, bool) {
	if len(s) != 25 || !strings.HasSuffix(s, ".json") {
		return 0, false
	}
	n, e := strconv.ParseUint(s[:20], 10, 64)
	return n, e == nil && pinOwnerRecordName(n) == s
}
func pinOwnerAppend(ctx context.Context, root *os.File, o *SnapshotPinOwner, dir *os.File, r pinOwnerRecord) error {
	if root == nil {
		return ErrPinOwnerInvalid
	}
	if err := pinOwnerCheckCapacity(o, 0); err != nil {
		return err
	}
	if err := pinOwnerValidateNamedIdentity(ctx, o); err != nil {
		return errors.Join(ErrPinOwnerUnavailable, err)
	}
	if err := pinOwnerValidateSuperblock(ctx, root, o); err != nil {
		return err
	}
	raw, _, err := pinOwnerEncodeRecord(r)
	if err != nil {
		return err
	}
	expectedRecord, _, err := pinOwnerDecodeRecord(raw)
	if err != nil {
		return err
	}
	entries, err := pinOwnerReadDirBounded(ctx, dir, pinOwnerMaxRecords)
	if err != nil {
		return errors.Join(ErrPinOwnerLimit, backup.ErrSnapshotLeaseLimit, err)
	}
	if len(entries) >= pinOwnerMaxRecords {
		return errors.Join(ErrPinOwnerLimit, backup.ErrSnapshotLeaseLimit)
	}
	name := pinOwnerRecordName(r.RecordVersion)
	if err = pinOwnerValidateNamedIdentity(ctx, o); err != nil {
		return errors.Join(ErrPinOwnerUnavailable, err)
	}
	if err = pinOwnerAtomicWriteAt(ctx, dir, name, raw); err != nil {
		return errors.Join(ErrPinOwnerOutcomeUnknown, err)
	}
	if err = pinOwnerValidateNamedIdentity(ctx, o); err != nil {
		return errors.Join(ErrPinOwnerOutcomeUnknown, err)
	}
	verified, err := pinOwnerLoadPlan(ctx, dir, o, r.PlanRef)
	if err != nil {
		return errors.Join(ErrPinOwnerOutcomeUnknown, err)
	}
	if verified.head != expectedRecord {
		return ErrPinOwnerOutcomeUnknown
	}
	pinOwnerCrashInject("after-event:" + r.Event)
	if err = ctx.Err(); err != nil {
		return errors.Join(ErrPinOwnerOutcomeUnknown, err)
	}
	return nil
}
func pinOwnerLoadAll(ctx context.Context, root *os.File, o *SnapshotPinOwner) (plans []pinOwnerPlan, retErr error) {
	rootEntries, rootErr := pinOwnerReadDirBounded(ctx, root, 3)
	if rootErr != nil {
		return nil, errors.Join(ErrPinOwnerCorrupt, rootErr)
	}
	allowed := map[string]bool{pinOwnerLockName: true, pinOwnerSuperName: true, pinOwnerReservationsName: true}
	for _, entry := range rootEntries {
		if !allowed[entry.Name()] {
			return nil, ErrPinOwnerCorrupt
		}
	}
	reservations, err := pinOwnerOpenDirAt(root, pinOwnerReservationsName)
	if err != nil {
		return nil, errors.Join(ErrPinOwnerUnavailable, err)
	}
	defer func() {
		retErr = errors.Join(retErr, reservations.Close())
		if retErr != nil {
			plans = nil
		}
	}()
	entries, err := pinOwnerReadDirBounded(ctx, reservations, o.options.MaxPlans)
	if err != nil {
		return nil, errors.Join(ErrPinOwnerLimit, backup.ErrSnapshotLeaseLimit, err)
	}
	plans = make([]pinOwnerPlan, 0, len(entries))
	seenOps := map[string]bool{}
	seenIDs := map[string]bool{}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !pinOwnerIsHex(entry.Name(), 64) {
			return nil, ErrPinOwnerCorrupt
		}
		dir, e := pinOwnerOpenDirAt(reservations, entry.Name())
		if e != nil {
			return nil, errors.Join(ErrPinOwnerCorrupt, e)
		}
		plan, e := pinOwnerLoadPlan(ctx, dir, o, entry.Name())
		closeErr := dir.Close()
		if e != nil || closeErr != nil {
			return nil, errors.Join(e, closeErr)
		}
		if seenOps[plan.head.OperationID] || seenIDs[plan.head.PlanRef] {
			return nil, ErrPinOwnerCorrupt
		}
		seenOps[plan.head.OperationID] = true
		seenIDs[plan.head.PlanRef] = true
		plans = append(plans, plan)
	}
	if err = pinOwnerCheckCapacity(o, len(plans)); err != nil {
		return nil, err
	}
	return plans, nil
}
func pinOwnerLoadPlan(ctx context.Context, dir *os.File, o *SnapshotPinOwner, expectedPlan string) (pinOwnerPlan, error) {
	entries, err := pinOwnerReadDirBounded(ctx, dir, pinOwnerMaxRecords)
	if err != nil {
		return pinOwnerPlan{}, errors.Join(ErrPinOwnerLimit, backup.ErrSnapshotLeaseLimit, err)
	}
	if len(entries) == 0 {
		return pinOwnerPlan{}, ErrPinOwnerCorrupt
	}
	records := make([]pinOwnerRecord, 0, len(entries))
	lastHash := pinOwnerZeroDigest
	var operation string
	for i, entry := range entries {
		if err := ctx.Err(); err != nil {
			return pinOwnerPlan{}, err
		}
		version, ok := pinOwnerParseRecordName(entry.Name())
		if !ok || version != uint64(i+1) {
			return pinOwnerPlan{}, ErrPinOwnerCorrupt
		}
		raw, e := pinOwnerReadBoundedAt(ctx, dir, entry.Name(), pinOwnerMaxRecordBytes)
		if e != nil {
			return pinOwnerPlan{}, errors.Join(ErrPinOwnerCorrupt, e)
		}
		r, h, e := pinOwnerDecodeRecord(raw)
		if e != nil {
			return pinOwnerPlan{}, e
		}
		if r.RecordVersion != version || r.PlanRef != expectedPlan || r.StoreID != o.storeID || r.PreviousSHA256 != lastHash {
			return pinOwnerPlan{}, ErrPinOwnerCorrupt
		}
		if i == 0 {
			operation = r.OperationID
		} else if r.OperationID != operation {
			return pinOwnerPlan{}, ErrPinOwnerCorrupt
		}
		records = append(records, r)
		lastHash = h
	}
	if err = pinOwnerValidateHistory(ctx, records); err != nil {
		return pinOwnerPlan{}, err
	}
	return pinOwnerPlan{records: records, head: records[len(records)-1], hash: lastHash}, nil
}
func pinOwnerValidateHistory(ctx context.Context, rs []pinOwnerRecord) error {
	if ctx == nil {
		return ErrPinOwnerInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(rs) == 0 || len(rs) > pinOwnerMaxRecords {
		return ErrPinOwnerCorrupt
	}
	first := rs[0]
	if first.Event != pinOwnerEventReserve || first.State == "" || first.State != string(PinOwnerReserved) || first.Version != 1 || first.ReservationVersion != 1 || first.RecordVersion != 1 || first.PreviousSHA256 != pinOwnerZeroDigest || !pinOwnerOperationIDPattern.MatchString(first.OperationID) || !pinOwnerIsHex(first.PlanRef, 64) || first.StoreID == "" || first.AttemptHighWater != 0 || first.AttemptVersion != 0 || first.LeaseID != "" || first.ManifestSHA256 != "" || first.AttemptState != "" || first.PinID != "" || first.ReleaseDisposition != "" || first.ReleaseRecordVersion != 0 || first.ReasonCode != "" {
		return ErrPinOwnerCorrupt
	}
	for i := 1; i < len(rs); i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		p, r := rs[i-1], rs[i]
		if r.Version != 1 || r.RecordVersion != p.RecordVersion+1 || r.StoreID != p.StoreID || r.PlanRef != p.PlanRef || r.ReservationVersion != p.ReservationVersion || r.OperationID != p.OperationID {
			return ErrPinOwnerCorrupt
		}
		if err := pinOwnerValidateTransition(p, r); err != nil {
			return err
		}
	}
	return ctx.Err()
}
func pinOwnerValidateTransition(p, r pinOwnerRecord) error {
	eqAttempt := func(a, b pinOwnerRecord) bool {
		return a.AttemptHighWater == b.AttemptHighWater && a.AttemptVersion == b.AttemptVersion && a.LeaseID == b.LeaseID && a.ManifestSHA256 == b.ManifestSHA256 && a.AttemptState == b.AttemptState && a.PinID == b.PinID
	}
	emptyRelease := func(x pinOwnerRecord) bool { return x.ReleaseDisposition == "" && x.ReleaseRecordVersion == 0 }
	switch r.Event {
	case pinOwnerEventBegin:
		if p.State != string(PinOwnerReserved) || (p.AttemptState != "" && p.AttemptState != pinOwnerAttemptCanceled) || p.AttemptHighWater >= pinOwnerMaxAttempts || r.State != string(PinOwnerPinPending) || r.AttemptHighWater != p.AttemptHighWater+1 || r.AttemptVersion != r.AttemptHighWater || !pinOwnerIsHex(r.LeaseID, 64) || !pinOwnerIsHex(r.ManifestSHA256, 64) || r.AttemptState != pinOwnerAttemptPending || r.PinID != "" || !emptyRelease(r) || r.ReasonCode != "" {
			return ErrPinOwnerCorrupt
		}
		if p.AttemptState == pinOwnerAttemptCanceled && (r.LeaseID == p.LeaseID || r.ManifestSHA256 != p.ManifestSHA256) {
			return ErrPinOwnerCorrupt
		}
	case pinOwnerEventCommit:
		if p.State != string(PinOwnerPinPending) || p.AttemptState != pinOwnerAttemptPending || r.State != string(PinOwnerPinned) || r.AttemptHighWater != p.AttemptHighWater || r.AttemptVersion != p.AttemptVersion || r.LeaseID != p.LeaseID || r.ManifestSHA256 != p.ManifestSHA256 || r.AttemptState != pinOwnerAttemptCommitted || !pinOwnerIsHex(r.PinID, 64) || !emptyRelease(r) || r.ReasonCode != "" {
			return ErrPinOwnerCorrupt
		}
	case pinOwnerEventCancel:
		if p.State != string(PinOwnerPinPending) || p.AttemptState != pinOwnerAttemptPending || r.State != string(PinOwnerReserved) || r.AttemptHighWater != p.AttemptHighWater || r.AttemptVersion != p.AttemptVersion || r.LeaseID != p.LeaseID || r.ManifestSHA256 != p.ManifestSHA256 || r.AttemptState != pinOwnerAttemptCanceled || r.PinID != "" || !emptyRelease(r) || r.ReasonCode != "" {
			return ErrPinOwnerCorrupt
		}
	case pinOwnerEventAbandon:
		if (p.State != string(PinOwnerReserved) && p.State != string(PinOwnerPinned)) || p.AttemptState == pinOwnerAttemptPending || r.State != string(PinOwnerAbandonedBeforeEffects) || !pinOwnerEqExceptReason(p, r) || !emptyRelease(r) || (r.ReasonCode != "OPERATOR_ABORT_BEFORE_READY" && r.ReasonCode != "SETUP_FAILURE_BEFORE_READY") {
			return ErrPinOwnerCorrupt
		}
	case pinOwnerEventReleaseBegin:
		if p.State != string(PinOwnerAbandonedBeforeEffects) || p.AttemptState != pinOwnerAttemptCommitted || r.State != string(PinOwnerReleasing) || !eqAttempt(p, r) || r.ReasonCode != "" || r.ReleaseDisposition != "ABANDONED_BEFORE_EFFECTS" || r.ReleaseRecordVersion != r.RecordVersion {
			return ErrPinOwnerCorrupt
		}
	case pinOwnerEventReleaseDone:
		if p.State != string(PinOwnerReleasing) || r.State != string(PinOwnerReleased) || !eqAttempt(p, r) || r.ReleaseDisposition != p.ReleaseDisposition || r.ReleaseRecordVersion != p.ReleaseRecordVersion || r.ReasonCode != p.ReasonCode {
			return ErrPinOwnerCorrupt
		}
	default:
		return ErrPinOwnerCorrupt
	}
	return nil
}
func pinOwnerEqExceptReason(a, b pinOwnerRecord) bool {
	return a.AttemptHighWater == b.AttemptHighWater && a.AttemptVersion == b.AttemptVersion && a.LeaseID == b.LeaseID && a.ManifestSHA256 == b.ManifestSHA256 && a.AttemptState == b.AttemptState && a.PinID == b.PinID && a.ReleaseDisposition == b.ReleaseDisposition && a.ReleaseRecordVersion == b.ReleaseRecordVersion && b.ReasonCode != ""
}
func pinOwnerFindPlan(ctx context.Context, root *os.File, o *SnapshotPinOwner, plan string) (pinOwnerPlan, *os.File, error) {
	if !pinOwnerIsHex(plan, 64) {
		return pinOwnerPlan{}, nil, ErrPinOwnerInvalid
	}
	if _, e := pinOwnerLoadAll(ctx, root, o); e != nil {
		return pinOwnerPlan{}, nil, e
	}
	reservations, e := pinOwnerOpenDirAt(root, pinOwnerReservationsName)
	if e != nil {
		return pinOwnerPlan{}, nil, errors.Join(ErrPinOwnerUnavailable, e)
	}
	dir, e := pinOwnerOpenDirAt(reservations, plan)
	closeErr := reservations.Close()
	if e != nil {
		return pinOwnerPlan{}, nil, errors.Join(pinOwnerNotFound(), closeErr)
	}
	p, e := pinOwnerLoadPlan(ctx, dir, o, plan)
	if e != nil {
		return pinOwnerPlan{}, nil, errors.Join(e, dir.Close())
	}
	if closeErr != nil {
		return pinOwnerPlan{}, nil, errors.Join(closeErr, dir.Close())
	}
	return p, dir, nil
}
