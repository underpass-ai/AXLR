package terminal

type Layout struct {
	Width, Height, TranscriptWidth, BodyHeight int
	SidePanel, TooSmall                        bool
}

func NewLayout(w, h int) Layout {
	return Layout{Width: w, Height: h, TooSmall: w < 50 || h < 15, TranscriptWidth: w, BodyHeight: max(1, h-8)}
}
