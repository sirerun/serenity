//go:build darwin || linux

package writer

import (
	"errors"
	"os"
	"syscall"
)

func safeMaintenanceMarker(info os.FileInfo) bool {
	if info == nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Geteuid() && st.Nlink == 1
}

func initialMaintenanceObservation(info os.FileInfo) (InitialMaintenanceObservation, error) {
	if !safeMaintenanceMarker(info) {
		return InitialMaintenanceObservation{}, ErrMaintenancePending
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return InitialMaintenanceObservation{}, ErrMaintenancePending
	}
	return InitialMaintenanceObservation{Present: true, Device: uint64(st.Dev), Inode: uint64(st.Ino), UID: uint32(st.Uid), GID: uint32(st.Gid), Mode: uint32(info.Mode().Perm()), Links: uint64(st.Nlink), Bytes: info.Size()}, nil
}

func initialMaintenanceAppend(state *os.Root, name string) (*os.File, error) {
	before, err := state.Lstat(name)
	if err != nil || !safeMaintenanceMarker(before) {
		return nil, ErrMaintenancePending
	}
	file, err := state.OpenFile(name, os.O_WRONLY|os.O_APPEND|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	after, err := file.Stat()
	if err != nil || !safeMaintenanceMarker(after) || !os.SameFile(before, after) {
		return nil, errors.Join(ErrMaintenanceIdentity, err, file.Close())
	}
	return file, nil
}

func validateInitialNamespace(m *BrainMaintenance, path string) error {
	if m == nil || m.closed || m.view == nil || m.owner == nil || m.state == nil || m.rootOwner == nil || m.rootIdentity == nil {
		return ErrMaintenancePending
	}
	root, info, err := openMaintenanceRoot(path, m.state)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if !os.SameFile(info, m.rootIdentity) {
		return ErrMaintenanceIdentity
	}
	retained, err := m.rootOwner.Lstat(".")
	if err != nil || !os.SameFile(retained, m.rootIdentity) {
		return ErrMaintenanceIdentity
	}
	for name, file := range map[string]*os.File{"writer.lock": m.owner, canonicalViewLockName: m.view} {
		actual, statErr := m.state.Lstat(name)
		held, heldErr := file.Stat()
		if statErr != nil || heldErr != nil || !safeCanonicalViewFile(actual) || !safeCanonicalViewFile(held) || !os.SameFile(actual, held) {
			return ErrMaintenanceIdentity
		}
	}
	current, err := os.Lstat(path)
	if err != nil || !safeMaintenanceDirectory(current) || !os.SameFile(current, m.rootIdentity) {
		return ErrMaintenanceIdentity
	}
	return nil
}
