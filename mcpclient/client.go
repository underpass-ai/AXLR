package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Client owns sessions to named external MCP servers.
type Client struct {
	mu        sync.RWMutex
	sessions  map[ServerName]*mcp.ClientSession
	observers map[ServerName]*rawCallObserver
	pending   map[ServerName]context.CancelFunc
	closed    bool
}

func New() *Client {
	return &Client{sessions: make(map[ServerName]*mcp.ClientSession), observers: make(map[ServerName]*rawCallObserver), pending: make(map[ServerName]context.CancelFunc)}
}

func (c *Client) Connect(ctx context.Context, server Server) error {
	transport, err := server.transport()
	if err != nil {
		return err
	}
	var observer *rawCallObserver
	if server.Command != "" {
		observer = &rawCallObserver{}
		transport = rawTransport{base: transport, observer: observer}
	}
	return c.connectObserved(ctx, server.Name, transport, observer)
}

func (c *Client) connect(ctx context.Context, name ServerName, transport mcp.Transport) error {
	return c.connectObserved(ctx, name, transport, nil)
}

func (c *Client) connectObserved(ctx context.Context, name ServerName, transport mcp.Transport, observer *rawCallObserver) error {
	if _, err := NewServerName(name.String()); err != nil || transport == nil {
		return errors.New("invalid MCP connection")
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("MCP client is closed")
	}
	_, exists := c.sessions[name]
	if _, connecting := c.pending[name]; exists || connecting {
		c.mu.Unlock()
		return fmt.Errorf("MCP server %q already connected", name)
	}
	connectCtx, cancel := context.WithCancel(ctx)
	c.pending[name] = cancel
	c.mu.Unlock()
	defer cancel()

	sdkClient := mcp.NewClient(&mcp.Implementation{Name: "axlr", Version: "0.1.0"}, &mcp.ClientOptions{
		ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) {},
	})
	session, err := sdkClient.Connect(connectCtx, transport, nil)
	c.mu.Lock()
	delete(c.pending, name)
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("connect MCP server %q: %w", name, err)
	}
	if c.closed {
		c.mu.Unlock()
		_ = session.Close()
		return errors.New("MCP client closed during connection")
	}
	c.sessions[name] = session
	if observer != nil {
		c.observers[name] = observer
	}
	c.mu.Unlock()
	return nil
}

func (c *Client) session(name ServerName) (*mcp.ClientSession, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return nil, errors.New("MCP client is closed")
	}
	session, ok := c.sessions[name]
	if !ok {
		return nil, fmt.Errorf("MCP server %q is not connected", name)
	}
	return session, nil
}

func (c *Client) ListTools(ctx context.Context, serverName ServerName) ([]Tool, error) {
	session, err := c.session(serverName)
	if err != nil {
		return nil, err
	}
	var tools []Tool
	cursor := ""
	seen := map[string]bool{}
	for page := 0; page < 1000; page++ {
		response, err := session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, fmt.Errorf("list MCP tools from %q: %w", serverName, err)
		}
		for _, remote := range response.Tools {
			name, err := NewToolName(remote.Name)
			if err != nil {
				return nil, fmt.Errorf("MCP server %q advertised invalid tool name: %w", serverName, err)
			}
			input, err := json.Marshal(remote.InputSchema)
			if err != nil {
				return nil, fmt.Errorf("MCP tool %q has invalid input schema: %w", remote.Name, err)
			}
			var output json.RawMessage
			if remote.OutputSchema != nil {
				output, err = json.Marshal(remote.OutputSchema)
				if err != nil {
					return nil, fmt.Errorf("MCP tool %q has invalid output schema: %w", remote.Name, err)
				}
			}
			tools = append(tools, Tool{Ref: ToolRef{Server: serverName, Name: name}, Description: remote.Description, InputSchema: input, OutputSchema: output})
		}
		if response.NextCursor == "" {
			return tools, nil
		}
		if seen[response.NextCursor] {
			return nil, errors.New("MCP server repeated a tools cursor")
		}
		seen[response.NextCursor] = true
		cursor = response.NextCursor
	}
	return nil, errors.New("MCP tool list exceeded 1000 pages")
}

func (c *Client) Call(ctx context.Context, ref ToolRef, arguments map[string]any) (Result, error) {
	if err := ref.Validate(); err != nil {
		return Result{}, err
	}
	session, err := c.session(ref.Server)
	if err != nil {
		return Result{}, err
	}
	if arguments == nil {
		arguments = map[string]any{}
	}
	if _, err := json.Marshal(arguments); err != nil {
		return Result{}, fmt.Errorf("invalid MCP tool arguments: %w", err)
	}
	c.mu.RLock()
	observer := c.observers[ref.Server]
	c.mu.RUnlock()
	if observer != nil {
		observer.begin()
	}
	response, err := session.CallTool(ctx, &mcp.CallToolParams{Name: ref.Name.String(), Arguments: arguments})
	var rawStructured json.RawMessage
	if observer != nil {
		rawStructured = observer.end()
	}
	if err != nil {
		return Result{}, fmt.Errorf("call MCP tool %q on %q: %w", ref.Name, ref.Server, err)
	}
	result := Result{IsError: response.IsError, Content: make([]json.RawMessage, 0, len(response.Content))}
	for _, content := range response.Content {
		encoded, err := json.Marshal(content)
		if err != nil {
			return Result{}, fmt.Errorf("encode MCP tool content: %w", err)
		}
		result.Content = append(result.Content, encoded)
	}
	if len(rawStructured) != 0 {
		result.StructuredContent = rawStructured
	} else if response.StructuredContent != nil {
		result.StructuredContent, err = json.Marshal(response.StructuredContent)
		if err != nil {
			return Result{}, fmt.Errorf("encode MCP structured content: %w", err)
		}
	}
	return result, nil
}

func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	for _, cancel := range c.pending {
		cancel()
	}
	sessions := c.sessions
	c.sessions = make(map[ServerName]*mcp.ClientSession)
	c.observers = make(map[ServerName]*rawCallObserver)
	c.mu.Unlock()
	var errs []error
	for name, session := range sessions {
		if err := session.Close(); err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				if runtime.GOOS == "windows" {
					// Closing a stdio transport terminates the Windows child.
					continue
				}
				if status, ok := exit.Sys().(syscall.WaitStatus); ok && (status.Signal() == syscall.SIGTERM || status.Signal() == syscall.SIGKILL) {
					continue
				}
			}
			errs = append(errs, fmt.Errorf("close MCP server %q: %w", name, err))
		}
	}
	return errors.Join(errs...)
}
