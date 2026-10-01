package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/adapters/openrouter"
	rootApplication "github.com/underpass-ai/AXLR/application"
	"github.com/underpass-ai/AXLR/buildinfo"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/adapters/axlr"
	"github.com/underpass-ai/AXLR/tui/application"
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
	executor, err := runtime.New(runtime.Config{Root: cfg.Workspace, Plugins: manager})
	if err != nil {
		fmt.Fprintln(stderr, "axlr-serve: workspace unavailable")
		return 1
	}
	defer executor.Close()
	client, err := openrouter.New(openrouter.ClientConfig{APIKey: key, HTTPClient: &http.Client{Timeout: 60 * time.Second}})
	if err != nil {
		fmt.Fprintln(stderr, "axlr-serve: model provider unavailable")
		return 1
	}
	pluginManager := axlr.NewPluginManager(manager, profiles, nil)
	validator := axlr.NewToolArgumentValidator()
	continuation := application.ContinueTurnUseCase{Validation: validator, Models: axlr.ModelStream{UseCase: rootApplication.StreamModelUseCase{Models: client}}}
	runner := axlr.ToolRunner{Executor: executor}
	catalog := axlr.ToolCatalog{Plugins: manager, Profiles: pluginManager.Profiles}
	deps := service.Dependencies{
		Start:   application.StartTurnUseCase{Catalog: catalog, Continue: continuation, Tools: runner, Approval: pluginManager},
		Resolve: application.ResolveToolUseCase{Validation: validator, Tools: runner, Approval: pluginManager, Continue: continuation},
		Catalog: catalog, Tools: runner, Approval: pluginManager, Validation: validator,
		Ready: service.EngineReadiness(manager, key),
	}
	server, err := service.NewServer(cfg, deps)
	if err != nil {
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
