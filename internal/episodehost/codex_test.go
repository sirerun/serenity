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
