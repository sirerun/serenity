package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestContainedReadNeverReadsSwappedExternalSymlink(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(repo, "owned")
	outside := filepath.Join(base, "outside")
	secret := []byte("external-fixture-bytes-must-never-be-read")
	if err := os.WriteFile(inside, []byte("owned bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, secret, 0600); err != nil {
		t.Fatal(err)
	}
	leaf := filepath.Join(repo, "README.md")
	if err := os.Link(inside, leaf); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	failure := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		stage := filepath.Join(repo, "swap")
		for {
			select {
			case <-stop:
				return
			default:
			}
			if err := os.Symlink(outside, stage); err != nil {
				failure <- err
				return
			}
			if err := os.Rename(stage, leaf); err != nil {
				failure <- err
				return
			}
			if err := os.Link(inside, stage); err != nil {
				failure <- err
				return
			}
			if err := os.Rename(stage, leaf); err != nil {
				failure <- err
				return
			}
		}
	}()
	defer func() {
		close(stop)
		wg.Wait()
		select {
		case err := <-failure:
			t.Errorf("owned fixture swap failed: %v", err)
		default:
		}
	}()
	successes := 0
	for i := 0; i < 20000; i++ {
		data, err := readContained(repo, "README.md")
		if err != nil {
			if !errors.Is(err, errNonRegular) && !errors.Is(err, errEscapes) && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("raced entry returned an uncounted error instead of a skip: %v", err)
			}
			continue
		}
		successes++
		if bytes.Equal(data, secret) {
			t.Fatalf("outside repository bytes escaped containment after pathname validation at read %d", i)
		}
		if !bytes.Equal(data, []byte("owned bytes")) {
			t.Fatalf("unexpected bytes: %q", data)
		}
	}
	if successes == 0 {
		t.Fatal("fixture did not exercise any successful regular-file read")
	}
}

func TestPollRejectsRepositoryRootReplacementDuringListing(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "repo")
	outside := filepath.Join(base, "outside")
	bin := filepath.Join(base, "bin")
	for _, dir := range []string{repo, outside, bin} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "README.md"), []byte("outside sentinel"), 0600); err != nil {
		t.Fatal(err)
	}
	// The fake read-only Git emits the original root, then replaces that path
	// while listing. It models the interval between discovery and file reads.
	script := `#!/bin/sh
case " $* " in
 *" --show-toplevel "*) printf '%s\n' "$FIXTURE_REPO" ;;
 *" rev-parse HEAD "*) printf 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n' ;;
 *" log "*) printf '2026-10-03T00:00:00Z\n' ;;
 *" ls-files "*) /bin/mv "$FIXTURE_REPO" "$FIXTURE_REPO.parked"; /bin/ln -s "$FIXTURE_OUTSIDE" "$FIXTURE_REPO"; printf 'README.md\000' ;;
 *) exit 2 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FIXTURE_REPO", repo)
	t.Setenv("FIXTURE_OUTSIDE", outside)
	c := New(Config{RepoRoot: repo})
	items, _, err := c.Poll(context.Background(), nil)
	if len(items) != 0 {
		t.Fatalf("replaced root yielded outside items: %v", items)
	}
	if err == nil || !strings.Contains(err.Error(), "root identity changed") {
		t.Fatalf("error=%v; want root identity refusal", err)
	}
}
