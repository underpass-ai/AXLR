package domain

import (
	"errors"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// ToolIdentity identifies exactly one local, host, or registered plugin operation.
type ToolIdentity struct {
	Kind           string
	LocalOperation string
	Plugin         axlr.PluginRef
}

const (
	ToolKindLocal         = "local"
	ToolKindPlugin        = "plugin"
	ToolKindHost          = "host"
	HostOperationTools    = "tools"
	HostOperationCallTool = "call_tool"
	HostOperationHistory  = "history"
	HostOperationSkill    = "skill"
	// HostOperationStepDone hands the current ceremony step's result to the
	// console, which checks it and advances MADE.
	HostOperationStepDone = "step_done"
)

func NewLocalToolIdentity(operation string) (ToolIdentity, error) {
	id := ToolIdentity{Kind: ToolKindLocal, LocalOperation: operation}
	return id, id.Validate()
}
func NewPluginToolIdentity(ref axlr.PluginRef) (ToolIdentity, error) {
	id := ToolIdentity{Kind: ToolKindPlugin, Plugin: ref}
	return id, id.Validate()
}
func NewHostToolIdentity(operation string) (ToolIdentity, error) {
	id := ToolIdentity{Kind: ToolKindHost, LocalOperation: operation}
	return id, id.Validate()
}
func (id ToolIdentity) Validate() error {
	switch id.Kind {
	case ToolKindHost:
		if id.Plugin != (axlr.PluginRef{}) {
			return errors.New("host identity cannot include plugin")
		}
		switch id.LocalOperation {
		case HostOperationTools, HostOperationCallTool, HostOperationHistory, HostOperationSkill, HostOperationStepDone:
			return nil
		}
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
