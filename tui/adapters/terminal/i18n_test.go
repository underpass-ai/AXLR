package terminal

import (
	"github.com/charmbracelet/x/ansi"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestCatalogsHaveTheSameLabeledMessages(t *testing.T) {
	if len(enMessages) != len(esMessages) {
		t.Fatalf("catalog size: en=%d es=%d", len(enMessages), len(esMessages))
	}
	verbs := regexp.MustCompile(`%[-+#0-9.]*[a-zA-Z]`)
	for label, english := range enMessages {
		spanish, ok := esMessages[label]
		if !ok || english == "" || spanish == "" {
			t.Fatalf("missing translation for %s", label)
		}
		if strings.Join(verbs.FindAllString(english, -1), ",") != strings.Join(verbs.FindAllString(spanish, -1), ",") {
			t.Fatalf("format placeholders differ for %s", label)
		}
	}
	for label := range esMessages {
		if _, ok := enMessages[label]; !ok {
			t.Fatalf("Spanish-only label %s", label)
		}
	}
	for _, item := range actionItems {
		a := item.(actionItem)
		for _, label := range []string{a.title, a.description} {
			if _, ok := enMessages[label]; !ok {
				t.Fatalf("palette uses missing label %s", label)
			}
		}
	}
}

func TestLiteralTranslationLabelsExist(t *testing.T) {
	fset := token.NewFileSet()
	packages, err := parser.ParseDir(fset, ".", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for name, file := range packages["terminal"].Files {
		if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "i18n.go") {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := ""
			switch fn := call.Fun.(type) {
			case *ast.SelectorExpr:
				name = fn.Sel.Name
			case *ast.Ident:
				name = fn.Name
			}
			index := 0
			switch name {
			case "T", "Tf":
			case "Translate", "Translatef":
				index = 1
			default:
				return true
			}
			if len(call.Args) <= index {
				return true
			}
			literal, ok := call.Args[index].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			label, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Error(err)
				return true
			}
			if _, ok := enMessages[label]; !ok {
				t.Errorf("%s uses missing label %q", name, label)
			}
			return true
		})
	}
}

func TestLocaleSelectionAndVisibleCopy(t *testing.T) {
	for _, test := range []struct {
		input string
		want  Locale
	}{
		{"", English}, {"en_US.UTF-8", English}, {"es_ES.UTF-8", Spanish}, {"es-CO", Spanish},
	} {
		locale, err := ParseLocale(test.input)
		if err != nil || locale != test.want {
			t.Fatalf("ParseLocale(%q) = %q, %v", test.input, locale, err)
		}
	}
	if _, err := ParseLocale("fr"); err == nil {
		t.Fatal("unsupported language was accepted")
	}
	for _, test := range []struct {
		locale Locale
		want   string
	}{
		{English, "Actions"}, {Spanish, "Acciones"},
	} {
		m := New(Dependencies{Locale: test.locale, Monochrome: true})
		m = update(m, tea.WindowSizeMsg{Width: 70, Height: 20})
		m = update(m, ControlIntent("palette"))
		view := m.View().Content
		if !strings.Contains(view, test.want) || strings.Contains(view, "palette.") {
			t.Fatalf("%s palette copy: %q", test.locale, view)
		}
		if test.locale == Spanish && !strings.Contains(view, "Sesiones") {
			t.Fatalf("Spanish palette items were not translated: %q", view)
		}
		m = update(m, ControlIntent("help"))
		if !strings.Contains(m.View().Content, Translate(test.locale, "help.subtitle")) {
			t.Fatalf("%s help is not localized", test.locale)
		}
		m.zones.Close()
	}
	m := New(Dependencies{Monochrome: true})
	if m.Theme.Locale != English {
		t.Fatalf("default locale is %q", m.Theme.Locale)
	}
	m.zones.Close()
}

func TestSpanishSurfacesAndStoredContent(t *testing.T) {
	m := New(Dependencies{Locale: Spanish, Monochrome: true})
	defer m.zones.Close()
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if m.Composer.Input.Placeholder != "Escribe un mensaje…" || !strings.Contains(m.View().Content, "enter enviar") || !strings.Contains(m.View().Content, "inactivo") {
		t.Fatal("main screen is not localized")
	}
	m = update(m, ControlIntent("info"))
	if !strings.Contains(m.View().Content, "Información y resultados") || !strings.Contains(m.Info.Viewport.GetContent(), "Todavía no hay herramientas ejecutadas") {
		t.Fatal("info screen is not localized")
	}
	m.overlay = "sessions"
	if !strings.Contains(m.View().Content, "No hay sesiones guardadas") {
		t.Fatal("sessions screen is not localized")
	}
	m.ThemePicker = NewThemePicker(domain.DefaultUIPreferences(), Spanish)
	m.overlay = "theme"
	if !strings.Contains(m.View().Content, "Vista previa · Automático") || !strings.Contains(m.View().Content, "Sigue el fondo del terminal") {
		t.Fatal("theme picker is not localized")
	}
	if got := m.Composer.Input.KeyMap.InsertNewline.Help().Desc; got != "insertar nueva línea" {
		t.Fatalf("composer binding help is not localized: %q", got)
	}
	m.Models.Theme = m.Theme
	m.Models.SetLoading(true)
	m.overlay = "models"
	if !strings.Contains(m.View().Content, "Cargando modelos") {
		t.Fatal("model picker is not localized")
	}
	m.Plugins = NewPluginPanel()
	m.Plugins.Theme = m.Theme
	m.Plugins.SetItems([]domain.PluginState{pluginItem("kmp")})
	m.overlay = "mcp"
	if !strings.Contains(m.View().Content, "servidores y herramientas") || !strings.Contains(m.Plugins.Details.GetContent(), "Herramientas permitidas") {
		t.Fatalf("plugin panel is not localized: view=%q details=%q", m.View().Content, m.Plugins.Details.GetContent())
	}
	state := domain.SessionState{Messages: []root.Message{{Role: root.RoleUser, Content: "Exact user text · no translation"}}}
	tr := NewTranscript()
	tr.Viewport.SetWidth(80)
	tr.Viewport.SetHeight(8)
	tr.SetSession(state, "", m.Theme)
	if !strings.Contains(ansi.Strip(tr.Viewport.GetContent()), m.Theme.Icon("user")+" Exact user text · no translation") {
		t.Fatalf("transcript labels or original user content changed: %q", tr.Viewport.GetContent())
	}
}

func TestLocalizedListFiltering(t *testing.T) {
	for _, test := range []struct {
		locale  Locale
		prompt  string
		actions string
		themes  string
	}{
		{English, "Search: ", "No matching actions", "No matching themes"},
		{Spanish, "Buscar: ", "No hay acciones coincidentes", "No hay temas coincidentes"},
	} {
		theme := Theme{ID: domain.ThemeAuto, Icons: domain.IconsSafe, Locale: test.locale, Monochrome: true}
		zones := zone.New()
		palette := NewActionPalette(test.locale)
		if palette.List.FilterInput.Prompt != test.prompt {
			t.Fatalf("%s action filter prompt: %q", test.locale, palette.List.FilterInput.Prompt)
		}
		palette.List.SetFilterText("zzzz-never-matches")
		view := palette.View(theme, zones, "", 80, 20)
		if !strings.Contains(view, test.actions) || strings.Contains(view, "No items.") {
			t.Fatalf("%s action empty state: %q", test.locale, view)
		}
		picker := NewThemePicker(domain.DefaultUIPreferences(), test.locale)
		if picker.List.FilterInput.Prompt != test.prompt {
			t.Fatalf("%s theme filter prompt: %q", test.locale, picker.List.FilterInput.Prompt)
		}
		picker.List.SetFilterText("zzzz-never-matches")
		view = picker.View(theme, 80, 20)
		if !strings.Contains(view, test.themes) || strings.Contains(view, "No items.") {
			t.Fatalf("%s theme empty state: %q", test.locale, view)
		}
		if test.locale == Spanish {
			picker.List.SetFilterText("papel")
			if len(picker.List.VisibleItems()) != 1 {
				t.Fatal("Spanish theme names cannot be searched")
			}
		}
		zones.Close()
	}
}
