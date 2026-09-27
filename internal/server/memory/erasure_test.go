package memory

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/writer"
)

// TestForgetErasesFactTreeAndIndexRowsInSameFlush drives PRIV-01 through
// the MCP verbs: after forget and one flush the fact's directory is gone
// from the working tree and the committed tree, and its FTS and vector
// rows are gone from the index.
func TestForgetErasesFactTreeAndIndexRowsInSameFlush(t *testing.T) {
	h, root := newTestHandlers(t)
	e := &recordingMemoryEmbedder{}
	h.deps.Embedder = e
	ctx := context.Background()
	v, bad, err := h.remember(ctx, mustMarshal(t, rememberRequest{Fact: "erasure marker b41d", Provenance: "test"}))
	if err != nil || bad {
		t.Fatalf("remember: %v %+v", err, v)
	}
	id := v.(rememberResponse).ID
	if _, err := writer.Flush(h.deps.Queue, root); err != nil {
		t.Fatal(err)
	}
	if has, err := h.deps.Index.HasVector(ctx, "fact:"+id, e.ModelVersion()); err != nil || !has {
		t.Fatalf("fixture has no vector: %v", err)
	}

	if resp, bad, err := h.forget(ctx, mustMarshal(t, map[string]string{"id": id})); err != nil || bad {
		t.Fatalf("forget: %v %+v", err, resp)
	}
	if _, err := writer.Flush(h.deps.Queue, root); err != nil {
		t.Fatal(err)
	}

	dir := h.deps.Sources.DirFor(id)
	if _, err := os.Lstat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("fact directory survived forget: %v", err)
	}
	rel, _ := filepath.Rel(root, dir)
	out, err := exec.Command("git", "-C", root, "ls-tree", "-r", "--name-only", "HEAD", "--", filepath.ToSlash(rel)).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "" {
		t.Fatalf("fact path still in HEAD after the flush: %q %v", out, err)
	}
	chunks, err := h.deps.Index.AllChunks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range chunks {
		if c.ChunkRef == "fact:"+id || c.SourceSHA256 == id {
			t.Fatalf("FTS row survived forget: %+v", c)
		}
	}
	if has, err := h.deps.Index.HasVector(ctx, "fact:"+id, e.ModelVersion()); err != nil || has {
		t.Fatalf("vector row survived forget: has=%v err=%v", has, err)
	}
	again, bad, err := h.forget(ctx, mustMarshal(t, map[string]string{"id": id}))
	if err != nil || bad || again.(forgetResponse).Expired {
		t.Fatalf("re-forget after erasure is not idempotent: %v %+v", err, again)
	}
}
