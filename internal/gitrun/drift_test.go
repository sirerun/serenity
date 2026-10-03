package gitrun_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// driftScan walks and parses the whole module, except the one package that
// owns the Git runner. It deliberately does not follow Go build selection:
// ignored commands and platform-specific sources are part of the source
// contract too.
func driftScan(moduleRoot string) ([]string, int, error) {
	root, err := filepath.Abs(moduleRoot)
	if err != nil {
		return nil, 0, fmt.Errorf("resolve module root: %w", err)
	}
	fset := token.NewFileSet()
	var findings []string
	parsed := 0
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			// Repository metadata and vendored/external dependency trees are not
			// module source. internal/gitrun is the sole source exemption.
			if rel == ".git" || rel == "vendor" || rel == "node_modules" || rel == "internal/gitrun" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
		if err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		}
		parsed++
		imports := map[string]bool{}
		dotImport := false
		for _, spec := range file.Imports {
			imp, err := strconvUnquote(spec.Path.Value)
			if err != nil {
				return fmt.Errorf("parse import in %s: %w", rel, err)
			}
			if imp != "os/exec" {
				continue
			}
			name := "exec"
			if spec.Name != nil {
				name = spec.Name.Name
			}
			if name == "." {
				dotImport = true
			} else if name != "_" {
				imports[name] = true
			}
		}
		// Resolve each candidate from the identifier at that callsite rather
		// than collecting same-name declarations across a function or file.
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			api := ""
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				// With parser object resolution enabled (the default), tests
				// confirm imported package uses remain unresolved and local
				// variable/parameter receivers link to their declaration. This
				// evaluates the binding at this exact call.
				if pkg, ok := sel.X.(*ast.Ident); ok && imports[pkg.Name] && pkg.Obj == nil && (sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext") {
					api = sel.Sel.Name
				}
			} else if id, ok := call.Fun.(*ast.Ident); ok && dotImport && id.Obj == nil && (id.Name == "Command" || id.Name == "CommandContext") {
				api = id.Name
			}
			if api == "" {
				return true
			}
			argIndex := 0
			if api == "CommandContext" {
				argIndex = 1
			}
			if len(call.Args) <= argIndex {
				return true
			}
			if isGitExecutable(call.Args[argIndex]) {
				pos := fset.Position(call.Pos())
				findings = append(findings, fmt.Sprintf("%s:%d", rel, pos.Line))
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, parsed, fmt.Errorf("walk module: %w", err)
	}
	sort.Strings(findings)
	return findings, parsed, nil
}

func strconvUnquote(s string) (string, error) {
	return strconv.Unquote(s)
}

func isGitExecutable(expr ast.Expr) bool {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			break
		}
		expr = paren.X
	}
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	value, err := strconv.Unquote(lit.Value)
	if err != nil {
		return false
	}
	base := filepath.Base(strings.ReplaceAll(filepath.ToSlash(value), "\\", "/"))
	return strings.EqualFold(base, "git") || strings.EqualFold(base, "git.exe")
}

func TestNoRawGitExecAcrossModule(t *testing.T) {
	root, err := findModuleRoot()
	if err != nil {
		t.Fatal(err)
	}
	findings, parsed, err := driftScan(root)
	if err != nil {
		t.Fatal(err)
	}
	if parsed == 0 {
		t.Fatal("whole-module scan parsed no non-test Go files")
	}
	t.Logf("parsed %d non-test Go files across the module", parsed)
	if len(findings) != 0 {
		t.Fatalf("raw Git subprocess calls outside exact internal/gitrun directory (ADR 018):\n  %s", strings.Join(findings, "\n  "))
	}
}

func TestDriftScanAliasesDotImportsAndLiteralForms(t *testing.T) {
	root := t.TempDir()
	writeGo(t, root, "cmd/ignored/main.go", "//go:build ignore\n\npackage main\nimport ex \"os/exec\"\nfunc run(){ ex.CommandContext(nil, `/usr/bin/git`, \"status\") }\n")
	writeGo(t, root, "evals/platform.go", "package evals\nimport . \"os/exec\"\nfunc run(){ Command(\"git.exe\", \"status\") }\n")
	writeGo(t, root, "pkg/safe.go", "package pkg\nimport \"os/exec\"\nfunc run(){ exec.Command(\"git-wrapper\") }\n")
	writeGo(t, root, "internal/gitrun/allowed.go", "package gitrun\nimport \"os/exec\"\nfunc run(){ exec.Command(\"git\") }\n")
	writeGo(t, root, "internal/gitrunner/sibling.go", "package gitrunner\nimport ex \"os/exec\"\nfunc run(){ ex.Command(\"git\") }\n")
	findings, parsed, err := driftScan(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"cmd/ignored/main.go:5", "evals/platform.go:3", "internal/gitrunner/sibling.go:3"}
	if parsed != 4 {
		t.Fatalf("parsed %d Go files, want 4 (exact internal/gitrun subtree is exempt)", parsed)
	}
	if strings.Join(findings, "\n") != strings.Join(want, "\n") {
		t.Fatalf("findings = %q, want %q", findings, want)
	}
	findings2, _, err := driftScan(root)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(findings2, "\n") != strings.Join(findings, "\n") {
		t.Fatalf("findings changed across identical scans: %q then %q", findings, findings2)
	}
}

func TestDriftScanHonorsLexicalShadowsAndRefusesErrors(t *testing.T) {
	root := t.TempDir()
	writeGo(t, root, "pkg/shadow.go", "package pkg\nimport ex \"os/exec\"\nimport . \"os/exec\"\nfunc local(ex struct{ Command func(string) }, Command func(string)){ ex.Command(\"git\"); Command(\"git\") }\nfunc safe(){ ex.Command(\"git-wrapper\") }\n")
	findings, _, err := driftScan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("lexically shadowed or non-Git calls reported: %v", findings)
	}
	writeGo(t, root, "pkg/bad.go", "package pkg\nfunc broken( {\n")
	if _, _, err := driftScan(root); err == nil || !strings.Contains(err.Error(), "parse pkg/bad.go") {
		t.Fatalf("parse error = %v, want named refusal", err)
	}
	if _, _, err := driftScan(filepath.Join(root, "missing")); err == nil || !strings.Contains(err.Error(), "walk module") {
		t.Fatalf("walk error = %v, want refusal", err)
	}
}

func TestDriftScanShadowsOnlyAtCallsite(t *testing.T) {
	root := t.TempDir()
	writeGo(t, root, "pkg/callsite.go", `package pkg
import ex "os/exec"
import . "os/exec"
type localExec struct{}
func laterAlias() {
	ex.Command("git")
	ex := localExec{}
	_ = ex
}
func innerAlias() {
	{
		ex := localExec{}
		ex.Command("git-wrapper")
	}
	ex.Command("git")
}
func otherAlias() {
	ex.Command("git")
}
func laterDot() {
	Command("git")
	Command := func(string) {}
	_ = Command
}
func innerDot() {
	{
		Command := func(string) {}
		Command("git-wrapper")
	}
	Command("git")
}
func otherDot() {
	Command("git")
}
`)
	findings, _, err := driftScan(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"pkg/callsite.go:15", "pkg/callsite.go:18", "pkg/callsite.go:21", "pkg/callsite.go:30", "pkg/callsite.go:33", "pkg/callsite.go:6"}
	if strings.Join(findings, "\n") != strings.Join(want, "\n") {
		t.Fatalf("findings = %q, want %q", findings, want)
	}
}

func TestDriftScanLiteralExecutableParenthesesAndWindowsPath(t *testing.T) {
	root := t.TempDir()
	writeGo(t, root, "cmd/windows.go", "//go:build windows\n\npackage windows\nimport ex \"os/exec\"\nfunc run(){ ex.Command((`git`)); ex.Command(`C:\\Program Files\\Git\\bin\\git.exe`); ex.Command(`C:\\Program Files\\Git\\bin\\GIT.EXE`) }\n")
	findings, _, err := driftScan(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"cmd/windows.go:5", "cmd/windows.go:5", "cmd/windows.go:5"}
	if strings.Join(findings, "\n") != strings.Join(want, "\n") {
		t.Fatalf("findings = %q, want %q", findings, want)
	}
}

func writeGo(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func findModuleRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", wd)
		}
	}
}
