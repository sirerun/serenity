package gateway

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/meter"
	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/writer"
)

func TestInspectorReadColdOwnedBrainDoesNotOpenOrMutateRuntime(t *testing.T) {
	g, brainPool, _, accountID, brainID, brainRoot := newLifecycleExportFixture(t)
	before, err := os.ReadDir(brainRoot)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	err = g.WithInspectorRead(context.Background(), accountID, brainID, func(view InspectorReadView) error {
		called = true
		if view.Root != brainRoot || view.Now.IsZero() {
			t.Fatalf("view = %#v", view)
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("cold fenced read: called=%v err=%v", called, err)
	}
	if _, _, err = brainPool.AcquireExistingForRead(context.Background(), brainID); !errors.Is(err, pool.ErrNotOpen) {
		t.Fatalf("cold read opened runtime: %v", err)
	}
	after, err := os.ReadDir(brainRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("cold read changed brain entries: before=%v after=%v", entryNames(before), entryNames(after))
	}
	for i := range before {
		if before[i].Name() != after[i].Name() {
			t.Fatalf("cold read changed brain entries: before=%v after=%v", entryNames(before), entryNames(after))
		}
	}
}

func TestInspectorReadRejectsCrossOwnerAndUnsafeCanonicalRoot(t *testing.T) {
	g, _, db, accountID, brainID, brainRoot := newLifecycleExportFixture(t)
	other, err := db.CreateAccount(context.Background(), "inspector-other@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if err = g.WithInspectorRead(context.Background(), other.ID, brainID, func(InspectorReadView) error { t.Fatal("cross-owner callback ran"); return nil }); !errors.Is(err, ErrInspectorBrainUnavailable) {
		t.Fatalf("cross-owner read error = %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err = os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(brainRoot, outside+"/brain"); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside+"/brain", brainRoot); err != nil {
		t.Fatal(err)
	}
	if err = g.WithInspectorRead(context.Background(), accountID, brainID, func(InspectorReadView) error { t.Fatal("symlink callback ran"); return nil }); err == nil {
		t.Fatal("symlinked canonical root was accepted")
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func inspectorMaintenanceID(brain string) writer.MaintenanceIdentity {
	return writer.MaintenanceIdentity{Schema: "serenity.canonical-maintenance/v1", OperationID: strings.Repeat("a", 64), BrainID: brain, SourceHead: strings.Repeat("b", 40), FenceDigest: strings.Repeat("c", 64)}
}
func TestInspectorCanonicalReadLeaseWarmAndCold(t *testing.T) {
	for _, warm := range []bool{false, true} {
		t.Run(map[bool]string{false: "cold", true: "warm"}[warm], func(t *testing.T) {
			g, p, _, account, brain, root := newLifecycleExportFixture(t)
			if warm {
				warmLifecycleRuntime(t, context.Background(), p, brain, root)
			}
			called := false
			if err := g.WithInspectorRead(context.Background(), account, brain, func(InspectorReadView) error {
				called = true
				m, err := writer.BeginBrainMaintenance(root, inspectorMaintenanceID(brain))
				if err == nil {
					_ = m.Close()
					t.Fatal("maintenance passed active read callback")
				}
				return nil
			}); err != nil || !called {
				t.Fatalf("fenced callback %v", err)
			}
			if err := p.Drop(brain); err != nil {
				t.Fatal(err)
			}
			m, err := writer.BeginBrainMaintenance(root, inspectorMaintenanceID(brain))
			if err != nil {
				t.Fatalf("read lease leaked %v", err)
			}
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			if err := g.WithInspectorRead(context.Background(), account, brain, func(InspectorReadView) error { t.Fatal("pending callback executed"); return nil }); !errors.Is(err, writer.ErrMaintenancePending) {
				t.Fatalf("pending inspector: %v", err)
			}
		})
	}
}
func TestInspectorMissingCanonicalViewDoesNotRepair(t *testing.T) {
	g, _, _, account, brain, root := newLifecycleExportFixture(t)
	path := filepath.Join(root, ".serenity", "canonical-view.lock")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(filepath.Join(root, ".serenity"))
	if err != nil {
		t.Fatal(err)
	}
	if err := g.WithInspectorRead(context.Background(), account, brain, func(InspectorReadView) error { t.Fatal("missing gate callback executed"); return nil }); err == nil {
		t.Fatal("missing view accepted")
	}
	after, err := os.ReadDir(filepath.Join(root, ".serenity"))
	if err != nil || len(after) != len(before) {
		t.Fatal("cold inspector repaired view state")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("reader created view lock")
	}
}

func TestInventoryAndGovernedLookupRejectPendingCanonicalMaintenance(t *testing.T) {
	g, p, db, account, brain, root := newLifecycleExportFixture(t)
	g.Meter = &meter.Meter{Store: db}
	token, err := g.Issuer.Issue(context.Background(), brain, account, []string{"memory:read"})
	if err != nil {
		t.Fatal(err)
	}
	// Establish that ordinary inventory uses a qualified cold view successfully.
	if _, err := g.Inventory(context.Background(), account, p.BrainsRoot()); err != nil {
		t.Fatal(err)
	}
	m, err := writer.BeginBrainMaintenance(root, inspectorMaintenanceID(brain))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Inventory(context.Background(), account, p.BrainsRoot()); !errors.Is(err, writer.ErrMaintenancePending) {
		t.Fatalf("inventory bypassed marker: %v", err)
	}
	request := httptest.NewRequest("GET", "/memory/v1/commands/00000000-0000-4000-8000-000000000002?project_id="+brain+"&brain_id="+governedBrain, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	g.ServeGovernedMemory(response, request)
	if response.Code != 503 || !strings.Contains(response.Body.String(), "canonical_unavailable") {
		t.Fatalf("lookup bypassed marker: HTTP%d", response.Code)
	}
}
