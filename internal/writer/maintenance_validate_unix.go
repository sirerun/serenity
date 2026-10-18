//go:build darwin || linux

package writer

import (
	"os"
	"syscall"
)

// ValidateFor binds an open maintenance owner to the supplied canonical root,
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
	if err != nil || expected != m.identity {
		return ErrMaintenanceIdentity
	}
	if err = m.checkLocked(raw); err != nil {
		return err
	}
	before, err := os.Lstat(path)
	if err != nil || !safeMaintenanceDirectory(before) {
		return ErrMaintenanceIdentity
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return ErrMaintenanceIdentity
	}
	defer func() { _ = root.Close() }()
	after, err := root.Lstat(".")
	if err != nil || !os.SameFile(before, after) || !safeMaintenanceDirectory(after) {
		return ErrMaintenanceIdentity
	}
	runtime, err := root.Lstat(".serenity")
	owned, ownedErr := m.state.Lstat(".")
	if err != nil || ownedErr != nil || !safeMaintenanceDirectory(runtime) || !os.SameFile(runtime, owned) {
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
	// Recheck through the retained directory after resolving the supplied root.
	// Replacement cannot be excused by matching marker contents alone.
	if err = m.checkLocked(raw); err != nil {
		return err
	}
	currentRoot, rootErr := os.Lstat(path)
	currentState, stateErr := root.Lstat(".serenity")
	if rootErr != nil || stateErr != nil || !safeMaintenanceDirectory(currentRoot) || !safeMaintenanceDirectory(currentState) || !os.SameFile(before, currentRoot) || !os.SameFile(owned, currentState) {
		return ErrMaintenanceIdentity
	}
	return nil
}

func safeMaintenanceDirectory(info os.FileInfo) bool {
	if info == nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid()
}
