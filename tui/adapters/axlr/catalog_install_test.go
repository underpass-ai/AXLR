package axlr

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/runtime"
	"github.com/underpass-ai/AXLR/tui/adapters/catalog"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// A catalog tool installed from a signed index served over HTTP runs at
// once through axlr_run_tool like a forged one: the same store, the same
// runtime, the same refusal when its files change.
func TestACatalogToolInstallsFromASignedIndexAndRuns(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("the catalog tool is a Python program")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not on PATH")
	}
	root := filepath.Join("..", "..", "..", "catalog")
	index, err := os.ReadFile(filepath.Join(root, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key := catalog.PublicKey{Key: public}
	copy(key.ID[:], []byte("e2e-test"))
	signature := catalog.Sign(private, key.ID, index, "timestamp:1760000000\tfile:index.json", true)
	files := http.FileServer(http.Dir(root))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.json.minisig" {
			_, _ = w.Write(signature)
			return
		}
		files.ServeHTTP(w, r)
	}))
	defer server.Close()

	workspace := t.TempDir()
	executor, err := runtime.New(runtime.Config{Root: workspace, Env: []string{"PATH=" + os.Getenv("PATH")}})
	if err != nil {
		t.Fatal(err)
	}
	defer executor.Close()
	store, err := storage.NewForgedToolStore(filepath.Join(t.TempDir(), "forged"))
	if err != nil {
		t.Fatal(err)
	}
	client := &catalog.Client{Dir: filepath.Join(t.TempDir(), "catalog"), Entries: []catalog.Catalog{{Name: "axlr", URL: server.URL + "/index.json", PublicKey: key.String()}}}
	host := application.HostToolUseCase{Forge: store, Catalog: client, Tools: ToolRunner{Executor: executor}, Validation: NewToolArgumentValidator()}
	session, err := domain.NewSession("0123456789abcdef0123456789abcdef", domain.Workspace(workspace), "model")
	if err != nil {
		t.Fatal(err)
	}
	read, _ := domain.NewHostToolIdentity(domain.HostOperationCatalog)
	install, _ := domain.NewHostToolIdentity(domain.HostOperationInstallTool)
	run, _ := domain.NewHostToolIdentity(domain.HostOperationRunTool)

	found, err := host.Execute(context.Background(), session, read, jsonValue(t, `{"query":"dhondt"}`))
	if err != nil || found.IsError || !strings.Contains(string(found.Content), `"name":"seat_allocation"`) {
		t.Fatalf("axlr_catalog = %+v, %v", found, err)
	}
	installed, err := host.Execute(context.Background(), session, install, jsonValue(t, `{"name":"seat_allocation"}`))
	if err != nil || installed.IsError || !strings.Contains(string(installed.Content), `"catalog":"axlr"`) {
		t.Fatalf("install = %+v, %v", installed, err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".axlr", "tools", "seat_allocation", "seat_allocation.py")); err != nil {
		t.Fatalf("tool file not written: %v", err)
	}
	tools, err := store.List(context.Background(), workspace)
	if err != nil || len(tools) != 1 || tools[0].Origin == nil || tools[0].Origin.Catalog != "axlr" || tools[0].Origin.Version != "1.0.0" {
		t.Fatalf("registry = %+v, %v", tools, err)
	}

	// The manifest's example: D'Hondt with a 3 % threshold.
	result, err := host.Execute(context.Background(), session, run, jsonValue(t, `{"name":"seat_allocation","arguments":{"seats":7,"votes":{"A":100000,"B":80000,"C":30000,"D":20000},"method":"dhondt","threshold":3}}`))
	if err != nil || result.IsError || !strings.Contains(string(result.Content), `\"A\": 3`) || !strings.Contains(string(result.Content), `\"D\": 0`) {
		t.Fatalf("run = %+v, %v", result, err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".axlr", "tools", "seat_allocation", "seat_allocation.py"), []byte("print('pwned')"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, _ := host.Execute(context.Background(), session, run, jsonValue(t, `{"name":"seat_allocation","arguments":{"seats":1,"votes":{"A":1}}}`))
	if !changed.IsError || !strings.Contains(string(changed.Content), "changed since it was forged") {
		t.Fatalf("a changed tool ran: %s", changed.Content)
	}
}
