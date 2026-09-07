package schemas

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestRegistryMatchesEmbeddedFiles catches the two ways Registry and the
// embedded *.schema.json set can drift apart: a schema file nobody
// registered (silently excluded from every check below) and a Registry
// entry naming a file that was renamed or removed.
func TestRegistryMatchesEmbeddedFiles(t *testing.T) {
	dir, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	onDisk := map[string]bool{}
	for _, d := range dir {
		if strings.HasSuffix(d.Name(), ".schema.json") {
			onDisk[d.Name()] = true
		}
	}
	if len(onDisk) == 0 {
		t.Fatal("no *.schema.json files found -- go:embed pattern or working directory is wrong")
	}
	registered := map[string]bool{}
	for _, e := range Registry {
		registered[e.File] = true
		if !onDisk[e.File] {
			t.Errorf("Registry entry %s/%s names %s, which does not exist on disk", e.Protocol, e.Object, e.File)
		}
	}
	for f := range onDisk {
		if !registered[f] {
			t.Errorf("%s exists on disk but no Registry entry embeds it -- it is invisible to every consistency check in this file", f)
		}
	}
}

// TestNoDuplicateEntries catches a copy-paste Registry entry that
// silently shadows another protocol object.
func TestNoDuplicateEntries(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range Registry {
		key := e.Protocol + "/" + e.Object
		if seen[key] {
			t.Errorf("duplicate Registry entry %s", key)
		}
		seen[key] = true
	}
}

// TestSchemasCarryIDAndProtocolVersion is T4.7's own acc-line clause,
// checked directly against the raw embedded document rather than through
// the compiler, so a missing/wrong $id or protocol_version fails here
// even if the rest of the document would otherwise compile.
func TestSchemasCarryIDAndProtocolVersion(t *testing.T) {
	for _, e := range Registry {
		e := e
		t.Run(e.Protocol+"/"+e.Object, func(t *testing.T) {
			doc, err := rawDoc(e)
			if err != nil {
				t.Fatal(err)
			}
			schemaKeyword, _ := doc["$schema"].(string)
			if schemaKeyword != "https://json-schema.org/draft/2020-12/schema" {
				t.Errorf("$schema = %q, want the draft-2020-12 URI", schemaKeyword)
			}
			id, _ := doc["$id"].(string)
			if id != e.ID() {
				t.Errorf("$id = %q, want %q", id, e.ID())
			}
			pv, ok := doc["protocol_version"].(float64)
			if !ok || pv != 1 {
				t.Errorf("protocol_version = %v, want the integer 1", doc["protocol_version"])
			}
		})
	}
}

// TestSchemasCompileDraft2020 is T4.7's own acc-line clause: a real
// draft-2020-12 validator (santhosh-tekuri/jsonschema/v6, the same
// library T4.20's pinned MEMORY_VERBS contract test already uses) must
// accept every schema, including every cross-file $ref.
func TestSchemasCompileDraft2020(t *testing.T) {
	for _, e := range Registry {
		e := e
		t.Run(e.Protocol+"/"+e.Object, func(t *testing.T) {
			if _, err := Compile(e); err != nil {
				t.Fatalf("compiling %s: %v", e.ID(), err)
			}
		})
	}
}

// TestSchemaMatchesGoStructReflection is T4.7's own acc-line clause: "one
// Go struct per wire object feeds both CLI --json and the server
// (asserted by a reflection test)". Every Entry pairs its schema file
// with the exact exported struct the live handler (and, where one
// exists, the CLI --json path -- check_plan's check.WireResult) marshals;
// this walks that struct's real field/tag set against the schema's
// declared properties/required, in both directions, recursively.
func TestSchemaMatchesGoStructReflection(t *testing.T) {
	for _, e := range Registry {
		e := e
		t.Run(e.Protocol+"/"+e.Object, func(t *testing.T) {
			mismatches, err := Check(e)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range mismatches {
				t.Error(m)
			}
		})
	}
}

// TestReflectionCheckCatchesRealDrift proves TestSchemaMatchesGoStructReflection
// is not a vacuous predicate: run Check against deliberately corrupted
// registry entries (a struct type swapped for one that does not match its
// neighbor's schema) and confirm each one is actually reported, not
// silently accepted. This is the genuine red->green proof the reflection
// check itself does real work, mirroring T4.9's own
// TestDriftSearchMatchesRecallCatchesAOneSidedFieldAddition precedent.
func TestReflectionCheckCatchesRealDrift(t *testing.T) {
	forgetResponseEntry, ok := Lookup("memory_verbs", "forget_response")
	if !ok {
		t.Fatal("memory_verbs/forget_response missing from Registry")
	}
	recallResponseEntry, ok := Lookup("memory_verbs", "recall_response")
	if !ok {
		t.Fatal("memory_verbs/recall_response missing from Registry")
	}

	t.Run("wrong_type_entirely", func(t *testing.T) {
		// forget_response's schema (id/expired/reason/protocol_version)
		// checked against recall_response's Go struct: wildly different
		// shapes, must produce mismatches on both sides.
		bad := forgetResponseEntry
		bad.Type = recallResponseEntry.Type
		mismatches, err := Check(bad)
		if err != nil {
			t.Fatal(err)
		}
		if len(mismatches) == 0 {
			t.Fatal("Check reported no mismatches comparing forget_response's schema against RecallResponse -- the reflection check is vacuous")
		}
	})

	t.Run("struct_with_extra_field_not_in_schema", func(t *testing.T) {
		type extra struct {
			ID              string `json:"id"`
			Expired         bool   `json:"expired"`
			Reason          *string
			ProtocolVersion int    `json:"protocol_version"`
			Surprise        string `json:"surprise_field_never_in_schema"`
		}
		bad := forgetResponseEntry
		bad.Type = reflect.TypeOf(extra{})
		mismatches, err := Check(bad)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, m := range mismatches {
			if strings.Contains(m, "surprise_field_never_in_schema") {
				found = true
			}
		}
		if !found {
			t.Fatalf("Check did not flag a struct field absent from the schema; got: %v", mismatches)
		}
	})

	t.Run("schema_requires_field_go_omits", func(t *testing.T) {
		// The real ForgetResponse.Reason has no omitempty (always
		// present, nullable) -- a hand-rolled copy that adds omitempty
		// must be flagged as a required-ness mismatch.
		type looserReason struct {
			ProtocolVersion int     `json:"protocol_version"`
			ID              string  `json:"id"`
			Expired         bool    `json:"expired"`
			Reason          *string `json:"reason,omitempty"`
		}
		bad := forgetResponseEntry
		bad.Type = reflect.TypeOf(looserReason{})
		mismatches, err := Check(bad)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, m := range mismatches {
			if strings.Contains(m, "reason") && strings.Contains(m, "required mismatch") {
				found = true
			}
		}
		if !found {
			t.Fatalf("Check did not flag reason's required-ness drift; got: %v", mismatches)
		}
	})
}

// TestAllReturnsSortedCopy guards the ordering `serenity protocol --json`
// relies on for a deterministic manifest, and that mutating the result
// cannot corrupt the package-level Registry.
func TestAllReturnsSortedCopy(t *testing.T) {
	all := All()
	if len(all) != len(Registry) {
		t.Fatalf("All() returned %d entries, Registry has %d", len(all), len(Registry))
	}
	if !sort.SliceIsSorted(all, func(i, j int) bool {
		if all[i].Protocol != all[j].Protocol {
			return all[i].Protocol < all[j].Protocol
		}
		return all[i].Object < all[j].Object
	}) {
		t.Fatal("All() is not sorted by protocol then object")
	}
	all[0].Object = "corrupted"
	if Registry[indexOf(t, Registry, all[0].File)].Object == "corrupted" {
		t.Fatal("mutating All()'s result corrupted the package-level Registry -- All() must return a defensive copy")
	}
}

func indexOf(t *testing.T, entries []Entry, file string) int {
	t.Helper()
	for i, e := range entries {
		if e.File == file {
			return i
		}
	}
	t.Fatalf("no entry for file %s", file)
	return -1
}

// TestLookup exercises the "<protocol>/<object>" addressing scheme
// `serenity protocol --json` and a future conformance command both use.
func TestLookup(t *testing.T) {
	e, ok := Lookup("direction", "brief_response")
	if !ok || e.File != "direction_brief_response.schema.json" {
		t.Fatalf("Lookup(direction, brief_response) = %+v, %v", e, ok)
	}
	if _, ok := Lookup("direction", "does_not_exist"); ok {
		t.Fatal("Lookup found an entry that should not exist")
	}
}

// TestSchemaFilesLiveUnderDocsProtocolSchemas is a literal check of the
// acc line's own path requirement -- catches this package ever being
// relocated without its schema files following, since go:embed would
// then embed nothing and every test above would report zero entries only
// if this test did not also run against the real filesystem path.
func TestSchemaFilesLiveUnderDocsProtocolSchemas(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("docs", "protocol", "schemas")
	if !strings.HasSuffix(filepath.ToSlash(wd), filepath.ToSlash(want)) {
		t.Fatalf("package schemas is not rooted at docs/protocol/schemas (cwd: %s)", wd)
	}
}
