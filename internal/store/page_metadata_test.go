package store

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/domain"
)

func TestPageMetadataPreservesPrivacyAndPrecision(t *testing.T) {
	p := NewEntityPage(domain.Entity{Type: "person", Slug: "ava", Aliases: []string{"Ava, Example", "name: value", "[brackets]"}})
	p.Frontmatter = map[string]any{"external_id": int64(9007199254740993)}
	p.Claims = []domain.Claim{{ID: "c1", SubjectSlug: "ava", Predicate: "prefers", Family: "prefers", Object: "feature flags", Confidence: .12345, State: domain.StateActive, Visibility: domain.VisibilityPrivate, Review: true, Provenance: domain.Provenance{Meta: map[string]string{"source": "original | source", "context": "<!-- serenity:metadata:end -->"}}}}
	fw := NewFenceWriter(t.TempDir())
	first, err := fw.RenderEntity(p)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseEntityBytes(first)
	if err != nil {
		t.Fatal(err)
	}
	c := parsed.Claims[0]
	if c.Confidence != .12345 || c.Visibility != domain.VisibilityPrivate || !c.Review || c.Provenance.Meta["source"] != "original | source" {
		t.Fatalf("lost claim data: %+v", c)
	}
	if len(parsed.Entity.Aliases) != 3 || parsed.Entity.Aliases[0] != "Ava, Example" {
		t.Fatalf("aliases: %+v", parsed.Entity.Aliases)
	}
	next, err := fw.RenderEntity(parsed)
	if err != nil || !bytes.Equal(first, next) {
		t.Fatalf("round-trip changed page: %v\n%s\n%s", err, first, next)
	}
	// A human table edit stays authoritative over the supplemental metadata.
	edited := bytes.Replace(first, []byte("| feature flags | 0.12 |"), []byte("| other approach | 0.88 |"), 1)
	parsed, err = ParseEntityBytes(edited)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Claims[0].Object != "other approach" || parsed.Claims[0].Confidence != .88 {
		t.Fatal("metadata overrode human table edit")
	}
}

func TestPageMetadataRejectsAmbiguousOrInvalidAttribution(t *testing.T) {
	p := NewEntityPage(domain.Entity{Type: "person", Slug: "ava"})
	p.Claims = []domain.Claim{{ID: "c1", Predicate: "prefers", Object: "tea", Family: "prefers", Confidence: .5, State: domain.StateActive, Visibility: domain.VisibilityPrivate}}
	raw, err := NewFenceWriter("").RenderEntity(p)
	if err != nil {
		t.Fatal(err)
	}
	for name, broken := range map[string]string{
		"unclosed":           strings.Replace(string(raw), endMetadata, "", 1),
		"unknown id":         strings.Replace(string(raw), `"id": "c1"`, `"id": "missing"`, 1),
		"invalid visibility": strings.Replace(string(raw), `"visibility": "private"`, `"visibility": "public"`, 1),
		"duplicate block":    string(raw) + beginMetadata + "{}" + endMetadata,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseEntityBytes([]byte(broken)); err == nil {
				t.Fatal("accepted invalid attribution")
			}
		})
	}
}
