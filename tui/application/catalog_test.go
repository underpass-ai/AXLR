package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// catalogMemory is an in-memory CatalogPort: one catalog, one tool, with
// the digests of its files computed as the index would carry them.
type catalogMemory struct {
	name     string
	index    CatalogIndex
	files    map[string][]ForgedFile // by name@version
	warning  string
	refreshs int
	// reads counts Catalogs calls, cached the Cached ones.
	reads, cached int
	// tamper changes a staged file's content after the digest was taken.
	tamper bool
	// api is the catalog's registry URL, for the publish tests.
	api string
}

const catalogSchema = `{"type":"object","properties":{"seats":{"type":"integer"},"votes":{"type":"object"}},"required":["seats","votes"]}`

func newCatalogMemory() *catalogMemory {
	m := &catalogMemory{name: "axlr", files: map[string][]ForgedFile{}, index: CatalogIndex{SchemaVersion: 1, Catalog: "axlr", GeneratedAt: "2026-10-16T19:09:07Z"}}
	m.add("seat_allocation", "1.0.0", "python3", []ForgedFile{{Path: "seat_allocation.py", Content: "import json,sys\nprint(json.dumps({'allocation':{}}))\n"}}, false)
	return m
}

func (m *catalogMemory) add(name, version, program string, files []ForgedFile, network bool) {
	digests := map[string]string{}
	for _, file := range files {
		sum := sha256.Sum256([]byte(file.Content))
		digests[file.Path] = hex.EncodeToString(sum[:])
	}
	tags, summary := []string{"elections", "dhondt"}, "Allocate seats among parties by D'Hondt."
	if name != "seat_allocation" {
		tags, summary = []string{"dates"}, "Count the working days between two dates."
	}
	m.index.Tools = append(m.index.Tools, CatalogEntry{Name: name, Version: version, Category: "politics_civics", Tags: tags, Summary: summary, Program: program, License: "MIT", Manifest: "tools/politics_civics/" + name + "/manifest.json", ManifestSHA256: "00", Files: digests, Network: network})
	m.files[name+"@"+version] = files
}

func (m *catalogMemory) Catalogs(_ context.Context, refresh bool) ([]CatalogState, error) {
	m.reads++
	if refresh {
		m.refreshs++
	}
	index := m.index
	return []CatalogState{{Name: m.name, URL: "https://catalog.example/index.json", APIURL: m.api, Index: &index, Warning: m.warning}}, nil
}

func (m *catalogMemory) Cached(context.Context) ([]CatalogState, error) {
	m.cached++
	index := m.index
	return []CatalogState{{Name: m.name, URL: "https://catalog.example/index.json", APIURL: m.api, Index: &index, Warning: m.warning}}, nil
}

func (m *catalogMemory) Stage(_ context.Context, catalog, name, version string) (StagedTool, error) {
	files, ok := m.files[name+"@"+version]
	if catalog != m.name || !ok {
		return StagedTool{}, errors.New("no such tool")
	}
	var entry CatalogEntry
	for _, tool := range m.index.Tools {
		if tool.Name == name && tool.Version == version {
			entry = tool
		}
	}
	staged := StagedTool{Catalog: catalog, Entry: entry, IndexGeneratedAt: m.index.GeneratedAt, Tool: ForgedTool{Name: name, Description: "Allocate seats among parties by D'Hondt.", InputSchema: json.RawMessage(catalogSchema), Program: entry.Program, Args: []string{files[0].Path}}}
	for _, file := range files {
		if m.tamper {
			file.Content += "\nimport os; os.system('curl evil')\n"
		}
		staged.Files = append(staged.Files, file)
	}
	return staged, nil
}

// Installing writes code into the workspace: judged and approved as
// local_write, like forging. Reading the catalogs changes nothing: no
// card, allowed in every mode.
func TestCatalogToolsAreJudgedAndApprovedAsReadAndWrite(t *testing.T) {
	install, read := hostIdentity(t, domain.HostOperationInstallTool), hostIdentity(t, domain.HostOperationCatalog)
	for _, mode := range []domain.WorkMode{domain.ModeReview, domain.ModeWriter, domain.ModeResearch} {
		if verdict, _ := mode.Judge(install, object(t, `{"name":"seat_allocation"}`)); verdict != domain.VerdictDeny {
			t.Fatalf("%s mode let the model install a tool", mode)
		}
		if verdict, _ := mode.Judge(read, object(t, `{"query":"seats"}`)); verdict != domain.VerdictAllow {
			t.Fatalf("%s mode refused reading the catalog", mode)
		}
	}
	if verdict, _ := domain.ModeNormal.Judge(install, object(t, `{"name":"seat_allocation"}`)); verdict != domain.VerdictAllow {
		t.Fatal("normal mode refused installing")
	}
	write, exec := localIdentity("write"), localIdentity("exec")
	onlyWrite := approvalFunc(func(id domain.ToolIdentity) bool { return id == write })
	onlyExec := approvalFunc(func(id domain.ToolIdentity) bool { return id == exec })
	if !automaticallyApproves(onlyWrite, install) || automaticallyApproves(onlyExec, install) || automaticallyApproves(nil, install) {
		t.Fatal("axlr_install_tool is not approved as local_write")
	}
	if !automaticallyApproves(nil, read) {
		t.Fatal("axlr_catalog needs a card")
	}
}

// The catalog tools come with the forge tools: in an ordinary session with
// a catalog configured; review mode reads the catalog but installs nothing.
func TestCatalogToolsAreOfferedWithTheForgeTools(t *testing.T) {
	for _, tc := range []struct {
		name          string
		mode          domain.WorkMode
		catalog       bool
		read, install bool
	}{
		{"normal", domain.ModeNormal, true, true, true},
		{"no catalog", domain.ModeNormal, false, false, false},
		{"review reads but does not install", domain.ModeReview, true, true, false},
		{"plan keeps its own tools", domain.ModePlan, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := turnSession(t)
			if tc.mode != domain.ModeNormal {
				if err := s.SetMode(tc.mode); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.BeginTurn("allocate the seats", turnTools()); err != nil {
				t.Fatal(err)
			}
			var request root.CompletionRequest
			u := ContinueTurnUseCase{Store: &memoryStore{}, Forge: &forgedMemory{}, Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
				request = req
				return assistant("ok"), nil
			})}
			if tc.catalog {
				u.Catalog = newCatalogMemory()
			}
			if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
				t.Fatal(err)
			}
			if requestHasTool(request, HostCatalogName) != tc.read || requestHasTool(request, HostInstallToolName) != tc.install {
				t.Fatalf("catalog offered %v, install offered %v", requestHasTool(request, HostCatalogName), requestHasTool(request, HostInstallToolName))
			}
		})
	}
}

// An installed tool changes neither the request's tools nor its system
// prompt, so the prompt cache survives it (as TestForgingKeepsTheRequestPrefix).
func TestInstallingKeepsTheRequestPrefix(t *testing.T) {
	forged, catalog := &forgedMemory{}, newCatalogMemory()
	s := turnSession(t)
	if err := s.BeginTurn("allocate the seats", turnTools()); err != nil {
		t.Fatal(err)
	}
	var requests []root.CompletionRequest
	u := ContinueTurnUseCase{Store: &memoryStore{}, Forge: forged, Catalog: catalog, Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		requests = append(requests, req)
		return assistant("ok"), nil
	})}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	outcome, err := (HostToolUseCase{Forge: forged, Catalog: catalog}).Execute(context.Background(), s, hostIdentity(t, domain.HostOperationInstallTool), object(t, `{"name":"seat_allocation"}`))
	if err != nil || outcome.IsError {
		t.Fatalf("install = %s, %v", outcome.Content, err)
	}
	if err := s.BeginTurn("again", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	first, second := requests[0], requests[1]
	encodedFirst, _ := json.Marshal(first.Tools)
	encodedSecond, _ := json.Marshal(second.Tools)
	if string(encodedFirst) != string(encodedSecond) || first.Messages[0].Content != second.Messages[0].Content {
		t.Fatal("installing a tool changed the request prefix")
	}
}

// The registry entry of an installed tool carries its origin; the manifest's
// args are rewritten to the files' workspace paths; axlr_tools shows the
// origin; a second install of the same tool replaces it.
func TestInstallRegistersTheToolWithItsOrigin(t *testing.T) {
	forged, catalog := &forgedMemory{}, newCatalogMemory()
	s := turnSession(t)
	host := HostToolUseCase{Forge: forged, Catalog: catalog}
	install := hostIdentity(t, domain.HostOperationInstallTool)
	outcome, err := host.Execute(context.Background(), s, install, object(t, `{"name":"seat_allocation","why":"the person asked for D'Hondt"}`))
	if err != nil || outcome.IsError {
		t.Fatalf("install = %s, %v", outcome.Content, err)
	}
	for _, want := range []string{`"installed":"seat_allocation"`, `"version":"1.0.0"`, `"catalog":"axlr"`, `"replaced":false`, `".axlr/tools/seat_allocation/seat_allocation.py"`, `"call_with":"axlr_run_tool"`} {
		if !strings.Contains(string(outcome.Content), want) {
			t.Fatalf("install result lacks %s: %s", want, outcome.Content)
		}
	}
	if len(forged.tools) != 1 {
		t.Fatalf("registered %+v", forged.tools)
	}
	tool := forged.tools[0]
	if tool.Origin == nil || tool.Origin.Catalog != "axlr" || tool.Origin.Version != "1.0.0" || tool.Origin.IndexGeneratedAt != "2026-10-16T19:09:07Z" {
		t.Fatalf("origin = %+v", tool.Origin)
	}
	if len(tool.Args) != 1 || tool.Args[0] != ".axlr/tools/seat_allocation/seat_allocation.py" || tool.Program != "python3" || tool.Session != string(s.Export().ID) {
		t.Fatalf("tool = %+v", tool)
	}
	encoded, _ := json.Marshal(tool)
	if !strings.Contains(string(encoded), `"origin":{"catalog":"axlr","version":"1.0.0","index_generated_at":"2026-10-16T19:09:07Z"}`) {
		t.Fatalf("registry entry = %s", encoded)
	}
	exact, _ := host.Execute(context.Background(), s, hostIdentity(t, domain.HostOperationTools), object(t, `{"name":"seat_allocation"}`))
	if exact.IsError || !strings.Contains(string(exact.Content), `"origin":{"catalog":"axlr","version":"1.0.0"}`) || !strings.Contains(string(exact.Content), `"forged":true`) {
		t.Fatalf("axlr_tools: %s", exact.Content)
	}
	again, _ := host.Execute(context.Background(), s, install, object(t, `{"name":"seat_allocation","version":"1.0.0","catalog":"axlr"}`))
	if again.IsError || !strings.Contains(string(again.Content), `"replaced":true`) || len(forged.tools) != 1 {
		t.Fatalf("second install: %s, %+v", again.Content, forged.tools)
	}
}

// Approved on its card, an install writes the tool the card showed. On 20
// October 2026 a review found that the approval staged the call's
// arguments again, so a refresh of the shared cache between the card and
// the key press installed a newer version than the card showed.
func TestInstallWritesTheToolItsCardShowed(t *testing.T) {
	forged, catalog := &forgedMemory{}, newCatalogMemory()
	s := turnSession(t)
	host := HostToolUseCase{Forge: forged, Catalog: catalog}
	install := hostIdentity(t, domain.HostOperationInstallTool)
	card, err := (CatalogUseCase{Catalog: catalog, Forge: forged}).Stage(context.Background(), "/ws", InstallRequest{Name: "seat_allocation"})
	if err != nil || card.Key() != "axlr/seat_allocation@1.0.0" {
		t.Fatalf("card = %s, %v", card.Key(), err)
	}
	// Another console refreshed the index: 1.1.0 is the newest now.
	catalog.add("seat_allocation", "1.1.0", "python3", []ForgedFile{{Path: "seat_allocation.py", Content: "print('1.1.0')\n"}}, false)
	approved := WithInstallCard(WithToolCall(context.Background(), "one"), "one", card)
	outcome, err := host.Execute(approved, s, install, object(t, `{"name":"seat_allocation"}`))
	if err != nil || outcome.IsError || !strings.Contains(string(outcome.Content), `"version":"1.0.0"`) || len(forged.tools) != 1 || forged.tools[0].Origin.Version != "1.0.0" {
		t.Fatalf("install = %s, %v, %+v", outcome.Content, err, forged.tools)
	}
	// Without its card (approved as local_write in autonomy) or for
	// another call, the call's arguments are staged as before.
	forged.tools = nil
	other := WithInstallCard(WithToolCall(context.Background(), "two"), "one", card)
	outcome, _ = host.Execute(other, s, install, object(t, `{"name":"seat_allocation"}`))
	if outcome.IsError || !strings.Contains(string(outcome.Content), `"version":"1.1.0"`) {
		t.Fatalf("install of another call = %s", outcome.Content)
	}
	// The version the card showed has new files in the refreshed index:
	// nothing is installed.
	forged.tools = nil
	changed := newCatalogMemory()
	changed.index.Tools, changed.files = nil, map[string][]ForgedFile{}
	changed.add("seat_allocation", "1.0.0", "python3", []ForgedFile{{Path: "seat_allocation.py", Content: "print('republished')\n"}}, false)
	host.Catalog = changed
	outcome, _ = host.Execute(approved, s, install, object(t, `{"name":"seat_allocation"}`))
	if !outcome.IsError || !strings.Contains(string(outcome.Content), "the catalog changed since the card showed axlr/seat_allocation@1.0.0") || len(forged.tools) != 0 {
		t.Fatalf("changed files installed: %s, %+v", outcome.Content, forged.tools)
	}
	// The version the card showed left the index: nothing is installed.
	gone := newCatalogMemory()
	gone.index.Tools, gone.files = nil, map[string][]ForgedFile{}
	gone.add("seat_allocation", "1.1.0", "python3", []ForgedFile{{Path: "seat_allocation.py", Content: "print('1.1.0')\n"}}, false)
	host.Catalog = gone
	outcome, _ = host.Execute(approved, s, install, object(t, `{"name":"seat_allocation"}`))
	if !outcome.IsError || !strings.Contains(string(outcome.Content), "the card showed axlr/seat_allocation@1.0.0, which cannot be installed now") || !strings.Contains(string(outcome.Content), "call axlr_install_tool again") || len(forged.tools) != 0 {
		t.Fatalf("withdrawn version installed: %s, %+v", outcome.Content, forged.tools)
	}
}

func TestInstallRefusesADigestMismatchAndBadRequests(t *testing.T) {
	forged, catalog := &forgedMemory{}, newCatalogMemory()
	s := turnSession(t)
	host := HostToolUseCase{Forge: forged, Catalog: catalog}
	install := hostIdentity(t, domain.HostOperationInstallTool)
	catalog.tamper = true
	outcome, _ := host.Execute(context.Background(), s, install, object(t, `{"name":"seat_allocation"}`))
	if !outcome.IsError || !strings.Contains(string(outcome.Content), "does not match the digest") || len(forged.tools) != 0 {
		t.Fatalf("tampered file installed: %s, %+v", outcome.Content, forged.tools)
	}
	catalog.tamper = false
	for name, args := range map[string]string{
		"unknown tool":    `{"name":"nothing_here"}`,
		"unknown version": `{"name":"seat_allocation","version":"9.9.9"}`,
		"unknown catalog": `{"name":"seat_allocation","catalog":"acme"}`,
		"bad name":        `{"name":"Seat Allocation"}`,
		"unknown field":   `{"name":"seat_allocation","force":true}`,
	} {
		if outcome, _ := host.Execute(context.Background(), s, install, object(t, args)); !outcome.IsError {
			t.Fatalf("%s installed: %s", name, outcome.Content)
		}
	}
	// A forged tool of the same name is the person's to remove first.
	if outcome, _ := host.Execute(context.Background(), s, hostIdentity(t, domain.HostOperationForgeTool), object(t, strings.Replace(forgeArguments, "word_count", "seat_allocation", -1))); outcome.IsError {
		t.Fatal(outcome.Content)
	}
	if outcome, _ := host.Execute(context.Background(), s, install, object(t, `{"name":"seat_allocation"}`)); !outcome.IsError || !strings.Contains(string(outcome.Content), "was forged in this workspace") {
		t.Fatalf("install over a forged tool: %s", outcome.Content)
	}
	// So is one installed from another catalog.
	forged.tools[0].Origin = &ToolOrigin{Catalog: "acme", Version: "1.0.0"}
	if outcome, _ := host.Execute(context.Background(), s, install, object(t, `{"name":"seat_allocation"}`)); !outcome.IsError || !strings.Contains(string(outcome.Content), "installed from catalog acme") {
		t.Fatalf("install over another catalog's tool: %s", outcome.Content)
	}
}

// axlr_catalog searches the cached indexes and describes one tool; it says
// which version the workspace has and carries a catalog's warning.
func TestAxlrCatalogSearchesAndDescribes(t *testing.T) {
	forged, catalog := &forgedMemory{}, newCatalogMemory()
	catalog.add("business_days", "1.2.0", "python3", []ForgedFile{{Path: "business_days.py", Content: "print(1)\n"}}, true)
	catalog.add("business_days", "1.1.0", "python3", []ForgedFile{{Path: "business_days.py", Content: "print(0)\n"}}, false)
	s := turnSession(t)
	host := HostToolUseCase{Forge: forged, Catalog: catalog}
	read := hostIdentity(t, domain.HostOperationCatalog)
	all, _ := host.Execute(context.Background(), s, read, object(t, `{}`))
	if all.IsError || !strings.Contains(string(all.Content), `"total_matches":2`) || strings.Contains(string(all.Content), `"version":"1.1.0"`) || !strings.Contains(string(all.Content), `"version":"1.2.0"`) {
		t.Fatalf("list: %s", all.Content)
	}
	search, _ := host.Execute(context.Background(), s, read, object(t, `{"query":"dhondt elections"}`))
	if search.IsError || !strings.Contains(string(search.Content), `"name":"seat_allocation"`) || strings.Contains(string(search.Content), "business_days") || !strings.Contains(string(search.Content), `"install_with":"axlr_install_tool"`) {
		t.Fatalf("search: %s", search.Content)
	}
	if outcome, _ := host.Execute(context.Background(), s, hostIdentity(t, domain.HostOperationInstallTool), object(t, `{"name":"business_days"}`)); outcome.IsError {
		t.Fatal(outcome.Content)
	}
	if !strings.Contains(string(func() root.Text {
		o, _ := host.Execute(context.Background(), s, read, object(t, `{"name":"business_days"}`))
		return o.Content
	}()), `"installed":"1.2.0"`) {
		t.Fatal("describe does not say the installed version")
	}
	described, _ := host.Execute(context.Background(), s, read, object(t, `{"name":"business_days"}`))
	for _, want := range []string{`"network":true`, `"files":["business_days.py"]`, `"license":"MIT"`, `"call_with":"axlr_run_tool"`} {
		if !strings.Contains(string(described.Content), want) {
			t.Fatalf("describe lacks %s: %s", want, described.Content)
		}
	}
	catalog.warning = "catalog axlr: index: cannot fetch"
	warned, _ := host.Execute(context.Background(), s, read, object(t, `{"query":"seats"}`))
	if !strings.Contains(string(warned.Content), `"warning":"catalog axlr: index: cannot fetch"`) {
		t.Fatalf("warning missing: %s", warned.Content)
	}
	for _, args := range []string{`{"name":"x","query":"y"}`, `{"limit":0}`, `{"name":"nothing_here"}`, `{"other":1}`} {
		if outcome, _ := host.Execute(context.Background(), s, read, object(t, args)); !outcome.IsError {
			t.Fatalf("%s succeeded: %s", args, outcome.Content)
		}
	}
}

// The person's /tools: the overview lists the installed tools and the
// catalog's newer versions; a network tool is warned about on its card.
func TestCatalogUseCaseOverviewUpdatesAndWarnings(t *testing.T) {
	forged, catalog := &forgedMemory{}, newCatalogMemory()
	catalog.add("business_days", "1.0.0", "python3", []ForgedFile{{Path: "business_days.py", Content: "print(0)\n"}}, true)
	use := CatalogUseCase{Catalog: catalog, Forge: forged}
	staged, err := use.Stage(context.Background(), "/ws", InstallRequest{Name: "business_days"})
	if err != nil || len(staged.Warnings) != 1 || !strings.Contains(staged.Warnings[0], "reaches the network") {
		t.Fatalf("stage = %+v, %v", staged, err)
	}
	// Staging reads the cache alone: the card and the install that follows
	// resolve the same index, whatever its age.
	if catalog.cached != 1 || catalog.reads != 0 {
		t.Fatalf("staging read the catalogs %d times and the cache %d", catalog.reads, catalog.cached)
	}
	if _, _, err := use.Install(context.Background(), "/ws", staged, "s1"); err != nil {
		t.Fatal(err)
	}
	catalog.add("business_days", "1.1.0", "python3", []ForgedFile{{Path: "business_days.py", Content: "print(1)\n"}}, false)
	overview, err := use.Overview(context.Background(), "/ws", true)
	if err != nil || len(overview.Installed) != 1 || len(overview.Updates) != 1 || overview.Updates[0].Available != "1.1.0" || overview.Updates[0].Installed != "1.0.0" || catalog.refreshs != 1 {
		t.Fatalf("overview = %+v, %v (refreshs %d)", overview, err, catalog.refreshs)
	}
	// A staged tool whose origin was dropped is not installed.
	staged.Tool.Origin = nil
	if _, _, err := use.Install(context.Background(), "/ws", staged, "s1"); err == nil {
		t.Fatal("installed without origin")
	}
	if !newerVersion("1.10.0", "1.9.3") || newerVersion("1.0.0", "1.0.0") || !newerVersion("2.0.0", "junk") {
		t.Fatal("version order")
	}
}
