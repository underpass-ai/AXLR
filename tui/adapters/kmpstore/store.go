// Package kmpstore chooses and prepares the KMP store a console opens for a
// workspace: the checkout's own .kernel inside a git repository, the
// per-user default outside one, or the directory an operator pinned in
// mcp.json. A store the console creates receives the repository's committed
// bundle and the KMP guide, so a clone on another machine starts with the
// project's memory (decision D1 of the lifecycle integration design).
package kmpstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// DataDirVariable pins the store of a kmp-mcp process.
	DataDirVariable = "KMP_MCP_DATA_DIR"
	// BackendVariable selects the embedded kernel.
	BackendVariable = "KMP_MCP_BACKEND"
	// StoreDirectory is the project store inside a checkout, the one KMP's
	// own precedence chooses for a git repository.
	StoreDirectory = ".kernel"
	// BundlePath is the committed memory of a repository, relative to its
	// top level, as kmp-mcp export maintains it and kmp-mcp import restores it.
	BundlePath = ".kmp/memory.jsonl"
	// Launcher is the basename of a KMP package's embedded launcher; the
	// engine binary sits beside it in bin/.
	Launcher = "run-embedded-mcp.sh"
)

// Reason says why a store was chosen.
type Reason string

const (
	// ReasonOperator is a KMP_MCP_DATA_DIR an operator set in mcp.json.
	ReasonOperator Reason = "operator"
	// ReasonProject is the checkout's .kernel.
	ReasonProject Reason = "project"
	// ReasonUser is the per-user default outside a checkout.
	ReasonUser Reason = "user"
)

// Selection is the store of one console launch.
type Selection struct {
	Path   string
	Reason Reason
	// TopLevel is the git checkout's top level for a project store.
	TopLevel string
	// Replaced is the installer's old per-user pin the project store
	// overrides, empty when there was none.
	Replaced string
}

// UserDefault is KMP's per-user default store under the data directory.
func UserDefault(dataBase string) string { return filepath.Join(dataBase, "kmp", "default") }

// Resolve chooses the store for a workspace from the registration's
// environment: a pinned directory other than the per-user default is the
// operator's and wins; the installer's pin of the per-user default (what it
// wrote until 13 October 2026) is replaced by the checkout's .kernel when
// the workspace is inside a git repository; outside one the per-user default
// is used.
func Resolve(workspace string, env []string, dataBase string) Selection {
	pinned := Value(env, DataDirVariable)
	user := UserDefault(dataBase)
	if pinned != "" && filepath.Clean(pinned) != filepath.Clean(user) {
		return Selection{Path: pinned, Reason: ReasonOperator}
	}
	if top, ok := GitTopLevel(workspace); ok {
		return Selection{Path: filepath.Join(top, StoreDirectory), Reason: ReasonProject, TopLevel: top, Replaced: pinned}
	}
	return Selection{Path: user, Reason: ReasonUser}
}

// GitTopLevel is the nearest directory at or above the workspace that holds
// a .git directory or file (a worktree or submodule has a file).
func GitTopLevel(workspace string) (string, bool) {
	dir := filepath.Clean(workspace)
	for {
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil && (info.IsDir() || info.Mode().IsRegular()) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// Value reads one variable from a KEY=VALUE environment.
func Value(env []string, key string) string {
	for _, entry := range env {
		if name, value, ok := strings.Cut(entry, "="); ok && name == key {
			return value
		}
	}
	return ""
}

// WithStore pins the store in a KEY=VALUE environment and selects the
// embedded backend unless one is set; the result is sorted, as mcp.json's
// loader sorts.
func WithStore(env []string, path string) []string {
	out := make([]string, 0, len(env)+2)
	backend := false
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if name == DataDirVariable {
			continue
		}
		if name == BackendVariable {
			backend = true
		}
		out = append(out, entry)
	}
	out = append(out, DataDirVariable+"="+path)
	if !backend {
		out = append(out, BackendVariable+"=embedded")
	}
	sort.Strings(out)
	return out
}

// Binary is the kmp-mcp executable behind a registration's command and the
// plugin root that holds the guide assets: a package launcher gives both,
// a bare binary gives itself and no root.
func Binary(command string) (binary, pluginRoot string) {
	if filepath.Base(command) != Launcher {
		return command, ""
	}
	root := filepath.Dir(filepath.Dir(command))
	binary = filepath.Join(root, "bin", "kmp-mcp")
	if info, err := os.Stat(filepath.Join(root, "guide", "guide.requests.json")); err == nil && info.Mode().IsRegular() {
		return binary, root
	}
	return binary, ""
}

// configurationFiles are the store's own settings a new project store copies
// from the person's per-user store, so the project is ready to use the way
// the person already works (decision B7). Indexes, agents and judgements are
// not copied: KMP rebuilds them.
var configurationFiles = []string{"typesafe.json", "rerank.json", "wake-focus.json", "write-relations.json", "judgement-book.json", "curate.json", "ask-judge.json", "write-expansions.json", "telemetry-salt"}

// Preparer creates a store the console chose and fills it.
type Preparer struct {
	// Binary runs kmp-mcp for the bundle import and the guide sync; empty
	// prepares the directory only.
	Binary string
	// PluginRoot holds guide/guide.requests.json; empty syncs no guide.
	PluginRoot string
	// UserStore is the per-user default whose configuration a new project
	// store copies; empty, or the selected store itself, copies nothing.
	UserStore string
	// Run executes one command; nil runs it with a two-minute limit.
	Run func(ctx context.Context, command string, args, env []string) ([]byte, error)
}

// Report says what Prepare did.
type Report struct {
	// Created is true when the directory did not hold a store yet.
	Created bool
	// Imported is the number of events restored from the bundle, -1 when
	// a bundle was imported but its receipt was unreadable, 0 without one.
	Imported int
	// Configured lists the configuration files copied from the user store.
	Configured []string
	// GuideSynced is true when the guide was written into the new store.
	GuideSynced bool
}

// Exists reports whether a directory holds a KMP store: its format marker.
func Exists(path string) bool {
	info, err := os.Stat(filepath.Join(path, "FORMAT_VERSION"))
	return err == nil && info.Mode().IsRegular()
}

// Prepare creates the selected store when missing: the directory with KMP's
// self-ignoring .gitignore, the checkout's bundle imported into it (import
// needs an empty store, so it comes first), the user store's configuration,
// then the guide. An existing store is left as it is. env is the
// registration's child environment; the store is pinned into it.
func (p Preparer) Prepare(ctx context.Context, selection Selection, env []string) (Report, error) {
	var report Report
	if selection.Path == "" || !filepath.IsAbs(selection.Path) {
		return report, errors.New("an absolute KMP store path is required")
	}
	if Exists(selection.Path) {
		return report, nil
	}
	report.Created = true
	if err := os.MkdirAll(selection.Path, 0o755); err != nil {
		return report, err
	}
	if ignore := filepath.Join(selection.Path, ".gitignore"); !fileExists(ignore) {
		if err := os.WriteFile(ignore, []byte("*\n"), 0o644); err != nil {
			return report, err
		}
	}
	child := WithStore(env, selection.Path)
	if bundle := filepath.Join(selection.TopLevel, filepath.FromSlash(BundlePath)); selection.TopLevel != "" && p.Binary != "" && fileExists(bundle) {
		out, err := p.run(ctx, p.Binary, []string{"import", bundle}, child)
		if err != nil {
			return report, fmt.Errorf("import %s: %w", bundle, err)
		}
		report.Imported = importedEvents(out)
	}
	if p.UserStore != "" && filepath.Clean(p.UserStore) != filepath.Clean(selection.Path) {
		for _, name := range configurationFiles {
			source := filepath.Join(p.UserStore, name)
			if !fileExists(source) {
				continue
			}
			if err := copyFile(source, filepath.Join(selection.Path, name)); err != nil {
				return report, err
			}
			report.Configured = append(report.Configured, name)
		}
	}
	if p.Binary != "" && p.PluginRoot != "" {
		if _, err := p.run(ctx, p.Binary, []string{"guide", "sync", "--plugin-root", p.PluginRoot}, child); err != nil {
			return report, fmt.Errorf("synchronise the KMP guide: %w", err)
		}
		report.GuideSynced = true
	}
	return report, nil
}

// importedEvents reads kmp-mcp import's receipt, {"events_imported":N} on
// its last line; -1 when it is not there.
func importedEvents(out []byte) int {
	lines := bytes.Split(bytes.TrimSpace(out), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		var receipt struct {
			Events *int `json:"events_imported"`
		}
		if json.Unmarshal(bytes.TrimSpace(lines[i]), &receipt) == nil && receipt.Events != nil {
			return *receipt.Events
		}
	}
	return -1
}

func (p Preparer) run(ctx context.Context, command string, args, env []string) ([]byte, error) {
	if p.Run != nil {
		return p.Run(ctx, command, args, env)
	}
	if info, err := os.Stat(command); err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return nil, fmt.Errorf("%s is missing or not executable", filepath.Base(command))
	}
	operation, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(operation, command, args...)
	cmd.Env = env
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		return output.Bytes(), fmt.Errorf("%s %s failed: %s", filepath.Base(command), strings.Join(args, " "), tail(output.String()))
	}
	return output.Bytes(), nil
}

func tail(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) > 6 {
		lines = lines[len(lines)-6:]
	}
	text := strings.Join(lines, " · ")
	if len(text) > 800 {
		text = text[len(text)-800:]
	}
	if text == "" {
		return "no output"
	}
	return text
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func copyFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// BundleAbout is the project about a checkout's bundle names: its header
// lists the abouts it carries. One project about is taken as it is; among
// several, the one equal to the directory's name ignoring case; otherwise
// none, and the directory rule applies (decision D2).
func BundleAbout(topLevel string) (string, error) {
	if topLevel == "" {
		return "", nil
	}
	file, err := os.Open(filepath.Join(topLevel, filepath.FromSlash(BundlePath)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	defer file.Close()
	line, err := readLine(file)
	if err != nil {
		return "", err
	}
	var header struct {
		Abouts []string `json:"abouts"`
	}
	if err := json.Unmarshal(line, &header); err != nil {
		return "", fmt.Errorf("unreadable bundle header: %w", err)
	}
	var projects []string
	for _, about := range header.Abouts {
		if strings.HasPrefix(about, "project:") {
			projects = append(projects, about)
		}
	}
	switch len(projects) {
	case 0:
		return "", nil
	case 1:
		return projects[0], nil
	}
	name := filepath.Base(topLevel)
	for _, about := range projects {
		if strings.EqualFold(strings.TrimPrefix(about, "project:"), name) {
			return about, nil
		}
	}
	return "", nil
}

// readLine reads the first line of a file, at most 64 KiB.
func readLine(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, 64<<10))
	if err != nil {
		return nil, err
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		data = data[:i]
	}
	return bytes.TrimSpace(data), nil
}
