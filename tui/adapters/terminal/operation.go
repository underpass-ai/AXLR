package terminal

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Operation runs against a private session snapshot. Only completion publishes it.
type Operation func(context.Context, *domain.Session, func(application.Event) error) error

func readOperation(ch <-chan tea.Msg) tea.Cmd { return func() tea.Msg { return <-ch } }
func (m *AppModel) BeginOperation(run Operation) tea.Cmd {
	if m.Busy {
		return nil
	}
	var snapshot domain.Session
	if m.deps.Session != nil {
		snapshot = *m.deps.Session
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.Busy = true
	ch := make(chan tea.Msg, 32)
	m.events = ch
	return func() tea.Msg {
		go func() {
			err := run(ctx, &snapshot, func(e application.Event) error {
				if e.Usage != nil {
					usage := *e.Usage
					e.Usage = &usage
				}
				select {
				case ch <- e:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			})
			ch <- operationComplete{Session: snapshot, Err: err}
			close(ch)
		}()
		return <-ch
	}
}
