package main

import (
	"github.com/underpass-ai/AXLR/tui/adapters/repairbuild"
)

// goVariables are the host's Go settings a repair build keeps, so it uses
// the same caches, proxy and toolchain as the person's own builds.
var goVariables = []string{"GOPATH", "GOCACHE", "GOMODCACHE", "GOPROXY", "GOPRIVATE", "GONOSUMDB", "GONOPROXY", "GOSUMDB", "GOTOOLCHAIN", "GOFLAGS", "GOROOT",
	// Windows: Go finds its caches and temporary directory through these.
	"LOCALAPPDATA", "APPDATA", "USERPROFILE", "SystemRoot", "TEMP", "TMP"}

// repairBuilder builds repaired consoles with the console's restricted
// environment plus the host's Go settings; never a model or plugin key.
func repairBuilder(getenv func(string) string) repairbuild.Builder {
	env := localRuntimeEnvironment(getenv)
	for _, name := range goVariables {
		if value := getenv(name); value != "" {
			env = append(env, name+"="+value)
		}
	}
	return repairbuild.Builder{Env: env}
}
