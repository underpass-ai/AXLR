package terminal

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// keysFake is an in-memory ClientKeyPort.
type keysFake struct{ key *application.ClientKey }

func (k *keysFake) Load(context.Context) (*application.ClientKey, error) { return k.key, nil }
func (k *keysFake) Create(context.Context) (*application.ClientKey, error) {
	if k.key != nil {
		return nil, errors.New("client.key exists")
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	k.key, _ = application.NewClientKey(private)
	return k.key, nil
}

// registryFake answers as told and verifies the bundle signature.
type registryFake struct {
	enrolls   []application.EnrollRequest
	uploads   []application.UploadRequest
	polls     int
	submitErr error
	reports   []application.VersionReport
}

func (r *registryFake) Enroll(_ context.Context, _ string, _ *application.ClientKey, request application.EnrollRequest) (application.EnrollResult, error) {
	r.enrolls = append(r.enrolls, request)
	if request.ID == "taken" {
		return application.EnrollResult{}, &application.RegistryError{Status: 409, Body: `{"error":"publisher taken is active"}`}
	}
	return application.EnrollResult{ID: request.ID, Status: "pending"}, nil
}
func (r *registryFake) Submit(_ context.Context, _ string, key *application.ClientKey, upload application.UploadRequest) (application.VersionReport, error) {
	if r.submitErr != nil {
		err := r.submitErr
		r.submitErr = nil
		return application.VersionReport{}, err
	}
	if !ed25519.Verify(key.Public, application.BundleSignatureMessage(application.BundleDigest(upload.Manifest, upload.Files)), upload.Signature) {
		return application.VersionReport{}, &application.RegistryError{Status: 401, Body: `{"error":"bundle signature does not verify"}`}
	}
	r.uploads = append(r.uploads, upload)
	return application.VersionReport{Name: "word_count", Version: "1.0.0", Status: "scanned", Findings: []application.Finding{{Rule: "fs.write", File: "word_count.py", Line: 2, Severity: "info", Message: "writes a file"}}}, nil
}
func (r *registryFake) Version(context.Context, string, *application.ClientKey, string, string) (application.VersionReport, error) {
	r.polls++
	if len(r.reports) == 0 {
		return application.VersionReport{}, errors.New("no report")
	}
	report := r.reports[0]
	if len(r.reports) > 1 {
		r.reports = r.reports[1:]
	}
	return report, nil
}

// filesFake is an in-memory ToolFilesPort.
type filesFake struct{ files map[string][]byte }

func (f *filesFake) ReadToolFile(_ context.Context, _, tool, file string) ([]byte, error) {
	data, ok := f.files[tool+"/"+file]
	if !ok {
		return nil, application.ErrNoToolFile
	}
	return data, nil
}
func (f *filesFake) WriteToolFile(_ context.Context, _, tool, file string, data []byte) error {
	f.files[tool+"/"+file] = data
	return nil
}

func publishModel(t *testing.T) (AppModel, *forgeFake, *registryFake, *filesFake, *keysFake) {
	t.Helper()
	content := "import json,sys\nprint(json.dumps({'words': 3}))\n"
	sum := sha256.Sum256([]byte(content))
	forge := &forgeFake{tools: []application.ForgedTool{{Name: "word_count", Description: "Count the words of a text.", InputSchema: json.RawMessage(`{"type":"object"}`), Program: "python3", Args: []string{".axlr/tools/word_count/word_count.py"}, Files: map[string]string{"word_count.py": hex.EncodeToString(sum[:])}}}}
	files := &filesFake{files: map[string][]byte{"word_count/word_count.py": []byte(content), "word_count/tests.json": []byte(`[{"name":"three","input":{"text":"a b c"},"expect":{"stdout_json":{"words":3}}}]`)}}
	catalog := newCatalogFake("business_days")
	catalog.api = "https://api.staging.example"
	registry, keys := &registryFake{}, &keysFake{}
	s := navSession(t)
	m := navModel(t, &s)
	m.deps.ToolCatalog = application.CatalogUseCase{Catalog: catalog, Forge: forge, Keys: keys, Registry: registry, Files: files}
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	return m, forge, registry, files, keys
}

// /tools identity says there is no key, /tools keygen creates it and shows
// the key id and the public key, a second keygen is refused.
func TestToolsIdentityAndKeygen(t *testing.T) {
	m, _, _, _, keys := publishModel(t)
	defer m.zones.Close()
	m = runTools(t, m, "/tools identity")
	if !strings.Contains(m.Info.Text(), "No client key yet") {
		t.Fatalf("identity without a key:\n%s", m.Info.Text())
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = runTools(t, m, "/tools keygen")
	text := m.Info.Text()
	if keys.key == nil || !strings.Contains(text, "key id "+keys.key.ID()) || !strings.Contains(text, "public key "+keys.key.PublicBase64()) || !strings.Contains(text, "created now") || !strings.Contains(m.Status.Notice, "client key created") {
		t.Fatalf("keygen:\n%s\n%q", text, m.Status.Notice)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = runTools(t, m, "/tools keygen")
	if !strings.Contains(m.Info.Text(), "exists") {
		t.Fatalf("second keygen:\n%s", m.Info.Text())
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = runTools(t, m, "/tools identity")
	if !strings.Contains(m.Info.Text(), "key id "+keys.key.ID()) || strings.Contains(m.Info.Text(), "created now") {
		t.Fatalf("identity:\n%s", m.Info.Text())
	}
}

// /tools enroll sends the key and reports the pending state, or the
// registry's error body; the key is created when there is none.
func TestToolsEnrollReportsPendingOrTheError(t *testing.T) {
	m, _, registry, _, keys := publishModel(t)
	defer m.zones.Close()
	m = runTools(t, m, "/tools enroll acme acme_* acme_tools_* --contact ops@acme.example")
	if keys.key == nil || len(registry.enrolls) != 1 || registry.enrolls[0].Contact != "ops@acme.example" || len(registry.enrolls[0].Namespaces) != 2 {
		t.Fatalf("enroll sent %+v", registry.enrolls)
	}
	if text := m.Info.Text(); !strings.Contains(text, "pending: an operator of axlr must approve it") || !strings.Contains(text, "publisher acme") || !strings.Contains(text, "key id "+keys.key.ID()) {
		t.Fatalf("enroll panel:\n%s", text)
	}
	if !strings.Contains(m.Status.Notice, "pending: an operator of axlr") {
		t.Fatalf("notice %q", m.Status.Notice)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = runTools(t, m, "/tools enroll taken taken_* --contact ops@taken.example")
	if !strings.Contains(m.Info.Text(), "409") || !strings.Contains(m.Info.Text(), "publisher taken is active") {
		t.Fatalf("409 panel:\n%s", m.Info.Text())
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	for _, short := range []string{"/tools enroll acme --contact ops@acme.example", "/tools enroll acme acme_*"} {
		m = runTools(t, m, short)
		if !strings.Contains(m.Status.Notice, "usage") || !strings.Contains(m.Status.Notice, "--contact <address>") {
			t.Fatalf("%s: %q", short, m.Status.Notice)
		}
	}
}

// /tools publish asks the missing catalog fields in the composer, shows
// the bundle on a card, uploads on y, waits out a 429 and polls until the
// tests settle, showing the findings and the result.
func TestToolsPublishAsksUploadsAndPolls(t *testing.T) {
	publishPollInterval = 0
	defer func() { publishPollInterval = application.PublishPollInterval }()
	m, _, registry, files, keys := publishModel(t)
	defer m.zones.Close()
	m = runTools(t, m, "/tools publish word_count")
	if m.publishAsking != "version" || m.publishDraft == nil {
		t.Fatalf("not asking the version: %q %q", m.Status.Notice, m.Status.Error)
	}
	if !strings.Contains(m.Status.Notice, "version of word_count?") || !strings.Contains(m.Info.Text(), "Publish word_count to catalog axlr (https://api.staging.example)") {
		t.Fatalf("question: %q\n%s", m.Status.Notice, m.Info.Text())
	}
	// A bad answer is refused and asked again; the panel keeps the draft.
	m.Composer.Input.SetValue("one")
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.publishAsking != "version" || !strings.Contains(m.Status.Error, "major.minor.patch") {
		t.Fatalf("bad version: %q %q", m.publishAsking, m.Status.Error)
	}
	for _, answer := range []string{"1.0.0", "text", "MIT"} {
		m.Composer.Input.SetValue(answer)
		next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		m = drain(t, next.(AppModel), cmd)
	}
	// Tags were never asked: catalog.json carries them empty. The bundle is
	// built, saved and on its card.
	if m.publishAsking != "" || !m.toolsAsking || m.publishBundle == nil {
		t.Fatalf("after the fields: asking %q, card %v, bundle %v, %q %q", m.publishAsking, m.toolsAsking, m.publishBundle != nil, m.Status.Notice, m.Status.Error)
	}
	if saved, ok := files.files["word_count/catalog.json"]; !ok || !strings.Contains(string(saved), `"license": "MIT"`) {
		t.Fatalf("catalog.json %s", saved)
	}
	card := m.Info.Text()
	for _, want := range []string{"version 1.0.0 · category text · license MIT", "files word_count.py", "tests tests.json", "bundle digest " + m.publishBundle.Digest, "key id " + keys.key.ID() + " · created now", "$ python3 .axlr/tools/word_count/word_count.py", "── word_count.py ──"} {
		if !strings.Contains(card, want) {
			t.Fatalf("card lacks %q:\n%s", want, card)
		}
	}
	if !strings.Contains(m.Status.Notice, "Upload word_count 1.0.0 to catalog axlr, signed with key "+keys.key.ID()) {
		t.Fatalf("confirm %q", m.Status.Notice)
	}
	// The first upload is rate limited: the panel shows the Retry-After and
	// the upload is tried again after it.
	registry.submitErr = &application.RateLimited{RetryAfter: time.Second}
	passed := true
	registry.reports = []application.VersionReport{
		{Name: "word_count", Version: "1.0.0", Status: "scanned", Findings: []application.Finding{{Rule: "fs.write", File: "word_count.py", Line: 2, Severity: "info", Message: "writes a file"}}},
		{Name: "word_count", Version: "1.0.0", Status: "scanned", Findings: []application.Finding{{Rule: "fs.write", File: "word_count.py", Line: 2, Severity: "info", Message: "writes a file"}}, TestsPassed: &passed},
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = next.(AppModel)
	// The y's operation completes with the 429; its completion schedules
	// the retry tick, which the test fires by hand.
	msg := cmd()
	for {
		batch, ok := msg.(tea.BatchMsg)
		if !ok {
			break
		}
		msg = batch[0]()
	}
	next, tick := m.Update(msg)
	m = next.(AppModel)
	if tick == nil || !strings.Contains(m.Info.Text(), "Retry-After 1s") || len(registry.uploads) != 0 {
		t.Fatalf("429 not shown: %v\n%s", tick, m.Info.Text())
	}
	next, cmd = m.Update(publishRetryMsg{gen: m.publishGen})
	m = drain(t, next.(AppModel), cmd)
	if len(registry.uploads) != 1 || registry.polls != 2 || m.publishDraft != nil || m.toolsAsking {
		t.Fatalf("uploads %d, polls %d, draft %v", len(registry.uploads), registry.polls, m.publishDraft)
	}
	text := m.Info.Text()
	for _, want := range []string{"uploaded word_count 1.0.0 to catalog axlr", "status scanned", "info · fs.write · word_count.py:2 · writes a file", "tests passed", "word_count 1.0.0 · status scanned · tests passed"} {
		if !strings.Contains(text, want) {
			t.Fatalf("report lacks %q:\n%s", want, text)
		}
	}
	if !strings.Contains(m.Status.Notice, "tests passed") {
		t.Fatalf("notice %q", m.Status.Notice)
	}
	var manifest map[string]any
	if err := json.Unmarshal(registry.uploads[0].Manifest, &manifest); err != nil || manifest["args"].([]any)[0] != "word_count.py" {
		t.Fatalf("manifest %s", registry.uploads[0].Manifest)
	}
	// The next publish finds catalog.json complete and goes to the card at
	// once; n sends nothing.
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	m = runTools(t, m, "/tools publish word_count@1.0.1")
	if m.publishAsking != "" || !m.toolsAsking || m.publishDraft.Metadata.Version != "1.0.1" {
		t.Fatalf("second publish: asking %q, card %v", m.publishAsking, m.toolsAsking)
	}
	m = update(m, tea.KeyPressMsg{Code: 'n', Text: "n"})
	if len(registry.uploads) != 1 || m.publishBundle != nil || !strings.Contains(m.Status.Notice, "Nothing published") {
		t.Fatalf("n uploaded: %d, %q", len(registry.uploads), m.Status.Notice)
	}
	// Esc while a field is asked publishes nothing either.
	delete(files.files, "word_count/catalog.json")
	m = runTools(t, m, "/tools publish word_count")
	if m.publishAsking != "version" {
		t.Fatalf("asking %q", m.publishAsking)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.publishAsking != "" || m.publishDraft != nil || !strings.Contains(m.Status.Notice, "Nothing published") {
		t.Fatalf("esc: %q %v %q", m.publishAsking, m.publishDraft, m.Status.Notice)
	}
}

// Without the registry ports the verbs say so, and the usage names them.
func TestToolsPublishVerbsNeedTheRegistry(t *testing.T) {
	m := toolsModel(t, &forgeFake{}, newCatalogFake("business_days"))
	defer m.zones.Close()
	for _, command := range []string{"/tools publish word_count", "/tools identity", "/tools keygen", "/tools enroll acme acme_* --contact ops@acme.example"} {
		m = runTools(t, m, command)
		if !strings.Contains(m.Status.Error, "No registry client") {
			t.Fatalf("%s: %q", command, m.Status.Error)
		}
	}
	if usage := slashUsage("/tools"); !strings.Contains(usage, "/tools publish <name>[@version] [--to <catalog>]") || !strings.Contains(usage, "/tools enroll <publisher> <namespace>…") {
		t.Fatalf("usage %q", usage)
	}
}

// On 20 October 2026 the /tools publish panel was unreadable on the paper
// theme: panels rendered their text in the dark theme's colours. Every
// panel's transcript now carries the console's theme.
func TestPanelsRenderInTheConsoleTheme(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Theme.ID = domain.ThemePaper
	for _, open := range []func(AppModel) AppModel{
		func(m AppModel) AppModel { return m.openToolsOverlay("text") },
		func(m AppModel) AppModel { return m.openUsagePanel() },
		func(m AppModel) AppModel { return m.openSessionPanel() },
	} {
		if got := open(m).Info.theme.ID; got != domain.ThemePaper {
			t.Fatalf("a panel renders in theme %q", got)
		}
	}
}

// On 20 October 2026 the person pressed another key at the upload question,
// which then lived only in the status line, and the panel closed with
// nothing uploaded and nothing said: only y uploads, only n or Esc gives
// up, and any other key keeps the question.
func TestThePublishQuestionTakesOnlyYNOrEsc(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.overlay = "tools"
	m.publishDraft = &application.PublishDraft{Catalog: "axlr", Tool: application.ForgedTool{Name: "qa8_probe"}, Metadata: application.PublishMetadata{Version: "1.0.0"}}
	m.publishBundle = &application.PublishBundle{KeyID: "abc"}
	m.toolsAsking = true
	m, _ = m.answerTools(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.toolsAsking || m.publishBundle == nil || m.overlay != "tools" || !strings.Contains(m.Status.Notice, "y uploads · n or Esc") {
		t.Fatalf("Enter dropped the question: asking %v, notice %q", m.toolsAsking, m.Status.Notice)
	}
	m, _ = m.answerTools(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if m.toolsAsking || m.publishBundle != nil || m.Status.Notice != "Nothing published" {
		t.Fatalf("n kept the upload: notice %q", m.Status.Notice)
	}
}
