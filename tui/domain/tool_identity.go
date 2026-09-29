package domain

import (
	"errors"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// ToolIdentity identifies exactly one local operation or registered plugin tool.
type ToolIdentity struct {
	Kind           string
	LocalOperation string
	Plugin         axlr.PluginRef
}

const (
	ToolKindLocal  = "local"
	ToolKindPlugin = "plugin"
)

func NewLocalToolIdentity(operation string) (ToolIdentity, error) {
	id := ToolIdentity{Kind: ToolKindLocal, LocalOperation: operation}
	return id, id.Validate()
}
func NewPluginToolIdentity(ref axlr.PluginRef) (ToolIdentity, error) {
	id := ToolIdentity{Kind: ToolKindPlugin, Plugin: ref}
	return id, id.Validate()
}
func (id ToolIdentity) Validate() error {
	switch id.Kind {
	case ToolKindLocal:
		if id.Plugin != (axlr.PluginRef{}) {
			return errors.New("local identity cannot include plugin")
		}
		switch id.LocalOperation {
		case "read", "write", "edit", "exec":
			return nil
		}
	case ToolKindPlugin:
		if id.LocalOperation != "" {
			return errors.New("plugin identity cannot include local operation")
		}
		if _, err := axlr.NewPluginID(string(id.Plugin.PluginID)); err != nil {
			return err
		}
		_, err := axlr.NewPluginToolName(string(id.Plugin.ToolName))
		return err
	}
	return errors.New("invalid tool identity")
}
