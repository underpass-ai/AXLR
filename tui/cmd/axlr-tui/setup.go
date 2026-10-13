package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/enginesetup"
	"github.com/underpass-ai/AXLR/tui/adapters/engineupdate"
	"github.com/underpass-ai/AXLR/tui/adapters/madesetup"
	catalog "github.com/underpass-ai/AXLR/tui/adapters/openrouter"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// setupCommand runs the setup assistant again: axlr-tui setup.
const setupCommand = "setup"

// setupTerminal reports whether the person can answer the assistant: its
// questions go to stderr and the answers come from stdin. The console's own
// tests turn it off.
var setupTerminal = func() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stderr.Fd())
}

// lightModels are the light models the assistant offers first when
// OpenRouter lists them with tool calls: GLM 5.3 Flash is the model the
// console's own tests and examples run on; Claude Haiku 5.5 the one its
// memory tests ran on. The catalog's cheaper flash/mini/lite models follow.
var lightModels = []string{"z-ai/glm-5.3-flash", "anthropic/claude-haiku-5.5"}

// localPorts are the loopback ports of the usual OpenAI-compatible servers:
// llama.cpp (8080), vLLM (8000), Ollama (11434), LM Studio (1234) and the
// neighbours of 8080 that run a second or third model.
var localPorts = []int{8080, 8081, 8082, 8000, 11434, 1234}

var errSetupInterrupted = errors.New("setup interrupted; run axlr-tui setup to finish it")

type setupPaths struct {
	settings, credentials, mcp, data string
}

// resolveSetupPaths finds settings.json, credentials.json, mcp.json and the
// data directory exactly as the console does.
func resolveSetupPaths(getenv func(string) string, mcpConfig string) (setupPaths, error) {
	home := getenv("HOME")
	base := func(variable string, fallback ...string) (string, error) {
		if dir := getenv(variable); filepath.IsAbs(dir) {
			return dir, nil
		}
		if !filepath.IsAbs(home) {
			return "", fmt.Errorf("absolute HOME or %s is required", variable)
		}
		return filepath.Join(append([]string{home}, fallback...)...), nil
	}
	config, err := base("XDG_CONFIG_HOME", ".config")
	if err != nil {
		return setupPaths{}, err
	}
	data, err := base("XDG_DATA_HOME", ".local", "share")
	if err != nil {
		return setupPaths{}, err
	}
	if mcpConfig == "" {
		mcpConfig = filepath.Join(config, "axlr", "mcp.json")
	}
	return setupPaths{
		settings:    filepath.Join(config, "axlr", "settings.json"),
		credentials: filepath.Join(config, "axlr", "credentials.json"),
		mcp:         mcpConfig,
		data:        data,
	}, nil
}

// runSetupCommand is axlr-tui setup: the assistant on demand, for a person
// who skipped a step or wants another model or the engines.
func runSetupCommand(ctx context.Context, args []string, getenv func(string) string, stdin io.Reader, stderr io.Writer) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: axlr-tui setup")
		return 2
	}
	if !setupTerminal() {
		fmt.Fprintln(stderr, "axlr-tui: setup asks questions and needs an interactive terminal")
		return 1
	}
	paths, err := resolveSetupPaths(getenv, "")
	if err != nil {
		fmt.Fprintln(stderr, "axlr-tui:", err)
		return 1
	}
	settings, err := storage.NewUserSettingsStore(paths.settings, storage.DefaultUserSettings())
	if err != nil {
		fmt.Fprintln(stderr, "axlr-tui:", err)
		return 1
	}
	if err := newSetupWizard(getenv, paths, settings, stdin, stderr).run(ctx); err != nil {
		fmt.Fprintln(stderr, "axlr-tui:", err)
		return 1
	}
	return 0
}

type setupWizard struct {
	in       *bufio.Reader
	out      io.Writer
	getenv   func(string) string
	paths    setupPaths
	settings *storage.UserSettingsStore
	lang     string
	// pause waits for Enter after the summary, before the console opens.
	pause bool
	// Seams the tests replace: hidden input, the OpenRouter catalog, the
	// local servers' discovery and the engine installation.
	secret  func() (string, error)
	catalog func(ctx context.Context, key string) ([]domain.AvailableModel, error)
	probe   func(ctx context.Context, base string) []localCandidate
	engines func(kmpApproval string) application.EngineUpdatePort
	bases   []string
}

func newSetupWizard(getenv func(string) string, paths setupPaths, settings *storage.UserSettingsStore, stdin io.Reader, out io.Writer) *setupWizard {
	client := &http.Client{Timeout: 30 * time.Second}
	w := &setupWizard{in: bufio.NewReader(stdin), out: out, getenv: getenv, paths: paths, settings: settings}
	w.secret = func() (string, error) {
		file, ok := stdin.(*os.File)
		if !ok || !term.IsTerminal(file.Fd()) {
			return w.line()
		}
		return readHiddenLine(file, out)
	}
	w.catalog = func(ctx context.Context, key string) ([]domain.AvailableModel, error) {
		return catalog.ModelCatalog{APIKey: key, HTTPClient: client}.List(ctx)
	}
	w.probe = func(ctx context.Context, base string) []localCandidate {
		return probeLocalServer(ctx, &http.Client{Timeout: 2 * time.Second}, base)
	}
	w.engines = func(kmpApproval string) application.EngineUpdatePort {
		store := &storage.MCPConfigStore{Path: paths.mcp}
		return &engineupdate.Updater{
			Configuration: store,
			Root:          filepath.Join(paths.data, "axlr", "engines"),
			Install: &enginesetup.Installer{
				Configuration: store, Getenv: getenv, KMPApproval: kmpApproval,
				PrepareMADE: (&madesetup.Preparer{ConfigPath: paths.mcp, Getenv: getenv, Store: store}).Prepare,
			},
		}
	}
	for _, port := range localPorts {
		w.bases = append(w.bases, fmt.Sprintf("http://127.0.0.1:%d/v1", port))
	}
	return w
}

func (w *setupWizard) run(ctx context.Context) error {
	w.lang = setupLanguage(w.getenv)
	fmt.Fprintf(w.out, "\n%s\n%s\n", w.t("title"), w.t("intro"))
	choice, err := w.choose(w.t("language.question"), []string{"English", "Español"}, map[string]int{"en": 1, "es": 2}[w.lang])
	if err != nil {
		return err
	}
	w.lang = []string{"en", "es"}[choice-1]
	if err := w.settings.Change(ctx, func(s *storage.UserSettings) { s.Language = w.lang }); err != nil {
		return err
	}
	if err := w.chooseModel(ctx); err != nil {
		return err
	}
	if err := w.installEngines(ctx); err != nil {
		return err
	}
	fmt.Fprintf(w.out, "\n%s\n", w.t("done", w.paths.settings))
	if w.pause {
		// The console takes the whole screen next; the summary stays until
		// the person has read it.
		fmt.Fprint(w.out, w.t("continue"))
		if _, err := w.line(); err != nil {
			return err
		}
	}
	return nil
}

// chooseModel offers OpenRouter, a local server or nothing, and saves the
// choice as the default model.
func (w *setupWizard) chooseModel(ctx context.Context) error {
	fmt.Fprintf(w.out, "\n%s\n", w.t("model.title"))
	var locals []localCandidate
	for _, base := range w.bases {
		locals = append(locals, w.probe(ctx, base)...)
	}
	if len(locals) > 0 {
		fmt.Fprintln(w.out, w.t("model.localFound", len(locals)))
	}
	options := []string{w.t("model.openrouter"), w.t("model.local"), w.t("model.skip")}
	preferred := 1
	if len(locals) > 0 && w.openRouterKey() == "" {
		preferred = 2
	}
	choice, err := w.choose(w.t("model.question"), options, preferred)
	if err != nil {
		return err
	}
	switch choice {
	case 1:
		return w.chooseOpenRouter(ctx)
	case 2:
		return w.chooseLocal(ctx, locals)
	}
	fmt.Fprintln(w.out, w.t("model.skipped"))
	return nil
}

func (w *setupWizard) openRouterKey() string {
	if key := strings.TrimSpace(w.getenv("OPENROUTER_API_KEY")); key != "" {
		return key
	}
	credentials, err := storage.CredentialsStore{Path: w.paths.credentials}.Load()
	if err != nil {
		return ""
	}
	return credentials.OpenRouterAPIKey
}

func (w *setupWizard) chooseOpenRouter(ctx context.Context) error {
	key, typed := w.openRouterKey(), false
	if key != "" {
		fmt.Fprintln(w.out, w.t("openrouter.keyFound"))
	}
	var models []domain.AvailableModel
	for attempt := 0; ; attempt++ {
		if key == "" {
			fmt.Fprint(w.out, w.t("openrouter.keyPrompt"))
			entered, err := w.secret()
			if err != nil {
				return errSetupInterrupted
			}
			if key, typed = strings.TrimSpace(entered), true; key == "" {
				fmt.Fprintln(w.out, w.t("model.skipped"))
				return nil
			}
		}
		fmt.Fprintln(w.out, w.t("openrouter.loading"))
		var err error
		models, err = w.catalog(ctx, key)
		if err == nil {
			break
		}
		if strings.Contains(err.Error(), "HTTP 401") || strings.Contains(err.Error(), "HTTP 403") {
			fmt.Fprintln(w.out, w.t("openrouter.keyRefused"))
			if key, typed = "", false; attempt < 2 {
				continue
			}
			fmt.Fprintln(w.out, w.t("model.skipped"))
			return nil
		}
		// The key may be good and the network down: keep it and offer the
		// light default, which /model can change later.
		fmt.Fprintln(w.out, w.t("openrouter.catalogFailed", err))
		break
	}
	if typed {
		if err := (storage.CredentialsStore{Path: w.paths.credentials}).SaveOpenRouterKey(ctx, key); err != nil {
			return err
		}
		fmt.Fprintln(w.out, w.t("openrouter.keySaved", w.paths.credentials))
	}
	recommended := recommendLightModels(models)
	var model string
	if len(recommended) == 0 {
		answer, err := w.ask(w.t("openrouter.modelID"), lightModels[0])
		if err != nil {
			return err
		}
		model = answer
	} else {
		fmt.Fprintln(w.out, w.t("openrouter.recommended"))
		options := make([]string, 0, len(recommended)+1)
		for _, m := range recommended {
			options = append(options, describeModel(m))
		}
		options = append(options, w.t("openrouter.other"))
		choice, err := w.choose(w.t("openrouter.question"), options, 1)
		if err != nil {
			return err
		}
		if choice <= len(recommended) {
			model = string(recommended[choice-1].ID)
		} else if model, err = w.ask(w.t("openrouter.modelID"), string(recommended[0].ID)); err != nil {
			return err
		}
	}
	if _, err := root.NewModelID(model); err != nil {
		return err
	}
	if err := w.settings.Change(ctx, func(s *storage.UserSettings) { s.Model = model }); err != nil {
		return err
	}
	fmt.Fprintln(w.out, w.t("model.saved", model))
	return nil
}

func (w *setupWizard) chooseLocal(ctx context.Context, locals []localCandidate) error {
	var picked localCandidate
	if len(locals) > 0 {
		options := make([]string, 0, len(locals)+1)
		for _, c := range locals {
			options = append(options, c.describe())
		}
		options = append(options, w.t("local.other"))
		choice, err := w.choose(w.t("local.question"), options, 1)
		if err != nil {
			return err
		}
		if choice <= len(locals) {
			picked = locals[choice-1]
		}
	}
	if picked.URL == "" {
		address, err := w.ask(w.t("local.url"), "http://127.0.0.1:8080/v1")
		if err != nil {
			return err
		}
		address = strings.TrimRight(address, "/")
		if found := w.probe(ctx, address); len(found) > 0 {
			picked = found[0]
		} else {
			fmt.Fprintln(w.out, w.t("local.unreachable", address))
			served, err := w.ask(w.t("local.served"), "")
			if err != nil {
				return err
			}
			picked = localCandidate{URL: address, Served: served}
		}
	}
	window := picked.Context
	if window == 0 {
		window = 32768
	}
	answer, err := w.ask(w.t("local.context"), strconv.Itoa(window))
	if err != nil {
		return err
	}
	if window, err = strconv.Atoi(answer); err != nil || window < domain.MinimumContextWindow {
		return fmt.Errorf("the context window must be a number of at least %d tokens", domain.MinimumContextWindow)
	}
	local := picked.localModel(window)
	if err := w.settings.Change(ctx, func(s *storage.UserSettings) {
		replaced := false
		for i := range s.LocalModels {
			if s.LocalModels[i].ID == local.ID {
				s.LocalModels[i], replaced = local, true
			}
		}
		if !replaced {
			s.LocalModels = append(s.LocalModels, local)
		}
		s.Model = local.ID
	}); err != nil {
		return err
	}
	fmt.Fprintln(w.out, w.t("model.saved", local.ID))
	return nil
}

// installEngines downloads, verifies, prepares and connects KMP and MADE,
// the console's default memory and ceremony engines.
func (w *setupWizard) installEngines(ctx context.Context) error {
	fmt.Fprintf(w.out, "\n%s\n%s\n", w.t("engines.title"), w.t("engines.intro"))
	targets, err := (&storage.MCPConfigStore{Path: w.paths.mcp}).EngineTargets(ctx)
	if err != nil {
		fmt.Fprintln(w.out, w.t("engines.configUnreadable", w.paths.mcp, err))
		return nil
	}
	connected := map[string]bool{}
	for _, target := range targets {
		connected[target.Engine] = true
	}
	if connected["kmp"] && connected["made"] {
		fmt.Fprintln(w.out, w.t("engines.connected"))
		return nil
	}
	install, err := w.yes(w.t("engines.question"), true)
	if err != nil {
		return err
	}
	if !install {
		fmt.Fprintln(w.out, w.t("engines.later"))
		return nil
	}
	approval := "manual"
	if !connected["kmp"] {
		auto, err := w.yes(w.t("engines.kmpAuto"), true)
		if err != nil {
			return err
		}
		if auto {
			approval = "auto"
		}
	}
	fmt.Fprintln(w.out, w.t("engines.working"))
	results, err := w.engines(approval).Update(ctx)
	if errors.Is(err, application.ErrEngineUpdatePlatform) {
		fmt.Fprintln(w.out, w.t("engines.platform"))
		return nil
	}
	for _, result := range results {
		name := strings.ToUpper(result.Engine)
		switch result.Status {
		case "installed":
			fmt.Fprintln(w.out, w.t("engines.installed", name, result.Version, result.Detail))
		case "updated", "current":
			fmt.Fprintln(w.out, w.t("engines.ready", name, result.Version))
		case "skipped":
			fmt.Fprintln(w.out, w.t("engines.skipped", name))
		default:
			fmt.Fprintln(w.out, w.t("engines.failed", name, result.Error))
		}
	}
	if err != nil {
		fmt.Fprintln(w.out, w.t("engines.failed", "", err))
	}
	return nil
}

// choose prints numbered options and returns the 1-based answer.
func (w *setupWizard) choose(question string, options []string, preferred int) (int, error) {
	for {
		fmt.Fprintln(w.out, question)
		for i, option := range options {
			marker := " "
			if i+1 == preferred {
				marker = "*"
			}
			fmt.Fprintf(w.out, "  %s %d) %s\n", marker, i+1, option)
		}
		answer, err := w.ask(w.t("choice"), strconv.Itoa(preferred))
		if err != nil {
			return 0, err
		}
		if n, err := strconv.Atoi(answer); err == nil && n >= 1 && n <= len(options) {
			return n, nil
		}
		fmt.Fprintln(w.out, w.t("invalid"))
	}
}

func (w *setupWizard) yes(question string, preferred bool) (bool, error) {
	hints := map[bool]string{true: "[Y/n]", false: "[y/N]"}
	if w.lang == "es" {
		hints = map[bool]string{true: "[S/n]", false: "[s/N]"}
	}
	hint := hints[preferred]
	for {
		fmt.Fprintf(w.out, "%s %s ", question, hint)
		answer, err := w.line()
		if err != nil {
			return false, err
		}
		switch strings.ToLower(answer) {
		case "":
			return preferred, nil
		case "y", "yes", "s", "si", "sí":
			return true, nil
		case "n", "no":
			return false, nil
		}
		fmt.Fprintln(w.out, w.t("invalid"))
	}
}

func (w *setupWizard) ask(question, preferred string) (string, error) {
	if preferred != "" {
		fmt.Fprintf(w.out, "%s [%s]: ", question, preferred)
	} else {
		fmt.Fprintf(w.out, "%s: ", question)
	}
	answer, err := w.line()
	if err != nil {
		return "", err
	}
	if answer == "" {
		return preferred, nil
	}
	return answer, nil
}

func (w *setupWizard) line() (string, error) {
	text, err := w.in.ReadString('\n')
	if err != nil && (text == "" || !errors.Is(err, io.EOF)) {
		return "", errSetupInterrupted
	}
	return strings.TrimSpace(text), nil
}

// readHiddenLine reads the key without echo. Ctrl+C there would end the
// process with echo still off, so an interrupt restores the terminal first.
func readHiddenLine(file *os.File, out io.Writer) (string, error) {
	state, err := term.GetState(file.Fd())
	if err != nil {
		return "", err
	}
	interrupted := make(chan os.Signal, 1)
	signal.Notify(interrupted, os.Interrupt)
	done := make(chan struct{})
	defer func() {
		signal.Stop(interrupted)
		close(done)
	}()
	go func() {
		select {
		case <-interrupted:
			_ = term.Restore(file.Fd(), state)
			fmt.Fprintln(out)
			os.Exit(130)
		case <-done:
		}
	}()
	data, err := term.ReadPassword(file.Fd())
	fmt.Fprintln(out)
	return strings.TrimSpace(string(data)), err
}

// setupLanguage starts in the language AXLR_LANG or the locale asks for.
func setupLanguage(getenv func(string) string) string {
	for _, variable := range []string{"AXLR_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := strings.ToLower(getenv(variable)); value != "" {
			if strings.HasPrefix(value, "es") {
				return "es"
			}
			return "en"
		}
	}
	return "en"
}

// recommendLightModels lists up to five light models from the catalog: the
// pinned ones first, then the cheapest flash/mini/lite/haiku models with
// tool calls and a window of at least 128k tokens. Free and batch variants
// are left out: they are rate-limited or answer late.
func recommendLightModels(models []domain.AvailableModel) []domain.AvailableModel {
	byID := map[root.ModelID]domain.AvailableModel{}
	for _, m := range models {
		if m.SupportsTools && m.TextOutput {
			byID[m.ID] = m
		}
	}
	var result []domain.AvailableModel
	for _, id := range lightModels {
		if m, ok := byID[root.ModelID(id)]; ok {
			result = append(result, m)
			delete(byID, m.ID)
		}
	}
	var others []domain.AvailableModel
	for _, m := range byID {
		id := strings.ToLower(string(m.ID))
		name := id[strings.LastIndex(id, "/")+1:]
		light := false
		for _, word := range []string{"flash", "mini", "lite", "haiku", "nano"} {
			light = light || strings.Contains(name, word)
		}
		for _, word := range []string{"preview", "image", "audio", "vision", "omni", "exp"} {
			light = light && !strings.Contains(name, word)
		}
		if light && !strings.Contains(id, ":") && m.Context >= 128000 && price(m) > 0 {
			others = append(others, m)
		}
	}
	sort.Slice(others, func(i, j int) bool {
		if price(others[i]) != price(others[j]) {
			return price(others[i]) < price(others[j])
		}
		return others[i].ID < others[j].ID
	})
	for _, m := range others {
		if len(result) == 5 {
			break
		}
		result = append(result, m)
	}
	return result
}

// price is the USD per million prompt plus completion tokens; 0 is unknown.
func price(m domain.AvailableModel) float64 {
	prompt, err1 := strconv.ParseFloat(perMillion(m.PromptRate), 64)
	completion, err2 := strconv.ParseFloat(perMillion(m.CompletionRate), 64)
	if err1 != nil || err2 != nil {
		return 0
	}
	return prompt + completion
}

func describeModel(m domain.AvailableModel) string {
	text := string(m.ID)
	if m.Context > 0 {
		text += fmt.Sprintf(" · %dk", int(m.Context)/1000)
	}
	if p, c := perMillion(m.PromptRate), perMillion(m.CompletionRate); p != "" && c != "" {
		text += fmt.Sprintf(" · $%s / $%s per 1M tokens", p, c)
	}
	return text
}

// perMillion is a rate's USD per million tokens without its unit.
func perMillion(rate domain.ModelRate) string {
	return strings.TrimPrefix(strings.TrimSuffix(rate.Display(), " / 1M tokens"), "$")
}

// localCandidate is one model a local OpenAI-compatible server serves.
type localCandidate struct {
	URL     string
	Served  string
	Context int
}

func (c localCandidate) describe() string {
	text := c.Served + " · " + c.URL
	if c.Context > 0 {
		text += fmt.Sprintf(" · %dk", c.Context/1000)
	}
	return text
}

// localModel is the local_models entry: local/<name>, the served name when
// the server needs it, and the window the person confirmed.
func (c localCandidate) localModel(window int) storage.LocalModel {
	name := strings.ToLower(path.Base(strings.ReplaceAll(c.Served, "\\", "/")))
	if c.Served == "" {
		// A server that names no model is known by its address.
		if parsed, err := url.Parse(c.URL); err == nil {
			name = parsed.Hostname() + "-" + parsed.Port()
		}
	}
	name = strings.TrimSuffix(name, ".gguf")
	var slug strings.Builder
	dash := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' {
			slug.WriteRune(r)
			dash = false
		} else if !dash && slug.Len() > 0 {
			slug.WriteByte('-')
			dash = true
		}
	}
	id := strings.Trim(slug.String(), "-.")
	if len(id) > 64 {
		id = id[:64]
	}
	if id == "" || id == "." {
		id = "model"
	}
	local := storage.LocalModel{ID: "local/" + id, Name: c.Served, URL: c.URL, ContextTokens: window}
	if _, err := root.NewModelID(c.Served); err == nil && c.Served != "" {
		local.Model = c.Served
	}
	if local.Name == "" {
		local.Name = local.ID
	}
	return local
}

// probeLocalServer lists the models of an OpenAI-compatible server at base
// (…/v1). llama.cpp reports its configured window on /props, vLLM as
// max_model_len, and LM Studio or Ollama report none.
func probeLocalServer(ctx context.Context, client *http.Client, base string) []localCandidate {
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil
	}
	var listing struct {
		Data []struct {
			ID            string `json:"id"`
			MaxModelLen   int    `json:"max_model_len"`
			ContextLength int    `json:"context_length"`
			Meta          struct {
				TrainContext int `json:"n_ctx_train"`
			} `json:"meta"`
		} `json:"data"`
	}
	if !getJSON(ctx, client, base+"/models", &listing) {
		return nil
	}
	var props struct {
		Settings struct {
			Context int `json:"n_ctx"`
		} `json:"default_generation_settings"`
	}
	getJSON(ctx, client, strings.TrimSuffix(base, "/v1")+"/props", &props)
	var found []localCandidate
	for _, model := range listing.Data {
		if model.ID == "" || len(found) == 8 {
			continue
		}
		window := props.Settings.Context
		for _, other := range []int{model.MaxModelLen, model.ContextLength, model.Meta.TrainContext} {
			if window == 0 {
				window = other
			}
		}
		found = append(found, localCandidate{URL: base, Served: model.ID, Context: window})
	}
	return found
}

func getJSON(ctx context.Context, client *http.Client, address string, into any) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return false
	}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(into) == nil
}
