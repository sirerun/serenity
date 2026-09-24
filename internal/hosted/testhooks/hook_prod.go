//go:build !hostedtest

package testhooks

// at is an unconditional no-op in every ordinary build. It reads no
// environment variable and opens no file: there is no code path in this
// build that can pause or crash a production process from an external
// signal.
func at(string) {}
