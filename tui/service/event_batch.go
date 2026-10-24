package service

import (
	"strings"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// eventBatcher bounds journal writes while preserving the order of model text
// and subsequent state/tool events. Buffered text is never sent to SSE.
type eventBatcher struct {
	server      *Server
	sessionID   string
	operationID string
	actor       string
	requestID   string
	// session is the session the use case drives, read when a state event
	// ends the turn to put its result in the event; nil records none.
	session     *domain.Session
	text        strings.Builder
	lastPersist time.Time
}

func newEventBatcher(server *Server, sessionID, operationID, actor, requestID string, session *domain.Session) *eventBatcher {
	return &eventBatcher{server: server, sessionID: sessionID, operationID: operationID, actor: actor, requestID: requestID, session: session, lastPersist: time.Now()}
}

func (b *eventBatcher) Emit(event application.Event) error {
	if event.Kind != application.EventTextDelta {
		if err := b.Flush(); err != nil {
			return err
		}
		return b.server.recordApplicationEvent(b.sessionID, b.operationID, b.actor, b.requestID, event, b.session)
	}
	for _, char := range string(event.Text) {
		if b.text.Len()+len(string(char)) > 4096 {
			if err := b.Flush(); err != nil {
				return err
			}
		}
		b.text.WriteRune(char)
	}
	if b.text.Len() >= 4096 || time.Since(b.lastPersist) >= 100*time.Millisecond {
		return b.Flush()
	}
	return nil
}

func (b *eventBatcher) Flush() error {
	if b.text.Len() == 0 {
		return nil
	}
	text := b.text.String()
	// The session goes along for its correlation alone: a text event
	// carries no result.
	if err := b.server.recordApplicationEvent(b.sessionID, b.operationID, b.actor, b.requestID, application.Event{Kind: application.EventTextDelta, Text: root.Text(text)}, b.session); err != nil {
		return err
	}
	b.text.Reset()
	b.lastPersist = time.Now()
	return nil
}
