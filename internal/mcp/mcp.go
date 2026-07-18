// Package mcp adapts external Model Context Protocol servers into the
// harness's normalized tool face. Connections are runtime state; discovered
// schemas are the durable facts. Server annotations influence the permission
// face only and are never trusted as replay/idempotency contracts.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/oauth2"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const namePrefix = "mcp__"

const (
	classRead    = "read"
	classExecute = "execute"
)

// ServerConfig is one production MCP connection declared by an agent spec.
// Secrets are referenced by environment-variable NAME and are never stored in
// the spec or journal.
type ServerConfig struct {
	Name           string            `yaml:"name" json:"name"`
	Transport      string            `yaml:"transport,omitempty" json:"transport,omitempty"` // stdio | http
	Command        []string          `yaml:"command,omitempty" json:"command,omitempty"`
	URL            string            `yaml:"url,omitempty" json:"url,omitempty"`
	EnvFrom        map[string]string `yaml:"env_from,omitempty" json:"env_from,omitempty"`
	HeadersFromEnv map[string]string `yaml:"headers_from_env,omitempty" json:"headers_from_env,omitempty"`
	OAuth          *OAuthConfig      `yaml:"oauth,omitempty" json:"oauth,omitempty"`
	AllowedTools   []string          `yaml:"allowed_tools,omitempty" json:"allowed_tools,omitempty"`
}

// OAuthConfig supplies a pre-obtained OAuth bearer token. The token value is
// read only at connection time from AccessTokenEnv.
type OAuthConfig struct {
	AccessTokenEnv string `yaml:"access_token_env" json:"access_token_env"`
}

// ValidateConfigs rejects ambiguous namespaces and incomplete transports.
func ValidateConfigs(configs []ServerConfig) error {
	seen := map[string]bool{}
	for i, c := range configs {
		field := fmt.Sprintf("mcp[%d]", i)
		if c.Name == "" {
			return fmt.Errorf("%s.name: required", field)
		}
		if strings.Contains(c.Name, "__") {
			return fmt.Errorf("%s.name: must not contain __", field)
		}
		if seen[c.Name] {
			return fmt.Errorf("%s.name: duplicate %q", field, c.Name)
		}
		seen[c.Name] = true
		transport := c.Transport
		if transport == "" {
			transport = "stdio"
		}
		switch transport {
		case "stdio":
			if len(c.Command) == 0 || c.Command[0] == "" {
				return fmt.Errorf("%s.command: required for stdio", field)
			}
			if c.URL != "" || c.OAuth != nil || len(c.HeadersFromEnv) > 0 {
				return fmt.Errorf("%s: url/headers_from_env/oauth require http transport", field)
			}
		case "http":
			if len(c.Command) > 0 {
				return fmt.Errorf("%s.command: not valid for http", field)
			}
			u, err := url.ParseRequestURI(c.URL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return fmt.Errorf("%s.url: valid http(s) URL required", field)
			}
			if c.OAuth != nil && c.OAuth.AccessTokenEnv == "" {
				return fmt.Errorf("%s.oauth.access_token_env: required", field)
			}
			if c.OAuth != nil {
				for header := range c.HeadersFromEnv {
					if strings.EqualFold(header, "Authorization") {
						return fmt.Errorf("%s.headers_from_env: Authorization conflicts with oauth", field)
					}
				}
			}
		default:
			return fmt.Errorf("%s.transport: unknown value %q (known: stdio, http)", field, c.Transport)
		}
		for _, name := range c.AllowedTools {
			if name == "" || strings.HasPrefix(name, namePrefix) {
				return fmt.Errorf("%s.allowed_tools: use bare non-empty server tool names", field)
			}
		}
		for target, source := range c.EnvFrom {
			if target == "" || source == "" {
				return fmt.Errorf("%s.env_from: target and source names must be non-empty", field)
			}
		}
		for header, source := range c.HeadersFromEnv {
			if header == "" || source == "" {
				return fmt.Errorf("%s.headers_from_env: header and source names must be non-empty", field)
			}
		}
	}
	return nil
}

func QualifiedName(server, tool string) string { return namePrefix + server + "__" + tool }

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

type DiscoveredTool struct {
	Server      string          `json:"server"`
	Tool        string          `json:"tool"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema,omitempty"`
	Class       string          `json:"class"`
}

type clientSession interface {
	ListTools(context.Context, *sdk.ListToolsParams) (*sdk.ListToolsResult, error)
	CallTool(context.Context, *sdk.CallToolParams) (*sdk.CallToolResult, error)
	ListResources(context.Context, *sdk.ListResourcesParams) (*sdk.ListResourcesResult, error)
	ListResourceTemplates(context.Context, *sdk.ListResourceTemplatesParams) (*sdk.ListResourceTemplatesResult, error)
	ReadResource(context.Context, *sdk.ReadResourceParams) (*sdk.ReadResourceResult, error)
	ListPrompts(context.Context, *sdk.ListPromptsParams) (*sdk.ListPromptsResult, error)
	GetPrompt(context.Context, *sdk.GetPromptParams) (*sdk.GetPromptResult, error)
	InitializeResult() *sdk.InitializeResult
	Wait() error
	Close() error
}

type sessionFactory func(context.Context) (clientSession, error)

// Conn is one reconnectable server. Calls are never replayed after an
// ambiguous transport error; only a later operation uses the replacement
// session.
type Conn struct {
	server  string
	allowed map[string]bool
	factory sessionFactory
	mu      sync.Mutex
	session clientSession
	changed atomic.Bool
	closed  bool
}

func NewConn(server string, session *sdk.ClientSession) *Conn {
	return &Conn{server: server, session: session}
}

func newDialConn(ctx context.Context, config ServerConfig, cwd string) (*Conn, error) {
	c := &Conn{server: config.Name}
	if len(config.AllowedTools) > 0 {
		c.allowed = make(map[string]bool, len(config.AllowedTools))
		for _, name := range config.AllowedTools {
			c.allowed[name] = true
		}
	}
	c.factory = func(connectCtx context.Context) (clientSession, error) {
		opts := &sdk.ClientOptions{
			ToolListChangedHandler:     func(context.Context, *sdk.ToolListChangedRequest) { c.changed.Store(true) },
			PromptListChangedHandler:   func(context.Context, *sdk.PromptListChangedRequest) { c.changed.Store(true) },
			ResourceListChangedHandler: func(context.Context, *sdk.ResourceListChangedRequest) { c.changed.Store(true) },
			ResourceUpdatedHandler:     func(context.Context, *sdk.ResourceUpdatedNotificationRequest) { c.changed.Store(true) },
		}
		client := sdk.NewClient(&sdk.Implementation{Name: "agentrunner", Version: "1"}, opts)
		var transport sdk.Transport
		switch defaultTransport(config.Transport) {
		case "stdio":
			if _, err := envValues(config.EnvFrom); err != nil {
				return nil, err
			}
			cmd := exec.Command(config.Command[0], config.Command[1:]...)
			cmd.Dir = cwd
			cmd.Env = serverEnv(config.EnvFrom)
			cmd.Stderr = os.Stderr
			transport = &sdk.CommandTransport{Command: cmd}
		case "http":
			headers, err := envValues(config.HeadersFromEnv)
			if err != nil {
				return nil, err
			}
			hc := &http.Client{Transport: headerTransport{base: http.DefaultTransport, headers: headers}}
			tr := &sdk.StreamableClientTransport{Endpoint: config.URL, HTTPClient: hc}
			if config.OAuth != nil {
				token := os.Getenv(config.OAuth.AccessTokenEnv)
				if token == "" {
					return nil, fmt.Errorf("mcp %q: oauth token environment variable %s is empty", config.Name, config.OAuth.AccessTokenEnv)
				}
				tr.OAuthHandler = staticOAuth{source: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token, TokenType: "Bearer"})}
			}
			transport = tr
		}
		return client.Connect(connectCtx, transport, nil)
	}
	if _, err := c.ensure(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

func defaultTransport(v string) string {
	if v == "" {
		return "stdio"
	}
	return v
}

func (c *Conn) ensure(ctx context.Context) (clientSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, fmt.Errorf("mcp %q: connection closed", c.server)
	}
	if c.session != nil {
		return c.session, nil
	}
	if c.factory == nil {
		return nil, fmt.Errorf("mcp %q: session unavailable", c.server)
	}
	s, err := c.factory(ctx)
	if err != nil {
		return nil, fmt.Errorf("mcp %q: connect: %w", c.server, err)
	}
	c.session = s
	go c.watch(s)
	return s, nil
}

func (c *Conn) watch(s clientSession) {
	_ = s.Wait()
	c.invalidate(s)
}

func (c *Conn) invalidate(s clientSession) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == s {
		c.session = nil
		c.changed.Store(true)
	}
}

func (c *Conn) Discover(ctx context.Context) ([]DiscoveredTool, error) {
	s, err := c.ensure(ctx)
	if err != nil {
		return nil, err
	}
	var out []DiscoveredTool
	seen := map[string]bool{}
	cursor := ""
	for {
		params := &sdk.ListToolsParams{Cursor: cursor}
		res, err := s.ListTools(ctx, params)
		if err != nil {
			c.invalidate(s)
			return nil, fmt.Errorf("mcp %q: list tools: %w", c.server, err)
		}
		for _, t := range res.Tools {
			if seen[t.Name] {
				return nil, fmt.Errorf("mcp %q: duplicate tool %q across pages", c.server, t.Name)
			}
			seen[t.Name] = true
			if c.allowed != nil && !c.allowed[t.Name] {
				continue
			}
			schema, _ := json.Marshal(t.InputSchema)
			out = append(out, DiscoveredTool{Server: c.server, Tool: t.Name,
				Name: QualifiedName(c.server, t.Name), Description: t.Description,
				InputSchema: schema, Class: classOf(t.Annotations)})
		}
		if res.NextCursor == "" {
			break
		}
		if res.NextCursor == cursor {
			return nil, fmt.Errorf("mcp %q: tools pagination cursor did not advance", c.server)
		}
		cursor = res.NextCursor
	}
	for _, t := range c.protocolTools(s) {
		if seen[t.Tool] {
			return nil, fmt.Errorf("mcp %q: server tool %q collides with agentrunner protocol tool", c.server, t.Tool)
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	c.changed.Store(false)
	return out, nil
}

const (
	resourcesList         = "resources_list"
	resourceTemplatesList = "resource_templates_list"
	resourcesRead         = "resources_read"
	promptsList           = "prompts_list"
	promptsGet            = "prompts_get"
)

func (c *Conn) protocolTools(s clientSession) []DiscoveredTool {
	init := s.InitializeResult()
	if init == nil || init.Capabilities == nil {
		return nil
	}
	var out []DiscoveredTool
	add := func(name, description, schema string) {
		if c.allowed != nil && !c.allowed[name] {
			return
		}
		out = append(out, DiscoveredTool{Server: c.server, Tool: name,
			Name: QualifiedName(c.server, name), Description: description,
			InputSchema: json.RawMessage(schema), Class: classRead})
	}
	if init.Capabilities.Resources != nil {
		add(resourcesList, "List MCP resources", `{"type":"object","properties":{"cursor":{"type":"string"}}}`)
		add(resourceTemplatesList, "List MCP resource templates", `{"type":"object","properties":{"cursor":{"type":"string"}}}`)
		add(resourcesRead, "Read an MCP resource including text or binary content", `{"type":"object","properties":{"uri":{"type":"string"}},"required":["uri"]}`)
	}
	if init.Capabilities.Prompts != nil {
		add(promptsList, "List MCP prompts", `{"type":"object","properties":{"cursor":{"type":"string"}}}`)
		add(promptsGet, "Render an MCP prompt including multimodal messages", `{"type":"object","properties":{"name":{"type":"string"},"arguments":{"type":"object","additionalProperties":{"type":"string"}}},"required":["name"]}`)
	}
	return out
}

func (c *Conn) Call(ctx context.Context, tool string, args json.RawMessage) (json.RawMessage, bool, error) {
	if c.allowed != nil && !c.allowed[tool] {
		return nil, false, fmt.Errorf("mcp %q: tool %q not permitted", c.server, tool)
	}
	s, err := c.ensure(ctx)
	if err != nil {
		return nil, false, err
	}
	if payload, handled, err := callProtocol(ctx, s, tool, args); handled {
		if err != nil {
			// Resource/prompt operations are read-only protocol helpers. Their
			// validation/server errors are ordinary model-visible tool errors,
			// not ambiguous side-effecting tool outcomes.
			payload, _ = json.Marshal(map[string]any{"error": err.Error()})
			return payload, true, nil
		}
		return payload, false, nil
	}
	var arguments any
	if len(args) > 0 {
		arguments = json.RawMessage(args)
	}
	res, err := s.CallTool(ctx, &sdk.CallToolParams{Name: tool, Arguments: arguments})
	if err != nil {
		c.invalidate(s)
		return nil, false, fmt.Errorf("mcp %q: call %q: %w", c.server, tool, err)
	}
	payload, err := json.Marshal(res) // preserve structured + text/image/audio/resource blocks
	if err != nil {
		return nil, false, err
	}
	return payload, res.IsError, nil
}

func callProtocol(ctx context.Context, s clientSession, name string, raw json.RawMessage) (json.RawMessage, bool, error) {
	var in struct {
		Cursor    string            `json:"cursor"`
		URI       string            `json:"uri"`
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, true, fmt.Errorf("mcp protocol tool %s arguments: %w", name, err)
		}
	}
	var value any
	var err error
	switch name {
	case resourcesList:
		value, err = s.ListResources(ctx, &sdk.ListResourcesParams{Cursor: in.Cursor})
	case resourceTemplatesList:
		value, err = s.ListResourceTemplates(ctx, &sdk.ListResourceTemplatesParams{Cursor: in.Cursor})
	case resourcesRead:
		if in.URI == "" {
			return nil, true, errors.New("mcp resources_read: uri is required")
		}
		value, err = s.ReadResource(ctx, &sdk.ReadResourceParams{URI: in.URI})
	case promptsList:
		value, err = s.ListPrompts(ctx, &sdk.ListPromptsParams{Cursor: in.Cursor})
	case promptsGet:
		if in.Name == "" {
			return nil, true, errors.New("mcp prompts_get: name is required")
		}
		value, err = s.GetPrompt(ctx, &sdk.GetPromptParams{Name: in.Name, Arguments: in.Arguments})
	default:
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	payload, err := json.Marshal(value)
	return payload, true, err
}

func (c *Conn) Changed() bool { return c.changed.Load() }

func (c *Conn) Close() error {
	c.mu.Lock()
	c.closed = true
	s := c.session
	c.session = nil
	c.mu.Unlock()
	if s != nil {
		return s.Close()
	}
	return nil
}

func classOf(ann *sdk.ToolAnnotations) string {
	if ann != nil && ann.ReadOnlyHint {
		return classRead
	}
	return classExecute
}

type Manager struct {
	conns   map[string]*Conn
	allowed map[string]bool
}

func NewManager() *Manager { return &Manager{conns: map[string]*Conn{}} }

// Connect opens every configured server. If one fails, already-opened
// servers are closed before the error is returned.
func Connect(ctx context.Context, configs []ServerConfig, cwd string) (*Manager, error) {
	if err := ValidateConfigs(configs); err != nil {
		return nil, err
	}
	m := NewManager()
	for _, config := range configs {
		c, err := newDialConn(ctx, config, cwd)
		if err != nil {
			_ = m.Close()
			return nil, err
		}
		if err := m.Add(c); err != nil {
			_ = c.Close()
			_ = m.Close()
			return nil, err
		}
	}
	return m, nil
}

func (m *Manager) Add(c *Conn) error {
	if _, dup := m.conns[c.server]; dup {
		return fmt.Errorf("mcp: duplicate server name %q", c.server)
	}
	m.conns[c.server] = c
	return nil
}

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

func (m *Manager) isAllowed(name string) bool { return m.allowed == nil || m.allowed[name] }

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
			// Discovery has no side effects and is safe to retry once on a
			// freshly-created session after a transport failure.
			tools, err = m.conns[name].Discover(ctx)
			if err != nil {
				return nil, err
			}
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

func (m *Manager) Changed() bool {
	for _, c := range m.conns {
		if c.Changed() {
			return true
		}
	}
	return false
}

func (m *Manager) Servers() []string {
	out := make([]string, 0, len(m.conns))
	for name := range m.conns {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// ErrNotDispatched marks a Call that never reached the server — blocked by
// allowed_tools, a malformed name, or no connection. Its outcome is KNOWN
// (nothing ran, no side effect), so the caller must NOT frame it as an
// unknown outcome (QA Wave4 karl-01).
var ErrNotDispatched = errors.New("mcp: call not dispatched")

func (m *Manager) Call(ctx context.Context, qualified string, args json.RawMessage) (json.RawMessage, bool, error) {
	if !m.isAllowed(qualified) {
		return nil, false, fmt.Errorf("%w: tool %q not permitted (not in this agent's allowed_tools)", ErrNotDispatched, qualified)
	}
	server, tool, ok := SplitName(qualified)
	if !ok {
		return nil, false, fmt.Errorf("%w: %q is not a fully-qualified mcp tool name", ErrNotDispatched, qualified)
	}
	conn, ok := m.conns[server]
	if !ok {
		return nil, false, fmt.Errorf("%w: no connected server %q", ErrNotDispatched, server)
	}
	return conn.Call(ctx, tool, args)
}

func (m *Manager) Close() error {
	var first error
	for _, c := range m.conns {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

type staticOAuth struct{ source oauth2.TokenSource }

var _ sdkauth.OAuthHandler = staticOAuth{}

func (s staticOAuth) TokenSource(context.Context) (oauth2.TokenSource, error) { return s.source, nil }
func (staticOAuth) Authorize(_ context.Context, _ *http.Request, response *http.Response) error {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	return errors.New("oauth access token was rejected; refresh the configured token environment variable")
}

type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func (t headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	for name, value := range t.headers {
		clone.Header.Set(name, value)
	}
	return t.base.RoundTrip(clone)
}

func envValues(from map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(from))
	for target, source := range from {
		value := os.Getenv(source)
		if value == "" {
			return nil, fmt.Errorf("mcp: environment variable %s for %s is empty", source, target)
		}
		out[target] = value
	}
	return out, nil
}

func serverEnv(from map[string]string) []string {
	// Keep only process mechanics while withholding the ambient application
	// environment. A server receives credentials and service configuration
	// exclusively through its explicit env_from declaration.
	var out []string
	targets := make(map[string]bool, len(from))
	for target := range from {
		targets[target] = true
	}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(name)
		if targets[name] || !safeProcessEnv(upper) {
			continue
		}
		out = append(out, entry)
	}
	values, _ := envValues(from) // validation/connect reports missing values below
	for target, value := range values {
		out = append(out, target+"="+value)
	}
	return out
}

func safeProcessEnv(name string) bool {
	switch name {
	case "PATH", "HOME", "TMPDIR", "TMP", "TEMP", "USER", "LOGNAME", "SHELL",
		"LANG", "LC_ALL", "LC_CTYPE", "TERM", "COLORTERM", "NO_COLOR",
		"SYSTEMROOT", "COMSPEC", "PATHEXT":
		return true
	default:
		return strings.HasPrefix(name, "LC_")
	}
}
