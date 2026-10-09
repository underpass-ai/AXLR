package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/adapters/openrouter"
	rootApp "github.com/underpass-ai/AXLR/application"
	"github.com/underpass-ai/AXLR/buildinfo"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/adapters/axlr"
	"github.com/underpass-ai/AXLR/tui/adapters/axlrplugin"
	"github.com/underpass-ai/AXLR/tui/adapters/ceremonyhost"
	"github.com/underpass-ai/AXLR/tui/adapters/diagnostics"
	"github.com/underpass-ai/AXLR/tui/adapters/engineupdate"
	"github.com/underpass-ai/AXLR/tui/adapters/localmodels"
	"github.com/underpass-ai/AXLR/tui/adapters/madesetup"
	catalog "github.com/underpass-ai/AXLR/tui/adapters/openrouter"
	"github.com/underpass-ai/AXLR/tui/adapters/repairclone"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/adapters/terminal"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func run(ctx context.Context, args []string, getenv func(string) string, launch func(tea.Model) error, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(stderr, buildinfo.Version)
		return 0
	}
	key := getenv("OPENROUTER_API_KEY")
	var trace application.DiagnosticPort
	var localKeys []string
	fail := func(err error) int {
		if trace != nil {
			_ = trace.Record(application.DiagnosticEvent{Stage: application.DiagnosticOperationFailed, ErrorClass: application.DiagnosticErrorInternal})
		}
		message := err.Error()
		for _, secret := range append([]string{key}, localKeys...) {
			if secret != "" {
				message = strings.ReplaceAll(message, secret, "[redacted]")
			}
		}
		fmt.Fprintln(stderr, "axlr-tui:", message)
		return 1
	}
	if ctx.Err() != nil {
		return 0
	}
	flags := flag.NewFlagSet("axlr-tui", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	workspaceFlag := flags.String("root", ".", "existing workspace root (default current directory)")
	modelFlag := flags.String("model", "", "OpenRouter model ID (optional; choose with /model)")
	langFlag := flags.String("lang", "", "interface language: en or es (overrides AXLR_LANG and settings.json)")
	traceFlag := flags.String("trace-file", "", "privacy-safe JSONL diagnostics (default a private file under $XDG_STATE_HOME/axlr/logs)")
	tracePayloads := flags.Bool("trace-payloads", false, "capture redacted request/response bodies, prompts and tool results included, in a private per-run directory (default: trace_payloads in settings.json, else off)")
	sessionFlag := flags.String("session", "", "saved session ID")
	mcpConfigFlag := flags.String("mcp-config", "", "absolute MCP configuration path (default $XDG_CONFIG_HOME/axlr/mcp.json)")
	repairFlag := flags.String("repair", "", "start a repair of the configured repository in a fresh clone: a failure brief or #issue")
	improveFlag := flags.String("improve", "", "start an improvement of the configured repository in a fresh clone: an improvement brief or #issue; the merge waits for you")
	var paths, selections []string
	flags.Func("plugin", "absolute MCP manifest path; repeatable", func(v string) error { paths = append(paths, v); return nil })
	flags.Func("plugin-env-from", "ID:KEY=HOST_ENV_VAR; repeatable", func(v string) error { selections = append(selections, v); return nil })
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			flags.SetOutput(stderr)
			flags.PrintDefaults()
			return 0
		}
		return fail(err)
	}
	if flags.NArg() != 0 {
		return fail(errors.New("positional arguments are not accepted"))
	}
	tracePath := *traceFlag
	var logger *diagnostics.FileLogger
	var openErr error
	if tracePath == "" {
		logger, tracePath, openErr = diagnostics.OpenDefault(getenv)
	} else {
		tracePath, openErr = filepath.Abs(tracePath)
		if openErr == nil {
			logger, openErr = diagnostics.Open(tracePath)
		}
	}
	if openErr != nil {
		return fail(openErr)
	}
	defer func() {
		if logger.Close() != nil {
			fmt.Fprintln(stderr, "axlr-tui: diagnostic trace incomplete")
		}
	}()
	trace = logger
	fmt.Fprintln(stderr, "axlr-tui: diagnostics:", tracePath)
	_ = trace.Record(application.DiagnosticEvent{Stage: application.DiagnosticStartup})
	defer trace.Record(application.DiagnosticEvent{Stage: application.DiagnosticShutdown})
	configBase := getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(configBase) {
		home := getenv("HOME")
		if !filepath.IsAbs(home) {
			return fail(errors.New("absolute HOME or XDG_CONFIG_HOME is required for settings"))
		}
		configBase = filepath.Join(home, ".config")
	}
	settingsPath := filepath.Join(configBase, "axlr", "settings.json")
	initialSettings := storage.DefaultUserSettings()
	if _, statErr := os.Lstat(settingsPath); errors.Is(statErr, os.ErrNotExist) {
		stateBase := getenv("XDG_STATE_HOME")
		if !filepath.IsAbs(stateBase) {
			home := getenv("HOME")
			if !filepath.IsAbs(home) {
				return fail(errors.New("absolute HOME or XDG_STATE_HOME is required for preferences"))
			}
			stateBase = filepath.Join(home, ".local", "state")
		}
		legacyDir := filepath.Join(stateBase, "axlr")
		legacyUI, legacyErr := storage.NewUIPreferenceStore(legacyDir)
		if legacyErr != nil {
			return fail(legacyErr)
		}
		ui, loadErr := legacyUI.Load(ctx)
		if loadErr == nil {
			initialSettings.Theme = string(ui.Theme)
			initialSettings.Icons = string(ui.Icons)
			initialSettings.ReduceMotion = ui.ReduceMotion
		} else {
			fmt.Fprintln(stderr, "axlr-tui: ignoring invalid saved UI preference")
		}
		legacyModel, legacyErr := storage.NewModelPreferenceStore(legacyDir)
		if legacyErr != nil {
			return fail(legacyErr)
		}
		selected, loadErr := legacyModel.Load(ctx)
		if loadErr == nil {
			initialSettings.Model = string(selected)
		} else if *modelFlag == "" && *sessionFlag == "" {
			fmt.Fprintln(stderr, "axlr-tui: ignoring invalid saved model preference")
		}
	} else if statErr != nil {
		return fail(statErr)
	}
	settingsStore, err := storage.NewUserSettingsStore(settingsPath, initialSettings)
	if err != nil {
		return fail(err)
	}
	settings, err := settingsStore.Load(ctx)
	if err != nil {
		return fail(err)
	}
	fmt.Fprintln(stderr, "axlr-tui: settings:", settingsPath)
	locals := newLocalModelSetup(settings, getenv)
	localKeys = locals.secrets
	judge, err := jevJudge(settings.Jev, getenv)
	if err != nil {
		return fail(err)
	}
	if judge != nil {
		localKeys = append(localKeys, strings.TrimSpace(getenv("TYPESAFE_API_KEY")))
		fmt.Fprintln(stderr, "axlr-tui: Jev enabled: questions and final answers are sent to TypeSafe")
	}
	language := settings.Language
	if fromEnvironment := getenv("AXLR_LANG"); fromEnvironment != "" {
		language = fromEnvironment
	}
	if *langFlag != "" {
		language = *langFlag
	}
	locale, err := terminal.ParseLocale(language)
	if err != nil {
		return fail(err)
	}
	dataBase := getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(dataBase) {
		home := getenv("HOME")
		if !filepath.IsAbs(home) {
			return fail(errors.New("absolute HOME or XDG_DATA_HOME is required for AXLR plugins"))
		}
		dataBase = filepath.Join(home, ".local", "share")
	}
	repairConfiguration := settings.RepairConfiguration()
	initialDraft := ""
	// --repair and --improve open the console in a fresh clone, in the mode
	// whose ceremony ends in the console's pull request.
	launchFlag, launchMode, launchKind, launchRequest := "", domain.WorkMode(""), "", ""
	switch {
	case *repairFlag != "" && *improveFlag != "":
		return fail(errors.New("--repair and --improve cannot be combined"))
	case *repairFlag != "":
		launchFlag, launchMode, launchRequest = "--repair", domain.ModeRepair, *repairFlag
	case *improveFlag != "":
		launchFlag, launchMode, launchKind, launchRequest = "--improve", domain.ModeImprove, application.ImproveKind, *improveFlag
	}
	if launchMode != "" {
		if *sessionFlag != "" {
			return fail(fmt.Errorf("%s starts a new session; it cannot be combined with --session", launchFlag))
		}
		clone, brief, err := prepareRepairClone(ctx, repairConfiguration, filepath.Join(dataBase, "axlr", "repairs"), launchKind, launchRequest, localRuntimeEnvironment(getenv), stderr)
		if err != nil {
			return fail(err)
		}
		*workspaceFlag, initialDraft = clone, brief
	}
	workspacePath, err := filepath.Abs(*workspaceFlag)
	if err != nil {
		return fail(err)
	}
	workspacePath, err = filepath.EvalSymlinks(workspacePath)
	if err != nil {
		return fail(err)
	}
	workspace, err := domain.NewWorkspace(workspacePath)
	if err != nil {
		return fail(err)
	}
	var model root.ModelID
	if *modelFlag != "" {
		model, err = root.NewModelID(*modelFlag)
		if err != nil {
			return fail(err)
		}
	}
	var transport http.RoundTripper = http.DefaultTransport
	var payloadDirectory string
	if trace != nil {
		var payloads *diagnostics.PayloadRecorder
		if capturePayloads(flags, *tracePayloads, settings) {
			payloadParent, note, parentErr := payloadParent(workspacePath, tracePath, getenv)
			if parentErr != nil {
				return fail(parentErr)
			}
			if note != "" {
				fmt.Fprintln(stderr, "axlr-tui:", note)
			}
			var createErr error
			payloadDirectory, createErr = os.MkdirTemp(payloadParent, filepath.Base(tracePath)+".payloads-")
			if createErr != nil {
				return fail(createErr)
			}
			payloads, err = diagnostics.NewPayloadRecorder(payloadDirectory, payloadSecrets(key, localKeys)...)
			if err != nil {
				_ = os.Remove(payloadDirectory)
				return fail(err)
			}
			fmt.Fprintln(stderr, "axlr-tui: payload capture is on: request and response bodies, prompts and tool results included, are stored in", payloadDirectory)
		}
		transport = diagnostics.Transport{Next: transport, Trace: trace, Payloads: payloads, Endpoints: locals.endpoints}
	}
	if *traceFlag == "" {
		// Every launch adds a trace and a payload directory to the default
		// directory; earlier ones past trace_retention_days go, silently.
		_ = diagnostics.PruneDefault(getenv, settings.TraceRetention(), time.Now(), tracePath, payloadDirectory)
	}
	clientHTTP := &http.Client{Transport: transport}
	defer clientHTTP.CloseIdleConnections()
	// OpenRouter is optional once local models are configured; without a
	// key and without local models the client reports the missing key.
	var remote localmodels.Client
	var remoteCatalog application.ModelCatalogPort
	if key != "" || len(locals.models) == 0 {
		client, err := openrouter.New(openrouter.ClientConfig{APIKey: key, HTTPClient: clientHTTP, Models: openRouterModelOptions(settings)})
		if err != nil {
			return fail(err)
		}
		remote = client
		remoteCatalog = catalog.ModelCatalog{APIKey: key, HTTPClient: clientHTTP}
	}
	routes, err := locals.connect(clientHTTP)
	if err != nil {
		return fail(err)
	}
	router := localmodels.Router{Default: remote, Routes: routes}
	configPath := *mcpConfigFlag
	if configPath == "" {
		configPath = filepath.Join(configBase, "axlr", "mcp.json")
	} else if _, err := os.Stat(configPath); err != nil {
		return fail(err)
	}
	var refusedKey string
	persisted, err := storage.LoadMCPConfiguration(configPath, locals.pluginGetenv(getenv, &refusedKey))
	if err == nil && refusedKey != "" {
		err = fmt.Errorf("model key %s cannot be passed to plugins", refusedKey)
	}
	if err != nil {
		return fail(err)
	}
	registrations, err := pluginRegistrations(paths, selections, getenv, locals.keyEnvs)
	if err != nil {
		return fail(err)
	}
	profiles := append([]domain.PluginProfile(nil), persisted.Profiles...)
	for _, registration := range registrations {
		profiles = append(profiles, domain.PluginProfile{ID: registration.Manifest.ID, Name: root.Text(registration.Manifest.ID), Purpose: domain.PluginPurposeTools, Approval: domain.ApprovalManual})
	}
	registrations = append(persisted.Registrations, registrations...)
	activeEngineCommands := map[string]string{}
	for _, registration := range registrations {
		activeEngineCommands[registration.Manifest.ID.String()] = registration.Manifest.Command
	}
	manager, err := plugins.NewManager(registrations)
	if err != nil {
		return fail(err)
	}
	defer manager.Close()
	configStore := storage.MCPConfigStore{Path: configPath}
	pluginManager := axlr.NewPluginManager(manager, profiles, configStore.SaveApproval)
	pluginManager.SetInstaller(configStore.AddManifest)
	pluginManager.SetEnvironmentInstaller(configStore.AddManifestWithEnvironment, locals.pluginGetenv(getenv, nil))
	pluginManager.SetURLInstaller(configStore.AddURL)
	pluginManager.Diagnostics = trace
	approvalSettings, err := storage.NewApprovalSettingsInUserSettings(settingsStore, filepath.Join(filepath.Dir(configPath), "approvals.json"), pluginManager)
	if err != nil {
		return fail(err)
	}
	executor, err := runtime.New(runtime.Config{Root: workspacePath, Env: localRuntimeEnvironment(getenv), Plugins: manager})
	if err != nil {
		return fail(err)
	}
	defer executor.Close()
	stateBase := getenv("XDG_STATE_HOME")
	if !filepath.IsAbs(stateBase) {
		home := getenv("HOME")
		if !filepath.IsAbs(home) {
			return fail(errors.New("absolute HOME or XDG_STATE_HOME is required for sessions"))
		}
		stateBase = filepath.Join(home, ".local", "state")
	}
	// What each model's prompt tokens measured sizes its prompt budget.
	calibration, err := storage.NewTokenCalibration(filepath.Join(stateBase, "axlr", "bytes-per-token.json"))
	if err != nil {
		return fail(err)
	}
	locals.windows.Calibration = calibration
	preferences := settingsStore.ModelPreference()
	uiStore := settingsStore.UIPreference()
	uiPreferences := settings.UIPreferences()
	if *modelFlag == "" && *sessionFlag == "" {
		model = root.ModelID(settings.Model)
	}
	store, err := storage.New(filepath.Join(stateBase, "axlr", "sessions"))
	if err != nil {
		return fail(err)
	}
	defer store.Close()
	loggedStore := diagnostics.SessionStore{Next: store, Trace: trace}
	sessionLabels, err := storage.NewSessionLabelStore(filepath.Join(stateBase, "axlr", "session-labels.json"))
	if err != nil {
		return fail(err)
	}
	var session *domain.Session
	var newID domain.SessionID
	if *sessionFlag != "" {
		id, e := domain.NewSessionID(*sessionFlag)
		if e != nil {
			return fail(e)
		}
		loaded, loadErr := loggedStore.Load(ctx, id)
		if loadErr != nil {
			return fail(loadErr)
		}
		session = &loaded
		if session.Export().Workspace != workspace || (*modelFlag != "" && session.Export().Model != model) {
			return fail(errors.New("saved session workspace/model differ from --root/--model"))
		}
	} else {
		var random [16]byte
		if _, err = rand.Read(random[:]); err != nil {
			return fail(err)
		}
		newID, _ = domain.NewSessionID(hex.EncodeToString(random[:]))
		if model != "" {
			created, createErr := (application.CreateSessionUseCase{Store: loggedStore}).Execute(ctx, newID, workspace, model)
			if createErr != nil {
				return fail(createErr)
			}
			if launchMode != "" {
				if err := created.SetMode(launchMode); err != nil {
					return fail(err)
				}
				if err := loggedStore.Save(ctx, created); err != nil {
					return fail(err)
				}
			}
			session = &created
		} else if launchMode != "" {
			return fail(fmt.Errorf("%s needs a model: pass --model or choose one in settings.json", launchFlag))
		}
	}
	axlrCatalog := &axlrplugin.Catalog{Root: filepath.Join(dataBase, "axlr"), MCP: pluginManager}
	validator := axlr.NewToolArgumentValidator()
	runner := axlr.ToolRunner{Executor: executor, Diagnostics: trace}
	models := axlr.ModelStream{UseCase: rootApp.StreamModelUseCase{Models: router}}
	ceremonies := ceremonyDriver(registrations, runner, sessionLabels)
	if ceremonies != nil {
		ceremonies.Compact = compactProfile(settings.CeremonyProfile(), locals.windows)
		plans, err := storage.NewPlanRegistry(filepath.Join(stateBase, "axlr", "plans.json"))
		if err != nil {
			return fail(err)
		}
		ceremonies.Plans = plans
		planner := settings.Planner()
		if _, local := routes[root.ModelID(planner)]; planner != "" && !local && key == "" {
			fmt.Fprintf(stderr, "axlr-tui: plan.model %s needs OPENROUTER_API_KEY; /plan uses the session model\n", planner)
			planner = ""
		}
		ceremonies.Plan = application.PlanSettings{Planner: planner, AutoApprove: settings.Plan != nil && settings.Plan.AutoApprove}
	}
	repairPolicy := application.RepairPolicy{AutoMerge: repairConfiguration.AutoMerge, WatchDeadline: time.Duration(repairConfiguration.WatchMinutes) * time.Minute}
	if ceremonies != nil {
		ceremonies.Files = ceremonyhost.Files{Tools: runner}
		ceremonies.Reviewer = ceremonyhost.Reviewer{Models: models, Model: settings.ReviewerModel}
		ceremonies.Approver = &madesetup.Approver{ConfigPath: configPath, Getenv: getenv}
		ceremonies.Forge = ceremonyhost.Forge{Checks: ceremonyhost.Checks{Tools: runner}}
		ceremonies.RepairPolicy = repairPolicy
	}
	// Self-repair: the agent of any session may ask the console to repair
	// AXLR in a separate session rooted in a fresh clone. The coordinator
	// exists whenever MADE is connected; without it the host tool refuses
	// with the reason.
	var repairs *application.SelfRepair
	if ceremonies != nil {
		registry, err := storage.NewRepairRegistry(filepath.Join(stateBase, "axlr", "repairs.json"))
		if err != nil {
			return fail(err)
		}
		repairsDirectory := repairConfiguration.Directory
		if repairsDirectory == "" {
			repairsDirectory = filepath.Join(dataBase, "axlr", "repairs")
		}
		var token [8]byte
		if _, err := rand.Read(token[:]); err != nil {
			return fail(err)
		}
		repairs = &application.SelfRepair{
			Registry:  registry,
			Clones:    repairclone.Preparer{Repairs: repairsDirectory, Env: localRuntimeEnvironment(getenv), Stderr: stderr},
			Workbench: repairWorkbenches{env: localRuntimeEnvironment(getenv), manager: manager, registrations: registrations, labels: sessionLabels, models: models, windows: locals.windows, store: loggedStore, trace: trace, validator: validator, approval: approvalSettings, profiles: pluginManager.Profiles, catalog: axlrCatalog, configPath: configPath, getenv: getenv, reviewerModel: settings.ReviewerModel, policy: repairPolicy, autonomous: repairConfiguration.AutonomousLocal(), calibration: calibration},
			Store:     loggedStore,
			Engine:    ceremonies.Engine,
			Settings:  application.RepairSettings{Repository: repairConfiguration.Repository, About: repairConfiguration.About, Directory: repairsDirectory, MaxAttempts: repairConfiguration.MaxAttempts},
			Build:     buildinfo.Version,
			RunToken:  hex.EncodeToString(token[:]),
			Lifetime:  ctx,
		}
		if err := repairs.Reconcile(ctx); err != nil {
			fmt.Fprintln(stderr, "axlr-tui: repair registry:", err)
		}
		defer repairs.Close()
	}
	// Plans: an approved plan runs its tasks in fresh worker sessions rooted
	// at the workspace, one after another, and a sync after each wave.
	var planRunner *application.PlanRunner
	var planPanel terminal.PlanPanelPort
	if ceremonies != nil && ceremonies.Plans != nil {
		var token [8]byte
		if _, err := rand.Read(token[:]); err != nil {
			return fail(err)
		}
		benches := planWorkbenches{repairWorkbenches{env: localRuntimeEnvironment(getenv), manager: manager, registrations: registrations, labels: sessionLabels, models: models, windows: locals.windows, store: loggedStore, trace: trace, validator: validator, approval: approvalSettings, profiles: pluginManager.Profiles, catalog: axlrCatalog, configPath: configPath, getenv: getenv, reviewerModel: settings.ReviewerModel, policy: repairPolicy, plans: ceremonies.Plans, compact: ceremonies.Compact, calibration: calibration}}
		planRunner = &application.PlanRunner{Plans: ceremonies.Plans, Store: loggedStore, Workbench: benches, RunToken: hex.EncodeToString(token[:]), Lifetime: ctx}
		if err := planRunner.Reconcile(ctx); err != nil {
			fmt.Fprintln(stderr, "axlr-tui: plan registry:", err)
		}
		defer planRunner.Close()
		ceremonies.Starter = planRunner
		planPanel = planRunner
	}
	continuation := application.ContinueTurnUseCase{Validation: validator, Models: models, Windows: locals.windows, Judge: judge, Store: loggedStore, Diagnostics: trace, PluginGuidance: axlrCatalog.Guidance, PluginSkills: axlrCatalog, SessionLabels: sessionLabels, Ceremonies: ceremonies, Calibration: calibration}
	var notices application.RepairNoticesPort
	if repairs != nil {
		continuation.SelfRepair = repairs
		notices = repairs
	}
	app := terminal.New(terminal.Dependencies{
		Context:           ctx,
		Diagnostics:       trace,
		Plugins:           pluginManager,
		InstalledPlugins:  axlrCatalog,
		EngineUpdates:     &engineupdate.Updater{Configuration: &configStore, Root: filepath.Join(dataBase, "axlr", "engines"), ActiveCommands: activeEngineCommands},
		MADEPreparation:   &madesetup.Preparer{ConfigPath: configPath, Getenv: getenv, Store: &configStore},
		Models:            application.ListModelsUseCase{Catalog: localmodels.Catalog{Local: locals.available, Remote: remoteCatalog}, Diagnostics: trace},
		ModelPreference:   preferences,
		SessionLabels:     sessionLabels,
		ModelFavorites:    settingsStore.ModelFavorites(),
		UIPreferenceStore: uiStore,
		UIPreferences:     uiPreferences,
		ApprovalSettings:  approvalSettings,
		Locale:            locale,
		Create:            application.CreateSessionUseCase{Store: loggedStore},
		Change:            application.ChangeSessionModelUseCase{Store: loggedStore},
		Workspace:         workspace,
		NewSessionID:      newID,
		Start:             application.StartTurnUseCase{Catalog: axlr.ToolCatalog{Plugins: manager, Diagnostics: trace, Profiles: pluginManager.Profiles}, Store: loggedStore, Continue: continuation, Tools: runner, Approval: approvalSettings, Notices: notices},
		Resolve:           application.ResolveToolUseCase{Validation: validator, Tools: runner, Approval: approvalSettings, Store: loggedStore, Continue: continuation, Diagnostics: trace},
		Agent:             application.AgentTurnUseCase{Continue: continuation, Tools: runner, Approval: approvalSettings},
		Search:            application.SearchSessionUseCase{},
		Store:             loggedStore,
		Session:           session,
		Monochrome:        getenv("NO_COLOR") != "" || getenv("TERM") == "dumb",
		InitialDraft:      initialDraft,
		Repairs:           repairPanelPort(repairs),
		Plans:             planPanel,
	})
	defer app.Close()
	if err = launch(app); err != nil && !errors.Is(err, context.Canceled) && ctx.Err() == nil {
		return fail(err)
	}
	return 0
}

// The local executor needs PATH to resolve commands, but receives no other
// ambient variables (in particular no model provider credentials).
func localRuntimeEnvironment(getenv func(string) string) []string {
	paths := make([]string, 0)
	seen := map[string]bool{}
	for _, dir := range filepath.SplitList(getenv("PATH")) {
		if filepath.IsAbs(dir) && !seen[dir] {
			paths = append(paths, dir)
			seen[dir] = true
		}
	}
	if len(paths) == 0 {
		paths = []string{"/usr/local/bin", "/usr/bin", "/bin"}
	}
	env := []string{"PATH=" + strings.Join(paths, string(os.PathListSeparator))}
	if home := getenv("HOME"); filepath.IsAbs(home) {
		env = append(env, "HOME="+home)
	}
	return env
}

func pluginRegistrations(paths, selections []string, getenv func(string) string, modelKeys map[string]bool) ([]plugins.Registration, error) {
	environments := map[root.PluginID][]string{}
	for _, selection := range selections {
		rawID, variable, ok := strings.Cut(selection, ":")
		id, err := root.NewPluginID(rawID)
		name, source, assigned := strings.Cut(variable, "=")
		if !ok || err != nil || !assigned || !environmentName(name) || !environmentName(source) {
			return nil, errors.New("invalid --plugin-env-from; expected ID:KEY=HOST_ENV_VAR")
		}
		if source == "OPENROUTER_API_KEY" {
			return nil, errors.New("OpenRouter key cannot be passed to plugins")
		}
		if modelKeys[source] {
			return nil, fmt.Errorf("model key %s cannot be passed to plugins", source)
		}
		environments[id] = append(environments[id], name+"="+getenv(source))
	}
	result := make([]plugins.Registration, 0, len(paths))
	for _, path := range paths {
		manifest, err := plugins.LoadManifest(path)
		if err != nil {
			return nil, err
		}
		registration, err := plugins.NewRegistration(manifest, environments[manifest.ID])
		if err != nil {
			return nil, err
		}
		result = append(result, registration)
		delete(environments, manifest.ID)
	}
	if len(environments) != 0 {
		return nil, errors.New("--plugin-env-from names an unregistered plugin")
	}
	return result, nil
}
func environmentName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (i == 0 || r < '0' || r > '9') {
			return false
		}
	}
	return true
}

// repairPanelPort hides a nil coordinator behind a nil interface, so the
// console shows no repairs panel without MADE.
func repairPanelPort(repairs *application.SelfRepair) terminal.RepairPanelPort {
	if repairs == nil {
		return nil
	}
	return repairs
}

// ceremonyDriver drives /debug, /delivery and /incident when MADE is connected. KMP is
// optional: without it ceremonies run without memory.
func ceremonyDriver(registrations []plugins.Registration, tools application.ToolExecutionPort, labels application.SessionLabelsPort) *application.CeremonyDriver {
	connected := map[string]bool{}
	for _, registration := range registrations {
		connected[registration.Manifest.ID.String()] = true
	}
	if !connected["made"] {
		return nil
	}
	driver := &application.CeremonyDriver{Engine: ceremonyhost.Engine{Tools: tools}, Checks: ceremonyhost.Checks{Tools: tools}, Labels: labels}
	if connected["kmp"] {
		driver.Memory = ceremonyhost.Memory{Tools: tools}
	}
	return driver
}

// payloadSecrets lists the non-empty keys the payload recorder redacts.
// payloadParent is where a launch's payload directory goes: beside the trace,
// unless that is inside the workspace. Payloads hold whole requests, tool
// results included, so a model searching the workspace would read its own
// earlier requests and send them back, larger each time. The note says where
// they went instead, or that no place outside the workspace was found.
func payloadParent(workspace, tracePath string, getenv func(string) string) (string, string, error) {
	parent := filepath.Dir(tracePath)
	resolved, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return "", "", err
	}
	if !pathWithin(workspace, resolved) {
		return parent, "", nil
	}
	fallback, err := diagnostics.DefaultDirectory(getenv)
	if err != nil {
		return "", "", err
	}
	if resolvedFallback, err := filepath.EvalSymlinks(fallback); err != nil {
		return "", "", err
	} else if pathWithin(workspace, resolvedFallback) {
		return parent, "payloads are inside the workspace, where the model's tools can read them; use --trace-payloads=false or a workspace that does not contain the state directory", nil
	}
	return fallback, "the trace is inside the workspace; payloads go to " + fallback + " so the model's tools do not read them", nil
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func payloadSecrets(key string, local []string) []string {
	secrets := make([]string, 0, 1+len(local))
	for _, secret := range append([]string{key}, local...) {
		if secret != "" {
			secrets = append(secrets, secret)
		}
	}
	return secrets
}
