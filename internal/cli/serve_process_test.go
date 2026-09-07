package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"
)

// A helper process exercises the actual Cobra entry point over OS pipes. It
// avoids rebuilding the application for every test while retaining real stdin,
// stdout, signal handling, and process shutdown behavior.
func TestMCPProcessHelper(t *testing.T) {
	if os.Getenv("SERENITY_MCP_PROCESS_TEST") != "1" {
		return
	}
	cmd := newRootCmd()
	cmd.SetArgs([]string{"serve", "--stdio"})
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

type mcpProcess struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	frames chan []byte
	done   chan struct{}
	stderr bytes.Buffer
	err    error
	once   sync.Once
}

func startMCPProcess(t *testing.T) *mcpProcess {
	t.Helper()
	p := &mcpProcess{frames: make(chan []byte, 32), done: make(chan struct{})}
	p.cmd = exec.Command(os.Args[0], "-test.run=^TestMCPProcessHelper$")
	p.cmd.Env = append(os.Environ(), "SERENITY_MCP_PROCESS_TEST=1")
	p.cmd.Stderr = &p.stderr
	var err error
	p.in, err = p.cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		scan := bufio.NewScanner(out)
		for scan.Scan() {
			p.frames <- bytes.Clone(scan.Bytes())
		}
		p.err = scan.Err()
		if err := p.cmd.Wait(); p.err == nil {
			p.err = err
		}
		close(p.frames)
		close(p.done)
	}()
	t.Cleanup(func() {
		p.closeInput()
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
			_ = p.cmd.Process.Kill()
			<-p.done
		}
	})
	return p
}
func (p *mcpProcess) closeInput() { p.once.Do(func() { _ = p.in.Close() }) }
func (p *mcpProcess) write(t *testing.T, frame string) {
	t.Helper()
	if _, err := io.WriteString(p.in, frame); err != nil {
		t.Fatal(err)
	}
}
func (p *mcpProcess) read(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	select {
	case line, ok := <-p.frames:
		if !ok {
			<-p.done
			t.Fatalf("MCP process exited: %v; stderr: %s", p.err, p.stderr.String())
		}
		var frame map[string]json.RawMessage
		if err := json.Unmarshal(line, &frame); err != nil {
			t.Fatalf("stdout is not a JSON frame: %q: %v", line, err)
		}
		if string(frame["jsonrpc"]) != `"2.0"` {
			t.Fatalf("invalid protocol frame: %s", line)
		}
		return frame
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for MCP frame")
		return nil
	}
}
func (p *mcpProcess) wait(t *testing.T) {
	t.Helper()
	select {
	case <-p.done:
		if p.err != nil {
			t.Fatalf("MCP process failed: %v; stderr: %s", p.err, p.stderr.String())
		}
		for f := range p.frames {
			t.Errorf("unexpected trailing stdout frame: %s", f)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("MCP process did not exit")
	}
}
func TestMCPProcessFramesAndEOF(t *testing.T) {
	p := startMCPProcess(t)
	p.write(t, `{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2099-01-01","capabilities":{},"clientInfo":{"name":"acceptance","version":"1"}}}`+"\n")
	init := p.read(t)
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(init["result"], &result); err != nil || result.ProtocolVersion != "2025-11-25" {
		t.Fatalf("version negotiation: %s (%v)", init["result"], err)
	}
	p.write(t, `{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n")
	p.write(t, `{"jsonrpc":"2.0","id":9007199254740993,"method":"ping"}`+"\n")
	if got := p.read(t); string(got["id"]) != "9007199254740993" || got["result"] == nil {
		t.Fatalf("integer ID changed: %v", got)
	}
	p.write(t, "{broken\n")
	bad := p.read(t)
	var rpcError struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(bad["error"], &rpcError); err != nil || rpcError.Code != -32700 {
		t.Fatalf("parse error: %v (%v)", bad, err)
	}
	p.write(t, `{"jsonrpc":"2.0","id":"split","method":`)
	p.write(t, "\"ping\"}\r\n")
	if got := p.read(t); string(got["id"]) != `"split"` || got["result"] == nil {
		t.Fatalf("split frame recovery: %v", got)
	}
	p.closeInput()
	p.wait(t)
}
func TestMCPProcessIdleSIGTERM(t *testing.T) {
	p := startMCPProcess(t)
	// Wait for a protocol response so signal handling is installed before SIGTERM.
	p.write(t, `{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n")
	p.read(t)
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	p.wait(t)
}
