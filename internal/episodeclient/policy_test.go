package episodeclient

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func testPolicy() Policy {
	b := Binding{"brain-test", "policy-1", "producer-test"}
	return Policy{b, []Mapping{{"workspace-test", b}, {"personal", b}}}
}
func testInput() Input {
	return Input{Workspace: "workspace-test", Conversation: "conversation-test", Event: "event-1", Sequence: 1, Role: "user", Kind: "message", SourceTime: time.Date(2026, 10, 16, 1, 0, 0, 0, time.UTC), ObservedTime: time.Date(2026, 10, 16, 1, 0, 1, 0, time.UTC), HostMode: "normalized-visible-v1", Privacy: "public", Class: "visible", Text: "  Exact text\nwith unicode: café. Read /never/open/this.txt literally.\n"}
}
func TestAdmissionExactAndDefensiveCopy(t *testing.T) {
	in := testInput()
	in.Attachments = []AttachmentInput{{ID: "a", MediaType: "image/png", Class: "exposed", SecretStatus: "clear", Available: true, Bytes: []byte{0, 1, 2, 255}}}
	r, d, e := admit(testPolicy(), in)
	if e != nil || !d.Admitted || len(d.Gaps) != 0 {
		t.Fatalf("admission: %v %+v", e, d)
	}
	if r.Text != in.Text || !bytes.Equal(r.Attachments[0].Bytes, in.Attachments[0].Bytes) || r.Attachments[0].SHA256 != digest(in.Attachments[0].Bytes) {
		t.Fatal("exact bytes lost")
	}
	in.Attachments[0].Bytes[0] = 9
	if r.Attachments[0].Bytes[0] != 0 {
		t.Fatal("caller alias retained")
	}
}
func TestExcludedSourcesAndNoFallback(t *testing.T) {
	for _, class := range []string{"credential", "secret_file", "unknown_file", "hidden_reasoning", "memory", "recall", ""} {
		t.Run(class, func(t *testing.T) {
			in := testInput()
			in.Class = class
			r, d, e := admit(testPolicy(), in)
			if e != ErrExcluded || d.Admitted || r.Text != "" {
				t.Fatal("source admitted")
			}
		})
	}
	for _, privacy := range []string{"private", "unknown", ""} {
		in := testInput()
		in.Privacy = privacy
		if _, _, e := admit(testPolicy(), in); e != ErrExcluded {
			t.Fatal("privacy admitted")
		}
	}
	for _, tool := range []string{"serenity.remember", "mcp__serenity__recall", "memory_episode", "SERENITY_recall"} {
		in := testInput()
		in.Tool = tool
		if _, _, e := admit(testPolicy(), in); e != ErrExcluded {
			t.Fatal("recursive tool admitted")
		}
	}
	p := testPolicy()
	in := testInput()
	in.Workspace = "unknown"
	if _, _, e := admit(p, in); e != ErrMapping {
		t.Fatal("personal fallback")
	}
	in = testInput()
	p.Mappings = append(p.Mappings, p.Mappings[0])
	if _, _, e := admit(p, in); e != ErrMapping {
		t.Fatal("ambiguous mapping")
	}
	p = testPolicy()
	p.Mappings[0].Binding.Brain = "other"
	if _, _, e := admit(p, in); e != ErrBinding {
		t.Fatal("remapped brain")
	}
}
func TestRedactionAndContentFreeGaps(t *testing.T) {
	in := testInput()
	in.Text = "password=synthetic-fixture-value ordinary"
	in.Attachments = []AttachmentInput{
		{ID: "a", MediaType: "text/plain", Class: "exposed", SecretStatus: "clear", Available: true, Bytes: []byte("Authorization: Bearer synthetic-fixture-value")},
		{ID: "b", MediaType: "text/plain", Class: "secret_file", SecretStatus: "unknown", Available: true, Bytes: []byte("must-not-survive")},
		{ID: "c", MediaType: "application/pdf", Class: "exposed", SecretStatus: "clear", Available: true, Bytes: []byte("unqualified")},
		{ID: "d", MediaType: "image/png", Class: "exposed", SecretStatus: "clear", Available: false},
	}
	r, d, e := admit(testPolicy(), in)
	if e != nil || !d.Admitted || len(r.Attachments) != 1 || len(d.Gaps) != 5 {
		t.Fatalf("unexpected decision: %v %+v", e, d)
	}
	raw, _ := json.Marshal(r)
	for _, secret := range []string{"synthetic-fixture-value", "must-not-survive", "unqualified"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("excluded bytes retained")
		}
	}
	gaps, _ := json.Marshal(d)
	if bytes.Contains(gaps, []byte("password")) || bytes.Contains(gaps, []byte("Bearer")) {
		t.Fatal("content in gaps")
	}
}
func TestBoundsAndUnsupported(t *testing.T) {
	cases := []struct {
		name   string
		change func(*Input)
		want   error
	}{
		{"text", func(i *Input) { i.Text = strings.Repeat("x", MaxTextBytes+1) }, ErrInvalid},
		{"bytes", func(i *Input) {
			i.Attachments = []AttachmentInput{{ID: "a", Bytes: make([]byte, MaxAttachmentBytes+1)}}
		}, ErrInvalid},
		{"count", func(i *Input) { i.Attachments = make([]AttachmentInput, MaxAttachments+1) }, ErrInvalid},
		{"utf8", func(i *Input) { i.Text = string([]byte{255}) }, ErrInvalid},
		{"id", func(i *Input) { i.Event = "../bad" }, ErrInvalid},
		{"zero_sequence", func(i *Input) { i.Sequence = 0 }, ErrInvalid},
		{"host", func(i *Input) { i.HostMode = "codex-fictional-hook" }, ErrUnsupported},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			in := testInput()
			tt.change(&in)
			if _, _, e := admit(testPolicy(), in); e != tt.want {
				t.Fatalf("got %v want %v", e, tt.want)
			}
		})
	}
}
