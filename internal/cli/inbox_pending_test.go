package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/writer"
)

func TestInboxRecoversPausedWritesBeforeInteractiveReview(t *testing.T) {
	s, root := openInboxTestStore(t)
	rec := writer.PendingRecord{Path: "brain/entities/person/ava.md", Human: "Human content.", Machine: "Proposed machine content.", DetectedAt: inboxFixedNow.Format(time.RFC3339)}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	path := writer.PendingPath(root, "ava")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	for _, opts := range []inboxOptions{{Parked: true}, {Unapplied: true}} {
		var out bytes.Buffer
		if err := runInbox(context.Background(), root, strings.NewReader(""), &out, opts, inboxFixedNow); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("read-only mode consumed producer record: %v", err)
		}
	}
	var out bytes.Buffer
	if err := runInbox(context.Background(), root, strings.NewReader("q\n"), &out, inboxOptions{}, inboxFixedNow); err != nil {
		t.Fatal(err)
	}
	items, err := s.List(context.Background())
	if err != nil || len(items) != 1 || items[0].Kind != disposition.KindDirtyEdit || items[0].State != disposition.StatePending || !strings.Contains(out.String(), "dirty_edit") {
		t.Fatalf("paused write absent from interactive review: items=%+v err=%v output=%q", items, err, out.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("consumed input remains: %v", err)
	}
}
