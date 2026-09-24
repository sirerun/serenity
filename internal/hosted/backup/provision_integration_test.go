package backup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/hosted/provision"
	"github.com/sirerun/serenity/internal/hosted/store"
)

type initializationEmbedder struct{}

func (initializationEmbedder) ModelVersion() string { return "initialization-fixture@v1" }
func (initializationEmbedder) Embed(context.Context, string) ([]float32, error) {
	panic("empty initialization must not call embeddings")
}

func TestProvisionedBrainVersion2BackupRestore(t *testing.T) {
	for _, openRuntime := range []bool{false, true} {
		name := "untouched"
		if openRuntime {
			name = "runtime_opened"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			base := t.TempDir()
			data := filepath.Join(base, "data")
			if err := os.MkdirAll(data, 0700); err != nil {
				t.Fatal(err)
			}
			db, err := store.Open(filepath.Join(data, "control.db"))
			if err != nil {
				t.Fatal(err)
			}
			a, err := db.CreateAccount(ctx, "backup-init@example.com")
			if err != nil {
				t.Fatal(err)
			}
			brains := filepath.Join(data, "brains")
			b, err := (&provision.Provisioner{Store: db, BrainsRoot: brains}).Provision(ctx, a.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
			if openRuntime {
				p, err := pool.New(pool.Config{BrainsRoot: brains, MaxOpen: 1, MaxInFlight: 1, Embedder: initializationEmbedder{}})
				if err != nil {
					t.Fatal(err)
				}
				_, release, err := p.Acquire(ctx, b.ID)
				if err != nil {
					t.Fatal(err)
				}
				release()
				if err = p.Close(); err != nil {
					t.Fatal(err)
				}
			}
			source := filepath.Join(brains, b.ID)
			head, err := exec.CommandContext(ctx, "git", "-C", source, "rev-parse", "HEAD").Output()
			if err != nil {
				t.Fatal(err)
			}
			snapshot := filepath.Join(base, "snapshot")
			if err = Create(ctx, data, snapshot, "integration-fixture", fakeJournal{}); err != nil {
				t.Fatal(err)
			}
			restored := filepath.Join(base, "restored")
			if err = Restore(ctx, snapshot, restored); err != nil {
				t.Fatal(err)
			}
			got, err := exec.CommandContext(ctx, "git", "-C", filepath.Join(restored, "brains", b.ID), "rev-parse", "HEAD").Output()
			if err != nil || string(got) != string(head) {
				t.Fatalf("canonical HEAD changed: got=%s want=%s err=%v", got, head, err)
			}
			out, err := exec.CommandContext(ctx, "git", "-C", filepath.Join(restored, "brains", b.ID), "status", "--porcelain").Output()
			if err != nil || len(out) != 0 {
				t.Fatalf("restored tree dirty: %s %v", out, err)
			}
		})
	}
}
