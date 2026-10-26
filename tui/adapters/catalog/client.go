// Package catalog reads the catalogs of pre-forged tools: a signed index
// per catalog, cached under the console's state, and the verified files
// of a tool to install.
package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

// Catalog is one configured catalog: its name, the URL of its index.json,
// its minisign public key and, when it takes uploads, the URL of its
// registry's publish API. A catalog without a key is never fetched.
type Catalog struct {
	Name, URL, PublicKey, APIURL string
}

// Bounds of what the client reads: an index, a manifest, one file.
const (
	maxIndexBytes    = 4 << 20
	maxManifestBytes = 64 << 10
	maxFileBytes     = 64 << 10
	// DefaultMaxAge is how old a cached index may be before a listing
	// fetches it again.
	DefaultMaxAge = 24 * time.Hour
	fetchTimeout  = 30 * time.Second
)

var (
	catalogName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	pythonName  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.]*$`)
	nodeName    = regexp.MustCompile(`^[@A-Za-z0-9_./-]+$`)
)

// Client is the console's catalog reader; it satisfies
// application.CatalogPort.
type Client struct {
	// Entries are the configured catalogs. Dir holds one directory per catalog with its verified index, its
	// signature and when it was fetched; owner-only.
	Dir     string
	Entries []Catalog
	HTTP    *http.Client
	// Now stamps the cache; nil uses time.Now. MaxAge zero is
	// DefaultMaxAge.
	Now    func() time.Time
	MaxAge time.Duration
	// LookPath finds a tool's program; nil uses exec.LookPath. Probe runs
	// a program to check a package it needs; nil runs it with exec.
	LookPath func(string) (string, error)
	Probe    func(ctx context.Context, program string, args ...string) error
	mu       sync.Mutex
}

var _ application.CatalogPort = (*Client)(nil)

// catalogCache is what the cache directory holds for one catalog.
type catalogCache struct {
	index     application.CatalogIndex
	raw       []byte
	fetchedAt time.Time
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) maxAge() time.Duration {
	if c.MaxAge > 0 {
		return c.MaxAge
	}
	return DefaultMaxAge
}

// Catalogs implements application.CatalogPort.
func (c *Client) Catalogs(ctx context.Context, refresh bool) ([]application.CatalogState, error) {
	return c.read(ctx, refresh, true)
}

// Cached implements application.CatalogPort: the cache alone, fetched
// only for a catalog never fetched.
func (c *Client) Cached(ctx context.Context) ([]application.CatalogState, error) {
	return c.read(ctx, false, false)
}

// read lists the catalogs: refresh fetches every index, fetchStale the
// ones older than MaxAge, and one never fetched is fetched either way.
func (c *Client) read(ctx context.Context, refresh, fetchStale bool) ([]application.CatalogState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	states := make([]application.CatalogState, 0, len(c.Entries))
	for _, entry := range c.Entries {
		if err := ctx.Err(); err != nil {
			return states, err
		}
		state := application.CatalogState{Name: entry.Name, URL: entry.URL, APIURL: entry.APIURL}
		key, err := c.key(entry)
		if err != nil {
			state.Warning = err.Error()
			states = append(states, state)
			continue
		}
		cached, cacheErr := c.readCache(entry, key)
		if cacheErr == nil {
			index := cached.index
			state.Index, state.FetchedAt = &index, cached.fetchedAt
		}
		stale := cacheErr != nil || refresh || (fetchStale && c.now().Sub(cached.fetchedAt) > c.maxAge())
		if stale {
			fresh, err := c.fetchIndex(ctx, entry, key, cached.index.GeneratedAt)
			if err != nil {
				state.Warning = err.Error()
			} else {
				index := fresh.index
				state.Index, state.FetchedAt, state.Warning = &index, fresh.fetchedAt, ""
			}
		}
		states = append(states, state)
	}
	return states, nil
}

// Stage implements application.CatalogPort.
func (c *Client) Stage(ctx context.Context, catalog, name, version string) (application.StagedTool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var entry Catalog
	found := false
	for _, candidate := range c.Entries {
		if candidate.Name == catalog {
			entry, found = candidate, true
		}
	}
	if !found {
		return application.StagedTool{}, fmt.Errorf("no catalog is named %s", catalog)
	}
	key, err := c.key(entry)
	if err != nil {
		return application.StagedTool{}, err
	}
	cached, err := c.readCache(entry, key)
	if err != nil {
		// Never fetched: the first read. A stale cache is kept as it is,
		// so the card and the install that follows see one index.
		if cached, err = c.fetchIndex(ctx, entry, key, ""); err != nil {
			return application.StagedTool{}, err
		}
	}
	var tool application.CatalogEntry
	found = false
	for _, candidate := range cached.index.Tools {
		if candidate.Name == name && candidate.Version == version {
			tool, found = candidate, true
		}
	}
	if !found {
		return application.StagedTool{}, fmt.Errorf("catalog %s has no %s %s", catalog, name, version)
	}
	base, err := indexBase(entry.URL)
	if err != nil {
		return application.StagedTool{}, err
	}
	if err := safeRelative(tool.Manifest); err != nil {
		return application.StagedTool{}, fmt.Errorf("catalog %s: manifest path of %s %s: %v", catalog, name, version, err)
	}
	manifestBytes, err := c.fetch(ctx, base+tool.Manifest, maxManifestBytes)
	if err != nil {
		return application.StagedTool{}, fmt.Errorf("catalog %s: manifest of %s %s: %v", catalog, name, version, err)
	}
	if sum := sha256.Sum256(manifestBytes); hex.EncodeToString(sum[:]) != strings.ToLower(tool.ManifestSHA256) {
		return application.StagedTool{}, fmt.Errorf("catalog %s: the manifest of %s %s does not match the digest the signed index carries; the install is refused", catalog, name, version)
	}
	var manifest struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"input_schema"`
		Program     string          `json:"program"`
		Args        []string        `json:"args"`
		Files       []string        `json:"files"`
		Catalog     struct {
			Version string `json:"version"`
		} `json:"catalog"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return application.StagedTool{}, fmt.Errorf("catalog %s: the manifest of %s %s is not JSON: %v", catalog, name, version, err)
	}
	if manifest.Name != name || manifest.Catalog.Version != version {
		return application.StagedTool{}, fmt.Errorf("catalog %s: the manifest at %s is %s %s, not %s %s", catalog, tool.Manifest, manifest.Name, manifest.Catalog.Version, name, version)
	}
	if len(manifest.Files) == 0 || len(manifest.Files) != len(tool.Files) {
		return application.StagedTool{}, fmt.Errorf("catalog %s: the manifest of %s %s lists %d files and the index %d", catalog, name, version, len(manifest.Files), len(tool.Files))
	}
	dir := path.Dir(tool.Manifest)
	files := make([]application.ForgedFile, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		if err := safeRelative(file); err != nil {
			return application.StagedTool{}, fmt.Errorf("catalog %s: file path %q of %s %s: %v", catalog, file, name, version, err)
		}
		want, listed := tool.Files[file]
		if !listed {
			return application.StagedTool{}, fmt.Errorf("catalog %s: file %s of %s %s is not in the signed index; the install is refused", catalog, file, name, version)
		}
		content, err := c.fetch(ctx, base+path.Join(dir, file), maxFileBytes)
		if err != nil {
			return application.StagedTool{}, fmt.Errorf("catalog %s: file %s of %s %s: %v", catalog, file, name, version, err)
		}
		if sum := sha256.Sum256(content); hex.EncodeToString(sum[:]) != strings.ToLower(want) {
			return application.StagedTool{}, fmt.Errorf("catalog %s: file %s of %s %s does not match the digest the signed index carries; the install is refused", catalog, file, name, version)
		}
		files = append(files, application.ForgedFile{Path: file, Content: string(content)})
	}
	staged := application.StagedTool{
		Catalog:          catalog,
		Entry:            tool,
		IndexGeneratedAt: cached.index.GeneratedAt,
		Tool:             application.ForgedTool{Name: manifest.Name, Description: manifest.Description, InputSchema: manifest.InputSchema, Program: manifest.Program, Args: manifest.Args},
		Files:            files,
		Warnings:         c.warnings(ctx, tool),
	}
	return staged, nil
}

// warnings says what the machine lacks for a tool: its program on PATH,
// the packages it needs.
func (c *Client) warnings(ctx context.Context, tool application.CatalogEntry) []string {
	lookPath := c.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	probe := c.Probe
	if probe == nil {
		probe = func(ctx context.Context, program string, args ...string) error {
			ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, program, args...)
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
			return cmd.Run()
		}
	}
	var warnings []string
	if tool.Program == "" {
		return warnings
	}
	if _, err := lookPath(tool.Program); err != nil {
		warnings = append(warnings, fmt.Sprintf("program %s is not on PATH; the tool will not run until it is installed", tool.Program))
		return warnings
	}
	for _, pkg := range tool.Requires.Packages {
		switch {
		case strings.HasPrefix(tool.Program, "python") && pythonName.MatchString(pkg):
			if probe(ctx, tool.Program, "-c", "import "+pkg) != nil {
				warnings = append(warnings, fmt.Sprintf("package %s is not importable by %s; the tool needs it", pkg, tool.Program))
			}
		case tool.Program == "node" && nodeName.MatchString(pkg):
			if probe(ctx, tool.Program, "-e", "require("+jsonString(pkg)+")") != nil {
				warnings = append(warnings, fmt.Sprintf("package %s is not installed for node; the tool needs it", pkg))
			}
		default:
			warnings = append(warnings, fmt.Sprintf("the tool needs package %s for %s, which the console cannot check", pkg, tool.Program))
		}
	}
	return warnings
}

func jsonString(s string) string {
	encoded, _ := json.Marshal(s)
	return string(encoded)
}

// key parses a catalog's public key.
func (c *Client) key(entry Catalog) (PublicKey, error) {
	if !catalogName.MatchString(entry.Name) {
		return PublicKey{}, fmt.Errorf("catalog name %q is not lowercase letters, digits, _ or -", entry.Name)
	}
	if strings.TrimSpace(entry.PublicKey) == "" {
		return PublicKey{}, fmt.Errorf("catalog %s has no public key configured; set catalogs[].public_key in settings.json (an unsigned index is never read)", entry.Name)
	}
	key, err := ParsePublicKey(entry.PublicKey)
	if err != nil {
		return PublicKey{}, fmt.Errorf("catalog %s: %v", entry.Name, err)
	}
	return key, nil
}

func (c *Client) cacheDir(entry Catalog) string {
	return filepath.Join(c.Dir, entry.Name)
}

// readCache reads a catalog's cached index and verifies it again against
// the key, so a cache changed on disk is not an index.
func (c *Client) readCache(entry Catalog, key PublicKey) (catalogCache, error) {
	if !filepath.IsAbs(c.Dir) {
		return catalogCache{}, errors.New("catalog cache directory must be absolute")
	}
	dir := c.cacheDir(entry)
	raw, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		return catalogCache{}, err
	}
	signature, err := os.ReadFile(filepath.Join(dir, "index.json.minisig"))
	if err != nil {
		return catalogCache{}, err
	}
	if err := Verify(key, raw, signature); err != nil {
		return catalogCache{}, fmt.Errorf("cached index of catalog %s: %v", entry.Name, err)
	}
	index, err := parseIndex(entry, raw)
	if err != nil {
		return catalogCache{}, err
	}
	fetchedAt := time.Time{}
	if stamp, err := os.ReadFile(filepath.Join(dir, "fetched_at")); err == nil {
		fetchedAt, _ = time.Parse(time.RFC3339, strings.TrimSpace(string(stamp)))
	}
	return catalogCache{index: index, raw: raw, fetchedAt: fetchedAt}, nil
}

// fetchIndex downloads, verifies and caches a catalog's index; on any
// failure the cache is left as it was. cachedAt is the generated_at of the
// cached index ("" without one): an index generated before it is refused.
// On 20 October 2026 a review found that any validly signed index was
// accepted, so a replayed old one, from an attacker or a stale mirror,
// could bring back versions the catalog had withdrawn. The cache's
// fetched_at is not renewed by a refusal, so the next listing tries again.
func (c *Client) fetchIndex(ctx context.Context, entry Catalog, key PublicKey, cachedAt string) (catalogCache, error) {
	if !filepath.IsAbs(c.Dir) {
		return catalogCache{}, errors.New("catalog cache directory must be absolute")
	}
	raw, err := c.fetch(ctx, entry.URL, maxIndexBytes)
	if err != nil {
		return catalogCache{}, fmt.Errorf("catalog %s: index: %v", entry.Name, err)
	}
	signature, err := c.fetch(ctx, entry.URL+".minisig", 4096)
	if err != nil {
		return catalogCache{}, fmt.Errorf("catalog %s: index signature: %v", entry.Name, err)
	}
	if err := Verify(key, raw, signature); err != nil {
		return catalogCache{}, fmt.Errorf("catalog %s: %v; the cached index, if any, is kept", entry.Name, err)
	}
	index, err := parseIndex(entry, raw)
	if err != nil {
		return catalogCache{}, err
	}
	if olderIndex(index.GeneratedAt, cachedAt) {
		return catalogCache{}, fmt.Errorf("catalog %s: the index served was generated at %s, before the cached one (%s); an older index is refused and the cached one is kept", entry.Name, index.GeneratedAt, cachedAt)
	}
	dir := c.cacheDir(entry)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return catalogCache{}, err
	}
	now := c.now().UTC()
	for name, data := range map[string][]byte{"index.json": raw, "index.json.minisig": signature, "fetched_at": []byte(now.Format(time.RFC3339) + "\n")} {
		if err := writeOwnerFile(filepath.Join(dir, name), data); err != nil {
			return catalogCache{}, err
		}
	}
	return catalogCache{index: index, raw: raw, fetchedAt: now}, nil
}

// olderIndex reports whether an index generated at fetched predates the
// cached one generated at cached; an equal time is the same index served
// again. A time that does not parse as RFC 3339 orders nothing.
func olderIndex(fetched, cached string) bool {
	if cached == "" {
		return false
	}
	fresh, freshErr := time.Parse(time.RFC3339, fetched)
	known, knownErr := time.Parse(time.RFC3339, cached)
	return freshErr == nil && knownErr == nil && fresh.Before(known)
}

func parseIndex(entry Catalog, raw []byte) (application.CatalogIndex, error) {
	var index application.CatalogIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		return application.CatalogIndex{}, fmt.Errorf("catalog %s: index is not JSON: %v", entry.Name, err)
	}
	if index.SchemaVersion != 1 {
		return application.CatalogIndex{}, fmt.Errorf("catalog %s: index schema version %d is not 1", entry.Name, index.SchemaVersion)
	}
	return index, nil
}

// writeOwnerFile replaces path with data through a temporary file in its
// directory, owner-only.
func writeOwnerFile(target string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(target), ".catalog-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), target)
}

// indexBase is the directory of the index URL, with a trailing slash,
// that manifest and file paths are relative to.
func indexBase(address string) (string, error) {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" {
		return "", fmt.Errorf("catalog URL %q is not an absolute URL", address)
	}
	parsed.RawQuery, parsed.Fragment = "", ""
	parsed.Path = path.Dir(parsed.Path)
	if !strings.HasSuffix(parsed.Path, "/") {
		parsed.Path += "/"
	}
	return parsed.String(), nil
}

// safeRelative accepts a clean relative path below the index: no scheme,
// no absolute path, no parent segment.
func safeRelative(p string) error {
	if p == "" || path.IsAbs(p) || strings.Contains(p, "\\") || strings.Contains(p, "://") || strings.HasPrefix(p, "./") || path.Clean(p) != p {
		return errors.New("must be a clean relative path")
	}
	for _, segment := range strings.Split(p, "/") {
		if segment == ".." || segment == "." || segment == "" {
			return errors.New("must not leave the catalog")
		}
	}
	return nil
}

// fetch reads a catalog URL: https anywhere, http only on a loopback host
// (a local test server, an intranet mirror on this machine).
func (c *Client) fetch(ctx context.Context, address string, limit int64) ([]byte, error) {
	if err := allowedURL(address); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AXLR-catalog")
	client := c.HTTP
	if client == nil {
		client = &http.Client{}
	}
	secured := *client
	secured.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := allowedURL(req.URL.String()); err != nil {
			return err
		}
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		return nil
	}
	response, err := secured.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot fetch %s: network request failed", address)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned HTTP %d", address, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", address, limit)
	}
	return data, nil
}

// allowedURL is the fetch rule: https, or http on a loopback host.
func allowedURL(address string) error {
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("catalog URL %q is not an absolute URL", address)
	}
	switch parsed.Scheme {
	case "https":
		return nil
	case "http":
		host := parsed.Hostname()
		if host == "localhost" || strings.HasSuffix(host, ".localhost") {
			return nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil
		}
		return fmt.Errorf("catalog URL %s must use https off the loopback", address)
	default:
		return fmt.Errorf("catalog URL %s must use https", address)
	}
}

// maxSkillFileBytes bounds one file of a skill, as the registry does.
const maxSkillFileBytes = application.MaxSkillFileBytes

var _ application.SkillStagePort = (*Client)(nil)

// StageSkill implements application.SkillStagePort: the skill's manifest
// and files, each checked against the digests the cached signed index
// carries.
func (c *Client) StageSkill(ctx context.Context, catalog, name, version string) (application.StagedSkill, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var entry Catalog
	found := false
	for _, candidate := range c.Entries {
		if candidate.Name == catalog {
			entry, found = candidate, true
		}
	}
	if !found {
		return application.StagedSkill{}, fmt.Errorf("no catalog is named %s", catalog)
	}
	key, err := c.key(entry)
	if err != nil {
		return application.StagedSkill{}, err
	}
	cached, err := c.readCache(entry, key)
	if err != nil {
		if cached, err = c.fetchIndex(ctx, entry, key, ""); err != nil {
			return application.StagedSkill{}, err
		}
	}
	var skill application.CatalogSkillEntry
	found = false
	for _, candidate := range cached.index.Skills {
		if candidate.Name == name && candidate.Version == version {
			skill, found = candidate, true
		}
	}
	if !found {
		return application.StagedSkill{}, fmt.Errorf("catalog %s has no skill %s %s", catalog, name, version)
	}
	base, err := indexBase(entry.URL)
	if err != nil {
		return application.StagedSkill{}, err
	}
	if err := safeRelative(skill.Manifest); err != nil {
		return application.StagedSkill{}, fmt.Errorf("catalog %s: manifest path of skill %s %s: %v", catalog, name, version, err)
	}
	manifestBytes, err := c.fetch(ctx, base+skill.Manifest, maxManifestBytes)
	if err != nil {
		return application.StagedSkill{}, fmt.Errorf("catalog %s: manifest of skill %s %s: %v", catalog, name, version, err)
	}
	if sum := sha256.Sum256(manifestBytes); hex.EncodeToString(sum[:]) != strings.ToLower(skill.ManifestSHA256) {
		return application.StagedSkill{}, fmt.Errorf("catalog %s: the manifest of skill %s %s does not match the digest the signed index carries; the install is refused", catalog, name, version)
	}
	var manifest struct {
		Kind        string   `json:"kind"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Files       []string `json:"files"`
		Tools       []string `json:"tools"`
		Catalog     struct {
			Version string `json:"version"`
		} `json:"catalog"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return application.StagedSkill{}, fmt.Errorf("catalog %s: the manifest of skill %s %s is not JSON: %v", catalog, name, version, err)
	}
	if manifest.Kind != "skill" {
		return application.StagedSkill{}, fmt.Errorf("catalog %s: the manifest at %s is not a skill's", catalog, skill.Manifest)
	}
	if manifest.Name != name || manifest.Catalog.Version != version {
		return application.StagedSkill{}, fmt.Errorf("catalog %s: the manifest at %s is %s %s, not %s %s", catalog, skill.Manifest, manifest.Name, manifest.Catalog.Version, name, version)
	}
	if len(manifest.Files) == 0 || len(manifest.Files) != len(skill.Files) {
		return application.StagedSkill{}, fmt.Errorf("catalog %s: the manifest of skill %s %s lists %d files and the index %d", catalog, name, version, len(manifest.Files), len(skill.Files))
	}
	dir := path.Dir(skill.Manifest)
	files := make([]application.SkillFile, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		if err := safeRelative(file); err != nil || strings.Contains(file, "/") {
			return application.StagedSkill{}, fmt.Errorf("catalog %s: file %q of skill %s %s is not a flat name", catalog, file, name, version)
		}
		want, listed := skill.Files[file]
		if !listed {
			return application.StagedSkill{}, fmt.Errorf("catalog %s: file %s of skill %s %s is not in the signed index; the install is refused", catalog, file, name, version)
		}
		content, err := c.fetch(ctx, base+path.Join(dir, file), maxSkillFileBytes)
		if err != nil {
			return application.StagedSkill{}, fmt.Errorf("catalog %s: file %s of skill %s %s: %v", catalog, file, name, version, err)
		}
		if sum := sha256.Sum256(content); hex.EncodeToString(sum[:]) != strings.ToLower(want) {
			return application.StagedSkill{}, fmt.Errorf("catalog %s: file %s of skill %s %s does not match the digest the signed index carries; the install is refused", catalog, file, name, version)
		}
		files = append(files, application.SkillFile{Path: file, Content: content})
	}
	return application.StagedSkill{
		Catalog:          catalog,
		Entry:            skill,
		IndexGeneratedAt: cached.index.GeneratedAt,
		Description:      strings.TrimSpace(manifest.Description),
		Tools:            append([]string{}, manifest.Tools...),
		Files:            files,
		Warnings:         append([]string(nil), skill.Warnings...),
	}, nil
}
