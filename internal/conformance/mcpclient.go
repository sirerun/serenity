package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/sirerun/serenity/internal/server/mcp"
)

// MCPClient is a minimal MEMORY_VERBS v1 client over MCP's Streamable HTTP
// transport (internal/server/mcp.HTTPHandler, T4.21): just enough to drive
// initialize and tools/call for this package's own conformance replay,
// not a general-purpose MCP SDK. gbrain's own StreamableHTTPClientTransport
// remains the authoritative external client (T4.14); this one exists so
// `serenity protocol conformance` can replay MEMORY_VERBS cases without a
// second toolchain.
type MCPClient struct {
	httpClient *http.Client
	baseURL    string
	token      string
	sessionID  string
	nextID     int
}

// NewMCPClient builds a client against baseURL (e.g. "http://127.0.0.1:PORT/mcp").
// token is sent as the daemon's own bearer credential (RFC 0001 §14) --
// the same Authorization header every other Serenity HTTP route requires.
func NewMCPClient(httpClient *http.Client, baseURL, token string) *MCPClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &MCPClient{httpClient: httpClient, baseURL: baseURL, token: token}
}

type jsonrpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  any             `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Initialize performs MCP's handshake: initialize, then the
// notifications/initialized notification, capturing the Mcp-Session-Id
// this client's subsequent CallTool requests must echo.
func (c *MCPClient) Initialize(ctx context.Context, clientName, clientVersion string) error {
	// "initialize" is a real request expecting a reply (unlike the
	// notification that follows it below), so it needs a JSON-RPC id --
	// distinct from, and sent before, the transport's own Mcp-Session-Id
	// header, which the server only assigns once this call succeeds.
	result, sessionID, err := c.post(ctx, "0", "initialize", map[string]any{
		"protocolVersion": mcp.ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]string{"name": clientName, "version": clientVersion},
	})
	if err != nil {
		return fmt.Errorf("conformance: mcp initialize: %w", err)
	}
	if sessionID == "" {
		return fmt.Errorf("conformance: mcp initialize: server did not assign a %s", mcp.SessionIDHeader)
	}
	var negotiated struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(result, &negotiated); err != nil || negotiated.ProtocolVersion != mcp.ProtocolVersion {
		return fmt.Errorf("conformance: mcp initialize: unexpected result %s", result)
	}
	c.sessionID = sessionID
	if _, _, err := c.post(ctx, "", "notifications/initialized", map[string]any{}); err != nil {
		return fmt.Errorf("conformance: mcp notifications/initialized: %w", err)
	}
	return nil
}

// CallTool invokes verb with args (tools/call) and returns the decoded
// domain response body plus whether MCP marked it isError -- the same
// split pinned_cases_test.go's own runPinnedCases makes between the
// JSON-RPC envelope (never allowed to fail here; a JSON-RPC-level error is
// returned as a Go error, distinct from a domain/tool failure) and the
// tool result's own isError flag.
func (c *MCPClient) CallTool(ctx context.Context, verb string, args map[string]any) (body map[string]any, isError bool, err error) {
	c.nextID++
	result, _, err := c.post(ctx, fmt.Sprintf("%d", c.nextID), "tools/call", map[string]any{
		"name": verb, "arguments": args,
	})
	if err != nil {
		return nil, false, err
	}
	var toolResult mcp.Result
	if err := json.Unmarshal(result, &toolResult); err != nil {
		return nil, false, fmt.Errorf("conformance: mcp tools/call %s: unmarshal result: %w", verb, err)
	}
	if len(toolResult.Content) != 1 || toolResult.Content[0].Type != "text" {
		return nil, false, fmt.Errorf("conformance: mcp tools/call %s: unexpected content shape: %s", verb, result)
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(toolResult.Content[0].Text)))
	if err := decoder.Decode(&body); err != nil {
		return nil, false, fmt.Errorf("conformance: mcp tools/call %s: domain body is not JSON: %w", verb, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, false, fmt.Errorf("conformance: mcp tools/call %s: domain body has trailing content", verb)
	}
	return body, toolResult.IsError, nil
}

// post sends one JSON-RPC request. An empty id sends a notification (no
// reply expected -- the server answers 202 with no body); a non-empty id
// sends a request and returns its raw "result" plus, on the very first
// call (initialize), the session id the server assigned.
func (c *MCPClient) post(ctx context.Context, id, method string, params any) (result json.RawMessage, sessionID string, err error) {
	env := jsonrpcEnvelope{JSONRPC: "2.0", Method: method, Params: params}
	if id != "" {
		env.ID = json.RawMessage(fmt.Sprintf("%q", id))
	}
	body, err := json.Marshal(env)
	if err != nil {
		return nil, "", fmt.Errorf("marshal %s: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("build %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.sessionID != "" {
		req.Header.Set(mcp.SessionIDHeader, c.sessionID)
		req.Header.Set(mcp.ProtocolVersionHeader, mcp.ProtocolVersion)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("%s: read response: %w", method, err)
	}
	if id == "" {
		// Notification: 202 with no JSON-RPC envelope.
		if resp.StatusCode != http.StatusAccepted {
			return nil, "", fmt.Errorf("%s: status %d: %s", method, resp.StatusCode, respBody)
		}
		return nil, "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("%s: status %d: %s", method, resp.StatusCode, respBody)
	}
	var envResp jsonrpcEnvelope
	if err := json.Unmarshal(respBody, &envResp); err != nil {
		return nil, "", fmt.Errorf("%s: unmarshal envelope: %w", method, err)
	}
	if envResp.Error != nil {
		return nil, "", fmt.Errorf("%s: jsonrpc error %d: %s", method, envResp.Error.Code, envResp.Error.Message)
	}
	return envResp.Result, resp.Header.Get(mcp.SessionIDHeader), nil
}
