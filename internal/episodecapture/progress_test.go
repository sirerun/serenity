//go:build linux || darwin

package episodecapture

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/episodeclient"
	"github.com/sirerun/serenity/internal/episodehost"
)

func historyLine(kind string, payload any) string {
	b, _ := json.Marshal(map[string]any{"timestamp": "2026-10-16T10:00:00Z", "type": kind, "payload": payload})
	return string(b) + "\n"
}
func historyMessage(text string) string {
	return historyLine("response_item", map[string]any{"type": "message", "role": "user", "content": []any{map[string]string{"type": "input_text", "text": text}}})
}
func progressOptions(t *testing.T, max int) episodeclient.Options {
	t.Helper()
	dir, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	return episodeclient.Options{Directory: filepath.Join(dir, "queue"), Key: bytes.Repeat([]byte{11}, 32), Policy: episodeclient.Policy{Binding: testBinding, Mappings: []episodeclient.Mapping{{Workspace: "workspace1", Binding: testBinding}}}, MaxItems: max, MaxBytes: 1 << 20}
}
func TestDurableCodexPageRestartGapsAndPartialTail(t *testing.T) {
	o := progressOptions(t, 10)
	q, e := episodeclient.OpenQueue(o)
	if e != nil {
		t.Fatal(e)
	}
	enrollment := episodehost.Enrollment{SessionID: "session1", VisibleSession: true}
	raw := historyLine("session_meta", map[string]string{"id": "session1", "cli_version": episodehost.CodexVersion}) + historyMessage("visible password=synthetic-secret") + historyLine("response_item", map[string]string{"type": "reasoning", "summary": "hidden"})
	tail := historyMessage("later")
	first, e := StageCodexDurable(q, strings.NewReader(raw+strings.TrimSuffix(tail, "\n")), enrollment, "workspace1", true)
	if e != nil || first.Admitted != 1 || !first.PartialTail || first.Cursor.Lines != 3 {
		t.Fatalf("first %+v %v", first, e)
	}
	if e = q.Close(); e != nil {
		t.Fatal(e)
	}
	q, e = episodeclient.OpenQueue(o)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := q.Close(); e != nil {
			t.Error(e)
		}
	}()
	second, e := StageCodexDurable(q, strings.NewReader(raw+tail), enrollment, "workspace1", true)
	if e != nil || second.Admitted != 1 || !second.Complete || second.Cursor.Lines != 4 {
		t.Fatalf("second %+v %v", second, e)
	}
	third, e := StageCodexDurable(q, strings.NewReader(raw+tail), enrollment, "workspace1", true)
	if e != nil || third.Admitted != 0 {
		t.Fatalf("replay %+v %v", third, e)
	}
	p, e := q.ImportProgress(episodeclient.ImportKey{Format: episodeclient.ProgressFormat("codex", episodehost.CodexVersion), Source: "session1", Workspace: "workspace1", Conversation: "session1"})
	if e != nil || p.Gaps["reasoning_excluded"] != 1 || p.Gaps["partial_tail"] != 0 || p.PartialTail {
		t.Fatalf("gaps %+v %v", p, e)
	}
	if _, e = StageCodexDurable(q, strings.NewReader(strings.Replace(raw, "visible", "changed", 1)+tail), enrollment, "workspace1", true); e != episodehost.ErrRewrite {
		t.Fatalf("rewrite %v", e)
	}
	rawState, e := os.ReadFile(filepath.Join(o.Directory, "queue.bin"))
	if e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{"visible", "synthetic-secret", "hidden", "reasoning_excluded", "session1"} {
		if bytes.Contains(rawState, []byte(s)) {
			t.Fatal("plaintext state")
		}
	}
}
func TestDurableCodexPageCapacityCannotSkipOrPartiallyStage(t *testing.T) {
	o := progressOptions(t, 1)
	q, e := episodeclient.OpenQueue(o)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := q.Close(); e != nil {
			t.Error(e)
		}
	}()
	raw := historyLine("session_meta", map[string]string{"id": "session1", "cli_version": episodehost.CodexVersion}) + historyMessage("one") + historyMessage("two")
	enroll := episodehost.Enrollment{SessionID: "session1", VisibleSession: true}
	if _, e = StageCodexDurable(q, strings.NewReader(raw), enroll, "workspace1", true); e != episodeclient.ErrCapacity {
		t.Fatalf("capacity %v", e)
	}
	st, e := q.Stats()
	if e != nil || st.Items != 0 {
		t.Fatalf("partial %+v %v", st, e)
	}
	if _, e = StageCodexDurable(q, strings.NewReader(raw), enroll, "workspace1", false); e != ErrUnavailable {
		t.Fatal("real history enabled")
	}
}
