package conformance

import (
	"encoding/json"
	"fmt"
	"os"
)

// Step is one real HTTP request/response pair captured against a live
// DISPOSITION v1 or DIRECTION v1 handler (RFC 0001 section 8.2/8.3).
// Every field is exactly what the wire carried -- these are not
// hand-authored fixtures, they are recorded round trips (see
// testdata/conformance/disposition/gen_transcripts.go and
// testdata/conformance/direction/gen_transcripts.go).
type Step struct {
	// Method is the HTTP method ("POST" or "GET").
	Method string `json:"method"`
	// Path is the request-URI, including any query string (e.g.
	// "/disposition/subscribe?cursor=0").
	Path string `json:"path"`
	// RequestBody is the exact JSON request body sent, nil for a
	// bodyless GET.
	RequestBody json.RawMessage `json:"request_body,omitempty"`
	// NoAuth, when true, means this step was sent with no Authorization
	// header on purpose -- it captures an unauthenticated-rejection
	// case (RFC 0001 section 14: "every endpoint rejects unauthenticated
	// calls"), not a mistake a replayer should "fix" by adding a token.
	NoAuth bool `json:"no_auth,omitempty"`
	// Response is the real response this step's request produced.
	Response StepResponse `json:"response"`
}

// StepResponse is one Step's captured response. Body is the raw response
// text, not json.RawMessage: most responses are JSON (parse Body as JSON
// to inspect them), but an auth-rejection or method-not-allowed step
// captures net/http's own plain-text body (e.g. "unauthorized\n" from
// http.Error) -- a real recorded byte stream must hold either without a
// distinct schema per content type.
type StepResponse struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

// Case is one named conformance scenario: an ordered sequence of Steps
// run against a single live listener, sharing whatever server-side state
// earlier steps produced (e.g. a dispose-replay case's second step reuses
// the first step's idempotency_key against the same item).
type Case struct {
	Name  string `json:"name"`
	Steps []Step `json:"steps"`
}

// Transcript is one JSON fixture file's top-level shape: every case for
// one operation within one protocol.
type Transcript struct {
	Protocol  string `json:"protocol"`  // "disposition" or "direction"
	Operation string `json:"operation"` // e.g. "list_pending", "check_plan"
	Cases     []Case `json:"cases"`
}

// LoadTranscript reads and parses one transcript JSON file.
func LoadTranscript(path string) (Transcript, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Transcript{}, fmt.Errorf("conformance: read transcript %s: %w", path, err)
	}
	var tr Transcript
	if err := json.Unmarshal(b, &tr); err != nil {
		return Transcript{}, fmt.Errorf("conformance: parse transcript %s: %w", path, err)
	}
	return tr, nil
}
