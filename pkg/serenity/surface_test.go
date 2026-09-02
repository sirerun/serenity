package serenity_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// ledgerMutators are direction.Store's write methods (ADR 012 §3). The
// facade may hold a *direction.Store; it may never call one of these.
var ledgerMutators = map[string]bool{
	"Create": true, "Put": true, "Delete": true,
	"CreateDraft": true, "Confirm": true, "Supersede": true, "Answer": true,
}

// TestFacadeNeverImportsWriterOrCallsMutators is ADR 012 §3's AST gate,
// in the family of T0.2's file-first gate and T3.12's precept-immutability
// gate: it parses every non-test file of pkg/serenity and fails on an
// import of internal/writer or a selector call named for a ledger mutator.
// It parses only (no type-check), so it errs on the side of flagging any
// call spelled like a mutator, whatever its receiver.
func TestFacadeNeverImportsWriterOrCallsMutators(t *testing.T) {
	fset := token.NewFileSet()
	des, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, de := range des {
		name := de.Name()
		if de.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		full, err := parser.ParseFile(fset, name, nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range full.Imports {
			if strings.Contains(imp.Path.Value, "/internal/writer") {
				t.Errorf("%s imports %s: the facade must not hold a writer (ADR 012 §3)", name, imp.Path.Value)
			}
		}
		ast.Inspect(full, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if ledgerMutators[sel.Sel.Name] {
				t.Errorf("%s: call to %s at %s: the facade must not call a ledger mutator (ADR 012 §3)",
					name, sel.Sel.Name, fset.Position(call.Pos()))
			}
			return true
		})
	}
}

// TestASTGateCatchesAMutatorCall proves the gate above is not vacuous by
// feeding it a synthetic file that calls Store.Confirm.
func TestASTGateCatchesAMutatorCall(t *testing.T) {
	src := "package x\nfunc f(s interface{ Confirm() error }) error { return s.Confirm() }\n"
	f, err := parser.ParseFile(token.NewFileSet(), "x.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && ledgerMutators[sel.Sel.Name] {
				found = true
			}
		}
		return true
	})
	if !found {
		t.Fatal("the mutator walk did not flag a synthetic Confirm() call")
	}
}

// TestGoDocListsExactlyTheReadSurface runs the real `go doc` over this
// package and asserts its exported surface is exactly Open, Brain, Option,
// the CheckPlan and Recall receivers, and their wire types -- nothing
// writable, nothing else (ADR 012 §1). A new exported name fails this test
// until it is added here deliberately.
func TestGoDocListsExactlyTheReadSurface(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "doc", "-all", ".")
	cmd.Dir = wd
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go doc: %v\n%s", err, out)
	}

	var got []string
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case strings.HasPrefix(line, "func "), strings.HasPrefix(line, "type "),
			strings.HasPrefix(line, "const "), strings.HasPrefix(line, "var "):
			got = append(got, strings.TrimSuffix(strings.TrimSpace(line), " {"))
		}
	}
	sort.Strings(got)

	want := []string{
		"func (b *Brain) CheckPlan(ctx context.Context, actions []Action) (Verdict, error)",
		"func (b *Brain) Recall(ctx context.Context, q string, budget Budget) (RecallResult, error)",
		"func Open(brainPath string, opts ...Option) (*Brain, error)",
		"type Action struct",
		"type Answer struct",
		"type Brain struct",
		"type Budget struct",
		"type Citation struct",
		"type Constraint struct",
		"type Hit struct",
		"type Option func(*Brain) error",
		"type RecallResult struct",
		"type Supersession struct",
		"type Verdict struct",
		"type Warning struct",
	}
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("go doc surface differs\n got:\n  %s\nwant:\n  %s\nfull output:\n%s",
			strings.Join(got, "\n  "), strings.Join(want, "\n  "), out)
	}

	if !strings.Contains(string(out), "docs/adr/012-embedded-read-facade-single-writer.md") {
		t.Fatal("package doc does not link ADR 012")
	}
	if _, err := os.Stat(filepath.Join(wd, "..", "..", "docs", "adr", "012-embedded-read-facade-single-writer.md")); err != nil {
		t.Fatalf("linked ADR 012 is missing: %v", err)
	}
}
