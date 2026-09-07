package conformance

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
)

// HTTPDoer is the subset of *http.Client this package's HTTP replay needs
// (defined here, the consuming side, per this repo's own interface
// convention -- internal/server/disposition.Registrar does the same for
// its own narrow server dependency). Satisfied by *http.Client directly;
// a caller wanting to observe or mutate traffic (this package's own tests,
// injecting a deliberately-broken transport) can satisfy it with anything
// else that round-trips an *http.Request.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// StepOutcome is one Step's replay result.
type StepOutcome struct {
	Method         string
	Path           string
	StatusExpected int
	StatusActual   int
	Mismatches     []Mismatch
	Err            error // transport-level failure; StatusActual/Mismatches are zero when set
}

// Passed reports whether this step's live response matched the frozen
// transcript, modulo CompareBodies' own dynamic-field normalization.
func (s StepOutcome) Passed() bool {
	return s.Err == nil && s.StatusExpected == s.StatusActual && len(s.Mismatches) == 0
}

// CaseOutcome is one Case's replay result: every one of its Steps, run in
// order against the same live target so later steps see earlier steps'
// side effects, exactly as the case's own recording did.
type CaseOutcome struct {
	Name  string
	Steps []StepOutcome
}

// Passed reports whether every step in this case passed.
func (c CaseOutcome) Passed() bool {
	for _, s := range c.Steps {
		if !s.Passed() {
			return false
		}
	}
	return len(c.Steps) > 0
}

// TranscriptOutcome is one Transcript file's replay result.
type TranscriptOutcome struct {
	Protocol  string
	Operation string
	Cases     []CaseOutcome
}

// ReplayTranscript replays every Case in tr against baseURL (e.g.
// "http://127.0.0.1:PORT") in order, using doer to send each Step's exact
// recorded method/path/request_body and comparing the live response
// against the Step's recorded one via CompareBodies. token is sent as the
// daemon bearer credential (RFC 0001 §14) on every step except one
// recorded with NoAuth -- replaying a NoAuth step WITH a token would stop
// exercising the exact unauthenticated-rejection case the fixture froze.
//
// A step whose live response differs only in dynamic-shaped fields
// (opaque ids, timestamps -- see CompareBodies) still passes: this
// function does not attempt to reproduce a transcript's own pre-existing
// server-side state (e.g. list_pending/dispose's items, created via
// internal Store.Create before the transcript's own steps ever run, per
// testdata/conformance/README.md and each protocol's gen_transcripts.go).
// Replayed against a target that lacks matching seeded state, such a case
// legitimately fails -- that is accurate reporting, not a bug in this
// function -- and internal/conformance's own boot-and-seed test is that
// case's byte-exact authority (see replay_seed_test.go).
func ReplayTranscript(ctx context.Context, doer HTTPDoer, baseURL, token string, tr Transcript) TranscriptOutcome {
	out := TranscriptOutcome{Protocol: tr.Protocol, Operation: tr.Operation}
	for _, c := range tr.Cases {
		out.Cases = append(out.Cases, replayCase(ctx, doer, baseURL, token, c))
	}
	return out
}

func replayCase(ctx context.Context, doer HTTPDoer, baseURL, token string, c Case) CaseOutcome {
	co := CaseOutcome{Name: c.Name}
	for _, step := range c.Steps {
		co.Steps = append(co.Steps, replayStep(ctx, doer, baseURL, token, step))
	}
	return co
}

func replayStep(ctx context.Context, doer HTTPDoer, baseURL, token string, step Step) StepOutcome {
	outcome := StepOutcome{Method: step.Method, Path: step.Path, StatusExpected: step.Response.Status}

	var reader io.Reader
	if len(step.RequestBody) > 0 {
		reader = bytes.NewReader(step.RequestBody)
	}
	req, err := http.NewRequestWithContext(ctx, step.Method, baseURL+step.Path, reader)
	if err != nil {
		outcome.Err = fmt.Errorf("build request: %w", err)
		return outcome
	}
	if !step.NoAuth {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if len(step.RequestBody) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := doer.Do(req)
	if err != nil {
		outcome.Err = fmt.Errorf("%s %s: %w", step.Method, step.Path, err)
		return outcome
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		outcome.Err = fmt.Errorf("%s %s: read response: %w", step.Method, step.Path, err)
		return outcome
	}

	outcome.StatusActual = resp.StatusCode
	outcome.Mismatches = CompareBodies(step.Response.Body, string(body))
	return outcome
}

// MemoryVerbOutcome is one MemoryVerbCase's replay result.
type MemoryVerbOutcome struct {
	Name  string
	Err   error   // interpolation, transport, or JSON-RPC-level failure
	Fails []error // failed assertions (error code, suggestion, Expect[] entries)
}

// Passed reports whether this case ran and every assertion held.
func (o MemoryVerbOutcome) Passed() bool { return o.Err == nil && len(o.Fails) == 0 }

// NewConformanceMarker generates a random per-run marker for
// {{marker}}-templated memory_verbs cases (see InterpolateParams). Unlike
// pinned_cases_test.go's fixed "serenity-pinned-v1" (safe there because
// every test run gets a brand-new, discarded temp store), a CLI replay
// run against a persistent live server must not collide with a previous
// run's own conformance data still sitting in that server's brain repo.
func NewConformanceMarker() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("conformance: generate marker: %w", err)
	}
	return "serenity-conformance-" + hex.EncodeToString(b), nil
}

// ReplayMemoryVerbCases runs every case against client, in order, so a
// later case's {{id:key}} placeholder can resolve against an earlier
// case's SaveAs -- the exact chaining runPinnedCases uses over stdio,
// replayed here over Streamable HTTP instead.
func ReplayMemoryVerbCases(ctx context.Context, client *MCPClient, cases []MemoryVerbCase, marker string) []MemoryVerbOutcome {
	saved := map[string]string{}
	outcomes := make([]MemoryVerbOutcome, 0, len(cases))
	for _, tc := range cases {
		outcomes = append(outcomes, replayMemoryVerbCase(ctx, client, tc, marker, saved))
	}
	return outcomes
}

func replayMemoryVerbCase(ctx context.Context, client *MCPClient, tc MemoryVerbCase, marker string, saved map[string]string) MemoryVerbOutcome {
	o := MemoryVerbOutcome{Name: tc.Name}
	params, err := InterpolateParams(tc.Params, marker, saved)
	if err != nil {
		o.Err = err
		return o
	}
	value, isError, err := client.CallTool(ctx, tc.Verb, params)
	if err != nil {
		o.Err = err
		return o
	}

	wantError := tc.ExpectErrorCode
	if tc.RequiresSynthesizeFlag {
		wantError = "unavailable"
	}
	switch {
	case wantError != "":
		if !isError || value["error"] != wantError {
			o.Fails = append(o.Fails, fmt.Errorf("error=%v isError=%t; want %s", value, isError, wantError))
		} else if suggestion, ok := value["suggestion"].(string); !ok || suggestion == "" {
			o.Fails = append(o.Fails, fmt.Errorf("missing corrective suggestion"))
		}
	case isError:
		o.Fails = append(o.Fails, fmt.Errorf("unexpected tool error: %v", value))
	}

	for _, expect := range tc.Expect {
		if err := AssertExpectation(value, expect); err != nil {
			o.Fails = append(o.Fails, err)
		}
	}

	if tc.SaveAs.Key != "" {
		v, ok := LookupPath(value, tc.SaveAs.Path)
		if !ok {
			o.Fails = append(o.Fails, fmt.Errorf("saveAs %s: path %s missing from response", tc.SaveAs.Key, tc.SaveAs.Path))
			return o
		}
		id, ok := v.(string)
		if !ok || id == "" {
			o.Fails = append(o.Fails, fmt.Errorf("saveAs %s: path %s is not a nonempty string", tc.SaveAs.Key, tc.SaveAs.Path))
			return o
		}
		saved[tc.SaveAs.Key] = id
	}
	return o
}
