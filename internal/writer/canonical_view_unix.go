//go:build darwin || linux

package writer

import (
	"errors"
	"os"
	"syscall"
)

// AcquireCanonicalRead pins the permanent shared view lock without creating
// files or directories. This excludes maintenance, not ordinary canonical
// writes; callers retain their existing writer/queue snapshot boundaries.
// Legacy brains lacking a qualified lock must first be opened by AcquireBrain.
func AcquireCanonicalRead(path string) (*os.File, error) {
	file, state, err := acquireCanonicalView(path, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = state.Close() }()
	if _, err = state.Lstat(maintenanceMarkerName); !errors.Is(err, os.ErrNotExist) {
		return nil, errors.Join(ErrCanonicalViewUnavailable, ErrMaintenancePending, err, file.Close())
	}
	return file, nil
}

// ensureCanonicalView runs only under the existing writer.lock exclusive owner.
// It never replaces or removes the permanent view inode, including on failure.
func ensureCanonicalView(state *os.Root) error {
	file, err := state.OpenFile(canonicalViewLockName, os.O_CREATE|os.O_EXCL|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if errors.Is(err, os.ErrExist) {
		file, err = state.OpenFile(canonicalViewLockName, os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	}
	if err != nil {
		return errors.Join(ErrCanonicalViewUnavailable, err)
	}
	info, err := file.Stat()
	if err != nil || !safeCanonicalViewFile(info) {
		return errors.Join(ErrCanonicalViewUnavailable, err, file.Close())
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	if err = errors.Join(syncErr, closeErr); err != nil {
		return errors.Join(ErrCanonicalViewUnavailable, err)
	}
	if err = maintenanceSync(state); err != nil {
		return errors.Join(ErrCanonicalViewUnavailable, err)
	}
	return nil
}

func safeCanonicalViewFile(info os.FileInfo) bool {
	if info == nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() != 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid() && stat.Nlink == 1
}

// acquireCanonicalView acquires view before writer ownership for maintenance.
// No creation is permitted here: AcquireBrain alone initializes the inode.
func acquireCanonicalView(path string, exclusive bool) (*os.File, *os.Root, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, nil, errors.Join(ErrCanonicalViewUnavailable, err)
	}
	defer func() { _ = root.Close() }()
	info, err := root.Lstat(".serenity")
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0022 != 0 {
		return nil, nil, errors.Join(ErrCanonicalViewUnavailable, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != os.Geteuid() {
		return nil, nil, ErrCanonicalViewUnavailable
	}
	state, err := root.OpenRoot(".serenity")
	if err != nil {
		return nil, nil, errors.Join(ErrCanonicalViewUnavailable, err)
	}
	success := false
	defer func() {
		if !success {
			_ = state.Close()
		}
	}()
	before, err := state.Lstat(canonicalViewLockName)
	if err != nil || !safeCanonicalViewFile(before) {
		return nil, nil, errors.Join(ErrCanonicalViewUnavailable, err)
	}
	flag := os.O_RDONLY
	if exclusive {
		flag = os.O_RDWR
	}
	file, err := state.OpenFile(canonicalViewLockName, flag|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, nil, errors.Join(ErrCanonicalViewUnavailable, err)
	}
	after, err := file.Stat()
	if err != nil || !safeCanonicalViewFile(after) || !os.SameFile(before, after) {
		return nil, nil, errors.Join(ErrCanonicalViewUnavailable, err, file.Close())
	}
	lock := syscall.LOCK_SH | syscall.LOCK_NB
	if exclusive {
		lock = syscall.LOCK_EX | syscall.LOCK_NB
	}
	if err = syscall.Flock(int(file.Fd()), lock); err != nil {
		closeErr := file.Close()
		if exclusive && (errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)) {
			return nil, nil, errors.Join(ErrCanonicalViewUnavailable, ErrBrainOwned, closeErr)
		}
		return nil, nil, errors.Join(ErrCanonicalViewUnavailable, closeErr)
	}
	current, err := state.Lstat(canonicalViewLockName)
	if err != nil || !safeCanonicalViewFile(current) || !os.SameFile(after, current) {
		return nil, nil, errors.Join(ErrCanonicalViewUnavailable, err, file.Close())
	}
	success = true
	return file, state, nil
}

func acquireMaintenanceLocks(path string, syncRoot func(*os.Root) error) (*os.File, *os.File, *os.Root, error) {
	view, viewState, err := acquireCanonicalView(path, true)
	if err != nil {
		return nil, nil, nil, err
	}
	if err = viewState.Close(); err != nil {
		_ = view.Close()
		return nil, nil, nil, err
	}
	owner, state, err := acquireBrainLockWithSync(path, syncRoot)
	if err != nil {
		_ = view.Close()
		return nil, nil, nil, err
	}
	// A changed runtime-directory namespace cannot combine unrelated locks.
	viewInfo, err := view.Stat()
	current, statErr := state.Lstat(canonicalViewLockName)
	if err != nil || statErr != nil || !safeCanonicalViewFile(current) || !os.SameFile(viewInfo, current) {
		return nil, nil, nil, errors.Join(ErrCanonicalViewUnavailable, err, statErr, state.Close(), owner.Close(), view.Close())
	}
	return view, owner, state, nil
}
