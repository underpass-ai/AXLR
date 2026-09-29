package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeManifest(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plugin.json")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadManifestAcceptsExplicitTool(t *testing.T) {
	m, err := LoadManifest(writeManifest(t, `{"manifest_version":1,"id":"search","command":"/bin/echo","args":["x"],"allow_tools":["find"]}`))
	if err != nil || m.ID.String() != "search" || len(m.AllowTools) != 1 || m.AllowTools[0].String() != "find" {
		t.Fatalf("manifest: %#v, %v", m, err)
	}
	r, err := NewRegistration(m, nil)
	if err != nil || r.Env == nil || len(r.Env) != 0 {
		t.Fatalf("child env inherited: %#v, %v", r.Env, err)
	}
}

func TestLoadManifestRejectsMalformedRegistration(t *testing.T) {
	base := `{"manifest_version":1,"id":"search","command":"/bin/echo","args":[],"allow_tools":["find"]}`
	for name, body := range map[string]string{
		"unknown field":       strings.Replace(base, `"args":[]`, `"extra":true,"args":[]`, 1),
		"duplicate field":     strings.Replace(base, `"id":"search"`, `"id":"search","id":"other"`, 1),
		"wrong version":       strings.Replace(base, `"manifest_version":1`, `"manifest_version":2`, 1),
		"relative command":    strings.Replace(base, `/bin/echo`, `echo`, 1),
		"empty allowlist":     strings.Replace(base, `["find"]`, `[]`, 1),
		"duplicate allowlist": strings.Replace(base, `["find"]`, `["find","find"]`, 1),
		"nul argument":        strings.Replace(base, `"args":[]`, `"args":["\u0000"]`, 1),
		"nul command":         strings.Replace(base, `/bin/echo`, `/bin/\u0000`, 1),
		"invalid id":          strings.Replace(base, `"id":"search"`, `"id":"bad.id"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadManifest(writeManifest(t, body)); err == nil {
				t.Fatal("accepted invalid manifest")
			}
		})
	}
	if _, err := LoadManifest(writeManifest(t, base+strings.Repeat(" ", 65537))); err == nil {
		t.Fatal("accepted manifest over 64 KiB")
	}
}

func TestNewRegistrationRejectsInvalidEnvironment(t *testing.T) {
	m, err := LoadManifest(writeManifest(t, `{"manifest_version":1,"id":"search","command":"/bin/echo","args":[],"allow_tools":["find"]}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, env := range [][]string{{""}, {"NO_EQUALS"}, {"=x"}, {"A=\x00"}, {"A=1", "A=2"}} {
		if _, err := NewRegistration(m, env); err == nil {
			t.Errorf("accepted environment %#v", env)
		}
	}
}
