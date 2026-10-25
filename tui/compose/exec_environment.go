package compose

import (
	"os"
	"path/filepath"
	"strings"
)

// ExecEnvironment is the environment local commands run with, in the
// console and in the service alike: PATH to resolve them and HOME, and no
// other ambient variable (in particular no model provider credentials).
func ExecEnvironment(getenv func(string) string) []string {
	paths := make([]string, 0)
	seen := map[string]bool{}
	for _, dir := range filepath.SplitList(getenv("PATH")) {
		if filepath.IsAbs(dir) && !seen[dir] {
			paths = append(paths, dir)
			seen[dir] = true
		}
	}
	// The system's own folders when the inherited PATH names none of them
	// (exec_environment_unix.go, exec_environment_windows.go).
	paths = systemPath(paths, getenv)
	env := []string{"PATH=" + strings.Join(paths, string(os.PathListSeparator))}
	if home := getenv("HOME"); filepath.IsAbs(home) {
		env = append(env, "HOME="+home)
	}
	return append(env, systemEnvironment(getenv)...)
}
