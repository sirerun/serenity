package provision_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/provision"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func TestConcurrentProvisionAndRecovery(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, e := store.Open(filepath.Join(dir, "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	a, e := s.CreateAccount(ctx, "a@example.com")
	if e != nil {
		t.Fatal(e)
	}
	id := store.ID()
	if _, e = s.InsertBrain(ctx, a.ID, id, id, "allocating", time.Now()); e != nil {
		t.Fatal(e)
	}
	p := &provision.Provisioner{Store: s, BrainsRoot: filepath.Join(dir, "brains")}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, err := p.Provision(ctx, a.ID)
			if err != nil {
				t.Error(err)
				return
			}
			if b.ID != id || b.State != "ready" {
				t.Errorf("recovered %+v", b)
			}
		}()
	}
	wg.Wait()
	var n int
	if e = s.DB().QueryRow(`SELECT count(*) FROM brains WHERE account_id=?`, a.ID).Scan(&n); e != nil || n != 1 {
		t.Fatalf("brain count %d %v", n, e)
	}
}

func TestRecoverAdditionalAllocation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := store.Open(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	a, err := s.CreateAccount(ctx, "recovery@example.com")
	if err != nil {
		t.Fatal(err)
	}
	p := &provision.Provisioner{Store: s, BrainsRoot: filepath.Join(dir, "brains")}
	if _, err = p.Provision(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	id := store.ID()
	_, err = s.DB().ExecContext(ctx, `INSERT INTO brains(id,account_id,path_key,state,is_default,created_at) VALUES(?,?,?,'allocating',0,?)`, id, a.ID, id, store.Stamp(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := p.Additional(ctx, a.ID, 2)
	if err != nil || resumed.ID != id {
		t.Fatalf("retry allocated %+v: %v", resumed, err)
	}
	for range 2 {
		if err = p.Recover(ctx); err != nil {
			t.Fatal(err)
		}
	}
	b, err := s.BrainByID(ctx, a.ID, id)
	if err != nil || b.State != "ready" {
		t.Fatalf("recovered %+v: %v", b, err)
	}
	if _, err = p.Additional(ctx, "missing-account", 3); err == nil {
		t.Fatal("allocated for missing account")
	}
}
