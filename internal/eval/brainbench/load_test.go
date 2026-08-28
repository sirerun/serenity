package brainbench

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadFixturesAndGoldRoundTrip(t *testing.T) {
	dir := t.TempDir()
	fixturesDir := filepath.Join(dir, "fixtures")
	goldDir := filepath.Join(dir, "gold")
	if err := os.MkdirAll(fixturesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(goldDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(fixturesDir, "b.fixture.json"), `{
		"schema_version": 1, "fixture_id": "b", "suites": ["push"],
		"seed_pages": [{"slug": "s", "content": "c"}],
		"turns": [{"turn_id": 1, "role": "user", "text": "hi"}]
	}`)
	writeFile(t, filepath.Join(fixturesDir, "a.fixture.json"), `{
		"schema_version": 1, "fixture_id": "a", "suites": ["know-to-ask"],
		"turns": [{"turn_id": 1, "role": "user", "text": "hi"}]
	}`)
	writeFile(t, filepath.Join(goldDir, "a.gold.json"), `{
		"fixture_id": "a", "turns": {"1": {"should_retrieve": false}}
	}`)
	writeFile(t, filepath.Join(goldDir, "b.gold.json"), `{
		"fixture_id": "b", "turns": {"1": {"should_retrieve": true, "gold_slugs": ["s"]}}
	}`)

	fixtures, err := LoadFixtures(fixturesDir)
	if err != nil {
		t.Fatalf("LoadFixtures: %v", err)
	}
	if len(fixtures) != 2 {
		t.Fatalf("got %d fixtures, want 2", len(fixtures))
	}
	// Filename-sorted: a.fixture.json before b.fixture.json.
	if fixtures[0].FixtureID != "a" || fixtures[1].FixtureID != "b" {
		t.Fatalf("fixture order = [%s, %s], want [a, b]", fixtures[0].FixtureID, fixtures[1].FixtureID)
	}

	gold, err := LoadGold(goldDir)
	if err != nil {
		t.Fatalf("LoadGold: %v", err)
	}
	if len(gold) != 2 {
		t.Fatalf("got %d gold records, want 2", len(gold))
	}
	if gt := gold["b"].Turns["1"]; !gt.ShouldRetrieve || len(gt.GoldSlugs) != 1 || gt.GoldSlugs[0] != "s" {
		t.Fatalf("gold[b].Turns[1] = %+v, want should_retrieve:true gold_slugs:[s]", gt)
	}
}

func TestLoadFixturesMissingDirReturnsEmptyNotError(t *testing.T) {
	fixtures, err := LoadFixtures(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("LoadFixtures: %v", err)
	}
	if fixtures == nil || len(fixtures) != 0 {
		t.Fatalf("got %v, want empty non-nil slice", fixtures)
	}
}

func TestLoadFixturesRejectsMissingFixtureID(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bad.fixture.json"), `{"schema_version": 1, "turns": []}`)
	if _, err := LoadFixtures(dir); err == nil {
		t.Fatal("LoadFixtures: want error for a fixture with no fixture_id, got nil")
	}
}

func TestLoadGoldRejectsDuplicateFixtureID(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "one.gold.json"), `{"fixture_id": "dup", "turns": {}}`)
	writeFile(t, filepath.Join(dir, "two.gold.json"), `{"fixture_id": "dup", "turns": {}}`)
	if _, err := LoadGold(dir); err == nil {
		t.Fatal("LoadGold: want error for two files sharing one fixture_id, got nil")
	}
}
