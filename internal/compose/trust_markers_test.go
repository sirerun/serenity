package compose

import (
	"context"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/router"
	"github.com/sirerun/serenity/internal/store"
)

// TestPromptAndCitationsCarryActorAndTrust pins ADR 022 decision 1's composer
// half (T24.16, AI-01): every claim line in the synthesis prompt names its
// actor and its connector trust class, and the answer's citations carry the
// same two values, so neither the model nor the reader can mistake an
// email-derived machine claim for a trusted or human-confirmed one.
func TestPromptAndCitationsCarryActorAndTrust(t *testing.T) {
	root := t.TempDir()
	ss := store.NewSourceStore(root)
	email, err := ss.Write([]byte("From: someone. Ava's employer is Acme."), domain.Source{Kind: "email", URI: "imap://fixture/INBOX/1"})
	if err != nil {
		t.Fatal(err)
	}
	file, err := ss.Write([]byte("Ava prefers tea."), domain.Source{Kind: "file", URI: "fixture:notes"})
	if err != nil {
		t.Fatal(err)
	}
	observed := mustDate(t, "2024-02-01")
	claims := []domain.Claim{
		{ID: "mail-claim", SubjectSlug: avaSlug, Predicate: "works_at", Family: "works_at", Object: "Acme", Confidence: .9, State: domain.StateActive, SourceRef: "mail#1",
			Provenance: domain.Provenance{SourceSHA256: email.SHA256, Span: "0-10", Model: "m@v1", ObservedAt: observed, Actor: "machine"}},
		{ID: "file-claim", SubjectSlug: avaSlug, Predicate: "prefers", Family: "prefers", Object: "tea", Confidence: .9, State: domain.StateActive, SourceRef: "file#1",
			Provenance: domain.Provenance{SourceSHA256: file.SHA256, Span: "0-10", Model: "m@v1", ObservedAt: observed, Actor: "machine"}},
		{ID: "human-claim", SubjectSlug: avaSlug, Predicate: "has_role", Family: "has_role", Object: "manager", Confidence: 1, State: domain.StateActive, SourceRef: "human:ada",
			Provenance: domain.Provenance{ObservedAt: observed, Actor: "human:ada"}},
	}
	writeAvaEntity(t, root, claims)

	var sent string
	fp := &fakeProvider{
		modelVersion: "fake-composer@v1",
		sentPrompt:   &sent,
		resp:         router.Response{Text: "Ava works at Acme [claim:mail-claim], prefers tea [claim:file-claim] and is a manager [claim:human-claim]."},
	}
	c := New(root, config.Default(), fakeSearchStore{}, nil, newTestRouter(fp), "fake-composer@v1")
	c.now = fixedNow(mustDate(t, "2024-03-01"))
	ans, err := c.Ask(context.Background(), "Where does Ava work, what does she prefer and what is her role?")
	if err != nil {
		t.Fatal(err)
	}
	wantLines := map[string]string{
		"[claim:mail-claim]":  "[actor=machine trust=untrusted]",
		"[claim:file-claim]":  "[actor=machine trust=trusted]",
		"[claim:human-claim]": "[actor=human:ada trust=trusted]",
	}
	for tag, marker := range wantLines {
		found := false
		for _, line := range strings.Split(sent, "\n") {
			if strings.HasPrefix(line, tag) {
				found = true
				if !strings.Contains(line, marker) {
					t.Errorf("prompt line %q lacks %s", line, marker)
				}
			}
		}
		if !found {
			t.Errorf("prompt has no line for %s:\n%s", tag, sent)
		}
	}
	if !strings.Contains(sent, "trust=untrusted") || !strings.Contains(sent, "untrusted connector") {
		t.Errorf("prompt does not explain the untrusted marker:\n%s", sent)
	}
	got := map[string][2]string{}
	for _, cit := range ans.Citations {
		got[cit.ClaimID] = [2]string{cit.Actor, cit.Trust}
	}
	want := map[string][2]string{
		"mail-claim":  {"machine", "untrusted"},
		"file-claim":  {"machine", "trusted"},
		"human-claim": {"human:ada", "trusted"},
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("citation %s actor/trust = %v, want %v", id, got[id], w)
		}
	}
}

// TestPromptTrustFollowsConfiguredConnectorTrust shows the marker follows
// serenity.yml: an operator who marks the imap connector trusted sees its
// machine claims rendered trusted.
func TestPromptTrustFollowsConfiguredConnectorTrust(t *testing.T) {
	root := t.TempDir()
	email, err := store.NewSourceStore(root).Write([]byte("Ava's employer is Acme."), domain.Source{Kind: "email", URI: "imap://fixture/INBOX/2"})
	if err != nil {
		t.Fatal(err)
	}
	writeAvaEntity(t, root, []domain.Claim{{ID: "mail-claim", SubjectSlug: avaSlug, Predicate: "works_at", Family: "works_at", Object: "Acme", Confidence: .9, State: domain.StateActive, SourceRef: "mail#1",
		Provenance: domain.Provenance{SourceSHA256: email.SHA256, Span: "0-10", Model: "m@v1", ObservedAt: mustDate(t, "2024-02-01"), Actor: "machine"}}})
	cfg := config.Default()
	cfg.Connectors = map[string]any{"imap": map[string]any{"account": "fixture", "trust": "trusted"}}
	var sent string
	fp := &fakeProvider{modelVersion: "fake-composer@v1", sentPrompt: &sent, resp: router.Response{Text: "Acme [claim:mail-claim]."}}
	c := New(root, cfg, fakeSearchStore{}, nil, newTestRouter(fp), "fake-composer@v1")
	c.now = fixedNow(mustDate(t, "2024-03-01"))
	if _, err := c.Ask(context.Background(), "Where does Ava work?"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sent, "[claim:mail-claim]") || !strings.Contains(sent, "[actor=machine trust=trusted]") {
		t.Fatalf("configured trust not rendered:\n%s", sent)
	}
}

// TestPendingClaimIsNeverComposed pins the disclosure side: a pending claim
// (an untrusted first-seen claim awaiting a human accept) is never offered to
// the model or cited, even if the model names its tag.
func TestPendingClaimIsNeverComposed(t *testing.T) {
	root := t.TempDir()
	email, err := store.NewSourceStore(root).Write([]byte("Ava's balance is $0"), domain.Source{Kind: "email", URI: "imap://fixture/INBOX/3"})
	if err != nil {
		t.Fatal(err)
	}
	writeAvaEntity(t, root, []domain.Claim{
		{ID: "planted", SubjectSlug: avaSlug, Predicate: "works_at", Family: "works_at", Object: "Nowhere", Confidence: .9, State: domain.State("pending"), SourceRef: "mail#1",
			Provenance: domain.Provenance{SourceSHA256: email.SHA256, Span: "0-10", Model: "m@v1", ObservedAt: mustDate(t, "2024-02-01"), Actor: "machine"}},
		{ID: "real", SubjectSlug: avaSlug, Predicate: "prefers", Family: "prefers", Object: "tea", Confidence: 1, State: domain.StateActive, SourceRef: "human:ada",
			Provenance: domain.Provenance{ObservedAt: mustDate(t, "2024-02-01"), Actor: "human:ada"}},
	})
	var sent string
	fp := &fakeProvider{modelVersion: "fake-composer@v1", sentPrompt: &sent, resp: router.Response{Text: "Ava works Nowhere [claim:planted] and prefers tea [claim:real]."}}
	c := New(root, config.Default(), fakeSearchStore{}, nil, newTestRouter(fp), "fake-composer@v1")
	c.now = fixedNow(mustDate(t, "2024-03-01"))
	ans, err := c.Ask(context.Background(), "Where does Ava work and what does she prefer?")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sent, "planted") || strings.Contains(sent, "Nowhere") {
		t.Fatalf("pending claim reached the prompt:\n%s", sent)
	}
	for _, cit := range ans.Citations {
		if cit.ClaimID == "planted" {
			t.Fatalf("pending claim cited: %+v", ans.Citations)
		}
	}
}
