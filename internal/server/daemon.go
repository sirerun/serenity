// daemon.go implements serenityd's lifecycle (RFC 0001, ADR 006, plan
// T4.1): a Daemon embeds internal/cron's job functions on an internal
// ticker instead of relying on an external timer (launchd/systemd, per
// docs/operator/scheduling.md), guards against a second instance starting
// against the same brain root, and shuts down gracefully on ctx
// cancellation (SIGTERM in production) by waiting for any in-flight job to
// finish before removing its pidfile and returning.
//
// internal/cron's own package doc already names this design: "In M4,
// serenityd embeds these same job functions on an internal ticker (ADR
// 006); serenity cron remains the manual operation and test entry point."
// Daemon therefore owns no job logic of its own -- every tick calls
// cron.Run(ctx, job, root, clock) unchanged, so a bug fix or new job lands
// in internal/cron once and both entry points pick it up.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sirerun/serenity/internal/cron"
)

// ErrAlreadyRunning is returned by Daemon.Run when the configured pidfile
// names a still-live process -- a second serenityd against the same brain
// root refuses to start rather than racing the first over cron's own
// dirty-tree guard (ADR 006: "the manual documents one timer per brain").
var ErrAlreadyRunning = errors.New("serenityd: another instance is already running")

// Ticker abstracts a periodic tick source so job scheduling is testable
// without real sleeps or wall-clock dependence -- the same injectable
// shape as internal/cron.Clock and internal/connector/file's
// Clock/WithClock (see internal/cron's own doc comment).
type Ticker interface {
	C() <-chan time.Time
	Stop()
}

// realTicker wraps time.Ticker for production use.
type realTicker struct{ t *time.Ticker }

func newRealTicker(d time.Duration) Ticker { return &realTicker{t: time.NewTicker(d)} }
func (r *realTicker) C() <-chan time.Time  { return r.t.C }
func (r *realTicker) Stop()                { r.t.Stop() }

// JobSchedule pairs one internal/cron job name with the interval it fires
// on.
type JobSchedule struct {
	Job      string
	Interval time.Duration
}

// DefaultSchedule mirrors docs/operator/scheduling.md's suggested cadence
// table: sweep and slo every 15 minutes, consolidate nightly, decay and
// revisit weekly. A caller with different cadence needs (or a fixture
// test that wants a single job) passes its own DaemonConfig.Schedule
// instead.
func DefaultSchedule() []JobSchedule {
	return []JobSchedule{
		{Job: "sweep", Interval: 15 * time.Minute},
		{Job: "slo", Interval: 15 * time.Minute},
		{Job: "consolidate", Interval: 24 * time.Hour},
		{Job: "decay", Interval: 7 * 24 * time.Hour},
		{Job: "revisit", Interval: 7 * 24 * time.Hour},
	}
}

// DaemonConfig configures a Daemon. Only Root is required; every other
// field defaults to its production value.
type DaemonConfig struct {
	// Root is the brain repo root, the same root every CLI command takes
	// via -C.
	Root string
	// Schedule lists the jobs to run and their intervals. Defaults to
	// DefaultSchedule().
	Schedule []JobSchedule
	// Clock supplies "now" to every cron.Run call, exactly as it does for
	// `serenity cron`. Defaults to cron.RealClock.
	Clock cron.Clock
	// NewTicker builds the Ticker each job loop reads from. Defaults to a
	// real time.Ticker; tests inject a fake to fire ticks deterministically.
	NewTicker func(d time.Duration) Ticker
	// Logger receives structured startup/shutdown/job-run logs, each job
	// log line carrying a per-run job id. Defaults to slog.Default().
	Logger *slog.Logger
	// PidPath is the pidfile path a second instance checks before
	// starting. Defaults to <Root>/.serenity/serenityd.pid.
	PidPath string

	// beforeJob and afterJob are test-only hooks (unexported: settable
	// only from within this package's own tests) that let a test observe
	// or interleave with a single job run without reaching into
	// production job logic. Production callers never set them.
	beforeJob func(job string)
	afterJob  func(job string, err error, dur time.Duration)
}

// Daemon runs internal/cron's jobs on an internal ticker until its
// context is canceled. Build one with NewDaemon.
type Daemon struct {
	cfg     DaemonConfig
	pidFile pidFile
	wg      sync.WaitGroup
}

// NewDaemon validates cfg, applies defaults, and returns a Daemon ready
// for Run. It does not touch the pidfile or start any goroutine -- that
// happens in Run.
func NewDaemon(cfg DaemonConfig) (*Daemon, error) {
	if cfg.Root == "" {
		return nil, errors.New("serenityd: DaemonConfig.Root is required")
	}
	if cfg.Schedule == nil {
		cfg.Schedule = DefaultSchedule()
	}
	valid := make(map[string]bool, len(cron.Names()))
	for _, n := range cron.Names() {
		valid[n] = true
	}
	for _, s := range cfg.Schedule {
		if !valid[s.Job] {
			return nil, fmt.Errorf("serenityd: unknown job %q in schedule (valid: %s)", s.Job, strings.Join(cron.Names(), ", "))
		}
		if s.Interval <= 0 {
			return nil, fmt.Errorf("serenityd: job %q has a non-positive interval %s", s.Job, s.Interval)
		}
	}
	if cfg.Clock == nil {
		cfg.Clock = cron.RealClock
	}
	if cfg.NewTicker == nil {
		cfg.NewTicker = newRealTicker
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.PidPath == "" {
		cfg.PidPath = filepath.Join(cfg.Root, ".serenity", "serenityd.pid")
	}
	return &Daemon{cfg: cfg, pidFile: pidFile{path: cfg.PidPath}}, nil
}

// Run acquires the pidfile (refusing with ErrAlreadyRunning if another
// live instance holds it), starts one ticker loop per scheduled job, and
// blocks until ctx is done. On shutdown it stops accepting new ticks and
// waits for every in-flight job to finish -- never force-aborts one --
// before removing the pidfile and returning. A graceful shutdown returns
// nil, not ctx.Err(): callers wire SIGTERM to ctx cancellation and treat a
// clean stop as success, not failure.
func (d *Daemon) Run(ctx context.Context) error {
	if err := d.pidFile.acquire(); err != nil {
		return err
	}
	defer func() {
		if err := d.pidFile.release(); err != nil {
			d.cfg.Logger.Warn("serenityd: failed to remove pidfile", "path", d.cfg.PidPath, "err", err)
		}
	}()

	names := make([]string, len(d.cfg.Schedule))
	for i, s := range d.cfg.Schedule {
		names[i] = s.Job
	}
	d.cfg.Logger.Info("serenityd starting", "root", d.cfg.Root, "pid", os.Getpid(), "jobs", strings.Join(names, ","))

	for _, sched := range d.cfg.Schedule {
		d.wg.Add(1)
		go d.runJobLoop(ctx, sched)
	}

	<-ctx.Done()
	d.cfg.Logger.Info("serenityd stopping, waiting for in-flight jobs")
	d.wg.Wait()
	d.cfg.Logger.Info("serenityd stopped")
	return nil
}

// runJobLoop fires job on interval sched.Interval until ctx is done. ctx
// gates only whether the loop waits for another tick, never a job that is
// already running: runOnce deliberately does not receive ctx (see its own
// comment), so a job already in flight when shutdown starts always runs
// to completion on its own terms before the loop rechecks ctx.Done() and
// returns.
func (d *Daemon) runJobLoop(ctx context.Context, sched JobSchedule) {
	defer d.wg.Done()
	ticker := d.cfg.NewTicker(sched.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C():
			d.runOnce(sched.Job, now)
		}
	}
}

var jobSeq atomic.Uint64

// runOnce runs one job to completion, logging its start and outcome with
// a per-run job id so concurrent job logs stay attributable (acc:
// "structured logs with job ids"). tick is the ticker-fired time.Time
// used only to build the job id; "now" for the job's own logic still
// comes from d.cfg.Clock, matching internal/cron's own Clock-injection
// convention.
//
// runOnce deliberately does not take the daemon's shutdown ctx: internal/
// cron's job bodies route their store calls through database/sql's
// *Context methods, so a job started with a context that is later
// canceled would abort mid-write the moment shutdown begins, not finish
// it -- the opposite of the acc bar this satisfies ("SIGTERM completes
// in-flight writer jobs", not "SIGTERM aborts them cleanly"). Running on
// context.Background() instead means the daemon's ctx governs only
// whether runJobLoop waits for another tick, never a job already
// in flight.
func (d *Daemon) runOnce(job string, tick time.Time) {
	jobID := fmt.Sprintf("%s-%d-%d", job, tick.UnixNano(), jobSeq.Add(1))
	log := d.cfg.Logger.With("job", job, "job_id", jobID)

	if d.cfg.beforeJob != nil {
		d.cfg.beforeJob(job)
	}

	log.Info("cron job starting")
	start := time.Now()
	err := cron.Run(context.Background(), job, d.cfg.Root, d.cfg.Clock)
	dur := time.Since(start)
	if err != nil {
		log.Error("cron job failed", "err", err, "duration", dur)
	} else {
		log.Info("cron job completed", "duration", dur)
	}

	if d.cfg.afterJob != nil {
		d.cfg.afterJob(job, err, dur)
	}
}

// pidFile guards against a second serenityd starting against the same
// brain root (acc: "a second instance refuses to start"). It is a plain
// os.ReadFile/os.WriteFile/os.Remove wrapper -- not a canonical brain-repo
// or index write, so it is outside the file-first gate's writeCalls set
// (internal/gate) by construction, not by allowlist entry.
type pidFile struct {
	path string
}

// acquire refuses with ErrAlreadyRunning if path names a still-live
// process, otherwise writes the current pid, overwriting a stale (dead or
// unparsable) pidfile left behind by a crash.
func (p pidFile) acquire() error {
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return fmt.Errorf("serenityd: create state dir for pidfile: %w", err)
	}
	data, err := os.ReadFile(p.path)
	switch {
	case err == nil:
		if pid, perr := strconv.Atoi(strings.TrimSpace(string(data))); perr == nil && processAlive(pid) {
			return fmt.Errorf("%w (pid %d, %s)", ErrAlreadyRunning, pid, p.path)
		}
		// Unparsable or dead -- a stale pidfile from a crash. Fall
		// through and overwrite it.
	case errors.Is(err, os.ErrNotExist):
		// No existing pidfile: nothing to check.
	default:
		return fmt.Errorf("serenityd: read pidfile %s: %w", p.path, err)
	}
	if err := os.WriteFile(p.path, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		return fmt.Errorf("serenityd: write pidfile %s: %w", p.path, err)
	}
	return nil
}

// release removes the pidfile. A missing pidfile is not an error --
// release is idempotent so a Run that failed partway through startup can
// always defer it safely.
func (p pidFile) release() error {
	err := os.Remove(p.path)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// processAlive reports whether pid names a live process, using signal 0
// (POSIX: checks permission/existence without actually delivering a
// signal) -- Unix only, matching this repo's macOS/Linux support matrix.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrProcessDone) {
		return false
	}
	// EPERM: the process exists but is owned by someone else. Treat it as
	// alive -- refusing a second start is the safe default when liveness
	// can't be disproven, versus clobbering another user's daemon.
	return errors.Is(err, syscall.EPERM)
}
