// Package voice implements the voice-note connector (RFC 0001 §10.1,
// plan T2.16): audio files dropped into a watched directory tree, each
// turned into a Source of kind "voice" whose Bytes are the TEXT
// TRANSCRIPT, not the raw audio. Transcription itself is delegated to a
// Transcriber -- the production implementation, RouterTranscriber, runs
// it through internal/router's TaskClassTranscription (RFC section 9's
// local-cheap tier), matching the RFC's "voice note (transcription via
// router)" line; test doubles implement Transcriber directly (zero-stub
// policy).
//
// Scanning otherwise mirrors internal/connector/file's poll mode: a full
// rescan per Poll call, with a per-path cursor recording each audio
// file's own content hash/size/mtime so an unchanged file is never
// re-read, re-transcribed, or re-emitted. Unlike the file connector, this
// package deliberately has no debounce window -- a dropped voice-note
// recording is written once and not touched again, unlike an editor's
// mid-write save burst -- disclosed scope, not an oversight.
package voice

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/connector"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/router"
)

// Transcriber converts raw audio bytes to a text transcript. The
// production implementation is RouterTranscriber; test doubles implement
// this directly, never a fake router.Provider -- Router.Complete's
// Prompt is text-shaped, so RouterTranscriber base64-encodes the audio
// into it (see RouterTranscriber's doc comment), a detail callers of this
// interface never need to know.
type Transcriber interface {
	Transcribe(ctx context.Context, audio []byte) (transcript string, err error)
}

// RouterTranscriber is the production Transcriber: it runs audio through
// Router.Complete on TaskClassTranscription (router.go's closed
// task-class table resolves this to TierLocalCheap, RFC section 9).
// Router.Complete's Prompt carries only a text string -- there is no
// audio-shaped call in this package's provider contract, and adding one
// would touch the router chokepoint every other task class also depends
// on -- so RouterTranscriber base64-encodes the raw audio bytes into
// Prompt.Text and returns Result.Text as the transcript unchanged. This
// mirrors internal/embed.RouterEmbedder's own interop convention (a
// package-local encoding decided at the router boundary, undocumented to
// Router itself), the established pattern in this codebase for adapting
// a non-text payload onto Router.Complete's text-only Prompt.
type RouterTranscriber struct {
	Router *router.Router
}

var _ Transcriber = (*RouterTranscriber)(nil)

// Transcribe implements Transcriber.
func (t *RouterTranscriber) Transcribe(ctx context.Context, audio []byte) (string, error) {
	prompt := router.Prompt{Text: encodeAudio(audio)}
	result, err := t.Router.Complete(ctx, router.TaskClassTranscription, prompt, router.Budget{})
	if err != nil {
		return "", fmt.Errorf("voice: transcribe: %w", err)
	}
	return result.Text, nil
}

// encodeAudio base64-encodes raw audio bytes for transport through
// Router.Complete's text-only Prompt (see RouterTranscriber's doc
// comment). Standard encoding, not URL-safe -- Prompt.Text has no
// character restrictions of its own.
func encodeAudio(audio []byte) string {
	return base64.StdEncoding.EncodeToString(audio)
}

// defaultExtensions is the set of audio file extensions (lowercase, with
// the leading dot) Poll treats as voice notes when the caller does not
// override it via WithExtensions.
var defaultExtensions = []string{".m4a", ".mp3", ".wav", ".ogg", ".flac", ".aac"}

// Connector is the voice-note implementation of connector.Connector.
// Construct with New; the zero value is not usable (Transcriber is
// required).
type Connector struct {
	root        string
	transcriber Transcriber
	exts        map[string]bool
}

// Option configures a Connector at construction.
type Option func(*Connector)

// WithExtensions overrides defaultExtensions -- the set of file
// extensions (case-insensitive, each with or without a leading dot)
// Poll treats as voice-note audio. An empty call clears the default set
// to nothing (every regular file under root would then be skipped),
// which is never useful; pass at least one extension.
func WithExtensions(exts ...string) Option {
	return func(c *Connector) {
		c.exts = extSet(exts)
	}
}

func extSet(exts []string) map[string]bool {
	m := make(map[string]bool, len(exts))
	for _, e := range exts {
		e = strings.ToLower(e)
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		m[e] = true
	}
	return m
}

// New constructs a poll-mode voice connector rooted at root, using
// transcriber to turn each new/changed audio file's bytes into a text
// transcript.
func New(root string, transcriber Transcriber, opts ...Option) *Connector {
	c := &Connector{root: root, transcriber: transcriber, exts: extSet(defaultExtensions)}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Name identifies this connector in job rows and RFC §10.1 provenance.
func (c *Connector) Name() string { return "voice" }

// cursorState is the voice connector's Cursor payload: the last audio
// content seen at each relative path, so Poll only re-reads (and
// re-transcribes) a file whose audio content actually changed since the
// previous call -- the same shape internal/connector/file's cursorState
// uses, for the same reason (RFC §10.1: "advancing the cursor never
// skips or duplicates a source").
type cursorState struct {
	Seen map[string]seenFile `json:"seen"`
}

type seenFile struct {
	SHA256  string    `json:"sha256"`
	ModTime time.Time `json:"mod_time"`
	Size    int64     `json:"size"`
}

func decodeCursor(cur connector.Cursor) (cursorState, error) {
	state := cursorState{Seen: map[string]seenFile{}}
	if len(cur) == 0 {
		return state, nil
	}
	if err := json.Unmarshal(cur, &state); err != nil {
		return cursorState{}, fmt.Errorf("voice: decode cursor: %w", err)
	}
	if state.Seen == nil {
		state.Seen = map[string]seenFile{}
	}
	return state, nil
}

func encodeCursor(state cursorState) connector.Cursor {
	b, err := json.Marshal(state)
	if err != nil {
		// state is built entirely from strings, times, and int64s, so
		// Marshal cannot fail in practice; fall back to an empty-but-valid
		// cursor rather than propagating from a Poll that already
		// succeeded (same fallback internal/connector/file's
		// encodeCursor uses).
		return connector.Cursor(`{"seen":{}}`)
	}
	return connector.Cursor(b)
}

// Poll rescans root for audio files matching c.exts and returns one
// RawItem per file whose content is new or changed since cursor,
// transcribing each through c.transcriber. Poll must be idempotent
// (connector.Connector's contract): replaying the same cursor against an
// unchanged tree returns zero items and never calls the transcriber
// again for a file it has already seen.
func (c *Connector) Poll(ctx context.Context, cursor connector.Cursor) ([]connector.RawItem, connector.Cursor, error) {
	state, err := decodeCursor(cursor)
	if err != nil {
		return nil, cursor, err
	}

	paths, err := c.scan()
	if err != nil {
		return nil, cursor, err
	}

	items := make([]connector.RawItem, 0, len(paths))
	for _, rel := range paths {
		if err := ctx.Err(); err != nil {
			return items, encodeCursor(state), err
		}

		item, changed, err := c.buildItem(ctx, rel, state)
		if err != nil {
			return items, encodeCursor(state), err
		}
		if changed {
			items = append(items, item)
		}
	}

	return items, encodeCursor(state), nil
}

// scan walks root and returns every regular, non-hidden file whose
// extension is in c.exts, as root-relative slash paths, sorted.
func (c *Connector) scan() ([]string, error) {
	var paths []string
	err := filepath.WalkDir(c.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == c.root {
			return nil
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !c.exts[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
		}
		rel, err := filepath.Rel(c.root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("voice: walk %s: %w", c.root, err)
	}
	sort.Strings(paths)
	return paths, nil
}

// buildItem stats, and if needed reads and transcribes, rel (relative to
// root), updating state.Seen in place. changed is false when the file
// was removed before it could be read, or when its audio content is
// identical to what state already recorded (only its mtime moved).
func (c *Connector) buildItem(ctx context.Context, rel string, state cursorState) (connector.RawItem, bool, error) {
	abs := filepath.Join(c.root, filepath.FromSlash(rel))

	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			delete(state.Seen, rel)
			return connector.RawItem{}, false, nil
		}
		return connector.RawItem{}, false, fmt.Errorf("voice: stat %s: %w", rel, err)
	}

	prev, known := state.Seen[rel]
	if known && prev.Size == info.Size() && prev.ModTime.Equal(info.ModTime()) {
		return connector.RawItem{}, false, nil // unchanged since the last poll
	}

	audio, err := os.ReadFile(abs)
	if err != nil {
		return connector.RawItem{}, false, fmt.Errorf("voice: read %s: %w", rel, err)
	}
	sum := sha256.Sum256(audio)
	audioSHA := hex.EncodeToString(sum[:])

	state.Seen[rel] = seenFile{SHA256: audioSHA, ModTime: info.ModTime(), Size: info.Size()}
	if known && prev.SHA256 == audioSHA {
		return connector.RawItem{}, false, nil // content identical; only mtime moved
	}

	transcript, err := c.transcriber.Transcribe(ctx, audio)
	if err != nil {
		return connector.RawItem{}, false, fmt.Errorf("voice: transcribe %s: %w", rel, err)
	}

	return connector.RawItem{
		URI:        uriFor(abs),
		Kind:       "voice",
		Bytes:      []byte(transcript),
		OccurredAt: info.ModTime(),
		Meta: map[string]string{
			"path":         rel,
			"audio_sha256": audioSHA,
		},
	}, true, nil
}

func uriFor(abs string) string { return "file://" + filepath.ToSlash(abs) }

// ToSource converts one transcribed RawItem into the domain.Source the
// store's content-address dedup runs on -- keyed on the TRANSCRIPT bytes,
// not the raw audio, so two audio files that transcribe to
// byte-identical text (including the same file transcribed twice by a
// deterministic transcriber) collapse onto one Source, matching plan
// T2.16's acc line ("double ingest yields zero duplicates"). SHA256 is
// left unset -- SourceStore.Write always recomputes it from the bytes it
// is given, never trusting the caller (internal/store/source.go), the
// same convention every other connector's ToSource follows.
func (c *Connector) ToSource(item connector.RawItem) (domain.Source, error) {
	return domain.Source{
		Kind:       item.Kind,
		URI:        item.URI,
		OccurredAt: item.OccurredAt,
		Meta:       item.Meta,
	}, nil
}
