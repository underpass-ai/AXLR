package terminal

import "github.com/underpass-ai/AXLR/tui/application"

// On 14 October 2026, right after a prompt was sent, the footer kept the
// last turn's "complete" beside "esc cancel" until the first stream event,
// a second or more with recall and a slow provider: people and scripts read
// the turn as finished. A turn sent or continued now reads "Sending…" until
// the turn reports its state.

// markSending marks the operation just begun as a turn waiting for its
// first event.
func (m *AppModel) markSending() {
	if m.Busy {
		m.sending = m.operationID
	}
}

// sendingNow reports a turn sent whose first event has not come.
func (m AppModel) sendingNow() bool {
	return m.Busy && m.sending != 0 && m.sending == m.operationID
}

// sentEvent ends the sending state when the turn reports its state; the
// waiting and executing labels take over before that by themselves.
func (m *AppModel) sentEvent(v application.Event) {
	if v.Kind == application.EventState {
		m.sending = 0
	}
}
