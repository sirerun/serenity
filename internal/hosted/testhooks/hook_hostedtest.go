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
//	release <phase>\n       unblock every process paused at <phase>
//
// Status-pipe wire protocol, written by this process:
//
//	paused <phase>\n        this process is blocked at <phase>
//
// Concurrency: exactly one goroutine ever reads the arm pipe — the goroutine
// that parses the initial "arm"/"start" lines inside ensureOpen, which then
// becomes the dispatch loop for "release" lines for the lifetime of the
// process. At() never reads the pipe itself; it only waits on a per-phase
// channel that the dispatch loop closes when the matching "release" line
// arrives. This replaces a prior design where every paused At() call read
// the shared *bufio.Reader directly: that let concurrent pauses race on the
// same reader (a documented data race — bufio.Reader is not safe for
// concurrent use) and let one goroutine consume and discard a "release"
// line addressed to a different, concurrently paused phase, hanging it
// forever (a lost-wakeup bug, independent of the race).
//
// EOF/error behavior: if the arm pipe hits EOF or a read error — whether
// during the initial parse or afterward in the dispatch loop — every phase
// currently paused, and any phase that pauses later, unblocks immediately.
// A harness that closes its write end (deliberately, or because it crashed)
// is treated the same as "release everything, right now," never as "wait
// forever." This is implemented by closing a single shared done channel
// exactly once; closing an already-closed channel would panic, so it is
// guarded by sync.Once, not by re-checking a boolean under the mutex.
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

type hookState struct {
	mu       sync.Mutex
	armed    map[string]instruction
	release  map[string]chan struct{} // pre-created for every phase armed "pause"
	released map[string]bool          // guards against double-close on a duplicate release line
	done     chan struct{}            // closed once via doneOnce when the arm pipe ends
	doneOnce sync.Once
	status   *os.File
}

func (s *hookState) closeDone() {
	s.doneOnce.Do(func() { close(s.done) })
}

var (
	setupOnce sync.Once
	st        *hookState
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

// ensureOpen parses every "arm"/"start" line synchronously (so every phase's
// release channel exists before any At() call can look one up), then hands
// the arm pipe off to a single background dispatch goroutine for the rest
// of the process's life. Safe to call from multiple goroutines: sync.Once
// guarantees the parse and the dispatch-goroutine start happen exactly once
// and that every caller sees the fully-initialized state afterward.
func ensureOpen() *hookState {
	setupOnce.Do(func() {
		s := &hookState{
			armed:    map[string]instruction{},
			release:  map[string]chan struct{}{},
			released: map[string]bool{},
			done:     make(chan struct{}),
		}
		st = s
		armRaw := openFD(armFDEnv)
		s.status = openFD(statusFDEnv)
		if armRaw == nil {
			s.closeDone() // no real pipe inherited: every checkpoint is inert immediately
			return
		}
		reader := bufio.NewReader(armRaw)
		for {
			line, err := reader.ReadString('\n')
			fields := strings.Fields(strings.TrimSpace(line))
			switch {
			case len(fields) == 3 && fields[0] == "arm" && fields[2] == "crash":
				s.armed[fields[1]] = instructionCrash
			case len(fields) == 3 && fields[0] == "arm" && fields[2] == "pause":
				s.armed[fields[1]] = instructionPause
				s.release[fields[1]] = make(chan struct{})
			case len(fields) == 1 && fields[0] == "start":
				go s.dispatch(reader)
				return
			}
			if err != nil {
				s.closeDone()
				return
			}
		}
	})
	return st
}

// dispatch is the sole reader of the arm pipe once the process has started.
// It only ever routes a "release <phase>" line to that phase's own channel;
// an unrecognized phase (never armed "pause") is silently ignored — there is
// no channel to signal and nothing waiting on one.
func (s *hookState) dispatch(reader *bufio.Reader) {
	for {
		line, err := reader.ReadString('\n')
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 2 && fields[0] == "release" {
			s.releasePhase(fields[1])
		}
		if err != nil {
			s.closeDone()
			return
		}
	}
}

func (s *hookState) releasePhase(phase string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.release[phase]
	if !ok || s.released[phase] {
		return
	}
	s.released[phase] = true
	close(ch)
}

func at(phase string) {
	s := ensureOpen()
	s.mu.Lock()
	instr := s.armed[phase]
	ch := s.release[phase]
	s.mu.Unlock()
	switch instr {
	case instructionCrash:
		// Deliberate fault barrier: simulate the process dying at exactly
		// this named checkpoint. Only reachable in a hostedtest-tagged
		// binary that was started with a real inherited arm pipe and an
		// explicit "arm <phase> crash" instruction.
		os.Exit(137)
	case instructionPause:
		if s.status != nil {
			s.mu.Lock()
			_, _ = s.status.WriteString("paused " + phase + "\n")
			s.mu.Unlock()
		}
		select {
		case <-ch:
		case <-s.done:
		}
	}
}
