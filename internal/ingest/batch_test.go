package ingest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/writer"
)

func batchGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, raw)
	}
	return string(raw)
}

func TestBatchExistingFencePreservesHumanContent(t *testing.T) {
	w, closeQ := newTestWriter(t)
	defer closeQ()
	now := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	old := obs("demo-person", "works_at", "Old company", "source-old", "0-10", .9, now)
	if _, err := w.Write([]domain.Observation{old}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Flush(w.Queue, w.Fence.Root); err != nil {
		t.Fatal(err)
	}
	path := w.Fence.PathFor("topic", "demo-person")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, []byte("\nHuman prose outside managed blocks.\n")...)
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	batchGit(t, w.Fence.Root, "add", "brain")
	batchGit(t, w.Fence.Root, "commit", "--quiet", "-m", "human prose")
	batch := []domain.Observation{obs("demo-person", "works_at", "Old company", "source-one", "0-10", .9, now), obs("demo-person", "has_role", "Engineer", "source-two", "0-10", .8, now)}
	stats, err := w.Write(batch)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Written != 2 {
		t.Fatalf("writes=%d", stats.Written)
	}
	if _, err := writer.Flush(w.Queue, w.Fence.Root); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Human prose outside managed blocks.") {
		t.Fatal("lost human prose")
	}
	page, err := w.Fence.ParseEntity(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Claims) != 3 {
		t.Fatalf("claims=%d", len(page.Claims))
	}
	if batchGit(t, w.Fence.Root, "status", "--porcelain") != "" {
		t.Fatal("batch left uncommitted files")
	}
	stats, err = w.Write(batch)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Written != 0 || stats.Skipped != 2 {
		t.Fatalf("repeat changed claims: %+v", stats)
	}
}

func TestBatchShardRotationCommitsEverySegment(t *testing.T) {
	w, closeQ := newTestWriter(t)
	defer closeQ()
	w.Shard.RolloverBytes = 1
	now := time.Now()
	first := obs("demo-person", "has_balance", "100", "source-zero", "0-10", .9, now)
	if _, err := w.Write([]domain.Observation{first}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Flush(w.Queue, w.Fence.Root); err != nil {
		t.Fatal(err)
	}
	path := w.Shard.PathFor("demo-person", "has_balance")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	batch := []domain.Observation{obs("demo-person", "has_balance", "200", "source-one", "0-10", .9, now), obs("demo-person", "has_balance", "300", "source-two", "0-10", .9, now)}
	if _, err := w.Write(batch); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Flush(w.Queue, w.Fence.Root); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("rewrote prior shard segment")
	}
	files := strings.Fields(batchGit(t, w.Fence.Root, "ls-files", "brain/claims"))
	if len(files) != 3 {
		t.Fatalf("committed segments=%d: %v", len(files), files)
	}
	if batchGit(t, w.Fence.Root, "status", "--porcelain") != "" {
		t.Fatal("rotated shard not fully committed")
	}
	lines, err := w.Shard.Lines("demo-person", "has_balance")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 {
		t.Fatalf("shard lines=%d", len(lines))
	}
}

func TestBatchRefusesUnsafeExistingPageBeforeAnyPublication(t *testing.T) {
	for _, scenario := range []string{"dirty", "corrupt", "missing-fences", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			w, closeQ := newTestWriter(t)
			defer closeQ()
			now := time.Now()
			old := obs("demo-person", "works_at", "Old company", "source-old", "0-10", .9, now)
			if _, err := w.Write([]domain.Observation{old}); err != nil {
				t.Fatal(err)
			}
			if _, err := writer.Flush(w.Queue, w.Fence.Root); err != nil {
				t.Fatal(err)
			}
			path := w.Fence.PathFor("topic", "demo-person")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "dirty":
				raw = append(raw, []byte("\nUncommitted human note.\n")...)
			case "corrupt":
				raw = []byte("---\ntype: [\n---\nHuman bytes\n")
			case "missing-fences":
				raw = []byte("---\ntype: topic\nslug: demo-person\n---\n# Human note\n")
			case "symlink":
				outside := filepath.Join(t.TempDir(), "human.md")
				if err := os.WriteFile(outside, raw, 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, path); err != nil {
					t.Fatal(err)
				}
			}
			if scenario != "symlink" {
				if err := os.WriteFile(path, raw, 0644); err != nil {
					t.Fatal(err)
				}
			}
			if scenario != "dirty" {
				batchGit(t, w.Fence.Root, "add", "brain")
				batchGit(t, w.Fence.Root, "commit", "--quiet", "-m", "human revision")
			}
			batch := []domain.Observation{obs("new-person", "works_at", "Other company", "source-new", "0-10", .9, now), obs("demo-person", "has_role", "Engineer", "source-role", "0-10", .9, now)}
			if _, err := w.Write(batch); err == nil {
				t.Fatal("unsafe page accepted")
			}
			actual, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(actual) != string(raw) {
				t.Fatal("changed existing human bytes")
			}
			if _, err := os.Stat(w.Fence.PathFor("topic", "new-person")); !os.IsNotExist(err) {
				t.Fatal("partially published another page before validation failed")
			}
		})
	}
}
