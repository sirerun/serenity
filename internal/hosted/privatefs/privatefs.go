//go:build darwin || linux

// Package privatefs provides fail-closed read-only access to small private
// control files and directories.
package privatefs

import (
	"context"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// ValidateDirectory checks an absolute, canonical directory and every parent
// for symlinks, unsafe ownership, and replaceable writable ancestors. The
// leaf must belong to the current effective UID and be private to that UID.
func ValidateDirectory(ctx context.Context, path string) error {
	if ctx == nil {
		return errors.New("privatefs: context required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("privatefs: absolute clean directory required")
	}
	uid := uint32(os.Geteuid())
	for current := path; ; current = filepath.Dir(current) {
		if err := ownershipEnforced(current); err != nil {
			return err
		}
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("privatefs: directory component required")
		}
		owner, ok := fileUID(info)
		if !ok || (owner != uid && owner != 0) {
			return errors.New("privatefs: unsafe directory owner")
		}
		if current == path {
			if owner != uid || info.Mode().Perm()&0077 != 0 {
				return errors.New("privatefs: owner-only directory required")
			}
		} else if info.Mode().Perm()&0022 != 0 && info.Mode()&os.ModeSticky == 0 {
			return errors.New("privatefs: unsafe writable ancestor")
		}
		if current == filepath.Dir(current) {
			break
		}
	}
	return ctx.Err()
}

// ReadFile reads one private regular file by basename. It rejects links,
// ownership changes, non-private modes, and files larger than maxBytes.
func ReadFile(ctx context.Context, directory, basename string, maxBytes int64) (result []byte, resultErr error) {
	if ctx == nil {
		return nil, errors.New("privatefs: context required")
	}
	if maxBytes <= 0 || maxBytes == math.MaxInt64 {
		return nil, errors.New("privatefs: positive size limit required")
	}
	if basename == "" || filepath.Base(basename) != basename || strings.ContainsAny(basename, `/\\`) || basename == "." || basename == ".." {
		return nil, errors.New("privatefs: single basename required")
	}
	if err := ValidateDirectory(ctx, directory); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path := filepath.Join(directory, basename)
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	defer func() {
		if err := file.Close(); err != nil {
			result = nil
			resultErr = errors.Join(resultErr, errors.New("privatefs: file close failed"))
		}
	}()
	var before syscall.Stat_t
	if err := syscall.Fstat(fd, &before); err != nil {
		return nil, err
	}
	if before.Mode&syscall.S_IFMT != syscall.S_IFREG || before.Uid != uint32(os.Geteuid()) || before.Mode&0077 != 0 || before.Mode&0100 != 0 || before.Mode&0400 == 0 || before.Size < 0 || before.Size > maxBytes {
		return nil, errors.New("privatefs: private bounded regular file required")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errors.New("privatefs: file exceeds size limit")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var after syscall.Stat_t
	if err := syscall.Fstat(fd, &after); err != nil {
		return nil, err
	}
	if !sameFileSnapshot(&before, &after) {
		return nil, errors.New("privatefs: file changed during read")
	}
	return data, nil
}
