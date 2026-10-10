//go:build linux

package runtimebroker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type unitState struct {
	pid    int
	group  string
	active bool
}

func readUnit(ctx context.Context, name string) (unitState, error) {
	if !validUnit(name) {
		return unitState{}, errors.New("broker: invalid unit")
	}
	bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(bounded, "/usr/bin/systemctl", "show", name, "--no-pager", "--property=MainPID,ControlGroup,ActiveState")
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "SYSTEMD_PAGER=cat"}
	output := &boundedOutput{limit: 4096}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	err := cmd.Run()
	out := output.data
	if err != nil {
		return unitState{}, fmt.Errorf("broker: query service unit: %w", err)
	}
	if len(out) > 4096 {
		return unitState{}, errors.New("broker: unit response exceeds limit")
	}
	var u unitState
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok || seen[k] {
			return u, errors.New("broker: malformed unit response")
		}
		seen[k] = true
		switch k {
		case "MainPID":
			u.pid, err = strconv.Atoi(v)
			if err != nil {
				return u, err
			}
		case "ControlGroup":
			u.group = v
		case "ActiveState":
			u.active = v == "active"
		default:
			return u, errors.New("broker: unknown unit property")
		}
	}
	if len(seen) != 3 || !u.active || u.pid < 1 || !strings.HasPrefix(u.group, "/") {
		return u, errors.New("broker: service unit inactive")
	}
	return u, nil
}

type peer struct {
	pid        int
	uid        uint32
	pidfd      int
	proc       *os.File
	start      string
	lifetime   string
	digest     string
	executable os.FileInfo
	unit       string
	group      string
	lookup     func(context.Context, string) (unitState, error)
}

func peerCredentials(c *net.UnixConn) (*unix.Ucred, error) {
	raw, err := c.SyscallConn()
	if err != nil {
		return nil, err
	}
	var cred *unix.Ucred
	var inner error
	err = raw.Control(func(fd uintptr) { cred, inner = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) })
	return cred, errors.Join(err, inner)
}
func bindPeer(ctx context.Context, c *net.UnixConn, uid uint32, unit, expected string) (*peer, error) {
	return bindPeerWithUnit(ctx, c, uid, unit, expected, readUnit)
}
func bindPeerWithUnit(ctx context.Context, c *net.UnixConn, uid uint32, unit, expected string, lookup func(context.Context, string) (unitState, error)) (*peer, error) {
	cred, err := peerCredentials(c)
	if err != nil || cred == nil || cred.Uid != uid {
		return nil, errors.New("broker: unauthorized Unix peer")
	}
	u, err := lookup(ctx, unit)
	if err != nil || !u.active || u.pid != int(cred.Pid) {
		return nil, errors.New("broker: peer is not managed service MainPID")
	}
	p := &peer{pid: int(cred.Pid), uid: cred.Uid, pidfd: -1, unit: unit, group: u.group, lookup: lookup}
	p.pidfd, err = unix.PidfdOpen(p.pid, 0)
	if err != nil {
		return nil, fmt.Errorf("broker: bind peer lifetime: %w", err)
	}
	p.proc, err = os.Open(fmt.Sprintf("/proc/%d", p.pid))
	if err != nil {
		return nil, errors.Join(err, p.Close())
	}
	p.start, err = p.read("stat", 4096)
	if err == nil {
		p.start, err = processStart(p.start)
		if err == nil {
			boot, e := os.ReadFile("/proc/sys/kernel/random/boot_id")
			if e != nil {
				err = e
			} else {
				p.lifetime = fmt.Sprintf("%s:%d:%s", strings.TrimSpace(string(boot)), p.pid, p.start)
			}
		}
	}
	if err == nil {
		err = p.checkGroup()
	}
	if err == nil {
		p.digest, p.executable, err = p.measure(ctx)
	}
	if err == nil && p.digest != expected {
		err = errors.New("broker: executable is not approved release")
	}
	if err == nil {
		err = p.Check(ctx)
	}
	if err != nil {
		return nil, errors.Join(err, p.Close())
	}
	return p, nil
}
func processStart(stat string) (string, error) {
	i := strings.LastIndex(stat, ") ")
	if i < 1 {
		return "", errors.New("broker: malformed process stat")
	}
	f := strings.Fields(stat[i+2:])
	if len(f) < 20 {
		return "", errors.New("broker: incomplete process stat")
	}
	if _, err := strconv.ParseUint(f[19], 10, 64); err != nil {
		return "", err
	}
	return f[19], nil
}
func (p *peer) read(name string, max int64) (string, error) {
	fd, err := unix.Openat(int(p.proc.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return "", err
	}
	f := os.NewFile(uintptr(fd), name)
	data, readErr := io.ReadAll(io.LimitReader(f, max+1))
	closeErr := f.Close()
	if int64(len(data)) > max {
		return "", errors.Join(errors.New("broker: proc data exceeds limit"), closeErr)
	}
	return string(data), errors.Join(readErr, closeErr)
}
func (p *peer) checkGroup() error {
	body, err := p.read("cgroup", 64*1024)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) == 3 && parts[0] == "0" && parts[1] == "" && parts[2] == p.group {
			return nil
		}
	}
	return errors.New("broker: managed unified cgroup mismatch")
}
func trustedPath(path string, regular bool) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return errors.New("broker: noncanonical root path")
	}
	for cur := path; ; cur = filepath.Dir(cur) {
		s, err := os.Lstat(cur)
		if err != nil {
			return err
		}
		// os.FileInfo uses syscall.Stat_t, so use Lstat directly for portable unix ownership.
		var x unix.Stat_t
		if err := unix.Lstat(cur, &x); err != nil {
			return err
		}
		if x.Uid != 0 || s.Mode()&os.ModeSymlink != 0 || s.Mode().Perm()&0022 != 0 {
			return errors.New("broker: root-owned immutable path required")
		}
		if cur == path && regular {
			if !s.Mode().IsRegular() {
				return errors.New("broker: regular executable required")
			}
		} else if !s.IsDir() {
			return errors.New("broker: directory required")
		}
		if cur == "/" {
			break
		}
	}
	return nil
}
func (p *peer) measure(ctx context.Context) (digest string, info os.FileInfo, resultErr error) {
	path, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d/exe", p.proc.Fd()))
	if err != nil {
		return "", nil, err
	}
	if err = trustedPath(path, true); err != nil {
		return "", nil, err
	}
	fd, err := unix.Openat(int(p.proc.Fd()), "exe", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return "", nil, err
	}
	f := os.NewFile(uintptr(fd), "peer-executable")
	defer func() { resultErr = errors.Join(resultErr, f.Close()) }()
	before, err := f.Stat()
	if err != nil {
		return "", nil, err
	}
	if before.Size() < 1 || before.Size() > 256*1024*1024 {
		return "", nil, errors.New("broker: executable size out of bounds")
	}
	h := sha256.New()
	buf := make([]byte, 64*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		n, e := f.Read(buf)
		if n > 0 {
			total += int64(n)
			if total > 256*1024*1024 {
				return "", nil, errors.New("broker: executable grew beyond limit")
			}
			if _, err := h.Write(buf[:n]); err != nil {
				return "", nil, err
			}
		}
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", nil, e
		}
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return "", nil, errors.New("broker: executable changed during measurement")
	}
	return hex.EncodeToString(h.Sum(nil)), before, nil
}
func (p *peer) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.checkLifetime(); err != nil {
		return err
	}
	u, err := p.lookup(ctx, p.unit)
	if err != nil || !u.active || u.pid != p.pid || u.group != p.group {
		return errors.New("broker: managed unit changed")
	}
	if err = p.checkGroup(); err != nil {
		return err
	}
	current, err := os.Stat(fmt.Sprintf("/proc/self/fd/%d/exe", p.proc.Fd()))
	if err != nil || !os.SameFile(p.executable, current) || current.Size() != p.executable.Size() || !current.ModTime().Equal(p.executable.ModTime()) {
		return errors.New("broker: executable changed")
	}
	return nil
}
func (p *peer) Close() error {
	var es []error
	if p.proc != nil {
		es = append(es, p.proc.Close())
		p.proc = nil
	}
	if p.pidfd >= 0 {
		es = append(es, unix.Close(p.pidfd))
		p.pidfd = -1
	}
	return errors.Join(es...)
}

func (p *peer) checkLifetime() error {
	fds := []unix.PollFd{{Fd: int32(p.pidfd), Events: unix.POLLIN}}
	n, err := unix.Poll(fds, 0)
	if err != nil || n != 0 {
		return errors.New("broker: peer lifetime ended")
	}
	stat, err := p.read("stat", 4096)
	if err != nil {
		return err
	}
	start, err := processStart(stat)
	if err != nil || start != p.start {
		return errors.New("broker: peer lifetime changed")
	}

	return nil
}

type boundedOutput struct {
	data  []byte
	limit int
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-len(b.data) {
		return 0, errors.New("broker: unit response exceeds limit")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
