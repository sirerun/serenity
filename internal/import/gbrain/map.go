package gbrain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

// Map translates all rows before writing anything, so dangling or cyclic
// supersession pointers cannot produce a partially imported page.
func Map(p Page) (*store.EntityPage, error) {
	out := store.NewEntityPage(domain.Entity{Type: p.Type, Slug: path.Base(p.Slug), Aliases: p.Aliases})
	out.Title = p.Title
	out.Frontmatter = p.Frontmatter
	out.OriginalBody = p.Body
	ids := map[string]string{}
	successors := map[string]string{}
	for _, r := range p.Rows {
		cells := r.Cells
		obj := cells["claim"]
		struck := strings.HasPrefix(obj, "~~") && strings.HasSuffix(obj, "~~") && len(obj) > 4
		if struck {
			obj = strings.TrimSpace(obj[2 : len(obj)-2])
		}
		if obj == "" {
			return nil, fmt.Errorf("%s %s #%d: empty claim", p.Path, r.Fence, r.Number)
		}
		pred := "relates_to"
		switch cells["kind"] {
		case "preference":
			pred = "prefers"
		case "commitment":
			pred = "committed_to"
		case "event":
			pred = "said"
		case "fact", "belief":
		default:
			if r.Fence == "facts" {
				return nil, fmt.Errorf("%s: unknown fact kind %q", p.Path, cells["kind"])
			}
		}
		scoreKey := "confidence"
		from, to := cells["valid_from"], cells["valid_until"]
		vis := domain.VisibilityPrivate
		actor := "machine"
		if r.Fence == "takes" {
			pred = "said"
			scoreKey = "weight"
			actor = cells["who"]
			from, to, _ = strings.Cut(strings.ReplaceAll(cells["since"], "→", "->"), "->")
			from = strings.TrimSpace(from)
			to = strings.TrimSpace(to)
		} else if cells["visibility"] == "world" {
			vis = domain.VisibilityShared
		}
		score, err := strconv.ParseFloat(cells[scoreKey], 64)
		if err != nil || math.IsNaN(score) || math.IsInf(score, 0) {
			return nil, fmt.Errorf("%s: invalid %s", p.Path, scoreKey)
		}
		meta := map[string]string{}
		for k, v := range cells {
			meta[k] = v
		}
		meta["gbrain_fence"] = r.Fence
		meta["gbrain_page"] = p.Path
		key := fmt.Sprintf("%s#%d", r.Fence, r.Number)
		// Include source path AND fence identity: two equal claims from different
		// rows/sources must never collapse, even when their content is identical.
		identity, err := json.Marshal(struct {
			Page, Fence string
			Number      int
			Cells       map[string]string
		}{p.Slug, r.Fence, r.Number, cells})
		if err != nil {
			return nil, err
		}
		digest := sha256.Sum256(identity)
		id := hex.EncodeToString(digest[:16])
		c := domain.Claim{ID: id, SubjectSlug: out.Entity.Slug, Predicate: pred, Family: pred, Object: obj, ObjectKey: store.NormalizeKey(obj), Confidence: math.Min(.90, math.Max(0, score)), ValidFrom: from, ValidTo: to, Visibility: vis, State: domain.StateActive, SourceRef: fmt.Sprintf("gbrain:%s#%d", p.Slug, r.Number), Review: true, Provenance: domain.Provenance{Actor: actor, Meta: meta}}
		if struck {
			lifecycle := cells["context"]
			if r.Fence == "takes" {
				lifecycle = cells["source"]
			}
			if match := superseded.FindStringSubmatch(lifecycle); len(match) > 0 {
				successors[key] = r.Fence + "#" + match[1]
				c.State = domain.StateSuperseded
			} else {
				c.State = domain.StateRetracted
				reason := lifecycle
				if reason == "" {
					reason = "inactive strikethrough; no reason recorded in source"
				}
				c.Provenance.Meta["forgotten_reason"] = reason
			}
		}
		ids[key] = id
		out.Claims = append(out.Claims, c)
	}
	for i, r := range p.Rows {
		key := fmt.Sprintf("%s#%d", r.Fence, r.Number)
		if next, ok := successors[key]; ok {
			id, ok := ids[next]
			if !ok {
				return nil, fmt.Errorf("%s: %s references missing %s", p.Path, key, next)
			}
			out.Claims[i].SupersededBy = id
			seen := map[string]bool{key: true}
			for n := next; n != ""; n = successors[n] {
				if seen[n] {
					return nil, fmt.Errorf("%s: supersession cycle", p.Path)
				}
				seen[n] = true
			}
		}
	}
	inTimeline := false
	for _, ln := range strings.Split(p.Body, "\n") {
		if strings.HasPrefix(ln, "## ") {
			inTimeline = strings.TrimSpace(ln) == "## Timeline"
			continue
		}
		if inTimeline && strings.HasPrefix(ln, "- ") {
			date, text, ok := strings.Cut(ln[2:], ": ")
			if !ok {
				date, text, ok = strings.Cut(ln[2:], " — ")
			}
			if !ok {
				return nil, fmt.Errorf("%s: unrecognized timeline entry %q", p.Path, ln)
			}
			out.Timeline = append(out.Timeline, store.TimelineEntry{Date: date, Text: text})
		}
	}
	seenLinks := map[string]bool{}
	for _, m := range wikiLink.FindAllStringSubmatch(p.Body, -1) {
		target, label, _ := strings.Cut(m[1], "|")
		target = strings.TrimSpace(target)
		if target == "" {
			return nil, fmt.Errorf("%s: empty wiki link", p.Path)
		}
		key := target + "\x00" + label
		if !seenLinks[key] {
			out.Links = append(out.Links, store.EntityLink{Target: target, Kind: "links_to", Label: label})
			seenLinks[key] = true
		}
	}
	return out, nil
}
