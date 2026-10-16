package contracts

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestEpisodeDeletionIdentityStrict(t *testing.T) {
	good := EpisodeDeletionIdentity{EpisodeDeletionBrainHash("brain-1"), strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)}
	parsed, err := ParseEpisodeDeletionIdentity(good.String())
	if err != nil || parsed != good {
		t.Fatalf("roundtrip %v %v", parsed, err)
	}
	for _, raw := range []string{"", "v2:" + strings.TrimPrefix(good.String(), "v1:"), good.String() + ":extra", strings.Replace(good.String(), strings.Repeat("a", 64), "conversation-name", 1), strings.Replace(good.String(), strings.Repeat("a", 64), strings.Repeat("A", 64), 1), strings.Replace(good.String(), strings.Repeat("b", 64), "/private/path", 1), strings.Replace(good.String(), strings.Repeat("b", 64), "email@example.test", 1), good.String() + "\n"} {
		e := DeletionEntry{SubjectType: DeletionSubjectEpisodeConversation, SubjectID: raw, Outcome: DeletionIntentRequested}
		if e.Validate() == nil {
			t.Fatalf("accepted malformed identity %q", raw)
		}
	}
	e := DeletionEntry{SubjectType: DeletionSubjectEpisodeConversation, SubjectID: good.String(), Outcome: DeletionIntentRequested}
	if err = e.Validate(); err != nil {
		t.Fatal(err)
	}
	e.Outcome = DeletionOutcomePurged
	if e.Validate() == nil {
		t.Fatal("episode entry falsely certified physical purge")
	}
	e.Outcome = DeletionIntentRequested
	e.SubjectType = "future_episode"
	if e.Validate() == nil {
		t.Fatal("unknown subject accepted")
	}
}

func TestEpisodeDeletionPreservesLegacyObjectBytes(t *testing.T) {
	for _, subject := range []DeletionSubjectType{DeletionSubjectAccount, DeletionSubjectBrain} {
		o := JournalObject{Kind: JournalKindEntry, Generation: 1, SequenceID: 1, WriterID: "writer", SubjectType: subject, SubjectID: "opaque-id", Outcome: DeletionIntentRequested, RecordedAt: time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)}
		raw, err := o.Encode()
		if err != nil {
			t.Fatal(err)
		}
		want := `{"kind":"entry","generation":1,"sequence_id":1,"prev_hash":"","writer_id":"writer","subject_type":"` + string(subject) + `","subject_id":"opaque-id","outcome":"requested","recorded_at":"2026-10-16T00:00:00Z"}`
		if string(raw) != want {
			t.Fatalf("legacy bytes changed: %s", raw)
		}
	}
}

func TestEpisodeDeletionOldReaderRejectsNewSubject(t *testing.T) {
	// Pin the pre-extension validator's subject dispatch. An old binary decodes
	// this unchanged layout but rejects the entry, including during chain reads.
	id := EpisodeDeletionIdentity{EpisodeDeletionBrainHash("brain"), strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)}
	o := JournalObject{Kind: JournalKindEntry, SubjectType: DeletionSubjectEpisodeConversation, SubjectID: id.String(), Outcome: DeletionIntentRequested}
	raw, err := o.Encode()
	if err != nil {
		t.Fatal(err)
	}
	var old JournalObject
	if err = json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	if old.SubjectType == DeletionSubjectAccount || old.SubjectType == DeletionSubjectBrain {
		t.Fatal("new entry was disguised as a legacy subject")
	}
	if strings.Contains(string(raw), `"brain_hash"`) {
		t.Fatal("immutable journal layout gained fields")
	}
}
