package mcp

import "testing"

// FuzzParse is T4.11's "go-fuzz on parsers 30s in CI" acc-line clause,
// applied to this package's own JSON-RPC frame parser -- parse's job is
// to turn an arbitrary byte slice from an untrusted MCP client (RFC §14
// adversary 2) into either a request or a clean protocol-error response,
// never a panic. A panic inside f.Fuzz's callback is itself the "crash"
// this task's acc line checks for -- go test's fuzzing runtime detects it
// natively and saves a reproducing corpus entry under testdata/fuzz/,
// so this target adds no recover() of its own; catching the panic here
// would defeat the point of fuzzing for it.
//
// Seeds cover: well-formed requests/notifications, the malformed shapes
// parse's own branches exist to reject (bad jsonrpc version, missing/
// empty method, a "result" or "error" member on a request, a duplicate
// top-level key, a non-string/non-integer id), non-JSON and non-UTF8
// input, and -- folding in the task's own prose scope ("oversized
// payloads, deep nesting"), since no acc-line clause names a separate
// size/depth-limit feature to build -- a deeply nested params array and
// a large params string.
func FuzzParse(f *testing.F) {
	seeds := []string{
		`{"jsonrpc":"2.0","method":"tools/list","id":1}`,
		`{"jsonrpc":"2.0","method":"tools/call","id":"abc","params":{"name":"x","arguments":{}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{}`,
		`null`,
		`[]`,
		`"just a string"`,
		`{"jsonrpc":"1.0","method":"x"}`,
		`{"jsonrpc":"2.0","method":""}`,
		`{"jsonrpc":"2.0"}`,
		`{"jsonrpc":"2.0","method":"x","id":-0}`,
		`{"jsonrpc":"2.0","method":"x","id":01}`,
		`{"jsonrpc":"2.0","method":"x","id":1.5}`,
		`{"jsonrpc":"2.0","method":"x","id":true}`,
		`{"jsonrpc":"2.0","method":"x","id":null}`,
		`{"jsonrpc":"2.0","method":"x","result":{}}`,
		`{"jsonrpc":"2.0","method":"x","error":{}}`,
		`{"jsonrpc":"2.0","method":"x","id":1,"id":2}`,
		`{"jsonrpc":"2.0","method":"x","params":"not an object"}`,
		`{"jsonrpc":"2.0","method":"x","params":null}`,
		``,
		`not json at all`,
		"\xff\xfe\x00\x01",
		`{"jsonrpc":"2.0","method":"x`, // truncated
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	// Deep nesting: "deep nesting" from the task's own prose scope.
	const depth = 5000
	deep := []byte(`{"jsonrpc":"2.0","method":"x","id":1,"params":`)
	for range depth {
		deep = append(deep, '[')
	}
	for range depth {
		deep = append(deep, ']')
	}
	deep = append(deep, '}')
	f.Add(deep)

	// Oversized payload: "oversized payloads" from the task's own prose
	// scope.
	const blobSize = 1 << 20
	prefix := `{"jsonrpc":"2.0","method":"x","id":1,"params":{"blob":"`
	large := make([]byte, 0, len(prefix)+blobSize+3)
	large = append(large, prefix...)
	for range blobSize {
		large = append(large, 'a')
	}
	large = append(large, '"', '}', '}')
	f.Add(large)

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parse(data)
	})
}
