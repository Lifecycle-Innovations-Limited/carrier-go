package carrier

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultMCPURL is the hosted Streamable HTTP endpoint.
	DefaultMCPURL = "https://mcp.carrier.llc/mcp"
	mcpProtocol   = "2025-11-25"
)

var acceptedProtocols = map[string]bool{
	"2025-11-25": true,
	"2025-06-18": true,
	"2025-03-26": true,
}

// MCP is a Streamable HTTP client for the Carrier MCP server.
type MCP struct {
	token     string
	baseURL   string
	auth      string
	http      *http.Client
	sessionID string
	protocol  string
	connected bool
	nextID    int
}

// MCPOption configures an MCP client.
type MCPOption func(*MCP)

// WithMCPURL overrides https://mcp.carrier.llc/mcp.
func WithMCPURL(baseURL string) MCPOption {
	return func(mcp *MCP) {
		mcp.baseURL = baseURL
	}
}

// WithMCPHTTPClient replaces the default HTTP client.
func WithMCPHTTPClient(client *http.Client) MCPOption {
	return func(mcp *MCP) {
		mcp.http = client
	}
}

// WithMCPAuth selects "token" (?TOKEN=) or "bearer" (Authorization).
func WithMCPAuth(auth string) MCPOption {
	return func(mcp *MCP) {
		mcp.auth = auth
	}
}

// NewMCP returns a client. An empty token fails before any request is sent.
// The default auth mode sends the value as the TOKEN query parameter.
func NewMCP(token string, opts ...MCPOption) (*MCP, error) {
	if strings.TrimSpace(token) == "" {
		return nil, invalidRequest("invalid_token", "An MCP token is required.")
	}
	mcp := &MCP{
		token:   token,
		baseURL: DefaultMCPURL,
		auth:    "token",
		http:    &http.Client{Timeout: defaultTimeout},
		nextID:  1,
	}
	for _, opt := range opts {
		opt(mcp)
	}
	if mcp.auth != "token" && mcp.auth != "bearer" {
		return nil, invalidRequest("invalid_auth", "MCP auth must be token or bearer.")
	}
	if strings.TrimSpace(mcp.baseURL) == "" {
		mcp.baseURL = DefaultMCPURL
	}
	if mcp.http == nil {
		mcp.http = &http.Client{Timeout: 30 * time.Second}
	}
	return mcp, nil
}

// Connect initializes a session and sends notifications/initialized.
func (m *MCP) Connect(ctx context.Context) (map[string]any, error) {
	result, err := m.rpc(ctx, "initialize", map[string]any{
		"protocolVersion": mcpProtocol,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"clientInfo":      map[string]any{"name": "carrier-go", "version": version},
	}, true)
	if err != nil {
		return nil, err
	}
	payload, ok := result.(map[string]any)
	if !ok {
		return nil, apiFailure(200, "invalid_response", "MCP initialize did not return an object.", "api_error")
	}
	negotiated, _ := payload["protocolVersion"].(string)
	if !acceptedProtocols[negotiated] {
		return nil, invalidRequest("unsupported_protocol", "MCP server negotiated "+negotiated+".")
	}
	m.protocol = negotiated
	if err := m.notify(ctx, "notifications/initialized"); err != nil {
		return nil, err
	}
	m.connected = true
	return payload, nil
}

// ListTools returns the tools advertised for this session.
func (m *MCP) ListTools(ctx context.Context) ([]any, error) {
	if err := m.ensure(ctx); err != nil {
		return nil, err
	}
	result, err := m.rpc(ctx, "tools/list", map[string]any{}, false)
	if err != nil {
		return nil, err
	}
	payload, ok := result.(map[string]any)
	tools, toolsOK := payload["tools"].([]any)
	if !ok || !toolsOK {
		return nil, apiFailure(200, "invalid_response", "MCP tools/list did not return tools.", "api_error")
	}
	return tools, nil
}

// CallTool calls one MCP tool and returns the tool result object.
func (m *MCP) CallTool(ctx context.Context, name string, arguments map[string]any) (map[string]any, error) {
	if err := m.ensure(ctx); err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, invalidRequest("invalid_tool", "An MCP tool name is required.")
	}
	if arguments == nil {
		arguments = map[string]any{}
	}
	result, err := m.rpc(ctx, "tools/call", map[string]any{"name": name, "arguments": arguments}, false)
	if err != nil {
		return nil, err
	}
	payload, ok := result.(map[string]any)
	if !ok {
		return nil, apiFailure(200, "invalid_response", "MCP tools/call did not return an object.", "api_error")
	}
	return payload, nil
}

// Request calls any MCP method. The session is opened on the first call.
func (m *MCP) Request(ctx context.Context, method string, params map[string]any) (any, error) {
	if method != "initialize" {
		if err := m.ensure(ctx); err != nil {
			return nil, err
		}
	}
	if params == nil {
		params = map[string]any{}
	}
	return m.rpc(ctx, method, params, false)
}

func (m *MCP) ensure(ctx context.Context) error {
	if m.connected {
		return nil
	}
	_, err := m.Connect(ctx)
	return err
}

func (m *MCP) notify(ctx context.Context, method string) error {
	_, err := m.post(ctx, map[string]any{"jsonrpc": "2.0", "method": method}, true, false)
	return err
}

func (m *MCP) rpc(ctx context.Context, method string, params map[string]any, handshake bool) (any, error) {
	message := map[string]any{"jsonrpc": "2.0", "id": m.nextID, "method": method, "params": params}
	m.nextID++
	return m.post(ctx, message, false, handshake)
}

func (m *MCP) post(ctx context.Context, message map[string]any, notify bool, handshake bool) (any, error) {
	payload, err := json.Marshal(message)
	if err != nil {
		return nil, invalidRequest("invalid_body", "MCP request could not be encoded.")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint(), bytes.NewReader(payload))
	if err != nil {
		return nil, apiFailure(0, "network_error", err.Error(), "api_error")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("User-Agent", "carrier-go/"+version)
	if m.auth == "bearer" {
		req.Header.Set("Authorization", "Bearer "+m.token)
	}
	if m.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", m.sessionID)
	}
	if m.protocol != "" {
		req.Header.Set("MCP-Protocol-Version", m.protocol)
	}
	resp, err := m.http.Do(req)
	if err != nil {
		return nil, apiFailure(0, "network_error", err.Error(), "api_error")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, apiFailure(0, "network_error", err.Error(), "api_error")
	}
	if handshake {
		if session := resp.Header.Get("Mcp-Session-Id"); session != "" {
			m.sessionID = session
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		text := strings.TrimSpace(string(raw))
		if text == "" {
			text = "MCP request failed."
		}
		if len(text) > 300 {
			text = text[:300]
		}
		return nil, apiFailure(resp.StatusCode, "mcp_error", text, "api_error")
	}
	if notify && len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	return decodeMCP(raw, resp.Header.Get("Content-Type"))
}

func (m *MCP) endpoint() string {
	if m.auth != "token" {
		return m.baseURL
	}
	parsed, err := url.Parse(m.baseURL)
	if err != nil {
		return m.baseURL
	}
	query := parsed.Query()
	query.Set("TOKEN", m.token)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func decodeMCP(raw []byte, contentType string) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	if strings.Contains(contentType, "text/event-stream") {
		return decodeSSE(raw)
	}
	var message map[string]any
	if err := json.Unmarshal(raw, &message); err != nil {
		return nil, apiFailure(200, "invalid_response", "MCP returned a body that is not JSON.", "api_error")
	}
	return rpcPayload(message)
}

func decodeSSE(raw []byte) (any, error) {
	for _, block := range bytes.Split(raw, []byte("\n\n")) {
		for _, line := range bytes.Split(block, []byte("\n")) {
			if !bytes.HasPrefix(line, []byte("data:")) {
				continue
			}
			data := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
			if len(data) == 0 || bytes.Equal(data, []byte("[DONE]")) {
				continue
			}
			var message map[string]any
			if err := json.Unmarshal(data, &message); err != nil {
				continue
			}
			if _, ok := message["result"]; ok {
				return rpcPayload(message)
			}
			if _, ok := message["error"]; ok {
				return rpcPayload(message)
			}
		}
	}
	return nil, apiFailure(200, "invalid_response", "MCP event stream did not include a result.", "api_error")
}

func rpcPayload(message map[string]any) (any, error) {
	if raw, ok := message["error"].(map[string]any); ok {
		text, _ := raw["message"].(string)
		if text == "" {
			text = "MCP request failed."
		}
		return nil, apiFailure(200, "mcp_error", text, "api_error")
	}
	return message["result"], nil
}
