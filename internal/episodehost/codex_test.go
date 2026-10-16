package episodehost

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func line(kind string, payload any) string {
	b, _ := json.Marshal(map[string]any{"timestamp": "2026-10-16T10:00:00Z", "type": kind, "payload": payload})
	return string(b) + "\n"
}
func metadata() string {
	return line("session_meta", map[string]any{"id": "session-a", "cli_version": CodexVersion})
}
func message(role, text string) string {
	return line("response_item", map[string]any{"type": "message", "role": role, "content": []any{map[string]string{"type": "output_text", "text": text}}})
}

var enrolled = Enrollment{SessionID: "session-a", VisibleSession: true}

func TestCodexPagingPreservesOccurrencesAndBytes(t *testing.T) {
	raw := metadata() + message("user", "same\n\t text") + message("assistant", "same\n\t text") + message("user", "third")
	first, err := ReadCodex(strings.NewReader(raw), enrolled, Cursor{}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if first.Complete || len(first.Messages) != 2 || first.Cursor.Lines != 3 {
		t.Fatalf("first %+v", first)
	}
	if first.Messages[0].ID == first.Messages[1].ID || first.Messages[0].Text != "same\n\t text" {
		t.Fatal("identity/text loss")
	}
	second, err := ReadCodex(strings.NewReader(raw), enrolled, first.Cursor, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Complete || len(second.Messages) != 1 || second.Messages[0].Text != "third" {
		t.Fatalf("second %+v", second)
	}
	replay, err := ReadCodex(strings.NewReader(raw), enrolled, second.Cursor, 2)
	if err != nil || len(replay.Messages) != 0 || !replay.Complete {
		t.Fatalf("replay %+v %v", replay, err)
	}
}
func TestCodexExcludesHiddenAndReportsGaps(t *testing.T) {
	raw := metadata() + line("response_item", map[string]any{"type": "reasoning", "summary": []any{map[string]string{"text": "HIDDEN"}}, "encrypted_content": "HIDDEN"}) +
		line("response_item", map[string]any{"type": "message", "role": "assistant", "channel": "analysis", "content": []any{map[string]string{"type": "output_text", "text": "HIDDEN"}}}) +
		message("developer", "HIDDEN") + line("event_msg", map[string]string{"type": "agent_reasoning", "text": "HIDDEN"}) +
		line("response_item", map[string]any{"type": "function_call_output", "output": "SECRET FILE"}) +
		line("response_item", map[string]any{"type": "message", "role": "user", "content": []any{map[string]string{"type": "input_text", "text": "visible"}, map[string]string{"type": "input_image", "image_url": "file:///private/never-read"}}})
	page, err := ReadCodex(strings.NewReader(raw), enrolled, Cursor{}, 64)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(page)
	if strings.Contains(string(b), "HIDDEN") || strings.Contains(string(b), "SECRET FILE") || strings.Contains(string(b), "/private/") {
		t.Fatal("excluded content leaked")
	}
	if len(page.Messages) != 1 || page.Messages[0].Text != "visible" || len(page.Gaps) != 5 {
		t.Fatalf("page %+v", page)
	}
}
func TestCodexPartialTailAndRewrite(t *testing.T) {
	raw := metadata() + message("user", "one")
	tail := message("assistant", "two")
	p, err := ReadCodex(strings.NewReader(raw+strings.TrimSuffix(tail, "\n")), enrolled, Cursor{}, 64)
	if err != nil {
		t.Fatal(err)
	}
	if !p.PartialTail || p.Complete || len(p.Messages) != 1 || p.Cursor.Lines != 2 {
		t.Fatalf("partial %+v", p)
	}
	next, err := ReadCodex(strings.NewReader(raw+tail), enrolled, p.Cursor, 64)
	if err != nil || len(next.Messages) != 1 {
		t.Fatalf("resume %+v %v", next, err)
	}
	for _, changed := range []string{metadata(), strings.Replace(raw, "one", "changed", 1)} {
		if _, err := ReadCodex(strings.NewReader(changed), enrolled, p.Cursor, 64); !errors.Is(err, ErrRewrite) {
			t.Fatalf("rewrite got %v", err)
		}
	}
}
func TestCodexRequiresEnrollmentVersionAndUnforkedHistory(t *testing.T) {
	for _, e := range []Enrollment{{}, {SessionID: "session-a"}} {
		if _, err := ReadCodex(strings.NewReader(metadata()), e, Cursor{}, 1); !errors.Is(err, ErrEnrollment) {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{strings.Replace(metadata(), CodexVersion, "0.999.0", 1), strings.Replace(metadata(), "session-a", "other", 1), line("session_meta", map[string]any{"id": "session-a", "cli_version": CodexVersion, "forked_from_id": "parent"})} {
		if _, err := ReadCodex(strings.NewReader(raw), enrolled, Cursor{}, 1); !errors.Is(err, ErrFormat) {
			t.Fatalf("got %v", err)
		}
	}
}
func TestCodexBoundsAndMalformedRecords(t *testing.T) {
	for _, raw := range []string{metadata() + message("user", strings.Repeat("x", MaxVisibleTextBytes+1)), strings.Repeat("x", MaxHistoryBytes+1)} {
		if _, err := ReadCodex(strings.NewReader(raw), enrolled, Cursor{}, 64); !errors.Is(err, ErrLimit) {
			t.Fatal(err)
		}
	}
	if _, err := ReadCodex(strings.NewReader(metadata()+"not-json\n"), enrolled, Cursor{}, 64); !errors.Is(err, ErrFormat) {
		t.Fatal(err)
	}
	if _, err := ReadCodex(strings.NewReader(metadata()), enrolled, Cursor{}, 0); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
}

func TestCodexAmbiguityAndGapOnlyPages(t *testing.T) {
	duplicate := metadata() + `{"type":"response_item","payload":{"type":"reasoning","type":"message","role":"user","content":[]},"timestamp":"2026-10-16T10:00:00Z"}` + "\n"
	if _, err := ReadCodex(strings.NewReader(duplicate), enrolled, Cursor{}, 64); !errors.Is(err, ErrFormat) {
		t.Fatal(err)
	}
	raw := metadata() + strings.Repeat(line("unknown", map[string]string{"text": "not copied"}), MaxPageLines+10)
	p, err := ReadCodex(strings.NewReader(raw), enrolled, Cursor{}, 64)
	if err != nil {
		t.Fatal(err)
	}
	if p.Complete || p.Cursor.Lines != MaxPageLines || len(p.Gaps) > MaxPageLines {
		t.Fatalf("unbounded page %+v", p)
	}
}

func TestCodexRejectsUnicodeReplacementAndCaseAliases(t *testing.T) {
	for _, payload := range []string{
		`{"type":"message","role":"user","ROLE":"assistant","content":[{"type":"output_text","text":"visible"}]}`,
		`{"type":"message","role":"user","content":[{"type":"output_text","text":"\ud800"}]}`,
	} {
		raw := metadata() + `{"timestamp":"2026-10-16T10:00:00Z","type":"response_item","payload":` + payload + "}\n"
		if _, err := ReadCodex(strings.NewReader(raw), enrolled, Cursor{}, 64); !errors.Is(err, ErrFormat) {
			t.Fatal(err)
		}
	}
}

func TestCodexEmptyTextOccurrenceAndMalformedText(t *testing.T) {
	page, err := ReadCodex(strings.NewReader(metadata()+message("user", "")), enrolled, Cursor{}, 64)
	if err != nil || len(page.Messages) != 1 || page.Messages[0].Text != "" || !page.Complete {
		t.Fatalf("empty occurrence lost: %+v %v", page, err)
	}
	for _, part := range []map[string]any{{"type": "input_text"}, {"type": "input_text", "text": nil}} {
		raw := metadata() + line("response_item", map[string]any{"type": "message", "role": "user", "content": []any{part}})
		if _, err = ReadCodex(strings.NewReader(raw), enrolled, Cursor{}, 64); !errors.Is(err, ErrFormat) {
			t.Fatalf("malformed text accepted: %v", err)
		}
	}
}

func TestCodexHarnessProvenanceIsNotAnOriginalOccurrence(t *testing.T) {
	for _, provenance := range []any{
		map[string]any{"compaction_output": true},
		map[string]any{"inherited_user_message": true},
		map[string]any{"delivered_assistant_message": "EXCLUDED"},
		map[string]any{"retained_source": map[string]any{"text": "EXCLUDED"}},
		map[string]any{"guardian_sources": []string{"EXCLUDED"}},
		map[string]any{"client_authored": false},
		map[string]any{"user_input_order": 1},
		map[string]any{"future_provenance": "EXCLUDED"},
	} {
		row := map[string]any{"timestamp": "2026-10-16T10:00:00Z", "type": "response_item", "metadata": provenance,
			"payload": map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]string{"type": "output_text", "text": "EXCLUDED"}}}}
		b, _ := json.Marshal(row)
		raw := metadata() + string(b) + "\n" + message("user", "original")
		p, err := ReadCodex(strings.NewReader(raw), enrolled, Cursor{}, 1)
		if err != nil || len(p.Messages) != 1 || p.Messages[0].Text != "original" || len(p.Gaps) != 1 || p.Gaps[0].Reason != "host_provenance_metadata_unsupported" || p.Cursor.Lines != 3 {
			t.Fatalf("provenance admission: %+v %v", p, err)
		}
		encoded, _ := json.Marshal(p)
		if strings.Contains(string(encoded), "EXCLUDED") {
			t.Fatal("provenance content escaped through a gap or event")
		}
		replay, err := ReadCodex(strings.NewReader(raw), enrolled, p.Cursor, 1)
		if err != nil || len(replay.Messages) != 0 || !replay.Complete {
			t.Fatalf("replay %+v %v", replay, err)
		}
	}
}

func TestCodexAbsentProvenanceAndMalformedMetadata(t *testing.T) {
	for _, value := range []string{"null", "{}"} {
		raw := metadata() + strings.TrimSuffix(strings.TrimSuffix(message("user", "original"), "\n"), "}") + `,"metadata":` + value + "}\n"
		p, err := ReadCodex(strings.NewReader(raw), enrolled, Cursor{}, 1)
		if err != nil || len(p.Messages) != 1 || p.Messages[0].Text != "original" {
			t.Fatalf("empty metadata: %+v %v", p, err)
		}
	}
	for _, value := range []string{`[]`, `"EXCLUDED"`, `true`, `1`} {
		raw := metadata() + strings.TrimSuffix(strings.TrimSuffix(message("user", "original"), "\n"), "}") + `,"metadata":` + value + "}\n"
		if _, err := ReadCodex(strings.NewReader(raw), enrolled, Cursor{}, 1); !errors.Is(err, ErrFormat) {
			t.Fatalf("malformed metadata accepted: %v", err)
		}
	}
}

func TestCodexEncryptedMessageDoesNotAdmitMixedPlaintext(t *testing.T) {
	for _, encrypted := range []any{"opaque", "", map[string]string{"future": "opaque"}} {
		raw := metadata() + line("response_item", map[string]any{"type": "message", "role": "assistant", "encrypted_content": encrypted,
			"content": []any{map[string]string{"type": "output_text", "text": "EXCLUDED"}}})
		p, err := ReadCodex(strings.NewReader(raw), enrolled, Cursor{}, 1)
		if err != nil || len(p.Messages) != 0 || len(p.Gaps) != 1 || p.Gaps[0].Reason != "encrypted_message_excluded" {
			t.Fatalf("encrypted admission: %+v %v", p, err)
		}
		encoded, _ := json.Marshal(p)
		if strings.Contains(string(encoded), "EXCLUDED") {
			t.Fatal("encrypted mixed payload copied")
		}
	}
}

func phaseMessage(phase, text string) string {
	return line("response_item", map[string]any{"type": "message", "role": "assistant", "phase": phase,
		"content": []any{map[string]string{"type": "output_text", "text": text}}})
}

func TestCodexPartialAndFinalAnswersRetainOccurrencesAcrossPagingAndRetry(t *testing.T) {
	// Identical text is still two occurrences, not a streaming replacement.
	raw := metadata() + phaseMessage("partial_answer", "same\n\t answer") + phaseMessage("final_answer", "same\n\t answer")
	first, err := ReadCodex(strings.NewReader(raw), enrolled, Cursor{}, 1)
	if err != nil || first.Complete || len(first.Messages) != 1 || first.Cursor.Lines != 2 || len(first.Gaps) != 0 {
		t.Fatalf("partial page: %+v %v", first, err)
	}
	retry, err := ReadCodex(strings.NewReader(raw), enrolled, Cursor{}, 1)
	if err != nil || len(retry.Messages) != 1 || retry.Messages[0] != first.Messages[0] || retry.Cursor != first.Cursor {
		t.Fatalf("unacknowledged retry changed: %+v %v", retry, err)
	}
	second, err := ReadCodex(strings.NewReader(raw), enrolled, first.Cursor, 1)
	if err != nil || !second.Complete || len(second.Messages) != 1 || second.Cursor.Lines != 3 || len(second.Gaps) != 0 {
		t.Fatalf("final page: %+v %v", second, err)
	}
	a, b := first.Messages[0], second.Messages[0]
	if a.ID == b.ID || a.Sequence != 2 || b.Sequence != 3 || a.Text != "same\n\t answer" || b.Text != a.Text || a.Role != "assistant" || b.Role != "assistant" {
		t.Fatalf("occurrence loss: %+v %+v", a, b)
	}
	whole, err := ReadCodex(strings.NewReader(raw), enrolled, Cursor{}, 2)
	if err != nil || len(whole.Messages) != 2 || whole.Messages[0] != a || whole.Messages[1] != b || whole.Cursor != second.Cursor {
		t.Fatalf("page size changed identities: %+v %v", whole, err)
	}
	replay, err := ReadCodex(strings.NewReader(raw), enrolled, second.Cursor, 1)
	if err != nil || len(replay.Messages) != 0 || replay.Cursor != second.Cursor || !replay.Complete {
		t.Fatalf("acknowledged replay: %+v %v", replay, err)
	}
}

func TestCodexPartialAnswerCannotBypassExclusions(t *testing.T) {
	cases := []struct {
		name, reason string
		change       func(map[string]any, map[string]any)
	}{
		{"provenance", "host_provenance_metadata_unsupported", func(row, item map[string]any) {
			row["metadata"] = map[string]any{"delivered_assistant_message": "EXCLUDED"}
		}},
		{"encrypted", "encrypted_message_excluded", func(row, item map[string]any) { item["encrypted_content"] = "EXCLUDED" }},
		{"analysis", "nonvisible_channel_excluded", func(row, item map[string]any) { item["channel"] = "analysis" }},
		{"unknown-channel", "nonvisible_channel_excluded", func(row, item map[string]any) { item["channel"] = "future" }},
		{"developer", "nonvisible_role_excluded", func(row, item map[string]any) { item["role"] = "developer" }},
		{"reasoning", "reasoning_excluded", func(row, item map[string]any) { item["type"] = "reasoning" }},
		{"tool", "tool_or_response_type_unsupported", func(row, item map[string]any) { item["type"] = "function_call_output" }},
		{"attachment", "attachment_or_content_unsupported", func(row, item map[string]any) {
			item["content"] = []any{map[string]string{"type": "input_image", "image_url": "EXCLUDED"}}
		}},
		{"encrypted-part", "attachment_or_content_unsupported", func(row, item map[string]any) {
			item["content"] = []any{map[string]string{"type": "encrypted_content", "encrypted_content": "EXCLUDED"}}
		}},
		{"unknown-phase", "unknown_phase_excluded", func(row, item map[string]any) { item["phase"] = "future_answer" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := map[string]any{"type": "message", "role": "assistant", "phase": "partial_answer", "content": []any{map[string]string{"type": "output_text", "text": "EXCLUDED"}}}
			row := map[string]any{"timestamp": "2026-10-16T10:00:00Z", "type": "response_item", "payload": item}
			tc.change(row, item)
			encoded, err := json.Marshal(row)
			if err != nil {
				t.Fatal(err)
			}
			raw := metadata() + string(encoded) + "\n" + phaseMessage("partial_answer", "visible")
			page, err := ReadCodex(strings.NewReader(raw), enrolled, Cursor{}, 1)
			if err != nil || len(page.Messages) != 1 || page.Messages[0].Text != "visible" || len(page.Gaps) != 1 || page.Gaps[0] != (Gap{2, tc.reason}) || page.Cursor.Lines != 3 {
				t.Fatalf("exclusion bypass: %+v %v", page, err)
			}
			encoded, err = json.Marshal(page)
			if err != nil || strings.Contains(string(encoded), "EXCLUDED") {
				t.Fatal("excluded bytes escaped")
			}
		})
	}
}
