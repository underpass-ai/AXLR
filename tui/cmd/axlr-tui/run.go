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

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/adapters/openrouter"
	rootApp "github.com/underpass-ai/AXLR/application"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/adapters/axlr"
	"github.com/underpass-ai/AXLR/tui/adapters/diagnostics"
	catalog "github.com/underpass-ai/AXLR/tui/adapters/openrouter"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/adapters/terminal"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func run(ctx context.Context, args []string, getenv func(string) string, launch func(tea.Model) error, stderr io.Writer) int {
	key := getenv("OPENROUTER_API_KEY")
	var trace application.DiagnosticPort
	fail := func(err error) int {
		if trace != nil {
			_ = trace.Record(application.DiagnosticEvent{Stage: application.DiagnosticOperationFailed, ErrorClass: application.DiagnosticErrorInternal})
		}
		message := err.Error()
		if key != "" {
			message = strings.ReplaceAll(message, key, "[redacted]")
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
	traceFlag := flags.String("trace-file", "", "privacy-safe JSONL diagnostics (default a private file under $XDG_STATE_HOME/axlr/logs)")
	tracePayloads := flags.Bool("trace-payloads", true, "capture redacted request/response bodies in a private per-run directory")
	sessionFlag := flags.String("session", "", "saved session ID")
	mcpConfigFlag := flags.String("mcp-config", "", "absolute MCP configuration path (default $XDG_CONFIG_HOME/axlr/mcp.json)")
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
	if trace != nil {
		var payloads *diagnostics.PayloadRecorder
		if *tracePayloads {
			payloadDirectory, createErr := os.MkdirTemp(filepath.Dir(tracePath), filepath.Base(tracePath)+".payloads-")
			if createErr != nil {
				return fail(createErr)
			}
			payloads, err = diagnostics.NewPayloadRecorder(payloadDirectory, key)
			if err != nil {
				_ = os.Remove(payloadDirectory)
				return fail(err)
			}
			fmt.Fprintln(stderr, "axlr-tui: payloads:", payloadDirectory)
		}
		transport = diagnostics.Transport{Next: transport, Trace: trace, Payloads: payloads}
	}
	clientHTTP := &http.Client{Transport: transport}
	defer clientHTTP.CloseIdleConnections()
	client, err := openrouter.New(openrouter.ClientConfig{APIKey: key, HTTPClient: clientHTTP})
	if err != nil {
		return fail(err)
	}
	configPath := *mcpConfigFlag
	if configPath == "" {
		configBase := getenv("XDG_CONFIG_HOME")
		if !filepath.IsAbs(configBase) {
			home := getenv("HOME")
			if !filepath.IsAbs(home) {
				return fail(errors.New("absolute HOME or XDG_CONFIG_HOME is required for MCP config"))
			}
			configBase = filepath.Join(home, ".config")
		}
		configPath = filepath.Join(configBase, "axlr", "mcp.json")
	} else if _, err := os.Stat(configPath); err != nil {
		return fail(err)
	}
	persisted, err := storage.LoadMCPConfiguration(configPath, getenv)
	if err != nil {
		return fail(err)
	}
	registrations, err := pluginRegistrations(paths, selections, getenv)
	if err != nil {
		return fail(err)
	}
	profiles := append([]domain.PluginProfile(nil), persisted.Profiles...)
	for _, registration := range registrations {
		profiles = append(profiles, domain.PluginProfile{ID: registration.Manifest.ID, Name: root.Text(registration.Manifest.ID), Purpose: domain.PluginPurposeTools, Approval: domain.ApprovalManual})
	}
	registrations = append(persisted.Registrations, registrations...)
	manager, err := plugins.NewManager(registrations)
	if err != nil {
		return fail(err)
	}
	defer manager.Close()
	configStore := storage.MCPConfigStore{Path: configPath}
	pluginManager := axlr.NewPluginManager(manager, profiles, configStore.SaveApproval)
	pluginManager.Diagnostics = trace
	executor, err := runtime.New(runtime.Config{Root: workspacePath, Plugins: manager})
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
	preferences, err := storage.NewModelPreferenceStore(filepath.Join(stateBase, "axlr"))
	if err != nil {
		return fail(err)
	}
	if *modelFlag == "" && *sessionFlag == "" {
		selected, loadErr := preferences.Load(ctx)
		if loadErr != nil {
			fmt.Fprintln(stderr, "axlr-tui: ignoring invalid saved model preference")
		} else {
			model = selected
		}
	}
	store, err := storage.New(filepath.Join(stateBase, "axlr", "sessions"))
	if err != nil {
		return fail(err)
	}
	defer store.Close()
	loggedStore := diagnostics.SessionStore{Next: store, Trace: trace}
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
			session = &created
		}
	}
	continuation := application.ContinueTurnUseCase{Models: axlr.ModelStream{UseCase: rootApp.StreamModelUseCase{Models: client}}, Store: loggedStore, Diagnostics: trace}
	runner := axlr.ToolRunner{Executor: executor, Diagnostics: trace}
	app := terminal.New(terminal.Dependencies{
		Context:         ctx,
		Diagnostics:     trace,
		Plugins:         pluginManager,
		Models:          application.ListModelsUseCase{Catalog: catalog.ModelCatalog{APIKey: key, HTTPClient: clientHTTP}, Diagnostics: trace},
		ModelPreference: preferences,
		Create:          application.CreateSessionUseCase{Store: loggedStore},
		Change:          application.ChangeSessionModelUseCase{Store: loggedStore},
		Workspace:       workspace,
		NewSessionID:    newID,
		Start:           application.StartTurnUseCase{Catalog: axlr.ToolCatalog{Plugins: manager, Diagnostics: trace, Profiles: pluginManager.Profiles}, Store: loggedStore, Continue: continuation, Tools: runner, Approval: pluginManager},
		Resolve:         application.ResolveToolUseCase{Tools: runner, Approval: pluginManager, Store: loggedStore, Continue: continuation, Diagnostics: trace},
		Agent:           application.AgentTurnUseCase{Continue: continuation, Tools: runner, Approval: pluginManager},
		Search:          application.SearchSessionUseCase{},
		Store:           loggedStore,
		Session:         session,
		Monochrome:      getenv("NO_COLOR") != "" || getenv("TERM") == "dumb",
	})
	defer app.Close()
	if err = launch(app); err != nil && !errors.Is(err, context.Canceled) && ctx.Err() == nil {
		return fail(err)
	}
	return 0
}

func pluginRegistrations(paths, selections []string, getenv func(string) string) ([]plugins.Registration, error) {
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
