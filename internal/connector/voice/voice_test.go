package voice_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/connector"
	"github.com/sirerun/serenity/internal/connector/voice"
	"github.com/sirerun/serenity/internal/store"
)

// fakeTranscriber is a test double implementing voice.Transcriber: it
// returns a deterministic transcript derived from the audio bytes'
// content (never from the caller-supplied path), so a fixture polled
// twice -- or two different fixtures that happen to carry identical
// audio bytes -- transcribes to the exact same text both times. Real
// audio codecs are not decoded; this only needs to prove the connector's
// own wiring, not a transcription model. Test-file only, per the
// zero-stub policy.
type fakeTranscriber struct {
	calls  int
	err    error
	prefix string
}

func (f *fakeTranscriber) Transcribe(_ context.Context, audio []byte) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	sum := sha256.Sum256(audio)
	return f.prefix + hex.EncodeToString(sum[:8]), nil
}

func writeFixture(t *testing.T, dir, rel string, content []byte) {
	t.Helper()
	abs := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

// countSources walks root's content-addressed source layout and returns
// how many distinct sha256 directories (identified by their meta.yaml
// sidecar) exist on disk -- the same helper convention
// internal/connector/imap's TestDoubleImportProducesZeroDuplicateSources
// uses.
func countSources(t *testing.T, root string) int {
	t.Helper()
	dir := filepath.Join(root, "brain", "sources")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return 0
	}
	n := 0
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "meta.yaml" {
			n++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return n
}

func writeItems(t *testing.T, ss *store.SourceStore, items []connector.RawItem, c *voice.Connector) {
	t.Helper()
	for _, item := range items {
		src, err := c.ToSource(item)
		if err != nil {
			t.Fatalf("ToSource: %v", err)
		}
		if _, err := ss.Write(item.Bytes, src); err != nil {
			t.Fatalf("SourceStore.Write: %v", err)
		}
	}
}

func TestPollProducesVoiceSourcesWithTranscriptBytesAndAudioSHA(t *testing.T) {
	audioDir := t.TempDir()
	audio1 := []byte("pretend-audio-bytes-note-one")
	audio2 := []byte("pretend-audio-bytes-note-two")
	writeFixture(t, audioDir, "note1.m4a", audio1)
	writeFixture(t, audioDir, "note2.wav", audio2)
	// Not an audio extension -- must be ignored entirely.
	writeFixture(t, audioDir, "readme.txt", []byte("not audio"))

	ft := &fakeTranscriber{prefix: "transcript:"}
	c := voice.New(audioDir, ft)

	items, _, err := c.Poll(context.Background(), nil)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("Poll returned %d items, want 2 (readme.txt must be skipped)", len(items))
	}

	sum1 := sha256.Sum256(audio1)
	wantSHA1 := hex.EncodeToString(sum1[:])

	for _, item := range items {
		if item.Kind != "voice" {
			t.Errorf("item.Kind = %q, want %q", item.Kind, "voice")
		}
		audioSHA, ok := item.Meta["audio_sha256"]
		if !ok || audioSHA == "" {
			t.Errorf("item.Meta[%q] missing or empty", "audio_sha256")
		}
		if len(item.Bytes) == 0 {
			t.Error("item.Bytes (the transcript) is empty")
		}
		if string(item.Bytes) == string(audio1) || string(item.Bytes) == string(audio2) {
			t.Error("item.Bytes carries raw audio, not a transcript -- ToSource must never index audio bytes directly")
		}
	}

	// The note1.m4a item specifically carries note1's own audio sha, not
	// some other file's.
	var found bool
	for _, item := range items {
		if item.Meta["path"] == "note1.m4a" {
			found = true
			if item.Meta["audio_sha256"] != wantSHA1 {
				t.Fatalf("note1.m4a audio_sha256 = %q, want %q", item.Meta["audio_sha256"], wantSHA1)
			}
		}
	}
	if !found {
		t.Fatal("no item for note1.m4a")
	}

	if ft.calls != 2 {
		t.Fatalf("transcriber called %d times, want 2 (one per audio file, never the .txt file)", ft.calls)
	}
}

// TestDoublePollProducesZeroDuplicateSources is T2.16's golden acc test:
// "double ingest yields zero duplicates" -- mirrors
// internal/connector/imap's TestDoubleImportProducesZeroDuplicateSources
// convention exactly (first Poll with a nil cursor, second Poll replays
// the cursor the first call returned).
func TestDoublePollProducesZeroDuplicateSources(t *testing.T) {
	audioDir := t.TempDir()
	writeFixture(t, audioDir, "note1.m4a", []byte("audio-one"))
	writeFixture(t, audioDir, "sub/note2.mp3", []byte("audio-two"))

	ft := &fakeTranscriber{prefix: "t:"}
	c := voice.New(audioDir, ft)
	root := t.TempDir()
	ss := store.NewSourceStore(root)
	ctx := context.Background()

	items1, cursor1, err := c.Poll(ctx, nil)
	if err != nil {
		t.Fatalf("first Poll: %v", err)
	}
	if len(items1) != 2 {
		t.Fatalf("first Poll returned %d items, want 2", len(items1))
	}
	writeItems(t, ss, items1, c)
	if got := countSources(t, root); got != 2 {
		t.Fatalf("after first poll: %d sources on disk, want 2", got)
	}
	if ft.calls != 2 {
		t.Fatalf("transcriber called %d times after first poll, want 2", ft.calls)
	}

	items2, _, err := c.Poll(ctx, cursor1)
	if err != nil {
		t.Fatalf("second Poll: %v", err)
	}
	if len(items2) != 0 {
		t.Fatalf("second Poll (no changed audio) returned %d items, want 0", len(items2))
	}
	writeItems(t, ss, items2, c)
	if got := countSources(t, root); got != 2 {
		t.Fatalf("after double poll: %d sources on disk, want 2 (zero duplicates)", got)
	}
	if ft.calls != 2 {
		t.Fatalf("transcriber called %d times after double poll, want 2 (unchanged audio must never be re-transcribed)", ft.calls)
	}
}

// TestSameTranscriptCollapsesToOneSourceRegardlessOfAudioFile proves
// ToSource keys the store's content-address dedup on the TRANSCRIPT
// bytes, not the audio: two different audio files whose transcriber
// output happens to be byte-identical collapse onto one Source -- the
// other half of "double ingest yields zero duplicates" (identical
// content, not just an identical file).
func TestSameTranscriptCollapsesToOneSourceRegardlessOfAudioFile(t *testing.T) {
	audioDir := t.TempDir()
	// Two DIFFERENT audio files; the fake transcriber below returns the
	// same fixed transcript for both, regardless of content.
	writeFixture(t, audioDir, "a.m4a", []byte("audio-a"))
	writeFixture(t, audioDir, "b.m4a", []byte("audio-b"))

	ft := &fixedTranscriber{transcript: "same transcript every time"}
	c := voice.New(audioDir, ft)
	root := t.TempDir()
	ss := store.NewSourceStore(root)

	items, _, err := c.Poll(context.Background(), nil)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("Poll returned %d items, want 2", len(items))
	}
	writeItems(t, ss, items, c)
	if got := countSources(t, root); got != 1 {
		t.Fatalf("sources on disk = %d, want 1 (identical transcripts must collapse to one Source)", got)
	}
}

type fixedTranscriber struct{ transcript string }

func (f *fixedTranscriber) Transcribe(_ context.Context, _ []byte) (string, error) {
	return f.transcript, nil
}

func TestPollSkipsRemovedFileWithoutError(t *testing.T) {
	audioDir := t.TempDir()
	writeFixture(t, audioDir, "note.m4a", []byte("audio"))

	ft := &fakeTranscriber{}
	c := voice.New(audioDir, ft)

	_, cursor, err := c.Poll(context.Background(), nil)
	if err != nil {
		t.Fatalf("first Poll: %v", err)
	}

	if err := os.Remove(filepath.Join(audioDir, "note.m4a")); err != nil {
		t.Fatal(err)
	}

	items, _, err := c.Poll(context.Background(), cursor)
	if err != nil {
		t.Fatalf("second Poll after removal: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("Poll after removing the file returned %d items, want 0", len(items))
	}
}

func TestPollPropagatesTranscriberError(t *testing.T) {
	audioDir := t.TempDir()
	writeFixture(t, audioDir, "note.m4a", []byte("audio"))

	ft := &fakeTranscriber{err: errors.New("transcription backend down")}
	c := voice.New(audioDir, ft)

	_, _, err := c.Poll(context.Background(), nil)
	if err == nil {
		t.Fatal("expected Poll to propagate a transcriber failure, not silently drop the item")
	}
}

func TestNameIsVoice(t *testing.T) {
	c := voice.New(t.TempDir(), &fakeTranscriber{})
	if c.Name() != "voice" {
		t.Fatalf("Name() = %q, want %q", c.Name(), "voice")
	}
}

func TestWithExtensionsOverridesDefaultSet(t *testing.T) {
	audioDir := t.TempDir()
	writeFixture(t, audioDir, "note.m4a", []byte("audio")) // default set, but excluded below
	writeFixture(t, audioDir, "note.opus", []byte("audio-opus"))

	ft := &fakeTranscriber{}
	c := voice.New(audioDir, ft, voice.WithExtensions("opus"))

	items, _, err := c.Poll(context.Background(), nil)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("Poll returned %d items, want 1 (only .opus, per WithExtensions override)", len(items))
	}
	if items[0].Meta["path"] != "note.opus" {
		t.Fatalf("item path = %q, want %q", items[0].Meta["path"], "note.opus")
	}
}

// Sanity check that Poll's cursor round-trips through JSON as documented
// (connector.Cursor is json.RawMessage) and survives a zero-time edge
// case -- mirrors internal/connector/file's own cursor convention.
func TestCursorSurvivesRoundTrip(t *testing.T) {
	audioDir := t.TempDir()
	writeFixture(t, audioDir, "note.m4a", []byte("audio"))

	c := voice.New(audioDir, &fakeTranscriber{})
	_, cursor, err := c.Poll(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cursor) == 0 {
		t.Fatal("cursor is empty after a successful Poll with at least one file")
	}

	// A second connector instance, decoding the same cursor bytes,
	// agrees the file is unchanged (no shared in-memory state).
	c2 := voice.New(audioDir, &fakeTranscriber{})
	items, _, err := c2.Poll(context.Background(), cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("second connector instance replaying the cursor returned %d items, want 0", len(items))
	}
}
