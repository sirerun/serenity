package gateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

func episodeExportAccount(t *testing.T, g *Gateway, brain string) string {
	t.Helper()
	var account string
	if err := g.Issuer.Store.DB().QueryRowContext(context.Background(), "SELECT account_id FROM brains WHERE id=?", brain).Scan(&account); err != nil {
		t.Fatal(err)
	}
	return account
}
func assertEpisodeExportDenied(t *testing.T, g *Gateway, account, brain string) {
	t.Helper()
	var output bytes.Buffer
	if err := g.Export(context.Background(), account, brain, &output); !errors.Is(err, ErrEpisodeExportUnsupported) || output.Len() != 0 {
		t.Fatalf("legacy episode export: err=%v bytes=%d", err, output.Len())
	}
}

func TestLegacyEpisodeExportRefusesHTTPArchiveExclusionAndRestart(t *testing.T) {
	g, p, brain, token := episodeHTTPSetup(t)
	account := episodeExportAccount(t, g, brain)
	var ordinary bytes.Buffer
	if err := g.Export(context.Background(), account, brain, &ordinary); err != nil || ordinary.Len() == 0 {
		t.Fatalf("ordinary export failed: %v", err)
	}
	req := episodeHTTPFixture(t, brain)
	if code, _, body := episodeHTTPCall(t, g, token, req); code != 200 {
		t.Fatalf("append %d %s", code, body)
	}
	assertEpisodeExportDenied(t, g, account, brain)
	if code, _, body := episodeHTTPCall(t, g, token, episodeExcludeRequest(brain, *req.Mutation)); code != 200 {
		t.Fatalf("exclude %d %s", code, body)
	}
	assertEpisodeExportDenied(t, g, account, brain)
	if err := p.Drop(brain); err != nil {
		t.Fatal(err)
	}
	// Dropping cached runtime/replay state and disabling archive do not establish
	// export qualification. Canonical bytes remain sufficient for refusal.
	g.mu.Lock()
	g.episodeReplay = nil
	g.mu.Unlock()
	g.EpisodeArchiveEnabled = false
	assertEpisodeExportDenied(t, g, account, brain)
}

func TestLegacyEpisodeExportInspectsDeletedHEADAndAllRetainedRefs(t *testing.T) {
	for _, scenario := range []string{"deleted_head", "other_branch", "annotated_tag", "alternative_yaml", "invalid_historical_metadata"} {
		t.Run(scenario, func(t *testing.T) {
			g, p, brain, token := episodeHTTPSetup(t)
			account := episodeExportAccount(t, g, brain)
			root := filepath.Join(p.BrainsRoot(), brain)
			base := string(bytes.TrimSpace(chainGit(t, root, "rev-parse", "HEAD")))
			req := episodeHTTPFixture(t, brain)
			code, res, body := episodeHTTPCall(t, g, token, req)
			if code != 200 || res.Receipt == nil {
				t.Fatalf("append %d %s", code, body)
			}
			if err := p.Drop(brain); err != nil {
				t.Fatal(err)
			}
			if scenario == "alternative_yaml" || scenario == "invalid_historical_metadata" {
				path := filepath.Join(store.NewSourceStore(root).DirFor(res.Receipt.SourceSHA256), "meta.yaml")
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "alternative_yaml" {
					raw = bytes.Replace(raw, []byte("kind: memory_episode"), []byte("kind: 'memory_episode'"), 1)
				} else {
					raw = []byte("kind: [invalid\n")
				}
				if err = os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
				chainGit(t, root, "add", "--", "brain/sources")
				chainGit(t, root, "commit", "-m", "historical metadata fixture")
			}
			switch scenario {
			case "other_branch":
				chainGit(t, root, "branch", "retained-episodes")
				chainGit(t, root, "reset", "--hard", base)
			case "annotated_tag":
				chainGit(t, root, "tag", "-a", "retained-episodes", "-m", "retained episode fixture")
				chainGit(t, root, "reset", "--hard", base)
			default:
				chainGit(t, root, "rm", "-r", "--", "brain/sources")
				chainGit(t, root, "commit", "-m", "remove HEAD sources")
			}
			g.mu.Lock()
			g.episodeReplay = nil
			g.mu.Unlock()
			g.EpisodeArchiveEnabled = false
			assertEpisodeExportDenied(t, g, account, brain)
		})
	}
}

func TestLegacyEpisodeExportRefusesPendingIntentWithoutCanonicalEpisode(t *testing.T) {
	g, _, brain, token, j := episodeReplayHTTPSetup(t)
	account := episodeExportAccount(t, g, brain)
	j.mode = "fail"
	req := episodeHTTPFixture(t, brain)
	if code, _, body := episodeHTTPCall(t, g, token, episodeExcludeRequest(brain, *req.Mutation)); code != 503 {
		t.Fatalf("failed intent %d %s", code, body)
	}
	assertEpisodeExportDenied(t, g, account, brain)
}

func TestLegacyEpisodeExportOtherBrainIntentDoesNotDenyOrdinaryExport(t *testing.T) {
	g, _, _, account, brain, _ := newLifecycleExportFixture(t)
	g.episodeReplay = map[string]map[string]episodeReplayIntent{"foreign-brain": {"pending": {}}}
	var output bytes.Buffer
	if err := g.Export(context.Background(), account, brain, &output); err != nil || output.Len() == 0 {
		t.Fatalf("foreign intent denied ordinary brain: %v", err)
	}
}

func TestLegacyEpisodeExportGitInventoryFailureEmitsNothing(t *testing.T) {
	g, p, _, account, brain, root := newLifecycleExportFixture(t)
	warmLifecycleRuntime(t, context.Background(), p, brain, root)
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := fmt.Sprintf(`#!/bin/sh
for arg in "$@"; do if [ "$arg" = rev-list ]; then exit 17; fi; done
exec %q "$@"
`, realGit)
	if err = os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	assertEpisodeExportDenied(t, g, account, brain)
}

func TestLegacyEpisodeExportOrdinaryIndexOnlyCanonicalSource(t *testing.T) {
	g, p, _, account, brain, root := newLifecycleExportFixture(t)
	warmLifecycleRuntime(t, context.Background(), p, brain, root)
	if err := p.Drop(brain); err != nil {
		t.Fatal(err)
	}
	sources := store.NewSourceStore(root)
	src, err := sources.Write([]byte("independent private source bytes"), domain.Source{Kind: "file", URI: "fixture://index-only", IndexOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	chainGit(t, root, "add", "--all")
	chainGit(t, root, "commit", "-m", "ordinary index-only source")
	bytesPath := filepath.ToSlash(filepath.Join("brain", "sources", src.SHA256[:2], src.SHA256, "bytes"))
	metaPath := filepath.ToSlash(filepath.Join("brain", "sources", src.SHA256[:2], src.SHA256, "meta.yaml"))
	if got := chainGit(t, root, "ls-tree", "--name-only", "HEAD", "--", bytesPath); len(got) != 0 {
		t.Fatal("index-only bytes unexpectedly tracked")
	}
	if got := chainGit(t, root, "ls-tree", "--name-only", "HEAD", "--", metaPath); len(got) == 0 {
		t.Fatal("index-only metadata not tracked")
	}
	var output bytes.Buffer
	if err = g.Export(context.Background(), account, brain, &output); err != nil || output.Len() == 0 {
		t.Fatalf("ordinary index-only source refused: %v", err)
	}
}

func TestLegacyEpisodeExportMissingSourcePairsRemainUnsupported(t *testing.T) {
	for _, scenario := range []string{"missing_bytes", "missing_metadata", "unknown_kind", "reserved_index_only"} {
		t.Run(scenario, func(t *testing.T) {
			g, p, _, account, brain, root := newLifecycleExportFixture(t)
			warmLifecycleRuntime(t, context.Background(), p, brain, root)
			if err := p.Drop(brain); err != nil {
				t.Fatal(err)
			}
			sources := store.NewSourceStore(root)
			kind := "file"
			if scenario == "unknown_kind" {
				kind = "unknown_raw_kind"
			}
			src, err := sources.Write([]byte("ordinary source"), domain.Source{Kind: kind, URI: "fixture://source"})
			if err != nil {
				t.Fatal(err)
			}
			dir := sources.DirFor(src.SHA256)
			switch scenario {
			case "missing_bytes":
				err = os.Remove(filepath.Join(dir, "bytes"))
			case "missing_metadata":
				err = os.Remove(filepath.Join(dir, "meta.yaml"))
			case "reserved_index_only":
				err = os.WriteFile(filepath.Join(dir, "meta.yaml"), []byte("kind: memory_fact\nuri: fixture://source\nindex_only: true\n"), 0600)
				if err == nil {
					err = os.Remove(filepath.Join(dir, "bytes"))
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			chainGit(t, root, "add", "--all")
			chainGit(t, root, "commit", "-m", "unsupported source inventory")
			assertEpisodeExportDenied(t, g, account, brain)
		})
	}
}
