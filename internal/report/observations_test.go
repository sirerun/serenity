package report

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/index"
)

func TestReportDoesNotExportConnectorIdentifiers(t *testing.T) {
	eng := reviewMetricDB(t)
	for _, name := range []string{"imap:private@example.test", "file:/Users/private/personal"} {
		if _, err := eng.StartJob(context.Background(), name); err != nil {
			t.Fatal(err)
		}
	}
	// Use real present time here so fixture jobs are within the report window.
	rep, err := Build(context.Background(), reviewCanonicalRoot(t), eng, time.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private@example.test", "/Users/", "imap:", "file:"} {
		if strings.Contains(string(data), private) {
			t.Fatalf("private identifier exported: %s", private)
		}
	}
	if len(rep.ConnectorHealth) != 2 || rep.ConnectorHealth[0].Connector != "connector-1" {
		t.Fatalf("missing aggregate connector health: %+v", rep.ConnectorHealth)
	}
}

func TestRepoGrowthRequiresHistoryAndStatesActualInterval(t *testing.T) {
	now := reviewMetricNow
	for _, samples := range [][]index.RepoSizeSample{
		nil, {{At: now, Bytes: 100}}, {{At: now.Add(-time.Second), Bytes: 100}, {At: now, Bytes: 200}},
		{{At: now.Add(-30 * 24 * time.Hour), Bytes: 100}, {At: now.Add(time.Hour), Bytes: 200}},
	} {
		got := repoGrowthSection(samples, now)
		if got.BytesPerMonth != nil {
			t.Fatalf("unobserved monthly change: %+v", got)
		}
	}
	got := repoGrowthSection([]index.RepoSizeSample{{At: now, Bytes: 400}, {At: now.Add(-60 * 24 * time.Hour), Bytes: 100}}, now)
	if got.BytesPerMonth == nil || *got.BytesPerMonth != 150 || got.ObservedDeltaBytes == nil || *got.ObservedDeltaBytes != 300 || got.IntervalStart == nil || got.IntervalEnd == nil {
		t.Fatalf("wrong observed interval: %+v", got)
	}
}

func TestMeasureRepoSizeCountsOnlyCanonicalRegularFiles(t *testing.T) {
	root := t.TempDir()
	reviewWriteFile(t, filepath.Join(root, "brain", "source.txt"), []byte("1234"))
	reviewWriteFile(t, filepath.Join(root, ".dira", "entry.md"), []byte("123"))
	reviewWriteFile(t, filepath.Join(root, ".serenity", "index.db"), []byte("not canonical"))
	reviewWriteFile(t, filepath.Join(root, ".git", "objects", "object"), []byte("not canonical"))
	if err := os.Symlink(filepath.Join(root, ".serenity", "index.db"), filepath.Join(root, "brain", "link")); err != nil {
		t.Fatal(err)
	}
	got, err := MeasureRepoSize(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != 7 {
		t.Fatalf("canonical byte count = %d, want7", got)
	}
}

func TestJobObservationsHandleFutureAndUnresolvedOutages(t *testing.T) {
	now := reviewMetricNow
	jobs := []index.Job{
		{Connector: "c", StartedAt: now.Add(-time.Hour), FinishedAt: now.Add(time.Hour), Status: index.JobSucceeded},
		{Connector: "future", StartedAt: now.Add(time.Hour), Status: index.JobRunning},
	}
	got := observedJobs(jobs, now)
	if len(got) != 1 || got[0].Status != index.JobRunning || !got[0].FinishedAt.IsZero() {
		t.Fatalf("future outcomes included: %+v", got)
	}
	health := connectorHealth([]index.Job{
		{Connector: "c", StartedAt: now.Add(-5 * time.Hour), FinishedAt: now.Add(-4 * time.Hour), Status: index.JobFailed},
		{Connector: "c", StartedAt: now.Add(-3 * time.Hour), FinishedAt: now.Add(-2 * time.Hour), Status: index.JobFailed},
		{Connector: "c", StartedAt: now.Add(-time.Hour), FinishedAt: now, Status: index.JobSucceeded},
	})
	if len(health) != 1 || health[0].CompletedRecoveries != 1 || *health[0].MTTRSeconds != 14400 {
		t.Fatalf("retries counted as multiple outages: %+v", health)
	}
	lag := ingestLag([]index.Job{
		{Connector: "c", Status: index.JobSucceeded, FinishedAt: now.Add(-time.Hour)},
		{Connector: "c", Status: index.JobSucceeded, FinishedAt: now.Add(-time.Minute)},
	}, now)
	if len(lag) != 1 || *lag[0].LagSeconds != 60 {
		t.Fatalf("lag not latest completion: %+v", lag)
	}
}
