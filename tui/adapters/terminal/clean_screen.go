package terminal

// cleanScreen empties the conversation on the screen and keeps the
// session: its messages, the model's context, /copy, /export and /usage
// are as they were, and what comes next is drawn below an empty screen.
// /new (alias /clear) is what starts another session.
func (m AppModel) cleanScreen() AppModel {
	m.Composer.Input.Reset()
	m.overlay = ""
	m.Transcript.Clear(m.Header.State)
	// The failures, the unsent prompts and the cancelled mark are drawn
	// from the console's own state, not the session's.
	m.failures, m.unsentPrompts, m.cancelledAt = nil, nil, 0
	m.refreshTranscript()
	m.Status.Error = ""
	m.Status.Notice = m.Theme.T("notice.screenCleaned")
	return m
}
