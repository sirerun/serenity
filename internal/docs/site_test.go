package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocsSiteSources(t *testing.T) {
	want := []string{
		"mkdocs.yml",
		"docs/operator/server.md",
		"docs/operator/conformance.md",
		"docs/protocol/MEMORY_VERBS_v1.md",
		"docs/protocol/DISPOSITION_v1.md",
		"docs/protocol/DIRECTION_v1.md",
		"docs/operator/threat-model.md",
	}
	for _, path := range want {
		if _, err := os.Stat(filepath.Join("../..", path)); err != nil {
			t.Errorf("required docs source %s: %v", path, err)
		}
	}
	b, err := os.ReadFile("../../docs/operator/threat-model.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `--8<-- "threat-model.md"`) {
		t.Fatal("threat model page must include docs/threat-model.md at build time")
	}
}
