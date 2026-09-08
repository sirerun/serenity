package messages_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/mail"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/eval/messages"
)

func TestGenerateTenThousandMessages(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "corpus")
	m, err := messages.Generate(context.Background(), dir, messages.Options{Count: 10000, Seed: 1, MessagesPerFile: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if m.Count != 10000 || len(m.Files) != 10 || m.Format != "mboxrd" {
		t.Fatalf("manifest: %+v", m)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var disk messages.Manifest
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m, disk) {
		t.Fatal("returned and persisted manifests differ")
	}
	seen := map[string]bool{}
	count := 0
	for _, file := range m.Files {
		data, err := os.ReadFile(filepath.Join(dir, file.Path))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		if hex.EncodeToString(digest[:]) != file.SHA256 {
			t.Fatalf("checksum mismatch: %s", file.Path)
		}
		parts := bytes.Split(data, []byte("\nFrom "))
		if len(parts) != file.Messages {
			t.Fatalf("%s: parsed %d records, manifest says %d", file.Path, len(parts), file.Messages)
		}
		for _, part := range parts {
			_, rfc, ok := bytes.Cut(part, []byte("\n"))
			if !ok {
				t.Fatal("missing envelope newline")
			}
			msg, err := mail.ReadMessage(bytes.NewReader(rfc))
			if err != nil {
				t.Fatal(err)
			}
			id := msg.Header.Get("Message-ID")
			if id == "" || seen[id] {
				t.Fatalf("missing/duplicate ID %q", id)
			}
			seen[id] = true
			if !strings.Contains(id, "@serenity.invalid>") {
				t.Fatalf("non-synthetic ID: %s", id)
			}
			if _, err := msg.Header.AddressList("From"); err != nil {
				t.Fatal(err)
			}
			if _, err := msg.Header.AddressList("To"); err != nil {
				t.Fatal(err)
			}
			date, err := msg.Header.Date()
			if err != nil {
				t.Fatal(err)
			}
			expected := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC).Add(time.Duration(count) * time.Minute)
			if !date.Equal(expected) {
				t.Fatalf("date %s, expected %s", date, expected)
			}
			body, err := io.ReadAll(msg.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Contains(body, []byte("\n>From the archive:")) || !bytes.Contains(body, []byte("\n>>From an earlier note:")) {
				t.Fatal("mboxrd escaping missing")
			}
			if !bytes.Contains(body, []byte(" prefers ")) {
				t.Fatal("empty synthetic claim")
			}
			count++
		}
	}
	if count != 10000 || len(seen) != 10000 {
		t.Fatalf("executed %d records, %d unique IDs", count, len(seen))
	}
}

func TestGenerateDeterministicAcrossDirectoriesAndRepeat(t *testing.T) {
	root := t.TempDir()
	opts := messages.Options{Count: 23, Seed: 42, MessagesPerFile: 10}
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	first, err := messages.Generate(context.Background(), a, opts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := messages.Generate(context.Background(), b, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.Files[2].Messages != 3 {
		t.Fatalf("non-deterministic manifest or incorrect remainder: %+v %+v", first, second)
	}
	for _, name := range []string{"manifest.json", "messages-00000.mbox", "messages-00001.mbox", "messages-00002.mbox"} {
		x, err := os.ReadFile(filepath.Join(a, name))
		if err != nil {
			t.Fatal(err)
		}
		y, err := os.ReadFile(filepath.Join(b, name))
		if err != nil || !bytes.Equal(x, y) {
			t.Fatalf("different bytes for %s: %v", name, err)
		}
	}
	repeated, err := messages.Generate(context.Background(), a, opts)
	if err != nil || !reflect.DeepEqual(repeated, first) {
		t.Fatalf("repeat: %v", err)
	}
	opts.Seed++
	changed, err := messages.Generate(context.Background(), filepath.Join(root, "other-seed"), opts)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Files[0].SHA256 == first.Files[0].SHA256 {
		t.Fatal("seed does not affect messages")
	}
	before, err := os.ReadFile(filepath.Join(a, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := messages.Generate(context.Background(), a, opts); err == nil {
		t.Fatal("overwrote different corpus")
	}
	after, err := os.ReadFile(filepath.Join(a, "manifest.json"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("changed prior corpus")
	}
}

func TestGenerateRejectsInvalidOptionsAndCancellation(t *testing.T) {
	for name, opts := range map[string]messages.Options{"zero count": {Count: 0, MessagesPerFile: 1}, "negative count": {Count: -1, MessagesPerFile: 1}, "zero partition": {Count: 1, MessagesPerFile: 0}} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "out")
			if _, err := messages.Generate(context.Background(), dir, opts); err == nil {
				t.Fatal("accepted invalid options")
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatal("published invalid corpus")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir := filepath.Join(t.TempDir(), "out")
	if _, err := messages.Generate(ctx, dir, messages.Options{Count: 10, Seed: 1, MessagesPerFile: 2}); err != context.Canceled {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("published cancelled corpus")
	}
}

func TestGeneratePreservesUnrelatedFilesAndRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "out")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(dir, "my-notes.txt")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := messages.Options{Count: 3, Seed: 1, MessagesPerFile: 2}
	if _, err := messages.Generate(context.Background(), dir, opts); err == nil {
		t.Fatal("accepted occupied directory")
	}
	data, err := os.ReadFile(sentinel)
	if err != nil || string(data) != "keep" {
		t.Fatal("changed unrelated file")
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	if _, err := messages.Generate(context.Background(), link, opts); err == nil {
		t.Fatal("accepted symlink destination")
	}
}
