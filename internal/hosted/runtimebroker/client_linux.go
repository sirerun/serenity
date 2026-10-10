//go:build linux

package runtimebroker

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func dialBroker(ctx context.Context, path string) (net.Conn, error) {
	if ctx == nil || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errors.New("broker: canonical socket required")
	}
	if err := trustedPath(filepath.Dir(path), false); err != nil {
		return nil, err
	}
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return nil, err
	}
	if st.Uid != 0 || st.Mode&unix.S_IFMT != unix.S_IFSOCK || st.Mode&0007 != 0 || st.Mode&0020 == 0 {
		return nil, errors.New("broker: protected root socket required")
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		return nil, err
	}
	u, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, errors.Join(errors.New("broker: Unix connection required"), conn.Close())
	}
	cred, err := peerCredentials(u)
	if err != nil || cred == nil || cred.Uid != 0 {
		return nil, errors.Join(errors.New("broker: root server peer required"), conn.Close())
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return nil, errors.Join(errors.New("broker: socket changed"), conn.Close())
	}
	return conn, nil
}
