package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// prepareSessionDefinition publishes axlr_session when the connected MADE
// store lacks it, through the same preparer as P in /mcp: an install from
// before the definition existed would otherwise ask the person to press P
// before any session is recorded. The preparer is idempotent and only the
// console publishes (decision H4). It returns one line for the log.
func prepareSessionDefinition(ctx context.Context, engine application.CeremonyEnginePort, preparer application.OrchestratorPreparationPort) string {
	if engine == nil {
		return ""
	}
	err := engine.Ready(ctx, application.SessionDefinition, application.SessionDefinitionVersion)
	if err == nil {
		return ""
	}
	if !errors.Is(err, application.ErrCeremonyNotPrepared) || preparer == nil {
		return "session ceremony: " + err.Error()
	}
	result, err := preparer.Prepare(ctx)
	if err != nil {
		return "session ceremony: publishing axlr_session failed: " + err.Error()
	}
	if len(result.Published) > 0 {
		return "session ceremony: published " + strings.Join(result.Published, ", ")
	}
	return fmt.Sprintf("session ceremony: preparation %s, axlr_session still missing: open /mcp, select MADE and press p", result.Status)
}

// sweepAbandonedSessions ends, as abandoned, the open instances of this
// workspace's sessions no console holds, and returns one line for the log.
func sweepAbandonedSessions(ctx context.Context, ceremony *application.SessionCeremony, store application.SessionLister, workspace domain.Workspace, current domain.SessionID) string {
	if ceremony == nil {
		return ""
	}
	ended, err := ceremony.SweepAbandoned(ctx, store, workspace, current)
	if err != nil {
		return "session ceremony: sweep of abandoned instances: " + err.Error()
	}
	if ended == 0 {
		return ""
	}
	return fmt.Sprintf("session ceremony: %d abandoned instances ended", ended)
}

// consoleSessionID is the session this launch opens: the resumed one, or
// the new id drawn for it.
func consoleSessionID(session *domain.Session, newID domain.SessionID) domain.SessionID {
	if session != nil {
		return session.Export().ID
	}
	return newID
}

// ensureWorkGrant issues the console's current MADE work grant once per
// grant id: the id changes when its actions or the work identity do
// (madesetup.WorkGrantFor), and
// an install prepared under the previous id would otherwise see MADE
// refuse the new verbs (the handoff, the draft explanation) until the
// person pressed P in /mcp. The marker file remembers the id issued; the
// preparer is idempotent. It returns one line for the log.
func ensureWorkGrant(ctx context.Context, preparer application.OrchestratorPreparationPort, marker, grant string) string {
	if preparer == nil || marker == "" {
		return ""
	}
	if data, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(data)) == grant {
		return ""
	}
	result, err := preparer.Prepare(ctx)
	if err != nil {
		return "MADE work grant " + grant + ": " + err.Error()
	}
	switch result.Status {
	case "ready", "granted", "remote", "unsupported":
		// A remote or operator-managed MADE will not take the grant from
		// this console at any start: the marker remembers that too.
		_ = os.MkdirAll(filepath.Dir(marker), 0o700)
		_ = os.WriteFile(marker, []byte(grant+"\n"), 0o600)
		return "MADE work grant " + grant + " " + strings.TrimSpace(result.Status+" "+result.Detail)
	}
	return "MADE work grant " + grant + ": " + result.Status + " " + result.Detail
}
