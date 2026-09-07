package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestToolFailurePreservesDomainFormat(t *testing.T) {
	for _, tc := range []struct {
		name    string
		handler func(context.Context, json.RawMessage) (Result, error)
	}{
		{"error", func(context.Context, json.RawMessage) (Result, error) {
			return Result{}, errors.New("private internal path")
		}},
		{"panic", func(context.Context, json.RawMessage) (Result, error) { panic("private internal path") }},
		{"invalid content", func(context.Context, json.RawMessage) (Result, error) {
			return Result{Content: []Content{{Type: "image"}}}, nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tool := Tool{Handler: tc.handler, Failure: func(kind FailureKind) Result {
				if kind != ExecutionFailed {
					t.Errorf("kind=%s", kind)
				}
				return Result{Content: []Content{{Type: "text", Text: `{"error":"internal","protocol_version":1}`}}}
			}}
			var got Result
			if err := json.Unmarshal(invoke(t.Context(), tool, json.RawMessage(`{}`)), &got); err != nil {
				t.Fatal(err)
			}
			if !got.IsError || len(got.Content) != 1 || got.Content[0].Text != `{"error":"internal","protocol_version":1}` {
				t.Fatalf("domain failure lost: %+v", got)
			}
		})
	}
}

func TestBrokenFailureFormatterUsesGenericFallback(t *testing.T) {
	for _, formatter := range []func(FailureKind) Result{
		func(FailureKind) Result { panic("private callback panic") },
		func(FailureKind) Result { return Result{} },
		func(FailureKind) Result { return Result{Content: []Content{{Type: "image"}}} },
	} {
		got := toolFailure(Tool{Failure: formatter}, ExecutionFailed)
		if !got.IsError || len(got.Content) != 1 || got.Content[0].Text != "Tool execution failed" {
			t.Fatalf("fallback=%+v", got)
		}
	}
}
