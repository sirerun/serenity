package authority

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

func TestEpisodeIntentAuthorityAdmission(t *testing.T) {
	id := contracts.EpisodeDeletionIdentity{BrainHash: contracts.EpisodeDeletionBrainHash("synthetic-brain"), ConversationHash: strings.Repeat("a", 64), CommandHash: strings.Repeat("b", 64), RequestDigest: strings.Repeat("c", 64)}.String()
	for _, tc := range []struct {
		name    string
		subject contracts.DeletionSubjectType
		id      string
		outcome contracts.DeletionOutcome
		valid   bool
	}{
		{"episode", contracts.DeletionSubjectEpisodeConversation, id, contracts.DeletionIntentRequested, true},
		{"episode-purged", contracts.DeletionSubjectEpisodeConversation, id, contracts.DeletionOutcomePurged, false},
		{"episode-plaintext", contracts.DeletionSubjectEpisodeConversation, "synthetic-conversation", contracts.DeletionIntentRequested, false},
		{"episode-uppercase", contracts.DeletionSubjectEpisodeConversation, strings.ToUpper(id), contracts.DeletionIntentRequested, false},
		{"unknown", "unknown", id, contracts.DeletionIntentRequested, false},
		{"account", contracts.DeletionSubjectAccount, "synthetic-account", contracts.DeletionIntentRequested, true},
		{"brain-purged", contracts.DeletionSubjectBrain, "synthetic-brain", contracts.DeletionOutcomePurged, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, c, l, now, _, _ := activeFixture(t)
			body, err := (contracts.JournalObject{Kind: contracts.JournalKindEntry, Generation: 1, SequenceID: 1, WriterID: l.WriterID, SubjectType: tc.subject, SubjectID: tc.id, Outcome: tc.outcome, RecordedAt: *now}).Encode()
			if err != nil {
				t.Fatal(err)
			}
			key := contracts.JournalKey(1, 1)
			_, err = c.Store(l).PutIfAbsent(context.Background(), key, body)
			if !tc.valid {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("invalid entry accepted or wrong refusal: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			// A retry has the same immutable object; the independent reader admits it.
			if _, err = c.Store(l).PutIfAbsent(context.Background(), key, body); err != nil {
				t.Fatal(err)
			}
		})
	}
}
