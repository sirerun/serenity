package memory

import (
	"context"
	"testing"
)

// rememberAs stores one world fact under principal p and returns its id.
func rememberAs(t *testing.T, h *Handlers, ctx context.Context, fact string) string {
	t.Helper()
	resp, isError, err := h.remember(ctx, mustMarshal(t, rememberRequest{Fact: fact, Provenance: "authz test"}))
	if err != nil || isError {
		t.Fatalf("remember: err=%v isError=%v resp=%+v", err, isError, resp)
	}
	return resp.(rememberResponse).ID
}

// AI-L03: remember records the calling principal as the fact's writer.
func TestRememberRecordsCallerAsWriter(t *testing.T) {
	h, _ := newTestHandlers(t)
	a := WithPrincipal(context.Background(), Principal{ID: "credential:a", Scopes: []string{"memory:write", ScopeForget}})
	id := rememberAs(t, h, a, "written by a")
	got, err := h.deps.memoryWriter().FactWriter(id)
	if err != nil || got != "credential:a" {
		t.Fatalf("writer = %q, %v; want credential:a", got, err)
	}
	local := rememberAs(t, h, context.Background(), "written locally")
	if got, err := h.deps.memoryWriter().FactWriter(local); err != nil || got != LocalWriter {
		t.Fatalf("local writer = %q, %v; want %q", got, err, LocalWriter)
	}
}

// AI-L03: forget by a non-human principal other than the writer is forbidden;
// the writer and a human may forget; the memory:forget scope is required.
func TestForgetRestrictedToWriterOrHuman(t *testing.T) {
	h, _ := newTestHandlers(t)
	a := WithPrincipal(context.Background(), Principal{ID: "credential:a", Scopes: []string{"memory:write", ScopeForget}})
	b := WithPrincipal(context.Background(), Principal{ID: "credential:b", Scopes: []string{"memory:write", ScopeForget}})
	aNoForget := WithPrincipal(context.Background(), Principal{ID: "credential:a", Scopes: []string{"memory:write"}})
	human := WithPrincipal(context.Background(), Principal{ID: "account:owner", Human: true})

	forget := func(ctx context.Context, id string) (any, bool) {
		t.Helper()
		resp, isError, err := h.forget(ctx, mustMarshal(t, forgetRequest{ID: id}))
		if err != nil {
			t.Fatalf("forget: %v", err)
		}
		return resp, isError
	}

	id := rememberAs(t, h, a, "a's shared fact")
	if resp, isError := forget(b, id); asVerbError(t, resp, isError).Error != ErrCodeForbidden {
		t.Fatalf("other principal forget = %+v, want %s", resp, ErrCodeForbidden)
	}
	if resp, isError := forget(aNoForget, id); asVerbError(t, resp, isError).Error != ErrCodeScopeDenied {
		t.Fatalf("writer without %s = %+v, want %s", ScopeForget, resp, ErrCodeScopeDenied)
	}
	if resp, isError := forget(a, id); isError || !resp.(forgetResponse).Expired {
		t.Fatalf("writer forget = %+v, want expired", resp)
	}

	id2 := rememberAs(t, h, a, "a's second shared fact")
	if resp, isError := forget(human, id2); isError || !resp.(forgetResponse).Expired {
		t.Fatalf("human forget = %+v, want expired", resp)
	}

	// A fact without a recorded writer (legacy, or written on the local
	// path) is forgettable only by a human actor.
	legacy := rememberAs(t, h, context.Background(), "local fact")
	if resp, isError := forget(a, legacy); asVerbError(t, resp, isError).Error != ErrCodeForbidden {
		t.Fatalf("non-human forget of writerless fact = %+v, want %s", resp, ErrCodeForbidden)
	}
	spoof := WithPrincipal(context.Background(), Principal{ID: LocalWriter, Scopes: []string{ScopeForget}})
	if resp, isError := forget(spoof, legacy); asVerbError(t, resp, isError).Error != ErrCodeForbidden {
		t.Fatalf("non-human principal named %q forget = %+v, want %s", LocalWriter, resp, ErrCodeForbidden)
	}
	if resp, isError := forget(context.Background(), legacy); isError || !resp.(forgetResponse).Expired {
		t.Fatalf("local forget = %+v, want expired", resp)
	}
}
