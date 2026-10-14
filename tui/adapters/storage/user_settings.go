package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const maxFavoriteModels = 50

// UserSettings is the editable, user-facing configuration in settings.json.
// An empty model means that the console opens without a selected model.
type UserSettings struct {
	Model        string               `json:"model"`
	Language     string               `json:"language"`
	Theme        string               `json:"theme"`
	Icons        string               `json:"icons"`
	ReduceMotion bool                 `json:"reduce_motion"`
	Approvals    *ApprovalPreferences `json:"approvals,omitempty"`
	// FavoriteModels are listed first in /model.
	FavoriteModels []string `json:"favorite_models,omitempty"`
	// ReviewerModel judges incident postmortems in a fresh context; empty
	// means the session model.
	ReviewerModel string `json:"reviewer_model,omitempty"`
	// Repair configures the /repair ceremony: which repository the console
	// may repair, where it clones it and whether green pull requests merge
	// without the person.
	Repair *RepairSettings `json:"repair,omitempty"`
	// ContextTokens caps the context window, in tokens, the console uses for
	// every model; zero means no cap. A local model's own window applies
	// when it is smaller.
	ContextTokens int `json:"context_tokens,omitempty"`
	// PromptTokens is the prompt, in tokens, a request to a model whose
	// window is unknown (OpenRouter's) may reach; zero means
	// domain.DefaultPromptTokens. A local model's window applies instead.
	PromptTokens int `json:"prompt_tokens,omitempty"`
	// Engines shares KMP and MADE between consoles (user_settings_engines.go).
	Engines *EngineSettings `json:"engines,omitempty"`
	// Sandbox confines local commands (user_settings_sandbox.go); absent
	// means off.
	Sandbox *ExecSandboxSettings `json:"exec_sandbox,omitempty"`
	// Jobs bounds the repair and improvement jobs (user_settings_jobs.go).
	Jobs *JobsSettings `json:"jobs,omitempty"`
	// TurnToolCalls is a turn's tool-call budget; zero means
	// domain.MaxTurnToolCalls (32).
	TurnToolCalls int `json:"turn_tool_calls,omitempty"`
	// MaxSessionUSD stops a session's model requests once their known cost
	// reaches it, in dollars; zero means no limit (user_settings_usage.go).
	MaxSessionUSD float64 `json:"max_session_usd,omitempty"`
	// LocalModels are OpenAI-compatible servers, such as llama.cpp or vLLM,
	// offered in /model next to OpenRouter's catalog.
	LocalModels []LocalModel `json:"local_models,omitempty"`
	// Models holds OpenRouter request options by exact model id: provider
	// routing, reasoning and max_tokens (user_settings_models.go).
	Models map[string]json.RawMessage `json:"models,omitempty"`
	// Jev enables TypeSafe Jev, an external judgement model; absent or with
	// both switches off, nothing is sent to TypeSafe.
	Jev *JevSettings `json:"jev,omitempty"`
	// Ceremonies selects the profile of /debug and /delivery.
	Ceremonies *CeremonySettings `json:"ceremonies,omitempty"`
	// Plan configures /plan.
	Plan *PlanSettings `json:"plan,omitempty"`
	// TraceRetentionDays is how long the default diagnostics directory keeps
	// an earlier launch's trace and payloads; absent means
	// DefaultTraceRetentionDays and 0 keeps them forever. A pointer keeps an
	// explicit 0 when another setting is saved.
	TraceRetentionDays *int `json:"trace_retention_days,omitempty"`
	// TracePayloads stores redacted request and response bodies beside the
	// trace by default; --trace-payloads decides for one launch.
	TracePayloads bool `json:"trace_payloads,omitempty"`
	// ForgedTools offers axlr_forge_tool and axlr_run_tool; absent means on.
	// A pointer keeps an explicit false when another setting is saved.
	ForgedTools *bool `json:"forged_tools,omitempty"`
	// Notify is how the console calls for attention when a long turn ends
	// or a card waits: "bell" (the default), "osc" (a desktop notification
	// and the bell) or "off".
	Notify string                     `json:"notify,omitempty"`
	Extra  map[string]json.RawMessage `json:"-"`
}

// NotifyMode is the notify setting, "bell" when unset.
func (s UserSettings) NotifyMode() string {
	if s.Notify == "" {
		return "bell"
	}
	return s.Notify
}

// ForgedToolsEnabled reports whether forged tools are offered: on unless
// forged_tools is false.
func (s UserSettings) ForgedToolsEnabled() bool { return s.ForgedTools == nil || *s.ForgedTools }

// turn_tool_calls bounds: below 8 a turn cannot read, edit and check; above
// 256 one turn could spend a session's budget without the person.
const (
	minTurnToolCalls = 8
	maxTurnToolCalls = 256
)

// DefaultTraceRetentionDays applies when trace_retention_days is absent.
const DefaultTraceRetentionDays = 30

const maxTraceRetentionDays = 36500

// TraceRetention is the configured retention with the default applied;
// zero disables pruning.
func (s UserSettings) TraceRetention() time.Duration {
	days := DefaultTraceRetentionDays
	if s.TraceRetentionDays != nil {
		days = *s.TraceRetentionDays
	}
	return time.Duration(days) * 24 * time.Hour
}

// Planner is the configured planner; empty means the session's model. The
// default was z-ai/glm-5.3-flash until 9 October 2026: routed by OpenRouter
// to OpenInference, its decompose request took 138 s, and a remote session
// already runs a large model. A session on a small local model names a
// larger planner here.
func (s UserSettings) Planner() string {
	if s.Plan == nil || s.Plan.Model == "session" {
		return ""
	}
	return s.Plan.Model
}

// CeremonyProfile is the configured profile with the default applied.
func (s UserSettings) CeremonyProfile() string {
	if s.Ceremonies == nil || s.Ceremonies.Profile == "" {
		return "auto"
	}
	return s.Ceremonies.Profile
}

// MaxRepairAttempts is the default bound on repair sessions per failure.
const MaxRepairAttempts = 2

// DefaultRepairRepository is what /repair repairs when settings.json is silent.
const DefaultRepairRepository = "underpass-ai/AXLR"

// RepairConfiguration is the effective repair section with defaults applied.
func (s UserSettings) RepairConfiguration() RepairSettings {
	r := RepairSettings{Repository: DefaultRepairRepository, WatchMinutes: 45, MaxAttempts: MaxRepairAttempts}
	if s.Repair != nil {
		if s.Repair.Repository != "" {
			r.Repository = s.Repair.Repository
		}
		r.Directory, r.AutoMerge, r.About, r.Autonomous, r.Install = s.Repair.Directory, s.Repair.AutoMerge, s.Repair.About, s.Repair.Autonomous, s.Repair.Install
		if s.Repair.WatchMinutes > 0 {
			r.WatchMinutes = s.Repair.WatchMinutes
		}
		if s.Repair.MaxAttempts > 0 {
			r.MaxAttempts = s.Repair.MaxAttempts
		}
	}
	return r
}

// Keep settings added by newer AXLR versions when an older console saves a
// model or theme. JSON values remain raw until the version that owns them.
func (s *UserSettings) UnmarshalJSON(data []byte) error {
	type known UserSettings
	value := known(DefaultUserSettings())
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errors.New("settings.json must contain a JSON object")
	}
	for key := range fields {
		for _, knownKey := range []string{"model", "language", "theme", "icons", "reduce_motion", "approvals", "favorite_models", "reviewer_model", "repair", "context_tokens", "local_models", "models", "jev", "ceremonies", "plan", "trace_retention_days", "trace_payloads", "turn_tool_calls", "prompt_tokens", "exec_sandbox", "engines", "max_session_usd", "jobs", "forged_tools", "notify"} {
			if strings.EqualFold(key, knownKey) {
				delete(fields, key)
				break
			}
		}
	}
	*s = UserSettings(value)
	s.Extra = fields
	return nil
}

func (s UserSettings) MarshalJSON() ([]byte, error) {
	type known UserSettings
	data, err := json.Marshal(known(s))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for key, value := range s.Extra {
		if _, owned := fields[key]; !owned {
			fields[key] = value
		}
	}
	return json.Marshal(fields)
}

func DefaultUserSettings() UserSettings {
	return UserSettings{Language: "en", Theme: string(domain.ThemeAuto), Icons: string(domain.IconsSafe)}
}

func (s UserSettings) Validate() error {
	if s.Model != "" {
		if _, err := root.NewModelID(s.Model); err != nil {
			return errors.New("invalid settings model")
		}
	}
	if s.ReviewerModel != "" {
		if _, err := root.NewModelID(s.ReviewerModel); err != nil {
			return errors.New("invalid reviewer_model in settings.json")
		}
	}
	if r := s.Repair; r != nil {
		if r.Repository != "" && !repositoryName(r.Repository) {
			return errors.New("settings repair.repository must be owner/name")
		}
		if r.Directory != "" && !filepath.IsAbs(r.Directory) {
			return errors.New("settings repair.directory must be an absolute path")
		}
		if r.WatchMinutes < 0 || r.WatchMinutes > 720 {
			return errors.New("settings repair.watch_minutes must be between 1 and 720")
		}
		if r.MaxAttempts < 0 || r.MaxAttempts > 5 {
			return errors.New("settings repair.max_attempts must be between 1 and 5")
		}
	}
	if s.ContextTokens != 0 && (s.ContextTokens < domain.MinimumContextWindow || s.ContextTokens > maxContextTokens) {
		return fmt.Errorf("settings context_tokens must be between %d and %d", domain.MinimumContextWindow, maxContextTokens)
	}
	if s.PromptTokens != 0 && (s.PromptTokens < domain.MinimumPromptTokens || s.PromptTokens > maxContextTokens) {
		return fmt.Errorf("settings prompt_tokens must be between %d and %d", domain.MinimumPromptTokens, maxContextTokens)
	}
	if err := s.Sandbox.validate(); err != nil {
		return err
	}
	if err := s.Jobs.validate(); err != nil {
		return err
	}
	switch s.Notify {
	case "", "bell", "osc", "off":
	default:
		return errors.New(`settings notify must be "bell", "osc" or "off"`)
	}
	if s.TurnToolCalls != 0 && (s.TurnToolCalls < minTurnToolCalls || s.TurnToolCalls > maxTurnToolCalls) {
		return fmt.Errorf("settings turn_tool_calls must be between %d and %d", minTurnToolCalls, maxTurnToolCalls)
	}
	if days := s.TraceRetentionDays; days != nil && (*days < 0 || *days > maxTraceRetentionDays) {
		return fmt.Errorf("settings trace_retention_days must be between 0 and %d", maxTraceRetentionDays)
	}
	if err := s.validateSessionBudget(); err != nil {
		return err
	}
	if len(s.LocalModels) > maxLocalModels {
		return errors.New("settings.json lists too many local models")
	}
	localIDs := make(map[string]struct{}, len(s.LocalModels))
	for _, local := range s.LocalModels {
		if err := local.validate(); err != nil {
			return err
		}
		if _, duplicate := localIDs[local.ID]; duplicate {
			return fmt.Errorf("settings local_models lists %s twice", local.ID)
		}
		localIDs[local.ID] = struct{}{}
	}
	if err := s.validateModels(localIDs); err != nil {
		return err
	}
	if profile := s.CeremonyProfile(); profile != "auto" && profile != "standard" && profile != "compact" {
		return errors.New("settings ceremonies.profile must be auto, standard or compact")
	}
	if p := s.Plan; p != nil && p.Model != "" && p.Model != "session" {
		if _, err := root.NewModelID(p.Model); err != nil {
			return errors.New("settings plan.model must be a model id or session")
		}
	}
	if j := s.Jev; j != nil {
		if j.FinalThreshold < 0 || j.FinalThreshold >= 1 {
			return errors.New("settings jev.final_threshold must be between 0 and 1")
		}
		if j.TimeoutMS != 0 && (j.TimeoutMS < 1000 || j.TimeoutMS > 60000) {
			return errors.New("settings jev.timeout_ms must be between 1000 and 60000")
		}
	}
	if len(s.FavoriteModels) > maxFavoriteModels {
		return errors.New("settings.json lists too many favorite models")
	}
	for _, id := range s.FavoriteModels {
		if _, err := root.NewModelID(id); err != nil {
			return errors.New("invalid favorite model in settings.json")
		}
	}
	if s.Language != "en" && s.Language != "es" {
		return errors.New("settings language must be en or es")
	}
	if s.Approvals != nil {
		for _, id := range s.Approvals.Allowed {
			if id.Validate() != nil || id.Kind == domain.ToolKindHost {
				return errors.New("invalid approved tool identity in settings.json")
			}
		}
	}
	return (domain.UIPreferences{Theme: domain.ThemeID(s.Theme), Icons: domain.IconProfile(s.Icons), ReduceMotion: s.ReduceMotion}).Validate()
}

func (s UserSettings) UIPreferences() domain.UIPreferences {
	return domain.UIPreferences{Theme: domain.ThemeID(s.Theme), Icons: domain.IconProfile(s.Icons), ReduceMotion: s.ReduceMotion}
}

// repositoryName accepts owner/name with the characters GitHub allows.
func repositoryName(raw string) bool {
	owner, name, ok := strings.Cut(raw, "/")
	if !ok || owner == "" || name == "" || strings.Contains(name, "/") {
		return false
	}
	for _, part := range []string{owner, name} {
		for _, r := range part {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
				return false
			}
		}
	}
	return true
}

// DefaultContextCapTokens is the prompt cap a complete settings file
// writes (decision D10 of the lifecycle integration design).
const DefaultContextCapTokens = 75000

// CompleteDefaults fills every section a clean install decides (decision
// D9): autonomy on with the destructive list, engines shared and required,
// forged tools on, Jev on when its key exists, the context cap, retention,
// jobs, the plan's model and the ceremony profile; a section already set is
// kept. Repair and the sandbox have no default a machine can decide.
func CompleteDefaults(s *UserSettings, jev bool) {
	if s.Language == "" {
		s.Language = "en"
	}
	if s.Approvals == nil {
		s.Approvals = &ApprovalPreferences{Autonomous: true, Allowed: []domain.ToolIdentity{}}
	}
	if s.Approvals.AlwaysAsk == nil {
		s.Approvals.AlwaysAsk = domain.DestructiveRules()
	}
	if s.Engines == nil {
		s.Engines = &EngineSettings{Shared: true, Required: true}
	}
	if s.ForgedTools == nil {
		on := true
		s.ForgedTools = &on
	}
	if s.Jev == nil {
		s.Jev = &JevSettings{Tool: jev, FinalCheck: jev, FinalThreshold: 0.5, Model: "jev-1.13.0", TimeoutMS: 20000}
	} else if jev && !s.Jev.Tool && !s.Jev.FinalCheck {
		// A setup run that gets the key turns on the Jev it announces; an
		// earlier run without one had written it off.
		s.Jev.Tool, s.Jev.FinalCheck = true, true
	}
	if s.PromptTokens == 0 {
		s.PromptTokens = DefaultContextCapTokens
	}
	if s.TraceRetentionDays == nil {
		days := DefaultTraceRetentionDays
		s.TraceRetentionDays = &days
	}
	if s.Jobs == nil {
		s.Jobs = &JobsSettings{MaxActive: DefaultMaxActiveJobs}
	}
	if s.Plan == nil {
		s.Plan = &PlanSettings{Model: "session"}
	}
	if s.Ceremonies == nil {
		s.Ceremonies = &CeremonySettings{Profile: "auto"}
	}
}
