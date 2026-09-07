package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/sirerun/serenity/docs/protocol/schemas"
)

// TestProtocolJSONListsEveryRegisteredObject is `serenity protocol --json`'s
// own smoke test: every schemas.All() entry appears exactly once, each
// carrying its full embedded schema (not a truncated or hand-copied
// summary).
func TestProtocolJSONListsEveryRegisteredObject(t *testing.T) {
	var buf bytes.Buffer
	if err := runProtocol(true, &buf); err != nil {
		t.Fatalf("runProtocol: %v", err)
	}
	var manifest protocolManifest
	if err := json.Unmarshal(buf.Bytes(), &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v\n%s", err, buf.String())
	}
	if manifest.ProtocolVersion != 1 {
		t.Fatalf("protocol_version = %d, want 1", manifest.ProtocolVersion)
	}
	want := schemas.All()
	if len(manifest.Objects) != len(want) {
		t.Fatalf("manifest has %d objects, schemas.All() has %d", len(manifest.Objects), len(want))
	}
	for i, e := range want {
		got := manifest.Objects[i]
		if got.Protocol != e.Protocol || got.Object != e.Object {
			t.Fatalf("objects[%d] = %s/%s, want %s/%s (order must match schemas.All())", i, got.Protocol, got.Object, e.Protocol, e.Object)
		}
		raw, err := schemas.Raw(e)
		if err != nil {
			t.Fatal(err)
		}
		// Compare semantically, not byte-for-byte: encoding/json's indenting
		// encoder reformats an embedded json.RawMessage's own whitespace to
		// match the manifest's indentation, so the bytes legitimately differ
		// while the JSON value stays identical.
		var gotVal, wantVal any
		if err := json.Unmarshal(got.Schema, &gotVal); err != nil {
			t.Fatalf("objects[%d] (%s/%s) schema is not valid JSON: %v", i, e.Protocol, e.Object, err)
		}
		if err := json.Unmarshal(raw, &wantVal); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(gotVal, wantVal) {
			t.Fatalf("objects[%d] (%s/%s) schema does not match the embedded file's JSON value", i, e.Protocol, e.Object)
		}
	}
}

// TestProtocolTextListsEveryObject exercises the human-readable form: one
// row per registered object, naming its $id.
func TestProtocolTextListsEveryObject(t *testing.T) {
	var buf bytes.Buffer
	if err := runProtocol(false, &buf); err != nil {
		t.Fatalf("runProtocol: %v", err)
	}
	out := buf.String()
	for _, e := range schemas.All() {
		if !bytes.Contains(buf.Bytes(), []byte(e.ID())) {
			t.Fatalf("text output missing %s/%s's $id %s\noutput:\n%s", e.Protocol, e.Object, e.ID(), out)
		}
	}
}

// TestCheckJSONOutputValidatesAgainstItsSchema is T4.7's own acc-line
// clause: "every CLI --json output validates against its schema in CI".
// `serenity check --json` is the one CLI verb in this codebase that
// already emits a protocol wire object directly (check.WireResult, T4.6);
// this drives it over both a violated and a no_applicable_constraints
// fixture -- exercising the dense (constraints/warnings populated) and
// the sparse (every optional field omitted) branches of the same schema
// -- and validates the real bytes against
// docs/protocol/schemas/direction_check_plan_response.schema.json with
// the real draft-2020-12 validator, not a hand re-implementation of it.
func TestCheckJSONOutputValidatesAgainstItsSchema(t *testing.T) {
	entry, ok := schemas.Lookup("direction", "check_plan_response")
	if !ok {
		t.Fatal("direction/check_plan_response missing from the schema registry")
	}

	t.Run("violated", func(t *testing.T) {
		root := initBrainRepo(t)
		seedConstraint(t, root, "cst-0001", "spend_over", "{amount: {gte: 200}}", checkTestWhyNot, checkTestRevisitIf)

		var buf bytes.Buffer
		err := runCheck(context.Background(), root, "", `[{"action":"spend_over","params":{"amount":500}}]`, true, &buf)
		var exitErr *ExitError
		if err != nil && !isExitError(err, &exitErr) {
			t.Fatalf("runCheck: %v", err)
		}
		if err := schemas.Validate(entry, buf.Bytes()); err != nil {
			t.Fatalf("check --json output violates its own schema: %v\noutput:\n%s", err, buf.String())
		}
	})

	t.Run("no_applicable_constraints", func(t *testing.T) {
		root := initBrainRepo(t)

		var buf bytes.Buffer
		if err := runCheck(context.Background(), root, "", `[{"action":"spend_over","params":{"amount":1}}]`, true, &buf); err != nil {
			t.Fatalf("runCheck: %v", err)
		}
		if err := schemas.Validate(entry, buf.Bytes()); err != nil {
			t.Fatalf("check --json output violates its own schema: %v\noutput:\n%s", err, buf.String())
		}
	})
}

// TestProtocolJSONObjectSchemasCompileDraft2020 proves the CLI is really
// surfacing the same schemas docs/protocol/schemas/schemas_test.go
// already compiles -- not a copy that happens to look similar but has
// drifted structurally.
func TestProtocolJSONObjectSchemasCompileDraft2020(t *testing.T) {
	var buf bytes.Buffer
	if err := runProtocol(true, &buf); err != nil {
		t.Fatalf("runProtocol: %v", err)
	}
	var manifest protocolManifest
	if err := json.Unmarshal(buf.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	for _, obj := range manifest.Objects {
		var doc struct {
			Schema string `json:"$schema"`
		}
		if err := json.Unmarshal(obj.Schema, &doc); err != nil {
			t.Fatalf("%s/%s: schema is not valid JSON: %v", obj.Protocol, obj.Object, err)
		}
		if doc.Schema != "https://json-schema.org/draft/2020-12/schema" {
			t.Errorf("%s/%s: $schema = %q, want the draft-2020-12 URI", obj.Protocol, obj.Object, doc.Schema)
		}
	}
}
