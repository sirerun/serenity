//go:build !darwin && !linux

package recovery

import (
	"errors"
	"os"
)

func verifyPrivateDirectory(string) error {
	return errors.New("hosted/recovery: safe plan filesystem unsupported")
}
func openPlanNoFollow(string) (*os.File, error) {
	return nil, errors.New("hosted/recovery: safe plan filesystem unsupported")
}
func linkPlanNoReplace(string, string) error {
	return errors.New("hosted/recovery: safe plan filesystem unsupported")
}
func syncPlanDirectory(string) error {
	return errors.New("hosted/recovery: safe plan filesystem unsupported")
}
