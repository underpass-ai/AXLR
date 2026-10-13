// Package engineupdate installs release-matched official packages for AXLR.
package engineupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

const maxArchive = 64 << 20

var stableVersion = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type Updater struct {
	Configuration application.EngineConfigurationPort
	Root          string
	Client        *http.Client
	// Commands registered by this running AXLR process, before any update.
	ActiveCommands map[string]string
	// Install, when set, installs and connects the engines mcp.json lacks;
	// without it they are skipped.
	Install application.EngineInstallPort
	// The release endpoint is fixed in production; tests use a local transport.
	apiBase  string
	platform string
}

type release struct {
	Tag        string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func (u *Updater) Update(ctx context.Context) ([]application.EngineUpdateResult, error) {
	if u.Configuration == nil || !filepath.IsAbs(u.Root) {
		return nil, errors.New("engine updater is not configured")
	}
	if _, ok := u.packagePlatform(); !ok {
		return nil, application.ErrEngineUpdatePlatform
	}
	targets, err := u.Configuration.EngineTargets(ctx)
	if err != nil {
		return nil, err
	}
	var results []application.EngineUpdateResult
	for _, engine := range []string{"made", "kmp"} {
		result := application.EngineUpdateResult{Engine: engine, Status: "skipped"}
		var target *application.EngineUpdateTarget
		for i := range targets {
			if targets[i].Engine == engine {
				target = &targets[i]
			}
		}
		switch {
		case target != nil && target.Command != "":
			operation, cancel := context.WithTimeout(ctx, 3*time.Minute)
			result, err = u.update(operation, *target)
			cancel()
			if err != nil {
				result.Status = "failed"
				result.Error = err.Error()
			}
		case target == nil && u.Install != nil:
			operation, cancel := context.WithTimeout(ctx, 5*time.Minute)
			result, err = u.install(operation, engine)
			cancel()
			if err != nil {
				result.Status = "failed"
				result.Error = err.Error()
			}
		}
		results = append(results, result)
		if ctx.Err() != nil {
			return results, ctx.Err()
		}
	}
	return results, nil
}

func (u *Updater) update(ctx context.Context, target application.EngineUpdateTarget) (application.EngineUpdateResult, error) {
	r := application.EngineUpdateResult{Engine: target.Engine}
	r.PreviousVersion = installedVersion(ctx, target)
	version, destination, err := u.fetchPackage(ctx, target.Engine, r.PreviousVersion)
	r.Version = version
	if err != nil {
		return r, err
	}
	launcher := filepath.Join(destination, "scripts", "run-embedded-mcp.sh")
	if target.Command == launcher {
		r.Status = "current"
		if active, known := u.ActiveCommands[target.Engine]; known {
			r.RestartRequired = active != launcher
		}
		return r, nil
	}
	if err := ctx.Err(); err != nil {
		return r, err
	}
	if err := u.Configuration.ActivateEngine(ctx, target, launcher); err != nil {
		return r, err
	}
	r.Status, r.RestartRequired = "updated", true
	return r, nil
}

// install downloads the stable release of an engine mcp.json lacks and hands
// the verified package to Install, which prepares its store and connects it.
func (u *Updater) install(ctx context.Context, engine string) (application.EngineUpdateResult, error) {
	r := application.EngineUpdateResult{Engine: engine}
	version, destination, err := u.fetchPackage(ctx, engine, "")
	r.Version = version
	if err != nil {
		return r, err
	}
	if err := ctx.Err(); err != nil {
		return r, err
	}
	detail, err := u.Install.InstallEngine(ctx, engine, destination)
	r.Detail = detail
	if err != nil {
		return r, err
	}
	r.Status, r.RestartRequired = "installed", true
	return r, nil
}

// fetchPackage downloads, verifies and unpacks the stable release of an
// engine into a content-derived directory under Root, and returns its version
// and that directory. A previous version newer than the release is kept.
func (u *Updater) fetchPackage(ctx context.Context, engine, previous string) (string, string, error) {
	platform, ok := u.packagePlatform()
	if !ok {
		return "", "", application.ErrEngineUpdatePlatform
	}
	base := u.apiBase
	if base == "" {
		base = "https://api.github.com"
	}
	data, err := u.fetch(ctx, base+"/repos/underpass-ai/"+engine+"/releases/latest", 2<<20)
	if err != nil {
		return "", "", err
	}
	var latest release
	if json.Unmarshal(data, &latest) != nil || latest.Draft || latest.Prerelease || !stableVersion.MatchString(latest.Tag) {
		return "", "", errors.New("invalid stable release metadata")
	}
	stable := strings.TrimPrefix(latest.Tag, "v")
	if previous != "" && compareVersions(previous, stable) > 0 {
		return stable, "", errors.New("installed version is newer than the stable release; keeping it")
	}
	name := engine + "-plugin-" + stable + "-" + platform + ".tar.gz"
	archiveURL, checksumURL := "", ""
	for _, asset := range latest.Assets {
		if asset.Name == name {
			archiveURL = asset.URL
		}
		if asset.Name == name+".sha256" {
			checksumURL = asset.URL
		}
	}
	if archiveURL == "" || checksumURL == "" {
		return stable, "", errors.New("release package or checksum is missing")
	}
	checksum, err := u.fetch(ctx, checksumURL, 4096)
	if err != nil {
		return stable, "", err
	}
	fields := strings.Fields(string(checksum))
	if len(fields) == 0 {
		return stable, "", errors.New("invalid release checksum")
	}
	expected, err := hex.DecodeString(fields[0])
	if err != nil || len(expected) != sha256.Size {
		return stable, "", errors.New("invalid release checksum")
	}
	archive, err := u.fetch(ctx, archiveURL, maxArchive)
	if err != nil {
		return stable, "", err
	}
	digest := sha256.Sum256(archive)
	if !bytes.Equal(digest[:], expected) {
		return stable, "", errors.New("release checksum mismatch; keeping the installed engine")
	}
	if err := os.MkdirAll(u.Root, 0700); err != nil {
		return stable, "", err
	}
	stage, err := os.MkdirTemp(u.Root, ".update-"+engine+"-")
	if err != nil {
		return stable, "", err
	}
	defer os.RemoveAll(stage)
	if err := extract(archive, stage, engine); err != nil {
		return stable, "", err
	}
	packageRoot := filepath.Join(stage, engine)
	manifest, err := os.ReadFile(filepath.Join(packageRoot, ".claude-plugin", "plugin.json"))
	var metadata struct {
		Version string `json:"version"`
	}
	if err != nil || json.Unmarshal(manifest, &metadata) != nil || metadata.Version != stable {
		return stable, "", errors.New("plugin version does not match the release")
	}
	binary := filepath.Join(packageRoot, "bin", engine+"-mcp")
	if version(ctx, binary, engine) != stable {
		return stable, "", errors.New("engine version does not match the release")
	}
	launcher := filepath.Join(packageRoot, "scripts", "run-embedded-mcp.sh")
	if info, err := os.Stat(launcher); err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return stable, "", errors.New("release launcher is missing or not executable")
	}
	// A content-derived directory avoids replacing files used by a running process.
	destination := filepath.Join(u.Root, engine+"-"+stable+"-"+hex.EncodeToString(digest[:8]))
	if _, err := os.Lstat(destination); err == nil {
		if err := sameTree(packageRoot, destination); err != nil {
			return stable, "", err
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.Rename(packageRoot, destination); err != nil {
			return stable, "", err
		}
	} else {
		return stable, "", err
	}
	return stable, destination, nil
}

func (u *Updater) fetch(ctx context.Context, address string, limit int64) ([]byte, error) {
	// Only the official repository is allowed to supply executable packages.
	if !strings.HasPrefix(address, "https://api.github.com/") && !strings.HasPrefix(address, "https://github.com/underpass-ai/") {
		return nil, errors.New("release URL is not an official GitHub URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AXLR-engine-updater")
	client := u.Client
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	secured := *client
	secured.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return errors.New("insecure release redirect")
		}
		if client.CheckRedirect != nil {
			return client.CheckRedirect(req, via)
		}
		if len(via) >= 10 {
			return errors.New("too many release redirects")
		}
		return nil
	}
	response, err := secured.Do(req)
	if err != nil {
		return nil, errors.New("cannot download release: network request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release download returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("release download exceeds size limit")
	}
	return data, nil
}

func version(ctx context.Context, binary, engine string) string {
	probe, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probe, binary, "--version")
	var output boundedOutput
	cmd.Stdout, cmd.Stderr = &output, io.Discard
	if cmd.Run() != nil {
		return ""
	}
	fields := strings.Fields(output.String())
	if len(fields) >= 2 && fields[0] == engine+"-mcp" && stableVersion.MatchString(fields[1]) {
		return strings.TrimPrefix(fields[1], "v")
	}
	return ""
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() < 4096 {
		b.Buffer.Write(p[:min(len(p), 4096-b.Len())])
	}
	return n, nil
}

func installedVersion(ctx context.Context, target application.EngineUpdateTarget) string {
	binary := target.Command
	if filepath.Base(binary) != target.Engine+"-mcp" {
		binary = filepath.Join(filepath.Dir(filepath.Dir(binary)), "bin", target.Engine+"-mcp")
		if _, err := os.Stat(binary); err != nil {
			binary, _ = exec.LookPath(target.Engine + "-mcp")
		}
	}
	return version(ctx, binary, target.Engine)
}

func compareVersions(a, b string) int {
	left, right := strings.Split(a, "."), strings.Split(b, ".")
	for i := range 3 {
		x, _ := strconv.ParseUint(left[i], 10, 64)
		y, _ := strconv.ParseUint(right[i], 10, 64)
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}

func extract(data []byte, directory, engine string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	var total int64
	seen := map[string]bool{}
	for count := 0; ; count++ {
		h, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(h.Name, "/")
		if count >= 2000 || name != path.Clean(name) || strings.Contains(name, "\\") || (name != engine && !strings.HasPrefix(name, engine+"/")) || seen[name] {
			return errors.New("unsafe release archive path")
		}
		seen[name] = true
		file := filepath.Join(directory, filepath.FromSlash(name))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(file, 0700); err != nil {
				return err
			}
		case tar.TypeReg:
			total += h.Size
			if h.Size < 0 || h.Size > maxArchive || total > 256<<20 {
				return errors.New("release archive exceeds size limit")
			}
			if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
				return err
			}
			mode := os.FileMode(0600)
			if h.Mode&0111 != 0 {
				mode = 0700
			}
			out, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			_, copyErr := io.CopyN(out, reader, h.Size)
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		default:
			return errors.New("release archive contains a link or unsupported entry")
		}
	}
}

func sameTree(source, destination string) error {
	info, err := os.Lstat(destination)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("existing engine package is not a directory")
	}
	return filepath.WalkDir(source, func(file string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, _ := filepath.Rel(source, file)
		other := filepath.Join(destination, relative)
		info, err := os.Lstat(other)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || info.IsDir() != entry.IsDir() {
			return errors.New("existing engine package differs from the verified release")
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return errors.New("existing engine package contains a special file")
		}
		expected, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		actual, err := os.ReadFile(other)
		if err != nil || !bytes.Equal(expected, actual) {
			return errors.New("existing engine package differs from the verified release")
		}
		original, err := entry.Info()
		if err != nil || original.Mode().Perm() != info.Mode().Perm() {
			return errors.New("existing engine package permissions differ from the verified release")
		}
		return nil
	})
}

// packagePlatform names the official MADE/KMP package for this host. Packages
// ship POSIX launchers and exist only for these targets.
func (u *Updater) packagePlatform() (string, bool) {
	if u.platform != "" {
		return u.platform, true
	}
	return officialPlatform(runtime.GOOS, runtime.GOARCH)
}

func officialPlatform(goos, goarch string) (string, bool) {
	switch goos + "/" + goarch {
	case "linux/amd64":
		return "linux-x86_64", true
	case "linux/arm64":
		return "linux-arm64", true
	case "darwin/arm64":
		return "macos-arm64", true
	}
	return "", false
}
