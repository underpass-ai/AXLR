package compose

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/localmodels"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/adapters/typesafe"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestLocalModelsReachTheModelPickerWithoutOpenRouter(t *testing.T) {
	no := false
	setup := NewLocalModels(storage.UserSettings{ContextTokens: 32768, LocalModels: []storage.LocalModel{
		{ID: "local/qwen", Name: "Qwen3.8-27B", URL: "http://127.0.0.1:8080/v1", ContextTokens: 262144},
		{ID: "local/plain", URL: "http://127.0.0.1:8083/v1", ContextTokens: 8192, Tools: &no},
	}}, func(string) string { return "" })
	models, err := application.ListModelsUseCase{Catalog: localmodels.Catalog{Local: setup.Available}}.Execute(context.Background())
	if err != nil || len(models) != 1 || models[0].ID != "local/qwen" || models[0].Name != "Qwen3.8-27B" || models[0].Context != 32768 {
		t.Fatalf("picker models = %+v, %v", models, err)
	}
}

// JevJudge hands the policy to the final check and, through Decisions,
// to the settlement; the default is legacy and an incoherent switch is
// refused.
func TestJevJudgeCarriesThePolicy(t *testing.T) {
	getenv := func(name string) string {
		if name == "TYPESAFE_API_KEY" {
			return "typesafe-never-print"
		}
		return ""
	}
	judge, err := JevJudge(&storage.JevSettings{FinalCheck: true}, getenv)
	if err != nil || judge.Decisions.Policy != application.JudgePolicyLegacy || judge.Decisions.Supersede || judge.Decisions.Model != typesafe.DefaultModel {
		t.Fatalf("default: %+v %v", judge, err)
	}
	judge, err = JevJudge(&storage.JevSettings{FinalCheck: true, Model: "jev-1.13.0", JudgePolicy: "axlr-v2", V2Supersede: true}, getenv)
	if err != nil || judge.Decisions != (application.JudgeDecisions{Policy: application.JudgePolicyAXLRv2, Supersede: true, Model: "jev-1.13.0"}) {
		t.Fatalf("axlr-v2: %+v %v", judge, err)
	}
	var ceremony application.SessionCeremony
	SettleWith(&ceremony, judge)
	if ceremony.Judge != judge.Port || ceremony.Decisions != judge.Decisions {
		t.Fatalf("the settlement did not get the policy: %+v", ceremony.Decisions)
	}
	if judge, err := JevJudge(&storage.JevSettings{JudgePolicy: "axlr-v2"}, getenv); judge != nil || err != nil {
		t.Fatalf("a policy turned Jev on: %+v %v", judge, err)
	}
	for _, settings := range []storage.JevSettings{{FinalCheck: true, JudgePolicy: "axlr-v5"}, {FinalCheck: true, V2Supersede: true}} {
		if _, err := JevJudge(&settings, getenv); err == nil {
			t.Fatalf("%+v accepted", settings)
		}
	}
}

func TestCompactProfileFollowsTheSettingAndTheWindow(t *testing.T) {
	windows := localmodels.Windows{Local: map[root.ModelID]domain.ContextWindow{"local/small": 32768, "local/large": 262144}}
	auto := CompactProfile("auto", windows)
	if !auto("local/small") || auto("local/large") || auto("openai/gpt-4o") {
		t.Fatal("auto profile does not follow the window")
	}
	if CompactProfile("standard", windows) != nil || !CompactProfile("compact", windows)("openai/gpt-4o") {
		t.Fatal("explicit profiles ignored")
	}
	capped := CompactProfile("auto", localmodels.Windows{Local: windows.Local, Cap: 65536})
	if !capped("local/large") {
		t.Fatal("the global cap does not make a large model compact")
	}
}

// A successor of a successor is "(3)", not "(2) (2)" (review of 14 October
// 2026).
func TestSuccessorTitlesCount(t *testing.T) {
	for title, want := range map[string]string{"Parser": "Parser (2)", "Parser (2)": "Parser (3)", "Parser (9)": "Parser (10)", "Año (x)": "Año (x) (2)"} {
		if got := successorTitle(title); got != want {
			t.Errorf("successorTitle(%q) = %q, want %q", title, got, want)
		}
	}
}

type memoryStore map[domain.SessionID]domain.Session

func (m memoryStore) Save(_ context.Context, s domain.Session) error        { m[s.ID()] = s; return nil }
func (m memoryStore) List(context.Context) ([]domain.SessionSummary, error) { return nil, nil }
func (m memoryStore) Load(_ context.Context, id domain.SessionID) (domain.Session, error) {
	return m[id], nil
}

// A service session past the cap continues in a session its principal
// owns: until 21 October 2026 the successor had no owner, and the client
// that started the turn was refused it.
func TestSuccessorKeepsModeAndOwner(t *testing.T) {
	store := memoryStore{}
	predecessor, err := domain.NewSession("0123456789abcdef0123456789abcdef", domain.Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	predecessor.SetServiceMetadata("alice", 3, "op")
	if err := predecessor.SetMode(domain.ModeReview); err != nil {
		t.Fatal(err)
	}
	successor, err := Successors{Create: application.CreateSessionUseCase{Store: store}, Store: store}.Continue(context.Background(), predecessor)
	if err != nil {
		t.Fatal(err)
	}
	saved := store[successor.ID()].Export()
	if successor.ID() == predecessor.ID() || saved.Owner != "alice" || saved.Model != "test/model" || store[successor.ID()].Mode() != domain.ModeReview || saved.Instance == nil || saved.Instance.Predecessor != predecessor.ID() {
		t.Fatalf("successor %+v", saved)
	}
}

// Equip gives the continuation the same tools in both binaries: forged and
// catalog tools only when settings enable them, memory only with KMP.
func TestEquipFollowsSettingsAndMemory(t *testing.T) {
	var bare application.ContinueTurnUseCase
	off := false
	tools, err := Equip(&bare, Equipment{Settings: storage.UserSettings{ForgedTools: &off, MaxSessionUSD: 2, TurnToolCalls: 7}, State: t.TempDir()})
	if err != nil || bare.Forge != nil || bare.Catalog != nil || bare.Publisher != nil || bare.Remember != nil || tools.Catalog != nil {
		t.Fatalf("forged tools off equipped %+v %v", bare, err)
	}
	if bare.MaxSessionUSD != 2 || bare.TurnToolCalls != 7 || bare.Tokens == nil || bare.Successor == nil {
		t.Fatalf("usage or cap missing: %+v", bare)
	}
	var full application.ContinueTurnUseCase
	tools, err = Equip(&full, Equipment{Settings: storage.DefaultUserSettings(), State: t.TempDir(), Memory: true})
	if err != nil || full.Forge == nil || full.Catalog == nil || full.Publisher == nil || full.Remember == nil || full.Recall == nil || tools.Catalog == nil {
		t.Fatalf("defaults not equipped %+v %v", full, err)
	}
}

// Connect builds the client each entry's api names: chat completions by
// default, the native Anthropic and OpenAI Responses clients otherwise; a
// remote entry without its key stops the launch naming the variable.
func TestLocalModelsConnectByAPI(t *testing.T) {
	setup := NewLocalModels(storage.UserSettings{LocalModels: []storage.LocalModel{
		{ID: "local/qwen", URL: "http://127.0.0.1:8080/v1", ContextTokens: 65536},
		{ID: "anthropic/sonnet", URL: "https://api.anthropic.com", Model: "claude-sonnet-5-5", API: "anthropic", APIKeyEnv: "ANTHROPIC_API_KEY", ContextTokens: 200000},
		{ID: "openai/gpt", URL: "https://api.openai.com/v1", Model: "gpt-5", API: "responses", APIKeyEnv: "OPENAI_API_KEY", ContextTokens: 400000},
	}}, func(name string) string {
		return map[string]string{"ANTHROPIC_API_KEY": "sk-ant-x", "OPENAI_API_KEY": "sk-proj-x"}[name]
	})
	routes, err := setup.Connect(&http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[root.ModelID]string{"local/qwen": "*openrouter.Client", "anthropic/sonnet": "*anthropic.Client", "openai/gpt": "*openai.Client"} {
		if got := fmt.Sprintf("%T", routes[id].Client); got != want {
			t.Fatalf("%s: %s, want %s", id, got, want)
		}
	}
	if routes["anthropic/sonnet"].Upstream != "claude-sonnet-5-5" || routes["openai/gpt"].Upstream != "gpt-5" || !setup.KeyEnvs["ANTHROPIC_API_KEY"] || len(setup.Secrets) != 2 {
		t.Fatalf("routes %+v keys %+v", routes, setup.KeyEnvs)
	}
	missing := NewLocalModels(storage.UserSettings{LocalModels: []storage.LocalModel{
		{ID: "anthropic/sonnet", URL: "https://api.anthropic.com", API: "anthropic", APIKeyEnv: "ANTHROPIC_API_KEY", ContextTokens: 200000},
	}}, func(string) string { return "" })
	if _, err := missing.Connect(&http.Client{}); err == nil || !strings.Contains(err.Error(), "set ANTHROPIC_API_KEY") {
		t.Fatalf("missing key: %v", err)
	}
}
