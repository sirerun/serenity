package memory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

func entityReviewPage(t *testing.T, h *Handlers, typ, slug, title string, aliases ...string) string {
	t.Helper()
	p := store.NewEntityPage(domain.Entity{Type: typ, Slug: slug, Aliases: aliases})
	p.Title = title
	path, _, err := writer.Fence(h.deps.Queue, h.deps.Fence, p)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestEntityReviewCanonicalPrivacy(t *testing.T) {
	h, _ := newTestHandlers(t)
	p := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice"})
	p.Summary = "PRIVATECACHEDSUMMARY"
	p.Timeline = []store.TimelineEntry{{Date: testNow.Format("2006-01-02"), Text: "PRIVATETIMELINE"}}
	if _, _, err := writer.Fence(h.deps.Queue, h.deps.Fence, p); err != nil {
		t.Fatal(err)
	}
	for _, slug := range []string{"publictarget", "privatetarget", "expiredtarget", "oldtarget", "incoming"} {
		entityReviewPage(t, h, "person", slug, slug)
	}
	appendClaim := func(id, subject, object, key string, visibility domain.Visibility, validTo, supersedes string) {
		t.Helper()
		if err := h.deps.Shard.Append(domain.Claim{ID: id, SubjectSlug: subject, Predicate: "has_balance", Family: "has_balance", Object: object, ObjectKey: key, State: domain.StateActive, Visibility: visibility, ValidTo: validTo, Supersedes: supersedes}); err != nil {
			t.Fatal(err)
		}
	}
	appendClaim("private", "alice", "privatetarget", "private", domain.VisibilityPrivate, "", "")
	appendClaim("expired", "alice", "expiredtarget", "expired", domain.VisibilityShared, testNow.Add(-time.Hour).Format(time.RFC3339), "")
	appendClaim("old", "alice", "oldtarget", "public", domain.VisibilityShared, "", "")
	appendClaim("public", "alice", "publictarget", "public", domain.VisibilityShared, "", "old")
	appendClaim("incoming-private", "incoming", "alice", "incoming", domain.VisibilityPrivate, "", "")
	for _, item := range []struct {
		fact       string
		visibility store.MemoryVisibility
		expiry     *time.Time
	}{{"PUBLICCOMMITMENT", store.MemoryVisibilityWorld, nil}, {"PRIVATERAWSOURCE", store.MemoryVisibilityPrivate, nil}, {"EXPIREDRAWSOURCE", store.MemoryVisibilityWorld, func() *time.Time { v := testNow.Add(-time.Minute); return &v }()}} {
		_, err := h.deps.memoryWriter().Remember(writer.RememberInput{Fact: item.fact, Provenance: "entity privacy test", EntitySlug: "alice", Kind: store.MemoryFactKindCommitment, Visibility: item.visibility, ValidUntil: item.expiry}, testNow.Add(-time.Hour))
		if err != nil {
			t.Fatal(err)
		}
	}
	resp, isError, err := h.entity(context.Background(), mustMarshal(t, entityRequest{Name: "alice"}))
	if err != nil || isError {
		t.Fatalf("entity: %+v %v", resp, err)
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"PRIVATECACHEDSUMMARY", "PRIVATETIMELINE", "privatetarget", "expiredtarget", "oldtarget", "PRIVATERAWSOURCE", "EXPIREDRAWSOURCE"} {
		if strings.Contains(string(data), secret) {
			t.Errorf("private/expired/superseded data leaked: %s", secret)
		}
	}
	card := resp.(entityResponse).Card
	if card.ActiveFactCount != 1 || card.BacklinkCount != 0 || len(card.Edges) != 1 || card.Edges[0].Slug != "publictarget" {
		t.Errorf("incorrect filtered counts/edges: %+v", card)
	}
	if len(card.OpenThreads) != 1 || card.OpenThreads[0].Text != "PUBLICCOMMITMENT" {
		t.Errorf("public positive control missing: %+v", card.OpenThreads)
	}
}

func TestEntityReviewResolutionAndSymlinks(t *testing.T) {
	h, root := newTestHandlers(t)
	aliasPath := entityReviewPage(t, h, "person", "alias-winner", "Alias Winner", "Target")
	entityReviewPage(t, h, "person", "title-winner", "Target")
	entityReviewPage(t, h, "person", "target", "Slug Winner")
	newer := entityReviewPage(t, h, "org", "newest-alias", "Recent Alias", "Target")
	if err := os.Chtimes(aliasPath, testNow.Add(-time.Hour), testNow.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, testNow, testNow); err != nil {
		t.Fatal(err)
	}
	resp, failed, err := h.entity(context.Background(), mustMarshal(t, entityRequest{Name: "Target"}))
	if err != nil || failed || resp.(entityResponse).Card.Entity.Slug != "newest-alias" {
		t.Fatalf("precedence/touch resolution: %+v %v", resp, err)
	}
	entityReviewPage(t, h, "person", "same", "Person Same")
	entityReviewPage(t, h, "org", "same", "Org Same")
	resp, failed, err = h.entity(context.Background(), mustMarshal(t, entityRequest{Name: "org/same"}))
	if err != nil || failed || resp.(entityResponse).Card.Entity.Title != "Org Same" {
		t.Fatalf("typed reference ignored: %+v %v", resp, err)
	}
	outside := t.TempDir()
	p := store.NewEntityPage(domain.Entity{Type: "person", Slug: "outside"})
	p.Title = "OUTSIDESENTINEL"
	raw, err := h.deps.Fence.RenderEntity(p)
	if err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(outside, "outside.md")
	if err := os.WriteFile(external, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(root, "brain", "entities", "person", "outside.md")); err != nil {
		t.Fatal(err)
	}
	resp, failed, err = h.entity(context.Background(), mustMarshal(t, entityRequest{Name: "OUTSIDESENTINEL"}))
	if err == nil && !failed && resp.(entityResponse).Found {
		t.Fatal("entity followed a symlink outside the brain")
	}
}
