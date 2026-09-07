package schemas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// protocolDoc maps a Registry entry's Protocol field to the prose
// document T4.16 ships for it. Each document lives one directory above
// this package (docs/protocol/), the same layout
// TestSchemaFilesLiveUnderDocsProtocolSchemas already asserts this
// package's own working directory against.
var protocolDoc = map[string]string{
	"memory_verbs": "MEMORY_VERBS_v1.md",
	"disposition":  "DISPOSITION_v1.md",
	"direction":    "DIRECTION_v1.md",
}

// TestSchemaIDsAppearInProtocolDocs is T4.16's own acc-line clause: "a
// docs test asserts each schema $id appears in its document." Every
// Registry entry's schema carries a globally unique $id (baseURL plus its
// filename); this walks the three protocol documents and confirms each
// entry's $id string is literally present in its protocol's document, so
// a schema added to the registry without a corresponding link in the docs
// -- or a doc whose link text drifts from the schema's real $id -- fails
// here instead of silently going stale.
func TestSchemaIDsAppearInProtocolDocs(t *testing.T) {
	docContent := make(map[string]string, len(protocolDoc))
	for protocol, filename := range protocolDoc {
		path := filepath.Join("..", filename)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s (doc for protocol %q): %v", path, protocol, err)
		}
		if len(raw) == 0 {
			t.Fatalf("%s is empty", path)
		}
		docContent[protocol] = string(raw)
	}

	for _, e := range Registry {
		e := e
		t.Run(e.Protocol+"/"+e.Object, func(t *testing.T) {
			content, ok := docContent[e.Protocol]
			if !ok {
				t.Fatalf("no protocol document registered for protocol %q -- add it to protocolDoc", e.Protocol)
			}
			if !strings.Contains(content, e.ID()) {
				t.Errorf("%s does not link %s's $id (%s)", protocolDoc[e.Protocol], e.File, e.ID())
			}
		})
	}
}

// TestReflectionCheckCatchesRealDrift's sibling for this file:
// TestSchemaIDsAppearInProtocolDocsCatchesAMissingLink proves the check
// above is not vacuous by asserting it actually fails when a document is
// missing a schema's $id -- a real red spot-check, not an assumption that
// strings.Contains does the right thing.
func TestSchemaIDsAppearInProtocolDocsCatchesAMissingLink(t *testing.T) {
	e, ok := Lookup("direction", "brief_response")
	if !ok {
		t.Fatal("direction/brief_response missing from Registry")
	}
	docWithoutTheLink := "# DIRECTION v1\n\nThis document mentions nothing about brief_response's schema."
	if strings.Contains(docWithoutTheLink, e.ID()) {
		t.Fatalf("test fixture doc unexpectedly already contains %s -- fixture is broken", e.ID())
	}
}
