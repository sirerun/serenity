//go:build !darwin && !linux

package writer

func (*BrainMaintenance) ValidateFor(string, MaintenanceIdentity) error {
	return ErrMaintenanceIdentity
}
