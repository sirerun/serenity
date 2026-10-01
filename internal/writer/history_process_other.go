//go:build !darwin && !linux

package writer

import (
	"errors"
	"os/exec"
)

func configureHistoryProcessGroup(*exec.Cmd) error {
	return errors.New("writer: process-group cancellation is unsupported on this platform")
}
