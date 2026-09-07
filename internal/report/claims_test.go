package report

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

func reviewWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalClaimsUseShardHistoryNotDerivedFenceOrIndex(t *testing.T) {
	root := reviewCanonicalRoot(t)
	cfg := config.Default()
	page := store.NewEntityPage(domain.Entity{Slug: "alice", Type: "person"})
	page.Claims = []domain.Claim{
		{ID: "f1", SubjectSlug: "alice", Family: "works_at", Predicate: "works_at", Object: "Acme", State: domain.StateActive},
		// Wrong derived shard fence head MUST NOT be counted.
		{ID: "bogus", SubjectSlug: "alice", Family: "has_balance", Predicate: "has_balance", Object: "999", State: domain.StateActive},
	}
	fw := store.NewFenceWriter(root)
	data, err := fw.RenderEntity(page)
	if err != nil {
		t.Fatal(err)
	}
	reviewWriteFile(t, fw.PathFor("person", "alice"), data)
	lines := []domain.Claim{
		{ID: "old", Family: "has_balance", State: domain.StateActive},
		{ID: "new", Family: "has_balance", State: domain.StateActive, Supersedes: "old"},
		{ID: "gone", Family: "has_balance", State: domain.StateActive},
		{ID: "gone", Family: "has_balance", State: domain.StateRetracted},
		{ID: "gone", Family: "has_balance", State: domain.StateActive}, // merge duplicate cannot undo retract
		{ID: "old", Family: "has_balance", State: domain.StateActive},  // duplicate not double counted
		{ID: "explicit", Family: "has_balance", State: domain.StateSuperseded},
		{ID: "explicit", Family: "has_balance", State: domain.StateActive},
	}
	var raw []byte
	for _, c := range lines {
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		raw = append(raw, b...)
		raw = append(raw, '\n')
	}
	ss := store.NewShardStore(root)
	path := ss.PathFor("alice", "has_balance")
	// Exercise rollover traversal: predecessor in base, later lifecycle lines
	// in a numbered segment. Families also lists that segment filename.
	split := bytes.IndexByte(raw, '\n') + 1
	reviewWriteFile(t, path, raw[:split])
	reviewWriteFile(t, strings.TrimSuffix(path, ".jsonl")+".1.jsonl", raw[split:])
	// Stale shard file for a fence-authoritative family must also be ignored.
	reviewWriteFile(t, ss.PathFor("alice", "works_at"), []byte("malformed stale derived file\n"))
	got, err := canonicalClaimCounts(root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"active": 2, "superseded": 2, "retracted": 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	eng := reviewMetricDB(t)
	if err := eng.UpsertClaim(context.Background(), domain.Claim{ID: "index-only", SubjectSlug: "other", State: domain.StateActive}); err != nil {
		t.Fatal(err)
	}
	rep, err := Build(context.Background(), root, eng, reviewMetricNow)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rep.ClaimsByState, want) {
		t.Fatalf("Build used derived state: %v", rep.ClaimsByState)
	}
}

func TestCanonicalClaimsRejectCorruptShard(t *testing.T) {
	for _, raw := range []string{"bad json\n", `{"id":"broken","state":"unknown"}` + "\n"} {
		t.Run(raw, func(t *testing.T) {
			root := reviewCanonicalRoot(t)
			reviewWriteFile(t, store.NewShardStore(root).PathFor("alice", "has_balance"), []byte(raw))
			if _, err := canonicalClaimCounts(root, config.Default()); err == nil {
				t.Fatal("corrupt canonical claim silently ignored")
			}
		})
	}
}

func TestReportBuildRejectsMissingConfigAndClosedIndex(t *testing.T) {
	eng := reviewMetricDB(t)
	if _, err := Build(context.Background(), t.TempDir(), eng, reviewMetricNow); err == nil {
		t.Fatal("missing config accepted")
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(context.Background(), reviewCanonicalRoot(t), eng, reviewMetricNow); err == nil {
		t.Fatal("closed index accepted")
	}
}
