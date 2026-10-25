//go:build !windows

package main

// systemPath is the PATH a command runs with: the inherited absolute
// folders, or the usual system folders when it names none.
func systemPath(paths []string, _ func(string) string) []string {
	if len(paths) == 0 {
		return []string{"/usr/local/bin", "/usr/bin", "/bin"}
	}
	return paths
}

// systemEnvironment is what a command needs of the host beyond PATH and
// HOME: nothing on Unix.
func systemEnvironment(func(string) string) []string { return nil }
