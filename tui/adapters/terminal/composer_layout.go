package terminal

func (m *AppModel) resizeComposer() {
	if m.Layout.Height <= 0 {
		return
	}
	maxHeight := min(8, max(2, m.Layout.Height/3))
	m.Composer.Input.MaxHeight = maxHeight
	m.Composer.Input.SetHeight(min(m.Composer.Input.Height(), maxHeight))
	m.Layout.BodyHeight = max(1, m.Layout.Height-m.Composer.Input.Height()-composerChromeRows)
	m.Transcript.Viewport.SetHeight(m.Layout.BodyHeight)
}

// composerChromeRows are the main view's fixed rows around the composer
// input: header, the rule above the input and the footer.
const composerChromeRows = 3
