// Package episodeclient provides offline capture primitives, not host enrollment
// or an archive wire client. All configuration and source classifications must
// come from a trusted adapter, never from captured text or repository settings.
package episodeclient

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type Code string

func (c Code) Error() string { return string(c) }

const (
	ErrInvalid     Code = "invalid_input"
	ErrExcluded    Code = "excluded"
	ErrMapping     Code = "workspace_unmapped_or_conflicting"
	ErrBinding     Code = "binding_changed"
	ErrPaused      Code = "paused"
	ErrCapacity    Code = "queue_capacity"
	ErrConflict    Code = "identity_conflict"
	ErrStorage     Code = "unsafe_or_corrupt_storage"
	ErrFence       Code = "fence_sync_required"
	ErrBlocked     Code = "replay_blocked"
	ErrPartial     Code = "partial_record"
	ErrRotation    Code = "log_rotated_or_rewritten"
	ErrUnsupported Code = "unsupported_format_or_mode"
)

// Binding is immutable for a queue. Policy is an opaque version, not policy text.
type Binding struct{ Brain, Policy, Producer string }

func (b Binding) valid() bool { return validID(b.Brain) && validID(b.Policy) && validID(b.Producer) }
func validID(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '_' && c != '.' {
			return false
		}
	}
	return true
}

// Mapping is an explicit owner setting. Even a personal workspace needs its own
// mapping. Multiple entries for the same workspace are rejected, not prioritized.
type Mapping struct {
	Workspace string
	Binding   Binding
}
type Policy struct {
	Binding  Binding
	Mappings []Mapping
}

func (p Policy) resolve(workspace string) error {
	if !p.Binding.valid() || !validID(workspace) {
		return ErrMapping
	}
	n := 0
	var b Binding
	for _, m := range p.Mappings {
		if m.Workspace == workspace {
			n++
			b = m.Binding
		}
	}
	if n != 1 {
		return ErrMapping
	}
	if b != p.Binding {
		return ErrBinding
	}
	return nil
}

const (
	MaxTextBytes       = 64 << 10
	MaxAttachmentBytes = 32 << 10
	MaxAttachments     = 8
	MaxEventBytes      = 512 << 10
)

// Input is the package's normalized visible-event schema, not a host schema.
// Privacy must be "public". Class must be "visible". Unknown is excluded.
// HostMode must be "normalized-visible-v1"; adapters remain unqualified.
type Input struct {
	Workspace    string            `json:"workspace"`
	Conversation string            `json:"conversation"`
	Event        string            `json:"event"`
	Sequence     uint64            `json:"sequence"`
	Role         string            `json:"role"`
	Kind         string            `json:"kind"`
	SourceTime   time.Time         `json:"source_time"`
	ObservedTime time.Time         `json:"observed_time"`
	HostMode     string            `json:"host_mode"`
	Privacy      string            `json:"privacy"`
	Class        string            `json:"class"`
	Tool         string            `json:"tool,omitempty"`
	Text         string            `json:"text"`
	Attachments  []AttachmentInput `json:"attachments,omitempty"`
}
type AttachmentInput struct {
	ID           string `json:"id"`
	MediaType    string `json:"media_type"`
	Class        string `json:"class"`         // "exposed" only; never a path or URL fetch
	SecretStatus string `json:"secret_status"` // "clear" only; uncertain files excluded
	Available    bool   `json:"available"`
	Bytes        []byte `json:"bytes,omitempty"`
}

// Gap contains only a fixed reason and an index (-1 means event text).
// Never put a filename, source excerpt, credential or free-form error here.
type Gap struct {
	Reason     string `json:"reason"`
	Attachment int    `json:"attachment"`
}
type Attachment struct {
	ID        string `json:"id"`
	MediaType string `json:"media_type"`
	Bytes     []byte `json:"bytes"`
	SHA256    string `json:"sha256"`
}
type Record struct {
	Binding      Binding      `json:"binding"`
	Workspace    string       `json:"workspace"`
	Conversation string       `json:"conversation"`
	Event        string       `json:"event"`
	Sequence     uint64       `json:"sequence"`
	Role         string       `json:"role"`
	Kind         string       `json:"kind"`
	Tool         string       `json:"tool,omitempty"`
	SourceTime   time.Time    `json:"source_time"`
	ObservedTime time.Time    `json:"observed_time"`
	Text         string       `json:"text"`
	Attachments  []Attachment `json:"attachments,omitempty"`
	Gaps         []Gap        `json:"gaps,omitempty"`
}

// Decision is content-free; rejected inputs are never staged as gap records.
type Decision struct {
	Admitted bool
	Gaps     []Gap
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(authorization\s*:\s*(?:bearer|basic)\s+)[^\s]+`),
	regexp.MustCompile(`(?i)\b((?:password|passwd|api[_-]?key|access[_-]?token|secret)\s*[:=]\s*)[^\s,;]+`),
	regexp.MustCompile(`\b(?:sk-[A-Za-z0-9_-]{12,}|AKIA[A-Z0-9]{16}|gh[pousr]_[A-Za-z0-9]{20,})\b`),
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
}

func redact(s string) (string, bool) {
	original := s
	for _, re := range secretPatterns {
		s = re.ReplaceAllString(s, "[REDACTED]")
	}
	return s, s != original
}
func ownTool(s string) bool {
	s = strings.ToLower(s)
	return s == "serenity" || strings.HasPrefix(s, "serenity.") || strings.HasPrefix(s, "serenity_") || strings.HasPrefix(s, "mcp__serenity__") || strings.HasPrefix(s, "memory_")
}
func admit(p Policy, in Input) (Record, Decision, error) {
	var r Record
	d := Decision{}
	if err := p.resolve(in.Workspace); err != nil {
		return r, d, err
	}
	if len(in.Tool) > 128 || (in.Tool != "" && !validID(in.Tool)) {
		return r, d, ErrInvalid
	}
	if in.Privacy != "public" || in.Class != "visible" || ownTool(in.Tool) {
		reason := "excluded_source"
		switch {
		case in.Privacy == "private":
			reason = "private_session"
		case in.Privacy != "public":
			reason = "privacy_unknown"
		case in.Class == "credential":
			reason = "credential_source"
		case in.Class == "secret_file" || in.Class == "unknown_file":
			reason = "secret_file_source"
		case in.Class == "hidden_reasoning":
			reason = "hidden_reasoning"
		case in.Class == "memory" || in.Class == "recall" || ownTool(in.Tool):
			reason = "recursive_memory"
		}
		d.Gaps = []Gap{{reason, -1}}
		return r, d, ErrExcluded
	}
	if in.HostMode != "normalized-visible-v1" {
		d.Gaps = []Gap{{"unsupported_host_mode", -1}}
		return r, d, ErrUnsupported
	}
	if !validID(in.Conversation) || !validID(in.Event) || in.Sequence == 0 || in.SourceTime.IsZero() || in.ObservedTime.IsZero() || len(in.Text) > MaxTextBytes || !utf8.ValidString(in.Text) || len(in.Attachments) > MaxAttachments || len(in.Tool) > 128 {
		return r, d, ErrInvalid
	}
	switch in.Role {
	case "user", "assistant", "tool", "system":
	default:
		return r, d, ErrInvalid
	}
	switch in.Kind {
	case "message", "tool_request", "tool_result":
	default:
		return r, d, ErrInvalid
	}
	r = Record{Binding: p.Binding, Workspace: in.Workspace, Conversation: in.Conversation, Event: in.Event, Sequence: in.Sequence, Role: in.Role, Kind: in.Kind, Tool: in.Tool, SourceTime: in.SourceTime, ObservedTime: in.ObservedTime}
	var changed bool
	r.Text, changed = redact(in.Text)
	if len(r.Text) > MaxTextBytes {
		return Record{}, Decision{}, ErrInvalid
	}
	if changed {
		r.Gaps = append(r.Gaps, Gap{"text_redacted", -1})
	}
	seen := map[string]bool{}
	for i, a := range in.Attachments {
		if !validID(a.ID) || seen[a.ID] || len(a.Bytes) > MaxAttachmentBytes || len(a.MediaType) > 128 {
			return Record{}, Decision{}, ErrInvalid
		}
		seen[a.ID] = true
		reason := ""
		switch {
		case a.Class != "exposed" || a.SecretStatus != "clear":
			reason = "attachment_excluded"
		case !a.Available:
			reason = "attachment_unavailable"
		case a.MediaType != "text/plain" && a.MediaType != "image/png" && a.MediaType != "image/jpeg":
			reason = "attachment_format_unsupported"
		}
		if reason != "" {
			r.Gaps = append(r.Gaps, Gap{reason, i})
			continue
		}
		b := append([]byte(nil), a.Bytes...)
		if a.MediaType == "text/plain" {
			if !utf8.Valid(b) {
				r.Gaps = append(r.Gaps, Gap{"attachment_encoding_unsupported", i})
				continue
			}
			s, c := redact(string(b))
			b = []byte(s)
			if len(b) > MaxAttachmentBytes {
				return Record{}, Decision{}, ErrInvalid
			}
			if c {
				r.Gaps = append(r.Gaps, Gap{"attachment_redacted", i})
			}
		}
		r.Attachments = append(r.Attachments, Attachment{a.ID, a.MediaType, b, digest(b)})
	}
	raw, _ := json.Marshal(r)
	if len(raw) > MaxEventBytes {
		return Record{}, Decision{}, ErrInvalid
	}
	d.Admitted = true
	d.Gaps = append([]Gap(nil), r.Gaps...)
	return r, d, nil
}
func digest(b []byte) string       { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func recordDigest(r Record) string { b, _ := json.Marshal(r); return digest(b) }
