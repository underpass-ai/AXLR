package terminal

func (m *AppModel) resizeComposer() {
	if m.Layout.Height <= 0 {
		return
	}
	maxHeight := min(8, max(2, m.Layout.Height/3))
	m.Composer.Input.MaxHeight = maxHeight
	m.Composer.Input.SetHeight(min(m.Composer.Input.Height(), maxHeight))
	m.Layout.BodyHeight = max(1, m.Layout.Height-m.Composer.Input.Height()-6)
	m.Transcript.Viewport.SetHeight(m.Layout.BodyHeight)
}
