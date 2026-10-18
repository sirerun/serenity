package writer

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"sync"
)

var ErrMaintenancePending = errors.New("writer: canonical maintenance pending or unsafe")
var ErrMaintenanceIdentity = errors.New("writer: canonical maintenance identity mismatch")

const maintenanceMarkerName = "canonical-maintenance.json"
const maintenanceSchema = "serenity.canonical-maintenance/v1"
const maintenanceMaxBytes = 1024

// MaintenanceIdentity contains only opaque maintenance pins. It is not authority
// to erase, activate, authenticate journal history, or assert physical completion.
type MaintenanceIdentity struct {
	Schema      string `json:"schema"`
	OperationID string `json:"operation_id"`
	BrainID     string `json:"brain_id"`
	SourceHead  string `json:"source_head"`
	FenceDigest string `json:"fence_digest"`
}

var maintenanceOpaqueID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
var maintenanceBrainID = regexp.MustCompile(`^[A-Za-z0-9]{16,64}$`)
var maintenanceSHA = regexp.MustCompile(`^[0-9a-f]{64}$`)
var maintenanceGitID = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)

func maintenanceBytes(id MaintenanceIdentity) ([]byte, error) {
	if id.Schema != maintenanceSchema || !maintenanceOpaqueID.MatchString(id.OperationID) || !maintenanceBrainID.MatchString(id.BrainID) || !maintenanceGitID.MatchString(id.SourceHead) || !maintenanceSHA.MatchString(id.FenceDigest) {
		return nil, ErrMaintenanceIdentity
	}
	raw, err := json.Marshal(id)
	if err != nil || len(raw)+1 > maintenanceMaxBytes {
		return nil, ErrMaintenanceIdentity
	}
	return append(raw, '\n'), nil
}

// BrainMaintenance holds the same stable writer.lock as AcquireBrain. Closing
// never removes a marker. Callers own authority, canonical validation, activation
// and copy disposition; this primitive only persists a cooperating-writer gate.
type BrainMaintenance struct {
	mu           sync.Mutex
	view         *os.File
	owner        *os.File
	state        *os.Root
	identity     MaintenanceIdentity
	marker       os.FileInfo
	rootIdentity os.FileInfo
	rootOwner    *os.Root
	closed       bool
}

// maintenanceSyncs is a per-call fault seam; no global callbacks or authority
// are replaced. A failed durability boundary must never return a usable owner.
type maintenanceSyncs struct {
	file      func(*os.File) error
	directory func(*os.Root) error
}

func durableMaintenanceSyncs() maintenanceSyncs {
	return maintenanceSyncs{file: func(f *os.File) error { return f.Sync() }, directory: maintenanceSync}
}

// BeginBrainMaintenance acquires writer ownership and exclusively creates a
// durable marker. Failure may leave a partial marker; ordinary writers refuse it.
// Existing markers are never adopted or overwritten. The permanent canonical
// view must already have been initialized by an ordinary AcquireBrain owner.
// Maintenance acquires exclusive view ownership before exclusive writer ownership.
func BeginBrainMaintenance(path string, id MaintenanceIdentity) (*BrainMaintenance, error) {
	return beginBrainMaintenance(path, id, durableMaintenanceSyncs())
}

func beginBrainMaintenance(path string, id MaintenanceIdentity, syncs maintenanceSyncs) (*BrainMaintenance, error) {
	raw, err := maintenanceBytes(id)
	if err != nil {
		return nil, err
	}
	view, owner, state, err := acquireMaintenanceLocks(path, syncs.directory)
	if err != nil {
		return nil, err
	}
	m := &BrainMaintenance{view: view, owner: owner, state: state, identity: id}
	if err = m.pinRoot(path); err != nil {
		_ = m.Close()
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = m.Close()
		}
	}()
	file, err := maintenanceCreate(state, maintenanceMarkerName)
	if err != nil {
		return nil, errors.Join(ErrMaintenancePending, err)
	}
	// No failure path deletes a partial marker: a crash or failed fsync must
	// remain unavailable, never silently resume canonical writes.
	_, writeErr := file.Write(raw)
	syncErr := syncs.file(file)
	info, statErr := file.Stat()
	closeErr := file.Close()
	if err = errors.Join(writeErr, syncErr, statErr, closeErr); err != nil {
		return nil, errors.Join(ErrMaintenancePending, err)
	}
	m.marker = info
	if err = m.checkLocked(raw); err != nil {
		return nil, err
	}
	if err = syncs.directory(state); err != nil {
		return nil, errors.Join(ErrMaintenancePending, err)
	}
	success = true
	return m, nil
}

// ResumeBrainMaintenance acquires the same writer lock, accepting an existing
// marker only if its canonical bytes equal independently supplied expected pins.
// Matching local pins never establish journal authority or authorize activation.
// The existing canonical view is acquired exclusively before writer ownership.
func ResumeBrainMaintenance(path string, expected MaintenanceIdentity) (*BrainMaintenance, error) {
	return resumeBrainMaintenance(path, expected, durableMaintenanceSyncs())
}

func resumeBrainMaintenance(path string, expected MaintenanceIdentity, syncs maintenanceSyncs) (*BrainMaintenance, error) {
	raw, err := maintenanceBytes(expected)
	if err != nil {
		return nil, err
	}
	view, owner, state, err := acquireMaintenanceLocks(path, syncs.directory)
	if err != nil {
		return nil, err
	}
	m := &BrainMaintenance{view: view, owner: owner, state: state, identity: expected}
	if err = m.pinRoot(path); err != nil {
		_ = m.Close()
		return nil, err
	}
	file, err := maintenanceOpen(state, maintenanceMarkerName)
	if err != nil {
		_ = m.Close()
		return nil, errors.Join(ErrMaintenancePending, err)
	}
	info, statErr := file.Stat()
	closeErr := file.Close()
	if err = errors.Join(statErr, closeErr); err != nil {
		_ = m.Close()
		return nil, errors.Join(ErrMaintenancePending, err)
	}
	m.marker = info
	if err = m.checkLocked(raw); err != nil {
		_ = m.Close()
		return nil, err
	}
	// Matching page-cache bytes after an interrupted Begin do not prove durable
	// publication. Re-sync the exact pinned marker and runtime directory before
	// returning ownership that could authorize subsequent canonical maintenance.
	file, err = maintenanceOpen(state, maintenanceMarkerName)
	if err != nil {
		_ = m.Close()
		return nil, errors.Join(ErrMaintenancePending, err)
	}
	current, statErr := file.Stat()
	if statErr != nil || !os.SameFile(current, m.marker) {
		_ = file.Close()
		_ = m.Close()
		return nil, errors.Join(ErrMaintenanceIdentity, statErr)
	}
	syncErr := syncs.file(file)
	closeErr = file.Close()
	if err = errors.Join(syncErr, closeErr); err == nil {
		err = syncs.directory(state)
	}
	if err != nil {
		_ = m.Close()
		return nil, errors.Join(ErrMaintenancePending, err)
	}
	return m, nil
}

func (m *BrainMaintenance) checkLocked(expected []byte) error {
	if m.closed || m.view == nil || m.owner == nil || m.state == nil || m.marker == nil {
		return ErrMaintenancePending
	}
	viewInfo, err := m.view.Stat()
	currentView, statErr := m.state.Lstat(canonicalViewLockName)
	if err != nil || statErr != nil || !os.SameFile(viewInfo, currentView) {
		return ErrCanonicalViewUnavailable
	}
	lockInfo, err := m.owner.Stat()
	if err != nil {
		return errors.Join(ErrMaintenancePending, err)
	}
	currentLock, err := m.state.Lstat("writer.lock")
	if err != nil || !os.SameFile(lockInfo, currentLock) {
		return ErrMaintenancePending
	}
	file, err := maintenanceOpen(m.state, maintenanceMarkerName)
	if err != nil {
		return errors.Join(ErrMaintenancePending, err)
	}
	info, statErr := file.Stat()
	if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return errors.Join(ErrMaintenancePending, statErr, file.Close())
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, maintenanceMaxBytes+1))
	closeErr := file.Close()
	if err = errors.Join(statErr, readErr, closeErr); err != nil {
		return errors.Join(ErrMaintenancePending, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || !os.SameFile(info, m.marker) || !bytes.Equal(raw, expected) {
		return ErrMaintenanceIdentity
	}
	return nil
}

// Finish removes only the exact marker retained by this owner, after the caller
// has independently verified its maintenance result. It retains writer ownership
// until Close. It does not attest activation, erasure or copy cleanup.
func (m *BrainMaintenance) Finish(expected MaintenanceIdentity) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	raw, err := maintenanceBytes(expected)
	if err != nil {
		return err
	}
	if expected != m.identity {
		return ErrMaintenanceIdentity
	}
	if err = m.checkLocked(raw); err != nil {
		return err
	}
	if err = m.state.Remove(maintenanceMarkerName); err != nil {
		return errors.Join(ErrMaintenancePending, err)
	}
	m.marker = nil
	return maintenanceSync(m.state)
}

func (m *BrainMaintenance) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	var rootErr error
	if m.rootOwner != nil {
		rootErr = m.rootOwner.Close()
	}
	return errors.Join(m.state.Close(), m.owner.Close(), m.view.Close(), rootErr)
}
