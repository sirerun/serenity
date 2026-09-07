package cron

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

func TestConsolidateRealSweep(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "test@example.invalid"}, {"config", "user.name", "Test"}, {"config", "core.hooksPath", "/dev/null"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v %v %s", args, err, out)
		}
	}
	cfg := config.Default()
	if err := cfg.Save(filepath.Join(root, config.FileName)); err != nil {
		t.Fatal(err)
	}
	fw := store.NewFenceWriter(root)
	p := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice"})
	p.Claims = []domain.Claim{{ID: "role", SubjectSlug: "alice", Predicate: "has_role", Family: "has_role", Object: "Engineer", State: domain.StateActive}}
	if _, err := fw.WriteEntity(p); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	if err := Run(context.Background(), "consolidate", root, fakeClock{at}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(fw.PathFor("person", "alice"))
	if err != nil || !strings.Contains(string(b), "- has_role: Engineer") {
		t.Fatalf("summary %s %v", b, err)
	}
	rec, err := ReadRecord(root, "consolidate")
	if err != nil || !rec.LastRun.Equal(at) || rec.RunCount != 1 {
		t.Fatalf("record %+v %v", rec, err)
	}
	cfg.Models.Embedding = "test@1"
	if err := cfg.Save(filepath.Join(root, config.FileName)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_EMBEDDINGS_BASE_URL", "")
	if err := Run(context.Background(), "consolidate", root, fakeClock{at.Add(time.Hour)}); err == nil {
		t.Fatal("missing provider passed")
	}
	after, err := ReadRecord(root, "consolidate")
	if err != nil || after.RunCount != 1 || !after.LastRun.Equal(at) {
		t.Fatalf("failure recorded %+v %v", after, err)
	}
}
