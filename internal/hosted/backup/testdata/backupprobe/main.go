// Command backupprobe is backup's own black-box proof fixture: it builds a
// minimal but real hosted data directory (one account, one ready brain with
// an actual committed canonical repository) and calls the real backup.Create
// path, then reports whether the destination was published. Built twice by
// backup_test.go, with and without -tags hostedtest, to prove the fault
// barrier at testhooks.PhaseBackupManifestWritten -- called from Create,
// this package's own code, not testhooks' generic self-test -- only
// activates in a tagged binary with a real armed control pipe, and that it
// fires strictly after every artifact is staged but strictly before the
// destination becomes visible.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/sirerun/serenity/internal/hosted/backup"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/store"
)

type emptyJournal struct{}

func (emptyJournal) AppendDeletion(context.Context, contracts.DeletionEntry) (contracts.DeletionEntry, error) {
	return contracts.DeletionEntry{}, errors.New("emptyJournal: AppendDeletion not implemented")
}
func (emptyJournal) ReadThrough(context.Context, contracts.DeletionWatermark) (contracts.DeletionRead, error) {
	return contracts.DeletionRead{}, nil
}
func (emptyJournal) Seal(context.Context, int64) (contracts.DeletionWatermark, error) {
	return contracts.DeletionWatermark{}, errors.New("emptyJournal: Seal not implemented")
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("usage: backupprobe <base-dir>")
		os.Exit(2)
	}
	base := os.Args[1]
	dataDir := filepath.Join(base, "data")
	dest := filepath.Join(base, "snapshot")
	must(os.MkdirAll(dataDir, 0700))

	db, err := store.Open(filepath.Join(dataDir, "control.db"))
	must(err)
	ctx := context.Background()
	acct, err := db.CreateAccount(ctx, "probe@example.com")
	must(err)
	id := store.ID()
	_, err = db.InsertBrain(ctx, acct.ID, id, id, "ready", time.Now())
	must(err)
	must(db.Close())

	root := filepath.Join(dataDir, "brains", id)
	must(os.MkdirAll(root, 0700))
	runGit(root, "init", "--quiet", "--initial-branch=main")
	runGit(root, "config", "user.name", "Serenity Hosted")
	runGit(root, "config", "user.email", "hosted@serenity.sire.run")
	must(os.WriteFile(filepath.Join(root, "f.txt"), []byte("x\n"), 0600))
	runGit(root, "add", "-A")
	runGit(root, "commit", "--quiet", "-m", "init")

	err = backup.Create(ctx, dataDir, dest, "probe-build-sha", emptyJournal{})
	if err != nil {
		fmt.Println("create-error")
		return
	}
	if _, statErr := os.Stat(dest); statErr == nil {
		fmt.Println("reached-published")
	} else {
		fmt.Println("reached-unpublished")
	}
}

func runGit(dir string, args ...string) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		panic(fmt.Sprintf("git %v: %v: %s", args, err, out))
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
