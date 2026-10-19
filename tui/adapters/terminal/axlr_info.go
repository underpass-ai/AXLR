package terminal

import (
	"context"
	_ "embed"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/buildinfo"
	"github.com/underpass-ai/AXLR/tui/application"
)

// The emblem the repository's README shows on GitHub, one file per
// background; a test keeps these copies equal to docs/assets/brand.
var (
	//go:embed brand/axlr-emblem-dark.svg
	emblemDarkSVG string
	//go:embed brand/axlr-emblem-light.svg
	emblemLightSVG string
)

// JevInfo is how TypeSafe Jev is connected: its pinned model and the
// methods on, the axlr_judge tool and the final check with its threshold.
// The zero value is Jev off. Policy names the automatic decisions' policy
// when it is not legacy; Supersede is axlr-v2's supersede switch.
type JevInfo struct {
	Model      string
	Tool       bool
	FinalCheck bool
	Threshold  float64
	Policy     string
	Supersede  bool
}

// emblem is the logo as a grid of colours, "" for a transparent pixel.
type emblem [][]string

var emblemRect = regexp.MustCompile(`<rect x="(\d+)" y="(\d+)" width="(\d+)" height="(\d+)" fill="(#[0-9A-Fa-f]{6})"/>`)
var emblemBox = regexp.MustCompile(`viewBox="0 0 (\d+) (\d+)"`)

// parseEmblem reads the SVG's pixel rectangles, dropping the empty rows
// above and below the drawing.
func parseEmblem(svg string) emblem {
	box := emblemBox.FindStringSubmatch(svg)
	if box == nil {
		return nil
	}
	width, _ := strconv.Atoi(box[1])
	height, _ := strconv.Atoi(box[2])
	grid := make(emblem, height)
	for y := range grid {
		grid[y] = make([]string, width)
	}
	for _, r := range emblemRect.FindAllStringSubmatch(svg, -1) {
		x, _ := strconv.Atoi(r[1])
		y, _ := strconv.Atoi(r[2])
		w, _ := strconv.Atoi(r[3])
		h, _ := strconv.Atoi(r[4])
		for row := y; row < min(y+h, height); row++ {
			for col := x; col < min(x+w, width); col++ {
				grid[row][col] = strings.ToUpper(r[5])
			}
		}
	}
	blank := func(row []string) bool { return strings.Join(row, "") == "" }
	for len(grid) > 0 && blank(grid[0]) {
		grid = grid[1:]
	}
	for len(grid) > 0 && blank(grid[len(grid)-1]) {
		grid = grid[:len(grid)-1]
	}
	return grid
}

var emblemDark, emblemLight = parseEmblem(emblemDarkSVG), parseEmblem(emblemLightSVG)

// The emblem's parts, in its columns: the two chevrons and the dash, then
// the four letters.
const emblemMarkEnd = 21

var emblemLetters = [4][2]int{{21, 31}, {33, 43}, {45, 55}, {57, 67}}

const (
	axlrInfoFPS     = 30
	axlrInfoTagline = "axlrInfo.tagline"
	// axlrInfoShine is the colour the sweep across the letters blends to.
	axlrInfoShine = "#FFF4D6"
)

// axlrSpring is one animated value: harmonica's damped spring moves pos to
// target from the frame start on.
type axlrSpring struct {
	pos, vel, target float64
	start            int
	spring           harmonica.Spring
}

func newAxlrSpring(from, to float64, start int, frequency, damping float64) axlrSpring {
	return axlrSpring{pos: from, target: to, start: start, spring: harmonica.NewSpring(harmonica.FPS(axlrInfoFPS), frequency, damping)}
}

// step advances the spring one frame and reports whether it still moves.
func (s *axlrSpring) step(frame int) bool {
	if frame < s.start {
		return true
	}
	s.pos, s.vel = s.spring.Update(s.pos, s.vel, s.target)
	if math.Abs(s.pos-s.target) < 0.05 && math.Abs(s.vel) < 0.05 {
		s.pos, s.vel = s.target, 0
		return false
	}
	return true
}

// AxlrInfo is the /axlr-info screen: the emblem flies in, the chevrons
// from the left and each letter dropping with a bounce, a light sweeps the
// letters and the tagline types itself; then the versions stay.
type AxlrInfo struct {
	gen      uint64
	frame    int
	chevrons axlrSpring
	letters  [4]axlrSpring
	shine    axlrSpring
	typed    axlrSpring
	// versions are the engines' installed versions once read; loaded
	// tells "not installed" from "still reading".
	versions map[string]string
	loaded   bool
}

func newAxlrInfo(gen uint64, tagline int) AxlrInfo {
	a := AxlrInfo{gen: gen}
	a.chevrons = newAxlrSpring(-28, 0, 0, 7, 0.45)
	for i := range a.letters {
		a.letters[i] = newAxlrSpring(-18, 0, 6+5*i, 8, 0.32)
	}
	a.shine = newAxlrSpring(-12, 80, 48, 4.5, 1)
	a.typed = newAxlrSpring(0, float64(tagline), 30, 4, 1)
	return a
}

// step advances every spring a frame; false once all of them rest.
func (a *AxlrInfo) step() bool {
	a.frame++
	moving := false
	for _, s := range append([]*axlrSpring{&a.chevrons, &a.shine, &a.typed}, &a.letters[0], &a.letters[1], &a.letters[2], &a.letters[3]) {
		if s.step(a.frame) {
			moving = true
		}
	}
	return moving
}

type axlrInfoTick struct{ gen uint64 }

type axlrInfoVersions struct {
	gen      uint64
	versions map[string]string
}

// axlrInfoFrame is how long a frame lasts; tests shorten it.
var axlrInfoFrame = time.Second / axlrInfoFPS

func axlrInfoNext(gen uint64) tea.Cmd {
	return tea.Tick(axlrInfoFrame, func(time.Time) tea.Msg { return axlrInfoTick{gen: gen} })
}

// openAxlrInfo starts the animation and reads the engines' versions; a
// replay keeps those it read, so only /axlr-info sees an /update.
func (m AppModel) openAxlrInfo(replay bool) (AppModel, tea.Cmd) {
	m.Composer.Input.Reset()
	versions, loaded := m.AxlrInfo.versions, m.AxlrInfo.loaded && replay
	m.axlrInfoGen++
	m.AxlrInfo = newAxlrInfo(m.axlrInfoGen, len([]rune(m.Theme.T(axlrInfoTagline))))
	m.AxlrInfo.versions, m.AxlrInfo.loaded = versions, loaded
	m.overlay = "axlr-info"
	cmds := []tea.Cmd{axlrInfoNext(m.axlrInfoGen)}
	if reader, ok := m.deps.EngineUpdates.(application.EngineVersionPort); ok && !loaded {
		ctx, gen := m.deps.Context, m.axlrInfoGen
		if ctx == nil {
			ctx = context.Background()
		}
		cmds = append(cmds, func() tea.Msg {
			read, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			versions, _ := reader.InstalledVersions(read)
			return axlrInfoVersions{gen: gen, versions: versions}
		})
	} else if !ok {
		m.AxlrInfo.loaded = true
	}
	return m, tea.Batch(cmds...)
}

// axlrInfoUpdate handles the screen's own messages; ok is false for others.
func (m AppModel) axlrInfoUpdate(msg tea.Msg) (AppModel, tea.Cmd, bool) {
	switch v := msg.(type) {
	case axlrInfoTick:
		// A tick of an earlier opening, or after Esc, ends there.
		if v.gen != m.AxlrInfo.gen || m.overlay != "axlr-info" {
			return m, nil, true
		}
		if m.AxlrInfo.step() {
			return m, axlrInfoNext(v.gen), true
		}
		return m, nil, true
	case axlrInfoVersions:
		if v.gen == m.AxlrInfo.gen {
			m.AxlrInfo.versions, m.AxlrInfo.loaded = v.versions, true
		}
		return m, nil, true
	}
	return m, nil, false
}

// axlrInfoKey: r plays the animation again, Enter and q close; every other
// key is the screen's, never the composer's.
func (m AppModel) axlrInfoKey(key string) (AppModel, tea.Cmd) {
	switch key {
	case "r":
		return m.openAxlrInfo(true)
	case "enter", "q":
		m.overlay = ""
	}
	return m, nil
}

// axlrInfoView draws the screen centred in width × height.
func (m AppModel) axlrInfoView(width, height int) string {
	a, theme := m.AxlrInfo, m.Theme
	info := []string{
		"",
		theme.Accent(axlrTyped(theme.T(axlrInfoTagline), a.typed.pos)),
		"",
		lipgloss.NewStyle().Bold(true).Render("AXLR v"+strings.TrimPrefix(buildinfo.Version, "v")) + theme.Muted(" · github.com/underpass-ai/AXLR"),
		m.axlrInfoEngines(),
	}
	if model := m.Header.State.Model; model != "" {
		route := theme.T("axlrInfo.openRouter")
		if url, local := m.deps.LocalEndpoints[model]; local {
			route = theme.Tf("axlrInfo.local", url)
		}
		info = append(info, theme.Tf("axlrInfo.model", model, route))
	}
	info = append(info, m.axlrInfoJev())
	info = append(info, "", theme.Muted(theme.T("axlrInfo.hint")))
	logo := m.emblem()
	scale := max(1, min(4, (width-4)/max(1, len(logo[0])), (height-len(info)-1)*2/max(1, len(logo))))
	rows := a.render(logo, scale, theme.Monochrome)
	for i, row := range rows {
		rows[i] = ansi.Truncate(row, width, "")
	}
	block := lipgloss.JoinVertical(lipgloss.Center, strings.Join(rows, "\n"), lipgloss.JoinVertical(lipgloss.Center, info...))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, block)
}

// axlrInfoEngines is the KMP and MADE versions installed.
func (m AppModel) axlrInfoEngines() string {
	theme := m.Theme
	if !m.AxlrInfo.loaded {
		return theme.Muted(theme.T("axlrInfo.reading"))
	}
	var parts []string
	for _, engine := range []string{"kmp", "made"} {
		version := m.AxlrInfo.versions[engine]
		if version == "" {
			version = theme.T("axlrInfo.notInstalled")
		}
		parts = append(parts, strings.ToUpper(engine)+" "+version)
	}
	return strings.Join(parts, theme.Muted(" · "))
}

// axlrInfoJev says whether Jev is on, its model and its methods.
func (m AppModel) axlrInfoJev() string {
	theme, jev := m.Theme, m.deps.Jev
	var methods []string
	if jev.Tool {
		methods = append(methods, theme.T("axlrInfo.jevTool"))
	}
	if jev.FinalCheck {
		methods = append(methods, theme.Tf("axlrInfo.jevFinal", jev.Threshold*100))
	}
	if len(methods) == 0 {
		return theme.Muted(theme.T("axlrInfo.jevOff"))
	}
	if jev.Policy != "" && jev.Policy != "legacy" {
		methods = append(methods, theme.Tf("axlrInfo.jevPolicy", jev.Policy))
		if jev.Supersede {
			methods = append(methods, theme.T("axlrInfo.jevSupersede"))
		}
	}
	return theme.Tf("axlrInfo.jev", jev.Model, strings.Join(methods, " + "))
}

func axlrTyped(text string, pos float64) string {
	runes := []rune(text)
	n := max(0, min(len(runes), int(math.Round(pos))))
	return string(runes[:n]) + strings.Repeat(" ", len(runes)-n)
}

// emblem is the variant drawn for the theme's background.
func (m AppModel) emblem() emblem {
	if themeIsLight(m.Theme) {
		return emblemLight
	}
	return emblemDark
}

func themeIsLight(t Theme) bool {
	r, g, b, ok := hexRGB(t.palette().Background)
	return ok && 0.2126*r+0.7152*g+0.0722*b > 0.5
}

func hexRGB(hex string) (r, g, b float64, ok bool) {
	v, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil || len(hex) != 7 {
		return 0, 0, 0, false
	}
	return float64(v>>16&0xFF) / 255, float64(v>>8&0xFF) / 255, float64(v&0xFF) / 255, true
}

func blendHex(from, to string, t float64) string {
	r1, g1, b1, ok1 := hexRGB(from)
	r2, g2, b2, ok2 := hexRGB(to)
	if !ok1 || !ok2 {
		return from
	}
	mix := func(a, b float64) int { return int(math.Round((a + (b-a)*t) * 255)) }
	return fmt.Sprintf("#%02X%02X%02X", mix(r1, r2), mix(g1, g2), mix(b1, b2))
}

// pixel is the colour of the animated emblem at sub-pixel (x, y), where a
// logo pixel is scale sub-pixels wide and high.
func (a AxlrInfo) pixel(logo emblem, x, y, scale int) string {
	width := len(logo[0])
	col := x / scale
	sx, sy := x, y
	switch {
	case col < emblemMarkEnd:
		sx = x - int(math.Round(a.chevrons.pos*float64(scale)))
		if sx < 0 || sx/scale >= emblemMarkEnd {
			return ""
		}
	default:
		for i, span := range emblemLetters {
			if col >= span[0] && col < span[1] {
				sy = y - int(math.Round(a.letters[i].pos*float64(scale)))
			}
		}
	}
	px, py := sx/scale, sy/scale
	if sy < 0 || py >= len(logo) || px >= width {
		return ""
	}
	colour := logo[py][px]
	if colour == "" || px < emblemMarkEnd {
		return colour
	}
	// The sweep: a slanted band of light crossing the letters.
	if d := math.Abs(float64(px) + float64(py)*0.6 - a.shine.pos); d < 3 {
		return blendHex(colour, axlrInfoShine, (1-d/3)*0.75)
	}
	return colour
}

// render draws the emblem with half blocks, two sub-pixel rows a line,
// each run of cells of the same colours styled once.
func (a AxlrInfo) render(logo emblem, scale int, monochrome bool) []string {
	if len(logo) == 0 {
		return nil
	}
	width, height := len(logo[0])*scale, len(logo)*scale
	rows := make([]string, 0, (height+1)/2)
	for y := 0; y < height; y += 2 {
		var line strings.Builder
		var run strings.Builder
		runFG, runBG := "", ""
		flush := func() {
			if run.Len() == 0 {
				return
			}
			style := lipgloss.NewStyle()
			if !monochrome && runFG != "" {
				style = style.Foreground(lipgloss.Color(runFG))
			}
			if !monochrome && runBG != "" {
				style = style.Background(lipgloss.Color(runBG))
			}
			line.WriteString(style.Render(run.String()))
			run.Reset()
		}
		for x := 0; x < width; x++ {
			top := a.pixel(logo, x, y, scale)
			bottom := ""
			if y+1 < height {
				bottom = a.pixel(logo, x, y+1, scale)
			}
			glyph, fg, bg := " ", "", ""
			switch {
			case top != "" && top == bottom:
				glyph, fg = "█", top
			case top != "" && bottom != "":
				glyph, fg, bg = "▀", top, bottom
			case top != "":
				glyph, fg = "▀", top
			case bottom != "":
				glyph, fg = "▄", bottom
			}
			if fg != runFG || bg != runBG {
				flush()
				runFG, runBG = fg, bg
			}
			run.WriteString(glyph)
		}
		flush()
		rows = append(rows, line.String())
	}
	return rows
}
