package engineupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
)

type config struct {
	targets     []application.EngineUpdateTarget
	activations []string
	err         error
}

func (c *config) EngineTargets(context.Context) ([]application.EngineUpdateTarget, error) {
	return c.targets, c.err
}
func (c *config) ActivateEngine(_ context.Context, target application.EngineUpdateTarget, launcher string) error {
	if c.err != nil {
		return c.err
	}
	c.activations = append(c.activations, launcher)
	for i := range c.targets {
		if c.targets[i].Engine == target.Engine {
			c.targets[i].Command = launcher
		}
	}
	return nil
}

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func archive(t *testing.T, engine, version string, extra ...*tar.Header) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	files := []struct {
		name, body string
		mode       int64
	}{
		{engine + "/.claude-plugin/plugin.json", `{"version":"` + version + `"}`, 0600},
		{engine + "/bin/" + engine + "-mcp", "#!/bin/sh\necho '" + engine + "-mcp " + version + "'\n", 0700},
		{engine + "/scripts/run-embedded-mcp.sh", "#!/bin/sh\nexit 0\n", 0700},
	}
	for _, f := range files {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: f.mode, Size: int64(len(f.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f.body)); err != nil {
			t.Fatal(err)
		}
	}
	for _, h := range extra {
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func updater(t *testing.T, cfg *config) (*Updater, map[string][]byte) {
	t.Helper()
	responses := map[string][]byte{}
	for _, e := range []string{"made", "kmp"} {
		body := archive(t, e, "1.2.3")
		name := e + "-plugin-1.2.3-linux-arm64.tar.gz"
		address := "https://github.com/underpass-ai/" + e + "/releases/download/v1.2.3/" + name
		meta, _ := json.Marshal(map[string]any{"tag_name": "v1.2.3", "assets": []any{map[string]string{"name": name, "browser_download_url": address}, map[string]string{"name": name + ".sha256", "browser_download_url": address + ".sha256"}}})
		responses["https://api.github.com/repos/underpass-ai/"+e+"/releases/latest"] = meta
		responses[address] = body
		responses[address+".sha256"] = fmt.Appendf(nil, "%x  %s\n", sha256.Sum256(body), name)
	}
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		if err := r.Context().Err(); err != nil {
			return nil, err
		}
		body, ok := responses[r.URL.String()]
		status := 200
		if !ok {
			status = 404
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})}
	return &Updater{Configuration: cfg, Root: t.TempDir(), Client: client, platform: "linux-arm64"}, responses
}

func TestUpdateInstallsVerifiedPackagesAndKeepsPreviousExecutables(t *testing.T) {
	cfg := &config{targets: []application.EngineUpdateTarget{{Engine: "made", Command: "/missing/made"}, {Engine: "kmp", Command: "/missing/kmp"}}}
	u, _ := updater(t, cfg)
	results, err := u.Update(context.Background())
	if err != nil || len(results) != 2 || len(cfg.activations) != 2 {
		t.Fatalf("%+v %v", results, err)
	}
	for _, r := range results {
		if r.Status != "updated" || r.Version != "1.2.3" || !r.RestartRequired {
			t.Fatalf("%+v", r)
		}
	}
	previous := append([]string(nil), cfg.activations...)
	results, err = u.Update(context.Background())
	if err != nil || len(cfg.activations) != 2 {
		t.Fatalf("repeat update rewrote config: %+v %v", results, err)
	}
	for i, r := range results {
		if r.Status != "current" || r.RestartRequired || r.PreviousVersion != "1.2.3" {
			t.Fatalf("%+v", r)
		}
		if _, err := os.Stat(previous[i]); err != nil {
			t.Fatal("running executable was removed", err)
		}
	}
	u.ActiveCommands = map[string]string{"made": "/previous/made-launcher", "kmp": cfg.targets[1].Command}
	results, err = u.Update(context.Background())
	if err != nil || !results[0].RestartRequired || results[1].RestartRequired {
		t.Fatalf("repeat check lost pending restart: %+v %v", results, err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(u.Root, ".update-*"))
	if len(leftovers) != 0 {
		t.Fatal("staging files remain")
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(previous[0]), "run-embedded-mcp.sh"), []byte("changed"), 0700); err != nil {
		t.Fatal(err)
	}
	results, err = u.Update(context.Background())
	if err != nil || results[0].Status != "failed" || results[1].Status != "current" || len(cfg.activations) != 2 {
		t.Fatalf("modified cache or second engine mishandled: %+v %v", results, err)
	}
}

func TestChecksumFailureCannotActivateAndOtherEngineStillUpdates(t *testing.T) {
	cfg := &config{targets: []application.EngineUpdateTarget{{Engine: "made", Command: "/missing/made"}, {Engine: "kmp", Command: "/missing/kmp"}}}
	u, responses := updater(t, cfg)
	for address := range responses {
		if strings.Contains(address, "/made/") && strings.HasSuffix(address, ".sha256") {
			responses[address] = []byte(strings.Repeat("0", 64))
		}
	}
	results, err := u.Update(context.Background())
	if err != nil || results[0].Status != "failed" || !strings.Contains(results[0].Error, "checksum mismatch") || results[1].Status != "updated" || len(cfg.activations) != 1 {
		t.Fatalf("%+v %v", results, err)
	}
}

func TestInvalidReleasesAndNetworkFailuresKeepConfiguration(t *testing.T) {
	for _, scenario := range []string{"metadata", "prerelease", "missing", "checksum", "version", "http", "network", "activation"} {
		t.Run(scenario, func(t *testing.T) {
			cfg := &config{targets: []application.EngineUpdateTarget{{Engine: "made", Command: "/missing/made"}}}
			u, responses := updater(t, cfg)
			metaURL := "https://api.github.com/repos/underpass-ai/made/releases/latest"
			switch scenario {
			case "metadata":
				responses[metaURL] = []byte(`{"tag_name":"../../bad"}`)
			case "prerelease":
				responses[metaURL] = []byte(`{"tag_name":"v1.2.3","prerelease":true}`)
			case "missing":
				responses[metaURL] = []byte(`{"tag_name":"v1.2.3"}`)
			case "checksum":
				for address := range responses {
					if strings.Contains(address, "/made/") && strings.HasSuffix(address, "sha256") {
						responses[address] = []byte("bad checksum")
					}
				}
			case "version":
				for address := range responses {
					if strings.Contains(address, "/made/") && strings.HasSuffix(address, "tar.gz") {
						data := archive(t, "made", "1.2.4")
						responses[address] = data
						responses[address+".sha256"] = fmt.Appendf(nil, "%x", sha256.Sum256(data))
					}
				}
			case "http":
				delete(responses, metaURL)
			case "network":
				u.Client = &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) { return nil, errors.New("secret transport detail") })}
			case "activation":
				u.Client.Transport = transport(func(r *http.Request) (*http.Response, error) {
					cfg.err = errors.New("configuration changed")
					return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(responses[r.URL.String()]))}, nil
				})
			}
			results, err := u.Update(context.Background())
			if err != nil || results[0].Status != "failed" || len(cfg.activations) != 0 {
				t.Fatalf("%+v %v", results, err)
			}
			if strings.Contains(results[0].Error, "secret") {
				t.Fatal("transport details leaked")
			}
		})
	}
}

func TestSkipCancellationAndConfigurationFailure(t *testing.T) {
	u, _ := updater(t, &config{targets: []application.EngineUpdateTarget{{Engine: "made"}}})
	results, err := u.Update(context.Background())
	if err != nil || len(results) != 2 || results[0].Status != "skipped" || results[1].Status != "skipped" {
		t.Fatalf("%+v %v", results, err)
	}
	cfg := &config{targets: []application.EngineUpdateTarget{{Engine: "made", Command: "/missing/made"}, {Engine: "kmp", Command: "/missing/kmp"}}}
	u, _ = updater(t, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results, err = u.Update(ctx)
	if !errors.Is(err, context.Canceled) || len(cfg.activations) != 0 || len(results) != 1 {
		t.Fatalf("%+v %v", results, err)
	}
	cfg.err = errors.New("config unavailable")
	if _, err = u.Update(context.Background()); err == nil {
		t.Fatal("config error ignored")
	}
	u.Configuration = nil
	if _, err = u.Update(context.Background()); err == nil {
		t.Fatal("missing configuration accepted")
	}
}

func TestArchiveRejectsTraversalLinksAndDuplicates(t *testing.T) {
	for _, h := range []*tar.Header{
		{Name: "made/../../outside", Typeflag: tar.TypeReg},
		{Name: "other/file", Typeflag: tar.TypeReg},
		{Name: "made/bin/link", Typeflag: tar.TypeSymlink, Linkname: "/tmp/outside"},
		{Name: "made/bin/link", Typeflag: tar.TypeLink, Linkname: "made/bin/made-mcp"},
		{Name: "made/bin/made-mcp", Typeflag: tar.TypeReg},
		{Name: "made\\file", Typeflag: tar.TypeReg},
	} {
		if err := extract(archive(t, "made", "1.2.3", h), t.TempDir(), "made"); err == nil {
			t.Fatalf("unsafe archive accepted: %+v", h)
		}
	}
	if err := extract([]byte("not gzip"), t.TempDir(), "made"); err == nil {
		t.Fatal("invalid archive accepted")
	}
}

func TestVersionProbesAndDownloadBounds(t *testing.T) {
	for _, pair := range []struct {
		a, b string
		want int
	}{{"1.2.3", "1.2.3", 0}, {"1.2.3", "1.10.0", -1}, {"2.0.0", "1.9.9", 1}} {
		if compareVersions(pair.a, pair.b) != pair.want {
			t.Fatal(pair)
		}
	}
	u, _ := updater(t, &config{})
	if _, err := u.fetch(context.Background(), "https://example.com/package", 10); err == nil {
		t.Fatal("unofficial URL accepted")
	}
	u.Client = &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("too much data"))}, nil
	})}
	if _, err := u.fetch(context.Background(), "https://api.github.com/test", 2); err == nil {
		t.Fatal("oversized response accepted")
	}
	var output boundedOutput
	n, err := output.Write(bytes.Repeat([]byte("x"), 8000))
	if err != nil || n != 8000 || output.Len() != 4096 {
		t.Fatal("unbounded process output")
	}
}
