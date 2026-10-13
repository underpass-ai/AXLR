package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// prepareSessionDefinition publishes axlr_session when the connected MADE
// store lacks it, through the same preparer as P in /mcp: an install from
// before the definition existed would otherwise ask the person to press P
// before any session is recorded. The preparer is idempotent and only the
// console publishes (decision H4). It returns one line for the log.
func prepareSessionDefinition(ctx context.Context, engine application.CeremonyEnginePort, preparer application.MADEPreparationPort) string {
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
