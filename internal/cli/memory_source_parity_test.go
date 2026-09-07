package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/router"
	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/server/memory"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

func parityCommand(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetArgs(append([]string{"-C", root}, args...))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.ExecuteContext(t.Context()); err != nil {
		t.Fatalf("CLI %v: %v\n%s", args, err, out.String())
	}
	return out.String()
}

// Each call opens a new index, queue, registry, and actual MCP stream session.
// Canonical identity survives transport shutdown and derived-index rebuilds.
func parityMCP(t *testing.T, root, verb string, args map[string]any, clock memory.Clock, completer *parityCompleter) map[string]any {
	t.Helper()
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := eng.Close(); err != nil {
			t.Error(err)
		}
	}()
	q := writer.NewQueue(nil)
	defer q.Close()
	deps := memory.Deps{Root: root, Config: config.Default(), Index: eng, Queue: q, Sources: store.NewSourceStore(root), Fence: store.NewFenceWriter(root), Shard: store.NewShardStore(root), Clock: clock}
	if completer != nil {
		deps.Composer = completer
		deps.ComposerModelVersion = "parity-test@v1"
	}
	srv, err := mcp.New("source-parity", memory.New(deps).Tools())
	if err != nil {
		t.Fatal(err)
	}
	input, send := io.Pipe()
	receive, output := io.Pipe()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, input, output) }()
	defer func() {
		cancel()
		_ = send.Close()
		_ = receive.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("MCP shutdown timeout")
		}
	}()
	enc, dec := json.NewEncoder(send), json.NewDecoder(receive)
	write := func(frame any) {
		t.Helper()
		if err := enc.Encode(frame); err != nil {
			t.Fatal(err)
		}
	}
	read := func() map[string]json.RawMessage {
		t.Helper()
		var frame map[string]json.RawMessage
		if err := dec.Decode(&frame); err != nil {
			t.Fatal(err)
		}
		if frame["error"] != nil {
			t.Fatalf("JSON-RPC error: %s", frame["error"])
		}
		return frame
	}
	write(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": mcp.ProtocolVersion, "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "parity-test", "version": "1"}}})
	read()
	write(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	write(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": verb, "arguments": args}})
	frame := read()
	var result mcp.Result
	if err := json.Unmarshal(frame["result"], &result); err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("MCP %s: %+v", verb, result)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &body); err != nil {
		t.Fatal(err)
	}
	if body["protocol_version"] != float64(1) {
		t.Fatalf("unversioned result: %v", body)
	}
	return body
}

type parityCompleter struct {
	text    string
	prompts []string
}

func (p *parityCompleter) Complete(_ context.Context, _ router.TaskClass, prompt router.Prompt, _ router.Budget) (router.Result, error) {
	p.prompts = append(p.prompts, prompt.Text)
	return router.Result{Text: p.text, ModelVersion: "parity-test@v1"}, nil
}

func TestMemoryV1SharedCLISourceParity(t *testing.T) {
	root := initBrainRepo(t)
	const public = "sourceparity PUBLICREPORT"
	const private = "sourceparity PRIVATEREPORT"
	// Fixed historical capture time keeps the canonical records deterministic;
	// unbounded records remain live at both the CLI and MCP query clocks.
	clock := driftClock{time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)}
	saved := parityMCP(t, root, "remember", map[string]any{"fact": public, "provenance": "parity attribution", "entity": "parityentity"}, clock, nil)
	id := saved["id"].(string)
	parityMCP(t, root, "remember", map[string]any{"fact": private, "provenance": "private attribution", "visibility": "private", "entity": "parityentity"}, clock, nil)
	parityCommand(t, root, "sync")
	local := parityCommand(t, root, "search", "sourceparity")
	if !strings.Contains(local, public) || !strings.Contains(local, private) {
		t.Fatalf("actual local search lost saved source: %s", local)
	}
	remote := parityMCP(t, root, "recall", map[string]any{"query": "sourceparity"}, nil, nil)
	encoded, err := json.Marshal(remote)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), public) || !strings.Contains(string(encoded), id) || strings.Contains(string(encoded), private) {
		t.Fatalf("reopened MCP source visibility: %s", encoded)
	}

	// The real ask command has no test-provider injection seam. Its unconfigured
	// behavior is exercised through Cobra; success below uses the actual shared
	// ask core also used by that command, with a deterministic provider double.
	unavailable := parityCommand(t, root, "ask", "sourceparity")
	if !strings.Contains(unavailable, "no composer model pinned") {
		t.Fatalf("actual ask did not disclose unavailable provider: %s", unavailable)
	}
	completer := &parityCompleter{text: "Public report [source:" + id + "]."}
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	answer, askErr := askAnswerFor(t.Context(), root, config.Default(), eng, nil, completer, "parity-test@v1", "sourceparity")
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}
	if askErr != nil {
		t.Fatal(askErr)
	}
	if len(answer.Citations) != 0 || len(answer.SourceCitations) != 1 || answer.SourceCitations[0].SHA256 != id || answer.SourceCitations[0].Provenance != "parity attribution" {
		t.Fatalf("raw source became fabricated claim or lost attribution: %+v", answer)
	}
	synthesized := parityMCP(t, root, "synthesize", map[string]any{"question": "sourceparity"}, nil, completer)
	if synthesized["answer"] != answer.Text {
		t.Fatalf("shared ask and real MCP synthesize diverged: %v / %+v", synthesized, answer)
	}
	if len(completer.prompts) != 2 {
		t.Fatalf("expected both real shared composition paths: %d prompts", len(completer.prompts))
	}
	for _, prompt := range completer.prompts {
		if !strings.Contains(prompt, public) || strings.Contains(prompt, private) {
			t.Fatalf("provider audience filtering failed: %s", prompt)
		}
	}

	forgotten := parityMCP(t, root, "forget", map[string]any{"id": id, "reason": "parity lifecycle"}, nil, nil)
	if forgotten["expired"] != true {
		t.Fatalf("forget did not expire: %v", forgotten)
	}
	// Before another rebuild, stale index rows must already be ineligible.
	local = parityCommand(t, root, "search", "sourceparity")
	remote = parityMCP(t, root, "recall", map[string]any{"query": "sourceparity"}, nil, nil)
	encoded, err = json.Marshal(remote)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(local, public) || strings.Contains(string(encoded), public) || !strings.Contains(local, private) {
		t.Fatalf("forget leaked or erased unrelated local source: CLI %s MCP %s", local, encoded)
	}
	parityCommand(t, root, "sync")
	if strings.Contains(parityCommand(t, root, "search", "sourceparity"), public) {
		t.Fatal("forgotten source resurrected after rebuild")
	}

	// Save under an old deterministic clock, then read under the live CLI/MCP
	// clocks: an elapsed TTL must stay invisible across reopen and rebuild.
	parityMCP(t, root, "remember", map[string]any{"fact": "ttlparity EXPIREDREPORT", "provenance": "TTL fixture", "ttl": "12h"}, clock, nil)
	parityCommand(t, root, "sync")
	if strings.Contains(parityCommand(t, root, "search", "ttlparity"), "EXPIREDREPORT") {
		t.Fatal("actual CLI returned expired source")
	}
	expired := parityMCP(t, root, "recall", map[string]any{"query": "ttlparity"}, nil, nil)
	encoded, err = json.Marshal(expired)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "EXPIREDREPORT") {
		t.Fatalf("reopened MCP returned expired source: %s", encoded)
	}
}
