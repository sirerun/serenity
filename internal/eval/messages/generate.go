// Package messages generates a reproducible, entirely synthetic mboxrd corpus
// for ingest benchmarks. It never uses wall-clock time, credentials or a network.
package messages

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Options determines every byte of the corpus. Seed uses math/rand's stable
// seeded source; MessagesPerFile controls the mbox partition boundaries.
type Options struct {
	Count           int
	Seed            int64
	MessagesPerFile int
}

type File struct {
	Path     string `json:"path"`
	Messages int    `json:"messages"`
	SHA256   string `json:"sha256"`
}

type Manifest struct {
	Version         int    `json:"version"`
	Format          string `json:"format"`
	Count           int    `json:"messages"`
	Seed            int64  `json:"seed"`
	MessagesPerFile int    `json:"messages_per_file"`
	Files           []File `json:"files"`
}

// Generate publishes a complete corpus directory. An identical existing corpus
// is a no-op; different or extra files are never removed or overwritten.
func Generate(ctx context.Context, dir string, opts Options) (Manifest, error) {
	manifest := Manifest{Version: 1, Format: "mboxrd", Count: opts.Count, Seed: opts.Seed, MessagesPerFile: opts.MessagesPerFile, Files: []File{}}
	if opts.Count <= 0 || opts.MessagesPerFile <= 0 {
		return manifest, fmt.Errorf("messages: count and messages-per-file must be positive")
	}
	dest, err := filepath.Abs(dir)
	if err != nil {
		return manifest, err
	}
	if info, err := os.Lstat(dest); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return manifest, fmt.Errorf("messages: destination must be a directory, not a symlink")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return manifest, err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return manifest, err
	}
	stage, err := os.MkdirTemp(filepath.Dir(dest), ".serenity-message-corpus-*")
	if err != nil {
		return manifest, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	rng := rand.New(rand.NewSource(opts.Seed))
	for start, part := 0, 0; start < opts.Count; part++ {
		if err := ctx.Err(); err != nil {
			return manifest, err
		}
		count := min(opts.MessagesPerFile, opts.Count-start)
		name := fmt.Sprintf("messages-%05d.mbox", part)
		digest, err := writeMbox(ctx, filepath.Join(stage, name), start, count, opts.Seed, rng)
		if err != nil {
			return manifest, err
		}
		manifest.Files = append(manifest.Files, File{Path: name, Messages: count, SHA256: digest})
		start += count
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return manifest, err
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(filepath.Join(stage, "manifest.json"), raw, 0o644); err != nil {
		return manifest, err
	}
	if err := ctx.Err(); err != nil {
		return manifest, err
	}
	entries, err := os.ReadDir(dest)
	if err == nil && len(entries) > 0 {
		if err := sameCorpus(stage, dest); err != nil {
			return manifest, err
		}
		return manifest, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return manifest, err
	}
	if info, err := os.Lstat(dest); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return manifest, fmt.Errorf("messages: destination is a symlink")
	}
	if err := os.Rename(stage, dest); err != nil {
		return manifest, fmt.Errorf("messages: publish corpus: %w", err)
	}
	return manifest, nil
}

func writeMbox(ctx context.Context, name string, start, count int, seed int64, rng *rand.Rand) (string, error) {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	hash := sha256.New()
	out := bufio.NewWriter(io.MultiWriter(f, hash))
	topics := []string{"release planning", "service reliability", "migration approach", "documentation", "project budget", "review workflow"}
	choices := []string{"feature flags", "a staged rollout", "an extra review", "a smaller scope", "weekly updates", "a rollback rehearsal"}
	base := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	for i := start; i < start+count; i++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		person := rng.Intn(50) + 1
		project := rng.Intn(20) + 1
		topic := topics[rng.Intn(len(topics))]
		choice := choices[rng.Intn(len(choices))]
		date := base.Add(time.Duration(i) * time.Minute)
		from := fmt.Sprintf("person-%03d@example.invalid", person)
		if _, err := fmt.Fprintf(out, "From %s %s\nFrom: Synthetic Person %03d <%s>\nTo: Project %02d <project-%02d@example.invalid>\nDate: %s\nMessage-ID: <m-%08d-s-%016x@serenity.invalid>\nSubject: Project %02d: %s (%08d)\nMIME-Version: 1.0\nContent-Type: text/plain; charset=utf-8\nContent-Transfer-Encoding: 8bit\n\n", from, date.Format(time.ANSIC), person, from, project, project, date.Format(time.RFC1123Z), i+1, seed, project, topic, i+1); err != nil {
			return "", err
		}
		body := fmt.Sprintf("Synthetic Person %03d prefers %s for Project %02d.\nThe discussion concerns %s.\nFrom the archive: this is invented benchmark data.\n>From an earlier note: no real correspondence is included.\nTracking number: %08d.\n", person, choice, project, topic, i+1)
		for _, line := range strings.Split(strings.TrimSuffix(body, "\n"), "\n") {
			if strings.HasPrefix(strings.TrimLeft(line, ">"), "From ") {
				line = ">" + line
			}
			if _, err := fmt.Fprintln(out, line); err != nil {
				return "", err
			}
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return "", err
		}
	}
	if err := out.Flush(); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func sameCorpus(stage, dest string) error {
	want, err := os.ReadDir(stage)
	if err != nil {
		return err
	}
	got, err := os.ReadDir(dest)
	if err != nil {
		return err
	}
	if len(want) != len(got) {
		return fmt.Errorf("messages: destination already contains a different corpus or extra files")
	}
	for i, e := range want {
		if got[i].Name() != e.Name() || !got[i].Type().IsRegular() {
			return fmt.Errorf("messages: destination contains a different file set")
		}
		a, err := os.ReadFile(filepath.Join(stage, e.Name()))
		if err != nil {
			return err
		}
		b, err := os.ReadFile(filepath.Join(dest, e.Name()))
		if err != nil {
			return err
		}
		if !bytes.Equal(a, b) {
			return fmt.Errorf("messages: destination %s differs; choose another output directory", e.Name())
		}
	}
	return nil
}
