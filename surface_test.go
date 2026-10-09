package carrier

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestPublicModulePath(t *testing.T) {
	if version != "0.1.10" {
		t.Fatalf("version %s", version)
	}
	mod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mod), "module github.com/Lifecycle-Innovations-Limited/carrier-go\n") {
		t.Fatalf("module path %s", mod)
	}
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(readme)
	if !strings.Contains(text, "go get github.com/Lifecycle-Innovations-Limited/carrier-go@latest") {
		t.Fatal("README missing the public install")
	}
	if strings.Contains(text, "carrier.llc/sdks/go") {
		t.Fatal("README still names the private module path")
	}
}

func TestCallFillsPathAndRejectsInjection(t *testing.T) {
	var gotPath, gotAuth string
	client, err := New("ak_test", WithBaseURL("https://api.test"))
	if err != nil {
		t.Fatal(err)
	}
	client.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotPath = r.URL.String()
		gotAuth = r.Header.Get("Authorization")
		return jsonResponse(http.StatusOK, map[string]any{"object": "list", "data": []any{}}), nil
	})}

	if _, err := client.Call(context.Background(), "esim_status_per_account", CallParams{Path: map[string]string{"accountId": "acct_1"}}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "https://api.test/v1/accounts/acct_1/esim-status" {
		t.Fatalf("path %s", gotPath)
	}
	if gotAuth != "Bearer ak_test" {
		t.Fatalf("auth %s", gotAuth)
	}
	_, err = client.Call(context.Background(), "esim_status_per_account", CallParams{Path: map[string]string{"accountId": "../admin"}})
	if err == nil || !strings.Contains(err.Error(), "not a valid id") {
		t.Fatalf("injection err %v", err)
	}
}

func TestCallRequiresIdempotency(t *testing.T) {
	var gotKey string
	var gotBody []byte
	client, err := New("ak_test", WithBaseURL("https://api.test"))
	if err != nil {
		t.Fatal(err)
	}
	client.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotKey = r.Header.Get("Idempotency-Key")
		gotBody, _ = io.ReadAll(r.Body)
		return jsonResponse(http.StatusCreated, map[string]any{"id": "cni_1"}), nil
	})}
	_, err = client.Call(context.Background(), "create_connectivity_intent", CallParams{Body: map[string]any{"plan": "test_global_1gb"}})
	if err == nil || !strings.Contains(err.Error(), "idempotency_key is required") {
		t.Fatalf("missing key %v", err)
	}
	if _, err := client.Call(context.Background(), "create_connectivity_intent", CallParams{
		Body:           map[string]any{"plan": "test_global_1gb"},
		IdempotencyKey: "order_1",
	}); err != nil {
		t.Fatal(err)
	}
	if gotKey != "order_1" {
		t.Fatalf("key %s", gotKey)
	}
	if !strings.Contains(string(gotBody), "test_global_1gb") {
		t.Fatalf("body %s", gotBody)
	}
}

func TestUnknownOperationDoesNotCallNetwork(t *testing.T) {
	called := false
	client, err := New("ak_test")
	if err != nil {
		t.Fatal(err)
	}
	client.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		called = true
		return jsonResponse(http.StatusOK, map[string]any{}), nil
	})}
	_, err = client.Call(context.Background(), "not_a_real_operation", CallParams{})
	apiErr, ok := err.(*Error)
	if !ok || apiErr.Code != "unknown_operation" || called {
		t.Fatalf("err %v called %v", err, called)
	}
}

func TestMCPSession(t *testing.T) {
	var calls []string
	mcp, err := NewMCP("ocs-token", WithMCPURL("https://mcp.test/mcp"), WithMCPHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls = append(calls, r.URL.RawQuery+" "+r.Header.Get("Mcp-Session-Id"))
		body, _ := io.ReadAll(r.Body)
		var message map[string]any
		_ = json.Unmarshal(body, &message)
		switch message["method"] {
		case "initialize":
			return mcpResponse(http.StatusOK, "ses_1", map[string]any{"protocolVersion": "2025-11-25"}), nil
		case "notifications/initialized":
			return &http.Response{StatusCode: http.StatusAccepted, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
		case "tools/list":
			return mcpResponse(http.StatusOK, "", map[string]any{"tools": []any{map[string]any{"name": "list_subscribers", "inputSchema": map[string]any{"type": "object"}}}}), nil
		default:
			return mcpResponse(http.StatusOK, "", map[string]any{"content": []any{map[string]any{"type": "text", "text": "ok"}}}), nil
		}
	})}))
	if err != nil {
		t.Fatal(err)
	}
	tools, err := mcp.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tools[0].(map[string]any)["name"] != "list_subscribers" {
		t.Fatalf("tools %v", tools)
	}
	if !strings.Contains(calls[0], "TOKEN=ocs-token") {
		t.Fatalf("query %s", calls[0])
	}
	if !strings.Contains(calls[2], "ses_1") {
		t.Fatalf("session %v", calls)
	}
	result, err := mcp.CallTool(context.Background(), "list_subscribers", map[string]any{"limit": 1})
	if err != nil {
		t.Fatal(err)
	}
	content := result["content"].([]any)
	if content[0].(map[string]any)["text"] != "ok" {
		t.Fatalf("result %v", result)
	}
}

func TestMCPRejectsUnknownProtocol(t *testing.T) {
	mcp, err := NewMCP("tok", WithMCPHTTPClient(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return mcpResponse(http.StatusOK, "", map[string]any{"protocolVersion": "1999-01-01"}), nil
	})}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = mcp.Connect(context.Background())
	apiErr, ok := err.(*Error)
	if !ok || apiErr.Code != "unsupported_protocol" {
		t.Fatalf("err %v", err)
	}
}

func TestCLIArgv(t *testing.T) {
	var seen []string
	cli, err := NewCLI(WithCLIRunner(func(ctx context.Context, binary string, args []string) (CLIResult, error) {
		seen = append([]string{binary}, args...)
		return CLIResult{Stdout: "[]"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := cli.Run(context.Background(), []string{"subscribers", "list", "--limit", "5"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "[]" || strings.Join(seen, " ") != "carrier subscribers list --limit 5" {
		t.Fatalf("seen %v result %v", seen, result)
	}
	if _, err := cli.Run(context.Background(), []string{"nope"}); err == nil {
		t.Fatal("expected unknown command")
	}
	if len(seen) != 5 {
		t.Fatalf("runner called for unknown command: %v", seen)
	}
}

func TestAdaptersExecute(t *testing.T) {
	client, err := New("ak_test", WithBaseURL("https://api.test"))
	if err != nil {
		t.Fatal(err)
	}
	client.http = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/accounts" {
			t.Fatalf("path %s", r.URL.Path)
		}
		return jsonResponse(http.StatusOK, map[string]any{"object": "list", "data": []any{}}), nil
	})}
	tools, err := client.RESTTools()
	if err != nil {
		t.Fatal(err)
	}
	ops, err := Operations()
	if err != nil || len(tools) != len(ops) {
		t.Fatalf("tools %d ops %d err %v", len(tools), len(ops), err)
	}
	var list Tool
	for _, tool := range tools {
		if tool.Name == "list_reseller_accounts" {
			list = tool
		}
	}
	if _, err := list.Execute(context.Background(), map[string]any{}); err != nil {
		t.Fatal(err)
	}

	cli, err := NewCLI(WithCLIRunner(func(ctx context.Context, binary string, args []string) (CLIResult, error) {
		if strings.Join(append([]string{binary}, args...), " ") != "carrier subscribers list --limit 1" {
			t.Fatalf("argv %s %v", binary, args)
		}
		return CLIResult{Stdout: "ok"}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	cliTools, err := cli.Tools()
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range cliTools {
		if tool.Name == "cli_subscribers_list" {
			if _, err := tool.Execute(context.Background(), map[string]any{"args": []any{"--limit", "1"}}); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func jsonResponse(status int, body any) *http.Response {
	encoded, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytesReader(encoded)),
		Header:     make(http.Header),
	}
}

func mcpResponse(status int, session string, result map[string]any) *http.Response {
	encoded, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
	header := make(http.Header)
	header.Set("Content-Type", "application/json")
	if session != "" {
		header.Set("Mcp-Session-Id", session)
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytesReader(encoded)), Header: header}
}

func bytesReader(raw []byte) io.Reader {
	return strings.NewReader(string(raw))
}
