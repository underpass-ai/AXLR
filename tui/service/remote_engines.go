package service

import (
	"context"
	"errors"
	"path/filepath"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func EngineRegistrations(cfg Config) ([]plugins.Registration, []domain.PluginProfile, error) {
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	var registrations []plugins.Registration
	var profiles []domain.PluginProfile
	for _, item := range []struct {
		id      root.PluginID
		purpose domain.PluginPurpose
		config  EngineConfig
		prefix  string
	}{
		{"kmp", domain.PluginPurposeMemory, cfg.KMP, "KMP_KERNEL_GRPC"},
		{"made", domain.PluginPurposeCeremony, cfg.MADE, "MADE_MCP_GRPC"},
	} {
		env := []string{
			item.prefix + "_ENDPOINT=" + item.config.Endpoint,
			item.prefix + "_TLS_MODE=mutual",
			item.prefix + "_TLS_CA_PATH=" + filepath.Join(item.config.TLSDir, "ca.crt"),
			item.prefix + "_TLS_CERT_PATH=" + filepath.Join(item.config.TLSDir, "tls.crt"),
			item.prefix + "_TLS_KEY_PATH=" + filepath.Join(item.config.TLSDir, "tls.key"),
			item.prefix + "_TLS_DOMAIN_NAME=" + item.config.ServerName,
		}
		if item.id == "kmp" {
			env = append(env, "KMP_MCP_BACKEND=grpc")
		} else {
			env = append(env, "MADE_MCP_BACKEND=grpc")
		}
		registration, err := plugins.NewRegistration(plugins.Manifest{ID: item.id, Command: item.config.Command, AllowAll: true}, env)
		if err != nil {
			return nil, nil, err
		}
		registrations = append(registrations, registration)
		profiles = append(profiles, domain.PluginProfile{ID: item.id, Name: root.Text(item.id), Purpose: item.purpose, Approval: domain.ApprovalManual})
	}
	return registrations, profiles, nil
}

func EngineReadiness(manager *plugins.Manager, key string) func(context.Context) error {
	return func(ctx context.Context) error {
		if key == "" || manager == nil {
			return errors.New("model or MCP engine unavailable")
		}
		for _, id := range []root.PluginID{"kmp", "made"} {
			if _, err := manager.ListServer(ctx, id); err != nil {
				return err
			}
		}
		return nil
	}
}
