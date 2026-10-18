//go:build darwin || linux

package writer

import (
	"os"
	"syscall"
)

// ValidateFor binds an open maintenance owner to its original canonical root,
// stable lock inodes and exact persistent marker. It performs no mutation and
// establishes no journal authority, installation success or copy disposition.
// It remains valid only while this owner stays open and the caller controls the
// namespace; arbitrary filesystem writers are outside the ownership protocol.
func (m *BrainMaintenance) ValidateFor(path string, expected MaintenanceIdentity) error {
	if m == nil {
		return ErrMaintenanceIdentity
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	raw, err := maintenanceBytes(expected)
	if err != nil || expected != m.identity || m.rootIdentity == nil || m.rootOwner == nil {
		return ErrMaintenanceIdentity
	}
	if err = m.checkLocked(raw); err != nil {
		return err
	}
	root, before, err := openMaintenanceRoot(path, m.state)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if !os.SameFile(before, m.rootIdentity) {
		return ErrMaintenanceIdentity
	}
	state, err := root.OpenRoot(".serenity")
	if err != nil {
		return ErrMaintenanceIdentity
	}
	defer func() { _ = state.Close() }()
	for name, retained := range map[string]*os.File{"writer.lock": m.owner, canonicalViewLockName: m.view} {
		current, statErr := state.Lstat(name)
		retainedInfo, retainedErr := retained.Stat()
		if statErr != nil || retainedErr != nil || !safeCanonicalViewFile(current) || !safeCanonicalViewFile(retainedInfo) || !os.SameFile(current, retainedInfo) {
			return ErrMaintenanceIdentity
		}
	}
	marker, err := state.Lstat(maintenanceMarkerName)
	if err != nil || !os.SameFile(marker, m.marker) {
		return ErrMaintenanceIdentity
	}
	if err = m.checkLocked(raw); err != nil {
		return err
	}
	currentRoot, rootErr := os.Lstat(path)
	currentState, stateErr := root.Lstat(".serenity")
	owned, ownedErr := m.state.Lstat(".")
	if rootErr != nil || stateErr != nil || ownedErr != nil || !safeMaintenanceDirectory(currentRoot) || !safeMaintenanceDirectory(currentState) || !os.SameFile(m.rootIdentity, currentRoot) || !os.SameFile(owned, currentState) {
		return ErrMaintenanceIdentity
	}
	return nil
}

// Pin the original brain directory before any marker publication/adoption.
// Matching locks cannot authorize a different brain after moving .serenity.
func (m *BrainMaintenance) pinRoot(path string) error {
	root, info, err := openMaintenanceRoot(path, m.state)
	if err != nil {
		return err
	}
	m.rootOwner = root
	m.rootIdentity = info
	return nil
}

func openMaintenanceRoot(path string, ownedState *os.Root) (*os.Root, os.FileInfo, error) {
	before, err := os.Lstat(path)
	if err != nil || !safeMaintenanceDirectory(before) || ownedState == nil {
		return nil, nil, ErrMaintenanceIdentity
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, nil, ErrMaintenanceIdentity
	}
	after, err := root.Lstat(".")
	runtime, stateErr := root.Lstat(".serenity")
	owned, ownedErr := ownedState.Lstat(".")
	current, currentErr := os.Lstat(path)
	if err != nil || stateErr != nil || ownedErr != nil || currentErr != nil || !safeMaintenanceDirectory(after) || !safeMaintenanceDirectory(runtime) || !safeMaintenanceDirectory(current) || !os.SameFile(before, after) || !os.SameFile(before, current) || !os.SameFile(runtime, owned) {
		_ = root.Close()
		return nil, nil, ErrMaintenanceIdentity
	}
	return root, before, nil
}

func safeMaintenanceDirectory(info os.FileInfo) bool {
	if info == nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid()
}
