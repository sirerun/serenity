//go:build !darwin && !linux

package privatefs

import (
	"context"
	"errors"
)

var errUnsupported = errors.New("privatefs: filesystem ownership verification unsupported")

func ValidateDirectory(context.Context, string) error { return errUnsupported }
func ReadFile(context.Context, string, string, int64) ([]byte, error) {
	return nil, errUnsupported
}
