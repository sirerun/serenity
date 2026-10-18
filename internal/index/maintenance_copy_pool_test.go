//go:build darwin || linux

package index_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/gitrun"
	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/index"
)

type maintenancePoolEmbedding struct{}

func (maintenancePoolEmbedding) ModelVersion() string { return "maintenance-cohort-test@v1" }
func (maintenancePoolEmbedding) Embed(context.Context, string) ([]float32, error) {
	panic("canonical pool fixture must not call a provider")
}

func TestMaintenanceCopyActualCanonicalPoolOpenDrop(t *testing.T) {
	ctx := context.Background()
	brains, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id := "b123456789abcdef"
	root := filepath.Join(brains, id)
	if err = os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".serenity/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	git := gitrun.Quarantine(root)
	for _, args := range [][]string{{"init", "--initial-branch=main"}, {"config", "user.name", "Synthetic maintenance"}, {"config", "user.email", "synthetic@example.test"}, {"add", "--", ".gitignore"}, {"commit", "-m", "Synthetic hosted baseline"}} {
		if _, err = git.Output(ctx, args...); err != nil {
			t.Fatal("initialize actual pool fixture", err)
		}
	}
	runtimePool, err := pool.New(pool.Config{BrainsRoot: brains, MaxOpen: 1, MaxInFlight: 1, Embedder: maintenancePoolEmbedding{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := runtimePool.Close(); err != nil {
			t.Error(err)
		}
	}()
	runtime, release, err := runtimePool.AcquireCanonical(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Root != root {
		release()
		t.Fatal("pool opened different brain")
	}
	release()
	if err = runtimePool.DropContext(ctx, id); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, ".serenity", "index.db")
	before := map[string][]byte{}
	copyDirectory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(copyDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		raw, readErr := os.ReadFile(source + suffix)
		if os.IsNotExist(readErr) {
			continue
		}
		if readErr != nil {
			t.Fatal(readErr)
		}
		before[suffix] = raw
		if err = os.WriteFile(filepath.Join(copyDirectory, "index.db"+suffix), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := index.QualifyMaintenanceCopy(ctx, filepath.Join(copyDirectory, "index.db"))
	if err != nil {
		t.Fatal("actual pool-created cohort refused", err)
	}
	if len(result.EmptyRuntimeTables) != 6 || len(result.SchemaSHA256) != 64 {
		t.Fatalf("incomplete actual pool qualification: %+v", result)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		after, readErr := os.ReadFile(source + suffix)
		original, existed := before[suffix]
		if !existed {
			if !os.IsNotExist(readErr) {
				t.Fatal("qualification created source sidecar")
			}
			continue
		}
		if readErr != nil || !bytes.Equal(original, after) {
			t.Fatal("qualification modified original pool DB family")
		}
	}
}
