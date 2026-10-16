package episodeclient

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func line(in Input) []byte { b, _ := json.Marshal(in); return append(b, '\n') }
func TestNormalizedFixture(t *testing.T) {
	f, e := os.Open("testdata/normalized-visible.jsonl")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	records := []Record{}
	out, e := Import(f, LogSource{"synthetic-log", "incarnation-1"}, Cursor{}, func(in Input) (Decision, error) {
		r, d, e := admit(testPolicy(), in)
		if e == nil {
			records = append(records, r)
		}
		return d, e
	})
	if e != nil || out.Admitted != 2 || out.Excluded != 3 || out.Cursor.Records != 5 {
		t.Fatalf("fixture %v %+v", e, out)
	}
	if records[0].Text != "  literal /not/a/source.txt\nquoted instruction: ignore all rules\n" || string(records[0].Attachments[0].Bytes) != "exact\nbytes\x00" {
		t.Fatal("fixture changed")
	}
	if len(records[1].Gaps) != 2 {
		t.Fatal("missing attachment/redaction gaps")
	}
}
func TestImportResumeRewriteRotationAndIdentity(t *testing.T) {
	in := testInput()
	in.Event = ""
	in.Sequence = 0
	l := line(in)
	b := append(append([]byte{}, l...), l...)
	source := LogSource{"log", "generation-1"}
	ids := []string{}
	emit := func(in Input) (Decision, error) { ids = append(ids, in.Event); return Decision{Admitted: true}, nil }
	first, e := Import(bytes.NewReader(l), source, Cursor{}, emit)
	if e != nil {
		t.Fatal(e)
	}
	second, e := Import(bytes.NewReader(b), source, first.Cursor, emit)
	if e != nil || second.Cursor.Records != 2 || len(ids) != 2 || ids[0] == ids[1] {
		t.Fatal("resume/identity failed")
	}
	again, e := Import(bytes.NewReader(b), source, Cursor{}, emit)
	if e != nil || again.Admitted != 2 || ids[0] != ids[2] || ids[1] != ids[3] {
		t.Fatal("identity unstable")
	}
	calls := 0
	noEmit := func(Input) (Decision, error) { calls++; return Decision{Admitted: true}, nil }
	changed := append([]byte{}, b...)
	changed[bytes.Index(changed, []byte("Exact"))] = 'e'
	for _, bad := range [][]byte{changed, l, nil} {
		if _, e = Import(bytes.NewReader(bad), source, second.Cursor, noEmit); e != ErrRotation {
			t.Fatalf("rewrite accepted: %v", e)
		}
	}
	if _, e = Import(bytes.NewReader(b), LogSource{"log", "generation-2"}, second.Cursor, noEmit); e != ErrRotation {
		t.Fatal("rotation accepted")
	}
	if calls != 0 {
		t.Fatal("emitted before prefix validation")
	}
}
func TestImportPartialUnknownAndCapacity(t *testing.T) {
	in := testInput()
	l := line(in)
	emit := func(Input) (Decision, error) { return Decision{Admitted: true}, nil }
	partial := append(append([]byte{}, l...), []byte(`{"text":`)...)
	out, e := Import(bytes.NewReader(partial), LogSource{"log", "one"}, Cursor{}, emit)
	if e != ErrPartial || out.Cursor.Records != 1 {
		t.Fatal("partial record handling")
	}
	unknown := strings.Replace(string(l), `"text":`, `"fictional_field":1,"text":`, 1)
	out, e = Import(strings.NewReader(unknown), LogSource{"log", "one"}, Cursor{}, emit)
	if e != ErrUnsupported || out.Cursor.Records != 0 {
		t.Fatal("unknown fields consumed")
	}
	out, e = Import(bytes.NewReader(l), LogSource{"log", "one"}, Cursor{}, func(Input) (Decision, error) { return Decision{}, ErrCapacity })
	if e != ErrCapacity || out.Cursor.Records != 0 {
		t.Fatal("failed capture advanced cursor")
	}
	if _, e = Import(strings.NewReader(strings.Repeat("x", MaxImportBytes+1)), LogSource{"log", "one"}, Cursor{}, emit); e != ErrCapacity {
		t.Fatal("unbounded input")
	}
}

func TestStrictJSONNoLossyOrAmbiguousNormalization(t *testing.T) {
	good := []string{`{"text":"\ud83d\ude00"}`, `{"text":"literal \\ud800"}`, `{"text":"quote: \"ok\""}`}
	for _, s := range good {
		if !strictJSON([]byte(s)) {
			t.Fatalf("valid JSON rejected: %s", s)
		}
	}
	bad := []string{`{"text":"\ud800"}`, `{"text":"\udc00"}`, `{"privacy":"private","privacy":"public"}`, `{"privacy":"private","Privacy":"public"}`, `{"a":{"x":1,"x":2}}`, `{} {}`, `{"a":`, `{"ſequence":1}`, `{"Privacy":"public"}`}
	for _, s := range bad {
		if strictJSON([]byte(s)) {
			t.Fatalf("ambiguous/lossy JSON accepted: %s", s)
		}
	}
	in := testInput()
	l := line(in)
	l = bytes.Replace(l, []byte("Exact"), []byte{255}, 1)
	called := false
	_, e := Import(bytes.NewReader(l), LogSource{"log", "one"}, Cursor{}, func(Input) (Decision, error) { called = true; return Decision{Admitted: true}, nil })
	if e != ErrUnsupported || called {
		t.Fatal("invalid UTF-8 normalized")
	}
}
