package serenity_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kazi-org/dira/ledger"
	"github.com/sirerun/serenity/pkg/serenity"
)

func TestEmbeddedLexicalCheckPreservesBrainAndStructuredContract(t *testing.T) {
	root := fixtureBrain(t)
	seedEntry(t, root, &ledger.Entry{ID: "dec-0999", Kind: ledger.KindDecision,
		State: ledger.StateAccepted, Title: "Keep the tool foreground only", Private: true,
		Created:      "2026-11-01T00:00:00Z",
		Alternatives: []ledger.Alternative{{Option: "background daemon", WhyNot: "private-only-rationale", RevisitIf: "private-only-trigger"}},
	})
	b, err := serenity.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	actions := []serenity.Action{{Action: "spend_over", Params: map[string]any{"amount": 600}}}
	beforeStructured, err := b.CheckPlan(ctx, actions)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func() map[string][32]byte {
		t.Helper()
		out := make(map[string][32]byte)
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			data, err := os.ReadFile(path)
			if err == nil {
				out[path] = sha256.Sum256(data)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	beforeFiles := snapshot()
	got, err := b.CheckLexical(ctx, "add a background daemon to track run state")
	if err != nil {
		t.Fatal(err)
	}
	if got.ExitCode != 2 || len(got.Conflicts) == 0 {
		t.Fatal("real lexical conflict was not retained")
	}
	wire, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(wire), "private-only-") {
		t.Fatal("private citation rationale disclosed")
	}
	afterStructured, err := b.CheckPlan(ctx, actions)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(beforeStructured, afterStructured) {
		t.Fatal("structured CheckPlan changed")
	}
	if !reflect.DeepEqual(beforeFiles, snapshot()) {
		t.Fatal("read-only lexical check changed brain files")
	}
}

func TestEmbeddedLexicalCheckRejectsUnopenedBrain(t *testing.T) {
	for _, b := range []*serenity.Brain{nil, {}} {
		if result, err := b.CheckLexical(context.Background(), "add daemon"); result != nil || err == nil {
			t.Fatal("unopened brain produced lexical result")
		}
	}
}
