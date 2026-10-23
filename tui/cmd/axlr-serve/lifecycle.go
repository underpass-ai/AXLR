package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/adapters/axlr"
	"github.com/underpass-ai/AXLR/tui/adapters/ceremonyhost"
	"github.com/underpass-ai/AXLR/tui/adapters/localmodels"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/compose"
	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/service"
)

// serviceOwner is the lease owner of the service's MADE claims: the
// engine adapter's default actor, which MADE accepts status reports from
// only when it is also the host identity's owner.
const serviceOwner = "axlr"

// serviceSettings reads settings_file as the console reads its own; no file
// configured keeps the defaults, and a configured file that is missing is
// refused rather than silently ignored.
func serviceSettings(ctx context.Context, path string) (storage.UserSettings, error) {
	defaults := storage.DefaultUserSettings()
	if path == "" {
		return defaults, nil
	}
	if _, err := os.Stat(path); err != nil {
		return storage.UserSettings{}, err
	}
	store, err := storage.NewUserSettingsStore(path, defaults)
	if err != nil {
		return storage.UserSettings{}, err
	}
	return store.Load(ctx)
}

// sessionLifecycle makes every service session a MADE axlr_session
// instance with its KMP context, as the console's sessions are (decisions
// D3 and D4), and sweeps the instances a stopped service left open. The
// ceremony driver it returns is nil when MADE is not registered.
func sessionLifecycle(ctx context.Context, cfg service.Config, registrations []plugins.Registration, settings storage.UserSettings, runner axlr.ToolRunner, models axlr.ModelStream, labels *storage.SessionLabelStore, sessions *service.Sessions, windows localmodels.Windows, judge *application.Judge, note func(string)) (*application.SessionCeremony, *application.CeremonyDriver, error) {
	installID, err := storage.InstallID(cfg.StateDir)
	if err != nil {
		return nil, nil, fmt.Errorf("install id: %w", err)
	}
	var runToken [8]byte
	if _, err := rand.Read(runToken[:]); err != nil {
		return nil, nil, err
	}
	hostname, _ := os.Hostname()
	host := application.HostIdentity{Owner: serviceOwner, AgentID: installID, Incarnation: hex.EncodeToString(runToken[:]), Hostname: hostname}
	lifecycle := &application.SessionCeremony{Labels: labels, Usage: sessions.Usage(), Store: sessions.Store(), Host: host, Log: note}
	memory := ceremonyhost.Memory{Tools: runner}
	// The registry key names the remote store the service's engine reaches.
	lifecycle.Identity = &application.MemoryIdentity{Store: "kmp:" + cfg.KMP.Endpoint, RegistrationKey: "axlr-serve:" + installID, Agents: storage.NewMemoryAgentStore(filepath.Join(cfg.StateDir, "kmp-agents.json")), Guide: memory, Recall: memory}
	ceremonies := compose.CeremonyDriver(registrations, runner, labels, serviceOwner)
	if ceremonies == nil {
		return lifecycle, nil, nil
	}
	ceremonies.Compact = compose.CompactProfile(settings.CeremonyProfile(), windows)
	ceremonies.Host, ceremonies.Roles = host, ceremonyhost.RoleOf
	ceremonies.Files = ceremonyhost.Files{Tools: runner}
	ceremonies.Reviewer = ceremonyhost.Reviewer{Models: models, Model: settings.ReviewerModel}
	ceremonies.Forge = ceremonyhost.Forge{Checks: ceremonyhost.Checks{Tools: runner}}
	lifecycle.Engine = ceremonies.Engine
	if journal, ok := ceremonies.Memory.(application.JournalPort); ok {
		lifecycle.Journal = journal
	}
	if judge != nil {
		// Jev decides what each turn settled (decision D5).
		compose.SettleWith(lifecycle, judge)
	}
	// The operator installs axlr_session through MADE's own channels; the
	// service says when it is missing and records no instance until then.
	if err := ceremonies.Engine.Ready(ctx, application.SessionDefinition, application.SessionDefinitionVersion); err != nil {
		if errors.Is(err, application.ErrCeremonyNotPrepared) {
			note("session ceremony: axlr_session is not published in MADE; sessions run without an instance")
		} else {
			note("session ceremony: " + err.Error())
		}
	}
	if ended, err := lifecycle.SweepAbandoned(ctx, sessions.Lister(), domain.Workspace(cfg.Workspace), ""); err != nil {
		note("session ceremony: sweep of abandoned instances: " + err.Error())
	} else if ended > 0 {
		note(fmt.Sprintf("session ceremony: %d abandoned instances ended", ended))
	}
	return lifecycle, ceremonies, nil
}
