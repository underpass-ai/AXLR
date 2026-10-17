package storage

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"

	"github.com/underpass-ai/AXLR/tui/adapters/catalog"
)

// CatalogSetting is one entry of catalogs: a catalog of pre-forged tools
// the console reads (docs/plans/2026-10-16-tool-registry-design.md,
// decision D1). The official catalog, OfficialCatalog, is built in: an
// entry named after it overrides its URL or key (a mirror, the key once
// published) or disables it with enabled false; it cannot be removed.
type CatalogSetting struct {
	Name      string `json:"name"`
	URL       string `json:"url,omitempty"`
	PublicKey string `json:"public_key,omitempty"`
	// APIURL is the catalog's registry, where /tools enroll and /tools
	// publish send signed requests; absent for a catalog that takes none.
	APIURL  string `json:"api_url,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

// OfficialCatalogPublicKey is the minisign public key of the official
// catalog. It is published with the catalog's first release; until then
// it is empty and the official entry is listed but never read, unless
// settings give the key.
const OfficialCatalogPublicKey = ""

// OfficialCatalog is the built-in entry.
var OfficialCatalog = CatalogSetting{Name: "axlr", URL: "https://catalog.axlr.dev/index.json", PublicKey: OfficialCatalogPublicKey}

const maxCatalogs = 16

var catalogSettingName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// IsEnabled reports whether the entry is read: on unless enabled is false.
func (c CatalogSetting) IsEnabled() bool { return c.Enabled == nil || *c.Enabled }

func (c CatalogSetting) validate() error {
	if !catalogSettingName.MatchString(c.Name) {
		return fmt.Errorf("settings catalogs name %q must be 1-32 lowercase letters, digits, _ or -, starting with a letter", c.Name)
	}
	official := c.Name == OfficialCatalog.Name
	if c.URL == "" && !official {
		return fmt.Errorf("settings catalogs %s needs a url", c.Name)
	}
	if c.URL != "" {
		if err := catalogURL(c.URL); err != nil {
			return fmt.Errorf("settings catalogs %s: %v", c.Name, err)
		}
	}
	if c.PublicKey == "" && !official {
		return fmt.Errorf("settings catalogs %s needs a public_key: an unsigned index is never read", c.Name)
	}
	if c.PublicKey != "" {
		if _, err := catalog.ParsePublicKey(c.PublicKey); err != nil {
			return fmt.Errorf("settings catalogs %s: %v", c.Name, err)
		}
	}
	if c.APIURL != "" {
		if err := registryURL(c.APIURL); err != nil {
			return fmt.Errorf("settings catalogs %s: %v", c.Name, err)
		}
	}
	return nil
}

// registryURL accepts an https origin of a registry's publish API, or
// http on this machine's loopback.
func registryURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("api_url must be an absolute URL without a query")
	}
	host := parsed.Hostname()
	loopback := host == "localhost" || strings.HasSuffix(host, ".localhost")
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		loopback = true
	}
	switch {
	case parsed.Scheme == "https", parsed.Scheme == "http" && loopback:
		return nil
	default:
		return errors.New("api_url must use https (http only on the loopback)")
	}
}

// catalogURL accepts an https URL of an index.json, or http on this
// machine's loopback.
func catalogURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || !strings.HasSuffix(parsed.Path, ".json") {
		return errors.New("url must be an absolute URL of an index.json")
	}
	host := parsed.Hostname()
	loopback := host == "localhost" || strings.HasSuffix(host, ".localhost")
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		loopback = true
	}
	switch {
	case parsed.Scheme == "https", parsed.Scheme == "http" && loopback:
		return nil
	default:
		return errors.New("url must use https (http only on the loopback)")
	}
}

func (s UserSettings) validateCatalogs() error {
	if len(s.Catalogs) > maxCatalogs {
		return fmt.Errorf("settings catalogs lists more than %d catalogs", maxCatalogs)
	}
	seen := map[string]bool{}
	for _, entry := range s.Catalogs {
		if err := entry.validate(); err != nil {
			return err
		}
		if seen[entry.Name] {
			return fmt.Errorf("settings catalogs lists %s twice", entry.Name)
		}
		seen[entry.Name] = true
	}
	return nil
}

// EffectiveCatalogs is the official catalog, as settings override it,
// followed by the configured ones, without those disabled; a nil result
// means every catalog is disabled.
func (s UserSettings) EffectiveCatalogs() []catalog.Catalog {
	official := OfficialCatalog
	var out []catalog.Catalog
	var others []CatalogSetting
	for _, entry := range s.Catalogs {
		if entry.Name == OfficialCatalog.Name {
			if entry.URL != "" {
				official.URL = entry.URL
			}
			if entry.PublicKey != "" {
				official.PublicKey = entry.PublicKey
			}
			if entry.APIURL != "" {
				official.APIURL = entry.APIURL
			}
			official.Enabled = entry.Enabled
			continue
		}
		others = append(others, entry)
	}
	if official.IsEnabled() {
		out = append(out, catalog.Catalog{Name: official.Name, URL: official.URL, PublicKey: official.PublicKey, APIURL: official.APIURL})
	}
	for _, entry := range others {
		if entry.IsEnabled() {
			out = append(out, catalog.Catalog{Name: entry.Name, URL: entry.URL, PublicKey: entry.PublicKey, APIURL: entry.APIURL})
		}
	}
	return out
}
