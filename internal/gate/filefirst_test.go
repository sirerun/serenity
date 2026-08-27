// Package gate is the file-first CI gate (RFC 0001 §7 / plan T0.2): the
// brain repo is canonical and the derived index is a rebuildable cache, so
// no code path may write knowledge into the index (Engine.UpsertEntity,
// UpsertClaim, InsertChunk, UpsertVector) except through the allowlisted
// rebuild path and the writer queue. This test walks internal/ with
// go/ast and fails the build the moment a new caller reaches the index
// without going through a canonical file write first, rather than trusting
// convention to hold.
package gate

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// repoRoot is relative to this package directory (go test's working
// directory), two levels up from internal/gate to the repo root.
const repoRoot = "../.."

// writeCalls are the Engine methods that mutate the derived index (RFC
// §7.5 BrainIndex). A call to one of these from outside the allowlist is a
// file-first violation.
var writeCalls = map[string]bool{
	"UpsertEntity": true,
	"UpsertClaim":  true,
	"InsertChunk":  true,
	"UpsertVector": true,
}

// allowlist holds repo-root-relative, forward-slash paths permitted to
// call the write methods above: either an exact file, or a directory
// prefix ending in "/".
var allowlist = []string{
	"internal/index/rebuild.go",
	"internal/writer/",
}

// violation is one disallowed write call site.
type violation struct {
	file string // repo-root-relative path
	line int
	call string // method name, e.g. "UpsertClaim"
}

func (v violation) String() string {
	return fmt.Sprintf("%s:%d: disallowed call to %s (not in the file-first allowlist)", v.file, v.line, v.call)
}

// allowed reports whether relPath (repo-root-relative, forward slashes)
// is covered by the allowlist.
func allowed(relPath string) bool {
	for _, a := range allowlist {
		if strings.HasSuffix(a, "/") {
			if strings.HasPrefix(relPath, a) {
				return true
			}
			continue
		}
		if relPath == a {
			return true
		}
	}
	return false
}

// scanForViolations walks root/internal, parses every non-test .go file
// with go/ast, and reports every call to a write method (writeCalls) whose
// calling file is not in the allowlist. root is a repo checkout root:
// violations and allowlist matches are both reported relative to it (e.g.
// "internal/index/rebuild.go").
func scanForViolations(root string) ([]violation, error) {
	start := filepath.Join(root, "internal")
	fset := token.NewFileSet()
	var out []violation

	err := filepath.WalkDir(start, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)

		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !writeCalls[sel.Sel.Name] {
				return true
			}
			if allowed(rel) {
				return true
			}
			out = append(out, violation{
				file: rel,
				line: fset.Position(call.Pos()).Line,
				call: sel.Sel.Name,
			})
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].file != out[j].file {
			return out[i].file < out[j].file
		}
		return out[i].line < out[j].line
	})
	return out, nil
}

// writeFixture writes src to root/relPath, creating parent directories as
// needed, so red-check subtests can exercise scanForViolations against a
// synthetic tree instead of the real repo.
func writeFixture(t *testing.T, root, relPath, src string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// disallowedCallerSrc is a self-contained fixture (no external imports
// needed since the gate only parses, never type-checks) with one call to
// an Engine write method.
const disallowedCallerSrc = `package somepkg

type engine struct{}

func (e *engine) UpsertClaim(x string) error { return nil }

func write(eng *engine) error {
	return eng.UpsertClaim("x")
}
`

func joinViolations(vs []violation) string {
	lines := make([]string, len(vs))
	for i, v := range vs {
		lines[i] = v.String()
	}
	return strings.Join(lines, "\n")
}

func TestFileFirstGate(t *testing.T) {
	t.Run("clean_tree", func(t *testing.T) {
		violations, err := scanForViolations(repoRoot)
		if err != nil {
			t.Fatal(err)
		}
		if len(violations) != 0 {
			t.Fatalf("file-first gate found %d violation(s) on the current tree:\n%s", len(violations), joinViolations(violations))
		}
	})

	t.Run("red_check_disallowed_caller", func(t *testing.T) {
		tmp := t.TempDir()
		writeFixture(t, tmp, "internal/somepkg/writer.go", disallowedCallerSrc)

		violations, err := scanForViolations(tmp)
		if err != nil {
			t.Fatal(err)
		}
		if len(violations) != 1 {
			t.Fatalf("want exactly 1 violation for a disallowed caller, got %d: %s", len(violations), joinViolations(violations))
		}
		if violations[0].call != "UpsertClaim" {
			t.Fatalf("want a violation on UpsertClaim, got %q", violations[0].call)
		}
	})

	t.Run("writer_prefix_allowed", func(t *testing.T) {
		tmp := t.TempDir()
		writeFixture(t, tmp, "internal/writer/apply.go", disallowedCallerSrc)

		violations, err := scanForViolations(tmp)
		if err != nil {
			t.Fatal(err)
		}
		if len(violations) != 0 {
			t.Fatalf("internal/writer/ is allowlisted, want 0 violations, got %d: %s", len(violations), joinViolations(violations))
		}
	})
}
