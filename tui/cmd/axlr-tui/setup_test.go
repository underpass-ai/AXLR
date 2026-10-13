package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/adapters/terminal"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type fakeEngines struct {
	approvals []string
	results   []application.EngineUpdateResult
	err       error
}

func (f *fakeEngines) Update(context.Context) ([]application.EngineUpdateResult, error) {
	return f.results, f.err
}

func available(t *testing.T, id string, context int, prompt, completion string) domain.AvailableModel {
	t.Helper()
	p, err := domain.NewModelRate(prompt)
	if err != nil {
		t.Fatal(err)
	}
	c, err := domain.NewModelRate(completion)
	if err != nil {
		t.Fatal(err)
	}
	return domain.AvailableModel{ID: root.ModelID(id), Name: root.Text(id), Context: domain.ContextWindow(context), PromptRate: p, CompletionRate: c, SupportsTools: true, TextOutput: true}
}

// wizard builds an assistant over temporary paths that answers from input.
func wizard(t *testing.T, env map[string]string, input string, engines *fakeEngines) (*setupWizard, *bytes.Buffer) {
	t.Helper()
	getenv := func(k string) string { return env[k] }
	paths, err := resolveSetupPaths(getenv, "")
	if err != nil {
		t.Fatal(err)
	}
	settings, err := storage.NewUserSettingsStore(paths.settings, storage.DefaultUserSettings())
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	w := newSetupWizard(getenv, paths, settings, strings.NewReader(input), &out)
	w.probe = func(context.Context, string) []localCandidate { return nil }
	w.catalog = func(context.Context, string) ([]domain.AvailableModel, error) {
		return nil, errors.New("catalog not expected")
	}
	w.engines = func(approval string) application.EngineUpdatePort {
		engines.approvals = append(engines.approvals, approval)
		return engines
	}
	return w, &out
}

func savedSettings(t *testing.T, w *setupWizard) storage.UserSettings {
	t.Helper()
	settings, err := w.settings.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(w.paths.settings); err != nil {
		t.Fatal("settings.json was not written:", err)
	}
	return settings
}

func TestSetupSavesTheKeyARecommendedLightModelAndInstallsTheEngines(t *testing.T) {
	env := cliEnv(t)
	delete(env, "OPENROUTER_API_KEY")
	engines := &fakeEngines{results: []application.EngineUpdateResult{
		{Engine: "made", Status: "installed", Version: "0.10.0", Detail: "work identity axlr-work-1"},
		{Engine: "kmp", Status: "installed", Version: "0.25.0", Detail: "memory store /m"},
	}}
	// Spanish, OpenRouter, the first recommendation, install, auto memory.
	w, out := wizard(t, env, "2\n1\n\n\n\n", engines)
	w.secret = func() (string, error) { return "sk-or-typed", nil }
	var usedKey string
	w.catalog = func(_ context.Context, key string) ([]domain.AvailableModel, error) {
		usedKey = key
		return []domain.AvailableModel{
			available(t, "big/model-pro", 1000000, "0.000003", "0.000015"),
			available(t, "z-ai/glm-5.3-flash", 1048576, "0.00000015", "0.0000005"),
		}, nil
	}
	if err := w.run(context.Background()); err != nil {
		t.Fatal(err, out)
	}
	settings := savedSettings(t, w)
	if settings.Language != "es" || settings.Model != "z-ai/glm-5.3-flash" || usedKey != "sk-or-typed" {
		t.Fatalf("settings %+v key %q\n%s", settings, usedKey, out)
	}
	credentials, err := storage.CredentialsStore{Path: w.paths.credentials}.Load()
	if err != nil || credentials.OpenRouterAPIKey != "sk-or-typed" {
		t.Fatalf("%+v %v", credentials, err)
	}
	if len(engines.approvals) != 1 || engines.approvals[0] != "auto" {
		t.Fatalf("approvals %v", engines.approvals)
	}
	for _, want := range []string{"KMP 0.25.0 · instalado y conectado · memory store /m", "MADE 0.10.0", "Modelo por defecto: z-ai/glm-5.3-flash"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out.String(), "sk-or-typed") {
		t.Fatal("the key was printed")
	}
}

func TestSetupUsesTheEnvironmentKeyWithoutSavingIt(t *testing.T) {
	env := cliEnv(t)
	engines := &fakeEngines{}
	// English, OpenRouter, another id, typed id, no engines.
	w, out := wizard(t, env, "1\n1\n3\nvendor/chosen\nn\n", engines)
	w.secret = func() (string, error) { t.Fatal("asked for a key the environment has"); return "", nil }
	w.catalog = func(context.Context, string) ([]domain.AvailableModel, error) {
		return []domain.AvailableModel{available(t, "z-ai/glm-5.3-flash", 1048576, "0.00000015", "0.0000005"), available(t, "anthropic/claude-haiku-5.5", 1000000, "0.0000001", "0.0000005")}, nil
	}
	if err := w.run(context.Background()); err != nil {
		t.Fatal(err, out)
	}
	if settings := savedSettings(t, w); settings.Model != "vendor/chosen" || settings.Language != "en" {
		t.Fatalf("%+v", settings)
	}
	if _, err := os.Stat(w.paths.credentials); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the environment key was copied to credentials.json")
	}
	if len(engines.approvals) != 0 || !strings.Contains(out.String(), "/update") {
		t.Fatalf("engines installed after no:\n%s", out)
	}
}

func TestSetupRetriesARefusedKeyAndNeverSavesIt(t *testing.T) {
	env := cliEnv(t)
	delete(env, "OPENROUTER_API_KEY")
	w, out := wizard(t, env, "1\n1\nn\n", &fakeEngines{})
	keys := []string{"bad-1", "bad-2", "bad-3"}
	w.secret = func() (string, error) { key := keys[0]; keys = keys[1:]; return key, nil }
	w.catalog = func(context.Context, string) ([]domain.AvailableModel, error) {
		return nil, errors.New("OpenRouter model catalog request failed (HTTP 401)")
	}
	if err := w.run(context.Background()); err != nil {
		t.Fatal(err, out)
	}
	if len(keys) != 0 || strings.Count(out.String(), "refused that key") != 3 {
		t.Fatalf("retries: %v\n%s", keys, out)
	}
	if _, err := os.Stat(w.paths.credentials); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a refused key was saved")
	}
	if savedSettings(t, w).Model != "" {
		t.Fatal("a model was saved without a provider")
	}
}

func TestSetupConfiguresADetectedLocalServer(t *testing.T) {
	env := cliEnv(t)
	delete(env, "OPENROUTER_API_KEY")
	engines := &fakeEngines{}
	// English; the local server is preferred without a key; first model;
	// keep the detected window; skip the engines.
	w, out := wizard(t, env, "1\n\n\n\nn\n", engines)
	w.probe = func(_ context.Context, base string) []localCandidate {
		if base != "http://127.0.0.1:8080/v1" {
			return nil
		}
		return []localCandidate{{URL: base, Served: "/models/Qwen3.8-27B-Q4_K_M.gguf", Context: 131072}}
	}
	if err := w.run(context.Background()); err != nil {
		t.Fatal(err, out)
	}
	settings := savedSettings(t, w)
	if settings.Model != "local/qwen3.8-27b-q4_k_m" || len(settings.LocalModels) != 1 {
		t.Fatalf("%+v\n%s", settings, out)
	}
	local := settings.LocalModels[0]
	if local.URL != "http://127.0.0.1:8080/v1" || local.ContextTokens != 131072 || local.ID != settings.Model {
		t.Fatalf("%+v", local)
	}
	// Running it again replaces the entry instead of duplicating it.
	w2, out2 := wizard(t, env, "1\n2\n\n8192\nn\n", engines)
	w2.probe = w.probe
	if err := w2.run(context.Background()); err != nil {
		t.Fatal(err, out2)
	}
	settings = savedSettings(t, w2)
	if len(settings.LocalModels) != 1 || settings.LocalModels[0].ContextTokens != 8192 {
		t.Fatalf("%+v", settings.LocalModels)
	}
}

func TestSetupSkipsEnginesAlreadyConnected(t *testing.T) {
	env := cliEnv(t)
	engines := &fakeEngines{}
	w, out := wizard(t, env, "1\n3\n", engines)
	store := storage.MCPConfigStore{Path: w.paths.mcp}
	for _, engine := range []string{"kmp", "made"} {
		launcher := filepath.Join(t.TempDir(), "scripts", "run-embedded-mcp.sh")
		if err := store.RegisterEngine(context.Background(), application.EngineRegistration{Engine: engine, Launcher: launcher, Purpose: "tools", Approval: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.run(context.Background()); err != nil {
		t.Fatal(err, out)
	}
	if len(engines.approvals) != 0 || !strings.Contains(out.String(), "already connected") {
		t.Fatalf("%v\n%s", engines.approvals, out)
	}
}

func TestSetupReportsAnUnsupportedPlatform(t *testing.T) {
	env := cliEnv(t)
	w, out := wizard(t, env, "1\n3\n\n\n", &fakeEngines{err: application.ErrEngineUpdatePlatform})
	if err := w.run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "docs/runbooks/kmp.md") {
		t.Fatal(out)
	}
}

func TestSetupStopsWhenInputEnds(t *testing.T) {
	w, _ := wizard(t, cliEnv(t), "", &fakeEngines{})
	if err := w.run(context.Background()); !errors.Is(err, errSetupInterrupted) {
		t.Fatal(err)
	}
}

func TestRecommendLightModelsPinsThenCheapestLightOnes(t *testing.T) {
	models := []domain.AvailableModel{
		available(t, "vendor/big-pro", 1000000, "0.000001", "0.000002"),
		available(t, "vendor/tiny-flash", 32000, "0.00000001", "0.00000001"),
		available(t, "vendor/cheap-flash", 1000000, "0.00000003", "0.00000013"),
		available(t, "vendor/cheap-flash:free", 1000000, "0", "0"),
		available(t, "vendor/next-flash-preview", 1000000, "0.00000001", "0.00000001"),
		available(t, "vendor/mid-mini", 400000, "0.000000125", "0.000001"),
		available(t, "anthropic/claude-haiku-5.5", 1000000, "0.0000001", "0.0000005"),
	}
	noTools := available(t, "vendor/notools-lite", 1000000, "0.00000001", "0.00000001")
	noTools.SupportsTools = false
	got := recommendLightModels(append(models, noTools))
	var ids []string
	for _, m := range got {
		ids = append(ids, string(m.ID))
	}
	if strings.Join(ids, ",") != "anthropic/claude-haiku-5.5,vendor/cheap-flash,vendor/mid-mini" {
		t.Fatal(ids)
	}
	if text := describeModel(got[1]); text != "vendor/cheap-flash · 1000k · $0.03 / $0.13 per 1M tokens" {
		t.Fatal(text)
	}
}

func TestProbeLocalServerReadsLlamaCppAndVLLMWindows(t *testing.T) {
	llama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Write([]byte(`{"object":"list","data":[{"id":"qwen.gguf","meta":{"n_ctx_train":262144}}]}`))
		case "/props":
			w.Write([]byte(`{"default_generation_settings":{"n_ctx":131072}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer llama.Close()
	vllm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`{"data":[{"id":"google/gemma-4-31b","max_model_len":65536}]}`))
	}))
	defer vllm.Close()
	got := probeLocalServer(context.Background(), llama.Client(), llama.URL+"/v1")
	if len(got) != 1 || got[0].Served != "qwen.gguf" || got[0].Context != 131072 {
		t.Fatalf("%+v", got)
	}
	got = probeLocalServer(context.Background(), vllm.Client(), vllm.URL+"/v1")
	if len(got) != 1 || got[0].Context != 65536 || got[0].localModel(65536).Model != "google/gemma-4-31b" || got[0].localModel(65536).ID != "local/gemma-4-31b" {
		t.Fatalf("%+v %+v", got, got[0].localModel(65536))
	}
	if id := (localCandidate{URL: "http://127.0.0.1:8081/v1"}).localModel(4096).ID; id != "local/127.0.0.1-8081" {
		t.Fatal(id)
	}
	if probeLocalServer(context.Background(), llama.Client(), "http://127.0.0.1:1/v1") != nil {
		t.Fatal("an unreachable server listed models")
	}
}

func TestSetupLanguageFollowsTheLocale(t *testing.T) {
	for env, want := range map[string]string{"es_ES.UTF-8": "es", "en_US.UTF-8": "en", "": "en"} {
		if got := setupLanguage(func(k string) string {
			if k == "LANG" {
				return env
			}
			return ""
		}); got != want {
			t.Fatalf("%q: %s", env, got)
		}
	}
}

func TestRunUsesTheKeyTheSetupSaved(t *testing.T) {
	env := cliEnv(t)
	delete(env, "OPENROUTER_API_KEY")
	path := filepath.Join(env["HOME"], ".config", "axlr", "credentials.json")
	if err := (storage.CredentialsStore{Path: path}).SaveOpenRouterKey(context.Background(), "saved-key-never-print"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	launched := false
	code := run(context.Background(), []string{"--root", t.TempDir(), "--model", "vendor/model"}, func(k string) string { return env[k] }, func(model tea.Model) error {
		launched = model.(terminal.AppModel).Header.State.Model == "vendor/model"
		return nil
	}, &out)
	if code != 0 || !launched || strings.Contains(out.String(), "saved-key-never-print") {
		t.Fatalf("code=%d launched=%v\n%s", code, launched, &out)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := run(context.Background(), []string{"--root", t.TempDir(), "--model", "vendor/model"}, func(k string) string { return env[k] }, func(tea.Model) error { return nil }, &out); code == 0 || !strings.Contains(out.String(), "chmod 600") {
		t.Fatalf("a readable credentials file was used: %d\n%s", code, &out)
	}
}
