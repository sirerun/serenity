//go:build linux

package runtimebroker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var peerCounter atomic.Uint64

func kernelPeer(t *testing.T) (*net.UnixConn, *exec.Cmd, string, string) {
	t.Helper()
	path := fmt.Sprintf("@serenity-peer-%d-%d", os.Getpid(), peerCounter.Add(1))
	l, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	cmd := exec.Command("/usr/bin/python3", "-c", "import socket,sys; s=socket.socket(socket.AF_UNIX); s.connect(chr(0)+sys.argv[1][1:]); s.recv(1)", path)
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	_ = l.SetDeadline(time.Now().Add(5 * time.Second))
	conn, err := l.AcceptUnix()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	body, err := os.ReadFile(fmt.Sprintf("/proc/%d/exe", cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	groups, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	group := ""
	for _, line := range strings.Split(string(groups), "\n") {
		if strings.HasPrefix(line, "0::") {
			group = strings.TrimPrefix(line, "0::")
		}
	}
	if group == "" {
		t.Fatal("unified cgroup required")
	}
	return conn, cmd, hex.EncodeToString(sum[:]), group
}
func TestKernelPeerBindingAndDeadProcess(t *testing.T) {
	conn, cmd, digest, group := kernelPeer(t)
	calls := 0
	lookup := func(context.Context, string) (unitState, error) {
		calls++
		return unitState{pid: cmd.Process.Pid, group: group, active: true}, nil
	}
	p, err := bindPeerWithUnit(context.Background(), conn, uint32(os.Geteuid()), "serenity-hosted.service", digest, lookup)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()
	if calls < 2 {
		t.Fatal("unit not checked around measurement")
	}
	if p.lifetime == "" || p.digest != digest {
		t.Fatal("kernel measurement missing")
	}
	if _, err = conn.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err = cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if p.checkLifetime() == nil {
		t.Fatal("dead process retained authority")
	}
}
func TestKernelPeerRejectsUIDMainPIDAndDigestSubstitution(t *testing.T) {
	conn, cmd, digest, group := kernelPeer(t)
	for _, name := range []string{"wrong-uid", "wrong-mainpid", "wrong-digest", "wrong-cgroup"} {
		t.Run(name, func(t *testing.T) {
			uid := uint32(os.Geteuid())
			pid := cmd.Process.Pid
			expected := digest
			g := group
			switch name {
			case "wrong-uid":
				uid++
			case "wrong-mainpid":
				pid++
			case "wrong-digest":
				expected = strings.Repeat("0", 64)
			case "wrong-cgroup":
				g = "/wrong"
			}
			lookup := func(context.Context, string) (unitState, error) {
				return unitState{pid: pid, group: g, active: true}, nil
			}
			if p, err := bindPeerWithUnit(context.Background(), conn, uid, "serenity-hosted.service", expected, lookup); err == nil {
				_ = p.Close()
				t.Fatal("substituted process accepted")
			}
		})
	}
}
func TestProcessStatHandlesParenthesesInName(t *testing.T) {
	stat := "55 (a ) b) S " + strings.Repeat("0 ", 18) + "987 0"
	start, err := processStart(stat)
	if err != nil || start != "987" {
		t.Fatalf("start %q error %v", start, err)
	}
	if _, err = processStart("broken"); err == nil {
		t.Fatal("malformed process stat accepted")
	}
}
