//go:build darwin || linux

package writer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestHistoryProcessGroupCancellationStopsDescendants(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	lateFile := filepath.Join(dir, "late-write")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 5 & echo $! > \"$1\"; (sleep 0.5; echo late > \"$2\") & wait", "history-test", pidFile, lateFile)
	if err := configureHistoryProcessGroup(cmd); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(pidFile); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			_ = cmd.Wait()
			t.Fatal("child process did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		cancel()
		_ = cmd.Wait()
		t.Fatal(err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(string(pidBytes)))
	if err != nil || childPID <= 0 {
		cancel()
		_ = cmd.Wait()
		t.Fatalf("invalid child pid %q: %v", pidBytes, err)
	}
	cancel()
	if err := cmd.Wait(); err == nil {
		t.Fatal("canceled process group unexpectedly succeeded")
	}
	time.Sleep(700 * time.Millisecond)
	if _, err := os.Stat(lateFile); !os.IsNotExist(err) {
		t.Fatalf("descendant performed a late write after cancellation: %v", err)
	}
}
