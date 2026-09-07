package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/internal/conformance"
)

// allConformanceProtocols is the fixed, documented set `serenity protocol
// conformance` knows how to replay -- RFC 0001 section 8's own three wire
// protocols, the same set testdata/conformance/README.md and
// internal/conformance/fixtures_test.go already enumerate.
var allConformanceProtocols = []string{"memory_verbs", "disposition", "direction"}

// newProtocolConformanceCmd wires `serenity protocol conformance`, T4.15:
// replays every case in testdata/conformance's frozen transcripts (T4.13)
// against a live server at --target, reporting pass/fail per case with a
// diff on mismatch.
//
// Disclosed scope, matching testdata/conformance/README.md's own disclosed
// gaps:
//   - disposition/subscribe's SSE mode has no transcript to replay (only
//     the long-poll fallback does) -- disposition_test.go's own
//     TestSubscribeSSEDropAndResumeReplaysExactlyMissedEvents stays SSE's
//     authoritative coverage.
//   - A fixed set of disposition and direction cases (named in
//     httpTranscriptCasesNeedingSeededState below) depend on server-side
//     state -- disposition item/group ids minted by crypto/rand, or
//     direction ledger constraints/questions -- that exists only inside
//     T4.13's own fixture-generator run (see each protocol's own
//     gen_transcripts.go) and can never be reproduced against an arbitrary
//     --target this command doesn't control. A mismatch on one of those
//     specific cases is reported as SKIP, not FAIL: this command has no way
//     to tell "the target genuinely regressed" from "the target was never
//     seeded to match," and reporting the latter as a failure would be a
//     false alarm, not a finding. internal/conformance's own boot-and-seed
//     test suite (go test ./internal/conformance) is the byte-exact
//     authority for every one of those cases, since only a test that
//     controls the server's own process can re-seed matching state.
func newProtocolConformanceCmd() *cobra.Command {
	var target, token, fixtures string
	var protocols []string
	var timeout time.Duration
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "conformance --target <url>",
		Short: "Replay testdata/conformance transcripts against a live server",
		Long: "conformance loads testdata/conformance's frozen transcripts (T4.13)\n" +
			"and replays every case's HTTP/MCP calls against a live server at\n" +
			"--target, comparing each response to the recorded one and reporting\n" +
			"pass, fail, or skip per case with a diff on mismatch. A case skips\n" +
			"instead of failing only when it needs server-side state (item ids,\n" +
			"ledger entries) this command cannot seed on an arbitrary target --\n" +
			"see docs/operator/conformance.md's disclosed gaps.\n\n" +
			"Dynamic fields (server-assigned ids, timestamps) are normalized by\n" +
			"shape before comparison, not by name: an opaque hex id or an RFC 3339\n" +
			"timestamp may differ in value as long as both sides have that shape;\n" +
			"anything else must match exactly.\n\n" +
			"--protocol restricts the run to one or more of memory_verbs,\n" +
			"disposition, direction (default: all three). memory_verbs is spoken\n" +
			"over MCP Streamable HTTP at <target>/mcp (RFC 0001 section 14);\n" +
			"disposition and direction are spoken directly against <target> (their\n" +
			"transcripts' own paths already carry the /disposition or /direction\n" +
			"prefix).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if target == "" {
				return fmt.Errorf("conformance: --target is required")
			}
			if token == "" {
				token = os.Getenv("SERENITY_API_TOKEN")
			}
			if fixtures == "" {
				fixtures = conformance.DefaultFixturesDir()
			}
			for _, p := range protocols {
				if !containsString(allConformanceProtocols, p) {
					return fmt.Errorf("conformance: unknown --protocol %q (want one of %s)", p, strings.Join(allConformanceProtocols, ", "))
				}
			}
			httpClient := &http.Client{Timeout: timeout}
			report, err := runConformance(cmd.Context(), httpClient, target, token, fixtures, protocols)
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(report); err != nil {
					return err
				}
			} else {
				printConformanceReport(cmd.OutOrStdout(), report)
			}
			if !report.Passed {
				return fmt.Errorf("conformance: %d case(s) failed against %s", report.Failed, target)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "base URL of a live serenity server, e.g. http://127.0.0.1:8443 (required)")
	cmd.Flags().StringVar(&token, "token", "", "daemon bearer token (defaults to $SERENITY_API_TOKEN)")
	cmd.Flags().StringVar(&fixtures, "fixtures", "", "override testdata/conformance/ directory (default: resolved from this build's own source tree)")
	cmd.Flags().StringSliceVar(&protocols, "protocol", allConformanceProtocols, "protocols to replay (memory_verbs, disposition, direction)")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "per-request HTTP timeout")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable JSON report")
	return cmd
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// conformanceReport is `--json`'s report shape.
type conformanceReport struct {
	Target    string                `json:"target"`
	Protocols []protocolConformance `json:"protocols"`
	Passed    bool                  `json:"passed"`
	Total     int                   `json:"total"`
	Failed    int                   `json:"failed"`
	Skipped   int                   `json:"skipped"`
}

type protocolConformance struct {
	Protocol string            `json:"protocol"`
	Cases    []caseConformance `json:"cases"`
}

// caseStatus is one of "pass", "fail", or "skip" -- see
// httpTranscriptCasesNeedingSeededState for what earns a case "skip".
type caseStatus string

const (
	caseStatusPass caseStatus = "pass"
	caseStatusFail caseStatus = "fail"
	caseStatusSkip caseStatus = "skip"
)

type caseConformance struct {
	Operation string     `json:"operation"`
	Name      string     `json:"name"`
	Status    caseStatus `json:"status"`
	Detail    string     `json:"detail,omitempty"`
}

func runConformance(ctx context.Context, httpClient *http.Client, target, token, fixturesDir string, protocols []string) (conformanceReport, error) {
	report := conformanceReport{Target: target, Passed: true}

	for _, protocol := range protocols {
		var pr protocolConformance
		var err error
		switch protocol {
		case "memory_verbs":
			pr, err = runMemoryVerbsConformance(ctx, httpClient, target, token, fixturesDir)
		case "disposition", "direction":
			pr, err = runHTTPTranscriptConformance(ctx, httpClient, target, token, fixturesDir, protocol)
		}
		if err != nil {
			return conformanceReport{}, fmt.Errorf("conformance: %s: %w", protocol, err)
		}
		report.Protocols = append(report.Protocols, pr)
	}

	for _, pr := range report.Protocols {
		for _, c := range pr.Cases {
			report.Total++
			switch c.Status {
			case caseStatusFail:
				report.Failed++
				report.Passed = false
			case caseStatusSkip:
				report.Skipped++
			}
		}
	}
	return report, nil
}

// httpTranscriptCasesNeedingSeededState names the specific disposition and
// direction fixture cases whose expected response depends on server-side
// state that exists only inside T4.13's own fixture-generator run (see
// testdata/conformance/{disposition,direction}/gen_transcripts.go):
// crypto/rand item/group ids for disposition's list_pending and dispose, and
// literal-id ledger constraints/questions seeded before direction's brief
// and check_plan run. This command has no way to seed an arbitrary --target
// to match either, so a mismatch on one of these cases means "not seeded to
// match," not "the target regressed" -- reported as skip, not fail.
// internal/conformance's own boot-and-seed suite (go test
// ./internal/conformance) re-derives the exact same seed sequences these
// case names reference and is the byte-exact authority for every one of
// them.
var httpTranscriptCasesNeedingSeededState = map[string]map[string]map[string]bool{
	"disposition": {
		"list_pending": {
			"list_pending filters by kind: only the reconcile item is returned":                    true,
			"list_pending with group:true collapses items sharing a group_id into one row":         true,
			"parked items appear only with the parked filter; the default view excludes them":      true,
			"cursor pagination walks all 5 items in pages of 2 and terminates with no next_cursor": true,
		},
		"dispose": {
			"dispose accept records the verdict": true,
			"replaying dispose with the same idempotency_key returns replayed:true, never a second write": true,
			"dispose by group_id disposes every member individually, one result per member":               true,
		},
	},
	"direction": {
		"brief": {
			"brief on a fresh ledger returns all four fixed sections, empty":                                                  true,
			"a zero token_budget returns the minimal valid object: four sections, real candidates all omitted":                true,
			"budget 800 fills sections by priority and drops an overflowing precepts section whole, never starving questions": true,
		},
		"check_plan": {
			"check_plan with a structured action matching an active constraint returns status violated": true,
		},
	},
}

func needsSeededState(protocol, operation, name string) bool {
	return httpTranscriptCasesNeedingSeededState[protocol][operation][name]
}

func seededStateSkipMessage(protocol, operation string) string {
	return fmt.Sprintf("skip: this %s/%s case needs server-side state (item/group ids or ledger entries) "+
		"seeded by T4.13's own fixture generator, which this command cannot reproduce against an "+
		"arbitrary --target; go test ./internal/conformance is the byte-exact authority for it", protocol, operation)
}

func runHTTPTranscriptConformance(ctx context.Context, httpClient *http.Client, target, token, fixturesDir, protocol string) (protocolConformance, error) {
	transcripts, err := conformance.LoadTranscripts(filepath.Join(fixturesDir, protocol))
	if err != nil {
		return protocolConformance{}, err
	}
	pr := protocolConformance{Protocol: protocol}
	for _, tr := range transcripts {
		out := conformance.ReplayTranscript(ctx, httpClient, target, token, tr)
		for _, c := range out.Cases {
			status := caseStatusPass
			detail := ""
			if !c.Passed() {
				if needsSeededState(protocol, out.Operation, c.Name) {
					status = caseStatusSkip
					detail = seededStateSkipMessage(protocol, out.Operation)
				} else {
					status = caseStatusFail
					detail = describeCaseOutcome(c)
				}
			}
			pr.Cases = append(pr.Cases, caseConformance{
				Operation: out.Operation,
				Name:      c.Name,
				Status:    status,
				Detail:    detail,
			})
		}
	}
	return pr, nil
}

func describeCaseOutcome(c conformance.CaseOutcome) string {
	var lines []string
	for _, s := range c.Steps {
		if s.Err != nil {
			lines = append(lines, fmt.Sprintf("%s %s: %v", s.Method, s.Path, s.Err))
			continue
		}
		if s.StatusExpected != s.StatusActual {
			lines = append(lines, fmt.Sprintf("%s %s: status expected %d, got %d", s.Method, s.Path, s.StatusExpected, s.StatusActual))
		}
		if len(s.Mismatches) > 0 {
			lines = append(lines, fmt.Sprintf("%s %s:\n%s", s.Method, s.Path, indent(conformance.FormatMismatches(s.Mismatches))))
		}
	}
	return strings.Join(lines, "\n")
}

func runMemoryVerbsConformance(ctx context.Context, httpClient *http.Client, target, token, fixturesDir string) (protocolConformance, error) {
	cases, err := conformance.LoadMemoryVerbCases(filepath.Join(fixturesDir, "memory_verbs", "cases.json"))
	if err != nil {
		return protocolConformance{}, err
	}
	marker, err := conformance.NewConformanceMarker()
	if err != nil {
		return protocolConformance{}, err
	}
	client := conformance.NewMCPClient(httpClient, strings.TrimRight(target, "/")+"/mcp", token)
	if err := client.Initialize(ctx, "serenity-protocol-conformance", Version); err != nil {
		return protocolConformance{}, err
	}

	outcomes := conformance.ReplayMemoryVerbCases(ctx, client, cases, marker)
	pr := protocolConformance{Protocol: "memory_verbs"}
	for _, o := range outcomes {
		status := caseStatusPass
		detail := ""
		if o.Err != nil {
			status = caseStatusFail
			detail = o.Err.Error()
		} else if len(o.Fails) > 0 {
			status = caseStatusFail
			msgs := make([]string, len(o.Fails))
			for i, f := range o.Fails {
				msgs[i] = f.Error()
			}
			detail = strings.Join(msgs, "\n")
		}
		pr.Cases = append(pr.Cases, caseConformance{
			Operation: "verbs",
			Name:      o.Name,
			Status:    status,
			Detail:    detail,
		})
	}
	return pr, nil
}

func indent(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = "    " + l
	}
	return strings.Join(lines, "\n")
}

func printConformanceReport(w io.Writer, report conformanceReport) {
	for _, pr := range report.Protocols {
		_, _ = fmt.Fprintf(w, "%s\n", pr.Protocol)
		for _, c := range pr.Cases {
			_, _ = fmt.Fprintf(w, "  [%s] %s/%s\n", strings.ToUpper(string(c.Status)), c.Operation, c.Name)
			if c.Status != caseStatusPass && c.Detail != "" {
				_, _ = fmt.Fprintf(w, "%s\n", indent(c.Detail))
			}
		}
	}
	passed := report.Total - report.Failed - report.Skipped
	_, _ = fmt.Fprintf(w, "\n%d/%d passed (%d skipped) against %s\n", passed, report.Total, report.Skipped, report.Target)
}
