// Package mcp implements MCP's newline-delimited JSON-RPC stdio transport.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// ProtocolVersion is the MCP revision implemented by this transport.
const ProtocolVersion = "2025-11-25"

// Tool is an immutable registration. Handlers must honor context cancellation
// and must not write to the protocol output. New copies schema bytes.
type Tool struct {
	Name        string                                                 `json:"name"`
	Description string                                                 `json:"description,omitempty"`
	InputSchema json.RawMessage                                        `json:"inputSchema"`
	Handler     func(context.Context, json.RawMessage) (Result, error) `json:"-"`
	// Failure optionally renders schema and execution failures in the domain's wire format.
	Failure func(FailureKind) Result `json:"-"`
}

// FailureKind identifies a tool-level failure without exposing internal errors.
type FailureKind string

const (
	InvalidArguments FailureKind = "invalid_arguments"
	ExecutionFailed  FailureKind = "execution_failed"
)

// Content is text returned by a tool. Domain envelopes can be serialized in Text.
type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Result separates domain/tool failures from JSON-RPC protocol errors.
type Result struct {
	Content           []Content      `json:"content"`
	StructuredContent map[string]any `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
}

type registered struct {
	tool   Tool
	schema *jsonschema.Schema
}

// Server is an immutable registry, safe to use for independent sessions.
type Server struct {
	version string
	tools   []Tool
	byName  map[string]registered
}

var toolName = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,128}$`)

// New validates and snapshots registrations. An empty registry advertises no
// tools; domain tools are supplied by the caller, never fabricated here.
func New(version string, tools []Tool) (*Server, error) {
	if version == "" {
		return nil, fmt.Errorf("MCP server version is required")
	}
	s := &Server{version: version, tools: make([]Tool, 0, len(tools)), byName: make(map[string]registered)}
	for _, tool := range tools {
		if !toolName.MatchString(tool.Name) || tool.Handler == nil {
			return nil, fmt.Errorf("invalid MCP tool registration %q", tool.Name)
		}
		if _, ok := s.byName[tool.Name]; ok {
			return nil, fmt.Errorf("duplicate MCP tool %q", tool.Name)
		}
		tool.InputSchema = bytes.Clone(tool.InputSchema)
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(tool.InputSchema))
		if err != nil {
			return nil, fmt.Errorf("tool %s schema: %w", tool.Name, err)
		}
		object, ok := doc.(map[string]any)
		if !ok || object["type"] != "object" {
			return nil, fmt.Errorf("tool %s input schema must have object type", tool.Name)
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource("urn:serenity:mcp:input", doc); err != nil {
			return nil, fmt.Errorf("tool %s schema: %w", tool.Name, err)
		}
		schema, err := c.Compile("urn:serenity:mcp:input")
		if err != nil {
			return nil, fmt.Errorf("tool %s schema: %w", tool.Name, err)
		}
		s.tools = append(s.tools, tool)
		s.byName[tool.Name] = registered{tool, schema}
	}
	return s, nil
}
