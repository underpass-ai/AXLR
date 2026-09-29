package terminal

type Layout struct {
	Width, Height, TranscriptWidth, BodyHeight int
	SidePanel, TooSmall                        bool
}

func NewLayout(w, h int) Layout {
	l := Layout{Width: w, Height: h, TooSmall: w < 50 || h < 15, SidePanel: w >= 90, TranscriptWidth: w, BodyHeight: max(1, h-7)}
	if l.SidePanel {
		l.TranscriptWidth = w - 29
	} else {
		l.BodyHeight--
	}
	return l
}
