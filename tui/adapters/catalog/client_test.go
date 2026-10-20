package catalog

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// catalogServer serves the repository's catalog/ directory (its real
// index.json, manifests and files) from a loopback HTTP server, with an
// index signature made in the test and whatever the test overrides.
type catalogServer struct {
	*httptest.Server
	key       PublicKey
	private   ed25519.PrivateKey
	index     []byte
	signature []byte
	// overrides replaces a served path's body; requests counts fetches.
	overrides map[string][]byte
	requests  atomic.Int64
}

func serveCatalog(t *testing.T) *catalogServer {
	t.Helper()
	root := filepath.Join("..", "..", "..", "catalog")
	index, err := os.ReadFile(filepath.Join(root, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	key, private := testKey(t)
	s := &catalogServer{key: key, private: private, index: index, overrides: map[string][]byte{}}
	s.signature = Sign(private, key.ID, index, "timestamp:1760000000\tfile:index.json", true)
	files := http.FileServer(http.Dir(root))
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		if body, ok := s.overrides[r.URL.Path]; ok {
			_, _ = w.Write(body)
			return
		}
		switch r.URL.Path {
		case "/index.json":
			_, _ = w.Write(s.index)
		case "/index.json.minisig":
			_, _ = w.Write(s.signature)
		default:
			files.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *catalogServer) client(t *testing.T) *Client {
	t.Helper()
	return &Client{Dir: filepath.Join(t.TempDir(), "catalog"), Entries: []Catalog{{Name: "axlr", URL: s.URL + "/index.json", PublicKey: s.key.String()}}, LookPath: func(string) (string, error) { return "/usr/bin/python3", nil }}
}

// The real index is fetched over HTTP, verified and cached; a tampered
// index is refused and the cache kept; the cache serves until it is a day
// old.
func TestClientFetchesVerifiesAndCachesTheIndex(t *testing.T) {
	server := serveCatalog(t)
	client := server.client(t)
	now := time.Date(2026, 10, 16, 20, 0, 0, 0, time.UTC)
	client.Now = func() time.Time { return now }
	states, err := client.Catalogs(context.Background(), false)
	if err != nil || len(states) != 1 || states[0].Index == nil || states[0].Warning != "" || len(states[0].Index.Tools) < 2 || states[0].Index.Catalog != "axlr" {
		t.Fatalf("states = %+v, %v", states, err)
	}
	if _, err := os.Stat(filepath.Join(client.Dir, "axlr", "index.json.minisig")); err != nil {
		t.Fatalf("signature not cached: %v", err)
	}
	fetched := server.requests.Load()
	if _, err := client.Catalogs(context.Background(), false); err != nil || server.requests.Load() != fetched {
		t.Fatalf("a fresh cache was fetched again (%d → %d)", fetched, server.requests.Load())
	}
	// A tampered index: refused, and the old cache stays the index.
	server.index = []byte(strings.Replace(string(server.index), `"network": false`, `"network": true`, 1))
	states, _ = client.Catalogs(context.Background(), true)
	if states[0].Index == nil || !strings.Contains(states[0].Warning, "does not verify") || states[0].Index.Tools[0].Network {
		t.Fatalf("tampered index accepted: %+v", states[0])
	}
	// A failed fetch keeps the cache too.
	server.overrides["/index.json"] = nil
	server.Close()
	states, _ = client.Catalogs(context.Background(), true)
	if states[0].Index == nil || !strings.Contains(states[0].Warning, "network request failed") {
		t.Fatalf("unreachable catalog lost its cache: %+v", states[0])
	}
	// A cache older than a day is read as it is by Cached and Stage, and
	// fetched again by a listing; the failure is a warning.
	now = now.Add(25 * time.Hour)
	fetched = server.requests.Load()
	if states, _ = client.Cached(context.Background()); states[0].Index == nil || states[0].Warning != "" || server.requests.Load() != fetched {
		t.Fatalf("Cached fetched a stale cache: %+v (%d → %d)", states[0], fetched, server.requests.Load())
	}
	states, _ = client.Catalogs(context.Background(), false)
	if states[0].Index == nil || states[0].Warning == "" {
		t.Fatalf("stale cache not refetched: %+v", states[0])
	}
	// A cache edited on disk is not an index.
	if err := os.WriteFile(filepath.Join(client.Dir, "axlr", "index.json"), []byte(`{"schema_version":1,"catalog":"axlr","tools":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	states, _ = client.Catalogs(context.Background(), false)
	if states[0].Index != nil {
		t.Fatalf("edited cache read as an index: %+v", states[0])
	}
}

// A validly signed index generated before the cached one is refused and
// the cached one kept; the same time again, or a later one, is read. On 20
// October 2026 a review found that a replayed old index was accepted, and
// with it the versions the catalog had withdrawn since.
func TestClientRefusesAnIndexOlderThanTheCachedOne(t *testing.T) {
	server := serveCatalog(t)
	client := server.client(t)
	states, err := client.Catalogs(context.Background(), false)
	if err != nil || states[0].Index == nil || states[0].Warning != "" {
		t.Fatalf("states = %+v, %v", states, err)
	}
	current := states[0].Index.GeneratedAt
	cachedPath := filepath.Join(client.Dir, "axlr", "index.json")
	cachedBytes, err := os.ReadFile(cachedPath)
	if err != nil {
		t.Fatal(err)
	}
	original := string(server.index)
	serve := func(generatedAt string) {
		server.index = []byte(strings.Replace(original, `"generated_at": "`+current+`"`, `"generated_at": "`+generatedAt+`"`, 1))
		if generatedAt != current && string(server.index) == original {
			t.Fatalf("the index names no generated_at %s", current)
		}
		server.signature = Sign(server.private, server.key.ID, server.index, "timestamp:1760000000\tfile:index.json", true)
	}
	serve("2026-01-01T00:00:00Z")
	states, _ = client.Catalogs(context.Background(), true)
	if states[0].Index == nil || states[0].Index.GeneratedAt != current || !strings.Contains(states[0].Warning, "before the cached one") {
		t.Fatalf("older index accepted: %+v", states[0])
	}
	if now, err := os.ReadFile(cachedPath); err != nil || string(now) != string(cachedBytes) {
		t.Fatalf("the cache changed: %v", err)
	}
	serve(current)
	if states, _ = client.Catalogs(context.Background(), true); states[0].Index == nil || states[0].Warning != "" {
		t.Fatalf("the same index again refused: %+v", states[0])
	}
	serve("2099-01-01T00:00:00Z")
	if states, _ = client.Catalogs(context.Background(), true); states[0].Index == nil || states[0].Warning != "" || states[0].Index.GeneratedAt != "2099-01-01T00:00:00Z" {
		t.Fatalf("newer index refused: %+v", states[0])
	}
}

// A catalog without a key, with a bad key or off https is never read.
func TestClientRefusesKeylessAndInsecureCatalogs(t *testing.T) {
	server := serveCatalog(t)
	client := server.client(t)
	client.Entries = append(client.Entries, Catalog{Name: "nokey", URL: server.URL + "/index.json"}, Catalog{Name: "badkey", URL: server.URL + "/index.json", PublicKey: "RWQ"}, Catalog{Name: "plain", URL: "http://catalog.example/index.json", PublicKey: server.key.String()})
	states, err := client.Catalogs(context.Background(), false)
	if err != nil || len(states) != 4 {
		t.Fatalf("states = %+v, %v", states, err)
	}
	for i, want := range []string{"", "no public key configured", "minisign public key must be", "must use https"} {
		if !strings.Contains(states[i].Warning, want) || (want != "" && states[i].Index != nil) {
			t.Fatalf("catalog %s: %+v", states[i].Name, states[i])
		}
	}
	if server.requests.Load() != 2 {
		t.Fatalf("%d requests for one readable catalog", server.requests.Load())
	}
}

// Staging fetches the manifest and the files and verifies each against
// the signed index; a file that differs from its digest, or a manifest
// that does, refuses the install, and the program is checked on PATH.
func TestClientStagesVerifiedFiles(t *testing.T) {
	server := serveCatalog(t)
	client := server.client(t)
	staged, err := client.Stage(context.Background(), "axlr", "seat_allocation", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if staged.Tool.Name != "seat_allocation" || staged.Tool.Program != "python3" || len(staged.Files) != 1 || staged.Files[0].Path != "seat_allocation.py" || !strings.Contains(staged.Files[0].Content, "def ") || staged.IndexGeneratedAt == "" || len(staged.Warnings) != 0 {
		t.Fatalf("staged = %+v", staged)
	}
	if staged.Entry.Files["seat_allocation.py"] == "" || len(staged.Tool.Args) != 1 || staged.Tool.Args[0] != "seat_allocation.py" {
		t.Fatalf("entry = %+v, args %v", staged.Entry, staged.Tool.Args)
	}
	server.overrides["/tools/politics_civics/seat_allocation/seat_allocation.py"] = append([]byte(staged.Files[0].Content), "\nimport os\n"...)
	if _, err := client.Stage(context.Background(), "axlr", "seat_allocation", "1.0.0"); err == nil || !strings.Contains(err.Error(), "does not match the digest") {
		t.Fatalf("tampered file staged: %v", err)
	}
	delete(server.overrides, "/tools/politics_civics/seat_allocation/seat_allocation.py")
	server.overrides["/tools/politics_civics/seat_allocation/manifest.json"] = []byte(`{"name":"seat_allocation","files":["seat_allocation.py"],"catalog":{"version":"1.0.0"}}`)
	if _, err := client.Stage(context.Background(), "axlr", "seat_allocation", "1.0.0"); err == nil || !strings.Contains(err.Error(), "manifest") {
		t.Fatalf("tampered manifest staged: %v", err)
	}
	delete(server.overrides, "/tools/politics_civics/seat_allocation/manifest.json")
	if _, err := client.Stage(context.Background(), "axlr", "seat_allocation", "2.0.0"); err == nil {
		t.Fatal("unknown version staged")
	}
	client.LookPath = func(string) (string, error) { return "", errors.New("not found") }
	staged, err = client.Stage(context.Background(), "axlr", "business_days", "1.0.0")
	if err != nil || len(staged.Warnings) != 1 || !strings.Contains(staged.Warnings[0], "python3 is not on PATH") {
		t.Fatalf("staged = %+v, %v", staged, err)
	}
	client.LookPath = func(string) (string, error) { return "/usr/bin/python3", nil }
	client.Probe = func(_ context.Context, program string, args ...string) error {
		return errors.New("ModuleNotFoundError")
	}
	index := strings.Replace(string(server.index), `"packages": []`, `"packages": ["numpy"]`, 1)
	server.index = []byte(index)
	server.signature = Sign(server.private, server.key.ID, server.index, "timestamp:1760000001\tfile:index.json", false)
	client.Dir = filepath.Join(t.TempDir(), "fresh")
	staged, err = client.Stage(context.Background(), "axlr", "business_days", "1.0.0")
	if err != nil || len(staged.Warnings) != 1 || !strings.Contains(staged.Warnings[0], "numpy is not importable") {
		t.Fatalf("staged = %+v, %v", staged, err)
	}
}

func TestSafeRelativeAndIndexBase(t *testing.T) {
	for _, bad := range []string{"", "/etc/passwd", "../x", "tools/../../x", "tools/./x", "http://x/y", "tools//x", "./x", "a\\b"} {
		if safeRelative(bad) == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
	if safeRelative("tools/politics_civics/seat_allocation/manifest.json") != nil {
		t.Fatal("a clean path refused")
	}
	base, err := indexBase("https://catalog.axlr.dev/v1/index.json?x=1")
	if err != nil || base != "https://catalog.axlr.dev/v1/" {
		t.Fatalf("base = %q, %v", base, err)
	}
	if err := allowedURL("http://127.0.0.1:8080/index.json"); err != nil {
		t.Fatal(err)
	}
	if err := allowedURL("http://10.0.0.1/index.json"); err == nil {
		t.Fatal("http off the loopback allowed")
	}
}
