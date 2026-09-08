package compose

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/router"
	"github.com/sirerun/serenity/internal/store"
)

func TestComposeNativeClaimWithDeletedSourceDoesNotEgress(t *testing.T) {
	root := reviewComposeRoot(t)
	ss := store.NewSourceStore(root)
	source, err := ss.Write([]byte("Original workshop evidence."), domain.Source{Kind: "file", URI: "fixture:workshop"})
	if err != nil {
		t.Fatal(err)
	}
	page := store.NewEntityPage(domain.Entity{Type: "person", Slug: "demo-person"})
	page.Claims = []domain.Claim{{ID: "native", SubjectSlug: "demo-person", Predicate: "works_at", Family: "works_at", Object: "Blue Heron Workshop", State: domain.StateActive, Confidence: .9, Provenance: domain.Provenance{Actor: "machine", SourceSHA256: source.SHA256}}}
	if _, err := store.NewFenceWriter(root).WriteEntity(page); err != nil {
		t.Fatal(err)
	}
	var prompt string
	provider := &fakeProvider{modelVersion: "fixture@v1", resp: router.Response{Text: "Blue Heron Workshop [claim:native]."}, sentPrompt: &prompt}
	composer := New(root, config.Default(), fakeSearchStore{}, nil, newTestRouter(provider), "fixture@v1")
	if _, err := composer.Ask(context.Background(), "Blue Heron Workshop"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "Blue Heron Workshop") {
		t.Fatal("positive canonical evidence did not reach provider")
	}
	if err := os.RemoveAll(ss.DirFor(source.SHA256)); err != nil {
		t.Fatal(err)
	}
	prompt = ""
	answer, err := composer.Ask(context.Background(), "Blue Heron Workshop")
	if err != nil {
		t.Fatal(err)
	}
	if prompt != "" || answer.Gap == "" || len(answer.Citations) != 0 {
		t.Fatalf("dangling attribution reached provider: prompt=%q answer=%+v", prompt, answer)
	}
}
