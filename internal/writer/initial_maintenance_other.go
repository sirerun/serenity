//go:build !darwin && !linux

package writer

import "os"

func safeMaintenanceMarker(os.FileInfo) bool { return false }
func initialMaintenanceObservation(os.FileInfo) (InitialMaintenanceObservation, error) {
	return InitialMaintenanceObservation{}, ErrMaintenancePending
}
func initialMaintenanceAppend(*os.Root, string) (*os.File, error) { return nil, ErrMaintenancePending }
func validateInitialNamespace(*BrainMaintenance, string) error    { return ErrMaintenancePending }
