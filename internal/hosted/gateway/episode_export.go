package gateway

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/gitrun"
	"github.com/sirerun/serenity/internal/hosted/pool"
	"github.com/sirerun/serenity/internal/store"
	"gopkg.in/yaml.v3"
)

// ErrEpisodeExportUnsupported prevents legacy Git bundles from bypassing the
// unsupported episode portable-export and current-fence reconciliation contract.
var ErrEpisodeExportUnsupported = errors.New("hosted: legacy export unavailable for episode history or unqualified source inventory")

const episodeExportMaxGitBytes = 16 << 20
const episodeExportMaxCommandBytes = 1 << 20
const episodeExportMaxCommits = 256
const episodeExportMaxMetadata = 4096

// episodeExportBuffer bounds Git output before allocation. Repository text is
// never included in errors or logs.
type episodeExportBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *episodeExportBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.buffer.Len() {
		return 0, ErrEpisodeExportUnsupported
	}
	return b.buffer.Write(p)
}

type episodeExportGit struct {
	runner *gitrun.Runner
	used   int
}

func (g *episodeExportGit) read(ctx context.Context, args ...string) ([]byte, error) {
	cmd, err := g.runner.Command(ctx, args...)
	if err != nil {
		return nil, ErrEpisodeExportUnsupported
	}
	out := &episodeExportBuffer{limit: episodeExportMaxCommandBytes}
	stderr := &episodeExportBuffer{limit: 4096}
	cmd.Stdout = out
	cmd.Stderr = stderr
	if err = cmd.Run(); err != nil {
		return nil, ErrEpisodeExportUnsupported
	}
	g.used += out.buffer.Len()
	if g.used > episodeExportMaxGitBytes {
		return nil, ErrEpisodeExportUnsupported
	}
	return out.buffer.Bytes(), nil
}

// checkLegacyEpisodeExport runs under maintenance/account/runtime mutation
// ownership before any export output or bundle creation. Missing qualification,
// history bounds or parse errors refuse disclosure; archive flags cannot bypass.
func (g *Gateway) checkLegacyEpisodeExport(ctx context.Context, brain string, runtime *pool.Runtime) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	g.mu.Lock()
	hasIntent := len(g.episodeReplay[brain]) != 0
	g.mu.Unlock()
	if hasIntent {
		return ErrEpisodeExportUnsupported
	}
	p, err := store.LoadEpisodes(ctx, runtime.Sources)
	if err != nil || len(p.SourceIDs) != 0 || p.BrainID != "" {
		return ErrEpisodeExportUnsupported
	}
	git := &episodeExportGit{runner: gitrun.CanonicalReadOnly(runtime.Root)}
	// Bundle --all also exports non-default branches and tags. Refuse unsupported
	// direct blob/tree refs or nested tags rather than treating them as no history.
	refs, err := git.read(ctx, "for-each-ref", "--format=%(objecttype) %(*objecttype)")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(strings.TrimSpace(string(refs)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 1 && fields[0] == "commit" {
			continue
		}
		if len(fields) == 2 && fields[0] == "tag" && fields[1] == "commit" {
			continue
		}
		return ErrEpisodeExportUnsupported
	}
	history, err := git.read(ctx, "rev-list", "--all", "--max-count=257")
	if err != nil {
		return err
	}
	commits := strings.Fields(string(history))
	if len(commits) == 0 || len(commits) > episodeExportMaxCommits {
		return ErrEpisodeExportUnsupported
	}
	seen := map[string]bool{}
	for _, commit := range commits {
		if !episodeExportGitID(commit) {
			return ErrEpisodeExportUnsupported
		}
		tree, err := git.read(ctx, "ls-tree", "-r", "-z", "--full-tree", commit, "--", "brain/sources")
		if err != nil {
			return err
		}
		entries := bytes.Split(tree, []byte{0})
		if len(entries) == 0 || len(entries[len(entries)-1]) != 0 {
			return ErrEpisodeExportUnsupported
		}
		pairs := map[string]int{}
		for _, entry := range entries[:len(entries)-1] {
			details, path, ok := strings.Cut(string(entry), "\t")
			fields := strings.Fields(details)
			parts := strings.Split(path, "/")
			if !ok || len(fields) != 3 || fields[0] != "100644" || fields[1] != "blob" || !episodeExportGitID(fields[2]) || len(parts) != 5 || parts[0] != "brain" || parts[1] != "sources" || !store.ValidSourceSHA(parts[3]) || parts[2] != parts[3][:2] {
				return ErrEpisodeExportUnsupported
			}
			switch parts[4] {
			case "bytes":
				pairs[parts[3]] |= 1
				continue
			case "meta.yaml":
				pairs[parts[3]] |= 2
			default:
				return ErrEpisodeExportUnsupported
			}
			object := fields[2]
			if seen[object] {
				continue
			}
			if len(seen) >= episodeExportMaxMetadata {
				return ErrEpisodeExportUnsupported
			}
			seen[object] = true
			raw, err := git.read(ctx, "cat-file", "blob", object)
			if err != nil {
				return err
			}
			if len(raw) > 64<<10 {
				return ErrEpisodeExportUnsupported
			}
			var meta struct {
				Kind string `yaml:"kind"`
			}
			decoder := yaml.NewDecoder(bytes.NewReader(raw))
			if decoder.Decode(&meta) != nil {
				return ErrEpisodeExportUnsupported
			}
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				return ErrEpisodeExportUnsupported
			}
			// Parsing every unique historical metadata blob avoids relying on a diff
			// regex, HEAD-only inventory, or the current spelling of generated YAML.
			switch meta.Kind {
			case "file", "email", "voice", "git_repo", store.SourceKindMemoryFact, store.SourceKindMemoryExpiry, store.SourceKindTombstone, store.GovernedMemoryKind:
			default:
				return ErrEpisodeExportUnsupported
			}
		}
		for _, present := range pairs {
			if present != 3 {
				return ErrEpisodeExportUnsupported
			}
		}
	}
	return ctx.Err()
}

func episodeExportGitID(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}
