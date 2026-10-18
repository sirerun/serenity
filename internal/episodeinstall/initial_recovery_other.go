//go:build !darwin && !linux

package episodeinstall

import "os"

func validateInitialInstalledMarker(os.FileInfo, string, initialRecoveryRecord, []byte) error {
	return ErrUnqualified
}
