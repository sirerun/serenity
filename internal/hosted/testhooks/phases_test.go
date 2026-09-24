package testhooks_test

import (
	"testing"

	"github.com/sirerun/serenity/internal/hosted/testhooks"
)

// Phase names are the wire vocabulary between feature files and the crash
// harness (task58); a duplicate or a name the line protocol cannot carry would
// arm the wrong checkpoint or none.
func TestPhaseNamesAreDistinctAndWireSafe(t *testing.T) {
	phases := []string{
		testhooks.PhaseOperationReserved, testhooks.PhaseOperationCanonicalEntered, testhooks.PhaseOperationCanonicalWritten,
		testhooks.PhaseOperationCommitted, testhooks.PhaseReconcileFenced, testhooks.PhaseStageMeasured, testhooks.PhaseStagePublished,
		testhooks.PhaseDeletionJournaled, testhooks.PhaseDeletionPurged, testhooks.PhaseJournalSealed,
		testhooks.PhaseBackupManifestWritten, testhooks.PhaseRestoreFenced, testhooks.PhaseRestoreUnfrozen,
	}
	seen := map[string]bool{}
	for _, p := range phases {
		if p == "" || seen[p] {
			t.Errorf("phase %q is empty or duplicated", p)
		}
		seen[p] = true
		for _, r := range p {
			if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
				t.Errorf("phase %q contains whitespace, which the arm-pipe protocol splits on", p)
			}
		}
	}
}
