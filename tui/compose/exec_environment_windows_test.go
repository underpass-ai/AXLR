//go:build windows

package compose

import (
	"strings"
	"testing"
)

// An empty PATH runs commands with the Windows system folders; a PATH that
// lacks them keeps its own folders first and gains the missing ones, in any
// case; SystemRoot and USERPROFILE go along.
func TestExecEnvironmentDefaultsTheWindowsPath(t *testing.T) {
	system := `D:\Win\System32;D:\Win;D:\Win\System32\Wbem;D:\Win\System32\WindowsPowerShell\v1.0`
	for raw, want := range map[string]string{
		"":                                 "PATH=" + system,
		`relative\bin`:                     "PATH=" + system,
		`C:\Tools`:                         `PATH=C:\Tools;` + system,
		`d:\win\system32\;C:\Tools;D:\Win`: `PATH=d:\win\system32\;C:\Tools;D:\Win;D:\Win\System32\Wbem;D:\Win\System32\WindowsPowerShell\v1.0`,
	} {
		env := ExecEnvironment(func(name string) string {
			switch name {
			case "PATH":
				return raw
			case "SystemRoot":
				return `D:\Win`
			case "USERPROFILE":
				return `C:\Users\ana`
			}
			return ""
		})
		if len(env) != 3 || env[0] != want || env[1] != `SystemRoot=D:\Win` || env[2] != `USERPROFILE=C:\Users\ana` {
			t.Fatalf("PATH %q: %v", raw, env)
		}
	}
	env := ExecEnvironment(func(name string) string { return "" })
	if !strings.HasPrefix(env[0], `PATH=C:\Windows\System32;C:\Windows;`) || env[1] != `SystemRoot=C:\Windows` || len(env) != 2 {
		t.Fatalf("without SystemRoot: %v", env)
	}
	env = ExecEnvironment(func(name string) string {
		if name == "windir" {
			return `E:\Windows`
		}
		return ""
	})
	if !strings.HasPrefix(env[0], `PATH=E:\Windows\System32;`) || env[1] != `SystemRoot=E:\Windows` {
		t.Fatalf("from windir: %v", env)
	}
}
