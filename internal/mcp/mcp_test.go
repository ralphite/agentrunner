package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

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
