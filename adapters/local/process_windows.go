//go:build windows

package local

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func pathEnvironment(value string) bool {
	return len(value) >= len("PATH=") && strings.EqualFold(value[:len("PATH=")], "PATH=")
}

func programCandidates(path string) []string {
	if filepath.Ext(path) != "" {
		return []string{path}
	}
	return []string{path + ".exe", path + ".com", path + ".bat", path + ".cmd"}
}

func executableFile(path string, info os.FileInfo) bool {
	if !info.Mode().IsRegular() {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".exe", ".com", ".bat", ".cmd":
		return true
	default:
		return false
	}
}

func configureProcess(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return killProcessTree(cmd) }
}

func killProcessTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	return cmd.Process.Kill()
}
