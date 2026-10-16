package storage

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/adapters/catalog"
)

func testCatalogKey(t *testing.T) string {
	t.Helper()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key := catalog.PublicKey{Key: public}
	copy(key.ID[:], []byte("settings"))
	return key.String()
}

// catalogs adds signed catalogs beside the official one, overrides or
// disables the official one, and refuses an unsigned or insecure entry.
func TestCatalogSettings(t *testing.T) {
	key := testCatalogKey(t)
	var s UserSettings
	if err := json.Unmarshal([]byte(`{"catalogs":[{"name":"acme","url":"https://tools.intranet.acme/index.json","public_key":"`+key+`"},{"name":"axlr","public_key":"`+key+`"}]}`), &s); err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, extra := s.Extra["catalogs"]; extra {
		t.Fatal("catalogs kept as an unknown key")
	}
	effective := s.EffectiveCatalogs()
	if len(effective) != 2 || effective[0].Name != "axlr" || effective[0].URL != OfficialCatalog.URL || effective[0].PublicKey != key || effective[1].Name != "acme" {
		t.Fatalf("effective = %+v", effective)
	}
	off := false
	s.Catalogs = []CatalogSetting{{Name: "axlr", Enabled: &off}}
	if err := s.Validate(); err != nil || len(s.EffectiveCatalogs()) != 0 {
		t.Fatalf("disabled official: %v, %+v", err, s.EffectiveCatalogs())
	}
	// Without settings the official entry is there, keyless until the
	// catalog publishes its key.
	if effective := (UserSettings{}).EffectiveCatalogs(); len(effective) != 1 || effective[0].Name != "axlr" || effective[0].PublicKey != OfficialCatalogPublicKey {
		t.Fatalf("default = %+v", effective)
	}
	for name, raw := range map[string]string{
		"no key":       `[{"name":"acme","url":"https://x/index.json"}]`,
		"bad key":      `[{"name":"acme","url":"https://x/index.json","public_key":"RWQ"}]`,
		"http":         `[{"name":"acme","url":"http://x/index.json","public_key":"` + key + `"}]`,
		"not an index": `[{"name":"acme","url":"https://x/","public_key":"` + key + `"}]`,
		"bad name":     `[{"name":"Acme","url":"https://x/index.json","public_key":"` + key + `"}]`,
		"no url":       `[{"name":"acme","public_key":"` + key + `"}]`,
		"twice":        `[{"name":"acme","url":"https://x/index.json","public_key":"` + key + `"},{"name":"acme","url":"https://y/index.json","public_key":"` + key + `"}]`,
	} {
		var s UserSettings
		if err := json.Unmarshal([]byte(`{"catalogs":`+raw+`}`), &s); err != nil {
			t.Fatal(err)
		}
		if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "catalogs") {
			t.Fatalf("%s accepted: %v", name, err)
		}
	}
	var loop UserSettings
	if err := json.Unmarshal([]byte(`{"catalogs":[{"name":"local","url":"http://127.0.0.1:8080/index.json","public_key":"`+key+`"}]}`), &loop); err != nil {
		t.Fatal(err)
	}
	if err := loop.Validate(); err != nil {
		t.Fatalf("loopback http refused: %v", err)
	}
}
