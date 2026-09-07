package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/providers"
)

func TestCronRevisitCLI(t *testing.T) {
	bin := buildSerenityBinary(t)
	root := t.TempDir()
	write := func(condition string) {
		t.Helper()
		entry := &ledger.Entry{ID: "dec-0001", Kind: ledger.KindDecision, Title: "A decision", State: ledger.StateAccepted, Created: "2020-01-01T00:00:00Z", Body: "Original rationale", Alternatives: []ledger.Alternative{{Option: "Alternative", WhyNot: "Rejected", RevisitIf: condition}, {Option: "Unstructured", WhyNot: "Rejected", RevisitIf: "when we learn more"}}}
		data, err := ledger.Encode(entry)
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, ".dira", "entries")
		if err = os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, entry.ID+".md"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("after:2020-01-01T00:00:00Z")
	for _, want := range []string{"created=1 unsupported=1", "created=0 unsupported=1"} {
		out, err := exec.Command(bin, "-C", root, "cron", "revisit").CombinedOutput()
		if err != nil {
			t.Fatalf("CLI: %v %s", err, out)
		}
		if !strings.Contains(string(out), want) {
			t.Fatalf("got %s want %s", out, want)
		}
	}
	db, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	items, err := disposition.NewStore(db).List(context.Background())
	if err != nil || len(items) != 1 {
		t.Fatalf("items %+v err %v", items, err)
	}
	checkpoint, err := db.RevisitCheckpoint(context.Background(), strings.TrimSuffix(items[0].ID, ":1"))
	if err != nil || checkpoint.LastRevisitedAt.IsZero() {
		t.Fatalf("checkpoint %+v err %v", checkpoint, err)
	}
	write("after:malformed")
	out, err := exec.Command(bin, "-C", root, "cron", "revisit").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "revisit") {
		t.Fatalf("malformed CLI: %v %s", err, out)
	}
}
