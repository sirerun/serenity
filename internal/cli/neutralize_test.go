package cli

import (
	"bytes"
	"context"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/compose"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/search"
)

// terminalPayload is deep review 001's SEC-M05 hide-and-spoof shape as it
// would arrive in ingested source text or a model answer: conceal, erase
// the line, move the cursor, redraw, plant a hyperlink and retitle the
// window.
const terminalPayload = "\x1b[8mhidden\x1b[0m\r\x1b[2K\x1b[1Aspoofed" +
	"\x1b]8;;https://attacker.example/\x07link\x1b]8;;\x07" +
	"\x1b]0;title\x1b\\\u009b2J\x07\x08end"

// assertNoTerminalControls fails when out carries ESC, a C0 control other
// than newline or tab, DEL, or a C1 control.
func assertNoTerminalControls(t *testing.T, out string) {
	t.Helper()
	for _, r := range out {
		if r == '\n' || r == '\t' {
			continue
		}
		if r < 0x20 || (r >= 0x7f && r < 0xa0) {
			t.Fatalf("output carries control character %U: %q", r, out)
		}
	}
}

func TestAskOutputStripsTerminalControls(t *testing.T) {
	for name, answer := range map[string]compose.Answer{
		"answer": {Text: "Ava works at " + terminalPayload, SourceCitations: []compose.SourceCitation{{
			SHA256: "abc123", Fact: "source fact " + terminalPayload, Provenance: "provenance " + terminalPayload,
		}}},
		"gap": {Gap: "no evidence " + terminalPayload},
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := writeAskAnswer(&out, answer); err != nil {
				t.Fatal(err)
			}
			assertNoTerminalControls(t, out.String())
			if !strings.Contains(out.String(), "hiddenspoofedlinkend") {
				t.Fatalf("visible text was lost: %q", out.String())
			}
		})
	}
}

func TestSearchOutputStripsTerminalControls(t *testing.T) {
	var out bytes.Buffer
	writeSearchResults(&out, "note", []search.Result{{Hit: index.Hit{ChunkRef: "src\x1b[2K/ref", Text: terminalPayload}, RRFScore: 1}})
	assertNoTerminalControls(t, out.String())
	if !strings.Contains(out.String(), "hiddenspoofedlinkend") || !strings.Contains(out.String(), "src/ref") {
		t.Fatalf("visible text was lost: %q", out.String())
	}
}

func TestInboxInteractiveOutputStripsTerminalControls(t *testing.T) {
	dispStore, root := openInboxTestStore(t)
	ctx := context.Background()
	seedReconcileItem(t, dispStore, ctx, inboxFixedNow, "ava-standardo", "has_balance", "$9000"+terminalPayload, "$0", "")

	var out bytes.Buffer
	if err := runInteractive(ctx, dispStore, newTestSupersedeWriter(t, root), newTestDirectionStore(t, root), strings.NewReader("q"), &out, "human:test", inboxFixedNow); err != nil {
		t.Fatalf("runInteractive: %v", err)
	}
	assertNoTerminalControls(t, out.String())
	if !strings.Contains(out.String(), "$9000hiddenspoofedlinkend") {
		t.Fatalf("visible text was lost: %q", out.String())
	}
}

func TestInboxParkedOutputStripsTerminalControls(t *testing.T) {
	dispStore, _ := openInboxTestStore(t)
	ctx := context.Background()
	toPark := seedReconcileItem(t, dispStore, ctx, inboxFixedNow, "ava-standardo", "has_balance", "$9000"+terminalPayload, "$0", "")
	now := inboxFixedNow
	for i := 0; i < disposition.MaxDeferCycles; i++ {
		now = now.Add(2 * time.Minute)
		if _, err := disposition.Sweep(ctx, dispStore, disposition.Thresholds{disposition.KindReconcile: time.Minute}, now); err != nil {
			t.Fatalf("Sweep: %v", err)
		}
	}
	if got, err := dispStore.Get(ctx, toPark.ID); err != nil || got.State != disposition.StateParked {
		t.Fatalf("fixture setup failed: %+v %v", got, err)
	}

	var out bytes.Buffer
	if err := runListParked(ctx, dispStore, &out); err != nil {
		t.Fatalf("runListParked: %v", err)
	}
	assertNoTerminalControls(t, out.String())
	if !strings.Contains(out.String(), "$9000hiddenspoofedlinkend") {
		t.Fatalf("visible text was lost: %q", out.String())
	}
}

// TestUntrustedPrintSitesAreNeutralized covers the print sites the
// scripted tests above cannot reach cheaply (a paused edit's preview, a
// published ledger title, a family separator): none of the listed
// ingested- or model-derived expressions may be handed to fmt.Fprint*
// without passing through neutralize.Text.
func TestUntrustedPrintSitesAreNeutralized(t *testing.T) {
	untrusted := map[string]map[string]bool{
		"ask.go":    {"answer.Gap": true, "answer.Text": true, "source.Fact": true, "source.Provenance": true},
		"search.go": {"r.ChunkRef": true, "r.Text": true},
		"inbox.go":  {"row.Family": true, "describeRow(row)": true, "preview": true, "entry.Title": true, "itemSummary(it)": true},
	}
	for file, exprs := range untrusted {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || !strings.HasPrefix(sel.Sel.Name, "Fprint") {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "fmt" {
				return true
			}
			for _, arg := range call.Args {
				var b bytes.Buffer
				if err := printer.Fprint(&b, fset, arg); err != nil {
					t.Fatal(err)
				}
				src := b.String()
				for expr := range exprs {
					wrapped := "neutralize.Text(" + expr + ")"
					switch {
					case strings.Contains(src, wrapped):
						seen[expr] = true
					case strings.Contains(src, expr):
						t.Errorf("%s: %s prints %s without neutralize.Text", file, fset.Position(arg.Pos()), src)
					}
				}
			}
			return true
		})
		for expr := range exprs {
			if !seen[expr] {
				t.Errorf("%s: no print of neutralize.Text(%s) found", file, expr)
			}
		}
	}
}
