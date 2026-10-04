package dashboard

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	hostgateway "github.com/sirerun/serenity/internal/hosted/gateway"
	brainstore "github.com/sirerun/serenity/internal/store"
)

func TestInspectorProjectionUsesEligibleCanonicalRecordsAndSafeMetadata(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	sources := brainstore.NewSourceStore(root)
	created := now.Add(-24 * time.Hour)

	publicFact, err := sources.WriteMemoryFact(brainstore.MemoryFactPayload{
		FormatVersion: brainstore.MemoryFactFormatVersion, RecordType: brainstore.SourceKindMemoryFact,
		LegacyID: 1, Fact: "public linked fact", Provenance: "projection fixture", EntitySlug: "alice", EntityType: "person",
		Kind: brainstore.MemoryFactKindFact, Visibility: brainstore.MemoryVisibilityWorld, CreatedAt: created,
	})
	if err != nil {
		t.Fatal(err)
	}
	expiredAt := now.Add(-time.Hour)
	expiredFact, err := sources.WriteMemoryFact(brainstore.MemoryFactPayload{
		FormatVersion: brainstore.MemoryFactFormatVersion, RecordType: brainstore.SourceKindMemoryFact,
		LegacyID: 2, Fact: "expired linked fact", Provenance: "projection fixture", EntitySlug: "expired-person", EntityType: "person",
		Kind: brainstore.MemoryFactKindFact, Visibility: brainstore.MemoryVisibilityWorld, CreatedAt: created.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sources.WriteMemoryExpiry(brainstore.MemoryExpiryPayload{
		FormatVersion: 1, RecordType: brainstore.SourceKindMemoryExpiry, TargetSHA256: expiredFact.SHA256, ExpiredAt: expiredAt,
	}); err != nil {
		t.Fatal(err)
	}
	privateFact, err := sources.WriteMemoryFact(brainstore.MemoryFactPayload{
		FormatVersion: brainstore.MemoryFactFormatVersion, RecordType: brainstore.SourceKindMemoryFact,
		LegacyID: 3, Fact: "private linked fact", Provenance: "projection fixture", EntitySlug: "private-person", EntityType: "person",
		Kind: brainstore.MemoryFactKindFact, Visibility: brainstore.MemoryVisibilityPrivate, CreatedAt: created,
	})
	if err != nil {
		t.Fatal(err)
	}

	secretBody := "RAW SOURCE BYTES projection-secret-468b"
	secretURI := "mail://private.example/message/468b"
	secretMeta := "metadata-secret-90ae"
	publicSource, err := sources.Write([]byte(secretBody), domain.Source{
		Kind: "email", URI: secretURI, OccurredAt: created,
		Meta: map[string]string{"private-note": secretMeta},
	})
	if err != nil {
		t.Fatal(err)
	}
	tombstonedSource, err := sources.Write([]byte("source to erase"), domain.Source{Kind: "note", URI: "note://tombstone-fixture", OccurredAt: created})
	if err != nil {
		t.Fatal(err)
	}

	shards := brainstore.NewShardStore(root)
	shards.Vocabulary = map[string]bool{"has_balance": true}
	appendClaim := func(claim domain.Claim) {
		t.Helper()
		if err := shards.Append(claim); err != nil {
			t.Fatal(err)
		}
	}
	appendClaim(domain.Claim{
		ID: "claim-old", SubjectSlug: "alice", Predicate: "has_balance", Family: "has_balance", Object: "old value",
		Confidence: .8, State: domain.StateSuperseded, Visibility: domain.VisibilityShared, SupersededBy: "claim-new",
		Provenance: domain.Provenance{SourceSHA256: publicSource.SHA256, ObservedAt: created},
	})
	appendClaim(domain.Claim{
		ID: "claim-new", SubjectSlug: "alice", Predicate: "has_balance", Family: "has_balance", Object: "new value",
		Confidence: .9, State: domain.StateActive, Visibility: domain.VisibilityShared, Supersedes: "claim-old",
		Provenance: domain.Provenance{SourceSHA256: publicSource.SHA256, ObservedAt: now},
	})
	appendClaim(domain.Claim{
		ID: "claim-missing-source", SubjectSlug: "missing-person", Predicate: "has_balance", Family: "has_balance", Object: "unavailable evidence",
		Confidence: .5, State: domain.StateActive, Visibility: domain.VisibilityShared,
		Provenance: domain.Provenance{SourceSHA256: strings.Repeat("f", 64), ObservedAt: now},
	})
	appendClaim(domain.Claim{
		ID: "claim-retracted", SubjectSlug: "alice", Predicate: "has_balance", Family: "has_balance", Object: "retracted value",
		Confidence: .5, State: domain.StateActive, Visibility: domain.VisibilityShared,
		Provenance: domain.Provenance{ObservedAt: now},
	})
	appendClaim(domain.Claim{
		ID: "claim-retracted", SubjectSlug: "alice", Predicate: "has_balance", Family: "has_balance", Object: "retracted value",
		Confidence: .5, State: domain.StateRetracted, Visibility: domain.VisibilityShared,
		Provenance: domain.Provenance{ObservedAt: now},
	})
	appendClaim(domain.Claim{
		ID: "claim-private", SubjectSlug: "private-person", Predicate: "has_balance", Family: "has_balance", Object: "private value",
		Confidence: .5, State: domain.StateActive, Visibility: domain.VisibilityPrivate,
		Provenance: domain.Provenance{ObservedAt: now},
	})
	appendClaim(domain.Claim{
		ID: "claim-tombstoned", SubjectSlug: "tombstoned-person", Predicate: "has_balance", Family: "has_balance", Object: "erased source value",
		Confidence: .5, State: domain.StateActive, Visibility: domain.VisibilityShared,
		Provenance: domain.Provenance{SourceSHA256: tombstonedSource.SHA256, ObservedAt: now},
	})
	if _, err = sources.TombstoneAt(tombstonedSource.SHA256, shards, now); err != nil {
		t.Fatal(err)
	}

	writeEntity := func(entity domain.Entity, title string) {
		t.Helper()
		writer := brainstore.NewFenceWriter(root)
		page := brainstore.NewEntityPage(entity)
		page.Title = title
		data, err := writer.RenderEntity(page)
		if err != nil {
			t.Fatal(err)
		}
		path := writer.PathFor(entity.Type, entity.Slug)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeEntity(domain.Entity{Type: "person", Slug: "alice"}, "Alice Public Label")
	writeEntity(domain.Entity{Type: "person", Slug: "orphan"}, "Orphan Page Must Not Appear")

	nodes, edges, err := loadInspectorGraph(context.Background(), hostgateway.InspectorReadView{Root: root, Now: now})
	if err != nil {
		t.Fatalf("load inspector projection: %v", err)
	}
	byID := make(map[string]inspectorNode, len(nodes))
	for _, node := range nodes {
		byID[node.ID] = node
	}
	for _, id := range []string{"fact:" + publicFact.SHA256, "claim:claim-old", "claim:claim-new", "source:" + publicSource.SHA256, "entity:alice"} {
		if _, ok := byID[id]; !ok {
			t.Errorf("eligible projection omitted %s", id)
		}
	}
	for _, id := range []string{
		"fact:" + expiredFact.SHA256, "fact:" + privateFact.SHA256,
		"claim:claim-missing-source", "claim:claim-retracted", "claim:claim-private", "claim:claim-tombstoned",
		"entity:expired-person", "entity:private-person", "entity:missing-person", "entity:tombstoned-person", "entity:orphan",
		"source:" + tombstonedSource.SHA256,
	} {
		if _, ok := byID[id]; ok {
			t.Errorf("ineligible projection leaked %s", id)
		}
	}
	if byID["claim:claim-old"].Status != string(domain.StateSuperseded) || byID["claim:claim-old"].SupersededBy != "claim:claim-new" || byID["claim:claim-new"].Supersedes != "claim:claim-old" {
		t.Fatalf("supersession history was not retained: old=%+v new=%+v", byID["claim:claim-old"], byID["claim:claim-new"])
	}
	if byID["entity:alice"].Label != "Alice Public Label" {
		t.Fatalf("eligible linked entity label = %q", byID["entity:alice"].Label)
	}
	var newerSupersedesOlder bool
	for _, edge := range edges {
		newerSupersedesOlder = newerSupersedesOlder || edge.Type == "supersedes" && edge.Source == "claim:claim-new" && edge.Target == "claim:claim-old"
	}
	if !newerSupersedesOlder {
		t.Fatalf("supersession edge missing: %+v", edges)
	}

	encoded, err := json.Marshal(struct {
		Nodes []inspectorNode `json:"nodes"`
		Edges []inspectorEdge `json:"edges"`
	}{Nodes: nodes, Edges: edges})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{secretBody, secretURI, secretMeta, "note://tombstone-fixture", "source to erase"} {
		if strings.Contains(string(encoded), secret) {
			t.Errorf("source content or metadata escaped into DTO: %q in %s", secret, encoded)
		}
	}
}
