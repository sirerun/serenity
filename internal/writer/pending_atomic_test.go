package writer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestPendingReadersOnlyObserveCompleteProducerRecords(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	gitRepo(t, root)
	path := filepath.Join(root, "page.md")
	if err := os.WriteFile(path, []byte("committed baseline"), 0644); err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, root, "baseline")
	human := bytes.Repeat([]byte("H"), 256*1024)
	if err := os.WriteFile(path, human, 0644); err != nil {
		t.Fatal(err)
	}
	q := NewQueue(nil)
	defer q.Close()
	machine := bytes.Repeat([]byte("M"), 1024*1024)
	write := func() error {
		_, err := guard(q, root, path, "page", machine, func() ([]byte, error) { return nil, errors.New("dirty target must not render") })
		if !errors.Is(err, ErrDirtyTree) {
			return fmt.Errorf("guard did not pause: %w", err)
		}
		return nil
	}
	if err := write(); err != nil {
		t.Fatal(err)
	}
	type observation struct {
		reads int
		err   error
	}
	stop := make(chan struct{})
	result := make(chan observation, 1)
	go func() {
		var got observation
		defer func() { result <- got }()
		for {
			select {
			case <-stop:
				return
			default:
			}
			raw, err := os.ReadFile(PendingPath(root, "page"))
			if err != nil {
				got.err = err
				return
			}
			var rec PendingRecord
			if err := json.Unmarshal(raw, &rec); err != nil {
				got.err = fmt.Errorf("reader saw incomplete record: %w", err)
				return
			}
			if len(rec.Human) != len(human) || len(rec.Machine) != len(machine) {
				got.err = errors.New("reader saw truncated conflict evidence")
				return
			}
			got.reads++
		}
	}()
	var writeErr error
	for i := 0; i < 32; i++ {
		machine[0] = byte('A' + i%26)
		if err := write(); err != nil {
			writeErr = err
			break
		}
	}
	close(stop)
	got := <-result
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if got.err != nil || got.reads == 0 {
		t.Fatalf("concurrent reader: reads=%d err=%v", got.reads, got.err)
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(current, human) {
		t.Fatalf("producer touched canonical human bytes: %v", err)
	}
}
