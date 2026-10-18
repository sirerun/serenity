//go:build darwin || linux

package episodeinstall

import (
	"bytes"
	"io"
	"os"
	"syscall"
)

func validateInitialInstalledMarker(info os.FileInfo, path string, record initialRecoveryRecord, expected []byte) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	o := record.Original
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || !ownedInfo(info) || st.Nlink != 1 || uint64(st.Dev) != o.Device || uint64(st.Ino) != o.Inode || st.Uid != o.UID || st.Gid != o.GID {
		return ErrUnqualified
	}
	f, err := os.Open(path)
	if err != nil {
		return ErrUnqualified
	}
	opened, statErr := f.Stat()
	raw, readErr := io.ReadAll(io.LimitReader(f, 1025))
	closeErr := f.Close()
	if statErr != nil || !os.SameFile(info, opened) || readErr != nil || closeErr != nil || !bytes.Equal(raw, expected) {
		return ErrUnqualified
	}
	return nil
}
