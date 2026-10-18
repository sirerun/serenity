package writer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"sync"
)

// InitialMaintenanceObservation describes actual local custody, not authority or
// historical intent. Prefix is detached and never exported in public JSON.
type InitialMaintenanceObservation struct {
	Present       bool
	Device, Inode uint64
	UID, GID      uint32
	Mode          uint32
	Links         uint64
	Bytes         int64
	SHA256        string
	Prefix        []byte `json:"-"`
}

type InitialMaintenanceVerifier func(context.Context, InitialMaintenanceObservation) error

// PendingInitialMaintenance owns both stable locks but cannot finish maintenance
// or be used for installation. Verifiers must not reenter it or mutate its paths.
type PendingInitialMaintenance struct {
	mu       sync.Mutex
	owner    *BrainMaintenance
	path     string
	expected []byte
	observed InitialMaintenanceObservation
	hooks    initialMaintenanceHooks
	closed   bool
}

// Private per-call seams exercise physical short writes and durability failures.
// They never replace authority or install a mutable package-global dependency.
type initialMaintenanceHooks struct {
	syncs      maintenanceSyncs
	write      func(*os.File, []byte) (int, error)
	create     func(*os.Root, string) (*os.File, error)
	checkpoint func(string) error
}

func durableInitialMaintenanceHooks() initialMaintenanceHooks {
	return initialMaintenanceHooks{syncs: durableMaintenanceSyncs(), write: func(f *os.File, p []byte) (int, error) { return f.Write(p) }, create: maintenanceCreate}
}

func OpenInitialBrainMaintenance(path string, expected MaintenanceIdentity) (*PendingInitialMaintenance, error) {
	return openInitialBrainMaintenance(path, expected, durableInitialMaintenanceHooks())
}

func openInitialBrainMaintenance(path string, expected MaintenanceIdentity, hooks initialMaintenanceHooks) (*PendingInitialMaintenance, error) {
	raw, err := maintenanceBytes(expected)
	if err != nil {
		return nil, err
	}
	if hooks.syncs.file == nil || hooks.syncs.directory == nil || hooks.write == nil || hooks.create == nil {
		return nil, ErrMaintenancePending
	}
	view, owner, state, err := acquireMaintenanceLocks(path, hooks.syncs.directory)
	if err != nil {
		return nil, err
	}
	m := &BrainMaintenance{view: view, owner: owner, state: state, identity: expected}
	if err = m.pinRoot(path); err != nil {
		_ = m.Close()
		return nil, err
	}
	p := &PendingInitialMaintenance{owner: m, path: path, expected: raw, hooks: hooks}
	observed, info, err := p.inspectLocked()
	if err != nil {
		_ = p.closeLocked()
		return nil, err
	}
	p.observed = observed
	m.marker = info
	return p, nil
}

func detachedInitialObservation(v InitialMaintenanceObservation) InitialMaintenanceObservation {
	v.Prefix = bytes.Clone(v.Prefix)
	return v
}
func sameInitialObservation(a, b InitialMaintenanceObservation) bool {
	return a.Present == b.Present && a.Device == b.Device && a.Inode == b.Inode && a.UID == b.UID && a.GID == b.GID && a.Mode == b.Mode && a.Links == b.Links && a.Bytes == b.Bytes && a.SHA256 == b.SHA256 && bytes.Equal(a.Prefix, b.Prefix)
}

func (p *PendingInitialMaintenance) inspectLocked() (InitialMaintenanceObservation, os.FileInfo, error) {
	var zero InitialMaintenanceObservation
	if p.closed || p.owner == nil || p.owner.closed || validateInitialNamespace(p.owner, p.path) != nil {
		return zero, nil, ErrMaintenancePending
	}
	info, err := p.owner.state.Lstat(maintenanceMarkerName)
	if errors.Is(err, os.ErrNotExist) {
		return zero, nil, nil
	}
	if err != nil || !safeMaintenanceMarker(info) {
		return zero, nil, ErrMaintenancePending
	}
	f, err := maintenanceOpen(p.owner.state, maintenanceMarkerName)
	if err != nil {
		return zero, nil, err
	}
	before, statErr := f.Stat()
	raw, readErr := io.ReadAll(io.LimitReader(f, maintenanceMaxBytes+1))
	after, afterErr := f.Stat()
	closeErr := f.Close()
	current, currentErr := p.owner.state.Lstat(maintenanceMarkerName)
	if err = errors.Join(statErr, readErr, afterErr, closeErr, currentErr); err != nil {
		return zero, nil, errors.Join(ErrMaintenancePending, err)
	}
	if !safeMaintenanceMarker(before) || !safeMaintenanceMarker(after) || !safeMaintenanceMarker(current) || !os.SameFile(info, before) || !os.SameFile(before, after) || !os.SameFile(after, current) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || after.Size() != int64(len(raw)) || len(raw) > maintenanceMaxBytes || !bytes.HasPrefix(p.expected, raw) {
		return zero, nil, ErrMaintenanceIdentity
	}
	observed, err := initialMaintenanceObservation(after)
	if err != nil {
		return zero, nil, err
	}
	sum := sha256.Sum256(raw)
	observed.SHA256 = hex.EncodeToString(sum[:])
	observed.Prefix = bytes.Clone(raw)
	return observed, after, nil
}

func (p *PendingInitialMaintenance) validateLocked(path string, expected MaintenanceIdentity) error {
	if p.closed || p.owner == nil || expected != p.owner.identity || validateInitialNamespace(p.owner, path) != nil {
		return ErrMaintenanceIdentity
	}
	observed, info, err := p.inspectLocked()
	if err != nil {
		return err
	}
	if !sameInitialObservation(observed, p.observed) {
		return ErrMaintenanceIdentity
	}
	if observed.Present && (p.owner.marker == nil || !os.SameFile(info, p.owner.marker)) {
		return ErrMaintenanceIdentity
	}
	return nil
}

func (p *PendingInitialMaintenance) Observation() (InitialMaintenanceObservation, error) {
	if p == nil {
		return InitialMaintenanceObservation{}, ErrMaintenancePending
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.owner == nil || p.validateLocked(p.path, p.owner.identity) != nil {
		return InitialMaintenanceObservation{}, ErrMaintenancePending
	}
	return detachedInitialObservation(p.observed), nil
}

func (p *PendingInitialMaintenance) ValidateFor(path string, expected MaintenanceIdentity) error {
	if p == nil {
		return ErrMaintenancePending
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.validateLocked(path, expected)
}

func (p *PendingInitialMaintenance) verifyLocked(ctx context.Context, verifier InitialMaintenanceVerifier) error {
	if ctx == nil || verifier == nil {
		return ErrMaintenancePending
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(ErrMaintenancePending, err)
	}
	if p.owner == nil {
		return ErrMaintenancePending
	}
	if err := p.validateLocked(p.path, p.owner.identity); err != nil {
		return err
	}
	if err := verifier(ctx, detachedInitialObservation(p.observed)); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return errors.Join(ErrMaintenancePending, err)
	}
	return p.validateLocked(p.path, p.owner.identity)
}

func (p *PendingInitialMaintenance) atLocked(point string) error {
	if p.hooks.checkpoint != nil {
		return p.hooks.checkpoint(point)
	}
	return nil
}

// EnsureGate creates only a durable zero-byte gate. It never completes marker
// identity or transfers ownership. The actual inode is observed only after create.
func (p *PendingInitialMaintenance) EnsureGate(ctx context.Context, verifier InitialMaintenanceVerifier) (err error) {
	if p == nil {
		return ErrMaintenancePending
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	success := false
	defer func() {
		if !success {
			err = errors.Join(err, p.closeLocked())
		}
	}()
	if p.closed || p.owner == nil || p.observed.Present {
		return ErrMaintenanceIdentity
	}
	if err = p.verifyLocked(ctx, verifier); err != nil {
		return err
	}
	f, err := p.hooks.create(p.owner.state, maintenanceMarkerName)
	if err != nil {
		return errors.Join(ErrMaintenancePending, err)
	}
	if err = p.atLocked("gate-created"); err != nil {
		_ = f.Close()
		return err
	}
	info, statErr := f.Stat()
	if statErr != nil || !safeMaintenanceMarker(info) || info.Size() != 0 {
		_ = f.Close()
		return errors.Join(ErrMaintenancePending, statErr)
	}
	p.owner.marker = info
	if err = p.hooks.syncs.file(f); err != nil {
		_ = f.Close()
		return errors.Join(ErrMaintenancePending, err)
	}
	if err = f.Close(); err != nil {
		return errors.Join(ErrMaintenancePending, err)
	}
	if err = p.atLocked("gate-file-synced"); err != nil {
		return err
	}
	if err = p.hooks.syncs.directory(p.owner.state); err != nil {
		return errors.Join(ErrMaintenancePending, err)
	}
	if err = p.atLocked("gate-runtime-synced"); err != nil {
		return err
	}
	if err = p.hooks.syncs.directory(p.owner.rootOwner); err != nil {
		return errors.Join(ErrMaintenancePending, err)
	}
	if err = p.atLocked("gate-root-synced"); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return errors.Join(ErrMaintenancePending, err)
	}
	observed, current, err := p.inspectLocked()
	if err != nil || !observed.Present || observed.Bytes != 0 || !os.SameFile(info, current) {
		return errors.Join(ErrMaintenanceIdentity, err)
	}
	p.observed = observed
	if err = p.validateLocked(p.path, p.owner.identity); err != nil {
		return err
	}
	success = true
	return nil
}

// SyncGate makes the actual existing prefix durable before evidence binds its
// inode. It never appends, creates, transfers or treats admission as saved grant.
func (p *PendingInitialMaintenance) SyncGate(ctx context.Context, verifier InitialMaintenanceVerifier) (err error) {
	if p == nil {
		return ErrMaintenancePending
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	success := false
	defer func() {
		if !success {
			err = errors.Join(err, p.closeLocked())
		}
	}()
	if p.closed || p.owner == nil || !p.observed.Present {
		return ErrMaintenancePending
	}
	if err = p.verifyLocked(ctx, verifier); err != nil {
		return err
	}
	file, err := maintenanceOpen(p.owner.state, maintenanceMarkerName)
	if err != nil {
		return err
	}
	info, statErr := file.Stat()
	if statErr != nil || !safeMaintenanceMarker(info) || !os.SameFile(info, p.owner.marker) {
		_ = file.Close()
		return errors.Join(ErrMaintenanceIdentity, statErr)
	}
	if err = p.validateLocked(p.path, p.owner.identity); err != nil {
		_ = file.Close()
		return err
	}
	if err = p.hooks.syncs.file(file); err != nil {
		_ = file.Close()
		return errors.Join(ErrMaintenancePending, err)
	}
	if err = file.Close(); err != nil {
		return errors.Join(ErrMaintenancePending, err)
	}
	if err = p.atLocked("sync-gate-file-synced"); err != nil {
		return err
	}
	if err = p.hooks.syncs.directory(p.owner.state); err != nil {
		return errors.Join(ErrMaintenancePending, err)
	}
	if err = p.atLocked("sync-gate-runtime-synced"); err != nil {
		return err
	}
	if err = p.hooks.syncs.directory(p.owner.rootOwner); err != nil {
		return errors.Join(ErrMaintenancePending, err)
	}
	if err = p.atLocked("sync-gate-root-synced"); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return errors.Join(ErrMaintenancePending, err)
	}
	if err = p.validateLocked(p.path, p.owner.identity); err != nil {
		return err
	}
	success = true
	return nil
}

// Complete performs exactly two successful authority checks: before append and
// after file/runtime/root durability. Any failure closes this restricted owner.
func (p *PendingInitialMaintenance) Complete(ctx context.Context, verifier InitialMaintenanceVerifier) (owner *BrainMaintenance, err error) {
	if p == nil {
		return nil, ErrMaintenancePending
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	success := false
	defer func() {
		if !success {
			err = errors.Join(err, p.closeLocked())
		}
	}()
	if p.closed || p.owner == nil || !p.observed.Present {
		return nil, ErrMaintenancePending
	}
	if err = p.verifyLocked(ctx, verifier); err != nil {
		return nil, err
	}
	f, err := initialMaintenanceAppend(p.owner.state, maintenanceMarkerName)
	if err != nil {
		return nil, err
	}
	info, statErr := f.Stat()
	if statErr != nil || !safeMaintenanceMarker(info) || !os.SameFile(info, p.owner.marker) {
		_ = f.Close()
		return nil, errors.Join(ErrMaintenanceIdentity, statErr)
	}
	if err = ctx.Err(); err != nil {
		_ = f.Close()
		return nil, errors.Join(ErrMaintenancePending, err)
	}
	if err = p.validateLocked(p.path, p.owner.identity); err != nil {
		_ = f.Close()
		return nil, err
	}
	suffix := p.expected[len(p.observed.Prefix):]
	if len(suffix) > 0 {
		n, writeErr := p.hooks.write(f, suffix)
		if writeErr == nil && n != len(suffix) {
			writeErr = io.ErrShortWrite
		}
		if writeErr != nil {
			_ = f.Close()
			return nil, errors.Join(ErrMaintenancePending, writeErr)
		}
	}
	if err = p.atLocked("marker-appended"); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err = p.hooks.syncs.file(f); err != nil {
		_ = f.Close()
		return nil, errors.Join(ErrMaintenancePending, err)
	}
	if err = f.Close(); err != nil {
		return nil, errors.Join(ErrMaintenancePending, err)
	}
	if err = p.atLocked("marker-file-synced"); err != nil {
		return nil, err
	}
	if err = p.hooks.syncs.directory(p.owner.state); err != nil {
		return nil, errors.Join(ErrMaintenancePending, err)
	}
	if err = p.atLocked("marker-runtime-synced"); err != nil {
		return nil, err
	}
	if err = p.hooks.syncs.directory(p.owner.rootOwner); err != nil {
		return nil, errors.Join(ErrMaintenancePending, err)
	}
	if err = p.atLocked("marker-root-synced"); err != nil {
		return nil, err
	}
	observed, current, err := p.inspectLocked()
	if err != nil || !observed.Present || !bytes.Equal(observed.Prefix, p.expected) || !os.SameFile(current, p.owner.marker) {
		return nil, errors.Join(ErrMaintenanceIdentity, err)
	}
	p.observed = observed
	p.owner.marker = current
	if err = p.verifyLocked(ctx, verifier); err != nil {
		return nil, err
	}
	if err = p.owner.checkLocked(p.expected); err != nil {
		return nil, err
	}
	owner = p.owner
	p.owner = nil
	p.closed = true
	success = true
	return owner, nil
}

func (p *PendingInitialMaintenance) closeLocked() error {
	if p.closed {
		return nil
	}
	p.closed = true
	if p.owner != nil {
		return p.owner.Close()
	}
	return nil
}
func (p *PendingInitialMaintenance) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closeLocked()
}
