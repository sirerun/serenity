//go:build !darwin && !linux

package admintransport

import (
	"errors"
	"net"
)

func osPeerUID(*net.UnixConn) (uint32, error) {
	return 0, errors.New("admin transport: peer credentials unsupported")
}
