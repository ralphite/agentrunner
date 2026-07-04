// Package mcp adapts external Model Context Protocol servers into the
// harness's normalized tool face (S5.1). MCP server LIFECYCLE is out-of-band:
// connections are runtime state, never event-sourced — only the discovered
// tool SCHEMAS enter the event log (so a resumed run knows its tool face and
// can re-connect + re-validate against the journaled schemas). Tools are
// exposed under the fully-qualified name `mcp__<server>__<tool>`, and an
// untagged tool defaults to the most conservative execute class.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// namePrefix namespaces every MCP tool away from built-in tools.
const namePrefix = "mcp__"

// Tool classes (mirror internal/tool classes; kept as strings to avoid a
// dependency cycle — the loop maps these onto its registry).
const (
	classRead    = "read"
	classExecute = "execute"
)

// QualifiedName builds the fully-qualified MCP tool name.
func QualifiedName(server, tool string) string {
	return namePrefix + server + "__" + tool
}

// SplitName parses a fully-qualified MCP tool name back into (server, tool).
// The tool half may itself contain "__"; only the first separator splits.
func SplitName(qualified string) (server, tool string, ok bool) {
	rest, found := strings.CutPrefix(qualified, namePrefix)
	if !found {
		return "", "", false
	}
	server, tool, found = strings.Cut(rest, "__")
	if !found || server == "" || tool == "" {
		return "", "", false
	}
	return server, tool, true
}

// DiscoveredTool is a normalized MCP tool ready to enter the tool face.
type DiscoveredTool struct {
	Server      string          `json:"server"`
	Tool        string          `json:"tool"` // bare name as the server knows it
	Name        string          `json:"name"` // fully-qualified mcp__server__tool
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	Class       string          `json:"class"` // read | execute
}

// clientSession is the slice of the SDK session the manager needs; an
// interface so tests can substitute a fake without a live transport.
type clientSession interface {
	ListTools(context.Context, *sdk.ListToolsParams) (*sdk.ListToolsResult, error)
	CallTool(context.Context, *sdk.CallToolParams) (*sdk.CallToolResult, error)
	Close() error
}

// Conn is one connected MCP server (lifecycle out-of-band).
type Conn struct {
	server  string
	session clientSession
}

// NewConn wraps an already-connected SDK client session under a server name.
// Connecting the transport (stdio command, sse, in-memory) is the caller's
// job — that lifecycle lives outside the event log.
func NewConn(server string, session *sdk.ClientSession) *Conn {
	return &Conn{server: server, session: session}
}

// Discover lists the server's tools, normalized and fully-qualified.
func (c *Conn) Discover(ctx context.Context) ([]DiscoveredTool, error) {
	res, err := c.session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp %q: list tools: %w", c.server, err)
	}
	out := make([]DiscoveredTool, 0, len(res.Tools))
	for _, t := range res.Tools {
		schema, _ := json.Marshal(t.InputSchema)
		out = append(out, DiscoveredTool{
			Server: c.server, Tool: t.Name, Name: QualifiedName(c.server, t.Name),
			Description: t.Description, InputSchema: schema, Class: classOf(t.Annotations),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Call invokes a bare tool on this server and renders the result as a JSON
// payload. isError reflects the MCP tool-level error flag (a failed tool is a
// model-visible result, not a harness failure — same contract as built-ins).
func (c *Conn) Call(ctx context.Context, tool string, args json.RawMessage) (payload json.RawMessage, isError bool, err error) {
	var arguments any
	if len(args) > 0 {
		arguments = json.RawMessage(args)
	}
	res, err := c.session.CallTool(ctx, &sdk.CallToolParams{Name: tool, Arguments: arguments})
	if err != nil {
		return nil, false, fmt.Errorf("mcp %q: call %q: %w", c.server, tool, err)
	}
	payload, merr := json.Marshal(map[string]any{"content": renderContent(res)})
	if merr != nil {
		return nil, false, merr
	}
	return payload, res.IsError, nil
}

// Close ends the session (out-of-band lifecycle).
func (c *Conn) Close() error {
	return c.session.Close()
}

// classOf maps MCP annotations to a permission class. A read-only tool is
// read-class; everything else — including untagged tools — is execute, the
// most conservative default (S5.1).
func classOf(ann *sdk.ToolAnnotations) string {
	if ann != nil && ann.ReadOnlyHint {
		return classRead
	}
	return classExecute
}

// renderContent flattens an MCP result's content blocks into a single string
// (text blocks concatenated); non-text blocks are summarized by type.
func renderContent(res *sdk.CallToolResult) string {
	var b strings.Builder
	for _, block := range res.Content {
		if tc, ok := block.(*sdk.TextContent); ok {
			b.WriteString(tc.Text)
		} else {
			fmt.Fprintf(&b, "[%T]", block)
		}
	}
	return b.String()
}

// Manager holds several server connections and applies spec-level
// `allowed_tools` narrowing across all of them.
type Manager struct {
	conns   map[string]*Conn
	allowed map[string]bool // nil = allow all; else the fully-qualified whitelist
}

// NewManager builds an empty manager.
func NewManager() *Manager {
	return &Manager{conns: map[string]*Conn{}}
}

// Add registers a connected server. A duplicate server name is an error —
// names namespace tools, so a collision would make dispatch ambiguous.
func (m *Manager) Add(c *Conn) error {
	if _, dup := m.conns[c.server]; dup {
		return fmt.Errorf("mcp: duplicate server name %q", c.server)
	}
	m.conns[c.server] = c
	return nil
}

// SetAllowed narrows the advertised/callable set to the given fully-qualified
// names (spec `allowed_tools`). Empty/nil leaves everything allowed.
func (m *Manager) SetAllowed(names []string) {
	if len(names) == 0 {
		m.allowed = nil
		return
	}
	m.allowed = make(map[string]bool, len(names))
	for _, n := range names {
		m.allowed[n] = true
	}
}

func (m *Manager) isAllowed(name string) bool {
	return m.allowed == nil || m.allowed[name]
}

// Discover returns the union of all servers' tools, filtered by allowed_tools,
// sorted by qualified name (stable tool face → stable prompt prefix).
func (m *Manager) Discover(ctx context.Context) ([]DiscoveredTool, error) {
	var out []DiscoveredTool
	servers := make([]string, 0, len(m.conns))
	for name := range m.conns {
		servers = append(servers, name)
	}
	sort.Strings(servers)
	for _, name := range servers {
		tools, err := m.conns[name].Discover(ctx)
		if err != nil {
			return nil, err
		}
		for _, t := range tools {
			if m.isAllowed(t.Name) {
				out = append(out, t)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Call routes a fully-qualified tool call to its server. A name outside the
// allowed set is rejected here too (defense in depth): even if a model
// fabricates a call to an un-advertised MCP tool, it never executes.
func (m *Manager) Call(ctx context.Context, qualified string, args json.RawMessage) (json.RawMessage, bool, error) {
	if !m.isAllowed(qualified) {
		return nil, false, fmt.Errorf("mcp: tool %q not permitted", qualified)
	}
	server, tool, ok := SplitName(qualified)
	if !ok {
		return nil, false, fmt.Errorf("mcp: %q is not a fully-qualified mcp tool name", qualified)
	}
	conn, ok := m.conns[server]
	if !ok {
		return nil, false, fmt.Errorf("mcp: no connected server %q", server)
	}
	return conn.Call(ctx, tool, args)
}

// Close tears down every connection (out-of-band).
func (m *Manager) Close() error {
	var first error
	for _, c := range m.conns {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
