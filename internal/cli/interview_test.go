package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/providers"
)

var interviewFixedNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

// TestRunInterviewSkipsGracefullyWithNoComposerModelPinned covers the one
// deterministic, credential-free path through runInterview: a freshly
// initialized brain repo pins no composer model (config.Default's
// Models.Composer == "none@v0"), so BuildComposerRouter returns ok=false
// and runInterview must report that and return nil rather than error --
// the same graceful-skip contract runAsk/runSync already follow for their
// own Build*Router calls, none of which this package unit-tests through a
// credentialed path either (no ask_test.go exists at all). Deep
// behavioral coverage of the interview loop itself (the ">= 10 staged
// drafts" acc line, the "do not adopt this" floor, the prompt-injection
// case) lives in internal/direction/interview's own Completer-level
// tests, mirroring T3.11/decompose_test.go's own choice not to test
// through a CLI layer that doesn't exist for it either.
func TestRunInterviewSkipsGracefullyWithNoComposerModelPinned(t *testing.T) {
	root := initBrainRepo(t)
	var out bytes.Buffer

	err := runInterview(context.Background(), root, strings.NewReader(""), &out, interviewFixedNow)
	if err != nil {
		t.Fatalf("runInterview: %v", err)
	}
	if !strings.Contains(out.String(), "interview:") {
		t.Fatalf("output = %q, want a skip note prefixed \"interview:\"", out.String())
	}

	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatalf("OpenIndex: %v", err)
	}
	defer func() { _ = eng.Close() }()
	items, err := disposition.NewStore(eng).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("len(items) = %d, want 0 -- no model pinned means no draft synthesis happened", len(items))
	}
}

func TestNewInterviewCmdIsRegistered(t *testing.T) {
	root := newRootCmd()
	found := false
	for _, c := range root.Commands() {
		if c.Name() == "interview" {
			found = true
		}
	}
	if !found {
		t.Fatal(`newRootCmd(): "interview" is not registered`)
	}
}
