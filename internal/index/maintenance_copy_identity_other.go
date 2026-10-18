//go:build !darwin && !linux

package index

import "os"

func maintenancePrivateInfo(os.FileInfo, bool) bool { return false }
func maintenanceOpenPrivate(*os.Root, string) (*os.File, error) {
	return nil, ErrMaintenanceCopyUnqualified
}
