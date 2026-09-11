// Package schemas embeds and serves the draft-2020-12 JSON Schemas for
// every wire object in Serenity's three protocols (MEMORY_VERBS v1,
// DISPOSITION v1, DIRECTION v1 -- RFC 0001 section 8, T4.7).
//
// Each schema file carries its own $id and a top-level protocol_version
// metadata field. The Go type paired with it in Registry is not a
// separate description of the wire shape: it is the exact same exported
// struct the live server handler (and, where one exists, the CLI --json
// path) marshals -- internal/server/memory.RecallResponse,
// internal/server/disposition.DisposeRequest,
// internal/direction/check.WireResult, and so on. Pairing the schema with
// that literal reflect.Type, rather than a hand-copied field list, is
// what lets schemas_test.go's reflection check catch a schema drifting
// away from its struct mechanically instead of trusting a human to
// notice.
//
// This package lives beside its own *.schema.json files (rather than
// under internal/) because go:embed cannot reach outside its own
// directory -- the same reason internal/dira/schema is colocated with
// entry.schema.json. It has no "internal" segment in its import path
// because the schemas are meant to be public per RFC 0001's own
// standalone-open-source strategy (section 3): a client of these
// protocols reads them from here, not from documentation prose.
package schemas

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/sirerun/serenity/internal/direction/check"
	coredisp "github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/events"
	serverdirection "github.com/sirerun/serenity/internal/server/direction"
	serverdisposition "github.com/sirerun/serenity/internal/server/disposition"
	"github.com/sirerun/serenity/internal/server/memory"
)

//go:embed *.schema.json
var files embed.FS

// baseURL is the $id prefix every schema file in this directory declares.
// Registering each document under this URL (rather than letting the
// compiler resolve $ref over the network) is what keeps compilation
// offline and deterministic -- a test that needs DNS is a test that fails
// on a plane (the same reasoning internal/dira/schema.schemaURL states).
const baseURL = "https://github.com/sirerun/serenity/docs/protocol/schemas/"

// Entry describes one protocol object: which protocol it belongs to, its
// embedded schema file, and the live Go type whose real JSON shape the
// schema documents.
type Entry struct {
	// Protocol is one of "memory_verbs", "disposition", "direction" --
	// RFC 0001 section 8's own three protocol names.
	Protocol string
	// Object is the schema's short name, e.g. "recall_request".
	Object string
	// File is the embedded filename under this directory.
	File string
	// Type is the exact exported Go struct the live code marshals for
	// this wire object. Never a locally-declared duplicate.
	Type reflect.Type
}

// ID is the $id this entry's schema file declares.
func (e Entry) ID() string { return baseURL + e.File }

// Registry lists every wire object across all three protocols. Order is
// protocol, then declaration order within this file -- stable so
// `serenity protocol` output has a deterministic order, never a map
// iteration order.
var Registry = []Entry{
	{"memory_verbs", "recall_request", "memory_verbs_recall_request.schema.json", reflect.TypeOf(memory.RecallRequest{})},
	{"memory_verbs", "recall_response", "memory_verbs_recall_response.schema.json", reflect.TypeOf(memory.RecallResponse{})},
	{"memory_verbs", "remember_request", "memory_verbs_remember_request.schema.json", reflect.TypeOf(memory.RememberRequest{})},
	{"memory_verbs", "remember_response", "memory_verbs_remember_response.schema.json", reflect.TypeOf(memory.RememberResponse{})},
	{"memory_verbs", "entity_request", "memory_verbs_entity_request.schema.json", reflect.TypeOf(memory.EntityRequest{})},
	{"memory_verbs", "entity_response", "memory_verbs_entity_response.schema.json", reflect.TypeOf(memory.EntityResponse{})},
	{"memory_verbs", "synthesize_request", "memory_verbs_synthesize_request.schema.json", reflect.TypeOf(memory.SynthesizeRequest{})},
	{"memory_verbs", "synthesize_response", "memory_verbs_synthesize_response.schema.json", reflect.TypeOf(memory.SynthesizeResponse{})},
	{"memory_verbs", "cancel_operation_request", "memory_extensions_cancel_operation_request.schema.json", reflect.TypeOf(memory.CancelOperationRequest{})},
	{"memory_verbs", "cancel_operation_response", "memory_extensions_cancel_operation_response.schema.json", reflect.TypeOf(memory.CancelOperationResponse{})},
	{"memory_verbs", "forget_request", "memory_verbs_forget_request.schema.json", reflect.TypeOf(memory.ForgetRequest{})},
	{"memory_verbs", "forget_response", "memory_verbs_forget_response.schema.json", reflect.TypeOf(memory.ForgetResponse{})},
	{"memory_verbs", "read_memory_fact_request", "memory_extensions_read_fact_request.schema.json", reflect.TypeOf(memory.ReadMemoryFactRequest{})},
	{"memory_verbs", "read_memory_fact_response", "memory_extensions_read_fact_response.schema.json", reflect.TypeOf(memory.ReadMemoryFactResponse{})},
	{"memory_verbs", "error", "memory_verbs_error.schema.json", reflect.TypeOf(memory.VerbError{})},

	{"disposition", "item", "disposition_item.schema.json", reflect.TypeOf(coredisp.Item{})},
	{"disposition", "list_pending_request", "disposition_list_pending_request.schema.json", reflect.TypeOf(serverdisposition.ListPendingRequest{})},
	{"disposition", "list_pending_response", "disposition_list_pending_response.schema.json", reflect.TypeOf(serverdisposition.ListPendingResponse{})},
	{"disposition", "dispose_request", "disposition_dispose_request.schema.json", reflect.TypeOf(serverdisposition.DisposeRequest{})},
	{"disposition", "dispose_response", "disposition_dispose_response.schema.json", reflect.TypeOf(serverdisposition.DisposeResponse{})},
	{"disposition", "capture_request", "disposition_capture_request.schema.json", reflect.TypeOf(serverdisposition.CaptureRequest{})},
	{"disposition", "capture_response", "disposition_capture_response.schema.json", reflect.TypeOf(serverdisposition.CaptureResponse{})},
	{"disposition", "event", "disposition_event.schema.json", reflect.TypeOf(events.Event{})},
	{"disposition", "subscribe_longpoll_response", "disposition_subscribe_longpoll_response.schema.json", reflect.TypeOf(serverdisposition.LongPollResponse{})},
	{"disposition", "error", "disposition_error.schema.json", reflect.TypeOf(serverdisposition.ProtoError{})},

	{"direction", "check_plan_request", "direction_check_plan_request.schema.json", reflect.TypeOf(serverdirection.CheckPlanRequest{})},
	{"direction", "check_plan_response", "direction_check_plan_response.schema.json", reflect.TypeOf(check.WireResult{})},
	{"direction", "brief_request", "direction_brief_request.schema.json", reflect.TypeOf(serverdirection.BriefRequest{})},
	{"direction", "brief_response", "direction_brief_response.schema.json", reflect.TypeOf(serverdirection.BriefResponse{})},
	{"direction", "propose_request", "direction_propose_request.schema.json", reflect.TypeOf(serverdirection.ProposeRequest{})},
	{"direction", "propose_response", "direction_propose_response.schema.json", reflect.TypeOf(serverdirection.ProposeResponse{})},
	{"direction", "error", "direction_error.schema.json", reflect.TypeOf(serverdirection.ProtoError{})},
}

// All returns a defensive copy of Registry, ordered by protocol then
// object name -- the order `serenity protocol --json` prints in, stable
// across Go map (Registry has none) or filesystem ordering.
func All() []Entry {
	out := append([]Entry(nil), Registry...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Protocol != out[j].Protocol {
			return out[i].Protocol < out[j].Protocol
		}
		return out[i].Object < out[j].Object
	})
	return out
}

// Lookup finds the entry named "<protocol>/<object>" (e.g.
// "direction/brief_response"), the same key `serenity protocol --json`
// prints and a caller would use to ask for one schema by name.
func Lookup(protocol, object string) (Entry, bool) {
	for _, e := range Registry {
		if e.Protocol == protocol && e.Object == object {
			return e, true
		}
	}
	return Entry{}, false
}

// Raw returns the entry's schema file exactly as embedded.
func Raw(e Entry) ([]byte, error) { return files.ReadFile(e.File) }

// offlineLoader refuses to resolve any $ref this package's own resources
// don't already satisfy -- a $ref that would otherwise silently reach the
// network fails compilation loudly instead (mirrors
// internal/server/memory's pinnedOfflineLoader and
// internal/dira/schema's own registered-$id convention).
type offlineLoader struct{}

func (offlineLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("schemas: refusing to load external resource %s", url)
}

// newCompiler registers every embedded schema as a resource under its own
// $id before compiling anything, so a cross-file $ref (e.g.
// disposition_list_pending_response.schema.json referencing
// disposition_item.schema.json) resolves against this package's own
// embedded set rather than needing network access or a specific compile
// order.
func newCompiler() (*jsonschema.Compiler, error) {
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	c.UseLoader(offlineLoader{})
	for _, e := range Registry {
		raw, err := Raw(e)
		if err != nil {
			return nil, fmt.Errorf("schemas: reading %s: %w", e.File, err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("schemas: parsing %s: %w", e.File, err)
		}
		if err := c.AddResource(e.ID(), doc); err != nil {
			return nil, fmt.Errorf("schemas: registering %s as %s: %w", e.File, e.ID(), err)
		}
	}
	return c, nil
}

// Compile compiles one entry's schema (with every sibling schema
// available for $ref resolution) as a draft-2020-12 schema. Compiling all
// 28 embedded documents costs low-single-digit milliseconds; callers
// validating more than one value should hold the result rather than
// calling Compile per value.
func Compile(e Entry) (*jsonschema.Schema, error) {
	c, err := newCompiler()
	if err != nil {
		return nil, err
	}
	return c.Compile(e.ID())
}

// Validate compiles e's schema and validates data (a JSON document,
// object or array at the top level) against it.
func Validate(e Entry, data []byte) error {
	sch, err := Compile(e)
	if err != nil {
		return err
	}
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("schemas: decoding value for %s: %w", e.ID(), err)
	}
	if err := sch.Validate(value); err != nil {
		return fmt.Errorf("value violates %s: %w", e.ID(), err)
	}
	return nil
}

// rawDoc unmarshals e's embedded file as a generic JSON object -- used by
// the metadata checks (schemas_test.go) that inspect $id/protocol_version
// without going through the schema compiler.
func rawDoc(e Entry) (map[string]any, error) {
	raw, err := Raw(e)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("schemas: %s is not a JSON object: %w", e.File, err)
	}
	return doc, nil
}
