//go:build !darwin && !linux

package backup

import (
	"errors"
	"os"
)

var errSecureSnapshotOpenUnsupported = errors.New("hosted/backup: secure snapshot file opens are unsupported on this platform")

func openSnapshotRegular(_ *os.Root, _ string) (*os.File, error) {
	return nil, errSecureSnapshotOpenUnsupported
}
