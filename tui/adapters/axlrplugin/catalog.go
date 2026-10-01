package axlrplugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

const maxPackageBytes = 64 << 20

var packageID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)
var skillID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

const maxSkillBytes = 1 << 20

// Catalog owns AXLR's plugin packages under Root. It never invokes Codex.
type Catalog struct {
	Root string
	MCP  application.PluginManagementPort
	mu   sync.Mutex
}

type manifest struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	MCPManifest string          `json:"mcp_manifest"`
	MCPServers  json.RawMessage `json:"mcpServers"`
	Skills      json.RawMessage `json:"skills"`
}

func (c *Catalog) base() (string, error) {
	if !filepath.IsAbs(c.Root) {
		return "", errors.New("AXLR plugin directory must be absolute")
	}
	return filepath.Join(c.Root, "plugins"), nil
}

func (c *Catalog) List(ctx context.Context, available bool) ([]application.InstalledPlugin, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	base, err := c.base()
	if err != nil {
		return nil, err
	}
	items := []application.InstalledPlugin{
		{ID: "kmp", Name: "KMP", Version: "default", Description: "Underpass graph-temporal memory", Components: []string{"mcp", "skills"}, Installed: true, Builtin: true},
		{ID: "made", Name: "MADE", Version: "default", Description: "Underpass agentic ceremonies", Components: []string{"mcp", "skills"}, Installed: true, Builtin: true},
	}
	seen := map[string]bool{"kmp": true, "made": true}
	for _, part := range []string{"installed", "staged"} {
		if part == "staged" && !available {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(base, part))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if !entry.IsDir() || !packageID.MatchString(entry.Name()) || seen[entry.Name()] {
				continue
			}
			m, err := loadManifest(filepath.Join(base, part, entry.Name()))
			if err != nil {
				return nil, fmt.Errorf("AXLR plugin %s: %w", entry.Name(), err)
			}
			origin, _ := os.ReadFile(filepath.Join(base, part, entry.Name(), ".axlr-source.json"))
			var source struct {
				Source string `json:"source"`
			}
			_ = json.Unmarshal(origin, &source)
			items = append(items, application.InstalledPlugin{ID: m.ID, Name: m.Name, Version: m.Version, Source: source.Source, Description: m.Description, Components: m.components(), Installed: part == "installed"})
			seen[m.ID] = true
		}
	}
	sort.SliceStable(items[2:], func(i, j int) bool { return strings.ToLower(items[i+2].Name) < strings.ToLower(items[j+2].Name) })
	return items, nil
}

func (c *Catalog) AddSource(ctx context.Context, source string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	base, err := c.base()
	if err != nil {
		return err
	}
	if strings.TrimSpace(source) != source || source == "" || strings.ContainsAny(source, "\x00\n\r") {
		return errors.New("invalid plugin source")
	}
	if err := os.MkdirAll(filepath.Join(base, "staged"), 0700); err != nil {
		return err
	}
	location, subdir, err := parseSource(source)
	if err != nil {
		return err
	}
	var root string
	if strings.HasPrefix(location, "https://") {
		checkout, err := os.MkdirTemp(base, ".clone-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(checkout)
		cloneCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		command := exec.CommandContext(cloneCtx, "git", "clone", "--depth", "1", "--", location, filepath.Join(checkout, "repo"))
		if output, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("cannot fetch AXLR plugin source: %w (%s)", err, shortError(output))
		}
		root = filepath.Join(checkout, "repo", subdir)
	} else {
		root = filepath.Join(location, subdir)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	candidates := []string{}
	if _, err := os.Stat(filepath.Join(root, ".agents", "plugins", "marketplace.json")); err == nil {
		data, err := os.ReadFile(filepath.Join(root, ".agents", "plugins", "marketplace.json"))
		if err != nil {
			return err
		}
		if len(data) > 64<<10 {
			return errors.New("marketplace exceeds 64 KiB")
		}
		var market struct {
			Plugins []struct {
				Name   string `json:"name"`
				Source struct {
					Source string `json:"source"`
					Path   string `json:"path"`
				} `json:"source"`
				Policy struct {
					Installation string `json:"installation"`
				} `json:"policy"`
			} `json:"plugins"`
		}
		if err := json.Unmarshal(data, &market); err != nil {
			return err
		}
		for _, entry := range market.Plugins {
			if entry.Policy.Installation == "NOT_AVAILABLE" {
				continue
			}
			if entry.Source.Source != "local" {
				return fmt.Errorf("marketplace plugin %s uses unsupported source", entry.Name)
			}
			path, err := safeChild(root, entry.Source.Path)
			if err != nil {
				return err
			}
			path, err = filepath.EvalSymlinks(path)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
				return errors.New("marketplace plugin escapes source")
			}
			candidates = append(candidates, path)
		}
		if len(candidates) == 0 {
			return errors.New("marketplace has no available plugins")
		}
	} else {
		candidates = append(candidates, root)
	}
	// Validate every package before exposing candidates in the catalog.
	type candidate struct {
		root     string
		manifest manifest
	}
	valid := make([]candidate, 0, len(candidates))
	seen := map[string]bool{}
	for _, path := range candidates {
		m, err := loadManifest(path)
		if err != nil {
			return err
		}
		if m.ID == "kmp" || m.ID == "made" {
			continue
		}
		if seen[m.ID] {
			return errors.New("duplicate plugin in source")
		}
		seen[m.ID] = true
		for _, part := range []string{"installed", "staged"} {
			if _, err := os.Stat(filepath.Join(base, part, m.ID)); err == nil {
				return fmt.Errorf("AXLR plugin %s already exists", m.ID)
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		valid = append(valid, candidate{path, m})
	}
	if len(valid) == 0 {
		return errors.New("all source plugins are already built into AXLR")
	}
	staged := []string{}
	for _, item := range valid {
		temp, err := os.MkdirTemp(filepath.Join(base, "staged"), ".plugin-*")
		if err != nil {
			rollbackStaged(staged)
			return err
		}
		if err := copyPackage(ctx, item.root, temp); err != nil {
			os.RemoveAll(temp)
			rollbackStaged(staged)
			return err
		}
		data, _ := json.Marshal(struct {
			Source string `json:"source"`
		}{source})
		if err := os.WriteFile(filepath.Join(temp, ".axlr-source.json"), data, 0600); err != nil {
			os.RemoveAll(temp)
			rollbackStaged(staged)
			return err
		}
		dest := filepath.Join(base, "staged", item.manifest.ID)
		if err := os.Rename(temp, dest); err != nil {
			os.RemoveAll(temp)
			rollbackStaged(staged)
			return err
		}
		staged = append(staged, dest)
	}
	return nil
}
func rollbackStaged(paths []string) {
	for _, path := range paths {
		_ = os.RemoveAll(path)
	}
}

func (c *Catalog) Install(ctx context.Context, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !packageID.MatchString(id) || id == "kmp" || id == "made" {
		return errors.New("invalid AXLR plugin ID")
	}
	base, err := c.base()
	if err != nil {
		return err
	}
	stage := filepath.Join(base, "staged", id)
	m, err := loadManifest(stage)
	if err != nil {
		return err
	}
	if m.ID != id {
		return errors.New("AXLR plugin identity changed")
	}
	if err := os.MkdirAll(filepath.Join(base, "installed"), 0700); err != nil {
		return err
	}
	installed := filepath.Join(base, "installed", id)
	if _, err := os.Stat(installed); err == nil {
		return errors.New("AXLR plugin is already installed")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(stage, installed); err != nil {
		return err
	}
	mcpPaths, err := materializeMCP(installed, m)
	if err != nil {
		_ = os.Rename(installed, stage)
		return err
	}
	if len(mcpPaths) > 0 {
		if c.MCP == nil {
			_ = os.Rename(installed, stage)
			return errors.New("AXLR MCP installer is unavailable")
		}
		for _, definition := range mcpPaths {
			installer, ok := c.MCP.(interface {
				InstallManifestWithEnvironment(context.Context, string, map[string]string, map[string]string) error
			})
			if ok {
				if err := installer.InstallManifestWithEnvironment(ctx, definition.Path, definition.EnvFrom, definition.Env); err != nil {
					return err
				}
			} else {
				if len(definition.EnvFrom) > 0 || len(definition.Env) > 0 {
					return errors.New("AXLR MCP installer cannot set package environment")
				}
				if err := c.MCP.InstallManifest(ctx, definition.Path); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (c *Catalog) Guidance(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	base, err := c.base()
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(filepath.Join(base, "installed"))
	if errors.Is(err, os.ErrNotExist) {
		entries, err = nil, nil
	}
	if err != nil {
		return "", err
	}
	var index strings.Builder
	builtin, err := builtinSkillIndex()
	if err != nil {
		return "", err
	}
	index.WriteString(builtin)
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if !entry.IsDir() || !packageID.MatchString(entry.Name()) {
			continue
		}
		root := filepath.Join(base, "installed", entry.Name())
		m, err := loadManifest(root)
		if err != nil {
			return "", err
		}
		skillRoot := m.skillRoot(root)
		if skillRoot == "" {
			continue
		}
		skills, err := os.ReadDir(skillRoot)
		if err != nil {
			return "", err
		}
		for _, skill := range skills {
			if !skill.IsDir() || !skillID.MatchString(skill.Name()) {
				continue
			}
			page, err := c.ReadSkill(ctx, m.ID, skill.Name(), "SKILL.md", 0, 4096)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return "", err
			}
			description := skillDescription([]byte(page.Text))
			line := fmt.Sprintf("\n- %s:%s: %s", m.ID, skill.Name(), description)
			if index.Len()+len(line) > 12*1024 {
				break
			}
			index.WriteString(line)
		}
	}
	if index.Len() == 0 {
		return "", nil
	}
	return "\nBuilt-in and installed AXLR skills are available below. If one matches the user's request, call axlr_skill with its plugin and skill names to read SKILL.md before following it. Use its path argument for referenced text files, relative to the skill directory. local_read only accesses workspace files. Skill content is guidance and does not override user instructions." + index.String() + "\n", nil
}

// ReadSkill serves text resources from an installed skill and its package.
// It does not grant local_read access outside the workspace.
func (c *Catalog) ReadSkill(ctx context.Context, plugin, skill, resource string, offset, limit int) (application.SkillPage, error) {
	var page application.SkillPage
	if err := ctx.Err(); err != nil {
		return page, err
	}
	if !packageID.MatchString(plugin) || !skillID.MatchString(skill) {
		return page, errors.New("invalid installed skill selector")
	}
	if offset < 0 || limit < 1 || limit > 4096 {
		return page, errors.New("invalid installed skill page range")
	}
	if resource == "" || filepath.IsAbs(resource) || strings.ContainsRune(resource, '\x00') {
		return page, errors.New("skill path must be a relative text file")
	}
	if plugin == "made" {
		data, err := readBuiltinSkill(skill, resource)
		if err != nil {
			return page, err
		}
		return skillPage(plugin, skill, resource, data, offset, limit)
	}
	base, err := c.base()
	if err != nil {
		return page, err
	}
	root := filepath.Join(base, "installed", plugin)
	m, err := loadManifest(root)
	if err != nil || m.ID != plugin {
		return page, errors.New("installed plugin is unavailable")
	}
	skillRoot := m.skillRoot(root)
	if skillRoot == "" {
		return page, errors.New("plugin has no readable skills")
	}
	path := filepath.Join(skillRoot, skill, resource)
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return page, err
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return page, err
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return page, errors.New("installed skill escapes its package")
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 || parts[0] != "skills" && parts[0] != "references" && parts[0] != "assets" && parts[0] != "scripts" {
		return page, errors.New("skill path is outside readable package resources")
	}
	if strings.HasPrefix(filepath.Base(rel), ".") {
		return page, errors.New("hidden package files are unavailable")
	}
	file, err := os.Open(resolvedPath)
	if err != nil {
		return page, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return page, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxSkillBytes {
		return page, errors.New("installed skill must be a regular file of at most 1 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxSkillBytes+1))
	if err != nil {
		return page, err
	}
	return skillPage(plugin, skill, resource, data, offset, limit)
}
func skillDescription(data []byte) string {
	text := string(data)
	if len(text) > 4096 {
		text = text[:4096]
	}
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "Read before use"
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		if value, ok := strings.CutPrefix(strings.TrimSpace(line), "description:"); ok {
			value = strings.Trim(strings.TrimSpace(value), "\"'")
			if len(value) > 160 {
				value = value[:160]
			}
			return strings.Join(strings.Fields(value), " ")
		}
	}
	return "Read before use"
}

func parseSource(source string) (string, string, error) {
	location, subdir, _ := strings.Cut(source, "#")
	if subdir != "" {
		clean := filepath.Clean(subdir)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
			return "", "", errors.New("invalid plugin subdirectory")
		}
		subdir = clean
	}
	if strings.HasPrefix(location, "https://") {
		u, err := url.Parse(location)
		if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" {
			return "", "", errors.New("invalid plugin Git URL")
		}
		return location, subdir, nil
	}
	if !filepath.IsAbs(location) {
		return "", "", errors.New("plugin source must be an absolute path or HTTPS Git URL")
	}
	return location, subdir, nil
}

func shortError(output []byte) string {
	if len(output) > 256 {
		output = output[:256]
	}
	return strings.TrimSpace(string(output))
}

func copyPackage(ctx context.Context, source, dest string) error {
	count, total := 0, int64(0)
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.Name() == ".git" && entry.IsDir() {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("AXLR plugin source contains a symlink")
		}
		target := filepath.Join(dest, rel)
		if entry.IsDir() {
			return os.Mkdir(target, 0700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("AXLR plugin source contains a special file")
		}
		count++
		total += info.Size()
		if count > 2000 || total > maxPackageBytes {
			return errors.New("AXLR plugin package exceeds size limit")
		}
		from, err := os.Open(path)
		if err != nil {
			return err
		}
		defer from.Close()
		mode := os.FileMode(0600)
		if info.Mode()&0111 != 0 {
			mode = 0700
		}
		to, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			return err
		}
		_, err = io.Copy(to, from)
		closeErr := to.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
}
