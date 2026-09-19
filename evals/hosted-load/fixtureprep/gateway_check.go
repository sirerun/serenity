package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/gateway"
	"github.com/sirerun/serenity/internal/hosted/meter"
	"github.com/sirerun/serenity/internal/hosted/plans"
	hstore "github.com/sirerun/serenity/internal/hosted/store"
)

// refusedAtCap is the gateway's remember limit rule (internal/hosted/gateway/gateway.go, the nonreplay remember
// check). A test pins the expression against that file so a change there fails here.
func refusedAtCap(inv gateway.Inventory, plan plans.Plan) bool {
	return inv.Memories >= plan.Memories || inv.StorageBytes >= plan.StorageBytes
}

// verifyThroughTheGateway asks the real gateway code what it sees. It copies control.db into a scratch directory,
// opens that copy with the hosted store, and calls gateway.Inventory and meter.Entitlement with the fixture's brain
// directories as the root. Both only read the brain directories, so the fixture is not opened for writing and is
// left unchanged. It cross-checks the verifier's own counts and shows the account-level limit inputs the gateway
// would use on a remember. No request is sent and no service runs.
func verifyThroughTheGateway(ctx context.Context, r *Report, dir string, plan *Plan, byLabel map[string]*acctInfo) {
	scratch, err := os.MkdirTemp("", "fixtureprep-gateway-")
	if err != nil {
		r.add("gateway.scratch_directory", "created", err.Error(), false, "")
		return
	}
	defer func() { _ = os.RemoveAll(scratch) }() // the scratch directory this function just created.
	src, err := os.ReadFile(filepath.Join(dir, "data", "control.db"))
	if err != nil {
		r.add("gateway.control_db_copy", "readable", err.Error(), false, "")
		return
	}
	copyPath := filepath.Join(scratch, "control.db")
	if err = os.WriteFile(copyPath, src, 0o600); err != nil {
		r.add("gateway.control_db_copy", "writable scratch", err.Error(), false, "")
		return
	}
	db, err := hstore.Open(copyPath)
	if err != nil {
		r.add("gateway.control_db_open", "opens", err.Error(), false, "")
		return
	}
	defer func() { _ = db.Close() }()
	g := &gateway.Gateway{Issuer: &credential.Issuer{Store: db}, Meter: &meter.Meter{Store: db}}
	brainsRoot := filepath.Join(dir, "data", "brains")
	var badMem, badBytes, badPlan, badRefusal, failed []string
	for i := range r.Accounts {
		a := &r.Accounts[i]
		info, ok := byLabel[a.Label]
		if !ok {
			continue
		}
		inv, err := g.Inventory(ctx, info.id, brainsRoot)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", a.Label, err))
			continue
		}
		ent, err := g.Meter.Entitlement(ctx, info.id)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: entitlement: %v", a.Label, err))
			continue
		}
		a.GatewayBrains, a.GatewayMemories, a.GatewayStorageBytes, a.GatewayPlan = inv.Brains, inv.Memories, inv.StorageBytes, ent.Plan.ID
		a.GatewayRefusesRemember = refusedAtCap(inv, ent.Plan)
		if inv.Brains != int64(a.Brains) || inv.Memories != int64(a.Memories) {
			badMem = append(badMem, fmt.Sprintf("%s: gateway sees %d brains, %d memories; verifier counted %d, %d", a.Label, inv.Brains, inv.Memories, a.Brains, a.Memories))
		}
		if inv.StorageBytes != a.StorageBytes {
			badBytes = append(badBytes, fmt.Sprintf("%s: gateway sees %d bytes; verifier summed %d", a.Label, inv.StorageBytes, a.StorageBytes))
		}
		if ent.Plan.ID != a.PlanID {
			badPlan = append(badPlan, fmt.Sprintf("%s: entitlement resolves to %s, want %s", a.Label, ent.Plan.ID, a.PlanID))
		}
		if want := !plan.Reduced; a.GatewayRefusesRemember != want {
			badRefusal = append(badRefusal, fmt.Sprintf("%s: refuses a remember=%v, want %v", a.Label, a.GatewayRefusesRemember, want))
		}
	}
	r.add("gateway.inventory_readable_for_every_account", []string{}, failed, len(failed) == 0, "gateway.Inventory and meter.Entitlement on a scratch copy of control.db; the fixture's brain directories are only read")
	r.add("gateway.inventory_brains_and_memories_equal_verifier_counts", []string{}, badMem, len(badMem) == 0, "")
	r.add("gateway.inventory_storage_bytes_equal_verifier_sum", []string{}, badBytes, len(badBytes) == 0, "")
	r.add("gateway.entitlement_resolves_to_the_planned_plan", []string{}, badPlan, len(badPlan) == 0, "")
	r.add("gateway.remember_limit_rule_at_the_cap", []string{}, badRefusal, len(badRefusal) == 0, "full: every account is at its memory cap, so the gateway's limit inputs refuse a nonreplay remember; smoke: none does. Computed from the gateway's own Inventory and Entitlement, not from a request")
}
