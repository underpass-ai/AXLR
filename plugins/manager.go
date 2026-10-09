package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/mcpclient"
)

type Manager struct {
	mu            sync.Mutex
	slot          chan struct{}
	lifetime      context.Context
	cancel        context.CancelFunc
	registrations map[domain.PluginID]Registration
	order         []domain.PluginID
	connected     map[domain.PluginID]bool
	client        *mcpclient.Client
	closed        bool
	// share, when set, returns the unix socket where a process shared with
	// other consoles serves a registration; an error launches the command.
	share func(context.Context, Registration) (string, error)
	// observer, when set, hears connections and the stderr of the servers
	// the manager launches.
	observer Observer
}

// ConnectionEvent is one change of a server's connection.
type ConnectionEvent string

const (
	// Connected: the server answered its initialization; shared tells a
	// process shared with other consoles from one this manager launched.
	Connected ConnectionEvent = "connected"
	// ConnectFailed: the server could not be reached or initialized; the
	// next operation tries again.
	ConnectFailed ConnectionEvent = "connect_failed"
	// ConnectionLost: a connected server went away, such as a stdio server
	// that exited; the next operation launches it again.
	ConnectionLost ConnectionEvent = "connection_lost"
)

// Observer hears the manager's connections by plugin ID. Stderr returns the
// writer for a stdio server's standard error, or nil to discard it. Calls
// come from the manager's operations, one at a time, and must not block.
type Observer interface {
	PluginConnection(id domain.PluginID, event ConnectionEvent, shared bool, err error)
	PluginStderr(id domain.PluginID) io.Writer
}

// SetObserver sets the manager's observer; it is set before the first
// operation.
func (m *Manager) SetObserver(observer Observer) {
	m.observer = observer
}

// SetShare lets the manager reach a registration through a shared process
// instead of launching its own; it is set before the first operation.
func (m *Manager) SetShare(share func(context.Context, Registration) (string, error)) {
	m.share = share
}

func NewManager(registrations []Registration) (*Manager, error) {
	lifetime, cancel := context.WithCancel(context.Background())
	m := &Manager{slot: make(chan struct{}, 1), lifetime: lifetime, cancel: cancel, registrations: make(map[domain.PluginID]Registration), connected: make(map[domain.PluginID]bool), client: mcpclient.New()}
	m.slot <- struct{}{}
	for _, registration := range registrations {
		validated, err := NewRegistration(registration.Manifest, registration.Env)
		if err != nil {
			cancel()
			return nil, err
		}
		id := validated.Manifest.ID
		if _, exists := m.registrations[id]; exists {
			cancel()
			return nil, fmt.Errorf("duplicate plugin ID %q", id)
		}
		m.registrations[id] = validated
		m.order = append(m.order, id)
	}
	return m, nil
}

// Register makes an installed MCP server available to the next tool discovery.
func (m *Manager) Register(ctx context.Context, registration Registration) error {
	validated, err := NewRegistration(registration.Manifest, registration.Env)
	if err != nil {
		return err
	}
	if err := m.acquire(ctx); err != nil {
		return err
	}
	defer m.release()
	id := validated.Manifest.ID
	if _, exists := m.registrations[id]; exists {
		return fmt.Errorf("MCP server %q is already registered", id)
	}
	m.registrations[id] = validated
	m.order = append(m.order, id)
	return nil
}

func (m *Manager) acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-m.lifetime.Done():
		return errors.New("plugin manager is closed")
	case <-m.slot:
	}
	if m.lifetime.Err() != nil {
		m.release()
		return errors.New("plugin manager is closed")
	}
	return nil
}

func (m *Manager) release() { m.slot <- struct{}{} }

func (m *Manager) operationContext(ctx context.Context) (context.Context, func()) {
	opCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(m.lifetime, cancel)
	return opCtx, func() { stop(); cancel() }
}

func (m *Manager) connect(ctx context.Context, id domain.PluginID) error {
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return errors.New("plugin manager is closed")
	}
	if m.connected[id] {
		return nil
	}
	name := mcpclient.ServerName(id)
	// A session the manager never recorded is stale: replace it, do not reuse it.
	_ = m.client.Disconnect(name)
	r := m.registrations[id]
	err := errors.New("not shared")
	shared := false
	if m.share != nil && r.Manifest.Command != "" {
		if socket, shareErr := m.share(ctx, r); shareErr == nil {
			err = m.client.Connect(ctx, mcpclient.Server{Name: name, Socket: socket})
			shared = err == nil
		}
	}
	if err != nil && ctx.Err() == nil {
		server := mcpclient.Server{Name: name, Command: r.Manifest.Command, URL: r.Manifest.URL, Args: r.Manifest.Args, Env: r.Env}
		if m.observer != nil && r.Manifest.Command != "" {
			server.Stderr = m.observer.PluginStderr(id)
		}
		err = m.client.Connect(ctx, server)
	}
	if ctx.Err() != nil {
		if err == nil {
			_ = m.client.Disconnect(name)
		}
		return ctx.Err()
	}
	if err != nil {
		m.observe(id, ConnectFailed, false, err)
		return err
	}
	m.connected[id] = true
	m.observe(id, Connected, shared, nil)
	return nil
}

func (m *Manager) observe(id domain.PluginID, event ConnectionEvent, shared bool, err error) {
	if m.observer != nil {
		m.observer.PluginConnection(id, event, shared, err)
	}
}

// forgetLost drops the session of a server that has gone away, such as a
// stdio server that exited, so the next operation launches it again. The
// failed operation itself is not retried.
func (m *Manager) forgetLost(id domain.PluginID, err error) {
	if !mcpclient.ConnectionLost(err) {
		return
	}
	if m.connected[id] {
		m.observe(id, ConnectionLost, false, err)
	}
	delete(m.connected, id)
	_ = m.client.Disconnect(mcpclient.ServerName(id))
}

func (m *Manager) List(ctx context.Context) ([]domain.PluginTool, error) {
	if err := m.acquire(ctx); err != nil {
		return nil, err
	}
	defer m.release()
	ctx, finish := m.operationContext(ctx)
	defer finish()
	result := []domain.PluginTool{}
	for _, id := range m.order {
		tools, err := m.listServer(ctx, id)
		if err != nil {
			return nil, err
		}
		result = append(result, tools...)

	}
	return result, nil
}

// ListServer discovers one configured server without conflating other failures.
func (m *Manager) ListServer(ctx context.Context, id domain.PluginID) ([]domain.PluginTool, error) {
	if err := m.acquire(ctx); err != nil {
		return nil, err
	}
	defer m.release()
	ctx, finish := m.operationContext(ctx)
	defer finish()
	if _, known := m.registrations[id]; !known {
		return nil, errors.New("unknown plugin")
	}
	return m.listServer(ctx, id)
}
func (m *Manager) listServer(ctx context.Context, id domain.PluginID) ([]domain.PluginTool, error) {
	result := []domain.PluginTool{}
	if err := m.connect(ctx, id); err != nil {
		return nil, err
	}
	tools, err := m.client.ListTools(ctx, mcpclient.ServerName(id))
	m.forgetLost(id, err)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	allowed := map[domain.PluginToolName]bool{}
	for _, name := range m.registrations[id].Manifest.AllowTools {
		allowed[name] = true
	}
	for _, tool := range tools {
		name := domain.PluginToolName(tool.Ref.Name)
		if !allowed[name] && !m.registrations[id].Manifest.AllowAll {
			continue
		}
		input, err := domain.NewJSONValue(tool.InputSchema)
		if err != nil {
			return nil, err
		}
		var output *domain.JSONValue
		if len(tool.OutputSchema) != 0 {
			v, err := domain.NewJSONValue(tool.OutputSchema)
			if err != nil {
				return nil, err
			}
			output = &v
		}
		result = append(result, domain.PluginTool{Ref: domain.PluginRef{PluginID: id, ToolName: name}, Description: tool.Description, InputSchema: input, OutputSchema: output})
	}
	return result, nil
}

func (m *Manager) Call(ctx context.Context, call domain.PluginCall) (domain.PluginResult, error) {
	if err := m.acquire(ctx); err != nil {
		return domain.PluginResult{}, err
	}
	defer m.release()
	ctx, finish := m.operationContext(ctx)
	defer finish()
	r, exists := m.registrations[call.Ref.PluginID]
	if !exists {
		return domain.PluginResult{}, domain.Reject("unknown_plugin", "unknown plugin")
	}
	allowed := r.Manifest.AllowAll
	for _, name := range r.Manifest.AllowTools {
		if name == call.Ref.ToolName {
			allowed = true
			break
		}
	}
	if !allowed {
		return domain.PluginResult{}, domain.Reject("unknown_plugin_tool", "plugin tool is not allowed")
	}
	var arguments map[string]any
	decoder := json.NewDecoder(bytes.NewReader(call.Arguments.Bytes()))
	decoder.UseNumber()
	if err := decoder.Decode(&arguments); err != nil || arguments == nil {
		return domain.PluginResult{}, domain.Reject("invalid_arguments", "plugin arguments must be an object")
	}
	if err := m.connect(ctx, call.Ref.PluginID); err != nil {
		return domain.PluginResult{}, err
	}
	tools, err := m.client.ListTools(ctx, mcpclient.ServerName(call.Ref.PluginID))
	m.forgetLost(call.Ref.PluginID, err)
	if ctx.Err() != nil {
		return domain.PluginResult{}, ctx.Err()
	}
	if err != nil {
		return domain.PluginResult{}, err
	}
	found := false
	for _, tool := range tools {
		if tool.Ref.Name.String() == call.Ref.ToolName.String() {
			found = true
			break
		}
	}
	if !found {
		return domain.PluginResult{}, domain.Reject("unknown_plugin_tool", "plugin tool is not advertised")
	}
	remote, err := m.client.Call(ctx, mcpclient.ToolRef{Server: mcpclient.ServerName(call.Ref.PluginID), Name: mcpclient.ToolName(call.Ref.ToolName)}, arguments)
	m.forgetLost(call.Ref.PluginID, err)
	if ctx.Err() != nil {
		return domain.PluginResult{}, ctx.Err()
	}
	if err != nil {
		return domain.PluginResult{}, err
	}
	result := domain.PluginResult{IsError: remote.IsError, Content: make([]domain.JSONValue, 0, len(remote.Content))}
	for _, raw := range remote.Content {
		value, err := domain.NewJSONValue(raw)
		if err != nil {
			return domain.PluginResult{}, err
		}
		result.Content = append(result.Content, value)
	}
	if len(remote.StructuredContent) != 0 {
		value, err := domain.NewJSONValue(remote.StructuredContent)
		if err != nil {
			return domain.PluginResult{}, err
		}
		result.StructuredContent = &value
	}
	return result, nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	m.cancel()
	m.mu.Unlock()
	return m.client.Close()
}
