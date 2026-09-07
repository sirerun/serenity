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
//   - list_pending and dispose seed their pending items via an internal
//     Store.Create call the generator makes before its transcript's own
//     steps ever run (see gen_transcripts.go) -- that seeding is not part
//     of the frozen transcript and item ids are crypto/rand, never
//     reproducible. Replayed against a --target that was not independently
//     seeded with matching items, those two operations' happy-path cases
//     will legitimately fail (the target has no such item) -- this command
//     reports that plainly rather than silently skipping it. Internal/
//     conformance's own boot-and-seed test suite (go test
//     ./internal/conformance) remains the byte-exact authority for those
//     two operations, since only a test that controls the server's own
//     process can re-seed matching state.
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
			"pass/fail per case with a diff on mismatch.\n\n" +
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
}

type protocolConformance struct {
	Protocol string            `json:"protocol"`
	Cases    []caseConformance `json:"cases"`
}

type caseConformance struct {
	Operation string `json:"operation"`
	Name      string `json:"name"`
	Passed    bool   `json:"passed"`
	Detail    string `json:"detail,omitempty"`
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
			if !c.Passed {
				report.Failed++
				report.Passed = false
			}
		}
	}
	return report, nil
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
			pr.Cases = append(pr.Cases, caseConformance{
				Operation: out.Operation,
				Name:      c.Name,
				Passed:    c.Passed(),
				Detail:    describeCaseOutcome(c),
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
		detail := ""
		if o.Err != nil {
			detail = o.Err.Error()
		} else if len(o.Fails) > 0 {
			msgs := make([]string, len(o.Fails))
			for i, f := range o.Fails {
				msgs[i] = f.Error()
			}
			detail = strings.Join(msgs, "\n")
		}
		pr.Cases = append(pr.Cases, caseConformance{
			Operation: "verbs",
			Name:      o.Name,
			Passed:    o.Passed(),
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
			status := "PASS"
			if !c.Passed {
				status = "FAIL"
			}
			_, _ = fmt.Fprintf(w, "  [%s] %s/%s\n", status, c.Operation, c.Name)
			if !c.Passed && c.Detail != "" {
				_, _ = fmt.Fprintf(w, "%s\n", indent(c.Detail))
			}
		}
	}
	_, _ = fmt.Fprintf(w, "\n%d/%d passed against %s\n", report.Total-report.Failed, report.Total, report.Target)
}
