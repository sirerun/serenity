package backup

import (
	"errors"
	"os"
)

// ErrAtomicPublicationUnavailable means the platform cannot guarantee a
// destination appears only when it was absent at the atomic rename itself.
var ErrAtomicPublicationUnavailable = errors.New("hosted/backup: atomic no-replace publication unavailable")

func publicationRenameError(oldPath, newPath string, err error) error {
	if err == nil {
		return nil
	}
	return &os.LinkError{Op: "rename-noreplace", Old: oldPath, New: newPath, Err: err}
}
