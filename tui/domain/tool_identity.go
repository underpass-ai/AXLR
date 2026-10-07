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
	HostOperationSession  = "session"
	// HostOperationStepDone hands the current ceremony step's result to the
	// console, which checks it and advances MADE.
	HostOperationStepDone = "step_done"
	// HostOperationRequestRepair asks the console to repair AXLR itself in a
	// separate session, with evidence the console validates before starting.
	HostOperationRequestRepair = "request_repair"
	// HostOperationRepairStatus reads the repairs linked to this session.
	HostOperationRepairStatus = "repair_status"
	// HostOperationJudge asks TypeSafe Jev, an external judgement model, one
	// question; offered only when settings enable it.
	HostOperationJudge = "judge"
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
		case HostOperationTools, HostOperationCallTool, HostOperationHistory, HostOperationSkill, HostOperationSession, HostOperationStepDone, HostOperationRequestRepair, HostOperationRepairStatus, HostOperationJudge:
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
