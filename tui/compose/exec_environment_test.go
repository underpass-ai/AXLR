package compose

import (
	"strings"
	"testing"
)

func TestExecEnvironmentSkipsRelativeHome(t *testing.T) {
	env := map[string]string{"PATH": "/usr/bin", "HOME": "relative/home"}
	for _, value := range ExecEnvironment(func(name string) string { return env[name] }) {
		if strings.HasPrefix(value, "HOME=") {
			t.Fatalf("relative HOME forwarded: %q", value)
		}
	}
}
