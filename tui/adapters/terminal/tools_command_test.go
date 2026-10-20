package terminal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// forgeFake is an in-memory ForgedToolsPort for the console's tests.
type forgeFake struct{ tools []application.ForgedTool }

func (f *forgeFake) List(context.Context, string) ([]application.ForgedTool, error) {
	return f.tools, nil
}
func (f *forgeFake) Forge(_ context.Context, _ string, tool application.ForgedTool, files []application.ForgedFile) (application.ForgedTool, bool, error) {
	tool.Files = map[string]string{}
	for _, file := range files {
		tool.Files[file.Path] = "digest"
	}
	for i, existing := range f.tools {
		if existing.Name == tool.Name {
			f.tools[i] = tool
			return tool, true, nil
		}
	}
	f.tools = append(f.tools, tool)
	return tool, false, nil
}
func (f *forgeFake) Verify(context.Context, string, application.ForgedTool) error { return nil }
func (f *forgeFake) Remove(_ context.Context, _ string, name string) (bool, error) {
	for i, existing := range f.tools {
		if existing.Name == name {
			f.tools = append(f.tools[:i], f.tools[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

// catalogFake is an in-memory CatalogPort with one catalog and the tools
// given, each one file; fail makes Stage fail.
type catalogFake struct {
	index application.CatalogIndex
	files map[string]application.ForgedFile
	fail  error
	// api is the catalog's registry URL, for the publish tests.
	api string
}

func newCatalogFake(names ...string) *catalogFake {
	f := &catalogFake{index: application.CatalogIndex{SchemaVersion: 1, Catalog: "axlr", GeneratedAt: "2026-10-16T19:09:07Z"}, files: map[string]application.ForgedFile{}}
	for _, name := range names {
		file := application.ForgedFile{Path: name + ".py", Content: "import json,sys\nprint(json.dumps({'ok':True}))\n"}
		sum := sha256.Sum256([]byte(file.Content))
		f.index.Tools = append(f.index.Tools, application.CatalogEntry{Name: name, Version: "1.0.0", Category: "administration", Tags: []string{"dates"}, Summary: "Count the working days.", Program: "python3", License: "MIT", Manifest: "tools/administration/" + name + "/manifest.json", Files: map[string]string{file.Path: hex.EncodeToString(sum[:])}})
		f.files[name] = file
	}
	return f
}

func (f *catalogFake) Catalogs(context.Context, bool) ([]application.CatalogState, error) {
	index := f.index
	return []application.CatalogState{{Name: "axlr", URL: "https://catalog.example/index.json", APIURL: f.api, Index: &index}}, nil
}
func (f *catalogFake) Cached(ctx context.Context) ([]application.CatalogState, error) {
	return f.Catalogs(ctx, false)
}
func (f *catalogFake) Stage(_ context.Context, catalog, name, version string) (application.StagedTool, error) {
	if f.fail != nil {
		return application.StagedTool{}, f.fail
	}
	file, ok := f.files[name]
	if !ok {
		return application.StagedTool{}, errors.New("no such tool")
	}
	var entry application.CatalogEntry
	for _, tool := range f.index.Tools {
		if tool.Name == name {
			entry = tool
		}
	}
	return application.StagedTool{Catalog: catalog, Entry: entry, IndexGeneratedAt: f.index.GeneratedAt, Tool: application.ForgedTool{Name: name, Description: "Count the working days.", InputSchema: json.RawMessage(`{"type":"object"}`), Program: "python3", Args: []string{file.Path}}, Files: []application.ForgedFile{file}}, nil
}

func toolsModel(t *testing.T, forge *forgeFake, catalog *catalogFake) AppModel {
	t.Helper()
	s := navSession(t)
	m := navModel(t, &s)
	m.deps.ToolCatalog = application.CatalogUseCase{Catalog: catalog, Forge: forge}
	return update(m, tea.WindowSizeMsg{Width: 100, Height: 40})
}

func runTools(t *testing.T, m AppModel, command string) AppModel {
	t.Helper()
	m.Composer.Input.SetValue(command)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return drain(t, next.(AppModel), cmd)
}

// /tools lists the workspace's tools with their origin and the catalogs'
// tools; /tools search narrows the catalogs'.
func TestToolsPanelListsInstalledAndCatalogTools(t *testing.T) {
	forge := &forgeFake{tools: []application.ForgedTool{{Name: "word_count", Description: "Count the words.", Origin: nil}, {Name: "seat_allocation", Description: "Allocate seats.", Origin: &application.ToolOrigin{Catalog: "axlr", Version: "0.9.0"}}}}
	catalog := newCatalogFake("business_days", "seat_allocation")
	m := toolsModel(t, forge, catalog)
	defer m.zones.Close()
	m = runTools(t, m, "/tools")
	if m.overlay != "tools" {
		t.Fatalf("overlay = %q", m.overlay)
	}
	text := m.Info.Text()
	for _, want := range []string{"word_count · forged here", "seat_allocation · catalog axlr · 0.9.0", "seat_allocation · 0.9.0 → 1.0.0 · axlr", "Catalog axlr", "business_days 1.0.0 · administration", "/tools install <name>"} {
		if !strings.Contains(text, want) {
			t.Fatalf("panel lacks %q:\n%s", want, text)
		}
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "Tools") {
		t.Fatal("the panel has no title")
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = runTools(t, m, "/herramientas search dates")
	if text := m.Info.Text(); !strings.Contains(text, "business_days 1.0.0 · axlr") || !strings.Contains(text, "seat_allocation 1.0.0 · axlr · administration · installed 0.9.0") {
		t.Fatalf("search:\n%s", text)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = runTools(t, m, "/tools remove word_count")
	if len(forge.tools) != 1 || !strings.Contains(m.Status.Notice, "removed word_count") {
		t.Fatalf("remove: %+v, %q", forge.tools, m.Status.Notice)
	}
	m = runTools(t, m, "/tools remove nothing_here")
	if m.Status.Error == "" {
		t.Fatal("removing an unknown tool said nothing")
	}
}

// /tools install shows the install card and writes the tool on y; another
// key keeps the workspace as it is. /tools update installs each newer tool
// on its own card.
func TestToolsInstallAsksOnACardThenRegistersTheTool(t *testing.T) {
	forge, catalog := &forgeFake{}, newCatalogFake("business_days", "seat_allocation")
	m := toolsModel(t, forge, catalog)
	defer m.zones.Close()
	m = runTools(t, m, "/tools install business_days@1.0.0 --from axlr")
	if !m.toolsAsking || m.toolsStaged == nil {
		t.Fatalf("no question asked: %q %q", m.Status.Notice, m.Status.Error)
	}
	card := m.Info.Text()
	for _, want := range []string{"catalog axlr · business_days 1.0.0 · verified", "license MIT", "$ python3 .axlr/tools/business_days/business_days.py", "── business_days.py ──", "import json,sys"} {
		if !strings.Contains(card, want) {
			t.Fatalf("card lacks %q:\n%s", want, card)
		}
	}
	if !strings.Contains(m.Status.Notice, "y installs") {
		t.Fatalf("notice = %q", m.Status.Notice)
	}
	// The card is read before it is answered: PgDn scrolls, it does not
	// cancel.
	m = update(m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	if !m.toolsAsking || m.toolsStaged == nil {
		t.Fatal("PgDn answered the install question")
	}
	m = update(m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if len(forge.tools) != 0 || m.toolsAsking || m.overlay != "" || !strings.Contains(m.Status.Notice, "Nothing installed") {
		t.Fatalf("n installed: %+v, %q", forge.tools, m.Status.Notice)
	}
	// The panel closed by its close button forgets the question too, so
	// the next character typed is not its answer.
	m = runTools(t, m, "/tools install business_days")
	next, cmd := m.Update(ControlIntent("close"))
	m = next.(AppModel)
	if cmd != nil || m.overlay != "" || m.toolsAsking || m.toolsStaged != nil {
		t.Fatalf("close kept the question: overlay %q, asking %v", m.overlay, m.toolsAsking)
	}
	m = update(m, tea.KeyPressMsg{Code: 'y', Text: "y"})
	if len(forge.tools) != 0 || m.Composer.Input.Value() != "y" {
		t.Fatalf("y after close installed: %+v, composer %q", forge.tools, m.Composer.Input.Value())
	}
	m.Composer.Input.Reset()
	m = runTools(t, m, "/tools install business_days")
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = drain(t, next.(AppModel), cmd)
	if len(forge.tools) != 1 || forge.tools[0].Origin == nil || forge.tools[0].Origin.Version != "1.0.0" || forge.tools[0].Args[0] != ".axlr/tools/business_days/business_days.py" {
		t.Fatalf("installed %+v", forge.tools)
	}
	if !strings.Contains(m.Status.Notice, "installed business_days 1.0.0 from catalog axlr") {
		t.Fatalf("notice = %q", m.Status.Notice)
	}
	// Two installed tools are behind the catalog: update stages the first,
	// and after its y the second.
	forge.tools[0].Origin.Version = "0.9.0"
	forge.tools = append(forge.tools, application.ForgedTool{Name: "seat_allocation", Origin: &application.ToolOrigin{Catalog: "axlr", Version: "0.1.0"}})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = runTools(t, m, "/tools update")
	if !m.toolsAsking || m.toolsStaged.Entry.Name != "business_days" || len(m.toolsQueue) != 1 {
		t.Fatalf("update staged %+v, queue %v", m.toolsStaged, m.toolsQueue)
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = drain(t, next.(AppModel), cmd)
	if !m.toolsAsking || m.toolsStaged.Entry.Name != "seat_allocation" || forge.tools[0].Origin.Version != "1.0.0" {
		t.Fatalf("second update not staged: %+v, %+v", m.toolsStaged, forge.tools)
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = drain(t, next.(AppModel), cmd)
	if forge.tools[1].Origin.Version != "1.0.0" || m.toolsAsking {
		t.Fatalf("second update not installed: %+v", forge.tools)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = runTools(t, m, "/tools update")
	if !strings.Contains(m.Info.Text(), "newest version") {
		t.Fatalf("nothing to update:\n%s", m.Info.Text())
	}
	// A failed staging says so in the panel and installs nothing.
	catalog.fail = errors.New("catalog axlr: file business_days.py does not match the digest")
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = runTools(t, m, "/tools install business_days")
	if m.toolsAsking || !strings.Contains(m.Info.Text(), "does not match the digest") {
		t.Fatalf("failed staging: asking %v, panel %q", m.toolsAsking, m.Info.Text())
	}
}

// The model's axlr_install_tool call is staged for its card: until the
// files are on the card nothing approves it; once they are, the card is
// the forge card with the origin line.
func TestTheInstallCardShowsTheFilesBeforeApproval(t *testing.T) {
	forge, catalog := &forgeFake{}, newCatalogFake("business_days")
	s := navSession(t)
	args, _ := root.NewJSONValue([]byte(`{"name":"business_days"}`))
	schema, _ := root.NewJSONValue([]byte(`{"type":"object"}`))
	s.BeginTurn("install", []domain.AvailableTool{{Definition: root.ToolDefinition{Name: application.HostInstallToolName, Description: "install", Parameters: schema}, Identity: domain.ToolIdentity{Kind: domain.ToolKindHost, LocalOperation: domain.HostOperationInstallTool}}})
	if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "one", Name: application.HostInstallToolName, Arguments: args}}}}); err != nil {
		t.Fatal(err)
	}
	m := update(New(Dependencies{Session: &s, Monochrome: true, ToolCatalog: application.CatalogUseCase{Catalog: catalog, Forge: forge}}), tea.WindowSizeMsg{Width: 100, Height: 40})
	defer m.zones.Close()
	if !strings.Contains(m.Approval.Target, "install catalog tool business_days") || !strings.Contains(m.Approval.Details.Text(), "Fetching business_days") || m.installCardReady() {
		t.Fatalf("card before staging: %q\n%s", m.Approval.Target, m.Approval.Details.Text())
	}
	m = update(m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	if m.Busy || !strings.Contains(m.Status.Notice, "still fetching") {
		t.Fatalf("approved before the files were shown: busy %v, %q", m.Busy, m.Status.Notice)
	}
	stage := m.Init()
	if stage == nil {
		t.Fatal("nothing stages the pending install")
	}
	var staged installStagedMsg
	found := false
	var walk func(msg tea.Msg)
	walk = func(msg tea.Msg) {
		switch v := msg.(type) {
		case tea.BatchMsg:
			for _, cmd := range v {
				if cmd != nil {
					walk(cmd())
				}
			}
		case installStagedMsg:
			staged, found = v, true
		}
	}
	walk(stage())
	if !found {
		t.Fatal("Init did not stage the install")
	}
	m = update(m, staged)
	card := m.Approval.Details.Text()
	for _, want := range []string{"catalog axlr · business_days 1.0.0 · verified", "$ python3 .axlr/tools/business_days/business_days.py", "── business_days.py ──", "import json,sys"} {
		if !strings.Contains(card, want) {
			t.Fatalf("card lacks %q:\n%s", want, card)
		}
	}
	if !m.installCardReady() || strings.Contains(ansi.Strip(m.View().Content), "l always") {
		t.Fatal("the card is not ready, or offers always-allow")
	}
	// A failed staging is on the card too, and still refuses approval.
	catalog.fail = errors.New("catalog axlr: the manifest does not match")
	m.installStagedCall, m.installStaged = "", nil
	m = update(m, func() tea.Msg { return m.stageInstallCard()() }())
	if m.installCardReady() || !strings.Contains(m.Approval.Details.Text(), "could not be staged") {
		t.Fatalf("failed staging: %s", m.Approval.Details.Text())
	}
}

// A on the install card installs the tool the card showed. On 20 October
// 2026 a review found that the approval staged the call again from the
// shared cache, which another console may refresh in between: here the
// index now has 1.1.0 instead of the 1.0.0 on the card, and nothing is
// installed rather than a version the person never saw.
func TestApprovingTheInstallCardInstallsWhatItShowed(t *testing.T) {
	forge, catalog := &forgeFake{}, newCatalogFake("business_days")
	s := navSession(t)
	args, _ := root.NewJSONValue([]byte(`{"name":"business_days"}`))
	schema, _ := root.NewJSONValue([]byte(`{"type":"object"}`))
	s.BeginTurn("install", []domain.AvailableTool{{Definition: root.ToolDefinition{Name: application.HostInstallToolName, Description: "install", Parameters: schema}, Identity: domain.ToolIdentity{Kind: domain.ToolKindHost, LocalOperation: domain.HostOperationInstallTool}}})
	if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "one", Name: application.HostInstallToolName, Arguments: args}}}}); err != nil {
		t.Fatal(err)
	}
	m := navModel(t, &s)
	defer m.zones.Close()
	m.deps.ToolCatalog = application.CatalogUseCase{Catalog: catalog, Forge: forge}
	m.deps.Resolve.Store = m.deps.Store
	m.deps.Resolve.Continue.Forge, m.deps.Resolve.Continue.Catalog = forge, catalog
	m.deps.Resolve.Continue.Models = navStream{}
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m.installStagedCall, m.installStaged = "", nil
	stage := m.stageInstallCard()
	m = update(m, stage())
	if call, card, shown := m.shownInstall(); !shown || call != "one" || card.Key() != "axlr/business_days@1.0.0" {
		t.Fatalf("card = %v %s %v", shown, card.Key(), call)
	}
	catalog.index.Tools[0].Version = "1.1.0"
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = drain(t, next.(AppModel), cmd)
	activity := m.deps.Session.Export().Activity
	if len(activity) != 1 || activity[0].Outcome == nil || !activity[0].Outcome.IsError || !strings.Contains(string(activity[0].Outcome.Content), "the card showed axlr/business_days@1.0.0, which cannot be installed now") || len(forge.tools) != 0 {
		t.Fatalf("error %q, activity %+v, installed %+v", m.Status.Error, activity, forge.tools)
	}
}
