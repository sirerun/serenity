package supersede

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/writer"
)

func dirtyPublicationFixture(t *testing.T, family string) (*Writer, *disposition.Store, disposition.Item, func(...string) string) {
	t.Helper()
	w, ds, _, git := publicationFixture(t, family, "person")
	path := w.Fence.PathFor("person", "demo-person")
	page, err := w.Fence.ParseEntity(path)
	if err != nil {
		t.Fatal(err)
	}
	machine := *page
	page.Claims = append(page.Claims[:0:0], page.Claims...)
	page.Claims[0].Object = "Human correction"
	raw, err := w.Fence.RenderEntity(page)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, []byte("\nHuman prose survives unchanged.\n")...)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := writer.Fence(w.Queue, w.Fence, &machine); err != writer.ErrDirtyTree {
		t.Fatalf("guard: %v", err)
	}
	if _, err := ds.ImportPending(context.Background(), w.Fence.Root, fixedNow); err != nil {
		t.Fatal(err)
	}
	items, err := ds.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Kind == disposition.KindDirtyEdit {
			return w, ds, item, git
		}
	}
	t.Fatal("missing dirty item")
	return nil, nil, disposition.Item{}, nil
}

func acceptDirty(t *testing.T, ds *disposition.Store, item disposition.Item) disposition.Item {
	t.Helper()
	res, err := ds.Dispose(context.Background(), item.ID, disposition.VerdictAccept, nil, "", "human:reviewer", "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	return res.Item
}

func TestDirtyPublicationCommitsExactHumanEditAndRetries(t *testing.T) {
	for _, family := range []string{"works_at", "has_balance"} {
		t.Run(family, func(t *testing.T) {
			w, ds, item, git := dirtyPublicationFixture(t, family)
			preview, err := w.PreviewDirtyEdit(item, "human:reviewer", fixedNow)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(preview, "Human correction") || !strings.Contains(preview, "Human prose survives unchanged.") {
				t.Fatalf("preview: %s", preview)
			}
			if err := os.WriteFile(filepath.Join(w.Fence.Root, "unrelated.txt"), []byte("keep staged"), 0600); err != nil {
				t.Fatal(err)
			}
			git("add", "unrelated.txt")
			item = acceptDirty(t, ds, item)
			id, err := w.ApplyAndCommitDirtyEdit(context.Background(), ds, item, fixedNow)
			if err != nil {
				t.Fatal(err)
			}
			head := git("rev-parse", "HEAD")
			if status := git("status", "--porcelain", "--", "brain"); strings.TrimSpace(status) != "" {
				t.Fatalf("human edit not committed: %s", status)
			}
			if got := git("diff", "--cached", "--name-only"); strings.TrimSpace(got) != "unrelated.txt" {
				t.Fatalf("unrelated stage changed: %q", got)
			}
			pagePath := w.Fence.PathFor("person", "demo-person")
			raw, err := os.ReadFile(pagePath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(raw, []byte("Human prose survives unchanged.")) {
				t.Fatal("prose lost")
			}
			stored, err := ds.Get(context.Background(), item.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.AppliedPublicationID != id || stored.AppliedClaimID != "" {
				t.Fatalf("marker: %+v", stored)
			}
			if family == "has_balance" {
				lines, err := w.Shard.Lines("demo-person", family)
				if err != nil {
					t.Fatal(err)
				}
				if len(lines) != 2 || lines[1].Object != "Human correction" || lines[1].Confidence != 1 || lines[1].Review || lines[1].Provenance.Actor != "human:reviewer" || lines[1].Provenance.SourceSHA256 != "" || lines[1].Provenance.Model != "" || lines[1].Provenance.Span != "" {
					t.Fatalf("human assertion: %+v", lines)
				}
			}
			// A completed receipt must not overwrite a later human edit.
			later := append(append([]byte(nil), raw...), []byte("Later unreviewed prose\n")...)
			if err := os.WriteFile(pagePath, later, 0600); err != nil {
				t.Fatal(err)
			}
			again, err := w.ApplyAndCommitDirtyEdit(context.Background(), ds, item, fixedNow.Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(pagePath)
			if err != nil {
				t.Fatal(err)
			}
			if again != id || head != git("rev-parse", "HEAD") || !bytes.Equal(got, later) {
				t.Fatal("retry changed completed publication")
			}
			history, err := ds.HistoryFor(context.Background(), item.ID)
			if err != nil || len(history) != 1 {
				t.Fatalf("history=%d err=%v", len(history), err)
			}
		})
	}
}

func TestDirtyPublicationCommitFailureResumesWithoutDuplicate(t *testing.T) {
	for _, laterEdit := range []bool{false, true} {
		t.Run(map[bool]string{false: "resume", true: "intervening-edit"}[laterEdit], func(t *testing.T) {
			w, ds, item, git := dirtyPublicationFixture(t, "has_balance")
			item = acceptDirty(t, ds, item)
			hook := filepath.Join(w.Fence.Root, ".git", "hooks", "pre-commit")
			if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
				t.Fatal(err)
			}
			before := git("rev-parse", "HEAD")
			if _, err := w.ApplyAndCommitDirtyEdit(context.Background(), ds, item, fixedNow); err == nil {
				t.Fatal("commit fault passed")
			}
			if before != git("rev-parse", "HEAD") {
				t.Fatal("unexpected commit")
			}
			stored, err := ds.Get(context.Background(), item.ID)
			if err != nil || stored.AppliedPublicationID != "" {
				t.Fatal("marked before commit")
			}
			if err := os.Remove(hook); err != nil {
				t.Fatal(err)
			}
			path := w.Fence.PathFor("person", "demo-person")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if laterEdit {
				raw = append(raw, []byte("Intervening unreviewed text\n")...)
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err = w.ApplyAndCommitDirtyEdit(context.Background(), ds, item, fixedNow.Add(time.Hour))
			if laterEdit {
				if err == nil {
					t.Fatal("overwrote intervening edit")
				}
				got, _ := os.ReadFile(path)
				if !bytes.Equal(got, raw) {
					t.Fatal("changed newer file")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			lines, err := w.Shard.Lines("demo-person", "has_balance")
			if err != nil || len(lines) != 2 {
				t.Fatalf("duplicate lines=%d err=%v", len(lines), err)
			}
		})
	}
}

func TestDirtyPublicationRefusesUnsafeOrStaleInputsBeforeCommit(t *testing.T) {
	for _, mode := range []string{"newer-human-edit", "newer-shard-head", "dirty-shard", "outside-path", "symlink", "missing-head", "changed-state", "unknown-row"} {
		t.Run(mode, func(t *testing.T) {
			w, ds, item, git := dirtyPublicationFixture(t, "has_balance")
			var rec writer.PendingRecord
			if err := json.Unmarshal(item.Payload, &rec); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "newer-human-edit":
				if err := os.WriteFile(rec.Path, []byte(rec.Human+"newer"), 0600); err != nil {
					t.Fatal(err)
				}
			case "newer-shard-head":
				c := claimFixture("new-head", "demo-person", "has_balance", "Other correction")
				c.Supersedes = "old-claim"
				if err := w.Shard.Append(c); err != nil {
					t.Fatal(err)
				}
				git("add", "brain/claims")
				git("commit", "--quiet", "-m", "new head", "--", "brain/claims")
			case "dirty-shard":
				path := w.Shard.PathFor("demo-person", "has_balance")
				f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				_, err = f.WriteString("\n")
				_ = f.Close()
				if err != nil {
					t.Fatal(err)
				}
			case "outside-path":
				rec.Path = filepath.Join(t.TempDir(), "brain/entities/person/demo-person.md")
			case "symlink":
				outside := filepath.Join(t.TempDir(), "page.md")
				if err := os.WriteFile(outside, []byte(rec.Human), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(rec.Path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, rec.Path); err != nil {
					t.Fatal(err)
				}
			default:
				page, err := w.Fence.ParseEntity(rec.Path)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "missing-head" {
					page.Claims = nil
				} else if mode == "changed-state" {
					page.Claims[0].State = domain.StateRetracted
				} else {
					page.Claims[0].ID = "unknown"
				}
				raw, err := w.Fence.RenderEntity(page)
				if err != nil {
					t.Fatal(err)
				}
				rec.Human = string(raw)
				if err := os.WriteFile(rec.Path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			item.Payload, _ = json.Marshal(rec)
			before := git("rev-parse", "HEAD")
			status := git("status", "--porcelain")
			if _, err := w.PreviewDirtyEdit(item, "human:reviewer", fixedNow); err == nil {
				t.Fatal("unsafe preview passed")
			}
			if git("rev-parse", "HEAD") != before || git("status", "--porcelain") != status {
				t.Fatal("preview mutated Git")
			}
			got, err := ds.Get(context.Background(), item.ID)
			if err != nil || got.State != disposition.StatePending {
				t.Fatal("preview disposed item")
			}
		})
	}
}

func TestDirtyPublicationPromotesExplicitConcurrentHeadsWithRotation(t *testing.T) {
	w, ds, item, git := dirtyPublicationFixture(t, "has_balance")
	other := claimFixture("other-head", "demo-person", "has_balance", "Other account")
	w.Shard.RolloverBytes = 1
	if err := w.Shard.Append(other); err != nil {
		t.Fatal(err)
	}
	git("add", "brain/claims")
	git("commit", "--quiet", "-m", "second current head", "--", "brain/claims")
	var rec writer.PendingRecord
	if err := json.Unmarshal(item.Payload, &rec); err != nil {
		t.Fatal(err)
	}
	page, err := w.Fence.ParseEntity(rec.Path)
	if err != nil {
		t.Fatal(err)
	}
	other.SourceRef = "shard"
	other.Object = "Other human correction"
	page.Claims = append(page.Claims, other)
	raw, err := w.Fence.RenderEntity(page)
	if err != nil {
		t.Fatal(err)
	}
	rec.Human = string(raw)
	if err := os.WriteFile(rec.Path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	item, err = ds.Create(context.Background(), disposition.KindDirtyEdit, payload, "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.PreviewDirtyEdit(item, "human:reviewer", fixedNow); err != nil {
		t.Fatal(err)
	}
	item = acceptDirty(t, ds, item)
	if _, err := w.ApplyAndCommitDirtyEdit(context.Background(), ds, item, fixedNow); err != nil {
		t.Fatal(err)
	}
	lines, err := w.Shard.Lines("demo-person", "has_balance")
	if err != nil || len(lines) != 4 {
		t.Fatalf("lines=%d err=%v", len(lines), err)
	}
	heads, err := w.Shard.ResolveHeads("demo-person", "has_balance")
	if err != nil || len(heads) != 2 {
		t.Fatalf("heads=%d err=%v", len(heads), err)
	}
	for _, head := range heads {
		if head.Provenance.Actor != "human:reviewer" || head.Supersedes == "" {
			t.Fatalf("incorrect head: %+v", head)
		}
	}
	if status := git("status", "--porcelain", "--", "brain"); strings.TrimSpace(status) != "" {
		t.Fatalf("uncommitted segments: %s", status)
	}
}

func TestDirtyPublicationMarkerFailureRetriesCompletedReceipt(t *testing.T) {
	w, ds, item, git := dirtyPublicationFixture(t, "has_balance")
	item = acceptDirty(t, ds, item)
	db, err := sql.Open("sqlite", filepath.Join(w.Fence.Root, ".serenity", "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TRIGGER reject_publication_marker BEFORE UPDATE ON disposition_items WHEN json_extract(NEW.payload, '$.applied_publication_id') IS NOT NULL BEGIN SELECT RAISE(ABORT, 'forced marker failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := w.ApplyAndCommitDirtyEdit(context.Background(), ds, item, fixedNow); err == nil {
		t.Fatal("marker fault swallowed")
	}
	head := git("rev-parse", "HEAD")
	stored, err := ds.Get(context.Background(), item.ID)
	if err != nil || stored.AppliedPublicationID != "" {
		t.Fatal("unexpected marker")
	}
	if _, err := db.Exec(`DROP TRIGGER reject_publication_marker`); err != nil {
		t.Fatal(err)
	}
	if _, err := w.ApplyAndCommitDirtyEdit(context.Background(), ds, item, fixedNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if git("rev-parse", "HEAD") != head {
		t.Fatal("marker retry added a commit")
	}
	lines, err := w.Shard.Lines("demo-person", "has_balance")
	if err != nil || len(lines) != 2 {
		t.Fatal("marker retry duplicated assertion")
	}
}
