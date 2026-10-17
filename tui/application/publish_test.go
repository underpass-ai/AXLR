package application

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
)

// keysMemory is an in-memory ClientKeyPort.
type keysMemory struct {
	key     *ClientKey
	creates int
}

func (k *keysMemory) Load(context.Context) (*ClientKey, error) { return k.key, nil }
func (k *keysMemory) Create(context.Context) (*ClientKey, error) {
	if k.key != nil {
		return nil, errors.New("a key exists")
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	k.key, _ = NewClientKey(private)
	k.creates++
	return k.key, nil
}

// registryMemory records what is sent and answers as told.
type registryMemory struct {
	api       string
	enrolls   []EnrollRequest
	uploads   []UploadRequest
	polls     int
	enrollErr error
	report    VersionReport
}

func (r *registryMemory) Enroll(_ context.Context, api string, key *ClientKey, request EnrollRequest) (EnrollResult, error) {
	r.api = api
	r.enrolls = append(r.enrolls, request)
	if r.enrollErr != nil {
		return EnrollResult{}, r.enrollErr
	}
	return EnrollResult{ID: request.ID, Status: "pending"}, nil
}
func (r *registryMemory) Submit(_ context.Context, api string, key *ClientKey, upload UploadRequest) (VersionReport, error) {
	r.api = api
	r.uploads = append(r.uploads, upload)
	if !ed25519.Verify(key.Public, BundleSignatureMessage(BundleDigest(upload.Manifest, upload.Files)), upload.Signature) {
		return VersionReport{}, &RegistryError{Status: 401, Body: `{"error":"bundle signature does not verify"}`}
	}
	return VersionReport{Name: "word_count", Version: "1.0.0", Status: "scanned", Findings: []Finding{}}, nil
}
func (r *registryMemory) Version(context.Context, string, *ClientKey, string, string) (VersionReport, error) {
	r.polls++
	return r.report, nil
}

// filesMemory is an in-memory ToolFilesPort over one tool's directory.
type filesMemory struct{ files map[string][]byte }

func (f *filesMemory) ReadToolFile(_ context.Context, _, tool, file string) ([]byte, error) {
	data, ok := f.files[tool+"/"+file]
	if !ok {
		return nil, ErrNoToolFile
	}
	return data, nil
}
func (f *filesMemory) WriteToolFile(_ context.Context, _, tool, file string, data []byte) error {
	f.files[tool+"/"+file] = data
	return nil
}

// forgeMemory is a ForgedToolsPort with one forged tool whose digests
// match filesMemory.
type forgeMemory struct {
	tools    []ForgedTool
	verified int
}

func (f *forgeMemory) List(context.Context, string) ([]ForgedTool, error) { return f.tools, nil }
func (f *forgeMemory) Forge(_ context.Context, _ string, tool ForgedTool, _ []ForgedFile) (ForgedTool, bool, error) {
	return tool, false, nil
}
func (f *forgeMemory) Verify(context.Context, string, ForgedTool) error { f.verified++; return nil }
func (f *forgeMemory) Remove(context.Context, string, string) (bool, error) {
	return false, nil
}

func publishFixture() (CatalogUseCase, *forgeMemory, *filesMemory, *registryMemory, *keysMemory) {
	content := []byte("import json,sys\nprint(json.dumps({'words': 3}))\n")
	sum := sha256.Sum256(content)
	forge := &forgeMemory{tools: []ForgedTool{{Name: "word_count", Description: "Count the words of a text. Returns words.", InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`), Program: "python3", Args: []string{".axlr/tools/word_count/word_count.py"}, Files: map[string]string{"word_count.py": hex.EncodeToString(sum[:])}}}}
	files := &filesMemory{files: map[string][]byte{"word_count/word_count.py": content, "word_count/tests.json": []byte(`[{"name":"three","input":{"text":"a b c"},"expect":{"stdout_json":{"words":3}}}]`)}}
	catalog := newCatalogMemory()
	catalog.api = "https://api.staging.example"
	registry := &registryMemory{}
	keys := &keysMemory{}
	return CatalogUseCase{Catalog: catalog, Forge: forge, Keys: keys, Registry: registry, Files: files}, forge, files, registry, keys
}

// The bundle digest and signature are the registry's: the listing in
// name order, SHA-256, signed under the bundle prefix.
func TestBundleDigestIsTheCanonicalListing(t *testing.T) {
	manifest := []byte(`{"name":"x"}`)
	files := map[string][]byte{"b.py": []byte("b"), "a.py": []byte("a")}
	h := func(data []byte) string { s := sha256.Sum256(data); return hex.EncodeToString(s[:]) }
	listing := "manifest " + h(manifest) + "\nfile a.py " + h([]byte("a")) + "\nfile b.py " + h([]byte("b")) + "\n"
	if got := BundleDigest(manifest, files); got != h([]byte(listing)) {
		t.Fatalf("digest %s", got)
	}
	if string(BundleSignatureMessage("abc")) != "axlr-tool-bundle-v1\nabc" {
		t.Fatal("signature message")
	}
}

// Prepare reads a forged tool's verified files, tests.json and catalog.json,
// says which fields are missing; Bundle writes the manifest with bare args
// and signs it with a key created on the spot; Submit uploads it.
func TestPublishPreparesBundlesAndUploads(t *testing.T) {
	use, forge, files, registry, keys := publishFixture()
	ctx := context.Background()
	draft, err := use.Prepare(ctx, "/ws", PublishRequest{Name: "word_count"})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Catalog != "axlr" || draft.API != "https://api.staging.example" || forge.verified != 1 || string(draft.Files["word_count.py"]) == "" || draft.Tests == nil {
		t.Fatalf("draft %+v", draft)
	}
	if strings.Join(draft.Missing, ",") != "version,category,license" {
		t.Fatalf("missing %v", draft.Missing)
	}
	if _, err := use.Bundle(ctx, draft); err == nil || !strings.Contains(err.Error(), "lacks version") {
		t.Fatalf("bundle before the fields: %v", err)
	}
	if err := SetPublishField(&draft, "version", "1.0"); err == nil {
		t.Fatal("a two-part version was accepted")
	}
	for field, value := range map[string]string{"version": "1.0.0", "category": "text", "license": "MIT", "tags": "words, counting"} {
		if err := SetPublishField(&draft, field, value); err != nil {
			t.Fatalf("%s: %v", field, err)
		}
	}
	if len(draft.Missing) != 0 || len(draft.Metadata.Tags) != 2 {
		t.Fatalf("after the fields: %+v", draft)
	}
	if err := use.SaveMetadata(ctx, "/ws", draft); err != nil {
		t.Fatal(err)
	}
	var saved PublishMetadata
	if err := json.Unmarshal(files.files["word_count/catalog.json"], &saved); err != nil || saved.Version != "1.0.0" || saved.License != "MIT" || saved.Requires.Packages == nil {
		t.Fatalf("catalog.json %s: %v", files.files["word_count/catalog.json"], err)
	}
	bundle, err := use.Bundle(ctx, draft)
	if err != nil {
		t.Fatal(err)
	}
	if !bundle.KeyCreated || keys.creates != 1 || bundle.KeyID != keys.key.ID() || bundle.Upload.KeyID != bundle.KeyID {
		t.Fatalf("key: %+v, creates %d", bundle, keys.creates)
	}
	var manifest struct {
		Name    string   `json:"name"`
		Args    []string `json:"args"`
		Files   []string `json:"files"`
		Catalog struct {
			Version  string   `json:"version"`
			Category string   `json:"category"`
			Tags     []string `json:"tags"`
			License  string   `json:"license"`
			Requires struct {
				Packages []string `json:"packages"`
			} `json:"requires"`
			Network bool `json:"network"`
		} `json:"catalog"`
	}
	if err := json.Unmarshal(bundle.Upload.Manifest, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Args[0] != "word_count.py" || manifest.Files[0] != "word_count.py" || manifest.Catalog.Version != "1.0.0" || manifest.Catalog.Category != "text" || manifest.Catalog.Requires.Packages == nil {
		t.Fatalf("manifest %s", bundle.Upload.Manifest)
	}
	var keys2 map[string]json.RawMessage
	_ = json.Unmarshal(bundle.Upload.Manifest, &keys2)
	for _, key := range []string{"name", "description", "input_schema", "program", "args", "files", "catalog"} {
		delete(keys2, key)
	}
	if len(keys2) != 0 {
		t.Fatalf("manifest has unknown keys %v", keys2)
	}
	if bundle.Digest != BundleDigest(bundle.Upload.Manifest, bundle.Upload.Files) || !ed25519.Verify(keys.key.Public, BundleSignatureMessage(bundle.Digest), bundle.Upload.Signature) {
		t.Fatal("bundle signature does not verify")
	}
	report, err := use.Submit(ctx, draft, bundle)
	if err != nil || report.Status != "scanned" || report.Settled() || len(registry.uploads) != 1 || registry.api != draft.API || string(registry.uploads[0].Tests) == "" {
		t.Fatalf("submit: %+v, %v, %+v", report, err, registry.uploads)
	}
	passed := true
	registry.report = VersionReport{Name: "word_count", Version: "1.0.0", Status: "scanned", TestsPassed: &passed}
	if polled, err := use.Poll(ctx, draft); err != nil || !polled.Settled() || registry.polls != 1 {
		t.Fatalf("poll: %+v %v", polled, err)
	}
	// A second Prepare reads catalog.json: nothing is missing, and a version
	// named on the command replaces the stored one.
	again, err := use.Prepare(ctx, "/ws", PublishRequest{Name: "word_count", Version: "1.0.1"})
	if err != nil || len(again.Missing) != 0 || again.Metadata.Version != "1.0.1" || again.Metadata.Category != "text" {
		t.Fatalf("again: %+v %v", again, err)
	}
}

// Prepare refuses what the registry would: a tool installed from a
// catalog, a nested file, bad tests; without tests it warns; a catalog
// without api_url, or two with one and none named, is refused.
func TestPublishRefusalsAndWarnings(t *testing.T) {
	use, forge, files, _, _ := publishFixture()
	ctx := context.Background()
	if _, err := use.Prepare(ctx, "/ws", PublishRequest{Name: "nothing"}); err == nil || !strings.Contains(err.Error(), "no tool named") {
		t.Fatalf("unknown: %v", err)
	}
	forge.tools[0].Origin = &ToolOrigin{Catalog: "axlr", Version: "1.0.0"}
	if _, err := use.Prepare(ctx, "/ws", PublishRequest{Name: "word_count"}); err == nil || !strings.Contains(err.Error(), "installed from catalog") {
		t.Fatalf("installed: %v", err)
	}
	forge.tools[0].Origin = nil
	files.files["word_count/tests.json"] = []byte(`[{"name":"a","input":[],"expect":{}}]`)
	if _, err := use.Prepare(ctx, "/ws", PublishRequest{Name: "word_count"}); err == nil || !strings.Contains(err.Error(), "object input") {
		t.Fatalf("bad tests: %v", err)
	}
	delete(files.files, "word_count/tests.json")
	draft, err := use.Prepare(ctx, "/ws", PublishRequest{Name: "word_count"})
	if err != nil || draft.Tests != nil || len(draft.Warnings) != 1 || !strings.Contains(draft.Warnings[0], "tests.json is missing") {
		t.Fatalf("no tests: %+v %v", draft, err)
	}
	forge.tools[0].Files["lib/helper.py"] = "00"
	if _, err := use.Prepare(ctx, "/ws", PublishRequest{Name: "word_count"}); err == nil || !strings.Contains(err.Error(), "bare file name") {
		t.Fatalf("nested: %v", err)
	}
	delete(forge.tools[0].Files, "lib/helper.py")
	catalog := use.Catalog.(*catalogMemory)
	catalog.api = ""
	if _, err := use.Prepare(ctx, "/ws", PublishRequest{Name: "word_count"}); err == nil || !strings.Contains(err.Error(), "no catalog has an api_url") {
		t.Fatalf("no api: %v", err)
	}
	if _, err := use.Prepare(ctx, "/ws", PublishRequest{Name: "word_count", Catalog: "axlr"}); err == nil || !strings.Contains(err.Error(), "has no api_url") {
		t.Fatalf("named without api: %v", err)
	}
	if _, err := use.Prepare(ctx, "/ws", PublishRequest{Name: "word_count", Catalog: "other"}); err == nil || !strings.Contains(err.Error(), "no catalog is named other") {
		t.Fatalf("unknown catalog: %v", err)
	}
	if _, err := use.Prepare(ctx, "/ws", PublishRequest{Name: "word_count", Version: "v1"}); err == nil {
		t.Fatal("a bad version on the command was accepted")
	}
	off := CatalogUseCase{Catalog: catalog, Forge: forge}
	if off.Publishable() {
		t.Fatal("publishable without a key store")
	}
	if _, err := off.Prepare(ctx, "/ws", PublishRequest{Name: "word_count"}); err == nil {
		t.Fatal("prepared without the ports")
	}
}

// Keygen creates the key once; Identity shows it; Enroll sends the id, the
// namespaces and the contact to the catalog's registry and reports the
// pending state, or the registry's error body.
func TestIdentityKeygenAndEnroll(t *testing.T) {
	use, _, _, registry, keys := publishFixture()
	ctx := context.Background()
	if _, found, err := use.Identity(ctx); err != nil || found {
		t.Fatalf("identity before keygen: %v %v", found, err)
	}
	identity, err := use.Keygen(ctx)
	if err != nil || len(identity.KeyID) != 16 || identity.PublicKey != keys.key.PublicBase64() {
		t.Fatalf("keygen: %+v %v", identity, err)
	}
	if _, err := use.Keygen(ctx); err == nil {
		t.Fatal("a second keygen replaced the key")
	}
	shown, found, err := use.Identity(ctx)
	if err != nil || !found || shown != identity {
		t.Fatalf("identity: %+v %v %v", shown, found, err)
	}
	result, catalog, err := use.Enroll(ctx, "", EnrollRequest{ID: "acme", Namespaces: []string{"acme_*"}, Contact: "ops@acme.example"})
	if err != nil || result.Status != "pending" || catalog != "axlr" || registry.api != "https://api.staging.example" || registry.enrolls[0].Contact != "ops@acme.example" {
		t.Fatalf("enroll: %+v %s %v", result, catalog, err)
	}
	for name, request := range map[string]EnrollRequest{
		"bad id":        {ID: "Acme", Namespaces: []string{"acme_*"}, Contact: "ops@acme.example"},
		"no namespace":  {ID: "acme", Contact: "ops@acme.example"},
		"bad namespace": {ID: "acme", Namespaces: []string{"../x"}, Contact: "ops@acme.example"},
		"no contact":    {ID: "acme", Namespaces: []string{"acme_*"}},
		"spaced":        {ID: "acme", Namespaces: []string{"acme_*"}, Contact: "ops @acme"},
	} {
		if _, _, err := use.Enroll(ctx, "", request); err == nil {
			t.Fatalf("%s was sent", name)
		}
	}
	registry.enrollErr = &RegistryError{Status: 409, Body: `{"error":"publisher acme is active"}`}
	if _, _, err := use.Enroll(ctx, "axlr", EnrollRequest{ID: "acme", Namespaces: []string{"acme_*"}, Contact: "ops@acme.example"}); err == nil || !strings.Contains(err.Error(), "409") || !strings.Contains(err.Error(), "acme is active") {
		t.Fatalf("409: %v", err)
	}
}
