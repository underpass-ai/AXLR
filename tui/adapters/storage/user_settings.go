package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const maxUserSettingsBytes = 64 << 10

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
	TraceRetentionDays *int                       `json:"trace_retention_days,omitempty"`
	Extra              map[string]json.RawMessage `json:"-"`
}

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

// DefaultPlanner is the model that decomposes a brief unless plan.model
// says otherwise: a large model plans, the session's local model works.
const DefaultPlanner = "z-ai/glm-5.3-flash"

// PlanSettings is the plan section: Model plans (default DefaultPlanner;
// "session" uses the session's model) and AutoApprove starts a verified
// plan without the person (default false).
type PlanSettings struct {
	Model       string `json:"model,omitempty"`
	AutoApprove bool   `json:"auto_approve,omitempty"`
}

// Planner is the configured planner with the default applied; empty means
// the session's model.
func (s UserSettings) Planner() string {
	if s.Plan == nil || s.Plan.Model == "" {
		return DefaultPlanner
	}
	if s.Plan.Model == "session" {
		return ""
	}
	return s.Plan.Model
}

// CeremonySettings is the ceremonies section. Profile is "auto" (the
// default: compact when the session model's known window is at most
// CompactWindowTokens), "standard" or "compact".
type CeremonySettings struct {
	Profile string `json:"profile,omitempty"`
}

// CompactWindowTokens is the largest window the auto profile treats as small.
const CompactWindowTokens = 65536

// CeremonyProfile is the configured profile with the default applied.
func (s UserSettings) CeremonyProfile() string {
	if s.Ceremonies == nil || s.Ceremonies.Profile == "" {
		return "auto"
	}
	return s.Ceremonies.Profile
}

// JevSettings is the jev section. Tool offers axlr_judge to the model;
// FinalCheck asks Jev whether a final answer completes the request and
// returns a doubted answer to the model once; FinalThreshold is the
// probability below which it does (default 0.5). Model is the pinned Jev
// version (default jev-1.13.0, as KMP); TimeoutMS bounds one request
// (default 20000). The key is read from TYPESAFE_API_KEY.
type JevSettings struct {
	Tool           bool    `json:"tool"`
	FinalCheck     bool    `json:"final_check"`
	FinalThreshold float64 `json:"final_threshold,omitempty"`
	Model          string  `json:"model,omitempty"`
	TimeoutMS      int     `json:"timeout_ms,omitempty"`
}

// Enabled reports whether either switch sends anything to TypeSafe.
func (j *JevSettings) Enabled() bool { return j != nil && (j.Tool || j.FinalCheck) }

// LocalModel is one entry of local_models. ID is what the session stores and
// /model shows (a "local/" prefix keeps it apart from OpenRouter ids); URL is
// the server's OpenAI base URL, such as http://127.0.0.1:8080/v1, to which
// /chat/completions is appended; Model is the name sent to the server
// (default ID; vLLM needs its served model name, llama.cpp ignores it);
// APIKeyEnv names the environment variable holding the key, required unless
// the URL is a loopback address; ContextTokens is the window the console
// respects, which may be smaller than the server's; Tools false hides a model
// whose server cannot return native tool calls, since AXLR needs them.
type LocalModel struct {
	ID                string `json:"id"`
	Name              string `json:"name,omitempty"`
	URL               string `json:"url"`
	Model             string `json:"model,omitempty"`
	APIKeyEnv         string `json:"api_key_env,omitempty"`
	ContextTokens     int    `json:"context_tokens"`
	Tools             *bool  `json:"tools,omitempty"`
	StreamIdleSeconds int    `json:"stream_idle_seconds,omitempty"`
	StreamMaxMinutes  int    `json:"stream_max_minutes,omitempty"`
	// Stream false asks the server for whole replies instead of a stream.
	Stream *bool `json:"stream,omitempty"`
	// Thinking false turns the model's thinking off through the chat
	// template (enable_thinking); workers want short replies.
	Thinking *bool `json:"thinking,omitempty"`
}

// Streams reports whether requests to this server are streamed (default).
func (m LocalModel) Streams() bool { return m.Stream == nil || *m.Stream }

// Defaults for a local model's stream: a cold prefill of a long prompt on a
// local GPU takes minutes before the first token, and decoding runs at a few
// tokens per second.
const (
	DefaultLocalStreamIdleSeconds = 600
	DefaultLocalStreamMaxMinutes  = 60
	maxLocalModels                = 16
	maxContextTokens              = 16 << 20
)

// SupportsTools reports whether the server returns native tool calls.
func (m LocalModel) SupportsTools() bool { return m.Tools == nil || *m.Tools }

// UpstreamModel is the model name sent to the server.
func (m LocalModel) UpstreamModel() string {
	if m.Model != "" {
		return m.Model
	}
	return m.ID
}

// ChatCompletionsURL is the endpoint the console posts to.
func (m LocalModel) ChatCompletionsURL() string {
	return strings.TrimRight(m.URL, "/") + "/chat/completions"
}

// StreamIdle and StreamMax are the stream limits with defaults applied.
func (m LocalModel) StreamIdle() time.Duration {
	if m.StreamIdleSeconds > 0 {
		return time.Duration(m.StreamIdleSeconds) * time.Second
	}
	return DefaultLocalStreamIdleSeconds * time.Second
}

func (m LocalModel) StreamMax() time.Duration {
	if m.StreamMaxMinutes > 0 {
		return time.Duration(m.StreamMaxMinutes) * time.Minute
	}
	return DefaultLocalStreamMaxMinutes * time.Minute
}

func (m LocalModel) validate() error {
	if _, err := root.NewModelID(m.ID); err != nil {
		return errors.New("settings local_models entry has an invalid id")
	}
	if !utf8.ValidString(m.Name) || len(m.Name) > 200 || strings.ContainsAny(m.Name, "\r\n") {
		return fmt.Errorf("settings local_models %s has an invalid name", m.ID)
	}
	parsed, err := url.Parse(m.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("settings local_models %s url must be an absolute http(s) URL without credentials, query or fragment", m.ID)
	}
	if m.Model != "" {
		if _, err := root.NewModelID(m.Model); err != nil {
			return fmt.Errorf("settings local_models %s has an invalid model", m.ID)
		}
	}
	if m.APIKeyEnv != "" && !mcpEnvironmentName.MatchString(m.APIKeyEnv) {
		return fmt.Errorf("settings local_models %s api_key_env must be an environment variable name", m.ID)
	}
	if m.ContextTokens < domain.MinimumContextWindow || m.ContextTokens > maxContextTokens {
		return fmt.Errorf("settings local_models %s context_tokens must be between %d and %d", m.ID, domain.MinimumContextWindow, maxContextTokens)
	}
	if m.StreamIdleSeconds < 0 || m.StreamIdleSeconds > 3600 {
		return fmt.Errorf("settings local_models %s stream_idle_seconds must be between 1 and 3600", m.ID)
	}
	if m.StreamMaxMinutes < 0 || m.StreamMaxMinutes > 720 {
		return fmt.Errorf("settings local_models %s stream_max_minutes must be between 1 and 720", m.ID)
	}
	return nil
}

// RepairSettings is the repair section of settings.json. Repository is
// owner/name; an empty Directory means <data>/axlr/repairs; WatchMinutes
// bounds one check round (default 45); About is the KMP project about
// (default project:<name>). Autonomous lets a repair session the agent
// started run local tools in its clone without a card (default true; the
// check command and the merge keep their approvals); MaxAttempts bounds the
// repair sessions one failure may start (default 2, at most 5).
type RepairSettings struct {
	Repository   string `json:"repository"`
	Directory    string `json:"directory,omitempty"`
	AutoMerge    bool   `json:"auto_merge"`
	WatchMinutes int    `json:"watch_minutes,omitempty"`
	About        string `json:"about,omitempty"`
	Autonomous   *bool  `json:"autonomous,omitempty"`
	MaxAttempts  int    `json:"max_attempts,omitempty"`
}

// AutonomousLocal reports whether an agent-started repair session runs local
// tools in its clone without the person's card.
func (r RepairSettings) AutonomousLocal() bool {
	return r.Autonomous == nil || *r.Autonomous
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
		r.Directory, r.AutoMerge, r.About, r.Autonomous = s.Repair.Directory, s.Repair.AutoMerge, s.Repair.About, s.Repair.Autonomous
		if s.Repair.WatchMinutes > 0 {
			r.WatchMinutes = s.Repair.WatchMinutes
		}
		if s.Repair.MaxAttempts > 0 {
			r.MaxAttempts = s.Repair.MaxAttempts
		}
	}
	return r
}

type ApprovalPreferences struct {
	Autonomous bool                       `json:"autonomous"`
	Allowed    []domain.ToolIdentity      `json:"allowed"`
	Extra      map[string]json.RawMessage `json:"-"`
}

func (p *ApprovalPreferences) UnmarshalJSON(data []byte) error {
	type known ApprovalPreferences
	var value known
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return errors.New("approvals must be a JSON object")
	}
	for key := range fields {
		if strings.EqualFold(key, "autonomous") || strings.EqualFold(key, "allowed") {
			delete(fields, key)
		}
	}
	*p = ApprovalPreferences(value)
	p.Extra = fields
	return nil
}

func (p ApprovalPreferences) MarshalJSON() ([]byte, error) {
	type known ApprovalPreferences
	data, err := json.Marshal(known(p))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for key, value := range p.Extra {
		if _, owned := fields[key]; !owned {
			fields[key] = value
		}
	}
	return json.Marshal(fields)
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
		for _, knownKey := range []string{"model", "language", "theme", "icons", "reduce_motion", "approvals", "favorite_models", "reviewer_model", "repair", "context_tokens", "local_models", "models", "jev", "ceremonies", "plan", "trace_retention_days"} {
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
	if days := s.TraceRetentionDays; days != nil && (*days < 0 || *days > maxTraceRetentionDays) {
		return fmt.Errorf("settings trace_retention_days must be between 0 and %d", maxTraceRetentionDays)
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

// UserSettingsStore preserves unrelated keys when /model or /theme saves.
// Initial is used only until settings.json is created, allowing old preference
// files to be read without changing them.
type UserSettingsStore struct {
	Path    string
	Initial UserSettings
	mu      sync.Mutex
}

func NewUserSettingsStore(path string, initial UserSettings) (*UserSettingsStore, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("settings path must be absolute")
	}
	if err := initial.Validate(); err != nil {
		return nil, err
	}
	return &UserSettingsStore{Path: path, Initial: initial}, nil
}

func (s *UserSettingsStore) Load(ctx context.Context) (UserSettings, error) {
	if err := ctx.Err(); err != nil {
		return UserSettings{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read()
}

func (s *UserSettingsStore) read() (UserSettings, error) {
	file, err := openNoFollow(s.Path, os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		return s.Initial, nil
	}
	if err != nil {
		return UserSettings{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return UserSettings{}, err
	}
	if !info.Mode().IsRegular() || writableByOthers(info) || info.Size() > maxUserSettingsBytes {
		return UserSettings{}, errors.New("settings.json must be a regular, non-writable-by-others file of at most 64 KiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxUserSettingsBytes+1))
	if err != nil {
		return UserSettings{}, err
	}
	if len(data) > maxUserSettingsBytes || !utf8.Valid(data) {
		return UserSettings{}, errors.New("invalid settings.json size or UTF-8")
	}
	settings := DefaultUserSettings()
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&settings); err != nil || decoder.Decode(new(any)) != io.EOF {
		return UserSettings{}, errors.New("invalid settings.json JSON")
	}
	if err := settings.Validate(); err != nil {
		return UserSettings{}, err
	}
	return settings, nil
}

func (s *UserSettingsStore) update(ctx context.Context, change func(*UserSettings)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return err
	}
	lock, err := openNoFollow(s.Path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	info, err := lock.Stat()
	if err != nil {
		return err
	}
	if !privateRegular(info) {
		return errors.New("settings lock must be a private regular file")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := tryLock(lock)
		if err == nil {
			break
		}
		if !lockBusy(err) {
			return err
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	settings, err := s.read()
	if err != nil {
		return err
	}
	change(&settings)
	if err := settings.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > maxUserSettingsBytes {
		return errors.New("settings.json exceeds 64 KiB")
	}
	file, err := os.CreateTemp(filepath.Dir(s.Path), ".axlr-settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), s.Path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(s.Path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return syncDirectoryFile(dir)
}

type settingsModelPreference struct{ store *UserSettingsStore }

var _ application.ModelPreferencePort = settingsModelPreference{}

func (s *UserSettingsStore) ModelPreference() application.ModelPreferencePort {
	return settingsModelPreference{store: s}
}

func (p settingsModelPreference) Load(ctx context.Context) (root.ModelID, error) {
	s, err := p.store.Load(ctx)
	return root.ModelID(s.Model), err
}

func (p settingsModelPreference) Save(ctx context.Context, model root.ModelID) error {
	if _, err := root.NewModelID(string(model)); err != nil {
		return err
	}
	return p.store.update(ctx, func(s *UserSettings) { s.Model = string(model) })
}

type settingsUIPreference struct{ store *UserSettingsStore }

var _ application.UIPreferencePort = settingsUIPreference{}

func (s *UserSettingsStore) UIPreference() application.UIPreferencePort {
	return settingsUIPreference{store: s}
}

func (p settingsUIPreference) Load(ctx context.Context) (domain.UIPreferences, error) {
	s, err := p.store.Load(ctx)
	return s.UIPreferences(), err
}

func (p settingsUIPreference) Save(ctx context.Context, ui domain.UIPreferences) error {
	if err := ui.Validate(); err != nil {
		return err
	}
	return p.store.update(ctx, func(s *UserSettings) {
		s.Theme = string(ui.Theme)
		s.Icons = string(ui.Icons)
		s.ReduceMotion = ui.ReduceMotion
	})
}

type settingsModelFavorites struct{ store *UserSettingsStore }

var _ application.ModelFavoritesPort = settingsModelFavorites{}

func (s *UserSettingsStore) ModelFavorites() application.ModelFavoritesPort {
	return settingsModelFavorites{store: s}
}

func (p settingsModelFavorites) Load(ctx context.Context) ([]root.ModelID, error) {
	s, err := p.store.Load(ctx)
	return modelIDs(s.FavoriteModels), err
}

func (p settingsModelFavorites) Toggle(ctx context.Context, model root.ModelID) ([]root.ModelID, error) {
	if _, err := root.NewModelID(string(model)); err != nil {
		return nil, err
	}
	var result []string
	err := p.store.update(ctx, func(s *UserSettings) {
		next := make([]string, 0, len(s.FavoriteModels)+1)
		removed := false
		for _, id := range s.FavoriteModels {
			if id == string(model) {
				removed = true
				continue
			}
			next = append(next, id)
		}
		if !removed && len(next) < maxFavoriteModels {
			next = append(next, string(model))
		}
		s.FavoriteModels = next
		result = next
	})
	return modelIDs(result), err
}

func modelIDs(ids []string) []root.ModelID {
	out := make([]root.ModelID, len(ids))
	for i, id := range ids {
		out[i] = root.ModelID(id)
	}
	return out
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
