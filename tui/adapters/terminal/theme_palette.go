package terminal

import "github.com/underpass-ai/AXLR/tui/domain"

type ThemePalette struct {
	Background, Surface, Raised, User, Memory, Text, Muted, Accent, Selected, Good, Warning, Border, DiffRemoved string
}

var themePalettes = map[domain.ThemeID]ThemePalette{
	domain.ThemeInk:      {"#11131C", "#1B2030", "#252C40", "#203C48", "#342B4D", "#E8EDF7", "#9BA6BC", "#7DD3FC", "#364765", "#6EE7B7", "#FBBF72", "#526078", "#FCA5A5"},
	domain.ThemeAurora:   {"#091D29", "#112B38", "#173B48", "#174B50", "#25465E", "#E4F9F4", "#A7C7C6", "#5EEAD4", "#22636A", "#A3E635", "#F9C66B", "#32717C", "#FDA4AF"},
	domain.ThemePaper:    {"#F7F3E9", "#FFFDF7", "#EBE4D8", "#E3EFE9", "#EEE7F2", "#263342", "#627083", "#315AA7", "#CEDDFA", "#187B61", "#A9542C", "#A7B1BA", "#B13D50"},
	domain.ThemePhosphor: {"#08120B", "#102015", "#18321D", "#16321A", "#26321B", "#D8F2CE", "#83A984", "#87E66C", "#23502A", "#B7F173", "#E4C66D", "#477D4B", "#EEA38C"},
}
