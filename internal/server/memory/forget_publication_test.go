package memory

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/writer"
)

func TestHostedForgetPublishesErasureBeforeHandlerReturns(t *testing.T) {
	h, root := newTestHandlers(t)
	ctx := context.Background()
	remembered, isError, err := h.remember(ctx, mustMarshal(t, rememberRequest{
		Fact:       "hosted forget source boundary marker",
		Provenance: "fixture",
	}))
	if err != nil || isError {
		t.Fatalf("remember: result=%+v isError=%v err=%v", remembered, isError, err)
	}
	id := remembered.(rememberResponse).ID
	if _, err := writer.Flush(h.deps.Queue, root); err != nil {
		t.Fatal(err)
	}
	forgotten, isError, err := h.forget(WithHostedForgetPublication(ctx), mustMarshal(t, map[string]string{"id": id}))
	if err != nil || isError || !forgotten.(forgetResponse).Expired {
		t.Fatalf("forget: result=%+v isError=%v err=%v", forgotten, isError, err)
	}
	path := "brain/sources/" + id[:2] + "/" + id
	cmd := exec.Command("git", "-C", root, "ls-tree", "-r", "--name-only", "HEAD", "--", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect committed forget result: %v: %s", err, out)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("handler returned before erasure reached HEAD: %q", out)
	}
}
