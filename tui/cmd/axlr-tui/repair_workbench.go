package main

import (
	"context"
	"errors"

	"github.com/underpass-ai/AXLR/adapters/local"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/adapters/axlr"
	"github.com/underpass-ai/AXLR/tui/adapters/axlrplugin"
	"github.com/underpass-ai/AXLR/tui/adapters/ceremonyhost"
	"github.com/underpass-ai/AXLR/tui/adapters/madesetup"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// repairWorkbenches is application.RepairWorkbenchPort for the console: each
// repair session gets its own AXLR runtime rooted in its clone, so its file
// tools, checks and forge commands never leave the clone, while the model
// client, the session store and the MCP connections are the console's.
type repairWorkbenches struct {
	env           []string
	manager       *plugins.Manager
	registrations []plugins.Registration
	labels        application.SessionLabelsPort
	models        application.ModelStreamPort
	windows       application.ModelContextWindowPort
	store         application.SessionStorePort
	trace         application.DiagnosticPort
	validator     application.ToolArgumentValidationPort
	approval      application.ToolApprovalPolicyPort
	profiles      func() []domain.PluginProfile
	catalog       *axlrplugin.Catalog
	configPath    string
	getenv        func(string) string
	reviewerModel string
	policy        application.RepairPolicy
	autonomous    bool
	// plans and compact serve plan workers: the registry the task driver
	// writes and the ceremony profile.
	plans   application.PlanRegistryPort
	compact func(root.ModelID) bool
	// calibration learns bytes per prompt token from these sessions too.
	calibration application.TokenCalibrationPort
	// turnToolCalls is settings' turn budget, as in the console's sessions.
	turnToolCalls int
	// sandbox confines the workbench's commands as the console's.
	sandbox *local.Sandbox
}

// planWorkbenches is application.PlanWorkbenchPort: a workbench rooted at
// the plan's workspace with autonomous local tools, since workers run
// without the person once the plan is approved.
type planWorkbenches struct{ repairWorkbenches }

var _ application.PlanWorkbenchPort = planWorkbenches{}

func (w planWorkbenches) Open(ctx context.Context, workspace string) (application.PlanWorkbench, error) {
	w.autonomous = true
	bench, err := w.repairWorkbenches.Open(ctx, workspace)
	if err != nil {
		return nil, err
	}
	return bench.(*application.UseCaseWorkbench), nil
}

var _ application.RepairWorkbenchPort = repairWorkbenches{}

func (w repairWorkbenches) Open(_ context.Context, clone string) (application.RepairWorkbench, error) {
	executor, err := runtime.New(runtime.Config{Root: clone, Env: w.env, Plugins: w.manager, Sandbox: w.sandbox})
	if err != nil {
		return nil, err
	}
	runner := axlr.ToolRunner{Executor: executor, Diagnostics: w.trace}
	driver := ceremonyDriver(w.registrations, runner, w.labels)
	if driver == nil {
		_ = executor.Close()
		return nil, errors.New("MADE is not connected")
	}
	driver.Files = ceremonyhost.Files{Tools: runner}
	driver.Reviewer = ceremonyhost.Reviewer{Models: w.models, Model: w.reviewerModel}
	driver.Approver = &madesetup.Approver{ConfigPath: w.configPath, Getenv: w.getenv}
	driver.Forge = ceremonyhost.Forge{Checks: ceremonyhost.Checks{Tools: runner}}
	driver.RepairPolicy = w.policy
	driver.Plans, driver.Compact = w.plans, w.compact
	approval := application.RepairToolPolicy{Next: w.approval, AutonomousLocal: w.autonomous}
	continuation := application.ContinueTurnUseCase{Validation: w.validator, Models: w.models, Windows: w.windows, Store: w.store, Diagnostics: w.trace, PluginGuidance: w.catalog.Guidance, PluginSkills: w.catalog, SessionLabels: w.labels, Ceremonies: driver, Calibration: w.calibration, TurnToolCalls: w.turnToolCalls}
	catalog := axlr.ToolCatalog{Plugins: w.manager, Diagnostics: w.trace, Profiles: w.profiles}
	return &application.UseCaseWorkbench{
		Start:    application.StartTurnUseCase{Catalog: catalog, Store: w.store, Continue: continuation, Tools: runner, Approval: approval},
		Resolver: application.ResolveToolUseCase{Validation: w.validator, Tools: runner, Approval: approval, Store: w.store, Continue: continuation, Diagnostics: w.trace},
		Agent:    application.AgentTurnUseCase{Continue: continuation, Tools: runner, Approval: approval},
		Closer:   executor.Close,
	}, nil
}
