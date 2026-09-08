// Package gbrain migrates file-canonical gbrain pages without a database or LLM.
// Every semantic mapping is flagged for review; original cells remain in provenance.
package gbrain

import (
	"fmt"
	"math"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Row retains each decoded cell verbatim, including optional extension columns.
// Numbers are scoped to their fence: facts #1 and takes #1 are distinct rows.
type Row struct {
	Fence  string
	Number int
	Cells  map[string]string
}

// Page is the parsed source representation, before semantic translation.
type Page struct {
	Path        string
	Slug        string
	Type        string
	Title       string
	Aliases     []string
	Frontmatter map[string]any
	Body        string
	Rows        []Row
}

var safeSegment = regexp.MustCompile(`^[\pL\pN][\pL\pN._-]*$`)
var superseded = regexp.MustCompile(`(?i)superseded by #(\d+)`)
var wikiLink = regexp.MustCompile(`\[\[([^\]\n]+)\]\]`)

// Parse rejects malformed or ambiguous fences instead of silently dropping rows.
func Parse(name string, raw []byte) (Page, error) {
	p := Page{Path: name}
	if !utf8.Valid(raw) {
		return p, fmt.Errorf("%s: source is not UTF-8", name)
	}
	s := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		return p, fmt.Errorf("%s: missing YAML frontmatter", name)
	}
	fm, body, ok := strings.Cut(s[4:], "\n---\n")
	if !ok {
		return p, fmt.Errorf("%s: unclosed frontmatter", name)
	}
	if err := yaml.Unmarshal([]byte(fm), &p.Frontmatter); err != nil {
		return p, fmt.Errorf("%s: frontmatter: %w", name, err)
	}
	p.Slug = strings.TrimSuffix(name, ".md")
	for _, part := range strings.Split(p.Slug, "/") {
		if !safeSegment.MatchString(part) {
			return p, fmt.Errorf("%s: unsafe page path", name)
		}
	}
	p.Type, _ = p.Frontmatter["type"].(string)
	if !safeSegment.MatchString(p.Type) {
		return p, fmt.Errorf("%s: missing or unsafe type", name)
	}
	// Source path identity is retained in provenance; target slug uses its basename.
	if a, ok := p.Frontmatter["aliases"]; ok {
		list, ok := a.([]any)
		if !ok {
			return p, fmt.Errorf("%s: aliases must be a list", name)
		}
		for _, v := range list {
			a, ok := v.(string)
			if !ok || strings.ContainsAny(a, "\n\r") {
				return p, fmt.Errorf("%s: invalid alias", name)
			}
			p.Aliases = append(p.Aliases, a)
		}
	}
	p.Body = body
	p.Title = path.Base(p.Slug)
	for _, ln := range strings.Split(body, "\n") {
		if strings.HasPrefix(ln, "# ") {
			p.Title = strings.TrimPrefix(ln, "# ")
			break
		}
	}
	for _, fence := range []string{"facts", "takes"} {
		rows, err := parseFence(body, fence)
		if err != nil {
			return p, fmt.Errorf("%s: %w", name, err)
		}
		p.Rows = append(p.Rows, rows...)
	}
	return p, nil
}

func parseFence(body, fence string) ([]Row, error) {
	begin, end := "<!--- gbrain:"+fence+":begin -->", "<!--- gbrain:"+fence+":end -->"
	nb, ne := strings.Count(body, begin), strings.Count(body, end)
	if nb == 0 && ne == 0 {
		return nil, nil
	}
	if nb != 1 || ne != 1 || strings.Index(body, begin) > strings.Index(body, end) {
		return nil, fmt.Errorf("%s: requires one complete fence", fence)
	}
	inner := body[strings.Index(body, begin)+len(begin) : strings.Index(body, end)]
	var header []string
	var rows []Row
	seen := map[int]bool{}
	for _, ln := range strings.Split(inner, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		if !strings.HasPrefix(ln, "|") || !strings.HasSuffix(ln, "|") {
			return nil, fmt.Errorf("%s: non-table content", fence)
		}
		cells := splitCells(ln)
		if header == nil {
			header = cells
			names := map[string]bool{}
			for _, h := range header {
				if h == "" || names[h] {
					return nil, fmt.Errorf("%s: duplicate or empty column", fence)
				}
				names[h] = true
			}
			required := []string{"#", "claim", "kind", "confidence", "visibility", "notability", "valid_from", "valid_until", "source", "context"}
			if fence == "takes" {
				required = []string{"#", "claim", "kind", "who", "weight", "since", "source"}
			}
			for _, h := range required {
				if !names[h] {
					return nil, fmt.Errorf("%s: missing column %s", fence, h)
				}
			}
			continue
		}
		separator := true
		for _, c := range cells {
			if strings.Trim(c, "-: ") != "" || c == "" {
				separator = false
			}
		}
		if separator {
			continue
		}
		if len(cells) != len(header) {
			return nil, fmt.Errorf("%s: row has %d cells, expected %d", fence, len(cells), len(header))
		}
		r := Row{Fence: fence, Cells: map[string]string{}}
		for i, h := range header {
			r.Cells[h] = cells[i]
		}
		n, err := strconv.Atoi(r.Cells["#"])
		if err != nil || n <= 0 || seen[n] {
			return nil, fmt.Errorf("%s: invalid or duplicate row %q", fence, r.Cells["#"])
		}
		seen[n] = true
		r.Number = n
		if strings.TrimSpace(r.Cells["claim"]) == "" {
			return nil, fmt.Errorf("%s #%d: empty claim", fence, n)
		}
		key := "confidence"
		if fence == "takes" {
			key = "weight"
		}
		f, err := strconv.ParseFloat(r.Cells[key], 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f > 1 {
			return nil, fmt.Errorf("%s #%d: %s must be finite in [0,1]", fence, n, key)
		}
		if fence == "facts" && r.Cells["visibility"] != "private" && r.Cells["visibility"] != "world" {
			return nil, fmt.Errorf("facts #%d: invalid visibility", n)
		}
		rows = append(rows, r)
	}
	if header == nil {
		return nil, fmt.Errorf("%s: missing header", fence)
	}
	return rows, nil
}

// gbrain only unescapes backslash-pipe; all other backslashes are literal.
func splitCells(ln string) []string {
	s := ln[1 : len(ln)-1]
	var out []string
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) && s[i+1] == '|' {
			b.WriteByte('|')
			i++
		} else if s[i] == '|' {
			out = append(out, strings.TrimSpace(b.String()))
			b.Reset()
		} else {
			b.WriteByte(s[i])
		}
	}
	return append(out, strings.TrimSpace(b.String()))
}
