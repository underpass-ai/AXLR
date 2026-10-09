package terminal

// AppendFailures shows, after the conversation, the errors of operations
// that ended while a message was queued. The queued message starts the next
// turn at once, which clears the footer error before it is ever drawn, so
// the failure is kept here instead.
func (t *Transcript) AppendFailures(failures []string) {
	if len(failures) == 0 {
		return
	}
	for _, failure := range failures {
		t.appendRow(transcriptRow{Label: t.theme.Icon("error") + " ", LabelTone: toneError, Text: t.theme.T("transcript.failedBeforeQueued") + singleLine(failure), Kind: transcriptRowAssistant, Indent: true})
	}
	t.renderRows()
	t.ApplyTheme(t.theme)
}

// AppendCancelled marks the request the person stopped with Esc, so the
// conversation does not end as if it were still waiting for the answer.
func (t *Transcript) AppendCancelled() {
	t.appendRow(transcriptRow{Label: t.theme.Icon("attention") + " ", LabelTone: toneWarning, Text: t.theme.T("transcript.cancelled"), Kind: transcriptRowPlain, Indent: true})
	t.renderRows()
	t.ApplyTheme(t.theme)
}
