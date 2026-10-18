package cli

import (
	"errors"
	"os"

	"github.com/sirerun/serenity/internal/writer"
	"github.com/spf13/cobra"
)

// Reviewed exemptions do not mutate canonical memory. Some readers can update
// the derived index/spend cache; ownership is not a read snapshot guarantee.
// Serve acquires in memoryTools after recognizing a valid brain, preserving its
// no-brain transport-only mode. New commands default to ownership.
func commandOwnsBrain(cmd *cobra.Command) bool {
	switch cmd.CommandPath() {
	case "serenity hosted broker", "serenity hosted plans", "serenity hosted authority new-operation-id", "serenity hosted authority inspect", "serenity hosted authority enroll", "serenity hosted authority restore-authority", "serenity hosted authority close", "serenity hosted authority seal", "serenity hosted authority inventory", "serenity hosted backup", "serenity hosted restore", "serenity hosted serve", "serenity check", "serenity doctor", "serenity status", "serenity search", "serenity ask", "serenity report", "serenity protocol", "serenity protocol conformance", "serenity connect", "serenity connect claude", "serenity connectors status", "serenity serve":
		return false
	default:
		return true
	}
}

// commandReadsBrain names actual canonical CLI entry points. Connector status
// prints static documentation and deliberately needs no brain or runtime files.
func commandReadsBrain(cmd *cobra.Command) bool {
	switch cmd.CommandPath() {
	case "serenity check", "serenity doctor", "serenity status", "serenity search", "serenity ask", "serenity report":
		return true
	default:
		return false
	}
}

func installBrainOwnership(cmd *cobra.Command) {
	for _, child := range cmd.Commands() {
		installBrainOwnership(child)
	}
	if !commandOwnsBrain(cmd) && !commandReadsBrain(cmd) {
		return
	}
	if cmd.Run != nil {
		run := cmd.Run
		cmd.RunE = func(c *cobra.Command, args []string) error { run(c, args); return nil }
		cmd.Run = nil
	}
	if cmd.RunE == nil {
		return
	}
	run := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) (runErr error) {
		if commandReadsBrain(c) {
			// Hook envelopes may describe unrelated events and must no-op without
			// touching a brain. Their actual check acquires its own read lease.
			if c.CommandPath() == "serenity check" {
				hook, _ := c.Flags().GetBool("claude-hook")
				if hook {
					return run(c, args)
				}
			}
			read, err := writer.AcquireCanonicalRead(flagRoot)
			if err != nil {
				return err
			}
			defer func() { runErr = errors.Join(runErr, read.Close()) }()
			return run(c, args)
		}
		// init alone may create the minimal root needed to acquire ownership.
		if c.CommandPath() == "serenity init" {
			if err := os.MkdirAll(flagRoot, 0755); err != nil {
				return err
			}
		}
		owner, err := writer.AcquireBrain(flagRoot)
		if err != nil {
			return err
		}
		defer func() { runErr = errors.Join(runErr, owner.Close()) }()
		return run(c, args)
	}
}
