package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// newTestConn wires an in-memory MCP server (two tools: a read-only "peek"
// and an untagged "run") to a client Conn under server name "demo".
func newTestConn(t *testing.T) *Conn {
	t.Helper()
	ctx := context.Background()

	server := sdk.NewServer(&sdk.Implementation{Name: "demo", Version: "v1"}, nil)
	server.AddTool(&sdk.Tool{
		Name: "peek", Description: "read-only peek",
		InputSchema: &jsonschema.Schema{Type: "object"},
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true},
	}, func(_ context.Context, _ *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "peeked"}}}, nil
	})
	server.AddTool(&sdk.Tool{
		Name: "run", Description: "untagged",
		InputSchema: &jsonschema.Schema{Type: "object"},
	}, func(_ context.Context, _ *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
		return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "ran it"}}, IsError: true}, nil
	})

	st, ct := sdk.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	client := sdk.NewClient(&sdk.Implementation{Name: "cli", Version: "v1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	conn := NewConn("demo", cs)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// TestMCPHelperProcess is a real child-process MCP server used by the stdio
// production-wiring test. It is inert in the ordinary test process.
func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MCP_HELPER") != "1" {
		return
	}
	server := richServer()
	if err := server.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		t.Fatal(err)
	}
}

func richServer() *sdk.Server {
	server := sdk.NewServer(&sdk.Implementation{Name: "rich", Version: "v1"}, nil)
	server.AddTool(&sdk.Tool{Name: "rich_result", Description: "structured image result",
		InputSchema: &jsonschema.Schema{Type: "object"},
		Annotations: &sdk.ToolAnnotations{ReadOnlyHint: true}},
		func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return &sdk.CallToolResult{
				Content: []sdk.Content{
					&sdk.TextContent{Text: "hello"},
					&sdk.ImageContent{Data: []byte("AQI="), MIMEType: "image/png"},
				},
				StructuredContent: map[string]any{"answer": 42},
			}, nil
		})
	server.AddResource(&sdk.Resource{URI: "memory://hello", Name: "hello", MIMEType: "text/plain"},
		func(context.Context, *sdk.ReadResourceRequest) (*sdk.ReadResourceResult, error) {
			return &sdk.ReadResourceResult{Contents: []*sdk.ResourceContents{{
				URI: "memory://hello", MIMEType: "text/plain", Text: "resource body",
			}}}, nil
		})
	server.AddPrompt(&sdk.Prompt{Name: "welcome", Description: "welcome prompt"},
		func(context.Context, *sdk.GetPromptRequest) (*sdk.GetPromptResult, error) {
			return &sdk.GetPromptResult{Description: "rendered", Messages: []*sdk.PromptMessage{{
				Role: sdk.Role("user"), Content: &sdk.TextContent{Text: "prompt body"},
			}}}, nil
		})
	return server
}

func TestConnectStdioPreservesRichResultsResourcesAndPrompts(t *testing.T) {
	t.Setenv("MCP_HELPER_ENABLE", "1")
	m, err := Connect(context.Background(), []ServerConfig{{
		Name: "stdio", Transport: "stdio",
		Command: []string{os.Args[0], "-test.run=^TestMCPHelperProcess$"},
		EnvFrom: map[string]string{"GO_WANT_MCP_HELPER": "MCP_HELPER_ENABLE"},
	}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()

	tools, err := m.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"mcp__stdio__rich_result": true, "mcp__stdio__resources_list": true,
		"mcp__stdio__resources_read": true, "mcp__stdio__prompts_list": true,
		"mcp__stdio__prompts_get": true,
	}
	for _, tool := range tools {
		delete(want, tool.Name)
	}
	if len(want) > 0 {
		t.Fatalf("missing discovered MCP capabilities: %v", want)
	}

	payload, isErr, err := m.Call(context.Background(), "mcp__stdio__rich_result", json.RawMessage(`{}`))
	if err != nil || isErr {
		t.Fatalf("rich call: %s / %v / %v", payload, isErr, err)
	}
	for _, fragment := range []string{`"structuredContent":{"answer":42}`, `"type":"image"`, `"mimeType":"image/png"`} {
		if !strings.Contains(string(payload), fragment) {
			t.Errorf("rich result lost %s: %s", fragment, payload)
		}
	}
	resource, _, err := m.Call(context.Background(), "mcp__stdio__resources_read", json.RawMessage(`{"uri":"memory://hello"}`))
	if err != nil || !strings.Contains(string(resource), "resource body") {
		t.Fatalf("resource read: %s / %v", resource, err)
	}
	prompt, _, err := m.Call(context.Background(), "mcp__stdio__prompts_get", json.RawMessage(`{"name":"welcome"}`))
	if err != nil || !strings.Contains(string(prompt), "prompt body") {
		t.Fatalf("prompt get: %s / %v", prompt, err)
	}
}

func TestConnectHTTPAuthListChangedAndReconnect(t *testing.T) {
	server := richServer()
	var authFailures atomic.Int32
	handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer oauth-value" || r.Header.Get("X-MCP-Test") != "header-value" {
			authFailures.Add(1)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer httpServer.Close()
	t.Setenv("MCP_OAUTH", "oauth-value")
	t.Setenv("MCP_HEADER", "header-value")
	m, err := Connect(context.Background(), []ServerConfig{{
		Name: "remote", Transport: "http", URL: httpServer.URL,
		HeadersFromEnv: map[string]string{"X-MCP-Test": "MCP_HEADER"},
		OAuth:          &OAuthConfig{AccessTokenEnv: "MCP_OAUTH"},
	}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	if _, err := m.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	if authFailures.Load() != 0 {
		t.Fatalf("authorized requests rejected: %d", authFailures.Load())
	}

	server.AddTool(&sdk.Tool{Name: "dynamic", InputSchema: &jsonschema.Schema{Type: "object"}},
		func(context.Context, *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: "new"}}}, nil
		})
	deadline := time.Now().Add(3 * time.Second)
	for !m.Changed() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !m.Changed() {
		t.Fatal("tools/list_changed notification was not observed")
	}
	tools, err := m.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range tools {
		found = found || tool.Name == "mcp__remote__dynamic"
	}
	if !found {
		t.Fatal("dynamic tool absent after list_changed refresh")
	}

	conn := m.conns["remote"]
	conn.mu.Lock()
	old := conn.session
	conn.mu.Unlock()
	if err := old.Close(); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for !conn.Changed() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := m.Discover(context.Background()); err != nil {
		t.Fatalf("discover after disconnected session did not reconnect: %v", err)
	}
}

// Discovery: fully-qualified names, and the class default — read-only → read,
// untagged → execute (the conservative default, S5.1).
func TestDiscoverNamingAndClass(t *testing.T) {
	conn := newTestConn(t)
	tools, err := conn.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]DiscoveredTool{}
	for _, tl := range tools {
		byName[tl.Name] = tl
	}
	peek, ok := byName["mcp__demo__peek"]
	if !ok || peek.Class != classRead {
		t.Errorf("peek = %+v, want read class under mcp__demo__peek", peek)
	}
	run, ok := byName["mcp__demo__run"]
	if !ok || run.Class != classExecute {
		t.Errorf("run = %+v, want execute class (untagged default)", run)
	}
	if len(peek.InputSchema) == 0 || !strings.Contains(string(peek.InputSchema), "object") {
		t.Errorf("peek schema not captured: %s", peek.InputSchema)
	}
}

// Call dispatch renders content and surfaces the tool-level error flag.
func TestConnCall(t *testing.T) {
	conn := newTestConn(t)
	payload, isErr, err := conn.Call(context.Background(), "peek", nil)
	if err != nil {
		t.Fatal(err)
	}
	if isErr || !strings.Contains(string(payload), "peeked") {
		t.Errorf("peek call = %s (isErr=%v)", payload, isErr)
	}
	// The untagged tool returns an MCP tool-level error → model-visible.
	_, isErr, err = conn.Call(context.Background(), "run", json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if !isErr {
		t.Errorf("run should report a tool-level error")
	}
}

// Manager: union discovery + allowed_tools narrowing, and Call defense in
// depth — a non-allowed tool is rejected even if invoked directly.
func TestManagerAllowedNarrowing(t *testing.T) {
	m := NewManager()
	if err := m.Add(newTestConn(t)); err != nil {
		t.Fatal(err)
	}
	// Narrow to only the read tool.
	m.SetAllowed([]string{"mcp__demo__peek"})

	tools, err := m.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Name != "mcp__demo__peek" {
		t.Fatalf("narrowed discovery = %+v, want only peek", tools)
	}

	// The un-advertised tool must be rejected at Call even if the model
	// fabricates it (negative test, S5.1).
	if _, _, err := m.Call(context.Background(), "mcp__demo__run", nil); err == nil ||
		!strings.Contains(err.Error(), "not permitted") {
		t.Errorf("call to narrowed-out tool = %v, want 'not permitted'", err)
	}
	// The allowed tool still routes and executes.
	payload, _, err := m.Call(context.Background(), "mcp__demo__peek", nil)
	if err != nil || !strings.Contains(string(payload), "peeked") {
		t.Errorf("allowed call failed: %s / %v", payload, err)
	}
}

func TestSplitName(t *testing.T) {
	cases := map[string][2]string{
		"mcp__srv__tool":       {"srv", "tool"},
		"mcp__srv__ns__nested": {"srv", "ns__nested"}, // only first sep splits
	}
	for in, want := range cases {
		s, tl, ok := SplitName(in)
		if !ok || s != want[0] || tl != want[1] {
			t.Errorf("SplitName(%q) = (%q,%q,%v), want %v", in, s, tl, ok, want)
		}
	}
	for _, bad := range []string{"read_file", "mcp__", "mcp__srv", "mcp__srv__"} {
		if _, _, ok := SplitName(bad); ok {
			t.Errorf("SplitName(%q) should fail", bad)
		}
	}
}

func TestDuplicateServerRejected(t *testing.T) {
	m := NewManager()
	if err := m.Add(newTestConn(t)); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(newTestConn(t)); err == nil {
		t.Error("duplicate server name must be rejected")
	}
}
