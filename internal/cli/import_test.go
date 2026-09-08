package cli

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/providers"
)

func TestImportCLIRebuildsCanonicalFixture(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "fixture@example.invalid"}, {"config", "user.name", "Fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v: %s", err, out)
		}
	}
	if err := config.Default().Save(filepath.Join(root, config.FileName)); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"-C", root, "import", "--from-gbrain", "../../testdata/gbrain-fixture"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "2 page(s), 8 claim(s)") {
		t.Fatalf("unexpected output: %s", out.String())
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	stats, err := eng.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats["claims"] != 8 || stats["entities"] != 2 {
		t.Fatalf("CLI failed to rebuild imported data: %v", stats)
	}
}
