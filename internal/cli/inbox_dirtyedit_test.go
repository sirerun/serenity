package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

func seedInboxDirtyEdit(t *testing.T) (*disposition.Store, string) {
	t.Helper()
	ds, root := openInboxTestStore(t)
	it := seedReconcileItem(t, ds, context.Background(), inboxFixedNow, "acme-corp", "has_balance", "$700", "$500", "")
	seedInboxCanonical(t, root, it)
	if _, err := ds.Dispose(context.Background(), it.ID, disposition.VerdictReject, nil, "fixture only", "human:test", "", inboxFixedNow); err != nil {
		t.Fatal(err)
	}
	fw := store.NewFenceWriter(root)
	path := fw.PathFor("topic", "acme-corp")
	page, err := fw.ParseEntity(path)
	if err != nil {
		t.Fatal(err)
	}
	machine := *page
	page.Claims = append(page.Claims[:0:0], page.Claims...)
	page.Claims[0].Object = "$650"
	raw, err := fw.RenderEntity(page)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	q := writer.NewQueue(nil)
	defer q.Close()
	if _, _, err := writer.Fence(q, fw, &machine); err != writer.ErrDirtyTree {
		t.Fatalf("guard: %v", err)
	}
	return ds, root
}

func TestInboxDirtyEditApprovalPublishesCanonicalShard(t *testing.T) {
	ds, root := seedInboxDirtyEdit(t)
	var out bytes.Buffer
	if err := runInbox(context.Background(), root, strings.NewReader(" y\n"), &out, inboxOptions{}, inboxFixedNow); err != nil {
		t.Fatal(err)
	}
	lines, err := store.NewShardStore(root).Lines("acme-corp", "has_balance")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[1].Object != "$650" || !strings.HasPrefix(lines[1].Provenance.Actor, "human:") {
		t.Fatalf("approval did not publish human correction: %+v; output=%s", lines, &out)
	}
	items, err := ds.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Kind == disposition.KindDirtyEdit && item.AppliedPublicationID == "" {
			t.Fatal("missing committed publication marker")
		}
	}
	cmd := exec.Command("git", "status", "--porcelain", "--", "brain")
	cmd.Dir = root
	status, err := cmd.Output()
	if err != nil || len(status) != 0 {
		t.Fatalf("canonical commit: %s %v", status, err)
	}
}

func TestInboxDirtyEditRequiresConfirmationAndPreservesNewerEdit(t *testing.T) {
	for _, mode := range []string{"cancel", "reject", "newer-edit"} {
		t.Run(mode, func(t *testing.T) {
			ds, root := seedInboxDirtyEdit(t)
			path := store.NewFenceWriter(root).PathFor("topic", "acme-corp")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			input := " n\nq"
			if mode == "reject" {
				input = "rkeep pending machine copy\n"
			}
			if mode == "newer-edit" {
				before = append(before, []byte("Newer human edit\n")...)
				if err := os.WriteFile(path, before, 0600); err != nil {
					t.Fatal(err)
				}
				input = " y\n"
			}
			var out bytes.Buffer
			err = runInbox(context.Background(), root, strings.NewReader(input), &out, inboxOptions{}, inboxFixedNow)
			if (err != nil) != (mode == "newer-edit") {
				t.Fatalf("err=%v output=%s", err, &out)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(after, before) {
				t.Fatal("changed human page")
			}
			lines, err := store.NewShardStore(root).Lines("acme-corp", "has_balance")
			if err != nil || len(lines) != 1 {
				t.Fatal("published without confirmation")
			}
			items, err := ds.List(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for _, it := range items {
				if it.Kind == disposition.KindDirtyEdit {
					want := disposition.StatePending
					if mode == "reject" {
						want = disposition.StateDisposed
					}
					if it.State != want || it.AppliedPublicationID != "" {
						t.Fatalf("unexpected review: %+v", it)
					}
				}
			}
		})
	}
}

func TestInboxDirtyEditUnappliedRetryCommitsAndClearsListing(t *testing.T) {
	ds, root := seedInboxDirtyEdit(t)
	if _, err := ds.ImportPending(context.Background(), root, inboxFixedNow); err != nil {
		t.Fatal(err)
	}
	items, err := ds.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, it := range items {
		if it.Kind == disposition.KindDirtyEdit {
			id = it.ID
		}
	}
	if _, err := ds.Dispose(context.Background(), id, disposition.VerdictAccept, nil, "", "human:original", "", inboxFixedNow); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runInbox(context.Background(), root, strings.NewReader(""), &out, inboxOptions{Unapplied: true}, inboxFixedNow); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), id) {
		t.Fatalf("accepted edit hidden: %s", &out)
	}
	out.Reset()
	if err := runInbox(context.Background(), root, strings.NewReader(""), &out, inboxOptions{ApplyID: id}, inboxFixedNow); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "publication") {
		t.Fatalf("wrong result label: %s", &out)
	}
	out.Reset()
	if err := runInbox(context.Background(), root, strings.NewReader(""), &out, inboxOptions{Unapplied: true}, inboxFixedNow); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), id) {
		t.Fatalf("completed edit still unapplied: %s", &out)
	}
}
