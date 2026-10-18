package episodeinstall

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/writer"
)

type initialUnitAdmission struct{ transactionAdmission }

func (a initialUnitAdmission) CheckInitialRecovery(_ context.Context, p InitialRecoveryPins) error {
	if p.Purpose != initialRecoveryPurpose || p.Phase == "" || p.Phase != "precreate" && !digestPattern.MatchString(p.RecordSHA256) {
		return ErrUnqualified
	}
	return nil
}
func initialUnitFixture(t *testing.T) (PreparedRef, initialUnitAdmission, string) {
	t.Helper()
	ref := transactionPrepared(t)
	m := transactionOwner(t, ref)
	if m.Close() != nil {
		t.Fatal("close owner")
	}
	path := filepath.Join(ref.Source, ".serenity", "canonical-maintenance.json")
	if os.WriteFile(path, expectedInitialMarker(ref.Identity)[:17], 0600) != nil {
		t.Fatal("set synthetic prefix")
	}
	return ref, initialUnitAdmission{transactionAdmissionFor(t, ref)}, path
}

func TestInitialRecoveryPublishedRecordPreservesOriginalPrefix(t *testing.T) {
	ref, a, path := initialUnitFixture(t)
	ctx := context.Background()
	_, _, err := recoverInitialMaintenance(ctx, ref, a, transactionHooks{checkpoint: func(point string) error {
		if point == "after-record-rename:"+initialRecoveryName {
			return ErrIncomplete
		}
		return nil
	}})
	if err == nil {
		t.Fatal("injected interruption returned owner")
	}
	record, present, err := loadInitialRecord(ref)
	if err != nil || !present || len(record.Original.Prefix) != 17 {
		t.Fatal("missing original evidence")
	}
	rawBefore, err := os.ReadFile(filepath.Join(ref.Directory, initialRecoveryName))
	if err != nil {
		t.Fatal(err)
	}
	if os.WriteFile(path, expectedInitialMarker(ref.Identity)[:31], 0600) != nil {
		t.Fatal("extend synthetic initial write")
	}
	owner, result, err := RecoverInitialMaintenance(ctx, ref, a)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close() }()
	if result.State != "initial-gate-reconciled" || !result.MarkerHeld || !result.ResidualCopies {
		t.Fatal("incorrect result")
	}
	after, err := os.ReadFile(filepath.Join(ref.Directory, initialRecoveryName))
	if err != nil || !bytes.Equal(rawBefore, after) {
		t.Fatal("original record changed")
	}
	if _, err = Install(ctx, ref, owner, a); err != nil {
		t.Fatal("matching published evidence blocked Install", err)
	}
}

func TestInitialRecoveryPendingRecordRetry(t *testing.T) {
	for _, point := range []string{"after-record-create:", "after-record-midwrite:", "after-record-file-sync:"} {
		t.Run(point, func(t *testing.T) {
			ref, a, _ := initialUnitFixture(t)
			_, _, err := recoverInitialMaintenance(context.Background(), ref, a, transactionHooks{checkpoint: func(got string) error {
				if got == point+initialRecoveryName {
					return ErrIncomplete
				}
				return nil
			}})
			if err == nil {
				t.Fatal("interruption accepted")
			}
			owner, _, err := RecoverInitialMaintenance(context.Background(), ref, a)
			if err != nil {
				t.Fatal(err)
			}
			_ = owner.Close()
		})
	}
}

func TestInitialRecoveryUnknownProgressAndSourceDriftRefuse(t *testing.T) {
	for _, kind := range []string{"unknown", "progress", "progress-pending", "retained", "replacement-extra", "canonical-drift", "index-extra", "record-unknown", "record-null-prefix", "pending-unknown"} {
		t.Run(kind, func(t *testing.T) {
			ref, a, path := initialUnitFixture(t)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			write := func(p string, b []byte) {
				t.Helper()
				if os.WriteFile(p, b, 0600) != nil {
					t.Fatal("synthetic write")
				}
			}
			switch kind {
			case "unknown":
				write(filepath.Join(ref.Directory, "unknown"), nil)
			case "progress":
				write(filepath.Join(ref.Directory, progressName), nil)
			case "progress-pending":
				write(filepath.Join(ref.Directory, progressName+".pending"), nil)
			case "retained":
				if os.Mkdir(filepath.Join(ref.Directory, "retained"), 0700) != nil {
					t.Fatal("synthetic directory")
				}
			case "replacement-extra":
				write(filepath.Join(ref.Directory, "replacement", "extra"), nil)
			case "canonical-drift":
				write(filepath.Join(ref.Source, "serenity.yml"), []byte("synthetic drift"))
			case "index-extra":
				write(filepath.Join(ref.Source, ".serenity", "extra.db"), nil)
			case "record-unknown":
				write(filepath.Join(ref.Directory, initialRecoveryName), []byte("{}\n"))
			case "record-null-prefix":
				pending, err := writer.OpenInitialBrainMaintenance(ref.Source, ref.Identity)
				if err != nil {
					t.Fatal(err)
				}
				observation, err := pending.Observation()
				_ = pending.Close()
				if err != nil {
					t.Fatal(err)
				}
				record := initialRecord(ref, observation)
				record.Original.Prefix = nil
				raw, _ := json.Marshal(record)
				write(filepath.Join(ref.Directory, initialRecoveryName), append(raw, '\n'))
			case "pending-unknown":
				write(filepath.Join(ref.Directory, initialRecoveryName+".pending"), []byte("foreign"))
			}
			owner, _, err := RecoverInitialMaintenance(context.Background(), ref, a)
			if err == nil || owner != nil {
				t.Fatal("ambiguous state accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("refusal edited initial marker")
			}
		})
	}
}

func TestInitialRecoveryRecordNeverAuthorizesOrdinaryInstall(t *testing.T) {
	ref, a, _ := initialUnitFixture(t)
	_, _, err := recoverInitialMaintenance(context.Background(), ref, a, transactionHooks{checkpoint: func(point string) error {
		if point == "after-record-midwrite:"+initialRecoveryName {
			return ErrIncomplete
		}
		return nil
	}})
	if err == nil {
		t.Fatal("interruption accepted")
	}
	// Full marker alone cannot cause ordinary installation to adopt pending
	// reconciliation evidence, even with separately valid ordinary cold admission.
	path := filepath.Join(ref.Source, ".serenity", "canonical-maintenance.json")
	if os.WriteFile(path, expectedInitialMarker(ref.Identity), 0600) != nil {
		t.Fatal("synthetic marker")
	}
	owner, err := writer.ResumeBrainMaintenance(ref.Source, ref.Identity)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = owner.Close() }()
	if _, err = Install(context.Background(), ref, owner, a); err == nil {
		t.Fatal("Install accepted pending recovery evidence")
	}
}

func TestInitialRecoveryGateSyncPrecedesEvidence(t *testing.T) {
	for _, point := range []string{"initial:before-gate-sync", "initial:after-gate-sync"} {
		t.Run(point, func(t *testing.T) {
			ref, a, path := initialUnitFixture(t)
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			owner, _, err := recoverInitialMaintenance(context.Background(), ref, a, transactionHooks{checkpoint: func(got string) error {
				if got == point {
					return ErrIncomplete
				}
				return nil
			}})
			if err == nil || owner != nil {
				t.Fatal("sync boundary failure returned owner")
			}
			for _, name := range []string{initialRecoveryName, initialRecoveryName + ".pending"} {
				if _, err = os.Lstat(filepath.Join(ref.Directory, name)); !os.IsNotExist(err) {
					t.Fatal("evidence preceded successful gate stabilization")
				}
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("sync boundary edited marker bytes")
			}
			owner, _, err = RecoverInitialMaintenance(context.Background(), ref, a)
			if err != nil {
				t.Fatal("fresh retry refused", err)
			}
			_ = owner.Close()
		})
	}
}
