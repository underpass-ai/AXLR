package storage

import (
	"errors"
	"fmt"
	"path/filepath"
)

// ExecSandboxSettings is the exec_sandbox section: Mode off (the default),
// auto (confine local commands when the platform can) or required (refuse
// them when it cannot); Network false cuts confined commands off the
// network (default true); Writable lists absolute paths outside the
// workspace they may write, such as a build cache.
type ExecSandboxSettings struct {
	Mode     string   `json:"mode,omitempty"`
	Network  *bool    `json:"network,omitempty"`
	Writable []string `json:"writable,omitempty"`
}

// maxSandboxWritable bounds exec_sandbox.writable.
const maxSandboxWritable = 32

// ExecSandbox is the configured section with defaults applied.
func (s UserSettings) ExecSandbox() ExecSandboxSettings {
	sandbox := ExecSandboxSettings{Mode: "off"}
	if s.Sandbox != nil {
		sandbox = *s.Sandbox
		if sandbox.Mode == "" {
			sandbox.Mode = "off"
		}
	}
	return sandbox
}

// AllowsNetwork reports whether confined commands keep the network.
func (s ExecSandboxSettings) AllowsNetwork() bool { return s.Network == nil || *s.Network }

func (s *ExecSandboxSettings) validate() error {
	if s == nil {
		return nil
	}
	switch s.Mode {
	case "", "off", "auto", "required":
	default:
		return errors.New("settings exec_sandbox.mode must be off, auto or required")
	}
	if len(s.Writable) > maxSandboxWritable {
		return fmt.Errorf("settings exec_sandbox.writable lists more than %d paths", maxSandboxWritable)
	}
	for _, path := range s.Writable {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path || path == string(filepath.Separator) {
			return fmt.Errorf("settings exec_sandbox.writable %q must be a clean absolute path other than the root", path)
		}
	}
	return nil
}
