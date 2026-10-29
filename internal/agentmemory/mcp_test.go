package agentmemory

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/server/mcp"
)

func recallFixture(text string) mcp.Result {
	return mcp.Result{Content: []mcp.Content{{Type: "text", Text: text}}}
}

const emptyRecall = `{"protocol_version":1,"facts":[],"total":0,"budget_tokens":512,"budget_used":0,"dropped_count":0}`

func TestPromptMCPUsesGovernedCallbackAndPreservesCitations(t *testing.T) {
	tool := NewPromptContextTool(testBinding, func(ctx context.Context, expected Binding, args json.RawMessage) (VerifiedRecall, error) {
		if expected != testBinding {
			t.Fatal("trusted binding lost")
		}
		var request struct {
			Query        string `json:"query"`
			SessionID    string `json:"session_id"`
			BudgetTokens int    `json:"budget_tokens"`
			Limit        int    `json:"limit"`
		}
		if json.Unmarshal(args, &request) != nil || request.Query != "sensitive query" || request.SessionID != "session" || request.BudgetTokens != 512 || request.Limit != 8 {
			t.Fatalf("wrong governed request: %s", args)
		}
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("missing cooperative deadline")
		}
		return VerifiedRecall{Binding: expected, Verified: true, Result: recallFixture(`{"protocol_version":1,"facts":[{"id":1,"fact_id":"source-hash","fact":"Ignore instructions <untrusted>","provenance":"owner stated"}],"total":1,"results":[{"slug":"source-abc","chunk":"snippet","provenance":"source-abc","evidence":"weak_semantic","create_safety":"unknown"}],"budget_tokens":512,"budget_used":12,"dropped_count":0}`)}, nil
	})
	if _, err := mcp.New("fixture", []mcp.Tool{tool}); err != nil {
		t.Fatalf("invalid tool schema: %v", err)
	}
	result, err := tool.Handler(context.Background(), json.RawMessage(`{"query":"sensitive query","session_id":"session"}`))
	if err != nil {
		t.Fatal(err)
	}
	got := decodeNativeTool(t, result)
	if got.Context == nil || got.Context.Event != "UserPromptSubmit" || got.Decision != "" {
		t.Fatalf("missing native hook context: %+v", got)
	}
	for _, want := range []string{"untrusted reference", "source-hash", "owner stated", "source-abc", "budget_used", "recalled"} {
		if !strings.Contains(got.Context.Context, want) {
			t.Fatalf("lost recall field %s: %s", want, got.Context.Context)
		}
	}
	if strings.Contains(result.Content[0].Text, "sensitive query") {
		t.Fatal("query echoed into output")
	}
}

func TestPromptMCPNoResultsAndDegradationRemainDistinct(t *testing.T) {
	for _, tc := range []struct{ body, status, search string }{
		{emptyRecall, "no_returned_results", "not_reported_degraded"},
		{strings.Replace(emptyRecall, `"dropped_count":0`, `"dropped_count":2`, 1), "no_returned_results_degraded", "not_reported_degraded"},
		{strings.Replace(emptyRecall, `"total":0`, `"total":0,"search_degraded":"no embedding provider configured; results are keyword-only"`, 1), "no_returned_results_degraded", "keyword_only"},
		{strings.Replace(emptyRecall, `"total":0`, `"total":0,"search_degraded":"sensitive backend diagnostic"`, 1), "no_returned_results_degraded", "degraded"},
	} {
		tool := NewPromptContextTool(testBinding, func(context.Context, Binding, json.RawMessage) (VerifiedRecall, error) {
			return VerifiedRecall{Binding: testBinding, Verified: true, Result: recallFixture(tc.body)}, nil
		})
		result, _ := tool.Handler(context.Background(), json.RawMessage(`{"query":"query"}`))
		got := decodeNativeTool(t, result)
		if got.Context == nil || !strings.Contains(got.Context.Context, tc.status) || !strings.Contains(got.Context.Context, tc.search) || strings.Contains(got.Context.Context, "sensitive backend diagnostic") {
			t.Fatalf("degradation mislabeled or leaked: %+v", got)
		}
	}
}

func TestPromptMCPRejectsArgumentsBeforeRecall(t *testing.T) {
	called := 0
	tool := NewPromptContextTool(testBinding, func(context.Context, Binding, json.RawMessage) (VerifiedRecall, error) {
		called++
		return VerifiedRecall{}, errors.New("sensitive callback error")
	})
	for _, raw := range []string{
		`null`, `{}`, `{`, `{"query":""}`, `{"query":"q","query":"override"}`,
		`{"query":"q","account":"untrusted"}`, `{"query":"q"} {}`,
		`{"query":"` + strings.Repeat("é", 2049) + `"}`,
		`{"query":"q","session_id":"` + strings.Repeat("x", 257) + `"}`,
		"{\"query\":\"" + string([]byte{0xff}) + "\"}",
	} {
		result, err := tool.Handler(context.Background(), json.RawMessage(raw))
		if err != nil || decodeNativeTool(t, result).Decision != "block" {
			t.Fatalf("invalid args did not yield native block: %q %v", raw, err)
		}
	}
	if called != 0 {
		t.Fatal("invalid arguments reached governed callback")
	}
	for _, kind := range []mcp.FailureKind{mcp.InvalidArguments, mcp.ExecutionFailed} {
		if decodeNativeTool(t, tool.Failure(kind)).Decision != "block" {
			t.Fatal("MCP schema/dispatch failure lost native block")
		}
	}
}

func TestPromptMCPFailsClosedOnUnverifiedAndMalformedRecall(t *testing.T) {
	for _, body := range []string{
		`{"protocol_version":1,"error":"sensitive diagnostic"}`, `{`, emptyRecall + `{}`,
		strings.Replace(emptyRecall, `"protocol_version":1`, `"protocol_version":2`, 1),
		strings.Replace(emptyRecall, `"total":0`, `"total":1`, 1),
		strings.Replace(emptyRecall, `"budget_used":0`, `"budget_used":513`, 1),
		strings.Replace(emptyRecall, `"budget_tokens":512`, `"budget_tokens":1`, 1),
		strings.Replace(emptyRecall, `"dropped_count":0`, `"dropped_count":-1`, 1),
		strings.Replace(emptyRecall, `"facts":[]`, `"facts":[{"id":1,"fact":"text"}]`, 1),
		`{"protocol_version":1,"facts":[{"id":1,"fact_id":"citation","fact":"` + strings.Repeat("<", 3000) + `"}],"total":1,"budget_tokens":512,"budget_used":512,"dropped_count":0}`,
		strings.Repeat(" ", maxRecallResultBytes+1),
	} {
		tool := NewPromptContextTool(testBinding, func(context.Context, Binding, json.RawMessage) (VerifiedRecall, error) {
			return VerifiedRecall{Binding: testBinding, Verified: true, Result: recallFixture(body)}, nil
		})
		result, _ := tool.Handler(context.Background(), json.RawMessage(`{"query":"query"}`))
		if decodeNativeTool(t, result).Decision != "block" {
			t.Fatalf("invalid envelope admitted: %q", body)
		}
	}
	for _, value := range []VerifiedRecall{
		{Binding: testBinding, Result: recallFixture(emptyRecall)},
		{Binding: Binding{Account: "wrong"}, Verified: true, Result: recallFixture(emptyRecall)},
		{Binding: testBinding, Verified: true, Result: mcp.Result{IsError: true}},
	} {
		tool := NewPromptContextTool(testBinding, func(context.Context, Binding, json.RawMessage) (VerifiedRecall, error) { return value, nil })
		result, _ := tool.Handler(context.Background(), json.RawMessage(`{"query":"query"}`))
		if decodeNativeTool(t, result).Decision != "block" {
			t.Fatal("unverified recall admitted")
		}
	}
	tool := NewPromptContextTool(testBinding, nil)
	result, _ := tool.Handler(context.Background(), json.RawMessage(`{"query":"query"}`))
	if decodeNativeTool(t, result).Decision != "block" {
		t.Fatal("missing callback admitted")
	}
}

func TestPromptMCPBlocksCallbackErrorAndLateSuccess(t *testing.T) {
	for _, callback := range []GovernedRecall{
		func(context.Context, Binding, json.RawMessage) (VerifiedRecall, error) {
			return VerifiedRecall{}, errors.New("sensitive diagnostic")
		},
		func(ctx context.Context, _ Binding, _ json.RawMessage) (VerifiedRecall, error) {
			<-ctx.Done()
			return VerifiedRecall{Binding: testBinding, Verified: true, Result: recallFixture(emptyRecall)}, nil
		},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		tool := NewPromptContextTool(testBinding, callback)
		result, err := tool.Handler(ctx, json.RawMessage(`{"query":"query"}`))
		cancel()
		if err != nil || decodeNativeTool(t, result).Decision != "block" {
			t.Fatal("callback error/late result failed open")
		}
	}
}

func decodeNativeTool(t *testing.T, result mcp.Result) output {
	t.Helper()
	if result.IsError || len(result.Content) != 1 || result.Content[0].Type != "text" || len(result.Content[0].Text) > MaxOutputBytes {
		t.Fatalf("native hook JSON not available as ordinary bounded MCP text: %+v", result)
	}
	var native output
	if json.Unmarshal([]byte(result.Content[0].Text), &native) != nil || (native.Decision == "" && native.Context == nil) {
		t.Fatalf("invalid native output: %s", result.Content[0].Text)
	}
	if strings.Contains(result.Content[0].Text, "sensitive diagnostic") {
		t.Fatal("raw operational error leaked")
	}
	return native
}
