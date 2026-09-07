package mcp

import (
	"bytes"
	"encoding/json"
	"regexp"
	"unicode/utf8"
)

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}
type request struct {
	id     json.RawMessage
	key    string
	method string
	params json.RawMessage
}

var integerID = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

func idKey(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return "", false
		}
		return "s:" + s, true
	}
	if integerID.Match(raw) {
		if string(raw) == "-0" {
			return "n:0", true
		}
		return "n:" + string(raw), true
	}
	return "", false
}

func failure(id json.RawMessage, code int, message string) response {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return response{JSONRPC: "2.0", ID: id, Error: &rpcError{code, message}}
}
func success(id json.RawMessage, result any) response {
	return response{JSONRPC: "2.0", ID: id, Result: result}
}

func parse(frame []byte) (request, *response) {
	var r request
	if !utf8.Valid(frame) || !json.Valid(frame) {
		e := failure(nil, -32700, "Parse error")
		return r, &e
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(frame, &fields) != nil || fields == nil {
		e := failure(nil, -32600, "Invalid Request")
		return r, &e
	}
	// Duplicate envelope members are ambiguous, especially IDs and methods.
	dec := json.NewDecoder(bytes.NewReader(frame))
	_, _ = dec.Token()
	seen := map[string]bool{}
	for dec.More() {
		token, _ := dec.Token()
		key := token.(string)
		if seen[key] {
			e := failure(nil, -32600, "Invalid Request")
			return r, &e
		}
		seen[key] = true
		var value json.RawMessage
		if dec.Decode(&value) != nil {
			e := failure(nil, -32600, "Invalid Request")
			return r, &e
		}
	}
	var version string
	if json.Unmarshal(fields["jsonrpc"], &version) != nil || version != "2.0" || json.Unmarshal(fields["method"], &r.method) != nil || r.method == "" {
		e := failure(nil, -32600, "Invalid Request")
		return r, &e
	}
	if raw, ok := fields["id"]; ok {
		key, valid := idKey(raw)
		if !valid {
			e := failure(nil, -32600, "Invalid Request")
			return r, &e
		}
		r.id = raw
		r.key = key
	}
	if _, ok := fields["result"]; ok {
		e := failure(nil, -32600, "Invalid Request")
		return r, &e
	}
	if _, ok := fields["error"]; ok {
		e := failure(nil, -32600, "Invalid Request")
		return r, &e
	}
	r.params = fields["params"]
	return r, nil
}

func objectParams(raw json.RawMessage) bool   { return len(raw) > 0 && raw[0] == '{' && json.Valid(raw) }
func optionalObject(raw json.RawMessage) bool { return len(raw) == 0 || objectParams(raw) }
