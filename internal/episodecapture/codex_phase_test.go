//go:build linux || darwin

package episodecapture

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/episodeclient"
	"github.com/sirerun/serenity/internal/episodehost"
)

func codexAnswer(phase string) string {
	return historyLine("response_item", map[string]any{"type": "message", "role": "assistant", "phase": phase,
		"content": []any{map[string]string{"type": "output_text", "text": "answer password=synthetic-secret"}}})
}

// In-memory sink only: no endpoint, credentials, or upload is involved.
type phaseSink struct{ records []episodeclient.Record }

func (s *phaseSink) Reconcile(_ context.Context, key episodeclient.ReplayKey) (episodeclient.Verification, error) {
	now := time.Now()
	return episodeclient.Verification{Binding: key.Binding, Workspace: key.Workspace, Allowed: true, Full: true,
		GenerationKnown: true, SyncedAt: now, ExpiresAt: now.Add(30 * time.Second), Conversations: map[string]bool{}}, nil
}
func (s *phaseSink) Publish(_ context.Context, record episodeclient.Record, _ uint64) (episodeclient.Receipt, error) {
	s.records = append(s.records, record)
	return episodeclient.Receipt{Key: episodeclient.ReplayKey{Binding: record.Binding, Workspace: record.Workspace,
		Conversation: record.Conversation, Event: record.Event}, Digest: episodeclient.DigestRecord(record), Complete: true}, nil
}

func TestDurableCodexPhasesRedactedAcrossRestartAndRetry(t *testing.T) {
	o := progressOptions(t, 10)
	q, err := episodeclient.OpenQueue(o)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if q == nil {
			return
		}
		if err := q.Close(); err != nil {
			t.Error(err)
		}
	}()
	enrollment := episodehost.Enrollment{SessionID: "session1", VisibleSession: true}
	raw := historyLine("session_meta", map[string]string{"id": "session1", "cli_version": episodehost.CodexVersion}) + codexAnswer("partial_answer")
	first, err := StageCodexDurable(q, strings.NewReader(raw), enrollment, "workspace1", true)
	if err != nil || first.Admitted != 1 || first.Cursor.Lines != 2 {
		t.Fatalf("partial: %+v %v", first, err)
	}
	if err := q.Close(); err != nil {
		t.Fatal(err)
	}
	q, err = episodeclient.OpenQueue(o)
	if err != nil {
		t.Fatal(err)
	}
	raw += codexAnswer("final_answer")
	second, err := StageCodexDurable(q, strings.NewReader(raw), enrollment, "workspace1", true)
	if err != nil || second.Admitted != 1 || second.Cursor.Lines != 3 {
		t.Fatalf("final: %+v %v", second, err)
	}
	retry, err := StageCodexDurable(q, strings.NewReader(raw), enrollment, "workspace1", true)
	if err != nil || retry.Admitted != 0 || retry.Cursor != second.Cursor {
		t.Fatalf("retry: %+v %v", retry, err)
	}
	sink := &phaseSink{}
	result, err := q.Drain(context.Background(), sink, sink, 10)
	if err != nil || result.Published != 2 || result.Remaining != 0 || len(sink.records) != 2 {
		t.Fatalf("drain: %+v %v", result, err)
	}
	for _, record := range sink.records {
		if record.Text != "answer [REDACTED]" || record.Role != "assistant" {
			t.Fatalf("unprotected answer: %+v", record)
		}
	}
	if sink.records[0].Event == sink.records[1].Event || sink.records[0].Sequence != 2 || sink.records[1].Sequence != 3 {
		t.Fatal("partial/final occurrences conflated")
	}
}

func TestDurableCodexPartialAnswerPolicyRefusalPreservesCursor(t *testing.T) {
	for _, mode := range []string{"paused", "private-or-unknown", "real-history"} {
		t.Run(mode, func(t *testing.T) {
			q, err := episodeclient.OpenQueue(progressOptions(t, 10))
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := q.Close(); err != nil {
					t.Error(err)
				}
			}()
			enrollment := episodehost.Enrollment{SessionID: "session1", VisibleSession: true}
			raw := historyLine("session_meta", map[string]string{"id": "session1", "cli_version": episodehost.CodexVersion}) + codexAnswer("partial_answer")
			key := episodeclient.ImportKey{Format: episodeclient.ProgressFormat("codex", episodehost.CodexVersion), Source: "session1", Workspace: "workspace1", Conversation: "session1"}
			synthetic := true
			var want error
			scope := episodeclient.Scope{Workspace: "workspace1", Conversation: "session1"}
			switch mode {
			case "paused":
				if _, err := q.Pause(scope); err != nil {
					t.Fatal(err)
				}
				want = episodeclient.ErrPaused
			case "private-or-unknown":
				// Host privacy is not inferred from the transcript; trusted visible enrollment is required.
				enrollment.VisibleSession = false
				want = episodehost.ErrEnrollment
			case "real-history":
				synthetic = false
				want = ErrUnavailable
			}
			result, err := StageCodexDurable(q, strings.NewReader(raw), enrollment, "workspace1", synthetic)
			if err != want || result.Cursor != (episodehost.Cursor{}) || result.Admitted != 0 {
				t.Fatalf("refusal: %+v %v", result, err)
			}
			progress, err := q.ImportProgress(key)
			if err != nil || progress.Cursor.Records != 0 {
				t.Fatalf("refused cursor advanced: %+v %v", progress, err)
			}
			stats, err := q.Stats()
			if err != nil || stats.Items != 0 {
				t.Fatalf("refused content staged: %+v %v", stats, err)
			}
			if mode == "paused" {
				if err := q.Resume(scope); err != nil {
					t.Fatal(err)
				}
				result, err = StageCodexDurable(q, strings.NewReader(raw), enrollment, "workspace1", true)
				if err != nil || result.Admitted != 1 || result.Cursor.Lines != 2 {
					t.Fatalf("resume skipped answer: %+v %v", result, err)
				}
			}
		})
	}
}
