package schema

import (
	"strings"
	"testing"
)

const validEntry = `---
id: dec-0001
kind: decision
title: vendor dira at a pinned commit
state: accepted
created: "2026-08-28T00:00:00Z"
alternatives:
  - option: fork and edit dira
    why_not: loses upstream fixes silently
---
Because RFC 0001 section 7.3 says so.
`

const invalidEntry = `---
id: dec-0001
kind: decision
title: no alternatives on an accepted decision
state: accepted
created: "2026-08-28T00:00:00Z"
---
This should fail entry.schema.json's alternatives requirement.
`

const notAnEntry = `just some prose with no frontmatter at all`

func TestValidatorAcceptsAValidEntry(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	if err := v.Validate([]byte(validEntry)); err != nil {
		t.Fatalf("Validate(valid entry): %v", err)
	}
}

func TestValidatorRejectsAnAcceptedDecisionWithNoAlternatives(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	if err := v.Validate([]byte(invalidEntry)); err == nil {
		t.Fatalf("Validate(invalid entry): want an error, got nil")
	}
}

func TestValidatorReportsMissingFrontmatterDistinctly(t *testing.T) {
	v, err := NewValidator()
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	err = v.Validate([]byte(notAnEntry))
	if err == nil {
		t.Fatalf("Validate(no frontmatter): want an error, got nil")
	}
	if !strings.Contains(err.Error(), ErrNoFrontmatter.Error()) {
		t.Fatalf("Validate(no frontmatter) error = %v, want it to wrap ErrNoFrontmatter", err)
	}
}

func TestSplitFrontmatterMatchesEntryBody(t *testing.T) {
	front, body, err := SplitFrontmatter([]byte(validEntry))
	if err != nil {
		t.Fatalf("SplitFrontmatter: %v", err)
	}
	if !strings.Contains(string(front), "id: dec-0001") {
		t.Fatalf("front = %q, want it to contain the id field", front)
	}
	if strings.TrimSpace(string(body)) != "Because RFC 0001 section 7.3 says so." {
		t.Fatalf("body = %q", body)
	}
}
