package main

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func mustGuard(t *testing.T, err error, what string) {
	t.Helper()
	if !errors.Is(err, ErrGuard) {
		t.Errorf("%s: want a guardrail refusal, got %v", what, err)
	}
}

func TestOutputDirGuardrails(t *testing.T) {
	root := t.TempDir()
	if _, err := CheckOutputDir(filepath.Join(root, "fresh")); err != nil {
		t.Fatalf("a new directory under an ordinary parent must be accepted: %v", err)
	}
	empty := filepath.Join(root, "empty")
	if err := os.Mkdir(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(root, "service")
	if err := os.MkdirAll(filepath.Join(dataDir, "brains"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "control.db"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(root, "other-fixture")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, MarkerFile), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"empty path":                "",
		"relative path":             "fixtures/x",
		"unclean path":              root + "/a/../b",
		"URL":                       "https://example.com/fixture",
		"existing empty directory":  empty,
		"existing file":             file,
		"existing symlink":          link,
		"missing parent":            filepath.Join(root, "no-such-parent", "x"),
		"service data root":         "/var/lib/serenity/fixture",
		"inside a service data dir": filepath.Join(dataDir, "fixture"),
		"inside another fixture":    filepath.Join(other, "fixture"),
		"inside a Git work tree":    filepath.Join(repo, "fixture"),
		"NUL byte":                  root + "/x\x00y",
	}
	for name, out := range cases {
		_, err := CheckOutputDir(out)
		mustGuard(t, err, name)
	}
}

func TestEndpointGuardrails(t *testing.T) {
	for _, ok := range []string{"", "http://127.0.0.1:8080", "http://127.0.0.1:8080/", "https://127.0.0.1:8443", "http://[::1]:8080", "http://127.5.5.5:1"} {
		if _, err := CheckLoopbackEndpoint(ok); err != nil {
			t.Errorf("%q must be accepted: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"https://example.com", "http://localhost:8080", "http://10.0.0.5:8080", "http://192.168.1.2", "http://0.0.0.0:80", "http://203.0.113.9",
		"ftp://127.0.0.1", "127.0.0.1:8080", "http://user:pw@127.0.0.1", "http://127.0.0.1/mcp", "http://127.0.0.1?x=1", "http://127.0.0.1#f", "http://[2001:db8::1]:80",
	} {
		_, err := CheckLoopbackEndpoint(bad)
		mustGuard(t, err, bad)
	}
}

func TestPrepareRefusesWithoutTheLocalFixtureFlagAndCreatesNothing(t *testing.T) {
	out := filepath.Join(t.TempDir(), "fixture")
	o := Options{Out: out, Workload: frozenWorkload, Profile: ProfileSmoke, SmokeFacts: 1, Dim: 8, CommitEvery: 1, Workers: 1, EntitlementDays: 1}
	_, err := Prepare(context.Background(), o)
	mustGuard(t, err, "missing -local-fixture-only")
	o.LocalFixture, o.Endpoint = true, "https://api.example.com"
	_, err = Prepare(context.Background(), o)
	mustGuard(t, err, "public endpoint")
	if _, err = os.Lstat(out); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a refused run must create nothing, got %v", err)
	}
}

// The tool must not be able to reach a network, read a secret from the environment or run anything but Git.
func TestSourceHasNoNetworkEnvironmentOrForeignExec(t *testing.T) {
	forbiddenImports := map[string]bool{"net": true, "net/http": true, "net/rpc": true, "crypto/tls": true, "net/smtp": true, "plugin": true}
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if forbiddenImports[p] {
				t.Errorf("%s imports %s: the fixture tool has no network code", path, p)
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			switch {
			case pkg.Name == "os" && (sel.Sel.Name == "Getenv" || sel.Sel.Name == "Environ" || sel.Sel.Name == "LookupEnv" || sel.Sel.Name == "ExpandEnv"):
				t.Errorf("%s calls os.%s: the tool reads no environment variable", fset.Position(call.Pos()), sel.Sel.Name)
			case pkg.Name == "exec" && (sel.Sel.Name == "Command" || sel.Sel.Name == "CommandContext"):
				idx := 0
				if sel.Sel.Name == "CommandContext" {
					idx = 1
				}
				lit, ok := call.Args[idx].(*ast.BasicLit)
				if !ok || lit.Value != `"git"` {
					t.Errorf("%s runs a program other than a literal git", fset.Position(call.Pos()))
				}
			}
			return true
		})
	}
}
