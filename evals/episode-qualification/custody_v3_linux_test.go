//go:build linux

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func sameReleaseProof() custodyProof {
	p := exampleProof()
	p.Version = 3
	p.Purpose = "serenity-episode-fixture-offline-v3"
	p.Operation = "prepare"
	op := strings.Repeat("a", 64)
	stage := "/var/lib/serenity-release/maintenance-" + op
	p.MaintenanceHold = &sameReleaseHold{Version: 1, Purpose: "serenity.fixture-maintenance/v1", OperationID: op, ManifestSHA256: strings.Repeat("1", 64), ServiceSource: p.ServiceSource, ExecutableSHA256: p.ServiceBinarySHA256, StateSHA256: p.StateSHA256, BrokerSHA256: strings.Repeat("2", 64), ConfigSHA256: p.ConfigSHA256, Supervisor: "serenity-release-" + op + ".service", Stage: stage, BackupTimerPriorState: "active", Phase: "maintenance-ready", Timestamp: 990, ExpiresAt: 1300}
	p.RuntimePath = stage + "/runtime.py"
	p.Hold = maintenanceHold{}
	return p
}
func TestV3ProofStrictDistinctWire(t *testing.T) {
	for _, operation := range []string{"prepare", "reissue", "revoke", "adopt-source"} {
		p := sameReleaseProof()
		p.Operation = operation
		if operation == "adopt-source" {
			p.SourceAdoption = adoptionProof().SourceAdoption
		}
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields["hold"] != nil || fields["maintenance_hold"] == nil {
			t.Fatal("cross-version wire alias")
		}
		var got custodyProof
		if decodeCustodyProof(raw, &got) != nil || !got.valid(1001, p.ToolSource, 1200, 1200) {
			t.Fatal("v3 refused")
		}
		for _, bad := range []string{"hold", "maintenance_hold", "source_adoption"} {
			mutated := map[string]json.RawMessage{}
			for k, v := range fields {
				mutated[k] = v
			}
			mutated[bad] = json.RawMessage("null")
			encoded, _ := json.Marshal(mutated)
			var got custodyProof
			if decodeCustodyProof(encoded, &got) == nil && got.valid(1001, p.ToolSource, 1200, 1200) {
				t.Fatalf("accepted null/cross version %s", bad)
			}
		}
	}
}
func TestV3ProofRefusesChangedIdentity(t *testing.T) {
	for _, damage := range []string{"expiry", "service", "operation", "previous", "runtime", "config", "purpose"} {
		p := sameReleaseProof()
		switch damage {
		case "expiry":
			p.MaintenanceHold.ExpiresAt = 1000
		case "service":
			p.MaintenanceHold.ServiceSource = strings.Repeat("9", 40)
		case "operation":
			p.MaintenanceHold.OperationID = strings.Repeat("b", 64)
		case "previous":
			p.Hold.PreviousStateSHA256 = p.StateSHA256
		case "runtime":
			p.RuntimePath = p.MaintenanceHold.Stage + "/activation-runtime.py"
		case "config":
			p.MaintenanceHold.ConfigSHA256 = strings.Repeat("9", 64)
		case "purpose":
			p.Purpose = "serenity-episode-fixture-offline-v2"
		}
		if p.valid(1001, p.ToolSource, 1200, 1200) {
			t.Fatalf("accepted %s", damage)
		}
	}
}
