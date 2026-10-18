//go:build !darwin && !linux

package writer

import (
	"errors"
	"os"
)

func acquireBrainLock(string) (*os.File, *os.Root, error) { return nil, nil, ErrMaintenancePending }
func maintenanceCreate(*os.Root, string) (*os.File, error) {
	return nil, errors.New("maintenance requires Darwin or Linux")
}
func maintenanceOpen(*os.Root, string) (*os.File, error) { return nil, ErrMaintenancePending }
func maintenanceSync(*os.Root) error                     { return ErrMaintenancePending }
