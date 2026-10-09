package terminal

func (m *AppModel) resizeComposer() {
	if m.Layout.Height <= 0 {
		return
	}
	maxHeight := min(8, max(2, m.Layout.Height/3))
	m.Composer.Input.MaxHeight = maxHeight
	m.Composer.Input.SetHeight(min(m.Composer.Input.Height(), maxHeight))
	m.Layout.BodyHeight = max(1, m.Layout.Height-m.Composer.Input.Height()-composerChromeRows)
	if m.Transcript.Viewport.Height() == m.Layout.BodyHeight {
		return
	}
	// On 10 October 2026 a pasted five-line prompt shrank the transcript
	// under a growing composer; the view fell off the bottom and the reply
	// to that prompt arrived out of sight. Keep following when it was.
	bottom := m.Transcript.Viewport.AtBottom()
	m.Transcript.Viewport.SetHeight(m.Layout.BodyHeight)
	if bottom {
		m.Transcript.Viewport.GotoBottom()
	}
}

// composerChromeRows are the main view's fixed rows around the composer
// input: header, the rule above the input and the footer.
const composerChromeRows = 3
