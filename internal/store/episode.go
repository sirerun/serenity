package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sirerun/serenity/internal/domain"
)

const EpisodeSchema = "serenity.episodes/v1"
const SourceKindEpisode = "memory_episode"
const EpisodeMaxRequestBytes = 1 << 20
const EpisodeMaxRecordBytes = 512 << 10
const EpisodeMaxRecords = 256
const EpisodeMaxFences = 256
const EpisodeMaxArchiveBytes = 32 << 20
const EpisodeMaxEvents = 64
const EpisodeMaxTextBytes = 64 << 10
const EpisodeMaxEventJSONBytes = 448 << 10
const EpisodeMaxAttachmentBytes = 32 << 10
const EpisodeMaxResponseBytes = 512 << 10

var ErrEpisodeUnpublished = errors.New("publication_pending")
var ErrEpisodeConflict = errors.New("episode_identity_conflict")
var ErrEpisodeFenced = errors.New("conversation_excluded")
var ErrEpisodeGeneration = errors.New("generation_mismatch")
var ErrEpisodeLimit = errors.New("archive_limit_exceeded")
var ErrEpisodeInvalid = errors.New("invalid_episode")

// IDs are opaque, stable producer identities, never file paths or authority.
func ValidEpisodeID(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && !strings.ContainsRune("._:-", c) {
			return false
		}
	}
	return true
}
func EpisodeHash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func EpisodeConversationKey(producer, conversation string) string {
	return EpisodeHash([]string{producer, conversation})
}
func EpisodeCommandKey(producer, command string) string {
	return EpisodeHash([]string{producer, command})
}
func EpisodeEventKey(producer, conversation, event string) string {
	return EpisodeHash([]string{producer, conversation, event})
}

type EpisodeAttachment struct {
	ID        string `json:"attachment_id"`
	MediaType string `json:"media_type"`
	Data      []byte `json:"data"`
	SHA256    string `json:"sha256"`
}
type EpisodeEvent struct {
	ID          string              `json:"event_id"`
	Sequence    int64               `json:"sequence"`
	Role        string              `json:"role"`
	Type        string              `json:"type"`
	OccurredAt  time.Time           `json:"occurred_at"`
	ObservedAt  time.Time           `json:"observed_at"`
	Attribution string              `json:"attribution"`
	Text        string              `json:"text"`
	Attachments []EpisodeAttachment `json:"attachments"`
	Gaps        []string            `json:"gaps"`
}

// Required visible-event fields distinguish a legitimate empty value from
// a host record that never supplied the field (especially sequence zero).
func strictEpisodeJSON(raw []byte, dst any, required ...string) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return ErrEpisodeInvalid
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	for _, key := range required {
		value, ok := fields[key]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return ErrEpisodeInvalid
		}
	}
	return nil
}
func (e *EpisodeEvent) UnmarshalJSON(raw []byte) error {
	type eventAlias EpisodeEvent
	var decoded eventAlias
	if err := strictEpisodeJSON(raw, &decoded, "event_id", "sequence", "role", "type", "occurred_at", "observed_at", "attribution", "text"); err != nil {
		return err
	}
	// Empty arrays and null both mean no exposed attachments/gaps.
	*e = EpisodeEvent(decoded)
	return nil
}

type EpisodeRef struct {
	EventKey string `json:"event_key"`
	Digest   string `json:"digest"`
}
type EpisodeCheckpoint struct {
	Text      string       `json:"text"`
	Sources   []EpisodeRef `json:"sources"`
	Model     string       `json:"model"`
	CreatedAt time.Time    `json:"created_at"`
}

func (c *EpisodeCheckpoint) UnmarshalJSON(raw []byte) error {
	type checkpointAlias EpisodeCheckpoint
	var decoded checkpointAlias
	if err := strictEpisodeJSON(raw, &decoded, "text", "sources", "model", "created_at"); err != nil {
		return err
	}
	*c = EpisodeCheckpoint(decoded)
	return nil
}

// EpisodeInput is the semantic mutation. Generation is an admission fence, not
// part of its immutable digest: a retry can synchronize a newer generation.
type EpisodeInput struct {
	Operation      string             `json:"operation"`
	ProducerID     string             `json:"producer_id"`
	ConversationID string             `json:"conversation_id"`
	CommandID      string             `json:"command_id"`
	Host           string             `json:"host"`
	HostVersion    string             `json:"host_version"`
	WorkspaceLabel string             `json:"workspace_label"`
	PolicyVersion  string             `json:"policy_version"`
	Events         []EpisodeEvent     `json:"events,omitempty"`
	Checkpoint     *EpisodeCheckpoint `json:"checkpoint,omitempty"`
	Gaps           []string           `json:"gaps"`
}

// EpisodeRecord stores originals once. A retry envelope is never retained.
// Exclusion records contain hashes only, no conversation names or old text.
type EpisodeRecord struct {
	Schema          string        `json:"schema"`
	BrainID         string        `json:"brain_id"`
	Kind            string        `json:"kind"`
	CommandKey      string        `json:"command_key"`
	ConversationKey string        `json:"conversation_key"`
	RequestDigest   string        `json:"request_digest"`
	Generation      uint64        `json:"generation"`
	RecordedAt      time.Time     `json:"recorded_at"`
	Input           *EpisodeInput `json:"original,omitempty"`
	Refs            []EpisodeRef  `json:"event_refs,omitempty"`
}
type EpisodeReceipt struct {
	CommandKey      string       `json:"command_key"`
	ConversationKey string       `json:"conversation_key"`
	Digest          string       `json:"digest"`
	SourceSHA256    string       `json:"source_sha256"`
	RecordedAt      time.Time    `json:"recorded_at"`
	EventRefs       []EpisodeRef `json:"event_refs"`
}
type EpisodeCapture struct {
	ProducerID     string   `json:"producer_id"`
	ConversationID string   `json:"conversation_id"`
	Host           string   `json:"host"`
	HostVersion    string   `json:"host_version"`
	WorkspaceLabel string   `json:"workspace_label"`
	PolicyVersion  string   `json:"policy_version"`
	Gaps           []string `json:"gaps"`
}

func (in EpisodeInput) Capture() EpisodeCapture {
	return EpisodeCapture{in.ProducerID, in.ConversationID, in.Host, in.HostVersion, in.WorkspaceLabel, in.PolicyVersion, in.Gaps}
}

type ArchivedEpisodeEvent struct {
	Host           string       `json:"host"`
	HostVersion    string       `json:"host_version"`
	CaptureGaps    []string     `json:"capture_gaps"`
	ProducerID     string       `json:"producer_id"`
	ConversationID string       `json:"conversation_id"`
	WorkspaceLabel string       `json:"workspace_label"`
	PolicyVersion  string       `json:"policy_version"`
	SourceSHA256   string       `json:"source_sha256"`
	Ref            EpisodeRef   `json:"ref"`
	Event          EpisodeEvent `json:"event"`
}
type EpisodeProjection struct {
	BrainID     string
	Generation  uint64
	Records     map[string]EpisodeRecord
	SourceIDs   map[string]string
	Events      map[string]ArchivedEpisodeEvent
	Fences      map[string]uint64
	Bytes       int64
	DataRecords int
}

func (p *EpisodeProjection) Receipt(key string) EpisodeReceipt {
	r := p.Records[key]
	refs := r.Refs
	if refs == nil {
		refs = []EpisodeRef{}
	}
	return EpisodeReceipt{key, r.ConversationKey, r.RequestDigest, p.SourceIDs[key], r.RecordedAt, refs}
}
func (p *EpisodeProjection) EventEligible(e ArchivedEpisodeEvent) bool {
	return p.Fences[EpisodeConversationKey(e.ProducerID, e.ConversationID)] == 0
}
func (p *EpisodeProjection) RecordEligible(r EpisodeRecord) bool {
	if p.Fences[r.ConversationKey] != 0 {
		return false
	}
	if r.Input != nil && r.Input.Checkpoint != nil {
		for _, ref := range r.Input.Checkpoint.Sources {
			e, ok := p.Events[ref.EventKey]
			if !ok || e.Ref.Digest != ref.Digest || !p.EventEligible(e) {
				return false
			}
		}
	}
	return true
}
func validEpisodeStrings(values []string, maxCount, maxBytes int) bool {
	if len(values) > maxCount {
		return false
	}
	for _, s := range values {
		if !utf8.ValidString(s) || len(s) > maxBytes {
			return false
		}
	}
	return true
}
func ValidateEpisodeInput(in EpisodeInput) error { return validateEpisodeInput(in, false) }

func validateEpisodeInput(in EpisodeInput, referencesOnly bool) error {
	if !ValidEpisodeID(in.ProducerID) || !ValidEpisodeID(in.ConversationID) || !ValidEpisodeID(in.CommandID) {
		return ErrEpisodeInvalid
	}
	if in.Operation == "exclude" {
		if in.Host != "" || in.HostVersion != "" || in.WorkspaceLabel != "" || in.PolicyVersion != "" || len(in.Events) != 0 || in.Checkpoint != nil || len(in.Gaps) != 0 {
			return ErrEpisodeInvalid
		}
		return nil
	}
	if !ValidEpisodeID(in.PolicyVersion) || !ValidEpisodeID(in.Host) || len(in.HostVersion) > 128 || !utf8.ValidString(in.HostVersion) || len(in.WorkspaceLabel) > 128 || !utf8.ValidString(in.WorkspaceLabel) || !validEpisodeStrings(in.Gaps, 32, 128) {
		return ErrEpisodeInvalid
	}
	if in.Operation == "checkpoint" {
		c := in.Checkpoint
		if len(in.Events) != 0 || c == nil || c.CreatedAt.IsZero() || len(c.Text) > EpisodeMaxTextBytes || !utf8.ValidString(c.Text) || len(c.Model) > 256 || !utf8.ValidString(c.Model) || len(c.Sources) == 0 || len(c.Sources) > EpisodeMaxEvents {
			return ErrEpisodeInvalid
		}
		seen := map[string]bool{}
		for _, ref := range c.Sources {
			if !ValidSourceSHA(ref.EventKey) || !ValidSourceSHA(ref.Digest) || seen[ref.EventKey] {
				return ErrEpisodeInvalid
			}
			seen[ref.EventKey] = true
		}
		return nil
	}
	if in.Operation != "append" || in.Checkpoint != nil || (!referencesOnly && len(in.Events) == 0) || len(in.Events) > EpisodeMaxEvents {
		return ErrEpisodeInvalid
	}
	ids := map[string]bool{}
	count := 0
	var seq int64 = -1
	for _, e := range in.Events {
		if !ValidEpisodeID(e.ID) || ids[e.ID] || e.Sequence < 0 || e.Sequence <= seq || e.OccurredAt.IsZero() || e.ObservedAt.IsZero() || len(e.Text) > EpisodeMaxTextBytes || !utf8.ValidString(e.Text) || len(e.Attribution) > 512 || !utf8.ValidString(e.Attribution) || !validEpisodeStrings(e.Gaps, 32, 128) {
			return ErrEpisodeInvalid
		}
		if e.Role != "user" && e.Role != "assistant" && e.Role != "tool" && e.Role != "system" && e.Role != "developer" {
			return ErrEpisodeInvalid
		}
		if e.Type != "message" && e.Type != "tool_call" && e.Type != "tool_result" {
			return ErrEpisodeInvalid
		}
		ids[e.ID] = true
		seq = e.Sequence
		if len(e.Attachments) > 8 {
			return ErrEpisodeLimit
		}
		aids := map[string]bool{}
		for _, a := range e.Attachments {
			h := sha256.Sum256(a.Data)
			if !ValidEpisodeID(a.ID) || aids[a.ID] || len(a.MediaType) == 0 || len(a.MediaType) > 128 || !utf8.ValidString(a.MediaType) || len(a.Data) > EpisodeMaxAttachmentBytes || a.Data == nil || hex.EncodeToString(h[:]) != a.SHA256 {
				return ErrEpisodeInvalid
			}
			aids[a.ID] = true
			count++
		}
		encoded, err := json.Marshal(e)
		if err != nil {
			return ErrEpisodeInvalid
		}
		if len(encoded) > EpisodeMaxEventJSONBytes {
			return ErrEpisodeLimit
		}
	}
	if count > 16 {
		return ErrEpisodeLimit
	}
	return nil
}

func validateEpisodeRecord(r EpisodeRecord) error {
	if r.Schema != EpisodeSchema || !ValidEpisodeID(r.BrainID) || !ValidSourceSHA(r.CommandKey) || !ValidSourceSHA(r.ConversationKey) || !ValidSourceSHA(r.RequestDigest) || r.RecordedAt.IsZero() {
		return ErrEpisodeInvalid
	}
	if r.Kind == "exclude" {
		if r.Input != nil || len(r.Refs) != 0 || r.Generation == 0 {
			return ErrEpisodeInvalid
		}
		return nil
	}
	if r.Input == nil || r.Kind != r.Input.Operation || r.CommandKey != EpisodeCommandKey(r.Input.ProducerID, r.Input.CommandID) || r.ConversationKey != EpisodeConversationKey(r.Input.ProducerID, r.Input.ConversationID) {
		return ErrEpisodeInvalid
	}
	// Only stored append records may consist entirely of deduplicated references.
	if err := validateEpisodeInput(*r.Input, true); err != nil {
		return err
	}
	if len(r.Refs) > EpisodeMaxEvents || (r.Kind == "append" && len(r.Refs) == 0) || (r.Kind == "checkpoint" && len(r.Refs) != 0) {
		return ErrEpisodeInvalid
	}
	seen := map[string]bool{}
	for _, ref := range r.Refs {
		if !ValidSourceSHA(ref.EventKey) || !ValidSourceSHA(ref.Digest) || seen[ref.EventKey] {
			return ErrEpisodeInvalid
		}
		seen[ref.EventKey] = true
	}
	return nil
}
func (s *SourceStore) PutEpisode(r EpisodeRecord) (domain.Source, error) {
	if err := validateEpisodeRecord(r); err != nil {
		return domain.Source{}, err
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return domain.Source{}, err
	}
	if len(raw) > EpisodeMaxRecordBytes {
		return domain.Source{}, ErrEpisodeLimit
	}
	occurred := r.RecordedAt
	if r.Input != nil {
		if len(r.Input.Events) > 0 {
			occurred = r.Input.Events[0].OccurredAt
		} else if r.Input.Checkpoint != nil {
			occurred = r.Input.Checkpoint.CreatedAt
		}
	}
	return s.writeSource(raw, domain.Source{Kind: SourceKindEpisode, URI: "memory://episode", OccurredAt: occurred})
}

// LoadEpisodes is a bounded, rebuildable projection over existing canonical
// SourceStore files. No database or plaintext request journal is introduced.
func LoadEpisodes(ctx context.Context, s *SourceStore) (*EpisodeProjection, error) {
	return loadEpisodes(ctx, s, false)
}

// LoadEpisodesForReplay discovers every episode during admitted startup replay,
// including foundation-only fences in unenrolled ordinary brains. Unlike the
// interactive loader, ordinary source entries do not consume a scan ceiling.
// Directory pages and traversal depth are bounded; all episode projection limits
// and validation remain shared with LoadEpisodes. Callers must hold startup or
// serialized runtime authority, and provide cancellation for maintenance work.
func LoadEpisodesForReplay(ctx context.Context, s *SourceStore) (*EpisodeProjection, error) {
	return loadEpisodes(ctx, s, true)
}

func loadEpisodes(ctx context.Context, s *SourceStore, replay bool) (*EpisodeProjection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p := &EpisodeProjection{Records: map[string]EpisodeRecord{}, SourceIDs: map[string]string{}, Events: map[string]ArchivedEpisodeEvent{}, Fences: map[string]uint64{}}
	base := filepath.Join(s.Root, "brain", "sources")
	seen := 0
	walk := filepath.WalkDir
	if replay {
		walk = func(root string, visit fs.WalkDirFunc) error { return walkEpisodeReplaySources(ctx, root, visit) }
	}
	err := walk(base, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) && path == base {
			return nil
		}
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !replay {
			seen++
			if seen > 65536 {
				return ErrEpisodeLimit
			}
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("episode source symlink")
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".source-stage-") {
			return filepath.SkipDir
		}
		if d.Name() != "meta.yaml" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() > 64<<10 {
			return ErrEpisodeLimit
		}
		sha := filepath.Base(filepath.Dir(path))
		if !ValidSourceSHA(sha) {
			return ErrEpisodeInvalid
		}
		meta, err := s.readMeta(sha)
		if err != nil {
			return err
		}
		if meta.Kind != SourceKindEpisode {
			return nil
		}
		info, err = os.Lstat(s.bytesPath(sha))
		if err != nil {
			return err
		}
		if info.Size() > EpisodeMaxRecordBytes {
			return ErrEpisodeLimit
		}
		p.Bytes += info.Size()
		if p.Bytes > EpisodeMaxArchiveBytes+(EpisodeMaxFences*4096) {
			return ErrEpisodeLimit
		}
		raw, _, err := s.Read(sha)
		if err != nil {
			return err
		}
		var r EpisodeRecord
		if err = json.Unmarshal(raw, &r); err != nil {
			return err
		}
		if err = validateEpisodeRecord(r); err != nil {
			return err
		}
		if p.BrainID != "" && p.BrainID != r.BrainID {
			return ErrEpisodeConflict
		}
		p.BrainID = r.BrainID
		if _, ok := p.Records[r.CommandKey]; ok {
			return ErrEpisodeConflict
		}
		p.Records[r.CommandKey] = r
		p.SourceIDs[r.CommandKey] = sha
		if r.Kind == "exclude" {
			if p.Fences[r.ConversationKey] != 0 {
				return ErrEpisodeConflict
			}
			p.Fences[r.ConversationKey] = r.Generation
			if r.Generation > p.Generation {
				p.Generation = r.Generation
			}
			if len(p.Fences) > EpisodeMaxFences {
				return ErrEpisodeLimit
			}
			return nil
		}
		p.DataRecords++
		if p.DataRecords > EpisodeMaxRecords {
			return ErrEpisodeLimit
		}
		for _, e := range r.Input.Events {
			key := EpisodeEventKey(r.Input.ProducerID, r.Input.ConversationID, e.ID)
			if _, ok := p.Events[key]; ok {
				return ErrEpisodeConflict
			}
			p.Events[key] = ArchivedEpisodeEvent{Host: r.Input.Host, HostVersion: r.Input.HostVersion, CaptureGaps: r.Input.Gaps, ProducerID: r.Input.ProducerID, ConversationID: r.Input.ConversationID, WorkspaceLabel: r.Input.WorkspaceLabel, PolicyVersion: r.Input.PolicyVersion, SourceSHA256: sha, Ref: EpisodeRef{key, EpisodeHash(e)}, Event: e}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	generations := map[uint64]bool{}
	for _, gen := range p.Fences {
		if generations[gen] || gen > uint64(len(p.Fences)) {
			return nil, ErrEpisodeInvalid
		}
		generations[gen] = true
	}
	sequences := map[string]map[int64]bool{}
	for _, e := range p.Events {
		k := EpisodeConversationKey(e.ProducerID, e.ConversationID)
		if sequences[k] == nil {
			sequences[k] = map[int64]bool{}
		}
		if sequences[k][e.Event.Sequence] {
			return nil, ErrEpisodeConflict
		}
		sequences[k][e.Event.Sequence] = true
	}
	for _, r := range p.Records {
		if r.Generation > p.Generation {
			return nil, ErrEpisodeInvalid
		}
		for _, ref := range r.Refs {
			e, ok := p.Events[ref.EventKey]
			if !ok || e.Ref.Digest != ref.Digest || EpisodeConversationKey(e.ProducerID, e.ConversationID) != r.ConversationKey {
				return nil, ErrEpisodeInvalid
			}
		}
		if r.Input != nil {
			original := *r.Input
			if r.Kind == "append" {
				original.Events = nil
				referenced := map[string]bool{}
				for _, ref := range r.Refs {
					original.Events = append(original.Events, p.Events[ref.EventKey].Event)
					referenced[ref.EventKey] = true
				}
				for _, event := range r.Input.Events {
					if !referenced[EpisodeEventKey(r.Input.ProducerID, r.Input.ConversationID, event.ID)] {
						return nil, ErrEpisodeInvalid
					}
				}
			}
			if err := ValidateEpisodeInput(original); err != nil {
				return nil, err
			}
			if EpisodeHash(original) != r.RequestDigest {
				return nil, ErrEpisodeInvalid
			}
			if r.Input.Checkpoint != nil {
				for _, ref := range r.Input.Checkpoint.Sources {
					e, ok := p.Events[ref.EventKey]
					if !ok || e.Ref.Digest != ref.Digest {
						return nil, ErrEpisodeInvalid
					}
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return p, nil
}

// Canonical sources use only two directory levels below the root. The generous
// depth bound rejects pathological trees without retaining an unbounded stack
// or directory handles. Ordinary inventory size is deliberately not bounded.
func walkEpisodeReplaySources(ctx context.Context, root string, visit fs.WalkDirFunc) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return visit(root, nil, err)
	}
	var walk func(string, fs.DirEntry, int) error
	walk = func(path string, entry fs.DirEntry, depth int) (retErr error) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := visit(path, entry, nil); err != nil {
			if err == filepath.SkipDir && entry.IsDir() {
				return nil
			}
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if depth > 64 {
			return fmt.Errorf("%w: episode source directory nesting", ErrEpisodeInvalid)
		}
		dir, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { retErr = errors.Join(retErr, dir.Close()) }()
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			entries, err := dir.ReadDir(128)
			if err != nil && err != io.EOF {
				return err
			}
			for _, child := range entries {
				if err := walk(filepath.Join(path, child.Name()), child, depth+1); err != nil {
					return err
				}
			}
			if err == io.EOF {
				return nil
			}
		}
	}
	return walk(root, fs.FileInfoToDirEntry(info), 0)
}

func (p *EpisodeProjection) SortedEvents() []ArchivedEpisodeEvent {
	out := make([]ArchivedEpisodeEvent, 0, len(p.Events))
	for _, e := range p.Events {
		if p.EventEligible(e) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.ProducerID != b.ProducerID {
			return a.ProducerID < b.ProducerID
		}
		if a.ConversationID != b.ConversationID {
			return a.ConversationID < b.ConversationID
		}
		return a.Event.Sequence < b.Event.Sequence
	})
	return out
}
