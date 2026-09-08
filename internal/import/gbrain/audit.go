package gbrain

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/sirerun/serenity/internal/domain"
)

// FieldLoss names a source field whose durable representation is missing or
// different. Values are local import output, never telemetry.
type FieldLoss struct {
	Page     string `json:"page"`
	Fence    string `json:"fence"`
	Row      int    `json:"row"`
	Field    string `json:"field"`
	Source   string `json:"source"`
	Imported string `json:"imported"`
}

// FieldReport is a representation audit, not a semantic-quality score. Fields
// counts the original fence cells inspected; Unmapped must be empty to publish.
type FieldReport struct {
	Rows     int         `json:"rows"`
	Fields   int         `json:"fields"`
	Unmapped []FieldLoss `json:"unmapped_fields"`
}

// Audit compares source rows with claims parsed from the rendered canonical
// page. It catches both mapper loss and serializer loss before publication.
// Predicate inference and confidence calibration remain reviewable semantics;
// this report verifies retained source cells and representational fields.
func Audit(p Page, claims []domain.Claim) FieldReport {
	report := FieldReport{Rows: len(p.Rows), Unmapped: []FieldLoss{}}
	byRow := map[string]domain.Claim{}
	for _, c := range claims {
		m := c.Provenance.Meta
		n, _ := strconv.Atoi(m["#"])
		key := fmt.Sprintf("%s#%d", m["gbrain_fence"], n)
		if _, duplicate := byRow[key]; duplicate || m["gbrain_page"] != p.Path || n <= 0 {
			report.Unmapped = append(report.Unmapped, FieldLoss{p.Path, m["gbrain_fence"], n, "row_identity", "one attributed claim", c.ID})
			continue
		}
		byRow[key] = c
	}
	for _, r := range p.Rows {
		report.Fields += len(r.Cells)
		key := fmt.Sprintf("%s#%d", r.Fence, r.Number)
		c, ok := byRow[key]
		if !ok {
			report.Unmapped = append(report.Unmapped, FieldLoss{p.Path, r.Fence, r.Number, "row", "present", "missing"})
			continue
		}
		check := func(field, want, got string) {
			if want != got {
				report.Unmapped = append(report.Unmapped, FieldLoss{p.Path, r.Fence, r.Number, field, want, got})
			}
		}
		keys := make([]string, 0, len(r.Cells))
		for key := range r.Cells {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, field := range keys {
			actual, exists := c.Provenance.Meta[field]
			if !exists {
				report.Unmapped = append(report.Unmapped, FieldLoss{p.Path, r.Fence, r.Number, "provenance." + field, r.Cells[field], "<missing>"})
			} else {
				check("provenance."+field, r.Cells[field], actual)
			}
		}
		check("source_ref", fmt.Sprintf("gbrain:%s#%d", p.Slug, r.Number), c.SourceRef)
		obj := r.Cells["claim"]
		if strings.HasPrefix(obj, "~~") && strings.HasSuffix(obj, "~~") && len(obj) > 4 {
			obj = strings.TrimSpace(obj[2 : len(obj)-2])
		}
		check("object", obj, c.Object)
		from, to := r.Cells["valid_from"], r.Cells["valid_until"]
		visibility := "private"
		if r.Fence == "takes" {
			from, to, _ = strings.Cut(strings.ReplaceAll(r.Cells["since"], "→", "->"), "->")
			from = strings.TrimSpace(from)
			to = strings.TrimSpace(to)
			check("actor", r.Cells["who"], c.Provenance.Actor)
		} else if r.Cells["visibility"] == "world" {
			visibility = "shared"
		}
		check("valid_from", from, c.ValidFrom)
		check("valid_to", to, c.ValidTo)
		check("visibility", visibility, string(c.Visibility))
		check("review", "true", strconv.FormatBool(c.Review))
	}
	known := map[string]bool{}
	for _, r := range p.Rows {
		known[fmt.Sprintf("%s#%d", r.Fence, r.Number)] = true
	}
	for _, c := range claims {
		n, _ := strconv.Atoi(c.Provenance.Meta["#"])
		fence := c.Provenance.Meta["gbrain_fence"]
		if !known[fmt.Sprintf("%s#%d", fence, n)] {
			report.Unmapped = append(report.Unmapped, FieldLoss{p.Path, fence, n, "row_identity", "source row", c.ID})
		}
	}
	return report
}
