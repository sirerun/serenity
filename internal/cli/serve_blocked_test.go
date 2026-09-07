package cli

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestMCPProcessBlockedStdoutSIGTERM(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestMCPProcessHelper$")
	cmd.Env = append(os.Environ(), "SERENITY_MCP_PROCESS_TEST=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var waitErr error
	go func() { waitErr = cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		_ = input.Close()
		_ = output.Close()
		_ = cmd.Process.Kill()
		<-done
	})
	if _, err := io.WriteString(input, `{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(output)
	if !scanner.Scan() {
		t.Fatal("missing readiness ping")
	}
	// Its response exceeds a pipe buffer. Keep stdout unread while signalling.
	if _, err := io.WriteString(input, `{"jsonrpc":"2.0","id":"`+strings.Repeat("x", 256<<10)+`","method":"ping"}`+"\n"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
		if waitErr != nil {
			t.Fatalf("shutdown: %v; stderr: %s", waitErr, stderr.String())
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatal("unread stdout blocked SIGTERM")
	}
}
