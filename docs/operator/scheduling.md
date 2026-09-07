# Scheduling: running `serenity cron` on a timer

`serenity cron <job>` (ADR 006, plan T2.19) runs one scheduled job to
completion and exits: `sweep`, `consolidate`, `decay`, `slo`, `revisit`. Nothing in
Serenity schedules these on its own — you supply the timer. This page ships
a launchd unit (macOS) and a systemd timer (Linux) for each job.

## Jobs

| Job           | What it does                                                              | Suggested cadence |
|---------------|----------------------------------------------------------------------------|--------------------|
| `sweep`       | Expiry sweep: ages pending disposition items to deferred, then parked      | Every 15–30 min    |
| `consolidate` | Rebuilds summary fences and re-embeds changed chunks                       | Nightly            |
| `decay`       | Read-time confidence decay, alias-candidate detection                      | Weekly             |
| `slo`         | Recomputes queue SLOs (depth, age, time-to-dispose) for `serenity status`  | Every 15–30 min    |
| `revisit`     | Creates [decision-review cards](revisit.md) for elapsed or changed-claim conditions | Weekly |

Two cron jobs racing on the same brain repo resolve through the dirty-tree
guard, not a lock file (ADR 006) — run at most one timer per job per brain.

## Verifying manually first

Before installing a timer, confirm the binary and brain root are both
right:

```sh
serenity -C /path/to/brain cron sweep
```

A successful run exits 0 and prints `cron sweep: ok`. Running it again
immediately is safe — it is idempotent by design (T2.19 acc).

## macOS: launchd

Find your `serenity` binary's absolute path first — `command -v serenity`
(a `go install` build) or your Homebrew prefix's `bin/serenity`. Substitute
it and your brain root into the template below, one plist per job.

`~/Library/LaunchAgents/run.sire.serenity.cron.sweep.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>run.sire.serenity.cron.sweep</string>
  <key>ProgramArguments</key>
  <array>
    <string>/absolute/path/to/serenity</string>
    <string>-C</string>
    <string>/absolute/path/to/brain</string>
    <string>cron</string>
    <string>sweep</string>
  </array>
  <key>StartInterval</key>
  <integer>900</integer>
  <key>StandardOutPath</key>
  <string>/tmp/serenity-cron-sweep.log</string>
  <key>StandardErrorPath</key>
  <string>/tmp/serenity-cron-sweep.log</string>
</dict>
</plist>
```

Repeat with `consolidate`/`decay`/`slo`/`revisit` in the `Label` and the final
`ProgramArguments` entry — adjust `StartInterval` (seconds) per the table
above (86400 for nightly, 604800 for weekly).

Load and verify:

```sh
launchctl load ~/Library/LaunchAgents/run.sire.serenity.cron.sweep.plist
launchctl list | grep run.sire.serenity.cron.sweep
```

Unload to stop: `launchctl unload ~/Library/LaunchAgents/run.sire.serenity.cron.sweep.plist`.

## Linux: systemd timer

One `.service` (what to run) plus one `.timer` (when) per job, in
`~/.config/systemd/user/` for a per-user timer.

`~/.config/systemd/user/serenity-cron-sweep.service`:

```ini
[Unit]
Description=serenity cron sweep

[Service]
Type=oneshot
ExecStart=/absolute/path/to/serenity -C /absolute/path/to/brain cron sweep
```

`~/.config/systemd/user/serenity-cron-sweep.timer`:

```ini
[Unit]
Description=Run serenity cron sweep every 15 minutes

[Timer]
OnBootSec=5min
OnUnitActiveSec=15min
Persistent=true

[Install]
WantedBy=timers.target
```

Repeat with `consolidate`/`decay`/`slo`/`revisit` in the filenames, `ExecStart`'s
final argument, and `OnUnitActiveSec` per the table above (`1d` nightly,
`1w` weekly).

Enable and verify:

```sh
systemctl --user daemon-reload
systemctl --user enable --now serenity-cron-sweep.timer
systemctl --user list-timers | grep serenity-cron
```

Run once by hand to confirm before waiting for the first tick:

```sh
systemctl --user start serenity-cron-sweep.service
journalctl --user -u serenity-cron-sweep.service -n 20
```

## M4 note

`serenityd` (plan T4.1) will embed these same job functions on an internal
ticker so a running daemon needs no external timer at all (ADR 006).
Until then, `serenity cron` plus the units above is the supported way to
run scheduled work.
