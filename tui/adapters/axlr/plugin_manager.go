package axlr

import (
	"context"
	"errors"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"sort"
	"sync"
	"time"
)

// PluginManager projects plugin capabilities and exact, persisted authorization policies.
type PluginManager struct {
	Diagnostics           application.DiagnosticPort
	manager               *plugins.Manager
	mu                    sync.RWMutex
	profiles              []domain.PluginProfile
	persist               func(context.Context, root.PluginID, domain.ApprovalMode) error
	persistInstall        func(context.Context, string) error
	persistInstallWithEnv func(context.Context, string, map[string]string, map[string]string) error
	resolveEnv            func(string) string
	persistURL            func(context.Context, root.PluginID, string) error
}

func (m *PluginManager) SetInstaller(install func(context.Context, string) error) {
	m.persistInstall = install
}

func (m *PluginManager) SetEnvironmentInstaller(install func(context.Context, string, map[string]string, map[string]string) error, getenv func(string) string) {
	m.persistInstallWithEnv = install
	m.resolveEnv = getenv
}

func (m *PluginManager) InstallManifestWithEnvironment(ctx context.Context, path string, envFrom, env map[string]string) error {
	if m.persistInstallWithEnv == nil || m.resolveEnv == nil {
		return errors.New("MCP environment installation is unavailable")
	}
	manifest, err := plugins.LoadManifest(path)
	if err != nil {
		return err
	}
	values := map[string]string{}
	for key, value := range env {
		values[key] = value
	}
	for key, source := range envFrom {
		if _, exists := values[key]; exists {
			return errors.New("duplicate MCP environment key")
		}
		values[key] = m.resolveEnv(source)
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	entries := make([]string, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, key+"="+values[key])
	}
	registration, err := plugins.NewRegistration(manifest, entries)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, profile := range m.profiles {
		if profile.ID == manifest.ID {
			return errors.New("MCP server is already installed")
		}
	}
	if err := m.persistInstallWithEnv(ctx, path, envFrom, env); err != nil {
		return err
	}
	if err := m.manager.Register(ctx, registration); err != nil {
		return err
	}
	m.profiles = append(m.profiles, domain.PluginProfile{ID: manifest.ID, Name: root.Text(manifest.ID), Purpose: domain.PluginPurposeTools, Approval: domain.ApprovalManual})
	return nil
}

func (m *PluginManager) SetURLInstaller(install func(context.Context, root.PluginID, string) error) {
	m.persistURL = install
}

func (m *PluginManager) InstallURL(ctx context.Context, id root.PluginID, endpoint string) error {
	if m.persistURL == nil {
		return errors.New("MCP installation requires persistent configuration")
	}
	registration, err := plugins.NewRegistration(plugins.Manifest{ID: id, URL: endpoint, AllowAll: true}, nil)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, profile := range m.profiles {
		if profile.ID == id {
			return errors.New("MCP server is already installed")
		}
	}
	if err := m.persistURL(ctx, id, endpoint); err != nil {
		return err
	}
	if err := m.manager.Register(ctx, registration); err != nil {
		return err
	}
	m.profiles = append(m.profiles, domain.PluginProfile{ID: id, Name: root.Text(id), Purpose: domain.PluginPurposeTools, Approval: domain.ApprovalManual})
	return nil
}

func (m *PluginManager) InstallManifest(ctx context.Context, path string) error {
	if m.persistInstall == nil {
		return errors.New("MCP installation requires persistent configuration")
	}
	manifest, err := plugins.LoadManifest(path)
	if err != nil {
		return err
	}
	registration, err := plugins.NewRegistration(manifest, nil)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, profile := range m.profiles {
		if profile.ID == manifest.ID {
			return errors.New("MCP server is already installed")
		}
	}
	if err := m.persistInstall(ctx, path); err != nil {
		return err
	}
	if err := m.manager.Register(ctx, registration); err != nil {
		return err
	}
	m.profiles = append(m.profiles, domain.PluginProfile{ID: manifest.ID, Name: root.Text(manifest.ID), Purpose: domain.PluginPurposeTools, Approval: domain.ApprovalManual})
	return nil
}

func NewPluginManager(manager *plugins.Manager, profiles []domain.PluginProfile, persist func(context.Context, root.PluginID, domain.ApprovalMode) error) *PluginManager {
	return &PluginManager{manager: manager, profiles: append([]domain.PluginProfile(nil), profiles...), persist: persist}
}

func (m *PluginManager) Profiles() []domain.PluginProfile {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]domain.PluginProfile(nil), m.profiles...)
}

func (m *PluginManager) AutoApproves(identity domain.ToolIdentity) bool {
	if identity.Kind != domain.ToolKindPlugin || identity.Validate() != nil {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, profile := range m.profiles {
		if profile.ID == identity.Plugin.PluginID && profile.Validate() == nil {
			return profile.Approval == domain.ApprovalAuto
		}
	}
	return false
}

func (m *PluginManager) List(ctx context.Context) (result []domain.PluginState, returnErr error) {
	ctx, span := application.StartDiagnosticSpan(ctx, m.Diagnostics, application.DiagnosticActionTools, application.DiagnosticEvent{})
	class := application.DiagnosticErrorNone
	defer func() {
		if returnErr != nil {
			class = pluginDiagnosticError(returnErr)
		}
		span.End(class)
	}()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	profiles := m.Profiles()
	states := make([]domain.PluginState, len(profiles))
	for i, profile := range profiles {
		states[i].Profile = profile
	}
	if len(states) == 0 {
		return states, nil
	}
	if m.manager == nil {
		class = application.DiagnosticErrorTool
		for i := range states {
			states[i].Error = "MCP manager is unavailable"
		}
		return states, nil
	}
	for i, profile := range profiles {
		bounded, cancel := context.WithTimeout(ctx, 12*time.Second)
		serverCtx, serverSpan := application.StartDiagnosticSpan(bounded, m.Diagnostics, application.DiagnosticActionPluginDiscovery, application.DiagnosticEvent{PluginOrdinal: pluginOrdinal(profiles, profile.ID)})
		tools, err := m.manager.ListServer(serverCtx, profile.ID)
		cancel()
		serverSpan.End(pluginDiagnosticError(errors.Join(err, ctx.Err())))
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			class = application.DiagnosticErrorTool
			// Process errors can contain paths or environment values. Only expose a safe status.
			states[i].Error = "MCP discovery failed; check the configured server"
			continue
		}
		for _, tool := range tools {
			if tool.Ref.PluginID == profile.ID {
				states[i].Tools = append(states[i].Tools, tool.Ref.ToolName)
			}
		}
	}
	return states, nil
}

func (m *PluginManager) SetApproval(ctx context.Context, id root.PluginID, mode domain.ApprovalMode) (returnErr error) {
	ctx, span := application.StartDiagnosticSpan(ctx, m.Diagnostics, application.DiagnosticActionPluginPolicy, application.DiagnosticEvent{PluginOrdinal: pluginOrdinal(m.Profiles(), id)})
	class := application.DiagnosticErrorNone
	defer func() {
		if returnErr != nil && class == application.DiagnosticErrorNone {
			class = pluginDiagnosticError(returnErr)
		}
		span.End(class)
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := mode.Validate(); err != nil {
		return err
	}
	if _, err := root.NewPluginID(id.String()); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, profile := range m.profiles {
		if profile.ID != id {
			continue
		}
		if m.persist == nil {
			return errors.New("plugin approval requires a persistent configuration")
		}
		if err := m.persist(ctx, id, mode); err != nil {
			class = application.DiagnosticErrorStorage
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				class = pluginDiagnosticError(err)
			}
			return err
		}
		m.profiles[i].Approval = mode
		return nil
	}
	return errors.New("unknown plugin")
}
