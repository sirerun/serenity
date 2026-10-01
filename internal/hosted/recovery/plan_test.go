package recovery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

func validPlanInput() PlanInput {
	return PlanInput{
		SnapshotSHA256: strings.Repeat("a", 64),
		JournalWatermark: contracts.DeletionWatermark{
			Generation: 1, SequenceID: 3, EntryHash: strings.Repeat("b", 64),
		},
		FenceGeneration:  2,
		ProviderObserved: time.Date(2026, 10, 1, 20, 15, 30, 123000000, time.UTC),
		Accounts:         []string{"account-z", "account-a"},
	}
}

func privatePlanDir(t *testing.T) string {
	t.Helper()
	dir := recoveryTestTempDir(t)
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func recoveryTestTempDir(t *testing.T) string {
	t.Helper()
	if base := os.Getenv("SERENITY_RECOVERY_TEST_TMPDIR"); base != "" {
		dir, err := os.MkdirTemp(base, "recovery-plan-test-")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		return dir
	}
	return t.TempDir()
}

func TestCreateAndLoadPlanIsCanonicalAndIdempotent(t *testing.T) {
	dir := privatePlanDir(t)
	input := validPlanInput()
	created, err := CreatePlan(context.Background(), dir, input)
	if err != nil {
		t.Fatal(err)
	}
	if created.PlanHash == "" || created.SnapshotSHA256 != input.SnapshotSHA256 {
		t.Fatalf("created plan did not bind snapshot content: %+v", created)
	}
	if got := strings.Join(created.Accounts, ","); got != "account-a,account-z" {
		t.Fatalf("canonical account order = %q", got)
	}
	payload, err := json.Marshal(payloadFromPlan(created))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	if created.PlanHash != hex.EncodeToString(sum[:]) {
		t.Fatal("plan hash does not cover canonical payload")
	}

	loaded, err := LoadPlan(context.Background(), dir, created.PlanHash)
	if err != nil {
		t.Fatal(err)
	}
	if !plansEqual(loaded, created) {
		t.Fatalf("reloaded plan differs: got %+v want %+v", loaded, created)
	}
	createdAgain, err := CreatePlan(context.Background(), dir, input)
	if err != nil {
		t.Fatalf("identical create should be idempotent: %v", err)
	}
	if !plansEqual(createdAgain, created) {
		t.Fatal("idempotent create returned different plan")
	}
}

func TestLoadPlanRejectsUnchangedHashMismatch(t *testing.T) {
	dir := privatePlanDir(t)
	plan, err := CreatePlan(context.Background(), dir, validPlanInput())
	if err != nil {
		t.Fatal(err)
	}
	other := strings.Repeat("c", 64)
	if _, err = LoadPlan(context.Background(), dir, other); err == nil {
		t.Fatal("LoadPlan accepted a different approved hash")
	}
	path := filepath.Join(dir, plan.PlanHash+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	copiedPath := filepath.Join(dir, other+".json")
	if err = os.WriteFile(copiedPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadPlan(context.Background(), dir, other); !errors.Is(err, ErrPlanInvalid) {
		t.Fatalf("LoadPlan accepted valid artifact bytes copied under wrong approved hash: %v", err)
	}
	changed := bytes.Replace(data, []byte(`"fence_generation":2`), []byte(`"fence_generation":3`), 1)
	if bytes.Equal(changed, data) {
		t.Fatal("fixture did not change plan payload")
	}
	if err = os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadPlan(context.Background(), dir, plan.PlanHash); err == nil {
		t.Fatal("LoadPlan accepted content changed under the approved hash")
	}
}

func TestLoadPlanRejectsFIFOWithoutBlocking(t *testing.T) {
	dir := privatePlanDir(t)
	fifoPath := filepath.Join(dir, "fifo.plan")
	if err := syscall.Mkfifo(fifoPath, 0600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := LoadPlan(ctx, dir, strings.Repeat("a", 64))
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("LoadPlan accepted a FIFO plan file")
		}
	case <-ctx.Done():
		t.Fatal("LoadPlan blocked opening a FIFO past its context deadline")
	}
}

func TestCreatePlanRejectsWritableHigherAncestor(t *testing.T) {
	base := recoveryTestTempDir(t)
	unsafe := filepath.Join(base, "writable")
	if err := os.Mkdir(unsafe, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(unsafe, 0777); err != nil {
		t.Fatal(err)
	}
	parent := filepath.Join(unsafe, "protected")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(parent, "plans")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlan(context.Background(), dir, validPlanInput()); !errors.Is(err, ErrPlanUntrustedDir) {
		t.Fatalf("CreatePlan accepted writable higher ancestor: %v", err)
	}
}

func TestLoadPlanRejectsDuplicateUnknownAliasAndTrailingJSON(t *testing.T) {
	mutations := map[string]func(string, []byte) []byte{
		"duplicate key": func(hash string, data []byte) []byte {
			return bytes.Replace(data, []byte(`"payload":`), []byte(`"plan_hash":"`+hash+`","payload":`), 1)
		},
		"unknown key": func(_ string, data []byte) []byte {
			return bytes.Replace(data, []byte(`"payload":`), []byte(`"unexpected":true,"payload":`), 1)
		},
		"case alias": func(_ string, data []byte) []byte {
			return bytes.Replace(data, []byte(`"plan_hash":`), []byte(`"PlanHash":`), 1)
		},
		"nested duplicate": func(_ string, data []byte) []byte {
			return bytes.Replace(data, []byte(`"accounts":[`), []byte(`"accounts":[],"accounts":[`), 1)
		},
		"trailing data": func(_ string, data []byte) []byte {
			return append(data, []byte(`{}`)...)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			dir := privatePlanDir(t)
			plan, err := CreatePlan(context.Background(), dir, validPlanInput())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, plan.PlanHash+".json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			mutated := mutate(plan.PlanHash, data)
			if bytes.Equal(mutated, data) {
				t.Fatal("mutation did not change fixture")
			}
			if err = os.WriteFile(path, mutated, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err = LoadPlan(context.Background(), dir, plan.PlanHash); !errors.Is(err, ErrPlanInvalid) {
				t.Fatalf("LoadPlan error = %v, want ErrPlanInvalid", err)
			}
		})
	}
}

func TestCreatePlanNeverOverwritesExistingArtifactOrFollowsSymlink(t *testing.T) {
	t.Run("corrupt existing file", func(t *testing.T) {
		dir := privatePlanDir(t)
		plan, err := newPlan(validPlanInput())
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, plan.PlanHash+".json")
		original := []byte("operator data must remain unchanged\n")
		if err = os.WriteFile(path, original, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlan(context.Background(), dir, validPlanInput()); !errors.Is(err, ErrPlanExists) {
			t.Fatalf("CreatePlan error = %v, want ErrPlanExists", err)
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, original) {
			t.Fatalf("existing bytes changed: %q, %v", got, err)
		}
	})

	t.Run("symlink target", func(t *testing.T) {
		dir := privatePlanDir(t)
		plan, err := newPlan(validPlanInput())
		if err != nil {
			t.Fatal(err)
		}
		victim := filepath.Join(recoveryTestTempDir(t), "victim")
		original := []byte("outside plan root")
		if err = os.WriteFile(victim, original, 0600); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, plan.PlanHash+".json")
		if err = os.Symlink(victim, path); err != nil {
			t.Fatal(err)
		}
		if _, err = CreatePlan(context.Background(), dir, validPlanInput()); err == nil {
			t.Fatal("CreatePlan accepted symlink plan path")
		}
		got, err := os.ReadFile(victim)
		if err != nil || !bytes.Equal(got, original) {
			t.Fatalf("symlink target changed: %q, %v", got, err)
		}
	})
}

func TestCreatePlanRejectsSymlinkDirectoryAndCanceledContextBeforeIO(t *testing.T) {
	parent := recoveryTestTempDir(t)
	realDir := filepath.Join(parent, "real")
	if err := os.Mkdir(realDir, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(parent, "alias")
	if err := os.Symlink(realDir, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := CreatePlan(context.Background(), alias, validPlanInput()); !errors.Is(err, ErrPlanUntrustedDir) {
		t.Fatalf("CreatePlan symlink directory error = %v", err)
	}

	missing := filepath.Join(parent, "missing")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CreatePlan(ctx, missing, validPlanInput()); !errors.Is(err, context.Canceled) {
		t.Fatalf("CreatePlan canceled error = %v", err)
	}
	if _, err := os.Lstat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled create touched target directory: %v", err)
	}
}

func TestCreatePlanRejectsMalformedInputs(t *testing.T) {
	cases := map[string]func(*PlanInput){
		"uppercase snapshot digest": func(in *PlanInput) { in.SnapshotSHA256 = strings.Repeat("A", 64) },
		"older fence generation":    func(in *PlanInput) { in.FenceGeneration = 0 },
		"duplicate account":         func(in *PlanInput) { in.Accounts = []string{"same", "same"} },
		"activate all alias":        func(in *PlanInput) { in.Accounts = []string{"all"} },
		"zero provider observation": func(in *PlanInput) { in.ProviderObserved = time.Time{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			in := validPlanInput()
			mutate(&in)
			if _, err := newPlan(in); !errors.Is(err, ErrPlanInvalid) {
				t.Fatalf("newPlan error = %v, want ErrPlanInvalid", err)
			}
		})
	}
}
