//go:build unix

package gbrain

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
)

func resumeBrain(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.name", "Fixture"}, {"config", "user.email", "fixture@example.invalid"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v: %s", err, out)
		}
	}
	return root
}
func sourceFixture(t *testing.T) string {
	t.Helper()
	source, err := filepath.Abs("../../../testdata/gbrain-fixture")
	if err != nil {
		t.Fatal(err)
	}
	return source
}
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = string(raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
func gitOutput(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func TestImportSIGKILLResume(t *testing.T) {
	source := sourceFixture(t)
	normal := resumeBrain(t)
	if _, err := Import(context.Background(), source, normal, config.Default()); err != nil {
		t.Fatal(err)
	}
	want := snapshot(t, normal)
	for _, stage := range []string{"page_published", "page_committed", "checkpoint_staged"} {
		t.Run(stage, func(t *testing.T) {
			root := resumeBrain(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestImportCrashHelper$")
			cmd.Env = append(os.Environ(), "SERENITY_IMPORT_CRASH_HELPER=1", "SERENITY_IMPORT_CRASH_STAGE="+stage, "SERENITY_IMPORT_CRASH_ROOT="+root, "SERENITY_IMPORT_CRASH_SOURCE="+source)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			finished := false
			t.Cleanup(func() {
				if !finished {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			})
			scanner := bufio.NewScanner(stdout)
			ready := false
			for scanner.Scan() {
				if scanner.Text() == "READY "+stage {
					ready = true
					break
				}
			}
			if !ready {
				_ = cmd.Wait()
				finished = true
				t.Fatalf("helper never reached %s: %v %s", stage, scanner.Err(), stderr.String())
			}
			if err := cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			err = cmd.Wait()
			finished = true
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				t.Fatalf("expected killed subprocess, got %v", err)
			}
			status, ok := exit.Sys().(syscall.WaitStatus)
			if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
				t.Fatalf("expected actual SIGKILL, got %v", exit)
			}
			// The first page is complete on disk at each cut point, but no checkpoint
			// has been published yet. A staged checkpoint must not masquerade as done.
			if _, err := os.Stat(filepath.Join(root, ".serenity", "import", "gbrain.json")); !os.IsNotExist(err) {
				t.Fatalf("checkpoint advanced before cut point: %v", err)
			}
			tracked := gitOutput(t, root, "ls-files", "brain/entities/person/ava.md")
			if (stage == "page_published") != (tracked == "") {
				t.Fatalf("unexpected commit state at %s: %q", stage, tracked)
			}
			result, err := Import(context.Background(), source, root, config.Default())
			if err != nil {
				t.Fatal(err)
			}
			if result.Pages != 2 || result.Claims != 8 {
				t.Fatalf("resume counts: %+v", result)
			}
			got := snapshot(t, root)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("resumed canonical/runtime bytes differ\ngot keys: %v\nwant keys: %v", keys(got), keys(want))
			}
			if strings.TrimSpace(gitOutput(t, root, "rev-list", "--count", "HEAD")) != "2" {
				t.Fatal("resume created duplicate commits")
			}
			if got := gitOutput(t, root, "ls-files", ".serenity/import"); got != "" {
				t.Fatalf("checkpoint committed as canonical data: %s", got)
			}
		})
	}
}
func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// This child uses the real importer; only the observer is test-only. Parent
// synchronization is an explicit stdout handshake, never a scheduling sleep.
func TestImportCrashHelper(t *testing.T) {
	if os.Getenv("SERENITY_IMPORT_CRASH_HELPER") != "1" {
		t.Skip("subprocess helper; exercised by TestImportSIGKILLResume")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, err := importObserved(ctx, os.Getenv("SERENITY_IMPORT_CRASH_SOURCE"), os.Getenv("SERENITY_IMPORT_CRASH_ROOT"), config.Default(), func(stage, page string) error {
		if stage == os.Getenv("SERENITY_IMPORT_CRASH_STAGE") {
			if _, err := fmt.Fprintln(os.Stdout, "READY "+stage); err != nil {
				return err
			}
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	})
	t.Fatalf("crash helper returned instead of being killed: %v", err)
}

func TestCompletedCheckpointSkipsWritesAndRetainsBothFenceRows(t *testing.T) {
	root := resumeBrain(t)
	source := sourceFixture(t)
	if _, err := Import(context.Background(), source, root, config.Default()); err != nil {
		t.Fatal(err)
	}
	result, err := importObserved(context.Background(), source, root, config.Default(), func(stage, page string) error { return fmt.Errorf("unexpected write stage %s for %s", stage, page) })
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped != 2 {
		t.Fatalf("did not skip checkpointed pages: %+v", result)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".serenity", "import", "gbrain.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cp checkpoint
	if err := json.Unmarshal(raw, &cp); err != nil {
		t.Fatal(err)
	}
	rows := cp.Pages["people/ava.md"].Rows
	if len(rows) != 7 || rows["facts#1"] == "" || rows["takes#1"] == "" || rows["facts#1"] == rows["takes#1"] {
		t.Fatalf("row checkpoint collapsed fence identity: %+v", rows)
	}
}

func TestCheckpointCannotSkipUncommittedOrMissingWork(t *testing.T) {
	source := sourceFixture(t)
	completed := resumeBrain(t)
	if _, err := Import(context.Background(), source, completed, config.Default()); err != nil {
		t.Fatal(err)
	}
	// Copy a completed checkpoint AND page bytes into a fresh git repo. The
	// importer must verify commit durability rather than trusting the copied state.
	root := resumeBrain(t)
	for name, raw := range snapshot(t, completed) {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Import(context.Background(), source, root, config.Default()); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(gitOutput(t, root, "rev-list", "--count", "HEAD")) != "2" {
		t.Fatal("trusted uncommitted checkpoint")
	}
}

func TestCheckpointRejectsCorruptionSourceChangesAndHumanDeletion(t *testing.T) {
	for _, mode := range []string{"corrupt", "checkpoint_source_hash_changed", "human_deleted", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			root := resumeBrain(t)
			source := sourceFixture(t)
			if _, err := Import(context.Background(), source, root, config.Default()); err != nil {
				t.Fatal(err)
			}
			cpPath := filepath.Join(root, ".serenity", "import", "gbrain.json")
			switch mode {
			case "corrupt":
				if err := os.WriteFile(cpPath, []byte("{broken"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "checkpoint_source_hash_changed":
				var cp checkpoint
				raw, err := os.ReadFile(cpPath)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(raw, &cp); err != nil {
					t.Fatal(err)
				}
				p := cp.Pages["people/ava.md"]
				p.SourceSHA256 = "changed"
				cp.Pages["people/ava.md"] = p
				raw, err = json.Marshal(cp)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(cpPath, raw, 0o600); err != nil {
					t.Fatal(err)
				}
			case "human_deleted":
				if err := os.Remove(filepath.Join(root, "brain", "entities", "person", "ava.md")); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Remove(cpPath); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, "brain", "entities", "person", "ava.md"), cpPath); err != nil {
					t.Fatal(err)
				}
			}
			before := snapshot(t, root)
			if _, err := Import(context.Background(), source, root, config.Default()); err == nil {
				t.Fatalf("accepted %s", mode)
			}
			after := snapshot(t, root)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("changed files after %s", mode)
			}
		})
	}
}

func TestConcurrentImporterIsRejected(t *testing.T) {
	root := resumeBrain(t)
	runtime, err := openRuntime(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()
	lock, err := lockRuntime(runtime)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if _, err := Import(context.Background(), sourceFixture(t), root, config.Default()); err == nil || !strings.Contains(err.Error(), "another import") {
		t.Fatalf("concurrent import not rejected: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "brain")); !os.IsNotExist(err) {
		t.Fatal("concurrent import wrote a page")
	}
}

func TestCheckpointRejectsChangedSourceBytes(t *testing.T) {
	source := t.TempDir()
	for name, raw := range snapshot(t, sourceFixture(t)) {
		path := filepath.Join(source, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	root := resumeBrain(t)
	if _, err := Import(context.Background(), source, root, config.Default()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(source, "people", "ava.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// A YAML comment changes snapshot bytes without changing the parsed page.
	raw = bytes.Replace(raw, []byte("---\n"), []byte("---\n# source snapshot changed\n"), 1)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, root)
	if _, err := Import(context.Background(), source, root, config.Default()); err == nil || !strings.Contains(err.Error(), "source or mapping changed") {
		t.Fatalf("accepted changed snapshot: %v", err)
	}
	if !reflect.DeepEqual(before, snapshot(t, root)) {
		t.Fatal("modified target after source changed")
	}
}
