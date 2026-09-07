package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/providers"
)

func TestReviewReportRejectsNonbrainWithoutWrites(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-a-brain")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := newRootCmd()
	cmd.SetArgs([]string{"-C", root, "report"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err == nil {
		t.Error("report accepted a directory without serenity.yml")
	}
	if _, err := os.Stat(filepath.Join(root, ".serenity")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("nonbrain report created .serenity or failed inspection: %v", err)
	}
}

func TestReviewReportExportIsStandaloneStdoutJSON(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	if err := runInit(root, io.Discard); err != nil {
		t.Fatal(err)
	}
	reviewExistingReportIndex(t, root)
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetArgs([]string{"-C", root, "report", "--export"})
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("boolean --export failed: %v", err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("export is not exactly one JSON object: %v; output=%q", err, out.String())
	}
	for _, key := range []string{"generated_at", "claims_by_state", "spend", "corrections_per_100_extractions"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("export missing metric %q", key)
		}
	}
}

type reviewReportBrokenWriter struct{}

var errReviewReportWrite = errors.New("review output unavailable")

func (reviewReportBrokenWriter) Write([]byte) (int, error) { return 0, errReviewReportWrite }

func TestReviewReportHumanOutputPropagatesWriteFailure(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	if err := runInit(root, io.Discard); err != nil {
		t.Fatal(err)
	}
	reviewExistingReportIndex(t, root)
	cmd := newRootCmd()
	cmd.SetArgs([]string{"-C", root, "report"})
	cmd.SetOut(reviewReportBrokenWriter{})
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); !errors.Is(err, errReviewReportWrite) {
		t.Fatalf("want wrapped output error, got %v", err)
	}
}

func reviewExistingReportIndex(t *testing.T, root string) {
	t.Helper()
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestReviewReportRequiresIndexWithoutCreatingIt(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	if err := runInit(root, io.Discard); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".serenity", "index.db")
	// init owns its metadata; reporting must not add a derived index.
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fixture index unexpectedly exists: %v", err)
	}
	cmd := newRootCmd()
	cmd.SetArgs([]string{"-C", root, "report", "--export"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err == nil {
		t.Fatal("missing index rendered as healthy empty observations")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("report created index: %v", err)
	}
}

func TestReviewReportDoesNotRecordSizeSamples(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	if err := runInit(root, io.Discard); err != nil {
		t.Fatal(err)
	}
	reviewExistingReportIndex(t, root)
	for range 2 {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"-C", root, "report", "--export"})
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	samples, err := eng.RepoSizeSamples(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 0 {
		t.Fatalf("read-only report recorded %d samples", len(samples))
	}
}

func TestReviewReportExportPropagatesWriteFailure(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	if err := runInit(root, io.Discard); err != nil {
		t.Fatal(err)
	}
	reviewExistingReportIndex(t, root)
	cmd := newRootCmd()
	cmd.SetArgs([]string{"-C", root, "report", "--export"})
	cmd.SetOut(reviewReportBrokenWriter{})
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); !errors.Is(err, errReviewReportWrite) {
		t.Fatalf("export swallowed output failure: %v", err)
	}
}

type reviewReportNetworkTrap struct{ calls int }

func (p *reviewReportNetworkTrap) RoundTrip(*http.Request) (*http.Response, error) {
	p.calls++
	return nil, errors.New("report network trap")
}

func TestReviewReportCLIExportNoNetwork(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	if err := runInit(root, io.Discard); err != nil {
		t.Fatal(err)
	}
	reviewExistingReportIndex(t, root)
	trap := &reviewReportNetworkTrap{}
	old := http.DefaultTransport
	http.DefaultTransport = trap
	t.Cleanup(func() { http.DefaultTransport = old })
	for _, key := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "OPENROUTER_API_KEY"} {
		t.Setenv(key, "")
	}
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetArgs([]string{"-C", root, "report", "--export"})
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(out.Bytes()) || trap.calls != 0 {
		t.Fatalf("report invalid or attempted network: calls=%d", trap.calls)
	}
}
