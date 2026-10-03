package gitrepo

import (
	"bytes"
	"os"
	"path/filepath"
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
