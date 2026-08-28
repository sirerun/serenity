package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/direction"
	"github.com/sirerun/serenity/internal/writer"
)

// checkTestWhyNot/checkTestRevisitIf are the exact, verbatim strings a
// violated verdict must reproduce byte-for-byte -- deliberately containing
// punctuation and a quote mark, so a test that passed only because of a
// lossy reformatting (trimming, escaping, summarizing) would fail.
const (
	checkTestWhyNot    = `Unbounded spend risk: "no ceiling" was rejected outright, per the Q3 freeze.`
	checkTestRevisitIf = "quarterly budget review"
)

// seedConstraint writes a real cst-NNNN entry under root/.dira/entries via
// the writer-queue-backed Store (T3.3/T3.5's own fixture path, not a
// synthetic in-memory double) so the built binary reads a genuine ledger
// file exactly as it would in production.
func seedConstraint(t *testing.T, root, id, action, paramsYAML, whyNot, revisitIf string) {
	t.Helper()
	q := writer.NewQueue(nil)
	t.Cleanup(q.Close)
	store := direction.NewStore(root, q)

	body := "Fixture constraint for CLI integration tests.\n\n```serenity:applies_when\naction: " + action + "\n"
	if paramsYAML != "" {
		body += "params: " + paramsYAML + "\n"
	}
	body += "```\n"

	entry := &ledger.Entry{
		ID:      id,
		Kind:    ledger.KindConstraint,
		Title:   "fixture constraint " + id,
		State:   ledger.StateActive,
		Created: "2026-08-28T00:00:00Z",
		Alternatives: []ledger.Alternative{
			{Option: "no ceiling", WhyNot: whyNot, RevisitIf: revisitIf},
		},
		Body: body,
	}
	if err := store.Create(context.Background(), entry); err != nil {
		t.Fatalf("seedConstraint %s: %v", id, err)
	}
}

// initBrainRepo scaffolds a fresh brain repo (serenity.yml, .dira/entries)
// in-process, mirroring TestCompactCLIConfirm's precedent: `init` execed as
// a separate binary would hit the real OS keychain, which this package's
// TestMain mock only covers in-process.
func initBrainRepo(t *testing.T) string {
	t.Helper()
	requireGit(t)
	root := t.TempDir()
	var out bytes.Buffer
	if err := runInit(root, &out); err != nil {
		t.Fatalf("init: %v\n%s", err, out.String())
	}
	return root
}

// --- acc line: CLI integration tests per exit code ---

func TestCheckCLI_PassExitsZero(t *testing.T) {
	bin := buildSerenityBinary(t)
	root := initBrainRepo(t)
	seedConstraint(t, root, "cst-0001", "spend_over", "{amount: {gte: 200}}", checkTestWhyNot, checkTestRevisitIf)

	cmd := exec.Command(bin, "-C", root, "check", "--actions", `[{"action":"spend_over","params":{"amount":100}}]`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("check: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "status: pass") {
		t.Fatalf("expected output to report status: pass, got:\n%s", out)
	}
}

func TestCheckCLI_ViolatedExitsTwoPrintsWhyNotVerbatim(t *testing.T) {
	bin := buildSerenityBinary(t)
	root := initBrainRepo(t)
	seedConstraint(t, root, "cst-0001", "spend_over", "{amount: {gte: 200}}", checkTestWhyNot, checkTestRevisitIf)

	cmd := exec.Command(bin, "-C", root, "check", "--actions", `[{"action":"spend_over","params":{"amount":500}}]`)
	out, err := cmd.CombinedOutput()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected an *exec.ExitError, got %v (output: %s)", err, out)
	}
	if code := exitErr.ExitCode(); code != 2 {
		t.Fatalf("expected exit 2, got %d; output: %s", code, out)
	}
	if !strings.Contains(string(out), "status: violated") {
		t.Fatalf("expected output to report status: violated, got:\n%s", out)
	}
	if !strings.Contains(string(out), checkTestWhyNot) {
		t.Fatalf("expected output to contain why_not verbatim %q, got:\n%s", checkTestWhyNot, out)
	}
	if !strings.Contains(string(out), checkTestRevisitIf) {
		t.Fatalf("expected output to contain revisit_if %q, got:\n%s", checkTestRevisitIf, out)
	}
}

func TestCheckCLI_NoApplicableConstraintsExitsZeroNeverPass(t *testing.T) {
	bin := buildSerenityBinary(t)
	root := initBrainRepo(t)
	seedConstraint(t, root, "cst-0001", "spend_over", "{amount: {gte: 200}}", checkTestWhyNot, checkTestRevisitIf)

	// start_project never matches the only constraint's applies_when action
	// type, so it must report no_applicable_constraints -- literally, never
	// the "pass" string standing in for it.
	cmd := exec.Command(bin, "-C", root, "check", "--actions", `[{"action":"start_project","params":{}}]`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("check: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "no_applicable_constraints") {
		t.Fatalf("expected output to contain the verdict string no_applicable_constraints, got:\n%s", out)
	}
	if strings.Contains(string(out), "status: pass") {
		t.Fatalf("no_applicable_constraints must never be printed as status: pass, got:\n%s", out)
	}
}

func TestCheckCLI_UnverifiedExitsOneWithVerdictInStdout(t *testing.T) {
	bin := buildSerenityBinary(t)
	root := initBrainRepo(t)
	seedConstraint(t, root, "cst-0001", "spend_over", "{amount: {gte: 200}}", checkTestWhyNot, checkTestRevisitIf)

	// Free-text plan input with no classification provider wired (no
	// models.classification pin exists in serenity.yml today) must report
	// unverified, with the verdict itself visible in stdout -- not a bare
	// exit-1 with nothing to show for it.
	cmd := exec.Command(bin, "-C", root, "check", "wire $800 to the new vendor tomorrow")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected an *exec.ExitError, got %v (stdout: %s, stderr: %s)", err, stdout.String(), stderr.String())
	}
	if code := exitErr.ExitCode(); code != 1 {
		t.Fatalf("expected exit 1, got %d; stdout: %s, stderr: %s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "unverified") {
		t.Fatalf("expected the unverified verdict in stdout, got stdout:\n%s", stdout.String())
	}
}

// --- genuine errors are distinct from a computed unverified verdict: both
// exit 1, but a genuine error never claims to have printed a verdict ---

func TestCheckCLI_MalformedActionsJSONIsAGenuineErrorNotAVerdict(t *testing.T) {
	bin := buildSerenityBinary(t)
	root := initBrainRepo(t)

	cmd := exec.Command(bin, "-C", root, "check", "--actions", `not json`)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected an *exec.ExitError, got %v", err)
	}
	if code := exitErr.ExitCode(); code != 1 {
		t.Fatalf("expected exit 1, got %d", code)
	}
	if strings.Contains(stdout.String(), "status:") {
		t.Fatalf("a JSON parse failure must never print a verdict to stdout, got:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "error") {
		t.Fatalf("expected the genuine-error path to report on stderr, got stderr:\n%s", stderr.String())
	}
}

func TestCheckCLI_UnknownActionIsAGenuineError(t *testing.T) {
	bin := buildSerenityBinary(t)
	root := initBrainRepo(t)
	seedConstraint(t, root, "cst-0001", "spend_over", "{amount: {gte: 200}}", checkTestWhyNot, checkTestRevisitIf)

	cmd := exec.Command(bin, "-C", root, "check", "--actions", `[{"action":"launch_the_missiles"}]`)
	out, err := cmd.CombinedOutput()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected an *exec.ExitError, got %v (output: %s)", err, out)
	}
	if code := exitErr.ExitCode(); code != 1 {
		t.Fatalf("expected exit 1, got %d; output: %s", code, out)
	}
}

func TestCheckCLI_BothPlanTextAndActionsRejected(t *testing.T) {
	bin := buildSerenityBinary(t)
	root := initBrainRepo(t)

	cmd := exec.Command(bin, "-C", root, "check", "some plan", "--actions", `[{"action":"spend_over"}]`)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected an error, got none; output: %s", out)
	}
	if !strings.Contains(strings.ToLower(string(out)), "not both") {
		t.Fatalf("expected the error to mention 'not both', got:\n%s", out)
	}
}

func TestCheckCLI_NeitherPlanTextNorActionsRejected(t *testing.T) {
	bin := buildSerenityBinary(t)
	root := initBrainRepo(t)

	cmd := exec.Command(bin, "-C", root, "check")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected an error, got none; output: %s", out)
	}
}

// --- --json machine-readable output ---

func TestCheckCLI_JSONOutputCarriesWhyNotVerbatim(t *testing.T) {
	bin := buildSerenityBinary(t)
	root := initBrainRepo(t)
	seedConstraint(t, root, "cst-0001", "spend_over", "{amount: {gte: 200}}", checkTestWhyNot, checkTestRevisitIf)

	cmd := exec.Command(bin, "-C", root, "check", "--json", "--actions", `[{"action":"spend_over","params":{"amount":500}}]`)
	out, err := cmd.CombinedOutput()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected an *exec.ExitError, got %v (output: %s)", err, out)
	}
	if code := exitErr.ExitCode(); code != 2 {
		t.Fatalf("expected exit 2, got %d; output: %s", code, out)
	}

	var got struct {
		Status          string `json:"status"`
		ConsideredCount int    `json:"considered_count"`
		Constraints     []struct {
			PreceptID string `json:"precept_id"`
			Outcome   string `json:"outcome"`
			WhyNot    string `json:"why_not"`
			RevisitIf string `json:"revisit_if"`
		} `json:"constraints"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("--json output did not parse as JSON: %v\noutput:\n%s", err, out)
	}
	if got.Status != "violated" {
		t.Fatalf("Status = %q, want violated", got.Status)
	}
	if len(got.Constraints) != 1 {
		t.Fatalf("Constraints = %+v, want exactly one", got.Constraints)
	}
	if got.Constraints[0].PreceptID != "cst-0001" {
		t.Fatalf("PreceptID = %q, want cst-0001", got.Constraints[0].PreceptID)
	}
	if got.Constraints[0].WhyNot != checkTestWhyNot {
		t.Fatalf("WhyNot = %q, want verbatim %q", got.Constraints[0].WhyNot, checkTestWhyNot)
	}
	if got.Constraints[0].RevisitIf != checkTestRevisitIf {
		t.Fatalf("RevisitIf = %q, want %q", got.Constraints[0].RevisitIf, checkTestRevisitIf)
	}
}

func TestCheckCLI_JSONOutputNoApplicableConstraints(t *testing.T) {
	bin := buildSerenityBinary(t)
	root := initBrainRepo(t)
	seedConstraint(t, root, "cst-0001", "spend_over", "{amount: {gte: 200}}", checkTestWhyNot, checkTestRevisitIf)

	cmd := exec.Command(bin, "-C", root, "check", "--json", "--actions", `[{"action":"start_project"}]`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("check: %v\n%s", err, out)
	}
	var got struct {
		Status          string `json:"status"`
		ConsideredCount int    `json:"considered_count"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("--json output did not parse as JSON: %v\noutput:\n%s", err, out)
	}
	if got.Status != "no_applicable_constraints" {
		t.Fatalf("Status = %q, want no_applicable_constraints", got.Status)
	}
	if got.ConsideredCount != 1 {
		t.Fatalf("ConsideredCount = %d, want 1", got.ConsideredCount)
	}
}

// --- empty --actions array is a valid (if degenerate) input, not an error ---

func TestCheckCLI_EmptyActionsArrayYieldsNoApplicableConstraints(t *testing.T) {
	bin := buildSerenityBinary(t)
	root := initBrainRepo(t)
	seedConstraint(t, root, "cst-0001", "spend_over", "{amount: {gte: 200}}", checkTestWhyNot, checkTestRevisitIf)

	cmd := exec.Command(bin, "-C", root, "check", "--actions", `[]`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("check: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "no_applicable_constraints") {
		t.Fatalf("expected no_applicable_constraints, got:\n%s", out)
	}
}

// --- not a brain repo: a clear error, not a silent empty-ledger pass ---

func TestCheckCLI_NotABrainRepoErrors(t *testing.T) {
	bin := buildSerenityBinary(t)
	root := t.TempDir() // never initialized -- no serenity.yml

	cmd := exec.Command(bin, "-C", root, "check", "--actions", `[{"action":"spend_over","params":{"amount":500}}]`)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected an error for a non-brain-repo root, got none; output: %s", out)
	}
	if !strings.Contains(strings.ToLower(string(out)), "brain repo") {
		t.Fatalf("expected the error to mention 'brain repo', got:\n%s", out)
	}
}
