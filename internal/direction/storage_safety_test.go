package direction

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/writer"
)

func TestLedgerStorageRejectsTraversalAndMismatchedIdentity(t *testing.T) {
	for _, mode := range []string{"read-traversal", "delete-traversal", "mismatched-file"} {
		t.Run(mode, func(t *testing.T) {
			s, root := newTestStore(t)
			raw, err := ledger.Encode(stagedDraft("dec-0001", "outside ledger"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, ".dira", "entries"), 0700); err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(root, "outside.md")
			if err := os.WriteFile(outside, raw, 0600); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "read-traversal":
				if _, err := s.Get(context.Background(), "../../outside"); err == nil {
					t.Fatal("read escaped the ledger")
				}
			case "delete-traversal":
				if err := s.Delete(context.Background(), "../../outside"); err == nil {
					t.Fatal("delete escaped the ledger")
				}
			case "mismatched-file":
				if err := os.WriteFile(s.PathFor("dec-0002"), raw, 0600); err != nil {
					t.Fatal(err)
				}
				if _, err := s.Get(context.Background(), "dec-0002"); err == nil {
					t.Fatal("returned another entry's identity")
				}
			}
			got, err := os.ReadFile(outside)
			if err != nil || !bytes.Equal(got, raw) {
				t.Fatal("outside bytes changed")
			}
		})
	}
}

func TestLedgerStorageRejectsSymlinksWithoutTouchingOutside(t *testing.T) {
	for _, location := range []string{"leaf", "entries", "dira"} {
		for _, op := range []string{"get", "list", "put", "create", "delete"} {
			t.Run(location+"/"+op, func(t *testing.T) {
				s, root := newTestStore(t)
				outside := t.TempDir()
				raw, err := ledger.Encode(stagedDraft("dec-0001", "outside ledger"))
				if err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(outside, "dec-0001.md")
				if err := os.WriteFile(target, raw, 0600); err != nil {
					t.Fatal(err)
				}
				switch location {
				case "leaf":
					if err := os.MkdirAll(filepath.Join(root, ".dira", "entries"), 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(target, s.PathFor("dec-0001")); err != nil {
						t.Fatal(err)
					}
				case "entries":
					if err := os.MkdirAll(filepath.Join(root, ".dira"), 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(outside, filepath.Join(root, ".dira", "entries")); err != nil {
						t.Fatal(err)
					}
				case "dira":
					if err := os.MkdirAll(filepath.Join(outside, "entries"), 0700); err != nil {
						t.Fatal(err)
					}
					target = filepath.Join(outside, "entries", "dec-0001.md")
					if err := os.WriteFile(target, raw, 0600); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(outside, filepath.Join(root, ".dira")); err != nil {
						t.Fatal(err)
					}
				}
				switch op {
				case "get":
					_, err = s.Get(context.Background(), "dec-0001")
				case "list":
					_, err = s.List(context.Background())
				case "put":
					err = s.Put(context.Background(), stagedDraft("dec-0001", "overwrite"))
				case "create":
					id := "dec-0002"
					if location == "leaf" {
						id = "dec-0001"
					}
					err = s.Create(context.Background(), stagedDraft(id, "new entry"))
				case "delete":
					err = s.Delete(context.Background(), "dec-0001")
				}
				if err == nil {
					t.Fatal("symlink operation succeeded")
				}
				got, readErr := os.ReadFile(target)
				if readErr != nil || !bytes.Equal(got, raw) {
					t.Fatal("outside bytes modified")
				}
				if location != "leaf" {
					if _, err := os.Stat(filepath.Join(filepath.Dir(target), "dec-0002.md")); !os.IsNotExist(err) {
						t.Fatal("created outside entry")
					}
				}
			})
		}
	}
}

func TestLedgerPutPublishesOnlyCompleteEntries(t *testing.T) {
	s, _ := newTestStore(t)
	entry := stagedDraft("dec-0001", "atomic entry")
	entry.Body = strings.Repeat("full human evidence ", 1<<16)
	if err := s.Create(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		first := true
		for {
			got, err := s.Get(context.Background(), entry.ID)
			if first {
				close(ready)
				first = false
			}
			if err != nil {
				done <- err
				return
			}
			if len(got.Body) != len(entry.Body) {
				done <- fmt.Errorf("partial body: %d want %d", len(got.Body), len(entry.Body))
				return
			}
			select {
			case <-stop:
				done <- nil
				return
			default:
			}
		}
	}()
	<-ready
	for i := 0; i < 32; i++ {
		copy := *entry
		copy.Title = fmt.Sprintf("complete version %d", i)
		if err := s.Put(context.Background(), &copy); err != nil {
			close(stop)
			<-done
			t.Fatal(err)
		}
	}
	close(stop)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestLedgerAtomicCreateAcrossIndependentQueuesHasOneWinner(t *testing.T) {
	first, root := newTestStore(t)
	q := writer.NewQueue(nil)
	defer q.Close()
	second := NewStore(root, q)
	start := make(chan struct{})
	results := make(chan error, 2)
	var group sync.WaitGroup
	for i, s := range []*Store{first, second} {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			results <- s.Create(context.Background(), stagedDraft("dec-0001", fmt.Sprintf("winner %d", i)))
		}()
	}
	close(start)
	group.Wait()
	close(results)
	wins, exists := 0, 0
	for err := range results {
		if err == nil {
			wins++
		} else if errors.Is(err, ledger.ErrExists) {
			exists++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || exists != 1 {
		t.Fatalf("wins=%d conflicts=%d", wins, exists)
	}
	got, err := first.Get(context.Background(), "dec-0001")
	if err != nil || !strings.HasPrefix(got.Title, "winner ") {
		t.Fatalf("published entry: %+v %v", got, err)
	}
	infos, err := first.List(context.Background())
	if err != nil || len(infos) != 1 {
		t.Fatalf("entries=%d err=%v", len(infos), err)
	}
}

func TestLedgerStoragePreservesModeAndIgnoresIncompleteTemps(t *testing.T) {
	s, root := newTestStore(t)
	e := stagedDraft("dec-0001", "mode preservation")
	if err := s.Create(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(s.PathFor(e.ID), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".dira", "entries", ".serenity-killed.tmp"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	e.Title = "new version"
	if err := s.Put(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(s.PathFor(e.ID))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("mode=%v err=%v", info, err)
	}
	infos, err := s.List(context.Background())
	if err != nil || len(infos) != 1 {
		t.Fatalf("temporary file became an entry: %+v %v", infos, err)
	}
	if err := s.Delete(context.Background(), e.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(context.Background(), e.ID); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("missing get: %v", err)
	}
	if err := s.Delete(context.Background(), e.ID); !errors.Is(err, ledger.ErrNotFound) {
		t.Fatalf("missing delete: %v", err)
	}
}

func TestLedgerStorageRejectsInvalidWritesWithoutCreatingDirectories(t *testing.T) {
	for _, e := range []*ledger.Entry{nil, stagedDraft("../../outside", "invalid id"), stagedDraft("dec-0001", "invalid kind")} {
		if e != nil && e.ID == "dec-0001" {
			e.Kind = ledger.Kind("invalid")
		}
		s, root := newTestStore(t)
		if err := s.Create(context.Background(), e); err == nil {
			t.Fatal("invalid create succeeded")
		}
		if err := s.Put(context.Background(), e); err == nil {
			t.Fatal("invalid put succeeded")
		}
		if _, err := os.Stat(filepath.Join(root, ".dira")); !os.IsNotExist(err) {
			t.Fatal("invalid write created ledger directory")
		}
	}
}
