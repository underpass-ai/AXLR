//go:build windows

package compose

import (
	"path/filepath"
	"strings"
)

// systemPath is the PATH a command runs with: the inherited absolute
// folders, then the Windows system folders it lacks, so cmd, where,
// PowerShell and the other system programs resolve whatever the console or
// the service inherited. An empty PATH, as a Windows service or a
// scheduled task may give, leaves the system folders alone.
func systemPath(paths []string, getenv func(string) string) []string {
	root := systemRoot(getenv)
	for _, dir := range []string{filepath.Join(root, "System32"), root, filepath.Join(root, "System32", "Wbem"), filepath.Join(root, "System32", "WindowsPowerShell", "v1.0")} {
		if !containsFolder(paths, dir) {
			paths = append(paths, dir)
		}
	}
	return paths
}

// systemEnvironment is what Windows programs need of the host beyond PATH:
// SystemRoot, without which the network and crypto libraries of many
// programs fail, and USERPROFILE, Windows' HOME, when it is absolute.
func systemEnvironment(getenv func(string) string) []string {
	env := []string{"SystemRoot=" + systemRoot(getenv)}
	if profile := getenv("USERPROFILE"); filepath.IsAbs(profile) {
		env = append(env, "USERPROFILE="+profile)
	}
	return env
}

// systemRoot is the Windows folder: SystemRoot, else windir, else
// C:\Windows.
func systemRoot(getenv func(string) string) string {
	for _, name := range []string{"SystemRoot", "windir"} {
		if root := getenv(name); filepath.IsAbs(root) {
			return filepath.Clean(root)
		}
	}
	return `C:\Windows`
}

// containsFolder compares folders as Windows does: without case and
// without a trailing separator.
func containsFolder(paths []string, dir string) bool {
	for _, path := range paths {
		if strings.EqualFold(filepath.Clean(path), filepath.Clean(dir)) {
			return true
		}
	}
	return false
}
