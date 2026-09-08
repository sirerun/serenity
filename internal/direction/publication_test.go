package direction

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/index"
)

func ledgerPublicationFixture(t *testing.T, kind disposition.Kind) (*Store, *disposition.Store, disposition.Item, func(...string) string) {
	t.Helper()
	s, root := newTestStore(t)
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "--quiet")
	git("config", "user.name", "Ledger publication fixture")
	git("config", "user.email", "fixture@example.invalid")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".serenity/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	parent := &ledger.Entry{Kind: ledger.KindIntent, Title: "Ship the planned release", State: ledger.StateActive, Created: decomposeFixedNow.Format(time.RFC3339)}
	if err := ledger.Add(context.Background(), s, parent); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "--quiet", "-m", "canonical parent")
	if err := os.MkdirAll(filepath.Join(root, ".serenity"), 0700); err != nil {
		t.Fatal(err)
	}
	eng, err := index.Open(filepath.Join(root, ".serenity", "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	ds := disposition.NewStore(eng)
	var payload any = fixturePayload()
	if kind == disposition.KindDecompose {
		payload = DecomposePayload{ParentID: parent.ID, Child: ChildIntentDraft{Title: "Prepare release notes", Rationale: "needed for review"}}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	item, err := ds.Create(context.Background(), kind, raw, "", decomposeFixedNow)
	if err != nil {
		t.Fatal(err)
	}
	return s, ds, item, git
}

func acceptLedger(t *testing.T, ds *disposition.Store, item disposition.Item) disposition.Item {
	t.Helper()
	res, err := ds.Dispose(context.Background(), item.ID, disposition.VerdictAccept, nil, "", "human:reviewer", "", decomposeFixedNow)
	if err != nil {
		t.Fatal(err)
	}
	return res.Item
}

func TestLedgerPublicationCommitsOneEntryAndPreservesLaterChanges(t *testing.T) {
	for _, kind := range []disposition.Kind{disposition.KindPreceptDraft, disposition.KindDecompose} {
		t.Run(string(kind), func(t *testing.T) {
			s, ds, item, git := ledgerPublicationFixture(t, kind)
			ctx := context.Background()
			before := git("rev-parse", "HEAD")
			if err := s.PreviewDisposition(ctx, item, "human:reviewer", decomposeFixedNow); err != nil {
				t.Fatal(err)
			}
			if before != git("rev-parse", "HEAD") || git("status", "--porcelain") != "" {
				t.Fatal("preview changed canonical state")
			}
			if err := os.WriteFile(filepath.Join(s.root, "unrelated.txt"), []byte("human staged work"), 0600); err != nil {
				t.Fatal(err)
			}
			git("add", "unrelated.txt")
			item = acceptLedger(t, ds, item)
			if !item.LedgerEffectPending {
				t.Fatal("acceptance lacks recovery marker")
			}
			entry, err := s.ApplyAndCommitDisposition(ctx, ds, item, decomposeFixedNow.Add(time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			if entry.Created != decomposeFixedNow.Format(time.RFC3339) {
				t.Fatal("retry time replaced decision time")
			}
			if kind == disposition.KindPreceptDraft && entry.ConfirmedBy != "human:reviewer" {
				t.Fatal("reviewer provenance lost")
			}
			if git("status", "--porcelain", "--", ".dira") != "" || git("diff", "--cached", "--name-only") != "unrelated.txt" {
				t.Fatal("wrong commit paths")
			}
			head := git("rev-parse", "HEAD")
			if head == before {
				t.Fatal("entry not committed")
			}
			stored, err := ds.Get(ctx, item.ID)
			if err != nil || stored.AppliedEntryID != entry.ID || stored.LedgerEffectPending {
				t.Fatalf("marker=%+v err=%v", stored, err)
			}
			path := s.PathFor(entry.ID)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			later := append(append([]byte(nil), raw...), []byte("Later human explanation\n")...)
			if err := os.WriteFile(path, later, 0600); err != nil {
				t.Fatal(err)
			}
			again, err := s.ApplyAndCommitDisposition(ctx, ds, item, decomposeFixedNow.Add(2*time.Hour))
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, later) || again.ID != entry.ID || head != git("rev-parse", "HEAD") {
				t.Fatal("completed receipt replayed over later work")
			}
			infos, err := s.List(ctx)
			if err != nil || len(infos) != 2 {
				t.Fatalf("entries=%d %v", len(infos), err)
			}
			history, err := ds.HistoryFor(ctx, item.ID)
			if err != nil || len(history) != 1 {
				t.Fatal("duplicate decision history")
			}
		})
	}
}

func TestLedgerPublicationFailedCommitResumesOrPreservesInterveningEdit(t *testing.T) {
	for _, mode := range []string{"resume", "newer-entry", "newer-parent"} {
		t.Run(mode, func(t *testing.T) {
			s, ds, item, git := ledgerPublicationFixture(t, disposition.KindDecompose)
			ctx := context.Background()
			item = acceptLedger(t, ds, item)
			hook := filepath.Join(s.root, ".git", "hooks", "pre-commit")
			if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
				t.Fatal(err)
			}
			before := git("rev-parse", "HEAD")
			if _, err := s.ApplyAndCommitDisposition(ctx, ds, item, decomposeFixedNow); err == nil {
				t.Fatal("commit failure swallowed")
			}
			if before != git("rev-parse", "HEAD") {
				t.Fatal("unexpected commit")
			}
			if err := os.Remove(hook); err != nil {
				t.Fatal(err)
			}
			path := s.PathFor("int-0002")
			if mode == "newer-parent" {
				path = s.PathFor("int-0001")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if mode != "resume" {
				raw = append(raw, []byte("Newer human text\n")...)
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err = s.ApplyAndCommitDisposition(ctx, ds, item, decomposeFixedNow.Add(time.Hour))
			if mode == "resume" {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil {
					t.Fatal("intervening edit was accepted")
				}
				got, _ := os.ReadFile(path)
				if !bytes.Equal(got, raw) {
					t.Fatal("newer bytes changed")
				}
			}
			infos, err := s.List(ctx)
			if err != nil || len(infos) != 2 {
				t.Fatalf("entries=%d %v", len(infos), err)
			}
		})
	}
}

func TestLedgerPublicationPreflightRejectsInvalidOrChangedParent(t *testing.T) {
	for _, mode := range []string{"missing-parent", "dirty-parent", "completed-parent", "invalid-draft"} {
		t.Run(mode, func(t *testing.T) {
			kind := disposition.KindDecompose
			if mode == "invalid-draft" {
				kind = disposition.KindPreceptDraft
			}
			s, ds, item, git := ledgerPublicationFixture(t, kind)
			ctx := context.Background()
			switch mode {
			case "missing-parent":
				if err := os.Remove(s.PathFor("int-0001")); err != nil {
					t.Fatal(err)
				}
			case "dirty-parent":
				raw, err := os.ReadFile(s.PathFor("int-0001"))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(s.PathFor("int-0001"), append(raw, []byte("Human edit\n")...), 0600); err != nil {
					t.Fatal(err)
				}
			case "completed-parent":
				parent, err := s.Get(ctx, "int-0001")
				if err != nil {
					t.Fatal(err)
				}
				parent.State = ledger.StateAchieved
				if err := s.Put(ctx, parent); err != nil {
					t.Fatal(err)
				}
				git("add", ".dira")
				git("commit", "--quiet", "-m", "complete parent")
			case "invalid-draft":
				p := fixturePayload()
				p.Alternatives = nil
				item.Payload, _ = json.Marshal(p)
			}
			head, status := git("rev-parse", "HEAD"), git("status", "--porcelain")
			if err := s.PreviewDisposition(ctx, item, "human:reviewer", decomposeFixedNow); err == nil {
				t.Fatal("invalid preview passed")
			}
			if git("rev-parse", "HEAD") != head || git("status", "--porcelain") != status {
				t.Fatal("preview changed canonical files")
			}
			stored, err := ds.Get(ctx, item.ID)
			if err != nil || stored.State != disposition.StatePending {
				t.Fatal("preflight disposed decision")
			}
		})
	}
}

func TestLedgerPublicationMarkerFailureRetriesWithoutNewAllocation(t *testing.T) {
	s, ds, item, git := ledgerPublicationFixture(t, disposition.KindPreceptDraft)
	ctx := context.Background()
	item = acceptLedger(t, ds, item)
	db, err := sql.Open("sqlite", filepath.Join(s.root, ".serenity", "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TRIGGER reject_entry_marker BEFORE UPDATE ON disposition_items WHEN json_extract(NEW.payload,'$.applied_entry_id') IS NOT NULL BEGIN SELECT RAISE(ABORT,'forced marker failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyAndCommitDisposition(ctx, ds, item, decomposeFixedNow); err == nil {
		t.Fatal("marker fault swallowed")
	}
	head := git("rev-parse", "HEAD")
	if _, err := db.Exec(`DROP TRIGGER reject_entry_marker`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyAndCommitDisposition(ctx, ds, item, decomposeFixedNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if head != git("rev-parse", "HEAD") {
		t.Fatal("marker retry committed again")
	}
	infos, err := s.List(ctx)
	if err != nil || len(infos) != 2 {
		t.Fatal("marker retry duplicated entry")
	}
}

func TestLedgerPublicationRefusesUnmarkedLegacyEffectAndMissingCompletedReceipt(t *testing.T) {
	for _, mode := range []string{"legacy", "missing-receipt"} {
		t.Run(mode, func(t *testing.T) {
			s, ds, item, git := ledgerPublicationFixture(t, disposition.KindPreceptDraft)
			ctx := context.Background()
			item = acceptLedger(t, ds, item)
			if mode == "legacy" {
				entry, err := s.ApplyDisposedPreceptDraft(ctx, item, decomposeFixedNow)
				if err != nil {
					t.Fatal(err)
				}
				git("add", ".dira")
				git("commit", "--quiet", "-m", "legacy applied entry")
				if entry.ID == "" {
					t.Fatal("missing legacy entry")
				}
				db, err := sql.Open("sqlite", filepath.Join(s.root, ".serenity", "index.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = db.Close() }()
				if _, err := db.Exec(`UPDATE disposition_items SET payload=CAST(json_remove(payload,'$.ledger_effect_pending') AS BLOB) WHERE id=?`, item.ID); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := s.ApplyAndCommitDisposition(ctx, ds, item, decomposeFixedNow); err != nil {
					t.Fatal(err)
				}
				names, err := filepath.Glob(filepath.Join(s.root, ".serenity", "direction", "*.json"))
				if err != nil || len(names) != 1 {
					t.Fatal("receipt missing")
				}
				if err := os.Remove(names[0]); err != nil {
					t.Fatal(err)
				}
			}
			before := git("rev-parse", "HEAD")
			if _, err := s.ApplyAndCommitDisposition(ctx, ds, item, decomposeFixedNow.Add(time.Hour)); err == nil {
				t.Fatal("allocated through missing recovery intent")
			}
			infos, err := s.List(ctx)
			if err != nil || len(infos) != 2 || git("rev-parse", "HEAD") != before {
				t.Fatal("duplicated legacy effect")
			}
		})
	}
}

func TestLedgerPublicationReadOnlyStoreRefusesBeforeStateAccess(t *testing.T) {
	s := NewStore(t.TempDir(), nil)
	if err := s.PreviewDisposition(context.Background(), disposition.Item{}, "human:test", decomposeFixedNow); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
	if _, err := s.ApplyAndCommitDisposition(context.Background(), nil, disposition.Item{}, decomposeFixedNow); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
}

func TestLedgerPublicationHonorsEditedChildIntent(t *testing.T) {
	s, ds, item, _ := ledgerPublicationFixture(t, disposition.KindDecompose)
	ctx := context.Background()
	edited, err := json.Marshal(DecomposePayload{ParentID: "int-0001", Child: ChildIntentDraft{Title: "Human corrected child", Rationale: "Human rationale"}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := ds.Dispose(ctx, item.ID, disposition.VerdictEditAccept, edited, "", "human:editor", "", decomposeFixedNow)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := s.ApplyAndCommitDisposition(ctx, ds, res.Item, decomposeFixedNow.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if entry.Title != "Human corrected child" || entry.Edges[0].Note != "Human rationale" {
		t.Fatalf("edited decision ignored: %+v", entry)
	}
}

func TestLedgerPublicationPreservesRecordedSubsecondDecisionTime(t *testing.T) {
	s, ds, item, _ := ledgerPublicationFixture(t, disposition.KindPreceptDraft)
	ctx := context.Background()
	at := decomposeFixedNow.Add(123456789 * time.Nanosecond)
	res, err := ds.Dispose(ctx, item.ID, disposition.VerdictAccept, nil, "", "human:precise", "", at)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := s.ApplyAndCommitDisposition(ctx, ds, res.Item, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	want := at.Format(time.RFC3339Nano)
	if entry.Created != want || entry.Updated != want {
		t.Fatalf("lost recorded precision: %s %s want %s", entry.Created, entry.Updated, want)
	}
}
