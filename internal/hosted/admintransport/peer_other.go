//go:build !darwin && !linux

package admintransport

import (
	"errors"
	"net"
	"os"
)

func fileUID(os.FileInfo) (uint32, bool) { return 0, false }
func osPeerUID(*net.UnixConn) (uint32, error) {
	return 0, errors.New("admin transport: peer credentials unsupported")
}
