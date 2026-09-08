package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/sirerun/serenity/internal/domain"
)

// EntityLink is a canonical graph edge extracted from a source wiki link.
// Target retains the gbrain page reference; Label is its optional display text.
type EntityLink struct {
	// EntitySlug names the imported destination when the target resolves.
	EntitySlug string `json:"entity_slug,omitempty"`
	Target     string `json:"target"`
	Kind       string `json:"kind"`
	Label      string `json:"label,omitempty"`
}

// Metadata supplements the human-editable claim table; it never overrides the
// table's text, state, validity or supersession pointers. Older pages need no
// migration. JSON escapes HTML marker text inside source-attribution strings.
type claimMetadata struct {
	Confidence *float64          `json:"confidence,omitempty"`
	ID         string            `json:"id"`
	Visibility domain.Visibility `json:"visibility,omitempty"`
	Provenance domain.Provenance `json:"provenance,omitzero"`
	Review     bool              `json:"review,omitempty"`
}
type pageMetadata struct {
	OriginalBody string          `json:"original_body,omitempty"`
	Frontmatter  map[string]any  `json:"frontmatter,omitempty"`
	Links        []EntityLink    `json:"links,omitempty"`
	Claims       []claimMetadata `json:"claims,omitempty"`
}

const beginMetadata = "<!-- serenity:metadata:begin -->"
const endMetadata = "<!-- serenity:metadata:end -->"

func renderPageMetadata(b *bytes.Buffer, p *EntityPage) error {
	m := pageMetadata{OriginalBody: p.OriginalBody, Frontmatter: p.Frontmatter, Links: append([]EntityLink(nil), p.Links...)}
	for _, c := range p.Claims {
		prov, err := json.Marshal(c.Provenance)
		if err != nil {
			return err
		}
		if c.Visibility != "" || c.Review || string(prov) != "{}" {
			m.Claims = append(m.Claims, claimMetadata{ID: c.ID, Visibility: c.Visibility, Provenance: c.Provenance, Review: c.Review, Confidence: &c.Confidence})
		}
	}
	if len(m.Frontmatter)+len(m.Links)+len(m.Claims)+len(m.OriginalBody) == 0 {
		return nil
	}
	sort.Slice(m.Claims, func(i, j int) bool { return m.Claims[i].ID < m.Claims[j].ID })
	sort.Slice(m.Links, func(i, j int) bool {
		a, c := m.Links[i], m.Links[j]
		if a.Target != c.Target {
			return a.Target < c.Target
		}
		if a.Kind != c.Kind {
			return a.Kind < c.Kind
		}
		return a.Label < c.Label
	})
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("page metadata: %w", err)
	}
	b.WriteString("\n" + beginMetadata + "\n")
	b.Write(raw)
	b.WriteString("\n" + endMetadata + "\n")
	return nil
}

func parsePageMetadata(s string, p *EntityPage) error {
	nb, ne := strings.Count(s, beginMetadata), strings.Count(s, endMetadata)
	if nb == 0 && ne == 0 {
		return nil
	}
	if nb != 1 || ne != 1 || strings.Index(s, beginMetadata) > strings.Index(s, endMetadata) {
		return fmt.Errorf("page metadata: requires one complete block")
	}
	raw, _ := section(s, beginMetadata, endMetadata)
	var m pageMetadata
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&m); err != nil {
		return fmt.Errorf("page metadata: %w", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("page metadata: trailing JSON data")
	}
	ids := map[string]int{}
	for i, c := range p.Claims {
		if _, ok := ids[c.ID]; ok {
			return fmt.Errorf("page metadata: duplicate claim %s", c.ID)
		}
		ids[c.ID] = i
	}
	seen := map[string]bool{}
	for _, c := range m.Claims {
		i, ok := ids[c.ID]
		if !ok || seen[c.ID] {
			return fmt.Errorf("page metadata: unknown or duplicate claim %s", c.ID)
		}
		seen[c.ID] = true
		if c.Visibility != "" && c.Visibility != domain.VisibilityPrivate && c.Visibility != domain.VisibilityShared {
			return fmt.Errorf("page metadata: invalid visibility %q", c.Visibility)
		}
		if c.Confidence != nil {
			baseline, err := strconv.ParseFloat(confCell(*c.Confidence), 64)
			if err != nil {
				return err
			}
			if baseline == p.Claims[i].Confidence {
				p.Claims[i].Confidence = *c.Confidence
			}
		}
		p.Claims[i].Visibility = c.Visibility
		p.Claims[i].Provenance = c.Provenance
		p.Claims[i].Review = c.Review
	}
	p.Frontmatter = m.Frontmatter
	p.Links = m.Links
	p.OriginalBody = m.OriginalBody
	return nil
}
