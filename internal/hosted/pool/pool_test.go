package pool_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/hosted/provision"
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
		brainRoot := filepath.Join(root, id)
		if err := os.Mkdir(brainRoot, 0700); err != nil {
			t.Fatal(err)
		}
		initializePoolTestGit(t, brainRoot)
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

func TestProvisionedBrainCommitsConfigOnFirstOpenAndRetry(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, interrupted := range []bool{false, true} {
		name := "first_open"
		if interrupted {
			name = "config_written_before_interruption"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			db, err := hoststore.Open(filepath.Join(dir, "db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := db.Close(); err != nil {
					t.Error(err)
				}
			})
			a, err := db.CreateAccount(ctx, "pool-init@example.com")
			if err != nil {
				t.Fatal(err)
			}
			brains := filepath.Join(dir, "brains")
			b, err := (&provision.Provisioner{Store: db, BrainsRoot: brains}).Provision(ctx, a.ID)
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(brains, b.ID)
			if interrupted {
				c := config.Default()
				c.Models.Embedding = (embedding{}).ModelVersion()
				if err := c.Save(filepath.Join(root, config.FileName)); err != nil {
					t.Fatal(err)
				}
			}
			p, err := pool.New(pool.Config{BrainsRoot: brains, MaxOpen: 1, MaxInFlight: 1, Embedder: embedding{}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := p.Close(); err != nil {
					t.Error(err)
				}
			})
			runtime, release, err := p.Acquire(ctx, b.ID)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			remembered := false
			for _, tool := range runtime.Tools {
				if tool.Name == "remember" {
					result, e := tool.Handler(ctx, json.RawMessage(`{"fact":"Canonical initialization marker is violet","provenance":"fixture"}`))
					if e != nil || result.IsError {
						t.Fatalf("remember: %+v %v", result, e)
					}
					remembered = true
				}
			}
			release()
			if !remembered {
				t.Fatal("remember tool missing")
			}
			if err := p.FlushAll(); err != nil {
				t.Fatalf("flush with isolated Git config: %v", err)
			}
			author, err := exec.CommandContext(ctx, "git", "-C", root, "log", "-1", "--format=%an <%ae>").Output()
			if err != nil || strings.TrimSpace(string(author)) != "Serenity Hosted <hosted@serenity.sire.run>" {
				t.Fatalf("unexpected hosted commit author: %q %v", author, err)
			}
			out, err := exec.CommandContext(ctx, "git", "-C", root, "show", "HEAD:"+config.FileName).CombinedOutput()
			if err != nil || !strings.Contains(string(out), (embedding{}).ModelVersion()) {
				t.Fatalf("model config not committed: %v %s", err, out)
			}
			out, err = exec.CommandContext(ctx, "git", "-C", root, "status", "--porcelain").CombinedOutput()
			if err != nil || len(out) != 0 {
				t.Fatalf("first open left dirty canonical state: %v %s", err, out)
			}
		})
	}
}

func TestPoolConfigCommitPreservesIdentityAndUnrelatedIndexEntries(t *testing.T) {
	_, _, brainsRoot, brain := provisionedPoolFixture(t)
	root := filepath.Join(brainsRoot, brain.ID)
	unrelated := filepath.Join(root, "unrelated-staged-file.txt")
	if err := os.WriteFile(unrelated, []byte("must remain staged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stage := exec.Command("git", "-C", root, "add", "--", "unrelated-staged-file.txt")
	if output, err := stage.CombinedOutput(); err != nil {
		t.Fatalf("stage unrelated fixture: %v: %s", err, output)
	}
	p, err := pool.New(pool.Config{BrainsRoot: brainsRoot, MaxOpen: 1, MaxInFlight: 1, Embedder: embedding{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Error(err)
		}
	})
	_, release, err := p.Acquire(context.Background(), brain.ID)
	if err != nil {
		t.Fatal(err)
	}
	release()

	identity := exec.Command("git", "-C", root, "log", "-1", "--format=%an <%ae>|%cn <%ce>")
	if output, err := identity.CombinedOutput(); err != nil {
		t.Fatalf("read config commit identity: %v: %s", err, output)
	} else if got, want := strings.TrimSpace(string(output)), "Serenity Hosted <hosted@serenity.sire.run>|Serenity Hosted <hosted@serenity.sire.run>"; got != want {
		t.Fatalf("config commit identity = %q, want %q", got, want)
	}
	show := exec.Command("git", "-C", root, "show", "HEAD:unrelated-staged-file.txt")
	if output, err := show.CombinedOutput(); err == nil {
		t.Fatalf("config commit unexpectedly included unrelated staged file: %s", output)
	}
	status := exec.Command("git", "-C", root, "status", "--porcelain")
	if output, err := status.CombinedOutput(); err != nil {
		t.Fatalf("read staged fixture status: %v: %s", err, output)
	} else if !strings.Contains(string(output), "A  unrelated-staged-file.txt") {
		t.Fatalf("unrelated staged file was not preserved in the index: %q", output)
	}
}

func TestPoolRejectsMissingOrUnsafeGitBeforeSideEffects(t *testing.T) {
	for _, test := range []string{"missing", "symlink", "regular_file"} {
		t.Run(test, func(t *testing.T) {
			ctx := context.Background()
			db, accountID, brainsRoot, brain := provisionedPoolFixture(t)
			root := filepath.Join(brainsRoot, brain.ID)
			gitDir := filepath.Join(root, ".git")
			var linkedTarget string
			switch test {
			case "missing":
				if err := os.RemoveAll(gitDir); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.RemoveAll(gitDir); err != nil {
					t.Fatal(err)
				}
				linkedTarget = filepath.Join(t.TempDir(), "external-git")
				if err := os.Mkdir(linkedTarget, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(linkedTarget, gitDir); err != nil {
					t.Fatal(err)
				}
			case "regular_file":
				if err := os.RemoveAll(gitDir); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(gitDir, []byte("not a git directory\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}

			before := snapshotTree(t, root)
			var beforeLinkedTarget map[string]string
			if linkedTarget != "" {
				beforeLinkedTarget = snapshotTree(t, linkedTarget)
			}
			p, err := pool.New(pool.Config{BrainsRoot: brainsRoot, MaxOpen: 1, MaxInFlight: 1, Embedder: embedding{}})
			if err != nil {
				t.Fatal(err)
			}
			_, release, openErr := p.Acquire(ctx, brain.ID)
			if release != nil {
				release()
			}
			if closeErr := p.Close(); closeErr != nil {
				t.Errorf("close pool: %v", closeErr)
			}
			if openErr == nil {
				t.Errorf("Acquire accepted %s .git metadata", test)
			}
			if after := snapshotTree(t, root); !reflect.DeepEqual(after, before) {
				t.Errorf("failed Git preflight changed brain tree:\nbefore=%v\nafter=%v", before, after)
			}
			if linkedTarget != "" {
				if after := snapshotTree(t, linkedTarget); !reflect.DeepEqual(after, beforeLinkedTarget) {
					t.Errorf("failed Git preflight changed linked external tree:\nbefore=%v\nafter=%v", beforeLinkedTarget, after)
				}
			}
			persisted, err := db.BrainByID(ctx, accountID, brain.ID)
			if err != nil {
				t.Fatal(err)
			}
			if persisted.State != "ready" {
				t.Errorf("brain state after failed open = %q, want ready", persisted.State)
			}
		})
	}
}

func TestPoolRefusesPreCanceledOpenBeforeMutation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	db, accountID, brainsRoot, brain := provisionedPoolFixture(t)
	root := filepath.Join(brainsRoot, brain.ID)
	before := snapshotTree(t, root)
	p, err := pool.New(pool.Config{BrainsRoot: brainsRoot, MaxOpen: 1, MaxInFlight: 1, Embedder: embedding{}})
	if err != nil {
		t.Fatal(err)
	}
	_, release, openErr := p.Acquire(ctx, brain.ID)
	if release != nil {
		release()
	}
	if closeErr := p.Close(); closeErr != nil {
		t.Errorf("close pool: %v", closeErr)
	}
	if !errors.Is(openErr, context.Canceled) {
		t.Errorf("Acquire with canceled context = %v, want context.Canceled", openErr)
	}
	if after := snapshotTree(t, root); !reflect.DeepEqual(after, before) {
		t.Errorf("canceled open changed brain tree:\nbefore=%v\nafter=%v", before, after)
	}
	persisted, err := db.BrainByID(context.Background(), accountID, brain.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.State != "ready" {
		t.Errorf("brain state after canceled open = %q, want ready", persisted.State)
	}
}

func TestPoolIgnoresInheritedGitDir(t *testing.T) {
	_, _, brainsRoot, brain := provisionedPoolFixture(t)
	root := filepath.Join(brainsRoot, brain.ID)
	externalGitDir := filepath.Join(t.TempDir(), "external-git-dir")
	if err := os.Mkdir(externalGitDir, 0700); err != nil {
		t.Fatal(err)
	}
	beforeExternal := snapshotTree(t, externalGitDir)
	t.Setenv("GIT_DIR", externalGitDir)
	p, err := pool.New(pool.Config{BrainsRoot: brainsRoot, MaxOpen: 1, MaxInFlight: 1, Embedder: embedding{}})
	if err != nil {
		t.Fatal(err)
	}
	runtime, release, openErr := p.Acquire(context.Background(), brain.ID)
	if release != nil {
		release()
	}
	if closeErr := p.Close(); closeErr != nil {
		t.Errorf("close pool: %v", closeErr)
	}
	if openErr != nil {
		t.Errorf("Acquire with unrelated GIT_DIR = %v, want local brain open", openErr)
	}
	if runtime != nil && runtime.Root != root {
		t.Errorf("runtime root = %q, want %q", runtime.Root, root)
	}
	t.Setenv("GIT_DIR", "")
	if after := snapshotTree(t, externalGitDir); !reflect.DeepEqual(after, beforeExternal) {
		t.Errorf("pool Git commands changed inherited external GIT_DIR tree:\nbefore=%v\nafter=%v", beforeExternal, after)
	}
}

func TestPoolIgnoresInheritedGitConfig(t *testing.T) {
	_, _, brainsRoot, brain := provisionedPoolFixture(t)
	hooks := filepath.Join(t.TempDir(), "injected-hooks")
	if err := os.Mkdir(hooks, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "unexpected-hook-run")
	preCommit := "#!/bin/sh\n: > " + marker + "\n"
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte(preCommit), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.hooksPath")
	t.Setenv("GIT_CONFIG_VALUE_0", hooks)
	p, err := pool.New(pool.Config{BrainsRoot: brainsRoot, MaxOpen: 1, MaxInFlight: 1, Embedder: embedding{}})
	if err != nil {
		t.Fatal(err)
	}
	_, release, openErr := p.Acquire(context.Background(), brain.ID)
	if release != nil {
		release()
	}
	if closeErr := p.Close(); closeErr != nil {
		t.Errorf("close pool: %v", closeErr)
	}
	if openErr != nil {
		t.Fatalf("Acquire with inherited config injection: %v", openErr)
	}
	if _, err := os.Lstat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("inherited core.hooksPath executed pre-commit hook; marker stat err=%v", err)
	}
}

func provisionedPoolFixture(t *testing.T) (*hoststore.Store, string, string, hoststore.Brain) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	db, err := hoststore.Open(filepath.Join(root, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	account, err := db.CreateAccount(ctx, "pool-gitrun@example.test")
	if err != nil {
		t.Fatal(err)
	}
	brainsRoot := filepath.Join(root, "brains")
	brain, err := (&provision.Provisioner{Store: db, BrainsRoot: brainsRoot}).Provision(ctx, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	return db, account.ID, brainsRoot, brain
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		value := info.Mode().String()
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			value += ":" + target
		case info.IsDir():
		case info.Mode().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			digest := sha256.Sum256(data)
			value += ":" + fmt.Sprintf("%x", digest)
		default:
			return fmt.Errorf("unexpected fixture file type at %s", path)
		}
		snapshot[rel] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func initializePoolTestGit(t *testing.T, root string) {
	t.Helper()
	commands := [][]string{
		{"init", "--initial-branch=main"},
		{"add", "--", ".gitignore"},
		{"-c", "user.name=Serenity Hosted", "-c", "user.email=hosted@serenity.sire.run", "commit", "--only", "-m", "Initialize hosted brain", "--", ".gitignore"},
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".serenity/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range commands {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("prepare canonical Git fixture with %v: %v: %s", args, err, output)
		}
	}
}

func TestPreCanceledWarmAcquireKeepsRuntimeAndOwnership(t *testing.T) {
	for _, tc := range []struct {
		name     string
		idleTTL  time.Duration
		existing bool
		other    bool
	}{
		{name: "cached acquire", idleTTL: time.Hour},
		{name: "cached existing acquire", idleTTL: time.Hour, existing: true},
		{name: "idle eviction", idleTTL: time.Nanosecond, other: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			ids := []string{hoststore.ID(), hoststore.ID()}
			for _, id := range ids {
				path := filepath.Join(root, id)
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				initializePoolTestGit(t, path)
			}
			p, err := pool.New(pool.Config{BrainsRoot: root, MaxOpen: 1, MaxInFlight: 2, IdleTimeout: tc.idleTTL, Embedder: embedding{}})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := p.Close(); err != nil {
					t.Error(err)
				}
			}()
			runtime, release, err := p.Acquire(context.Background(), ids[0])
			if err != nil {
				t.Fatal(err)
			}
			release()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			requested := ids[0]
			if tc.other {
				requested = ids[1]
			}
			var got *pool.Runtime
			var releaseCanceled func()
			if tc.existing {
				got, releaseCanceled, err = p.AcquireExisting(ctx, requested)
			} else {
				got, releaseCanceled, err = p.Acquire(ctx, requested)
			}
			if releaseCanceled != nil {
				releaseCanceled()
			}
			if !errors.Is(err, context.Canceled) || got != nil {
				t.Fatalf("pre-canceled warm acquire returned runtime=%v err=%v", got != nil, err)
			}
			kept, releaseKept, err := p.AcquireExisting(context.Background(), ids[0])
			if err != nil {
				t.Fatalf("pre-canceled request evicted warm runtime: %v", err)
			}
			defer releaseKept()
			if kept != runtime {
				t.Fatal("pre-canceled request replaced warm runtime")
			}
			if owner, err := writer.AcquireBrain(runtime.Root); err == nil {
				_ = owner.Close()
				t.Fatal("pre-canceled request released runtime ownership")
			}
		})
	}
}
