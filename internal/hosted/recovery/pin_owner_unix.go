//go:build darwin || linux

package recovery

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

func pinOwnerOpenDirectory(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	if err = pinOwnerCheckDirFile(f); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	return f, nil
}

func pinOwnerOpenDirAt(parent *os.File, name string) (*os.File, error) {
	if parent == nil || !pinOwnerSafeName(name) {
		return nil, ErrPinOwnerInvalid
	}
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), name)
	if err = pinOwnerCheckDirFile(f); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	return f, nil
}

func pinOwnerReopenDirectory(dir *os.File) (*os.File, error) {
	if dir == nil {
		return nil, ErrPinOwnerInvalid
	}
	fd, err := unix.Openat(int(dir.Fd()), ".", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), "pin-owner-directory-scan")
	if err = pinOwnerCheckDirFile(f); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	return f, nil
}

func pinOwnerOpenLockAt(root *os.File, name string, create bool) (*os.File, error) {
	if root == nil || !pinOwnerSafeName(name) {
		return nil, ErrPinOwnerInvalid
	}
	flags := unix.O_RDWR | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	fd, err := unix.Openat(int(root.Fd()), name, flags, 0)
	if errors.Is(err, unix.ENOENT) && create {
		fd, err = unix.Openat(int(root.Fd()), name, flags|unix.O_CREAT|unix.O_EXCL, 0600)
		if errors.Is(err, unix.EEXIST) {
			fd, err = unix.Openat(int(root.Fd()), name, flags, 0)
		}
	}
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), name)
	if err = pinOwnerCheckRegularFile(f, 0600); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	return f, nil
}

func pinOwnerOpenRegularAt(dir *os.File, name string, create bool) (*os.File, error) {
	if dir == nil || !pinOwnerSafeName(name) {
		return nil, ErrPinOwnerInvalid
	}
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if create {
		flags = unix.O_WRONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CREAT | unix.O_EXCL
	}
	fd, err := unix.Openat(int(dir.Fd()), name, flags, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), name)
	if err = pinOwnerCheckRegularFile(f, 0600); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	return f, nil
}

func pinOwnerCreateFileAt(dir *os.File, name string) (*os.File, error) {
	return pinOwnerOpenRegularAt(dir, name, true)
}

func pinOwnerMkdirAt(dir *os.File, name string, mode uint32) error {
	if dir == nil || !pinOwnerSafeName(name) || mode != 0700 {
		return ErrPinOwnerInvalid
	}
	return unix.Mkdirat(int(dir.Fd()), name, mode)
}

func pinOwnerLockFile(ctx context.Context, f *os.File) error {
	if ctx == nil || f == nil {
		return ErrPinOwnerInvalid
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return nil
		}
		if err != unix.EWOULDBLOCK && err != unix.EAGAIN {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func pinOwnerUnlockFile(f *os.File) error {
	if f == nil {
		return nil
	}
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}

func pinOwnerFileIdentity(f *os.File) (uint64, uint64, error) {
	if f == nil {
		return 0, 0, ErrPinOwnerInvalid
	}
	var s unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &s); err != nil {
		return 0, 0, err
	}
	return uint64(s.Dev), uint64(s.Ino), nil
}

func pinOwnerFilePathIdentity(dir *os.File, name string) (uint64, uint64, error) {
	if dir == nil || !pinOwnerSafeName(name) {
		return 0, 0, ErrPinOwnerInvalid
	}
	var s unix.Stat_t
	if err := unix.Fstatat(int(dir.Fd()), name, &s, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return 0, 0, err
	}
	if s.Mode&unix.S_IFMT != unix.S_IFREG {
		return 0, 0, ErrPinOwnerCorrupt
	}
	return uint64(s.Dev), uint64(s.Ino), nil
}

func pinOwnerCheckDirFile(f *os.File) error {
	i, err := f.Stat()
	if err != nil {
		return err
	}
	if !i.IsDir() || i.Mode()&os.ModeSymlink != 0 || i.Mode().Perm() != 0700 {
		return ErrPinOwnerUnavailable
	}
	var s unix.Stat_t
	if err = unix.Fstat(int(f.Fd()), &s); err != nil {
		return err
	}
	if s.Uid != uint32(os.Geteuid()) {
		return ErrPinOwnerUnavailable
	}
	return nil
}

func pinOwnerCheckRegularFile(f *os.File, mode os.FileMode) error {
	i, err := f.Stat()
	if err != nil {
		return err
	}
	if !i.Mode().IsRegular() || i.Mode().Perm() != mode {
		return ErrPinOwnerCorrupt
	}
	var s unix.Stat_t
	if err = unix.Fstat(int(f.Fd()), &s); err != nil {
		return err
	}
	if s.Uid != uint32(os.Geteuid()) || s.Nlink != 1 {
		return ErrPinOwnerCorrupt
	}
	return nil
}

func pinOwnerEnsureDirectoryAt(ctx context.Context, parent *os.File, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	child, err := pinOwnerOpenDirAt(parent, name)
	if err == nil {
		return child.Close()
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err = pinOwnerMkdirAt(parent, name, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err = parent.Sync(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	child, err = pinOwnerOpenDirAt(parent, name)
	if err != nil {
		return err
	}
	return child.Close()
}

func pinOwnerAtomicWriteAt(ctx context.Context, dir *os.File, name string, data []byte) (retErr error) {
	if dir == nil || !pinOwnerSafeName(name) || len(data) == 0 || len(data) > pinOwnerMaxRecordBytes {
		return ErrPinOwnerInvalid
	}
	if ctx == nil {
		return ErrPinOwnerInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if existing, err := pinOwnerOpenRegularAt(dir, name, false); err == nil {
		closeErr := existing.Close()
		return errors.Join(ErrPinOwnerConflict, closeErr)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	tempID, err := pinOwnerRandomHex(32)
	if err != nil {
		return err
	}
	tempName := ".pin-owner-tmp-" + tempID
	pinOwnerCrashInject("before-temp-create")
	f, err := pinOwnerCreateFileAt(dir, tempName)
	if err != nil {
		return err
	}
	dev, ino, err := pinOwnerFileIdentity(f)
	if err == nil {
		pinOwnerCrashInject("after-temp-create")
	}
	if err != nil {
		return errors.Join(err, f.Close())
	}
	published := false
	defer func() {
		closeErr := f.Close()
		retErr = errors.Join(retErr, closeErr)
		if !published {
			if d, i, e := pinOwnerFilePathIdentity(dir, tempName); e == nil && d == dev && i == ino {
				retErr = errors.Join(retErr, pinOwnerUnlinkAt(dir, tempName))
			}
		}
	}()
	firstWrite := true
	for len(data) > 0 {
		if err = ctx.Err(); err != nil {
			return err
		}
		chunk := len(data)
		if firstWrite && chunk > 1 {
			chunk = (chunk + 1) / 2
		}
		n, e := f.Write(data[:chunk])
		if e != nil {
			return e
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
		if firstWrite {
			firstWrite = false
			pinOwnerCrashInject("after-partial-write")
		}
	}
	pinOwnerCrashInject("after-complete-write")
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	pinOwnerCrashInject("after-file-fsync")
	if d, i, e := pinOwnerFileIdentity(f); e != nil || d != dev || i != ino {
		return errors.Join(ErrPinOwnerUnavailable, e)
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = pinOwnerRenameNoReplaceAt(dir, tempName, name); err != nil {
		return err
	}
	published = true
	pinOwnerCrashInject("after-publish")
	d, i, err := pinOwnerFilePathIdentity(dir, name)
	if err != nil || d != dev || i != ino {
		return errors.Join(ErrPinOwnerOutcomeUnknown, err)
	}
	if err = dir.Sync(); err != nil {
		return errors.Join(ErrPinOwnerOutcomeUnknown, err)
	}
	pinOwnerCrashInject("after-dir-sync")
	if err = ctx.Err(); err != nil {
		return errors.Join(ErrPinOwnerOutcomeUnknown, err)
	}
	return nil
}

func pinOwnerRenameNoReplaceAt(dir *os.File, oldName, newName string) error {
	if dir == nil || !pinOwnerSafeName(oldName) || !pinOwnerSafeName(newName) {
		return ErrPinOwnerInvalid
	}
	return pinOwnerPlatformRenameNoReplaceAt(dir, oldName, newName)
}

func pinOwnerUnlinkAt(dir *os.File, name string) error {
	if dir == nil || !pinOwnerSafeName(name) {
		return ErrPinOwnerInvalid
	}
	return unix.Unlinkat(int(dir.Fd()), name, 0)
}
func pinOwnerSafeName(s string) bool {
	return s != "" && s != "." && s != ".." && filepath.Base(s) == s && !strings.ContainsRune(s, '/') && !strings.ContainsRune(s, '\\')
}
