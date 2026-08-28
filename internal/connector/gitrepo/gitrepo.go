// Package gitrepo is the git-repo crawler connector (RFC 0001 §10.1, plan
// T1.5): it walks a git working tree at the state of HEAD and turns its
// docs/READMEs into RawItems for internal/connector's pipeline.
//
// Two behaviors are load-bearing, not incidental:
//
//   - The brain repo is excluded by default. The brain repo (RFC §7.1's
//     layout) is itself a git repository holding, among other things, the
//     real dira ledger under .dira/entries/ -- crawling it would re-ingest
//     the user's own precepts and claims as if they were new source
//     material. Config.BrainRoot names that path; Poll compares it against
//     the crawled repo's resolved toplevel and returns zero items when they
//     match, unless IncludeBrainRepo opts back in.
//   - No file this connector reads is ever a precept, and nothing it does
//     can write one. A crawled document is data: Poll only reads bytes and
//     ToSource only builds a domain.Source. When a crawled file happens to
//     be, byte for byte, a valid dira ledger entry (frontmatter decodes and
//     validates against internal/dira/ledger.Entry -- ADR 008's schema), the
//     resulting Source carries a "precept_draft_candidate" meta flag so a
//     human can later choose to promote it through the disposition queue.
//     The flag is metadata on a source, not a write under .dira/ -- this
//     package imports no filesystem-write call that could ever target
//     .dira, so a fixture document that instructs "create precept X" in its
//     prose has nothing to act on: it is either not a valid dira entry (the
//     common case -- prose is not frontmatter) and ingests as an ordinary,
//     unflagged source, or it is a well-formed entry and still only earns a
//     metadata flag. Either way .dira/ is untouched (internal/gate's
//     precept-immutability AST gate is the structural backstop; see plan
//     T3.12).
package gitrepo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/connector"
	"github.com/sirerun/serenity/internal/dira/ledger"
	"github.com/sirerun/serenity/internal/domain"
)

// KindGitRepo is the domain.Source.Kind value every item this connector
// produces carries.
const KindGitRepo = "git_repo"

// PreceptDraftCandidateMeta is the RawItem/Source meta key set to "true" on
// a crawled file that decodes as a well-formed dira ledger entry.
const PreceptDraftCandidateMeta = "precept_draft_candidate"

// docExts are the extensions treated as documentation when found under a
// "docs" directory. README files are recognized by name regardless of
// extension (see isReadmeName).
var docExts = map[string]bool{
	".md":       true,
	".mdx":      true,
	".markdown": true,
	".txt":      true,
	".rst":      true,
}

// Config configures one crawl target. One Connector instance crawls exactly
// one repository; the M1 AC's "5 repos" means 5 Connector instances.
type Config struct {
	// RepoRoot is a path inside (or at) the git repository to crawl.
	RepoRoot string
	// BrainRoot is the absolute path to the user's own brain repo. When the
	// crawled repo's resolved toplevel equals this path, Poll excludes it
	// (see the package doc). Leave empty when there is nothing to compare
	// against -- Poll then never excludes on this basis.
	BrainRoot string
	// IncludeBrainRepo opts back into crawling the brain repo. Defaults to
	// false, which is the "exclude by default" the plan's acc line asks for.
	IncludeBrainRepo bool
}

// Connector crawls one git working tree. It implements connector.Connector.
type Connector struct {
	cfg Config
}

var _ connector.Connector = (*Connector)(nil)

// New returns a Connector for cfg. RepoRoot is resolved lazily, on Poll, so
// constructing a Connector never touches the filesystem.
func New(cfg Config) *Connector { return &Connector{cfg: cfg} }

// Name identifies this connector instance for the jobs table. It is derived
// from the configured repo root's base name, so distinct repos crawled in
// the same run get distinct job rows.
func (c *Connector) Name() string {
	return "git-repo:" + filepath.Base(filepath.Clean(c.cfg.RepoRoot))
}

// cursorState is this connector's Cursor shape: the HEAD commit sha last
// seen. Connectors own their cursor's shape (connector.Cursor's doc
// comment); nothing outside this package interprets it.
type cursorState struct {
	Head string `json:"head"`
}

// Poll walks the crawled repo's tree at its current HEAD and returns one
// RawItem per doc-shaped file (see isDoc). Replaying the same cursor against
// an unchanged HEAD returns zero items -- Poll is a no-op until the repo
// moves -- and the underlying content-address dedup in internal/store
// (T1.2) means even a forced re-poll of an unchanged tree adds no new
// sources.
func (c *Connector) Poll(ctx context.Context, cursor connector.Cursor) ([]connector.RawItem, connector.Cursor, error) {
	toplevel, err := c.toplevel(ctx)
	if err != nil {
		return nil, cursor, err
	}

	if !c.cfg.IncludeBrainRepo && c.isBrainRepo(toplevel) {
		return nil, cursor, nil
	}

	head, err := c.headSHA(ctx, toplevel)
	if err != nil {
		return nil, cursor, err
	}

	var st cursorState
	if len(cursor) > 0 {
		if err := json.Unmarshal(cursor, &st); err != nil {
			return nil, cursor, fmt.Errorf("gitrepo: decode cursor: %w", err)
		}
	}
	if head != "" && head == st.Head {
		return nil, cursor, nil // repo has not moved since the last poll
	}

	occurredAt, err := c.headTime(ctx, toplevel, head)
	if err != nil {
		return nil, cursor, err
	}

	paths, err := c.listFiles(ctx, toplevel)
	if err != nil {
		return nil, cursor, err
	}

	repoName := filepath.Base(toplevel)
	var items []connector.RawItem
	for _, rel := range paths {
		if !isDoc(rel) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(toplevel, filepath.FromSlash(rel)))
		if err != nil {
			return nil, cursor, fmt.Errorf("gitrepo: read %s: %w", rel, err)
		}

		meta := map[string]string{
			"repo": repoName,
			"path": rel,
		}
		if isDiraPattern(data) {
			meta[PreceptDraftCandidateMeta] = "true"
		}

		uri := fmt.Sprintf("git-repo://%s/%s", repoName, rel)
		if head != "" {
			uri += "@" + head
		}

		items = append(items, connector.RawItem{
			URI:        uri,
			Kind:       KindGitRepo,
			Bytes:      data,
			OccurredAt: occurredAt,
			Meta:       meta,
		})
	}

	next := cursor
	if head != "" {
		b, err := json.Marshal(cursorState{Head: head})
		if err != nil {
			return nil, cursor, fmt.Errorf("gitrepo: encode cursor: %w", err)
		}
		next = connector.Cursor(b)
	}
	return items, next, nil
}

// ToSource builds the domain.Source shell for one crawled item. It never
// sets SHA256 -- internal/store.SourceStore.Write derives that from the
// bytes it is handed, never from a caller's claim (source.go's doc comment).
func (c *Connector) ToSource(item connector.RawItem) (domain.Source, error) {
	return domain.Source{
		Kind:       item.Kind,
		URI:        item.URI,
		OccurredAt: item.OccurredAt,
		Meta:       item.Meta,
	}, nil
}

// isDiraPattern reports whether data decodes as a well-formed dira ledger
// entry: YAML frontmatter with id/kind/title/state/created matching
// entry.schema.json, validated (ADR 008). This is the connector's only
// signal for "dira-pattern file" (RFC §10.1) -- reusing the vendored
// decoder rather than a hand-rolled heuristic means a candidate is flagged
// if and only if it would actually be a legal dira entry, which is exactly
// what "precept-draft candidate" should mean.
func isDiraPattern(data []byte) bool {
	_, err := ledger.Decode(data)
	return err == nil
}

// isDoc reports whether rel (a repo-relative, forward-slash path) is in
// this connector's scope: RFC §10.1's "docs/READMEs" -- a README file at any
// depth, by name, regardless of extension; or a documentation-extensioned
// file under any directory named "docs".
func isDoc(rel string) bool {
	dir, base := path.Split(rel)
	if isReadmeName(base) {
		return true
	}
	dir = strings.TrimSuffix(dir, "/")
	if dir == "" {
		return false
	}
	for _, seg := range strings.Split(dir, "/") {
		if seg == "docs" {
			return docExts[strings.ToLower(path.Ext(base))]
		}
	}
	return false
}

// isReadmeName reports whether base (a bare filename) is a README, ignoring
// case and any extension: README, README.md, Readme.txt all match.
func isReadmeName(base string) bool {
	name := strings.TrimSuffix(base, path.Ext(base))
	return strings.EqualFold(name, "readme")
}

// isBrainRepo reports whether toplevel (already resolved by git) is the
// configured brain repo. Returns false whenever BrainRoot is unset -- there
// is nothing to exclude against.
func (c *Connector) isBrainRepo(toplevel string) bool {
	if c.cfg.BrainRoot == "" {
		return false
	}
	brain, err := canonical(c.cfg.BrainRoot)
	if err != nil {
		return false
	}
	top, err := canonical(toplevel)
	if err != nil {
		top = toplevel
	}
	return brain == top
}

// canonical resolves p to an absolute, symlink-resolved path for stable
// comparison. Falls back to the absolute (unresolved) path when p does not
// exist or a symlink cannot be resolved, rather than failing outright.
func canonical(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real, nil
	}
	return abs, nil
}

// toplevel resolves Config.RepoRoot to the git working tree's root.
func (c *Connector) toplevel(ctx context.Context) (string, error) {
	out, err := c.git(ctx, c.cfg.RepoRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("gitrepo: %s is not a git repository: %w", c.cfg.RepoRoot, err)
	}
	return strings.TrimSpace(out), nil
}

// headSHA returns dir's current HEAD commit sha, or "" for a repo with no
// commits yet (an unborn branch) -- not treated as an error, since a freshly
// initialized repo is a valid, if empty, crawl target.
func (c *Connector) headSHA(ctx context.Context, dir string) (string, error) {
	out, err := c.git(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(out), nil
}

// headTime returns head's committer timestamp, or the current time for a
// repo with no commits (head == "").
func (c *Connector) headTime(ctx context.Context, dir, head string) (time.Time, error) {
	if head == "" {
		return time.Now().UTC(), nil
	}
	out, err := c.git(ctx, dir, "log", "-1", "--format=%cI", head)
	if err != nil {
		return time.Time{}, fmt.Errorf("gitrepo: read commit time for %s: %w", head, err)
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(out))
	if err != nil {
		return time.Time{}, fmt.Errorf("gitrepo: parse commit time %q: %w", out, err)
	}
	return t.UTC(), nil
}

// listFiles enumerates every path git considers part of the working tree --
// tracked files plus untracked files that are not gitignored -- which is
// what gives .gitignore real semantics (nested files, negation patterns)
// without this package reimplementing gitignore matching. Deliberately
// shells out to git rather than hand-rolling a directory walk plus a
// gitignore parser: git's own matcher is the reference implementation, and
// duplicating it invites exactly the drift a "respect .gitignore" acc line
// is there to catch.
func (c *Connector) listFiles(ctx context.Context, dir string) ([]string, error) {
	out, err := c.git(ctx, dir, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, fmt.Errorf("gitrepo: list files: %w", err)
	}
	var rel []string
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			rel = append(rel, p)
		}
	}
	sort.Strings(rel)
	return rel, nil
}

// git runs one git subcommand rooted at dir and returns its stdout.
func (c *Connector) git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
