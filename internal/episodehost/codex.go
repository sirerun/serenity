// Package episodehost contains bounded, version-pinned history decoders. It does
// not discover files, enroll hosts, trust repository settings, or upload data.
package episodehost

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sirerun/serenity/internal/episodeclient"
)

const CodexVersion = "0.163.0"
const MaxHistoryBytes = 16 << 20
const MaxLineBytes = 1 << 20
const MaxPageEvents = 64
const MaxPageLines = 256
const MaxVisibleTextBytes = 64 << 10

var ErrFormat = errors.New("unsupported_history_format")
var ErrRewrite = errors.New("history_rewritten_reset_required")
var ErrLimit = errors.New("history_limit_exceeded")
var ErrEnrollment = errors.New("explicit_session_enrollment_required")

// Enrollment is supplied by trusted owner settings, never read from the log.
// The caller must separately apply capture policy/redaction before staging.
// This decoder cannot detect a host's private mode; unknown sessions are refused.
type Enrollment struct {
	SessionID      string
	VisibleSession bool
}

// Cursor binds an acknowledged prefix to a session and parser version. Persist
// it only after every returned event is durably admitted by the capture client.
// PrefixSHA256 contains no log text. A truncated/rewritten prefix never becomes
// a new stream automatically, preventing accidental duplicate re-import.
type Cursor struct {
	SessionID    string `json:"session_id"`
	Version      string `json:"version"`
	Lines        int    `json:"lines"`
	PrefixSHA256 string `json:"prefix_sha256"`
}

type Message struct {
	ID         string    `json:"event_id"`
	Sequence   int64     `json:"sequence"`
	Role       string    `json:"role"`
	Text       string    `json:"text"`
	OccurredAt time.Time `json:"occurred_at"`
}

// Gap reasons are fixed codes; never include transcript fragments or paths.
type Gap struct {
	Line   int    `json:"line"`
	Reason string `json:"reason"`
}
type Page struct {
	Messages    []Message `json:"messages"`
	Gaps        []Gap     `json:"gaps"`
	Cursor      Cursor    `json:"cursor"`
	Complete    bool      `json:"complete"`
	PartialTail bool      `json:"partial_tail"`
}
type codexLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

// ReadCodex reads a bounded transcript snapshot from its beginning. Re-reading
// validates the prior prefix before emitting new records. It accepts only the
// explicitly enrolled session and public 0.163.0 response-item message shape.
// Tool evidence, attachments, inherited fork history, and hidden reasoning are
// not captured. Their gaps remain visible. No path in a record is ever opened.
func ReadCodex(r io.Reader, enrollment Enrollment, previous Cursor, limit int) (Page, error) {
	var out Page
	if r == nil {
		return out, ErrFormat
	}
	if !enrollment.VisibleSession || enrollment.SessionID == "" || len(enrollment.SessionID) > 128 {
		return out, ErrEnrollment
	}
	if limit < 1 || limit > MaxPageEvents {
		return out, ErrLimit
	}
	if previous.Lines == 0 && previous != (Cursor{}) {
		return out, ErrRewrite
	}
	if previous.Lines < 0 || (previous.Lines > 0 && (previous.SessionID != enrollment.SessionID || previous.Version != CodexVersion || len(previous.PrefixSHA256) != 64)) {
		return out, ErrRewrite
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxHistoryBytes+1))
	if err != nil {
		return out, err
	}
	if len(data) > MaxHistoryBytes {
		return out, ErrLimit
	}
	reader := bufio.NewReader(bytes.NewReader(data))
	hash := sha256.New()
	lineNo := 0
	sessionSeen := false
	prefixChecked := previous.Lines == 0
	for {
		raw, readErr := reader.ReadBytes('\n')
		if len(raw) > MaxLineBytes {
			return Page{}, ErrLimit
		}
		if readErr == io.EOF && len(raw) == 0 {
			out.Complete = true
			break
		}
		if readErr == io.EOF {
			out.PartialTail = true
			out.Gaps = append(out.Gaps, Gap{lineNo + 1, "partial_tail"})
			break
		}
		if readErr != nil {
			return Page{}, readErr
		}
		lineNo++
		if !utf8.Valid(raw) {
			return Page{}, ErrFormat
		}
		var row codexLine
		if !episodeclient.ValidJSON(raw) || json.Unmarshal(raw, &row) != nil || len(row.Payload) == 0 {
			return Page{}, ErrFormat
		}
		if lineNo == 1 {
			if row.Type != "session_meta" {
				return Page{}, ErrFormat
			}
			var meta struct {
				ID          string          `json:"id"`
				Version     string          `json:"cli_version"`
				Fork        *string         `json:"forked_from_id"`
				HistoryBase json.RawMessage `json:"history_base"`
			}
			if json.Unmarshal(row.Payload, &meta) != nil || meta.ID != enrollment.SessionID || meta.Version != CodexVersion {
				return Page{}, ErrFormat
			}
			if meta.Fork != nil || len(meta.HistoryBase) > 0 && string(meta.HistoryBase) != "null" {
				return Page{}, ErrFormat
			}
			sessionSeen = true
		} else if row.Type == "session_meta" {
			return Page{}, ErrRewrite
		}
		_, _ = hash.Write(raw)
		if lineNo <= previous.Lines {
			if lineNo == previous.Lines {
				if hex.EncodeToString(hash.Sum(nil)) != previous.PrefixSHA256 {
					return Page{}, ErrRewrite
				}
				prefixChecked = true
			}
			continue
		}
		if !prefixChecked {
			return Page{}, ErrRewrite
		}
		messages, gaps, err := decodeCodexLine(row, lineNo, enrollment.SessionID)
		if err != nil {
			return Page{}, err
		}
		if len(out.Messages)+len(messages) > limit {
			// Return the previously acknowledged boundary. The current line is read
			// again next time, retaining its complete content instead of splitting it.
			return out, nil
		}
		out.Messages = append(out.Messages, messages...)
		out.Gaps = append(out.Gaps, gaps...)
		out.Cursor = Cursor{SessionID: enrollment.SessionID, Version: CodexVersion, Lines: lineNo, PrefixSHA256: hex.EncodeToString(hash.Sum(nil))}
		if lineNo-previous.Lines >= MaxPageLines {
			return out, nil
		}
	}
	if !sessionSeen || !prefixChecked {
		return Page{}, ErrRewrite
	}
	if out.Cursor.Lines == 0 {
		out.Cursor = previous
	}
	return out, nil
}

func decodeCodexLine(row codexLine, line int, session string) ([]Message, []Gap, error) {
	gap := func(reason string) ([]Message, []Gap, error) { return nil, []Gap{{line, reason}}, nil }
	switch row.Type {
	case "session_meta", "turn_context", "event_msg":
		return nil, nil, nil // event_msg duplicates response items, and may include reasoning.
	case "compacted":
		return gap("derived_compaction_excluded")
	case "response_item":
	default:
		return gap("unknown_record_type")
	}
	var discriminator struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(row.Payload, &discriminator) != nil {
		return nil, nil, ErrFormat
	}
	if discriminator.Type == "reasoning" {
		return gap("reasoning_excluded")
	}
	if discriminator.Type != "message" {
		return gap("tool_or_response_type_unsupported")
	}
	var item struct {
		Type    string `json:"type"`
		Role    string `json:"role"`
		Channel string `json:"channel"`
		Phase   string `json:"phase"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(row.Payload, &item) != nil {
		return nil, nil, ErrFormat
	}
	if item.Type == "reasoning" {
		return gap("reasoning_excluded")
	}
	if item.Type != "message" {
		return gap("tool_or_response_type_unsupported")
	}
	if item.Role != "user" && item.Role != "assistant" {
		return gap("nonvisible_role_excluded")
	}
	if item.Channel != "" && item.Channel != "final" && item.Channel != "commentary" {
		return gap("nonvisible_channel_excluded")
	}
	if item.Phase != "" && item.Phase != "final_answer" && item.Phase != "commentary" {
		return gap("unknown_phase_excluded")
	}
	var text strings.Builder
	var gaps []Gap
	for _, part := range item.Content {
		if part.Type != "input_text" && part.Type != "output_text" {
			gaps = append(gaps, Gap{line, "attachment_or_content_unsupported"})
			continue
		}
		if text.Len()+len(part.Text) > MaxVisibleTextBytes {
			return nil, nil, ErrLimit
		}
		text.WriteString(part.Text)
	}
	if text.Len() == 0 {
		return nil, gaps, nil
	}
	occurred, err := time.Parse(time.RFC3339Nano, row.Timestamp)
	if err != nil {
		return nil, nil, ErrFormat
	}
	// Identity uses occurrence position, never content alone: equal messages at
	// different positions remain distinct. Prefix verification detects rewrites.
	sum := sha256.Sum256([]byte(fmt.Sprintf("codex-visible-v1\x00%s\x00%d", session, line)))
	return []Message{{ID: hex.EncodeToString(sum[:]), Sequence: int64(line), Role: item.Role, Text: text.String(), OccurredAt: occurred}}, gaps, nil
}
