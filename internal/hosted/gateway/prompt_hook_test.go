package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/agentmemory"
	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/meter"
	"github.com/sirerun/serenity/internal/server/mcp"
)

// Explicitly synthetic policy fixture. It is not a production enrollment,
// private-session or redaction implementation.
func syntheticPromptPolicy(_ context.Context, _ credential.Binding, query, _ string) (string, error) {
	if query != "synthetic recall marker" {
		return "", errors.New("not the enrolled synthetic prompt")
	}
	return query, nil
}

func TestPromptHookDefaultUnavailableDoesNotEnterRecall(t *testing.T) {
	ctx := context.Background()
	g, _, db, account, brain, _ := newLifecycleExportFixture(t)
	g.Meter = &meter.Meter{Store: db}
	token, err := g.Issuer.Issue(ctx, brain, account, []string{"memory:read"})
	if err != nil {
		t.Fatal(err)
	}
	// Any runtime/provider access would fail. Nil policy must stop before it.
	g.Pool = nil
	result, err := g.call(ctx, token, agentmemory.PromptToolName, json.RawMessage(`{"query":"synthetic recall marker"}`))
	assertPromptHookBlock(t, result, err)
	var count int
	if err := db.DB().QueryRowContext(ctx, `SELECT count(*) FROM usage_windows WHERE account_id=?`, account).Scan(&count); err != nil || count != 0 {
		t.Fatalf("default unavailable path touched recall meter: count=%d err=%v", count, err)
	}
}

func TestPromptHookSyntheticPolicyUsesExistingRecallOnce(t *testing.T) {
	for _, kind := range []string{"manual", "oauth-verifier-seam"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			g, _, db, account, brain, _ := newLifecycleExportFixture(t)
			g.Meter = &meter.Meter{Store: db}
			g.PromptRecallPolicy = syntheticPromptPolicy
			token, err := g.Issuer.Issue(ctx, brain, account, []string{"memory:read"})
			if err != nil {
				t.Fatal(err)
			}
			binding, err := g.Issuer.Verify(ctx, token)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "oauth-verifier-seam" {
				// Exercises the gateway's current OAuth-verifier boundary only;
				// no live OAuth grant or consent acceptance is inferred.
				token = "synthetic-oauth-access"
				binding.CredentialID = "oauth:synthetic-grant"
				g.Issuer.OAuthVerify = func(_ context.Context, raw string) (credential.Binding, error) {
					if raw != token {
						return credential.Binding{}, credential.ErrInvalidCredential
					}
					return binding, nil
				}
			}
			result, err := g.promptContextTool(token, binding).Handler(ctx, json.RawMessage(`{"query":"synthetic recall marker"}`))
			if err != nil || result.IsError || len(result.Content) != 1 {
				t.Fatalf("prompt recall: %+v %v", result, err)
			}
			var native struct {
				Decision string `json:"decision"`
				Context  struct {
					Event string `json:"hookEventName"`
					Text  string `json:"additionalContext"`
				} `json:"hookSpecificOutput"`
			}
			if json.Unmarshal([]byte(result.Content[0].Text), &native) != nil || native.Decision != "" || native.Context.Event != "UserPromptSubmit" || !strings.Contains(native.Context.Text, "bounded_candidate_window_not_exhaustive") {
				t.Fatalf("not native context: %s", result.Content[0].Text)
			}
			var used int
			if err := db.DB().QueryRowContext(ctx, `SELECT COALESCE(sum(committed),0) FROM usage_windows WHERE account_id=? AND metric='recalls'`, account).Scan(&used); err != nil || used != 1 {
				t.Fatalf("recall not metered exactly once: used=%d err=%v", used, err)
			}
		})
	}
}

func TestPromptHookRejectsChangedAuthorityBeforePolicy(t *testing.T) {
	for _, kind := range []string{"revoked", "locked", "wrong-brain", "write-only", "generation-changed"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			g, _, db, account, brain, _ := newLifecycleExportFixture(t)
			g.Meter = &meter.Meter{Store: db}
			scopes := []string{"memory:read"}
			if kind == "write-only" {
				scopes = []string{"memory:write"}
			}
			token, err := g.Issuer.Issue(ctx, brain, account, scopes)
			if err != nil {
				t.Fatal(err)
			}
			binding, err := g.Issuer.Verify(ctx, token)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			g.PromptRecallPolicy = func(context.Context, credential.Binding, string, string) (string, error) {
				calls++
				return "synthetic recall marker", nil
			}
			switch kind {
			case "revoked":
				err = g.Issuer.RevokeKey(ctx, binding.CredentialID)
			case "locked":
				_, err = db.DB().ExecContext(ctx, `UPDATE accounts SET status='restore_pending' WHERE id=?`, account)
			case "wrong-brain":
				binding.BrainID = "foreign-brain"
			case "generation-changed":
				binding.Generation++
			}
			if err != nil {
				t.Fatal(err)
			}
			result, err := g.promptContextTool(token, binding).Handler(ctx, json.RawMessage(`{"query":"synthetic recall marker"}`))
			assertPromptHookBlock(t, result, err)
			if calls != 0 {
				t.Fatal("invalid current authority reached policy")
			}
		})
	}
}

func TestPromptHookRechecksRevocationAfterPolicy(t *testing.T) {
	ctx := context.Background()
	g, _, db, account, brain, _ := newLifecycleExportFixture(t)
	g.Meter = &meter.Meter{Store: db}
	token, err := g.Issuer.Issue(ctx, brain, account, []string{"memory:read"})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := g.Issuer.Verify(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	g.PromptRecallPolicy = func(context.Context, credential.Binding, string, string) (string, error) {
		return "synthetic recall marker", g.Issuer.RevokeKey(ctx, binding.CredentialID)
	}
	g.Pool = nil // no runtime/provider must be entered after policy revokes.
	result, err := g.promptContextTool(token, binding).Handler(ctx, json.RawMessage(`{"query":"synthetic recall marker"}`))
	assertPromptHookBlock(t, result, err)
}

func TestPromptHookAdvertisedWithExistingToolsButDefaultOff(t *testing.T) {
	ctx := context.Background()
	g, _, _, account, brain, _ := newLifecycleExportFixture(t)
	t.Cleanup(g.Close)
	token, err := g.Issuer.Issue(ctx, brain, account, []string{"memory:read"})
	if err != nil {
		t.Fatal(err)
	}
	post := func(body, session string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/mcp", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		if session != "" {
			r.Header.Set(mcp.SessionIDHeader, session)
			r.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
		}
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		return w
	}
	init := post(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"fixture","version":"1"}}}`, "")
	session := init.Header().Get(mcp.SessionIDHeader)
	if init.Code != 200 || session == "" {
		t.Fatalf("initialize: %d %s", init.Code, init.Body.String())
	}
	post(`{"jsonrpc":"2.0","method":"notifications/initialized"}`, session)
	listed := post(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, session)
	var frame struct {
		Result struct {
			Tools []mcp.Tool `json:"tools"`
		} `json:"result"`
	}
	if listed.Code != 200 || json.Unmarshal(listed.Body.Bytes(), &frame) != nil || len(frame.Result.Tools) != 6 {
		t.Fatalf("list: %s", listed.Body.String())
	}
	found := false
	for _, tool := range frame.Result.Tools {
		if tool.Name == agentmemory.PromptToolName {
			found = true
		}
	}
	if !found {
		t.Fatal("new prompt tool not advertised")
	}
	called := post(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"recall_prompt_context","arguments":{"query":"synthetic recall marker"}}}`, session)
	var response struct {
		Result mcp.Result `json:"result"`
	}
	if called.Code != 200 || json.Unmarshal(called.Body.Bytes(), &response) != nil {
		t.Fatalf("call: %s", called.Body.String())
	}
	assertPromptHookBlock(t, response.Result, nil)
}

func assertPromptHookBlock(t *testing.T, result mcp.Result, err error) {
	t.Helper()
	if err != nil || result.IsError || len(result.Content) != 1 {
		t.Fatalf("expected ordinary native block: %+v %v", result, err)
	}
	var out struct{ Decision string }
	if json.Unmarshal([]byte(result.Content[0].Text), &out) != nil || out.Decision != "block" {
		t.Fatalf("expected native block JSON: %s", result.Content[0].Text)
	}
}
