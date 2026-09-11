//go:build darwin || linux

package writer

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestBrainOwnershipAliasesAndRelease(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "brain")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(parent, "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	owner, err := AcquireBrain(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{root, alias, filepath.Join(root, ".")} {
		second, err := AcquireBrain(path)
		if second != nil {
			_ = second.Close()
		}
		if !errors.Is(err, ErrBrainOwned) {
			t.Fatalf("alias admitted another owner: %v", err)
		}
	}
	other, err := AcquireBrain(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	recovered, err := AcquireBrain(alias)
	if err != nil {
		t.Fatal(err)
	}
	if err := recovered.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".serenity", "writer.lock")); err != nil {
		t.Fatal("release removed stable lock inode")
	}
}

func TestBrainOwnershipProcessDeathReleases(t *testing.T) {
	if root := os.Getenv("SERENITY_OWNERSHIP_HELPER"); root != "" {
		owner, err := AcquireBrain(root)
		if err != nil {
			os.Exit(2)
		}
		_, _ = fmt.Fprintln(os.Stdout, "held")
		_, _ = io.Copy(io.Discard, os.Stdin)
		runtime.KeepAlive(owner)
		os.Exit(0)
	}
	root := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestBrainOwnershipProcessDeathReleases$")
	cmd.Env = append(os.Environ(), "SERENITY_OWNERSHIP_HELPER="+root)
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close() }()
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	ready := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(output).ReadString('\n'); ready <- line }()
	select {
	case line := <-ready:
		if line != "held\n" {
			t.Fatal("helper did not hold ownership")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("helper readiness timed out")
	}
	contender, err := AcquireBrain(root)
	if contender != nil {
		_ = contender.Close()
	}
	if !errors.Is(err, ErrBrainOwned) {
		t.Fatal("live subprocess did not exclude contender", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("helper did not terminate by signal")
	}
	owner, err := AcquireBrain(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBrainOwnershipRejectsSymlinkLock(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".serenity"), 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "unrelated")
	if err := os.WriteFile(target, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, ".serenity", "writer.lock")); err != nil {
		t.Fatal(err)
	}
	if owner, err := AcquireBrain(root); err == nil {
		_ = owner.Close()
		t.Fatal("symlink lock accepted")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "preserve" {
		t.Fatal("unrelated file changed")
	}
}
