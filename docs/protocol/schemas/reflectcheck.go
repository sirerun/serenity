package schemas

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// Check reflects over e.Type and reports every place its real JSON shape
// (as encoding/json would produce it: field name from the json tag,
// required-ness from the absence of omitempty/omitzero, pointer-ness for
// nullability) disagrees with e's own schema file -- in both directions,
// so a schema property nobody's struct emits is caught exactly like a
// struct field nobody's schema declares. An empty result means the
// schema and the live Go type cannot have drifted apart undetected.
func Check(e Entry) ([]string, error) {
	doc, err := rawDoc(e)
	if err != nil {
		return nil, err
	}
	defs, _ := doc["$defs"].(map[string]any)
	return checkNode(e.Object, e.Type, doc, defs), nil
}

var (
	rawMessageType = reflect.TypeOf(json.RawMessage{})
	anyType        = reflect.TypeOf((*any)(nil)).Elem()
	timeType       = reflect.TypeOf(time.Time{})
)

// tagOpts is the subset of a json struct tag's options this checker
// needs. omitempty and omitzero both make a field optional and,
// crucially, both mean a nil/zero pointer is OMITTED rather than
// marshaled as null -- the same presence rule encoding/json itself
// applies (Go 1.24+ added omitzero; this repo is on Go 1.26).
type tagOpts struct{ omitempty, omitzero bool }

func (o tagOpts) optional() bool { return o.omitempty || o.omitzero }

// jsonField parses one struct field's json tag, mirroring
// encoding/json's own precedence: an empty or "-" name falls back to
// (or, for "-", skips) the field.
func jsonField(f reflect.StructField) (name string, opts tagOpts, skip bool) {
	tag := f.Tag.Get("json")
	if tag == "-" {
		return "", tagOpts{}, true
	}
	parts := strings.Split(tag, ",")
	name = parts[0]
	if name == "" {
		name = f.Name
	}
	for _, p := range parts[1:] {
		switch p {
		case "omitempty":
			opts.omitempty = true
		case "omitzero":
			opts.omitzero = true
		}
	}
	return name, opts, false
}

// resolveRef follows one level of $ref: "#/$defs/name" resolves within
// the same document's own defs map; a bare filename (e.g.
// "disposition_item.schema.json") resolves against this package's own
// Registry -- the only two $ref shapes any schema in this directory
// uses. Anything else is reported as an error rather than silently
// skipped.
func resolveRef(node map[string]any, defs map[string]any) (map[string]any, map[string]any, error) {
	ref, ok := node["$ref"].(string)
	if !ok {
		return node, defs, nil
	}
	if rest, ok := strings.CutPrefix(ref, "#/$defs/"); ok {
		target, ok := defs[rest].(map[string]any)
		if !ok {
			return nil, nil, fmt.Errorf("unresolved local $ref %q", ref)
		}
		return target, defs, nil
	}
	for _, e := range Registry {
		if e.File == ref {
			doc, err := rawDoc(e)
			if err != nil {
				return nil, nil, err
			}
			newDefs, _ := doc["$defs"].(map[string]any)
			return doc, newDefs, nil
		}
	}
	return nil, nil, fmt.Errorf("unresolved external $ref %q (no Registry entry has this File)", ref)
}

// declaredType returns node's "type" keyword normalized to a set, so
// both the single-string form ("string") and the nullable-union form
// (["string","null"]) are handled identically.
func declaredType(node map[string]any) map[string]bool {
	out := map[string]bool{}
	switch v := node["type"].(type) {
	case string:
		out[v] = true
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				out[s] = true
			}
		}
	}
	return out
}

func scalarKind(t reflect.Type) string {
	if t == timeType {
		return "string" // encoding/json marshals time.Time via MarshalJSON as an RFC3339 string.
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "integer"
	case reflect.Float32, reflect.Float64:
		return "number"
	default:
		return ""
	}
}

// checkNode is the recursive comparator. path is a human-readable
// breadcrumb for error messages, not a real JSON Pointer.
func checkNode(path string, t reflect.Type, node map[string]any, defs map[string]any) []string {
	node, defs, err := resolveRef(node, defs)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", path, err)}
	}

	types := declaredType(node)
	_, hasProps := node["properties"]
	_, hasItems := node["items"]

	// An opaque catch-all leaf (no type, no properties, no items) is this
	// package's deliberate way of describing a per-kind polymorphic
	// payload (disposition.Item.Payload, DisposeRequest.EditedPayload,
	// ProposeRequest.Payload) -- accept any Go shape there without
	// further structural comparison.
	if len(types) == 0 && !hasProps && !hasItems {
		return nil
	}

	if hasProps || types["object"] {
		if t.Kind() == reflect.Map || t == rawMessageType || t == anyType {
			return nil // opaque object-shaped payload (e.g. map[string]any params), not further constrained
		}
		if t.Kind() != reflect.Struct {
			return []string{fmt.Sprintf("%s: schema declares object but go type is %s", path, t)}
		}
		return checkStruct(path, t, node, defs)
	}

	if hasItems || types["array"] {
		if t.Kind() != reflect.Slice && t.Kind() != reflect.Array {
			return []string{fmt.Sprintf("%s: schema declares array but go type is %s", path, t)}
		}
		items, _ := node["items"].(map[string]any)
		if items == nil {
			return nil
		}
		return checkNode(path+"[]", t.Elem(), items, defs)
	}

	// Scalar leaf.
	if t == rawMessageType || t.Kind() == reflect.Interface {
		return nil
	}
	want := scalarKind(t)
	if want == "" {
		return nil // an opaque/unmodeled Go type; nothing useful to compare
	}
	if !types[want] {
		return []string{fmt.Sprintf("%s: schema type %v does not include go kind %q (%s)", path, node["type"], want, t)}
	}
	return nil
}

// checkStruct compares every exported, non-"-" field of t against node's
// "properties"/"required", bidirectionally.
func checkStruct(path string, t reflect.Type, node map[string]any, defs map[string]any) []string {
	var errs []string
	props, _ := node["properties"].(map[string]any)
	requiredList, _ := node["required"].([]any)
	required := map[string]bool{}
	for _, r := range requiredList {
		if s, ok := r.(string); ok {
			required[s] = true
		}
	}

	seenGoFields := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue // unexported: encoding/json never touches it
		}
		name, opts, skip := jsonField(f)
		if skip {
			continue
		}
		seenGoFields[name] = true

		propNode, hasProp := props[name].(map[string]any)
		if !hasProp {
			errs = append(errs, fmt.Sprintf("%s: struct field %s (json %q) has no matching schema property", path, f.Name, name))
			continue
		}

		ft := f.Type
		isPtr := ft.Kind() == reflect.Pointer
		if isPtr {
			ft = ft.Elem()
		}

		wantRequired := !opts.optional()
		if wantRequired != required[name] {
			errs = append(errs, fmt.Sprintf("%s.%s: required mismatch (go field always present=%v, schema required=%v)", path, name, wantRequired, required[name]))
		}
		// A required pointer field marshals literal JSON null when nil
		// (no omitempty to suppress it) -- the schema must say so.
		if isPtr && wantRequired && !declaredType(propNode)["null"] {
			errs = append(errs, fmt.Sprintf("%s.%s: go field is a required pointer (nullable) but schema type %v omits \"null\"", path, name, propNode["type"]))
		}

		errs = append(errs, checkNode(path+"."+name, ft, propNode, defs)...)
	}

	for name := range props {
		if !seenGoFields[name] {
			errs = append(errs, fmt.Sprintf("%s: schema property %q has no matching struct field", path, name))
		}
	}
	return errs
}
