package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	r := m.registrations[id]
	err := m.client.Connect(ctx, mcpclient.Server{Name: mcpclient.ServerName(id), Command: r.Manifest.Command, Args: r.Manifest.Args, Env: r.Env})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	m.connected[id] = true
	return nil
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
		if err := m.connect(ctx, id); err != nil {
			return nil, err
		}
		tools, err := m.client.ListTools(ctx, mcpclient.ServerName(id))
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
			if !allowed[name] {
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
	allowed := false
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
