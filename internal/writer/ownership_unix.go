//go:build darwin || linux

package writer

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// ErrBrainOwned means another CLI process owns canonical writes to this brain.
var ErrBrainOwned = errors.New("another Serenity writer owns this brain")

// AcquireBrain owns the brain until the returned file is closed or the process
// exits. The stable lock file must never be removed on release. This fences
// cooperating CLI processes on one filesystem, not arbitrary local writers.
func AcquireBrain(path string) (*os.File, error) {
	file, state, err := acquireBrainLock(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = state.Close() }()
	if _, err = state.Lstat(maintenanceMarkerName); !errors.Is(err, os.ErrNotExist) {
		return nil, errors.Join(ErrMaintenancePending, err, file.Close())
	}
	if err = ensureCanonicalView(state); err != nil {
		return nil, errors.Join(err, file.Close())
	}
	return file, nil
}

func acquireBrainLock(path string) (*os.File, *os.Root, error) {
	return acquireBrainLockWithSync(path, maintenanceSync)
}

func acquireBrainLockWithSync(path string, syncRoot func(*os.Root) error) (*os.File, *os.Root, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, nil, fmt.Errorf("writer ownership: open brain: %w", err)
	}
	defer func() { _ = root.Close() }()
	if err := root.Mkdir(".serenity", 0700); err != nil && !os.IsExist(err) {
		return nil, nil, err
	}
	// The runtime directory entry must survive a crash, including when it was
	// created by a prior failed attempt. Sync the brain root on every ownership
	// acquisition before allowing a durable maintenance gate to be relied on.
	if err := syncRoot(root); err != nil {
		return nil, nil, err
	}
	info, err := root.Lstat(".serenity")
	if err != nil {
		return nil, nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, fmt.Errorf("writer ownership: unsafe runtime directory")
	}
	state, err := root.OpenRoot(".serenity")
	if err != nil {
		return nil, nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = state.Close()
		}
	}()
	file, err := state.OpenFile("writer.lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, nil, fmt.Errorf("writer ownership: open lock: %w", err)
	}
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, fmt.Errorf("writer ownership: unsafe lock file")
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, nil, ErrBrainOwned
		}
		return nil, nil, fmt.Errorf("writer ownership: lock: %w", err)
	}
	success = true
	return file, state, nil
}
