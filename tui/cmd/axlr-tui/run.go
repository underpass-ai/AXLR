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
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/adapters/openrouter"
	rootApp "github.com/underpass-ai/AXLR/application"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/adapters/axlr"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/adapters/terminal"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func run(ctx context.Context, args []string, getenv func(string) string, launch func(tea.Model) error, stderr io.Writer) int {
	key := getenv("OPENROUTER_API_KEY")
	fail := func(err error) int {
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
	workspaceFlag := flags.String("root", "", "existing workspace root (required)")
	modelFlag := flags.String("model", "", "OpenRouter model ID (required)")
	sessionFlag := flags.String("session", "", "saved session ID")
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
	if *workspaceFlag == "" || *modelFlag == "" || flags.NArg() != 0 {
		return fail(errors.New("--root and --model are required; positional arguments are not accepted"))
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
	model, err := root.NewModelID(*modelFlag)
	if err != nil {
		return fail(err)
	}
	clientHTTP := &http.Client{}
	defer clientHTTP.CloseIdleConnections()
	client, err := openrouter.New(openrouter.ClientConfig{APIKey: key, HTTPClient: clientHTTP})
	if err != nil {
		return fail(err)
	}
	registrations, err := pluginRegistrations(paths, selections, getenv)
	if err != nil {
		return fail(err)
	}
	manager, err := plugins.NewManager(registrations)
	if err != nil {
		return fail(err)
	}
	defer manager.Close()
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
	store, err := storage.New(filepath.Join(stateBase, "axlr", "sessions"))
	if err != nil {
		return fail(err)
	}
	defer store.Close()
	var session domain.Session
	if *sessionFlag != "" {
		id, e := domain.NewSessionID(*sessionFlag)
		if e != nil {
			return fail(e)
		}
		session, err = store.Load(ctx, id)
		if err != nil {
			return fail(err)
		}
		if session.Export().Workspace != workspace || session.Export().Model != model {
			return fail(errors.New("saved session workspace/model differ from --root/--model"))
		}
	} else {
		var random [16]byte
		if _, err = rand.Read(random[:]); err != nil {
			return fail(err)
		}
		id, _ := domain.NewSessionID(hex.EncodeToString(random[:]))
		session, err = domain.NewSession(id, workspace, model)
		if err != nil {
			return fail(err)
		}
		if err = store.Save(ctx, session); err != nil {
			return fail(err)
		}
	}
	continuation := application.ContinueTurnUseCase{Models: axlr.ModelStream{UseCase: rootApp.StreamModelUseCase{Models: client}}, Store: store}
	app := terminal.New(terminal.Dependencies{
		Context:    ctx,
		Start:      application.StartTurnUseCase{Catalog: axlr.ToolCatalog{Plugins: manager}, Store: store, Continue: continuation},
		Resolve:    application.ResolveToolUseCase{Tools: axlr.ToolRunner{Executor: executor}, Store: store, Continue: continuation},
		Agent:      application.AgentTurnUseCase{Continue: continuation},
		Search:     application.SearchSessionUseCase{},
		Store:      store,
		Session:    &session,
		Monochrome: getenv("NO_COLOR") != "" || getenv("TERM") == "dumb",
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
