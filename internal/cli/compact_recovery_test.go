package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/store"
)

func approvedCompactFixture(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	var out bytes.Buffer
	if err := runInit(root, &out); err != nil {
		t.Fatal(err)
	}
	ss := store.NewShardStore(root)
	ss.RolloverBytes = 1
	old := domain.Claim{ID: "old", SubjectSlug: "ava", Family: "has_balance", Predicate: "has_balance", Object: "100", ObjectKey: "balance", State: domain.StateActive, Confidence: .9}
	if err := ss.Append(old); err != nil {
		t.Fatal(err)
	}
	next := old
	next.ID = "new"
	next.Object = "200"
	next.Supersedes = old.ID
	if err := ss.Append(next); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "brain/claims"}, {"commit", "-qm", "seed compaction"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v %s", err, out)
		}
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	ds := disposition.NewStore(eng)
	now := time.Now()
	item, err := ds.Create(context.Background(), disposition.KindCompact, json.RawMessage(`{}`), "", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ds.Dispose(context.Background(), item.ID, disposition.VerdictAccept, nil, "", "human:test", "", now); err != nil {
		t.Fatal(err)
	}
	return root, item.ID, ss.PathFor("ava", "has_balance")
}

func TestApprovedCompactRefusesDirtyCanonicalInput(t *testing.T) {
	root, id, path := approvedCompactFixture(t)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("\n")
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runCompactCmd(context.Background(), root, false, id, &out, time.Now()); err == nil {
		t.Fatal("compaction rewrote uncommitted human input")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("dirty input changed")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "has_balance.archive.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("archive changed before refusal: %v", err)
	}
}

func TestApprovedCompactCommitsExactFiles(t *testing.T) {
	root, id, _ := approvedCompactFixture(t)
	var out bytes.Buffer
	if err := os.WriteFile(filepath.Join(root, "human-note.txt"), []byte("retain my staged note"), 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", root, "add", "human-note.txt").CombinedOutput(); err != nil {
		t.Fatalf("git %v %s", err, output)
	}
	if err := runCompactCmd(context.Background(), root, false, id, &out, time.Now()); err != nil {
		t.Fatal(err)
	}
	dirty, err := exec.Command("git", "-C", root, "status", "--porcelain", "--", "brain/claims").Output()
	if err != nil || len(dirty) != 0 {
		t.Fatalf("compaction left canonical files uncommitted: %s %v", dirty, err)
	}
	staged, err := exec.Command("git", "-C", root, "diff", "--cached", "--name-only").Output()
	if err != nil || string(staged) != "human-note.txt\n" {
		t.Fatalf("unrelated staging changed %s %v", staged, err)
	}
}

func TestApprovedCompactRetryIsOnePass(t *testing.T) {
	root, id, path := approvedCompactFixture(t)
	ctx := context.Background()
	var out bytes.Buffer
	if err := runCompactCmd(ctx, root, false, id, &out, time.Now()); err != nil {
		t.Fatal(err)
	}
	head, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(before, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runCompactCmd(ctx, root, false, id, &out, time.Now()); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, append(before, '\n')) {
		t.Fatal("completed receipt replayed over human edit")
	}
	current, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil || !bytes.Equal(head, current) {
		t.Fatal("retry made a new commit")
	}
	if !strings.Contains(out.String(), "already complete") {
		t.Fatal(out.String())
	}
}

func TestApprovedCompactCommitFailureRecoversWithoutDuplicateArchive(t *testing.T) {
	for _, edit := range []bool{false, true} {
		t.Run(fmt.Sprintf("intervening-edit-%v", edit), func(t *testing.T) {
			root, id, path := approvedCompactFixture(t)
			ctx := context.Background()
			var out bytes.Buffer
			hooks := filepath.Join(root, ".git", "hooks")
			if err := os.MkdirAll(hooks, 0700); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.Command("git", "-C", root, "config", "core.hooksPath", hooks).CombinedOutput(); err != nil {
				t.Fatalf("git %v %s", err, output)
			}
			hook := filepath.Join(hooks, "pre-commit")
			if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := runCompactCmd(ctx, root, false, id, &out, time.Now()); err == nil {
				t.Fatal("failed commit reported success")
			}
			archive := filepath.Join(filepath.Dir(path), "has_balance.archive.jsonl")
			raw, err := os.ReadFile(archive)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(hook); err != nil {
				t.Fatal(err)
			}
			if edit {
				if err := os.WriteFile(path, []byte("human intervening edit"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err = runCompactCmd(ctx, root, false, id, &out, time.Now())
			if edit {
				if err == nil {
					t.Fatal("retry overwrote intervening edit")
				}
				got, err := os.ReadFile(path)
				if err != nil || string(got) != "human intervening edit" {
					t.Fatal("human edit lost")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				after, err := os.ReadFile(archive)
				if err != nil || !bytes.Equal(raw, after) {
					t.Fatal("archive duplicated on retry")
				}
				eng, err := providers.OpenIndex(root)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = eng.Close() }()
				ds := disposition.NewStore(eng)
				item, err := ds.Get(ctx, id)
				if err != nil || item.AppliedPublicationID == "" {
					t.Fatal("commit not marked")
				}
				history, err := ds.HistoryFor(ctx, id)
				if err != nil || len(history) != 1 {
					t.Fatal("decision duplicated")
				}
			}
		})
	}
}

func TestApprovedCompactMissingReceiptAndSymlinkAreRefused(t *testing.T) {
	for _, mode := range []string{"missing-receipt", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			root, id, path := approvedCompactFixture(t)
			ctx := context.Background()
			var out bytes.Buffer
			if mode == "missing-receipt" {
				if err := runCompactCmd(ctx, root, false, id, &out, time.Now()); err != nil {
					t.Fatal(err)
				}
				receipts, err := filepath.Glob(filepath.Join(root, ".serenity/reconcile/compact-*.json"))
				if err != nil || len(receipts) != 1 {
					t.Fatalf("receipts %v %v", receipts, err)
				}
				if err := os.Remove(receipts[0]); err != nil {
					t.Fatal(err)
				}
			} else {
				outside := filepath.Join(t.TempDir(), "outside.jsonl")
				if err := os.WriteFile(outside, []byte("outside bytes"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, filepath.Join(filepath.Dir(path), "has_balance.archive.jsonl")); err != nil {
					t.Fatal(err)
				}
				defer func() {
					got, err := os.ReadFile(outside)
					if err != nil || string(got) != "outside bytes" {
						t.Fatal("outside bytes changed")
					}
				}()
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := runCompactCmd(ctx, root, false, id, &out, time.Now()); err == nil {
				t.Fatal("unsafe or missing receipt accepted")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("refusal changed canonical bytes")
			}
		})
	}
}

func TestApprovedCompactIsVisibleUntilPublication(t *testing.T) {
	root, id, _ := approvedCompactFixture(t)
	ctx := context.Background()
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	ds := disposition.NewStore(eng)
	var out bytes.Buffer
	if err := runListUnapplied(ctx, ds, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "serenity compact --item "+id) {
		t.Fatal(out.String())
	}
	out.Reset()
	if err := runCompactCmd(ctx, root, false, id, &out, time.Now()); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runListUnapplied(ctx, ds, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), id) {
		t.Fatal("completed compaction still unapplied")
	}
}
