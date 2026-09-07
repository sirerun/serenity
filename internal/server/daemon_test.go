package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/cron"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/providers"
)

// fakeClock is a fixed clock -- the same injectable shape internal/cron's
// own tests use, so job bodies never depend on wall-clock skew here
// either.
type fakeClock struct{ at time.Time }

func (f fakeClock) Now() time.Time { return f.at }

// discardLogger keeps test output quiet; daemon.go's structured logging
// itself is exercised by every test below, just not asserted on here --
// job ids/attribution are covered by inspection, not string-matching a
// log format that's free to change.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// manualTicker is a fake Ticker whose channel a test sends on directly,
// so a job loop fires deterministically instead of racing a real timer.
// The channel is unbuffered: fire blocks until the daemon's job-loop
// goroutine has actually received the tick, which is what makes "fire one
// tick" in a test a synchronization point rather than a race.
type manualTicker struct{ ch chan time.Time }

func newManualTicker() *manualTicker        { return &manualTicker{ch: make(chan time.Time)} }
func (m *manualTicker) C() <-chan time.Time { return m.ch }
func (m *manualTicker) Stop()               {}
func (m *manualTicker) fire(at time.Time)   { m.ch <- at }

// waitForFile polls for path to exist, failing the test if it doesn't
// appear within the deadline. Used to synchronize on a Daemon having
// actually acquired its pidfile before a second Daemon probes it.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s to appear", path)
}

// seedExpiredPendingItem opens root's index, creates one pending
// disposition item at start, and closes the index again -- the same
// fixture shape internal/cron's own TestSweepJobAdvancesExpiredDispositionItem
// uses, reused here rather than re-derived so the daemon test proves the
// same real sweep behavior, just reached through Daemon.Run instead of
// cron.Run directly.
func seedExpiredPendingItem(t *testing.T, root string, start time.Time) disposition.Item {
	t.Helper()
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	store := disposition.NewStore(eng)
	item, err := store.Create(context.Background(), disposition.KindReconcile, nil, "", start)
	if err != nil {
		_ = eng.Close()
		t.Fatalf("Create: %v", err)
	}
	if err := eng.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return item
}

func getDispositionItem(t *testing.T, root, id string) disposition.Item {
	t.Helper()
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatalf("OpenIndex (reopen): %v", err)
	}
	defer func() { _ = eng.Close() }()
	got, err := disposition.NewStore(eng).Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return got
}

// TestDaemonFiresTickAndRunsSweep is T4.1's acc line "start on a fixture
// brain, fire one tick, observe sweep side effects": a disposition item
// seeded past the expiry threshold actually transitions to deferred after
// exactly one manually-fired tick, proving the ticker loop really calls
// through to cron.Run/disposition.Sweep rather than just scaffolding.
func TestDaemonFiresTickAndRunsSweep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	root := t.TempDir()
	start := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	item := seedExpiredPendingItem(t, root, start)
	later := start.Add(15 * 24 * time.Hour) // past the 14-day sweep threshold

	ticker := newManualTicker()
	jobDone := make(chan struct{}, 1)
	d, err := NewDaemon(DaemonConfig{
		Root:      root,
		Schedule:  []JobSchedule{{Job: "sweep", Interval: time.Hour}},
		Clock:     fakeClock{at: later},
		NewTicker: func(time.Duration) Ticker { return ticker },
		Logger:    discardLogger(),
		afterJob:  func(string, error, time.Duration) { jobDone <- struct{}{} },
	})
	if err != nil {
		t.Fatalf("NewDaemon: %v", err)
	}

	runDone := make(chan error, 1)
	go func() { runDone <- d.Run(ctx) }()

	ticker.fire(later)
	select {
	case <-jobDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the sweep job to run after firing one tick")
	}

	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Daemon.Run to return after cancel")
	}

	got := getDispositionItem(t, root, item.ID)
	if got.State != disposition.StateDeferred || got.DeferCount != 1 {
		t.Fatalf("after one daemon tick: State=%q DeferCount=%d, want deferred/1", got.State, got.DeferCount)
	}
}

// TestDaemonShutdownWaitsForInFlightJobAndExitsQuickly is T4.1's acc line
// "SIGTERM completes in-flight writer jobs and exits within 2s with no
// partial file": shutdown is requested (ctx canceled, standing in for a
// real SIGTERM) from inside the job itself, strictly before its real
// write runs, via the beforeJob test hook -- proving Run does not race
// the in-flight job but genuinely waits for it, and that the result is
// the item's full transition, not a partial one.
func TestDaemonShutdownWaitsForInFlightJobAndExitsQuickly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	root := t.TempDir()
	start := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	item := seedExpiredPendingItem(t, root, start)
	later := start.Add(15 * 24 * time.Hour)

	ticker := newManualTicker()
	var cancelAt time.Time
	d, err := NewDaemon(DaemonConfig{
		Root:      root,
		Schedule:  []JobSchedule{{Job: "sweep", Interval: time.Hour}},
		Clock:     fakeClock{at: later},
		NewTicker: func(time.Duration) Ticker { return ticker },
		Logger:    discardLogger(),
		beforeJob: func(string) {
			cancelAt = time.Now()
			cancel()
		},
	})
	if err != nil {
		t.Fatalf("NewDaemon: %v", err)
	}
	pidPath := d.cfg.PidPath

	runDone := make(chan error, 1)
	go func() { runDone <- d.Run(ctx) }()

	waitForFile(t, pidPath)
	ticker.fire(later)

	var runErr error
	select {
	case runErr = <-runDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Daemon.Run to return")
	}
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if elapsed := time.Since(cancelAt); elapsed > 2*time.Second {
		t.Fatalf("shutdown took %s after cancellation, want <= 2s", elapsed)
	}

	got := getDispositionItem(t, root, item.ID)
	if got.State != disposition.StateDeferred || got.DeferCount != 1 {
		t.Fatalf("in-flight job left a partial result: State=%q DeferCount=%d, want the full transition deferred/1", got.State, got.DeferCount)
	}
	if _, err := os.Stat(pidPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pidfile still present after graceful shutdown (err=%v), want it removed", err)
	}
}

// TestDaemonSecondInstanceRefusesToStart is T4.1's acc line "a second
// instance refuses to start": a second Daemon pointed at the same brain
// root, while the first is still running, gets ErrAlreadyRunning back
// from Run immediately rather than starting its own ticker loops.
func TestDaemonSecondInstanceRefusesToStart(t *testing.T) {
	root := t.TempDir()
	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()

	d1, err := NewDaemon(DaemonConfig{
		Root:      root,
		Schedule:  []JobSchedule{{Job: "sweep", Interval: time.Hour}},
		NewTicker: func(time.Duration) Ticker { return newManualTicker() },
		Logger:    discardLogger(),
	})
	if err != nil {
		t.Fatalf("NewDaemon (first): %v", err)
	}

	run1Done := make(chan error, 1)
	go func() { run1Done <- d1.Run(ctx1) }()
	waitForFile(t, d1.cfg.PidPath)

	d2, err := NewDaemon(DaemonConfig{
		Root:      root,
		Schedule:  []JobSchedule{{Job: "sweep", Interval: time.Hour}},
		NewTicker: func(time.Duration) Ticker { return newManualTicker() },
		Logger:    discardLogger(),
	})
	if err != nil {
		t.Fatalf("NewDaemon (second): %v", err)
	}

	err = d2.Run(context.Background())
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("second instance Run() = %v, want ErrAlreadyRunning", err)
	}

	cancel1()
	select {
	case err := <-run1Done:
		if err != nil {
			t.Fatalf("first instance Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the first instance to shut down")
	}
}

// TestPidFileAcquireRefusesLiveProcess and
// TestPidFileAcquireOverwritesStalePidfile cover pidFile's two branches
// directly (live vs. stale) without the goroutine/channel machinery the
// Daemon-level tests above need -- unit tests for logic the acc line's
// Daemon-level test above only exercises the live-process half of.

func TestPidFileAcquireRefusesLiveProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serenityd.pid")
	// The test process itself is unquestionably alive.
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		t.Fatal(err)
	}
	pf := pidFile{path: path}
	err := pf.acquire()
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("acquire() over a live pidfile = %v, want ErrAlreadyRunning", err)
	}
}

func TestPidFileAcquireOverwritesStalePidfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serenityd.pid")

	// A pid guaranteed dead: spawn a trivial child and let it exit, then
	// reuse the now-reaped pid as the stale pidfile's content.
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("spawn short-lived helper process: %v", err)
	}
	deadPID := cmd.Process.Pid
	if err := os.WriteFile(path, []byte(strconv.Itoa(deadPID)), 0o644); err != nil {
		t.Fatal(err)
	}

	pf := pidFile{path: path}
	if err := pf.acquire(); err != nil {
		t.Fatalf("acquire() over a stale pidfile = %v, want nil (should overwrite)", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := strconv.Itoa(os.Getpid()); strings.TrimSpace(string(got)) != want {
		t.Fatalf("pidfile = %q after acquire, want our own pid %s", got, want)
	}
	if err := pf.release(); err != nil {
		t.Fatalf("release: %v", err)
	}
}

// TestDefaultScheduleCoversEveryCronJob keeps DefaultSchedule honest
// against internal/cron's own registry (mirrors cron's own
// TestNamesIsSortedAndComplete): every registered job appears exactly
// once with a positive interval, so a new job added to internal/cron
// without a matching schedule entry here fails loudly instead of quietly
// never running under serenityd.
func TestDefaultScheduleCoversEveryCronJob(t *testing.T) {
	sched := DefaultSchedule()
	seen := make(map[string]bool, len(sched))
	for _, s := range sched {
		if s.Interval <= 0 {
			t.Fatalf("job %q has a non-positive interval %s", s.Job, s.Interval)
		}
		if seen[s.Job] {
			t.Fatalf("job %q appears more than once in DefaultSchedule", s.Job)
		}
		seen[s.Job] = true
	}
	for _, name := range cron.Names() {
		if !seen[name] {
			t.Fatalf("DefaultSchedule is missing cron job %q", name)
		}
	}
	if len(seen) != len(cron.Names()) {
		t.Fatalf("DefaultSchedule has %d jobs, want exactly %d (one per cron.Names())", len(seen), len(cron.Names()))
	}
}

func TestNewDaemonRejectsUnknownJob(t *testing.T) {
	_, err := NewDaemon(DaemonConfig{
		Root:     t.TempDir(),
		Schedule: []JobSchedule{{Job: "bogus", Interval: time.Minute}},
	})
	if err == nil {
		t.Fatal("NewDaemon with an unknown job name = nil error, want an error")
	}
}

func TestNewDaemonRequiresRoot(t *testing.T) {
	if _, err := NewDaemon(DaemonConfig{}); err == nil {
		t.Fatal("NewDaemon with empty Root = nil error, want an error")
	}
}
