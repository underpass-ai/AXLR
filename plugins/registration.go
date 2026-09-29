package plugins

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/underpass-ai/AXLR/domain"
)

type Registration struct {
	Manifest Manifest
	Env      []string
}

func NewRegistration(manifest Manifest, env []string) (Registration, error) {
	if _, err := domain.NewPluginID(manifest.ID.String()); err != nil {
		return Registration{}, err
	}
	if !filepath.IsAbs(manifest.Command) || strings.ContainsRune(manifest.Command, 0) || len(manifest.AllowTools) == 0 {
		return Registration{}, errors.New("invalid plugin command or allowlist")
	}
	for _, arg := range manifest.Args {
		if strings.ContainsRune(arg, 0) {
			return Registration{}, errors.New("plugin argument contains NUL")
		}
	}
	allowed := map[domain.PluginToolName]bool{}
	for _, tool := range manifest.AllowTools {
		if _, err := domain.NewPluginToolName(tool.String()); err != nil || allowed[tool] {
			return Registration{}, errors.New("invalid plugin allowlist")
		}
		allowed[tool] = true
	}
	seen := map[string]bool{}
	for _, entry := range env {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || key == "" || strings.ContainsAny(key, "\x00=") || strings.ContainsRune(entry, 0) || seen[key] {
			return Registration{}, errors.New("invalid plugin child environment")
		}
		seen[key] = true
	}
	copyManifest := manifest
	copyManifest.Args = append([]string{}, manifest.Args...)
	copyManifest.AllowTools = append([]domain.PluginToolName{}, manifest.AllowTools...)
	return Registration{Manifest: copyManifest, Env: append([]string{}, env...)}, nil
}
