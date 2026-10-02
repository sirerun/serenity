package gateway

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/pool"
	hoststore "github.com/sirerun/serenity/internal/hosted/store"
)

func TestExportBundleUsesOwnedGitEnvironmentAndAllRefs(t *testing.T) {
	ctx := context.Background()
	g, brainPool, db, accountID, brainID, brainRoot := newLifecycleExportFixture(t)
	warmLifecycleRuntime(t, ctx, brainPool, brainID, brainRoot)
	other, err := db.CreateAccount(ctx, "other-export-owner@example.test")
	if err != nil {
		t.Fatal(err)
	}
	var denied bytes.Buffer
	if err = g.Export(ctx, other.ID, brainID, &denied); err == nil || denied.Len() != 0 {
		t.Fatalf("Export accepted another account or emitted bytes: err=%v bytes=%d", err, denied.Len())
	}

	marker, _ := installLifecycleGitShim(t, "pass")
	badGitDir := t.TempDir()
	oldGitDir, wasSet := os.LookupEnv("GIT_DIR")
	if err = os.Setenv("GIT_DIR", badGitDir); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	err = g.Export(ctx, accountID, brainID, &output)
	if wasSet {
		_ = os.Setenv("GIT_DIR", oldGitDir)
	} else {
		_ = os.Unsetenv("GIT_DIR")
	}
	if err != nil {
		t.Fatalf("Export under unrelated inherited GIT_DIR: %v", err)
	}
	invocation, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitN(string(invocation), "\n", 3)
	if len(lines) < 2 || lines[0] != brainRoot {
		t.Fatalf("bundle command cwd = %q, want authenticated pool root %q", lines[0], brainRoot)
	}
	if !strings.Contains(lines[1], "bundle create ") || !strings.HasSuffix(lines[1], " --all") || strings.Contains(lines[1], " -C ") {
		t.Fatalf("bundle argv = %q, want bound-root `bundle create <file> --all`", lines[1])
	}
	argv := strings.Fields(lines[1])
	bundleArg := argv[len(argv)-2]
	if _, err = os.Lstat(bundleArg); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary bundle remained after export: %v", err)
	}
	archive, err := zip.NewReader(bytes.NewReader(output.Bytes()), int64(output.Len()))
	if err != nil {
		t.Fatalf("open export zip: %v", err)
	}
	var bundle []byte
	for _, entry := range archive.File {
		if entry.Name != "brain.bundle" {
			continue
		}
		file, e := entry.Open()
		if e != nil {
			t.Fatal(e)
		}
		bundle, e = io.ReadAll(file)
		if e = errors.Join(e, file.Close()); e != nil {
			t.Fatal(e)
		}
	}
	if len(bundle) == 0 {
		t.Fatal("export omitted brain.bundle")
	}
	bundlePath := filepath.Join(t.TempDir(), "export.bundle")
	if err = os.WriteFile(bundlePath, bundle, 0600); err != nil {
		t.Fatal(err)
	}
	refs, err := exec.Command("git", "bundle", "list-heads", bundlePath).CombinedOutput()
	if err != nil {
		t.Fatalf("list exported bundle refs: %v: %s", err, refs)
	}
	if !strings.Contains(string(refs), "refs/heads/export/other") {
		t.Fatalf("export bundle omitted a non-default head: %s", refs)
	}
}

func TestExportBundleFailureIncludesOutputAndCleansTemporaryFiles(t *testing.T) {
	ctx := context.Background()
	g, brainPool, _, accountID, brainID, brainRoot := newLifecycleExportFixture(t)
	warmLifecycleRuntime(t, ctx, brainPool, brainID, brainRoot)
	marker, _ := installLifecycleGitShim(t, "fail")
	tmpRoot := filepath.Join(t.TempDir(), "private-tmp")
	if err := os.Mkdir(tmpRoot, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmpRoot)
	var output bytes.Buffer
	err := g.Export(ctx, accountID, brainID, &output)
	if err == nil || !strings.Contains(err.Error(), "fixture bundle failure") {
		t.Fatalf("bundle error did not retain child output: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("failed bundle emitted %d bytes", output.Len())
	}
	if entries, e := os.ReadDir(tmpRoot); e != nil || len(entries) != 0 {
		t.Fatalf("export temporary files remain: entries=%v err=%v", entries, e)
	}
	invocation, err := os.ReadFile(marker)
	if err != nil || !strings.HasPrefix(string(invocation), brainRoot+"\n") {
		t.Fatalf("bundle ran outside the authenticated pool root: %q err=%v", invocation, err)
	}
}

func TestExportBundleCancellationStopsChildAndCleansTemporaryFiles(t *testing.T) {
	baseCtx := context.Background()
	g, brainPool, _, accountID, brainID, brainRoot := newLifecycleExportFixture(t)
	warmLifecycleRuntime(t, baseCtx, brainPool, brainID, brainRoot)
	marker, started := installLifecycleGitShim(t, "wait")
	tmpRoot := filepath.Join(t.TempDir(), "private-tmp")
	if err := os.Mkdir(tmpRoot, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmpRoot)
	ctx, cancel := context.WithCancel(baseCtx)
	done := make(chan error, 1)
	go func() {
		var output bytes.Buffer
		done <- g.Export(ctx, accountID, brainID, &output)
	}()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		select {
		case <-ticker.C:
		case <-timeout.C:
			cancel()
			t.Fatal("bundle child did not start")
		}
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled export unexpectedly succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("export did not stop after caller cancellation")
	}
	if entries, err := os.ReadDir(tmpRoot); err != nil || len(entries) != 0 {
		t.Fatalf("canceled export temporary files remain: entries=%v err=%v", entries, err)
	}
	invocation, err := os.ReadFile(marker)
	if err != nil || !strings.HasPrefix(string(invocation), brainRoot+"\n") {
		t.Fatalf("canceled bundle ran outside the authenticated pool root: %q err=%v", invocation, err)
	}
}

func newLifecycleExportFixture(t *testing.T) (*Gateway, *pool.Pool, *hoststore.Store, string, string, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := hoststore.Open(filepath.Join(dir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e := db.Close(); e != nil {
			t.Error(e)
		}
	})
	account, err := db.CreateAccount(ctx, "export-lifecycle@example.test")
	if err != nil {
		t.Fatal(err)
	}
	brainID := hoststore.ID()
	if _, err = db.InsertBrain(ctx, account.ID, brainID, brainID, "ready", time.Now()); err != nil {
		t.Fatal(err)
	}
	brainsRoot := filepath.Join(dir, "brains")
	brainRoot := filepath.Join(brainsRoot, brainID)
	if err = os.MkdirAll(brainRoot, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Models.Embedding = journalTestEmbedder{}.ModelVersion()
	if err = cfg.Save(filepath.Join(brainRoot, config.FileName)); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(brainRoot, ".gitignore"), []byte(".serenity/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "--initial-branch=main"},
		{"config", "user.name", "Serenity Hosted"},
		{"config", "user.email", "hosted@serenity.sire.run"},
		{"add", "--", config.FileName, ".gitignore"},
		{"commit", "-m", "export fixture"},
		{"branch", "export/other"},
	} {
		if output, e := exec.CommandContext(ctx, "git", append([]string{"-C", brainRoot}, args...)...).CombinedOutput(); e != nil {
			t.Fatalf("git %v: %v: %s", args, e, output)
		}
	}
	brainPool, err := pool.New(pool.Config{MaxOpen: 1, MaxInFlight: 1, BrainsRoot: brainsRoot, Embedder: journalTestEmbedder{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e := brainPool.Close(); e != nil {
			t.Error(e)
		}
	})
	return &Gateway{Issuer: &credential.Issuer{Store: db}, Pool: brainPool}, brainPool, db, account.ID, brainID, brainRoot
}

func warmLifecycleRuntime(t *testing.T, ctx context.Context, brainPool *pool.Pool, brainID, brainRoot string) {
	t.Helper()
	activeRuntime, release, err := brainPool.Acquire(ctx, brainID)
	if err != nil {
		t.Fatal(err)
	}
	if activeRuntime.Root != brainRoot {
		t.Fatalf("pool runtime root = %q, want authenticated brain root %q", activeRuntime.Root, brainRoot)
	}
	release()
}

func installLifecycleGitShim(t *testing.T, mode string) (markerPath, startedPath string) {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	markerPath = filepath.Join(bin, "git-invocation.txt")
	startedPath = filepath.Join(bin, "git-started")
	script := "#!/bin/sh\n" +
		"bundle_create=0\nprevious=\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$previous\" = bundle ] && [ \"$arg\" = create ]; then bundle_create=1; fi\n" +
		"  previous=$arg\n" +
		"done\n" +
		"if [ \"$bundle_create\" = 1 ]; then\n" +
		"  /bin/pwd > \"$HOSTED_EXPORT_MARKER\"\n" +
		"  printf '%s\\n' \"$*\" >> \"$HOSTED_EXPORT_MARKER\"\n" +
		"  case \"$HOSTED_EXPORT_MODE\" in\n" +
		"    fail) printf '%s\\n' 'fixture bundle failure' >&2; exit 23 ;;\n" +
		"    wait) : > \"$HOSTED_EXPORT_STARTED\"; exec /bin/sleep 60 ;;\n" +
		"  esac\n" +
		"fi\n" +
		"exec \"$HOSTED_EXPORT_REAL_GIT\" \"$@\"\n"
	if err = os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOSTED_EXPORT_MODE", mode)
	t.Setenv("HOSTED_EXPORT_MARKER", markerPath)
	t.Setenv("HOSTED_EXPORT_STARTED", startedPath)
	t.Setenv("HOSTED_EXPORT_REAL_GIT", realGit)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skipf("test Git shim requires a Unix-like shell, got %s", runtime.GOOS)
	}
	return markerPath, startedPath
}
