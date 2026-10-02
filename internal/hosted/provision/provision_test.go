package provision_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	if out, err := exec.CommandContext(ctx, "git", "-C", filepath.Join(p.BrainsRoot, id), "rev-parse", "--verify", "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("ready brain has no canonical commit: %v %s", err, out)
	}
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

func TestInterruptedCanonicalInitializationPreservesOtherFiles(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	a, err := db.CreateAccount(ctx, "initialization@example.com")
	if err != nil {
		t.Fatal(err)
	}
	id := store.ID()
	if _, err = db.InsertBrain(ctx, a.ID, id, id, "allocating", time.Now()); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "brains", id)
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return string(out)
	}
	git("init", "--initial-branch=main")
	if err = os.WriteFile(filepath.Join(root, "unrelated.txt"), []byte("preserve me"), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "--", "unrelated.txt")
	// A caller's Git environment must not redirect owned canonical reads or
	// override the fixed identity used for the baseline commit.
	badGitDir := filepath.Join(dir, "foreign-git-dir")
	if err = os.Mkdir(badGitDir, 0700); err != nil {
		t.Fatal(err)
	}
	originalEnv := make(map[string]string)
	originallySet := make(map[string]bool)
	for key, value := range map[string]string{
		"GIT_DIR":             badGitDir,
		"GIT_CONFIG_COUNT":    "1",
		"GIT_CONFIG_KEY_0":    "user.name",
		"GIT_CONFIG_VALUE_0":  "Inherited Attacker",
		"GIT_AUTHOR_NAME":     "Inherited Author",
		"GIT_AUTHOR_EMAIL":    "inherited-author@example.invalid",
		"GIT_COMMITTER_NAME":  "Inherited Committer",
		"GIT_COMMITTER_EMAIL": "inherited-committer@example.invalid",
	} {
		originalEnv[key], originallySet[key] = os.LookupEnv(key)
		t.Setenv(key, value)
	}
	restoreGitEnv := func() {
		for key, value := range originalEnv {
			if originallySet[key] {
				_ = os.Setenv(key, value)
			} else {
				_ = os.Unsetenv(key)
			}
		}
	}
	p := &provision.Provisioner{Store: db, BrainsRoot: filepath.Join(dir, "brains")}
	b, err := p.Provision(ctx, a.ID)
	restoreGitEnv()
	if err != nil || b.State != "ready" {
		t.Fatalf("brain=%+v err=%v", b, err)
	}
	files := git("ls-tree", "--name-only", "HEAD")
	if strings.TrimSpace(files) != ".gitignore" {
		t.Fatalf("committed unrelated data: %q", files)
	}
	identity := strings.TrimSpace(git("show", "-s", "--format=%an%n%ae%n%cn%n%ce", "HEAD"))
	if identity != "Serenity Hosted\nhosted@serenity.sire.run\nSerenity Hosted\nhosted@serenity.sire.run" {
		t.Fatalf("canonical baseline identity = %q", identity)
	}
	if name := strings.TrimSpace(git("config", "--local", "--get", "user.name")); name != "Serenity Hosted" {
		t.Fatalf("persistent canonical writer name = %q", name)
	}
	if email := strings.TrimSpace(git("config", "--local", "--get", "user.email")); email != "hosted@serenity.sire.run" {
		t.Fatalf("persistent canonical writer email = %q", email)
	}
	if status := git("status", "--porcelain"); !strings.Contains(status, "A  unrelated.txt") {
		t.Fatalf("lost staged file: %q", status)
	}
	head := git("rev-parse", "HEAD")
	if _, err = p.Provision(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if next := git("rev-parse", "HEAD"); next != head {
		t.Fatal("retry changed canonical history")
	}
}

func TestProvisionValidCanonicalReadsIgnoreInheritedGitDir(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e := db.Close(); e != nil {
			t.Error(e)
		}
	})
	account, err := db.CreateAccount(ctx, "valid-canonical@example.test")
	if err != nil {
		t.Fatal(err)
	}
	id := store.ID()
	if _, err = db.InsertBrain(ctx, account.ID, id, id, "allocating", time.Now()); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "brains", id)
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.name", "Fixture"},
		{"config", "user.email", "fixture@example.test"},
	} {
		if output, e := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...).CombinedOutput(); e != nil {
			t.Fatalf("git %v: %v: %s", args, e, output)
		}
	}
	if err = os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".serenity/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "--", ".gitignore"}, {"commit", "-m", "canonical fixture"}} {
		if output, e := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...).CombinedOutput(); e != nil {
			t.Fatalf("git %v: %v: %s", args, e, output)
		}
	}
	headBefore, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	badGitDir := filepath.Join(dir, "foreign-git-dir")
	if err = os.Mkdir(badGitDir, 0700); err != nil {
		t.Fatal(err)
	}
	oldGitDir, wasSet := os.LookupEnv("GIT_DIR")
	if err = os.Setenv("GIT_DIR", badGitDir); err != nil {
		t.Fatal(err)
	}
	p := &provision.Provisioner{Store: db, BrainsRoot: filepath.Join(dir, "brains")}
	brain, provisionErr := p.Provision(ctx, account.ID)
	if wasSet {
		_ = os.Setenv("GIT_DIR", oldGitDir)
	} else {
		_ = os.Unsetenv("GIT_DIR")
	}
	if provisionErr != nil || brain.State != "ready" {
		t.Fatalf("valid committed brain=%+v err=%v", brain, provisionErr)
	}
	headAfter, err := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil || string(headAfter) != string(headBefore) {
		t.Fatalf("valid canonical retry changed HEAD: err=%v before=%q after=%q", err, headBefore, headAfter)
	}
	baseline, err := exec.CommandContext(ctx, "git", "-C", root, "show", "HEAD:.gitignore").Output()
	if err != nil || string(baseline) != ".serenity/\n" {
		t.Fatalf("committed baseline after retry = %q, err=%v", baseline, err)
	}
}

func TestCanceledCanonicalCommitLeavesAllocatingBrainRetryable(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e := db.Close(); e != nil {
			t.Error(e)
		}
	})
	account, err := db.CreateAccount(ctx, "canceled-canonical@example.test")
	if err != nil {
		t.Fatal(err)
	}
	id := store.ID()
	if _, err = db.InsertBrain(ctx, account.ID, id, id, "allocating", time.Now()); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "brains", id)
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	started := filepath.Join(bin, "commit-started")
	script := "#!/bin/sh\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$arg\" = commit ]; then : > \"$HOSTED_PROVISION_STARTED\"; exec /bin/sleep 60; fi\n" +
		"done\n" +
		"exec \"$HOSTED_PROVISION_REAL_GIT\" \"$@\"\n"
	if err = os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+oldPath)
	t.Setenv("HOSTED_PROVISION_REAL_GIT", realGit)
	t.Setenv("HOSTED_PROVISION_STARTED", started)
	p := &provision.Provisioner{Store: db, BrainsRoot: filepath.Join(dir, "brains")}
	callCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		_, e := p.Provision(callCtx, account.ID)
		done <- e
	}()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		if _, e := os.Stat(started); e == nil {
			break
		}
		select {
		case <-ticker.C:
		case <-timeout.C:
			cancel()
			t.Fatal("canonical commit child did not start")
		}
	}
	cancel()
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("canceled canonical commit unexpectedly succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canonical commit did not stop after cancellation")
	}
	if err = os.Setenv("PATH", oldPath); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = db.DB().QueryRowContext(ctx, `SELECT state FROM brains WHERE id=?`, id).Scan(&state); err != nil || state != "allocating" {
		t.Fatalf("canceled brain state=%s err=%v", state, err)
	}
	if status, e := exec.CommandContext(ctx, "git", "-C", root, "status", "--porcelain").CombinedOutput(); e != nil || strings.TrimSpace(string(status)) != "A  .gitignore" {
		t.Fatalf("canceled canonical retry state = %q err=%v", status, e)
	}
	brain, err := p.Provision(ctx, account.ID)
	if err != nil || brain.State != "ready" {
		t.Fatalf("retry after canceled commit = %+v err=%v", brain, err)
	}
}

func TestProvisionRefusesSymlinkedGit(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	a, err := db.CreateAccount(ctx, "unsafe-init@example.com")
	if err != nil {
		t.Fatal(err)
	}
	id := store.ID()
	if _, err = db.InsertBrain(ctx, a.ID, id, id, "allocating", time.Now()); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "brains", id)
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	victim := t.TempDir()
	if err = os.Symlink(victim, filepath.Join(root, ".git")); err != nil {
		t.Fatal(err)
	}
	p := &provision.Provisioner{Store: db, BrainsRoot: filepath.Join(dir, "brains")}
	if _, err = p.Provision(ctx, a.ID); err == nil {
		t.Fatal("unsafe Git accepted")
	}
	var state string
	if err = db.DB().QueryRowContext(ctx, `SELECT state FROM brains WHERE id=?`, id).Scan(&state); err != nil || state != "allocating" {
		t.Fatalf("state=%s err=%v", state, err)
	}
	entries, err := os.ReadDir(victim)
	if err != nil || len(entries) != 0 {
		t.Fatalf("victim changed: %v %v", entries, err)
	}
}

func TestProvisionRejectsExistingCommitWithoutBaseline(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	a, err := db.CreateAccount(ctx, "missing-baseline@example.com")
	if err != nil {
		t.Fatal(err)
	}
	id := store.ID()
	if _, err = db.InsertBrain(ctx, a.ID, id, id, "allocating", time.Now()); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(dir, "brains", id)
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "--initial-branch=main"}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.com", "commit", "--allow-empty", "-m", "unrelated commit"}} {
		if out, err := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
	}
	p := &provision.Provisioner{Store: db, BrainsRoot: filepath.Join(dir, "brains")}
	if _, err = p.Provision(ctx, a.ID); err == nil {
		t.Fatal("commit without baseline marked ready")
	}
	var state string
	if err = db.DB().QueryRowContext(ctx, `SELECT state FROM brains WHERE id=?`, id).Scan(&state); err != nil || state != "allocating" {
		t.Fatalf("state=%s err=%v", state, err)
	}
}

func TestProvisionRejectsInvalidUnbornHeadWithoutRepair(t *testing.T) {
	tests := []struct {
		name       string
		branch     string
		head       string
		mainRef    string
		packedRefs string
		headLink   bool
		regularGit bool
	}{
		{name: "non-main symbolic HEAD", branch: "other"},
		{name: "corrupt HEAD", branch: "main", head: "broken HEAD\n"},
		{name: "oversized HEAD", branch: "main", head: strings.Repeat("x", 129)},
		{name: "symlinked HEAD", branch: "main", headLink: true},
		{name: "detached missing-object HEAD", branch: "main", head: strings.Repeat("1", 40) + "\n"},
		{name: "malformed main ref", branch: "main", mainRef: "not-an-object\n"},
		{name: "dangling main ref", branch: "main", mainRef: strings.Repeat("1", 40) + "\n"},
		{name: "malformed packed main ref", branch: "main", packedRefs: "not-an-object refs/heads/main\n"},
		{name: "dangling packed main ref", branch: "main", packedRefs: strings.Repeat("1", 40) + " refs/heads/main\n"},
		{name: "regular .git file", regularGit: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			db, err := store.Open(filepath.Join(dir, "db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if e := db.Close(); e != nil {
					t.Error(e)
				}
			})
			account, err := db.CreateAccount(ctx, "invalid-head@example.test")
			if err != nil {
				t.Fatal(err)
			}
			id := store.ID()
			if _, err = db.InsertBrain(ctx, account.ID, id, id, "allocating", time.Now()); err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(dir, "brains", id)
			if err = os.MkdirAll(root, 0700); err != nil {
				t.Fatal(err)
			}
			if tt.regularGit {
				if err = os.WriteFile(filepath.Join(root, ".git"), []byte("not a git directory\n"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if output, e := exec.CommandContext(ctx, "git", "-C", root, "init", "--initial-branch="+tt.branch).CombinedOutput(); e != nil {
					t.Fatalf("git init: %v: %s", e, output)
				}
				if err = os.WriteFile(filepath.Join(root, "unrelated.txt"), []byte("preserve staged data"), 0600); err != nil {
					t.Fatal(err)
				}
				if output, e := exec.CommandContext(ctx, "git", "-C", root, "add", "--", "unrelated.txt").CombinedOutput(); e != nil {
					t.Fatalf("stage unrelated file: %v: %s", e, output)
				}
				if tt.headLink {
					if err = os.Remove(filepath.Join(root, ".git", "HEAD")); err != nil {
						t.Fatal(err)
					}
					if err = os.WriteFile(filepath.Join(root, "head-target"), []byte("ref: refs/heads/main\n"), 0600); err != nil {
						t.Fatal(err)
					}
					if err = os.Symlink(filepath.Join(root, "head-target"), filepath.Join(root, ".git", "HEAD")); err != nil {
						t.Fatal(err)
					}
				} else if tt.head != "" {
					if err = os.WriteFile(filepath.Join(root, ".git", "HEAD"), []byte(tt.head), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if tt.mainRef != "" {
					ref := filepath.Join(root, ".git", "refs", "heads", "main")
					if err = os.WriteFile(ref, []byte(tt.mainRef), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if tt.packedRefs != "" {
					if err = os.WriteFile(filepath.Join(root, ".git", "packed-refs"), []byte(tt.packedRefs), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}

			unrelated := filepath.Join(root, "unrelated.txt")
			var unrelatedBefore, indexBefore, headBefore, configBefore, gitFileBefore, packedRefsBefore, mainRefBefore []byte
			packedRefsPath := filepath.Join(root, ".git", "packed-refs")
			mainRefPath := filepath.Join(root, ".git", "refs", "heads", "main")
			packedRefsExists := false
			mainRefExists := false
			if tt.regularGit {
				gitFileBefore, err = os.ReadFile(filepath.Join(root, ".git"))
				if err != nil {
					t.Fatal(err)
				}
			} else {
				_, packedRefsStatErr := os.Lstat(packedRefsPath)
				packedRefsExists = packedRefsStatErr == nil
				if packedRefsStatErr != nil && !errors.Is(packedRefsStatErr, os.ErrNotExist) {
					t.Fatal(packedRefsStatErr)
				}
				_, mainRefStatErr := os.Lstat(mainRefPath)
				mainRefExists = mainRefStatErr == nil
				if mainRefStatErr != nil && !errors.Is(mainRefStatErr, os.ErrNotExist) {
					t.Fatal(mainRefStatErr)
				}
				var unrelatedErr, indexErr, headErr, configErr error
				unrelatedBefore, unrelatedErr = os.ReadFile(unrelated)
				indexPath := filepath.Join(root, ".git", "index")
				indexBefore, indexErr = os.ReadFile(indexPath)
				headBefore, headErr = os.ReadFile(filepath.Join(root, ".git", "HEAD"))
				configBefore, configErr = os.ReadFile(filepath.Join(root, ".git", "config"))
				if packedRefsExists {
					packedRefsBefore, err = os.ReadFile(packedRefsPath)
					if err != nil {
						t.Fatal(err)
					}
				}
				if mainRefExists {
					mainRefBefore, err = os.ReadFile(mainRefPath)
					if err != nil {
						t.Fatal(err)
					}
				}
				if unrelatedErr != nil || indexErr != nil || headErr != nil || configErr != nil {
					t.Fatalf("fixture snapshots: unrelated=%v index=%v HEAD=%v config=%v", unrelatedErr, indexErr, headErr, configErr)
				}
			}
			p := &provision.Provisioner{Store: db, BrainsRoot: filepath.Join(dir, "brains")}
			if _, err = p.Provision(ctx, account.ID); err == nil {
				t.Fatal("invalid or non-main canonical HEAD was repaired")
			}
			var state string
			if err = db.DB().QueryRowContext(ctx, `SELECT state FROM brains WHERE id=?`, id).Scan(&state); err != nil || state != "allocating" {
				t.Fatalf("state=%s err=%v", state, err)
			}
			if _, err = os.Lstat(filepath.Join(root, ".gitignore")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("invalid HEAD was repaired with .gitignore: %v", err)
			}
			if !tt.regularGit {
				indexPath := filepath.Join(root, ".git", "index")
				indexAfter, e := os.ReadFile(indexPath)
				if e != nil || string(indexAfter) != string(indexBefore) {
					t.Fatalf("unrelated Git index changed: read=%v", e)
				}
				headAfter, e := os.ReadFile(filepath.Join(root, ".git", "HEAD"))
				if e != nil || string(headAfter) != string(headBefore) {
					t.Fatalf("HEAD changed: read=%v before=%q after=%q", e, headBefore, headAfter)
				}
				configAfter, e := os.ReadFile(filepath.Join(root, ".git", "config"))
				if e != nil || string(configAfter) != string(configBefore) {
					t.Fatalf("Git config changed: read=%v", e)
				}
				packedRefsAfter, e := os.ReadFile(packedRefsPath)
				if packedRefsExists && (e != nil || string(packedRefsAfter) != string(packedRefsBefore)) {
					t.Fatalf("packed refs changed: read=%v", e)
				}
				if !packedRefsExists && !errors.Is(e, os.ErrNotExist) {
					t.Fatalf("packed refs appeared after refusal: read=%v", e)
				}
				mainRefAfter, e := os.ReadFile(mainRefPath)
				if mainRefExists && (e != nil || string(mainRefAfter) != string(mainRefBefore)) {
					t.Fatalf("main ref changed: read=%v", e)
				}
				if !mainRefExists && !errors.Is(e, os.ErrNotExist) {
					t.Fatalf("main ref appeared after refusal: read=%v", e)
				}
				unrelatedAfter, e := os.ReadFile(unrelated)
				if e != nil || string(unrelatedAfter) != string(unrelatedBefore) {
					t.Fatalf("unrelated staged bytes changed: read=%v", e)
				}
			} else {
				gitFileAfter, e := os.ReadFile(filepath.Join(root, ".git"))
				if e != nil || string(gitFileAfter) != string(gitFileBefore) {
					t.Fatalf("regular .git file changed: read=%v", e)
				}
			}
		})
	}
}
