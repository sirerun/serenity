package docs

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile("../../" + path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestErasureDisclosureMatchesCheckedInBehavior(t *testing.T) {
	threat := readRepoFile(t, "docs/threat-model.md")
	readme := readRepoFile(t, "README.md")
	operator := readRepoFile(t, "docs/operator/forget.md")

	for name, content := range map[string]string{
		"threat model":   threat,
		"README":         readme,
		"operator guide": operator,
	} {
		for _, want := range []string{
			"30-day",
			"all Git refs",
			"31-day",
		} {
			if !strings.Contains(strings.ToLower(content), strings.ToLower(want)) {
				t.Errorf("%s does not disclose %q", name, want)
			}
		}
	}
	for name, content := range map[string]string{
		"threat model":   threat,
		"README":         readme,
		"operator guide": operator,
	} {
		if !strings.Contains(strings.ToLower(content), "source tombstone") {
			t.Errorf("%s does not distinguish source tombstones from fact forget", name)
		}
	}
	if !strings.Contains(strings.ToLower(threat), "history rewrite") ||
		!strings.Contains(strings.ToLower(readme), "rewrites git history") ||
		!strings.Contains(strings.ToLower(operator), "rewrites the brain repository history") {
		t.Error("all three disclosures must describe the memory-fact history rewrite")
	}

	for _, stale := range []string{
		"rewriting brain history on forget is tracked separately",
		"until it lands, the history limit above applies",
		"a git-canonical brain remembers unless the operator",
	} {
		if strings.Contains(strings.ToLower(threat), strings.ToLower(stale)) {
			t.Errorf("threat model retains stale erasure claim %q", stale)
		}
	}

	// Keep the hosted retention disclosure tied to the checked-in bucket rule.
	var template struct {
		Resources map[string]struct {
			Properties struct {
				LifecycleConfiguration struct {
					Rules []struct {
						ExpirationInDays            int `json:"ExpirationInDays"`
						NoncurrentVersionExpiration struct {
							NoncurrentDays int `json:"NoncurrentDays"`
						} `json:"NoncurrentVersionExpiration"`
					} `json:"Rules"`
				} `json:"LifecycleConfiguration"`
			} `json:"Properties"`
		} `json:"Resources"`
	}
	if err := json.Unmarshal([]byte(readRepoFile(t, "deploy/hosted/stack.json")), &template); err != nil {
		t.Fatalf("parse hosted stack template: %v", err)
	}
	bucket, ok := template.Resources["Backups"]
	if !ok || len(bucket.Properties.LifecycleConfiguration.Rules) == 0 {
		t.Fatal("hosted Backups bucket must have a lifecycle rule")
	}
	rule := bucket.Properties.LifecycleConfiguration.Rules[0]
	if rule.ExpirationInDays != 30 || rule.NoncurrentVersionExpiration.NoncurrentDays != 30 {
		t.Fatalf("hosted retention changed: current=%d days, noncurrent=%d days; review the disclosure", rule.ExpirationInDays, rule.NoncurrentVersionExpiration.NoncurrentDays)
	}
}
