package backup

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/hosted/provision"
	"github.com/sirerun/serenity/internal/hosted/store"
)

type initializationEmbedder struct{ allowWrites bool }

func (initializationEmbedder) ModelVersion() string { return "initialization-fixture@v1" }
func (e initializationEmbedder) Embed(context.Context, string) ([]float32, error) {
	if !e.allowWrites {
		panic("empty initialization must not call embeddings")
	}
	return []float32{1, 2, 3}, nil
}

func TestProvisionedBrainVersion2BackupRestore(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
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
			// Exercise storage only; restored account eligibility remains frozen.
			recoveredPool, err := pool.New(pool.Config{BrainsRoot: filepath.Join(restored, "brains"), MaxOpen: 1, MaxInFlight: 1, Embedder: initializationEmbedder{allowWrites: true}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := recoveredPool.Close(); err != nil {
					t.Error(err)
				}
			})
			runtime, release, err := recoveredPool.Acquire(ctx, b.ID)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			remembered := false
			for _, tool := range runtime.Tools {
				if tool.Name == "remember" {
					result, e := tool.Handler(ctx, json.RawMessage(`{"fact":"Restored write marker is cobalt","provenance":"restore fixture"}`))
					if e != nil || result.IsError {
						t.Fatalf("remember after restore: %+v %v", result, e)
					}
					remembered = true
				}
			}
			if !remembered {
				t.Fatal("remember tool missing")
			}
			release()
			if err := recoveredPool.FlushAll(); err != nil {
				t.Fatalf("flush after restore: %v", err)
			}
			author, err := exec.CommandContext(ctx, "git", "-C", filepath.Join(restored, "brains", b.ID), "log", "-1", "--format=%an <%ae>").Output()
			if err != nil || strings.TrimSpace(string(author)) != "Serenity Hosted <hosted@serenity.sire.run>" {
				t.Fatalf("restored writer identity: %q %v", author, err)
			}

		})
	}
}
