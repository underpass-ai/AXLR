package service

import (
	"strings"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
)

// eventBatcher bounds journal writes while preserving the order of model text
// and subsequent state/tool events. Buffered text is never sent to SSE.
type eventBatcher struct {
	server      *Server
	sessionID   string
	operationID string
	actor       string
	requestID   string
	text        strings.Builder
	lastPersist time.Time
}

func newEventBatcher(server *Server, sessionID, operationID, actor, requestID string) *eventBatcher {
	return &eventBatcher{server: server, sessionID: sessionID, operationID: operationID, actor: actor, requestID: requestID, lastPersist: time.Now()}
}

func (b *eventBatcher) Emit(event application.Event) error {
	if event.Kind != application.EventTextDelta {
		if err := b.Flush(); err != nil {
			return err
		}
		return b.server.recordApplicationEvent(b.sessionID, b.operationID, b.actor, b.requestID, event)
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
	if err := b.server.recordApplicationEvent(b.sessionID, b.operationID, b.actor, b.requestID, application.Event{Kind: application.EventTextDelta, Text: root.Text(text)}); err != nil {
		return err
	}
	b.text.Reset()
	b.lastPersist = time.Now()
	return nil
}
