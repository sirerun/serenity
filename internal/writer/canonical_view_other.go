//go:build !darwin && !linux

package writer

import "os"

func AcquireCanonicalRead(string) (*os.File, error) { return nil, ErrCanonicalViewUnavailable }
func acquireMaintenanceLocks(string, func(*os.Root) error) (*os.File, *os.File, *os.Root, error) {
	return nil, nil, nil, ErrCanonicalViewUnavailable
}
