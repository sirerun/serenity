//go:build !darwin && !linux

package writer

import (
	"errors"
	"os"
)

var ErrBrainOwned = errors.New("another Serenity writer owns this brain")

// AcquireBrain fails closed on platforms without the supported ownership lock.
func AcquireBrain(string) (*os.File, error) {
	return nil, errors.New("writer ownership requires Darwin or Linux")
}
