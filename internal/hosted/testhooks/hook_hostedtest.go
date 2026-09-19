//go:build hostedtest

package testhooks

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Transport: two anonymous pipes the harness creates and inherits into this
// process via exec.Cmd.ExtraFiles, named only by the fd number each env var
// carries. Never an HTTP endpoint, never a listener, never a directly
// interpreted production switch: an env var with a stray or unmapped fd
// number simply fails to open below and every hook stays inert.
//
//	SERENITY_HOSTED_TESTHOOKS_ARM_FD    fd the harness writes instructions to;
//	                                    this process reads it.
//	SERENITY_HOSTED_TESTHOOKS_STATUS_FD fd this process writes status lines
//	                                    to; the harness reads it.
//
// Arm-pipe wire protocol, read once before the first checkpoint fires:
//
//	arm <phase> crash\n     mark <phase> to exit(137) when reached
//	arm <phase> pause\n     mark <phase> to block until released
//	start\n                 all arm lines sent; this process may proceed
//
// After "start", additional lines may arrive:
//
//	release <phase>\n       unblock a process paused at <phase>
//
// Status-pipe wire protocol, written by this process:
//
//	paused <phase>\n        this process is blocked at <phase>
const (
	armFDEnv    = "SERENITY_HOSTED_TESTHOOKS_ARM_FD"
	statusFDEnv = "SERENITY_HOSTED_TESTHOOKS_STATUS_FD"
)

type instruction int

const (
	instructionNone instruction = iota
	instructionPause
	instructionCrash
)

var (
	once   sync.Once
	mu     sync.Mutex
	armed  map[string]instruction
	armIn  *bufio.Reader
	armRaw *os.File
	status *os.File
)

func openFD(env string) *os.File {
	raw := os.Getenv(env)
	if raw == "" {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return nil
	}
	return os.NewFile(uintptr(n), env)
}

func ensureOpen() {
	once.Do(func() {
		armed = map[string]instruction{}
		armRaw = openFD(armFDEnv)
		status = openFD(statusFDEnv)
		if armRaw == nil {
			return
		}
		armIn = bufio.NewReader(armRaw)
		for {
			line, err := armIn.ReadString('\n')
			fields := strings.Fields(strings.TrimSpace(line))
			switch {
			case len(fields) == 3 && fields[0] == "arm" && fields[2] == "crash":
				armed[fields[1]] = instructionCrash
			case len(fields) == 3 && fields[0] == "arm" && fields[2] == "pause":
				armed[fields[1]] = instructionPause
			case len(fields) == 1 && fields[0] == "start":
				return
			}
			if err != nil {
				return
			}
		}
	})
}

func at(phase string) {
	mu.Lock()
	ensureOpen()
	instr := armed[phase]
	mu.Unlock()
	switch instr {
	case instructionCrash:
		// Deliberate fault barrier: simulate the process dying at exactly
		// this named checkpoint. Only reachable in a hostedtest-tagged
		// binary that was started with a real inherited arm pipe and an
		// explicit "arm <phase> crash" instruction.
		os.Exit(137)
	case instructionPause:
		if status != nil {
			_, _ = status.WriteString("paused " + phase + "\n")
		}
		for {
			line, err := armIn.ReadString('\n')
			if strings.TrimSpace(line) == "release "+phase {
				return
			}
			if err != nil {
				return
			}
		}
	}
}
