//go:build !linux && !darwin

package backup

import (
	"context"
	"os"
)

func lockFileContext(context.Context, *os.File, bool) error { return ErrSnapshotLeaseInvalid }
func unlockFile(*os.File) error                             { return ErrSnapshotLeaseInvalid }
func openLockFile(string, bool) (*os.File, error)           { return nil, ErrSnapshotLeaseInvalid }
func openRegularFile(string) (*os.File, error)              { return nil, ErrSnapshotLeaseInvalid }
func fileIdentity(os.FileInfo) (uint64, uint64, error)      { return 0, 0, ErrSnapshotLeaseInvalid }
