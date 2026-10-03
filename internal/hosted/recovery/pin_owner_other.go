//go:build !darwin && !linux

package recovery

import (
	"context"
	"os"
)

func pinOwnerOpenDirectory(string) (*os.File, error)              { return nil, ErrPinOwnerUnavailable }
func pinOwnerOpenDirAt(*os.File, string) (*os.File, error)        { return nil, ErrPinOwnerUnavailable }
func pinOwnerReopenDirectory(*os.File) (*os.File, error)          { return nil, ErrPinOwnerUnavailable }
func pinOwnerOpenLockAt(*os.File, string, bool) (*os.File, error) { return nil, ErrPinOwnerUnavailable }
func pinOwnerOpenRegularAt(*os.File, string, bool) (*os.File, error) {
	return nil, ErrPinOwnerUnavailable
}
func pinOwnerCreateFileAt(*os.File, string) (*os.File, error) { return nil, ErrPinOwnerUnavailable }
func pinOwnerMkdirAt(*os.File, string, uint32) error          { return ErrPinOwnerUnavailable }
func pinOwnerLockFile(context.Context, *os.File) error        { return ErrPinOwnerUnavailable }
func pinOwnerUnlockFile(*os.File) error                       { return ErrPinOwnerUnavailable }
func pinOwnerFileIdentity(*os.File) (uint64, uint64, error)   { return 0, 0, ErrPinOwnerUnavailable }
func pinOwnerFilePathIdentity(*os.File, string) (uint64, uint64, error) {
	return 0, 0, ErrPinOwnerUnavailable
}
func pinOwnerCheckDirFile(*os.File) error                  { return ErrPinOwnerUnavailable }
func pinOwnerCheckRegularFile(*os.File, os.FileMode) error { return ErrPinOwnerUnavailable }
func pinOwnerEnsureDirectoryAt(context.Context, *os.File, string) error {
	return ErrPinOwnerUnavailable
}
func pinOwnerAtomicWriteAt(context.Context, *os.File, string, []byte) error {
	return ErrPinOwnerUnavailable
}
func pinOwnerRenameNoReplaceAt(*os.File, string, string) error         { return ErrPinOwnerUnavailable }
func pinOwnerUnlinkAt(*os.File, string) error                          { return ErrPinOwnerUnavailable }
func pinOwnerPlatformRenameNoReplaceAt(*os.File, string, string) error { return ErrPinOwnerUnavailable }
