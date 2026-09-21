package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func runCLI(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestCLIPlanPrintsTheFullCardinalitiesAndWhatItDoesNotClaim(t *testing.T) {
	code, out, _ := runCLI("plan", "-workload", frozenWorkload, "-profile", "full")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var got struct {
		Plan       Plan     `json:"plan"`
		NotClaimed []string `json:"not_claimed"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Plan.TotalFacts != 90000 || got.Plan.TotalBrains != 29 || !got.Plan.SatisfiesFullCardinality {
		t.Fatalf("plan %+v", got.Plan)
	}
	if !strings.Contains(strings.Join(got.NotClaimed, " "), "unqualified") {
		t.Fatal("the plan output must say provider quality stays unqualified")
	}
	code, out, _ = runCLI("plan", "-workload", frozenWorkload, "-profile", "smoke")
	if code != 0 || !strings.Contains(out, "REDUCED SMOKE") || !strings.Contains(out, `"satisfies_full_cardinality": false`) {
		t.Fatalf("smoke plan must say REDUCED SMOKE and not satisfy the cardinalities (exit %d):\n%s", code, out)
	}
}

func TestCLIRefusals(t *testing.T) {
	cases := map[string][]string{
		"no command":              {},
		"unknown command":         {"launch"},
		"prepare without flag":    {"prepare", "-out", "/nonexistent-parent/x"},
		"verify without expect":   {"verify", "-dir", "/tmp/x"},
		"verify without dir":      {"verify", "-expect", "full"},
		"plan with a bad profile": {"plan", "-workload", frozenWorkload, "-profile", "huge"},
		"prepare bad endpoint":    {"prepare", "-local-fixture-only", "-out", "/nonexistent-parent/x", "-endpoint", "https://example.com"},
	}
	for name, args := range cases {
		if code, _, _ := runCLI(args...); code != 2 {
			t.Errorf("%s: exit %d, want 2", name, code)
		}
	}
	if _, _, msg := runCLI("prepare", "-out", "/nonexistent-parent/x"); !strings.Contains(msg, "-local-fixture-only") {
		t.Errorf("the refusal must name the required flag, got %q", msg)
	}
}
