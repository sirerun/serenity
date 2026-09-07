package cli

import (
	"bytes"
	"context"
	"encoding/json"

	"os/exec"

	"testing"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/report"
)

// TestReportExportCLI is plan T5.11's acc line, the CLI half: the real
// built `serenity` binary's `report --export` prints a valid,
// populated report.Report as JSON to stdout -- not runReport()
// called in-process, the same Tier 2 integration convention
// internal/cli/compact_test.go and check_test.go already established for
// a new CLI verb's own flag/exit-code behavior.
func TestReportExportCLI(t *testing.T) {
	requireGit(t)
	bin := buildSerenityBinary(t)
	root := t.TempDir()

	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatalf("init: %v\n%s", err, initOut.String())
	}

	ctx := context.Background()
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()

	claims := []domain.Claim{
		{ID: "c1", SubjectSlug: "alice", Family: "email", Predicate: "has_balance", Object: "100", ObjectKey: "100", State: domain.StateActive},
		{ID: "c2", SubjectSlug: "alice", Family: "email", Predicate: "has_balance", Object: "50", ObjectKey: "50", State: domain.StateSuperseded, SupersededBy: "c1"},
		{ID: "c1", SubjectSlug: "bob", Family: "email", Predicate: "works_at", Object: "acme", ObjectKey: "acme", State: domain.StateRetracted},
	}
	for _, c := range claims {
		if err := eng.UpsertClaim(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	ds := disposition.NewStore(eng)
	item, err := ds.Create(ctx, disposition.KindReconcile, nil, "", now.Add(-2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ds.Dispose(ctx, item.ID, disposition.VerdictReject, nil, "wrong", "human:t", "", now.Add(-1*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := ds.Create(ctx, disposition.KindReconcile, nil, "", now.Add(-30*time.Minute)); err != nil {
		t.Fatal(err)
	}

	if err := eng.RecordRebuildTiming(ctx, 100*time.Millisecond, now.Add(-5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "-C", root, "report", "--export")
	b, err := cmd.Output()
	if err != nil {
		t.Fatalf("serenity report --export: %v", err)
	}

	var got report.Report
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("exported report is not valid JSON: %v\n%s", err, b)
	}

	if want := (map[string]int64{"active": 0, "superseded": 0, "retracted": 0}); !mapsEqual(got.ClaimsByState, want) {
		t.Fatalf("ClaimsByState = %v, want %v", got.ClaimsByState, want)
	}
	// Index-only claims are not canonical history or extraction observations.
	if got.Extractions != nil || got.Corrections != nil || got.CorrectionsPer100Extractions != nil {
		t.Fatal("unmeasured extraction/correction counts must remain unavailable")
	}
	if got.ReconcileBacklog != 1 {
		t.Fatalf("ReconcileBacklog = %d, want 1", got.ReconcileBacklog)
	}
	if got.RebuildTiming.Never {
		t.Fatal("RebuildTiming.Never = true, want a recorded rebuild")
	}
	if got.RepoGrowth.SampleCount != 0 {
		t.Fatal("report must not record size samples")
	}
	if got.Spend.CeilingUSD != 50 {
		t.Fatalf("Spend.CeilingUSD = %v, want 50", got.Spend.CeilingUSD)
	}
	// RFC 0001 section 16 fields with no underlying population yet must
	// still appear, honestly empty -- see internal/report's own package
	// doc.
	if got.TrustLadderPromotionsDemotions.Cells == nil {
		t.Fatal("TrustLadderPromotionsDemotions.Cells is nil, want an empty (present) slice")
	}
	if got.SearchP95MS != nil {
		t.Fatalf("SearchP95MS = %v, want nil (no latency-sampling pipeline exists yet)", *got.SearchP95MS)
	}
}

func mapsEqual(a, b map[string]int64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
