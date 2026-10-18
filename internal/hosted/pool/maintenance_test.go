//go:build darwin || linux

package pool_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/writer"
)

func TestPoolRefusesPersistentCanonicalMaintenance(t *testing.T) {
	_, _, brainsRoot, brain := provisionedPoolFixture(t)
	root := filepath.Join(brainsRoot, brain.ID)
	id := writer.MaintenanceIdentity{Schema: "serenity.canonical-maintenance/v1", OperationID: "pool-maintenance", BrainID: brain.ID, SourceHead: strings.Repeat("a", 40), FenceDigest: strings.Repeat("b", 64)}
	owner, err := writer.BeginBrainMaintenance(root, id)
	if err != nil {
		t.Fatal(err)
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, root)
	p, err := pool.New(pool.Config{BrainsRoot: brainsRoot, MaxOpen: 1, MaxInFlight: 1, Embedder: embedding{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	}()
	for _, mode := range []string{"ordinary", "canonical", "existing"} {
		var release func()
		switch mode {
		case "ordinary":
			_, release, err = p.Acquire(context.Background(), brain.ID)
		case "canonical":
			_, release, err = p.AcquireCanonical(context.Background(), brain.ID)
		case "existing":
			_, release, err = p.AcquireExisting(context.Background(), brain.ID)
		}
		if release != nil {
			release()
		}
		if !errors.Is(err, writer.ErrMaintenancePending) {
			t.Fatalf("%s admitted pending brain: %v", mode, err)
		}
		if after := snapshotTree(t, root); !reflect.DeepEqual(before, after) {
			t.Fatalf("%s mutated pending brain", mode)
		}
	}
	owner, err = writer.ResumeBrainMaintenance(root, id)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := owner.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err = owner.Finish(id); err != nil {
		t.Fatal(err)
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
	_, release, err := p.AcquireCanonical(context.Background(), brain.ID)
	if err != nil {
		t.Fatalf("exact reconciled brain could not open: %v", err)
	}
	release()
}
