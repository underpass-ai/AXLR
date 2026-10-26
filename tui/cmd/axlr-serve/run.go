package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/adapters/local"
	"github.com/underpass-ai/AXLR/adapters/openrouter"
	rootApplication "github.com/underpass-ai/AXLR/application"
	"github.com/underpass-ai/AXLR/buildinfo"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/adapters/axlr"
	"github.com/underpass-ai/AXLR/tui/adapters/ceremonyhost"
	"github.com/underpass-ai/AXLR/tui/adapters/localmodels"
	catalogModels "github.com/underpass-ai/AXLR/tui/adapters/openrouter"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/compose"
	"github.com/underpass-ai/AXLR/tui/service"
)

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(stdout, buildinfo.Version)
		return 0
	}
	flags := flag.NewFlagSet("axlr-serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	configPath := flags.String("config", "", "absolute JSON service config path")
	probe := flags.String("probe", "", "probe to check: livez or readyz")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *probe != "" {
		if *probe != "livez" && *probe != "readyz" {
			return 2
		}
		client := http.Client{Timeout: 2 * time.Second}
		url := "http://127.0.0.1:8081/" + *probe
		response, err := client.Get(url)
		if err != nil {
			return 1
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			return 1
		}
		return 0
	}
	if *configPath == "" || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "axlr-serve: --config is required")
		return 2
	}
	cfg, err := service.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(stderr, "axlr-serve: invalid configuration")
		return 1
	}
	keyBytes, err := os.ReadFile(cfg.ModelAPIKeyFile)
	if err != nil {
		fmt.Fprintln(stderr, "axlr-serve: model credential unavailable")
		return 1
	}
	key := strings.TrimSpace(string(keyBytes))
	if key == "" {
		fmt.Fprintln(stderr, "axlr-serve: model credential unavailable")
		return 1
	}
	registrations, profiles, err := service.EngineRegistrations(cfg)
	if err != nil {
		fmt.Fprintln(stderr, "axlr-serve: invalid engine configuration")
		return 1
	}
	manager, err := plugins.NewManager(registrations)
	if err != nil {
		fmt.Fprintln(stderr, "axlr-serve: engine registry unavailable")
		return 1
	}
	defer manager.Close()
	settings, err := serviceSettings(ctx, cfg.SettingsFile)
	if err != nil {
		fmt.Fprintln(stderr, "axlr-serve: settings unavailable:", err)
		return 1
	}
	note := func(line string) { fmt.Fprintln(stderr, "axlr-serve:", line) }
	// Local commands run in the console's exec sandbox when settings ask
	// for it: a service takes prompts from its clients, not from the
	// person at the keyboard.
	sandbox, _, err := compose.ExecSandbox(ctx, "axlr-serve", settings.ExecSandbox(), exec.LookPath, stderr)
	if err != nil {
		fmt.Fprintln(stderr, "axlr-serve: exec sandbox:", err)
		return 1
	}
	executor, err := localExecutor(cfg.Workspace, manager, sandbox, os.Getenv)
	if err != nil {
		fmt.Fprintln(stderr, "axlr-serve: workspace unavailable")
		return 1
	}
	defer executor.Close()
	httpClient := modelHTTPClient(60 * time.Second)
	client, err := openrouter.New(openrouter.ClientConfig{APIKey: key, HTTPClient: httpClient, Models: compose.OpenRouterModelOptions(settings)})
	if err != nil {
		fmt.Fprintln(stderr, "axlr-serve: model provider unavailable")
		return 1
	}
	// local_models routes a model to its own OpenAI-compatible server,
	// local or remote over HTTPS with its api_key_env.
	locals := compose.NewLocalModels(settings, os.Getenv)
	routes, err := locals.Connect(httpClient)
	if err != nil {
		fmt.Fprintln(stderr, "axlr-serve:", err)
		return 1
	}
	calibration, err := storage.NewTokenCalibration(filepath.Join(cfg.StateDir, "bytes-per-token.json"))
	if err != nil {
		fmt.Fprintln(stderr, "axlr-serve: state unavailable")
		return 1
	}
	locals.Windows.Calibration = calibration
	judge, err := compose.JevJudge(settings.Jev, os.Getenv)
	if err != nil {
		fmt.Fprintln(stderr, "axlr-serve:", err)
		return 1
	}
	sessions, err := service.OpenSessions(cfg)
	if err != nil {
		fmt.Fprintln(stderr, "axlr-serve: state unavailable")
		return 1
	}
	labels, err := storage.NewSessionLabelStore(filepath.Join(cfg.StateDir, "session-labels.json"))
	if err != nil {
		sessions.Close()
		fmt.Fprintln(stderr, "axlr-serve: state unavailable")
		return 1
	}
	pluginManager := axlr.NewPluginManager(manager, profiles, nil)
	validator := axlr.NewToolArgumentValidator()
	// Each session's commands run under its execution's sandbox, when it
	// has one; the profile none under the settings' exec sandbox.
	sandboxes := compose.NewExecutionSandboxes(settings.ExecSandbox(), sandbox, exec.LookPath)
	runner := axlr.ToolRunner{Executor: executor, Confine: sandboxes.Confine}
	models := axlr.ModelStream{UseCase: rootApplication.StreamModelUseCase{Models: localmodels.Router{Default: client, Routes: routes}}}
	lifecycle, ceremonies, err := sessionLifecycle(ctx, cfg, registrations, settings, runner, models, labels, sessions, locals.Windows, judge, note)
	if err != nil {
		sessions.Close()
		fmt.Fprintln(stderr, "axlr-serve: state unavailable")
		return 1
	}
	continuation := application.ContinueTurnUseCase{Engines: ceremonyhost.Guidance{}, Validation: validator, Models: models, Windows: locals.Windows, Judge: judge, Store: sessions.Store(), Ceremonies: ceremonies, Session: lifecycle, Calibration: calibration, Language: "en"}
	routing := compose.NewModelRouting(locals, key != "", catalogModels.ModelCatalog{APIKey: key, HTTPClient: httpClient})
	// The settings' routing is the default of a session created without one.
	defaultRouting, err := settings.ExecutionRouting()
	if err != nil {
		sessions.Close()
		fmt.Fprintln(stderr, "axlr-serve:", err)
		return 1
	}
	if _, err := compose.Equip(&continuation, compose.Equipment{Settings: settings, State: cfg.StateDir, Store: sessions.Store(), Usage: sessions.Usage(), Receipts: sessions.Receipts(), Labels: labels, Windows: locals.Windows, Tools: runner, Memory: true, Routing: routing, Note: note}); err != nil {
		sessions.Close()
		fmt.Fprintln(stderr, "axlr-serve: tools unavailable:", err)
		return 1
	}
	catalog := axlr.ToolCatalog{Plugins: manager, Profiles: pluginManager.Profiles, RequireWhy: true, Surfaces: ceremonyhost.Surface}
	deps := service.Dependencies{
		Start:   application.StartTurnUseCase{Catalog: catalog, Continue: continuation, Tools: runner, Approval: pluginManager},
		Resolve: application.ResolveToolUseCase{Validation: validator, Tools: runner, Approval: pluginManager, Continue: continuation},
		Catalog: catalog, Tools: runner, Approval: pluginManager, Validation: validator,
		Ready:        service.EngineReadiness(manager, key),
		ProbeSandbox: sandboxes.Probe,
		Routing:      defaultRouting,
		Sessions:     sessions,
		Lifecycle:    lifecycle,
	}
	server, err := service.NewServer(cfg, deps)
	if err != nil {
		sessions.Close()
		fmt.Fprintln(stderr, "axlr-serve: unable to initialize service")
		return 1
	}
	defer server.Close()
	if err := server.Serve(ctx); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintln(stderr, "axlr-serve: listener stopped")
		return 1
	}
	return 0
}

// localExecutor runs the workspace's tools. Its commands get the console's
// environment: the service's absolute PATH folders, or the system's when it
// names none, and HOME; never the model key or any other variable.
func localExecutor(workspace string, manager *plugins.Manager, sandbox *local.Sandbox, getenv func(string) string) (*runtime.Executor, error) {
	return runtime.New(runtime.Config{Root: workspace, Env: compose.ExecEnvironment(getenv), Plugins: manager, Sandbox: sandbox})
}

// modelHTTPClient bounds connecting and the wait for the provider's response
// headers. It sets no Client.Timeout: that would also cover reading the body
// and cut every streamed answer longer than the bound while tokens still
// arrive. The adapter's own inactivity and total-duration limits govern the
// stream.
func modelHTTPClient(headerTimeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = headerTimeout
	transport.TLSHandshakeTimeout = min(transport.TLSHandshakeTimeout, headerTimeout)
	transport.DialContext = (&net.Dialer{Timeout: min(30*time.Second, headerTimeout), KeepAlive: 30 * time.Second}).DialContext
	return &http.Client{Transport: transport}
}
