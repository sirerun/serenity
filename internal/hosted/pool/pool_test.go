package pool_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/pool"
	hoststore "github.com/sirerun/serenity/internal/hosted/store"
	"github.com/sirerun/serenity/internal/writer"
)

type embedding struct{}

func (embedding) ModelVersion() string                             { return "pool-test@v1" }
func (embedding) Embed(context.Context, string) ([]float32, error) { return []float32{1, 2, 3}, nil }

func TestEvictionOwnershipAndBoundedAdmission(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	ids := []string{hoststore.ID(), hoststore.ID(), hoststore.ID(), hoststore.ID()}
	for _, id := range ids {
		if err := os.Mkdir(filepath.Join(root, id), 0700); err != nil {
			t.Fatal(err)
		}
	}
	p, err := pool.New(pool.Config{BrainsRoot: root, MaxOpen: 2, MaxInFlight: 2, Embedder: embedding{}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	}()
	a, releaseA, err := p.Acquire(ctx, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	defer releaseA()
	if owner, err := writer.AcquireBrain(a.Root); err == nil {
		_ = owner.Close()
		t.Fatal("open brain has no exclusive owner")
	}
	b, releaseB, err := p.Acquire(ctx, ids[1])
	if err != nil {
		t.Fatal(err)
	}
	// Release even if a later assertion fails, so the pool can drain.
	defer releaseB()
	saved := false
	for _, tool := range b.Tools {
		if tool.Name == "remember" {
			result, e := tool.Handler(ctx, json.RawMessage(`{"fact":"The eviction marker is silver otter","provenance":"pool test"}`))
			if e != nil || result.IsError {
				t.Fatalf("remember %+v %v", result, e)
			}
			saved = true
		}
	}
	if !saved {
		t.Fatal("remember tool absent")
	}
	releaseB()
	_, releaseC, err := p.Acquire(ctx, ids[2])
	if err != nil {
		t.Fatal(err)
	}
	defer releaseC()
	owner, err := writer.AcquireBrain(b.Root)
	if err != nil {
		t.Fatalf("idle brain not evicted: %v", err)
	}
	if err = owner.Close(); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, release, err := p.Acquire(ctx, ids[3]); !errors.Is(err, pool.ErrCapacity) {
		if release != nil {
			release()
		}
		t.Fatalf("capacity: %v", err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("admission blocked")
	}
	releaseA()
	releaseC()
	reopened, release, err := p.Acquire(ctx, ids[1])
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	recalled := false
	for _, tool := range reopened.Tools {
		if tool.Name == "recall" {
			result, e := tool.Handler(ctx, json.RawMessage(`{"query":"silver otter"}`))
			data, _ := json.Marshal(result)
			if e != nil || result.IsError || !strings.Contains(string(data), "silver otter") {
				t.Fatalf("reopen lost memory: %s %v", data, e)
			}
			recalled = true
		}
	}
	if !recalled {
		t.Fatal("recall tool absent")
	}
}
